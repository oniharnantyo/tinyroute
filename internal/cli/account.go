package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/cli/interactive"
	"github.com/oniharnantyo/tinyroute/internal/config"
	"github.com/oniharnantyo/tinyroute/internal/credential"
	"github.com/oniharnantyo/tinyroute/internal/dialect"
	"github.com/oniharnantyo/tinyroute/internal/route"
	"github.com/urfave/cli/v3"
)

func cmdProvidersAccount() *cli.Command {
	return &cli.Command{
		Name:  "account",
		Usage: "Manage accounts and credentials for provider multi-account routing",
		Commands: []*cli.Command{
			{
				Name:      "add",
				Usage:     "Add a credentialed account to a provider",
				ArgsUsage: "[provider] [name]",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "account",
						Aliases: []string{"a"},
						Usage:   "account name",
					},
					&cli.StringFlag{
						Name:    "type",
						Aliases: []string{"t"},
						Usage:   "credential type (static or oauth_refresh)",
					},
					&cli.StringFlag{
						Name:    "key",
						Aliases: []string{"k"},
						Usage:   "API key value for static credentials",
					},
					&cli.BoolFlag{
						Name:    "interactive",
						Aliases: []string{"i"},
						Usage:   "enable interactive wizard",
						Value:   true,
					},
					&cli.BoolFlag{
						Name:  "no-interactive",
						Usage: "disable interactive prompts",
					},
					&cli.BoolFlag{
						Name:  "force",
						Usage: "skip interactive prompts",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					isInteractive := cmd.Bool("interactive") && !cmd.Bool("no-interactive") && !cmd.Bool("force")
					return cmdAccountAdd(ctx, cmd, isInteractive)
				},
			},
			{
				Name:      "list",
				Usage:     "List accounts and credential status for a provider",
				ArgsUsage: "[provider]",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "interactive",
						Aliases: []string{"i"},
						Usage:   "enable interactive selection",
						Value:   true,
					},
					&cli.BoolFlag{
						Name:  "no-interactive",
						Usage: "disable interactive selection",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					isInteractive := cmd.Bool("interactive") && !cmd.Bool("no-interactive")
					return cmdAccountList(cmd.Args().Slice(), isInteractive)
				},
			},
			{
				Name:      "remove",
				Usage:     "Remove an account from a provider",
				ArgsUsage: "[provider] [name]",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "interactive",
						Aliases: []string{"i"},
						Usage:   "enable interactive selection",
						Value:   true,
					},
					&cli.BoolFlag{
						Name:  "no-interactive",
						Usage: "disable interactive selection",
					},
					&cli.BoolFlag{
						Name:  "force",
						Usage: "skip interactive confirmation",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					isInteractive := cmd.Bool("interactive") && !cmd.Bool("no-interactive") && !cmd.Bool("force")
					return cmdAccountRemove(cmd.Args().Slice(), isInteractive)
				},
			},
			{
				Name:      "test",
				Usage:     "Test health/connectivity probe for a provider account",
				ArgsUsage: "[provider] [name]",
				Flags: []cli.Flag{
					&cli.DurationFlag{
						Name:  "timeout",
						Usage: "probe timeout",
						Value: 10 * time.Second,
					},
					&cli.BoolFlag{
						Name:    "interactive",
						Aliases: []string{"i"},
						Usage:   "enable interactive selection",
						Value:   true,
					},
					&cli.BoolFlag{
						Name:  "no-interactive",
						Usage: "disable interactive selection",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					isInteractive := cmd.Bool("interactive") && !cmd.Bool("no-interactive")
					timeout := cmd.Duration("timeout")
					return cmdAccountTest(ctx, cmd.Args().Slice(), timeout, isInteractive)
				},
			},
			{
				Name:      "select",
				Usage:     "Set account selection strategy for a provider (round_robin, fill_first, sticky, sticky_round_robin)",
				ArgsUsage: "[provider]",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "strategy",
						Aliases: []string{"s"},
						Usage:   "account selection strategy (round_robin, fill_first, sticky, sticky_round_robin)",
					},
					&cli.BoolFlag{
						Name:    "interactive",
						Aliases: []string{"i"},
						Usage:   "enable interactive selection",
						Value:   true,
					},
					&cli.BoolFlag{
						Name:  "no-interactive",
						Usage: "disable interactive selection",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					isInteractive := cmd.Bool("interactive") && !cmd.Bool("no-interactive")
					strategy := cmd.String("strategy")
					return cmdAccountSelect(cmd.Args().Slice(), strategy, isInteractive)
				},
			},
			{
				Name:      "import",
				Usage:     "Import multiple accounts into a provider from file or stdin",
				ArgsUsage: "[provider]",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "file",
						Aliases: []string{"f"},
						Usage:   "file path containing accounts (name|key lines or JSON array)",
					},
					&cli.BoolFlag{
						Name:    "interactive",
						Aliases: []string{"i"},
						Usage:   "enable interactive selection",
						Value:   true,
					},
					&cli.BoolFlag{
						Name:  "no-interactive",
						Usage: "disable interactive selection",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					isInteractive := cmd.Bool("interactive") && !cmd.Bool("no-interactive")
					filePath := cmd.String("file")
					return cmdAccountImport(cmd.Args().Slice(), filePath, isInteractive)
				},
			},
		},
	}
}

func cmdAccountAdd(ctx context.Context, cmd *cli.Command, isInteractive bool) error {
	svc, err := config.LoadService()
	if err != nil {
		return fmt.Errorf("load service config: %w", err)
	}
	data, err := os.ReadFile(svc.ConfigPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", svc.ConfigPath, err)
	}
	topo, err := config.ParseRawTopology(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", svc.ConfigPath, err)
	}

	args := cmd.Args().Slice()
	var providerName, accountName string
	if len(args) > 0 {
		providerName = args[0]
	}
	if len(args) > 1 {
		accountName = args[1]
	}
	if accountName == "" {
		accountName = cmd.String("account")
	}

	if providerName == "" {
		if !isInteractive {
			return errors.New("usage: tinyroute providers account add <provider> <name>")
		}
		providers := make([]string, 0, len(topo.Providers))
		for p := range topo.Providers {
			providers = append(providers, p)
		}
		sort.Strings(providers)
		if len(providers) == 0 {
			return errors.New("no providers configured; add one first with `tinyroute providers add`")
		}
		providerName, err = interactive.Select("Select provider for account:", providers)
		if err != nil {
			return fmt.Errorf("select provider: %w", err)
		}
	}

	p, ok := topo.Providers[providerName]
	if !ok {
		return fmt.Errorf("unknown provider %q", providerName)
	}

	if accountName == "" {
		if !isInteractive {
			return errors.New("account name is required")
		}
		accountName, err = interactive.Input("Account name (e.g. key-1, prod, backup):", "", nil)
		if err != nil {
			return fmt.Errorf("read account name: %w", err)
		}
		accountName = strings.TrimSpace(accountName)
	}

	if err := credential.ValidateAccountName(accountName); err != nil {
		return fmt.Errorf("invalid account name: %w", err)
	}

	for _, acc := range p.Accounts {
		if acc.Name == accountName {
			return fmt.Errorf("account %q already exists for provider %q", accountName, providerName)
		}
	}

	credType := cmd.String("type")
	if credType == "" {
		if isInteractive {
			credType, err = interactive.Select("Credential type:", []string{"static", "oauth_refresh"})
			if err != nil {
				return fmt.Errorf("select credential type: %w", err)
			}
		} else {
			credType = "static"
		}
	}

	acc := config.Account{
		Name: accountName,
		Type: credType,
	}

	if credType == "static" {
		apiKey := cmd.String("key")
		if apiKey == "" {
			if isInteractive {
				apiKey, err = interactive.Password(fmt.Sprintf("Enter API key for %s/%s:", providerName, accountName))
				if err != nil {
					return fmt.Errorf("read API key: %w", err)
				}
			} else {
				return errors.New("--key is required for static account in non-interactive mode")
			}
		}
		acc.APIKey = apiKey
	} else if credType == "oauth_refresh" {
		if isInteractive {
			fmt.Printf("Delegating to OAuth login for provider %q account %q...\n", providerName, accountName)
		}
		loginArgs := []string{providerName}
		if err := cmdAuthLoginWithAccount(ctx, loginArgs, accountName, isInteractive); err != nil {
			return fmt.Errorf("oauth login failed: %w", err)
		}
	} else {
		return fmt.Errorf("invalid credential type %q", credType)
	}

	p = p.UpsertAccount(acc)
	topo.Providers[providerName] = p

	if err := config.WriteTopology(svc.ConfigPath, topo); err != nil {
		return fmt.Errorf("write topology: %w", err)
	}

	fmt.Printf("added account %q to provider %q\n", accountName, providerName)
	return nil
}

func cmdAccountList(args []string, isInteractive bool) error {
	svc, err := config.LoadService()
	if err != nil {
		return fmt.Errorf("load service config: %w", err)
	}
	data, err := os.ReadFile(svc.ConfigPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", svc.ConfigPath, err)
	}
	topo, err := config.ParseRawTopology(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", svc.ConfigPath, err)
	}

	var providerName string
	if len(args) > 0 {
		providerName = args[0]
	}

	if providerName == "" {
		if isInteractive && len(topo.Providers) > 0 {
			providers := make([]string, 0, len(topo.Providers))
			for p := range topo.Providers {
				providers = append(providers, p)
			}
			sort.Strings(providers)
			providerName, err = interactive.Select("Select provider to list accounts:", providers)
			if err != nil {
				return fmt.Errorf("select provider: %w", err)
			}
		}
	}

	home, _ := os.UserHomeDir()
	credStore, _ := credential.NewStore(filepath.Join(home, ".tinyroute", "credentials.json"))

	healthPath := filepath.Join(home, ".tinyroute", "state.json")
	healthStore := route.NewHealthStore(route.RealClock{}, healthPath)
	_ = healthStore.Load()

	if providerName != "" {
		p, ok := topo.Providers[providerName]
		if !ok {
			return fmt.Errorf("unknown provider %q", providerName)
		}
		printProviderAccounts(providerName, p, credStore, healthStore)
		return nil
	}

	// List all providers if no specific provider requested
	providerNames := make([]string, 0, len(topo.Providers))
	for p := range topo.Providers {
		providerNames = append(providerNames, p)
	}
	sort.Strings(providerNames)

	for _, pName := range providerNames {
		printProviderAccounts(pName, topo.Providers[pName], credStore, healthStore)
		fmt.Println()
	}
	return nil
}

func printProviderAccounts(providerName string, p config.Provider, credStore *credential.Store, healthStore *route.HealthStore) {
	w := newTabWriter()
	fmt.Fprintf(w, "PROVIDER\tACCOUNT\tTYPE\tCREDENTIAL\tACTIVE COOLDOWNS\n")
	strategy := p.Selection
	if strategy == "" {
		strategy = config.StrategyRoundRobin
	}
	fmt.Fprintf(w, "[%s strategy=%s]\t\t\t\t\n", providerName, strategy)

	activeCDs := healthStore.ActiveCooldowns()

	if len(p.Accounts) == 0 {
		// Print default/single-key credential status
		credStatus := "none"
		if p.APIKey != "" {
			credStatus = maskKey(p.APIKey)
		} else if credStore != nil {
			if rec, ok := credStore.Get(providerName); ok {
				credStatus = fmt.Sprintf("oauth(%s)", rec.Masked().RefreshToken)
			}
		}
		cdCount := 0
		for k := range activeCDs {
			if k == providerName || strings.HasPrefix(k, providerName+"/") || strings.HasPrefix(k, providerName+"#") {
				cdCount++
			}
		}
		cdStr := "none"
		if cdCount > 0 {
			cdStr = fmt.Sprintf("%d active", cdCount)
		}
		fmt.Fprintf(w, "\t(default)\tstatic\t%s\t%s\n", credStatus, cdStr)
		w.Flush()
		return
	}

	for _, acc := range p.Accounts {
		accType := acc.Type
		if accType == "" {
			accType = "static"
		}
		credStatus := "none"
		if acc.APIKey != "" {
			credStatus = maskKey(acc.APIKey)
		} else if acc.Credential != nil && acc.Credential.APIKey != "" {
			credStatus = maskKey(acc.Credential.APIKey)
		} else if credStore != nil {
			if rec, ok := credStore.GetAccount(providerName, acc.Name); ok {
				credStatus = fmt.Sprintf("oauth(%s)", rec.Masked().RefreshToken)
			} else if rec, ok := credStore.Get(providerName + "/" + acc.Name); ok {
				credStatus = fmt.Sprintf("oauth(%s)", rec.Masked().RefreshToken)
			}
		}

		key := providerName + "/" + acc.Name
		cdCount := 0
		for k := range activeCDs {
			if k == key || strings.HasPrefix(k, key+"#") {
				cdCount++
			}
		}
		cdStr := "none"
		if cdCount > 0 {
			cdStr = fmt.Sprintf("%d active", cdCount)
		}

		fmt.Fprintf(w, "\t%s\t%s\t%s\t%s\n", acc.Name, accType, credStatus, cdStr)
	}
	w.Flush()
}

func maskKey(s string) string {
	if len(s) <= 8 {
		return "****"
	}
	return s[:4] + "..." + s[len(s)-4:]
}

func cmdAccountRemove(args []string, isInteractive bool) error {
	svc, err := config.LoadService()
	if err != nil {
		return fmt.Errorf("load service config: %w", err)
	}
	data, err := os.ReadFile(svc.ConfigPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", svc.ConfigPath, err)
	}
	topo, err := config.ParseRawTopology(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", svc.ConfigPath, err)
	}

	var providerName, accountName string
	if len(args) > 0 {
		providerName = args[0]
	}
	if len(args) > 1 {
		accountName = args[1]
	}

	if providerName == "" {
		if !isInteractive {
			return errors.New("usage: tinyroute providers account remove <provider> <name>")
		}
		providers := make([]string, 0, len(topo.Providers))
		for p := range topo.Providers {
			providers = append(providers, p)
		}
		sort.Strings(providers)
		providerName, err = interactive.Select("Select provider:", providers)
		if err != nil {
			return fmt.Errorf("select provider: %w", err)
		}
	}

	p, ok := topo.Providers[providerName]
	if !ok {
		return fmt.Errorf("unknown provider %q", providerName)
	}

	if accountName == "" {
		if !isInteractive || len(p.Accounts) == 0 {
			return errors.New("account name is required (or provider has no accounts)")
		}
		accNames := make([]string, len(p.Accounts))
		for i, a := range p.Accounts {
			accNames[i] = a.Name
		}
		accountName, err = interactive.Select("Select account to remove:", accNames)
		if err != nil {
			return fmt.Errorf("select account: %w", err)
		}
	}

	foundIdx := -1
	for i, acc := range p.Accounts {
		if acc.Name == accountName {
			foundIdx = i
			break
		}
	}
	if foundIdx == -1 {
		return fmt.Errorf("account %q not found under provider %q", accountName, providerName)
	}

	p.Accounts = append(p.Accounts[:foundIdx], p.Accounts[foundIdx+1:]...)
	topo.Providers[providerName] = p

	var modifiedCombos []string
	topo.Combos, modifiedCombos = config.DowngradeComboAccount(topo.Combos, providerName, accountName)

	if err := config.WriteTopology(svc.ConfigPath, topo); err != nil {
		return fmt.Errorf("write topology: %w", err)
	}

	home, _ := os.UserHomeDir()
	if credStore, err := credential.NewStore(filepath.Join(home, ".tinyroute", "credentials.json")); err == nil {
		_ = credStore.Delete(providerName + "/" + accountName)
	}

	fmt.Printf("removed account %q from provider %q\n", accountName, providerName)
	if len(modifiedCombos) > 0 {
		fmt.Printf("downgraded pin in combos: %s\n", strings.Join(modifiedCombos, ", "))
	}
	return nil
}

func cmdAccountTest(ctx context.Context, args []string, timeout time.Duration, isInteractive bool) error {
	svc, err := config.LoadService()
	if err != nil {
		return fmt.Errorf("load service config: %w", err)
	}
	data, err := os.ReadFile(svc.ConfigPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", svc.ConfigPath, err)
	}
	topo, err := config.ParseRawTopology(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", svc.ConfigPath, err)
	}

	var providerName, accountName string
	if len(args) > 0 {
		providerName = args[0]
	}
	if len(args) > 1 {
		accountName = args[1]
	}

	if providerName == "" {
		if !isInteractive {
			return errors.New("usage: tinyroute providers account test <provider> [name]")
		}
		providers := make([]string, 0, len(topo.Providers))
		for p := range topo.Providers {
			providers = append(providers, p)
		}
		sort.Strings(providers)
		providerName, err = interactive.Select("Select provider to test:", providers)
		if err != nil {
			return fmt.Errorf("select provider: %w", err)
		}
	}

	p, ok := topo.Providers[providerName]
	if !ok {
		return fmt.Errorf("unknown provider %q", providerName)
	}

	if len(p.Accounts) == 0 {
		fmt.Printf("provider %q has no accounts configured; probing default credential...\n", providerName)
		return testProviderAccount(ctx, providerName, "default", p, timeout)
	}

	if accountName == "" {
		if isInteractive {
			accNames := make([]string, len(p.Accounts))
			for i, a := range p.Accounts {
				accNames[i] = a.Name
			}
			accountName, err = interactive.Select("Select account to test:", accNames)
			if err != nil {
				return fmt.Errorf("select account: %w", err)
			}
		} else {
			// Test all accounts
			var testErrs []string
			for _, acc := range p.Accounts {
				if err := testProviderAccount(ctx, providerName, acc.Name, p, timeout); err != nil {
					testErrs = append(testErrs, fmt.Sprintf("%s: %v", acc.Name, err))
				}
			}
			if len(testErrs) > 0 {
				return fmt.Errorf("account test failures: %s", strings.Join(testErrs, "; "))
			}
			return nil
		}
	}

	return testProviderAccount(ctx, providerName, accountName, p, timeout)
}

func testProviderAccount(ctx context.Context, providerName, accountName string, p config.Provider, timeout time.Duration) error {
	home, _ := os.UserHomeDir()
	credStore, _ := credential.NewStore(filepath.Join(home, ".tinyroute", "credentials.json"))
	cred := p.BuildAccountCredential(providerName, accountName, credStore)

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	tokRes, err := cred.Token(ctx)
	if err != nil {
		fmt.Printf("FAIL account %s/%s: credential error: %v\n", providerName, accountName, err)
		return err
	}

	d, ok := dialect.ByName(p.Dialect)
	if !ok {
		return fmt.Errorf("unknown dialect %q", p.Dialect)
	}
	_ = d

	// Create test request
	req, err := http.NewRequestWithContext(ctx, "GET", p.BaseURL, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	if tokRes.Value != "" {
		req.Header.Set("Authorization", "Bearer "+tokRes.Value)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("FAIL account %s/%s: request error: %v\n", providerName, accountName, err)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		fmt.Printf("FAIL account %s/%s: status=%d\n", providerName, accountName, resp.StatusCode)
		return fmt.Errorf("probe failed with status %d", resp.StatusCode)
	}

	fmt.Printf("OK account %s/%s: status=%d\n", providerName, accountName, resp.StatusCode)
	return nil
}

func cmdAccountSelect(args []string, strategy string, isInteractive bool) error {
	svc, err := config.LoadService()
	if err != nil {
		return fmt.Errorf("load service config: %w", err)
	}
	data, err := os.ReadFile(svc.ConfigPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", svc.ConfigPath, err)
	}
	topo, err := config.ParseRawTopology(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", svc.ConfigPath, err)
	}

	var providerName string
	if len(args) > 0 {
		providerName = args[0]
	}

	if providerName == "" {
		if !isInteractive {
			return errors.New("usage: tinyroute providers account select <provider> [--strategy=NAME]")
		}
		providers := make([]string, 0, len(topo.Providers))
		for p := range topo.Providers {
			providers = append(providers, p)
		}
		sort.Strings(providers)
		providerName, err = interactive.Select("Select provider:", providers)
		if err != nil {
			return fmt.Errorf("select provider: %w", err)
		}
	}

	p, ok := topo.Providers[providerName]
	if !ok {
		return fmt.Errorf("unknown provider %q", providerName)
	}

	strategies := []string{
		string(config.StrategyRoundRobin),
		string(config.StrategyFillFirst),
		string(config.StrategySticky),
		string(config.StrategyStickyRoundRobin),
	}

	if strategy == "" {
		if !isInteractive {
			return errors.New("--strategy is required in non-interactive mode")
		}
		strategy, err = interactive.Select("Select account selection strategy:", strategies)
		if err != nil {
			return fmt.Errorf("select strategy: %w", err)
		}
	}

	valid := false
	for _, s := range strategies {
		if s == strategy {
			valid = true
			break
		}
	}
	if !valid {
		return fmt.Errorf("invalid strategy %q; must be one of: %s", strategy, strings.Join(strategies, ", "))
	}

	p.Selection = config.AccountStrategy(strategy)
	topo.Providers[providerName] = p

	if err := config.WriteTopology(svc.ConfigPath, topo); err != nil {
		return fmt.Errorf("write topology: %w", err)
	}

	fmt.Printf("set account selection strategy for provider %q to %q\n", providerName, strategy)
	return nil
}

func cmdAccountImport(args []string, filePath string, isInteractive bool) error {
	svc, err := config.LoadService()
	if err != nil {
		return fmt.Errorf("load service config: %w", err)
	}
	data, err := os.ReadFile(svc.ConfigPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", svc.ConfigPath, err)
	}
	topo, err := config.ParseRawTopology(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", svc.ConfigPath, err)
	}

	var providerName string
	if len(args) > 0 {
		providerName = args[0]
	}

	if providerName == "" {
		if !isInteractive {
			return errors.New("usage: tinyroute providers account import <provider> [--file=PATH]")
		}
		providers := make([]string, 0, len(topo.Providers))
		for p := range topo.Providers {
			providers = append(providers, p)
		}
		sort.Strings(providers)
		providerName, err = interactive.Select("Select target provider for account import:", providers)
		if err != nil {
			return fmt.Errorf("select provider: %w", err)
		}
	}

	p, ok := topo.Providers[providerName]
	if !ok {
		return fmt.Errorf("unknown provider %q", providerName)
	}

	var reader io.Reader
	if filePath != "" {
		f, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("open import file: %w", err)
		}
		defer f.Close()
		reader = f
	} else {
		if !isInteractive {
			reader = os.Stdin
		} else {
			return errors.New("--file flag or piped stdin required for account import")
		}
	}

	content, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("read import content: %w", err)
	}

	existing := make(map[string]bool)
	for _, acc := range p.Accounts {
		existing[acc.Name] = true
	}

	var imported []config.Account
	var collisions []string

	// Try JSON array first
	var jsonAccounts []struct {
		Name   string `json:"name"`
		Type   string `json:"type"`
		APIKey string `json:"api_key"`
	}
	if err := json.Unmarshal(content, &jsonAccounts); err == nil && len(jsonAccounts) > 0 {
		for _, ja := range jsonAccounts {
			name := ja.Name
			if name == "" {
				name = fmt.Sprintf("account-%d", len(p.Accounts)+len(imported)+1)
			}
			if existing[name] {
				collisions = append(collisions, name)
				continue
			}
			tp := ja.Type
			if tp == "" {
				tp = "static"
			}
			imported = append(imported, config.Account{
				Name:   name,
				Type:   tp,
				APIKey: ja.APIKey,
			})
			existing[name] = true
		}
	} else {
		// Parse line-by-line name|key
		scanner := bufio.NewScanner(strings.NewReader(string(content)))
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			parts := strings.SplitN(line, "|", 2)
			var name, key string
			if len(parts) == 2 {
				name = strings.TrimSpace(parts[0])
				key = strings.TrimSpace(parts[1])
			} else {
				key = strings.TrimSpace(parts[0])
				name = fmt.Sprintf("imported-%d", len(p.Accounts)+len(imported)+1)
			}
			if existing[name] {
				collisions = append(collisions, name)
				continue
			}
			imported = append(imported, config.Account{
				Name:   name,
				Type:   "static",
				APIKey: key,
			})
			existing[name] = true
		}
	}

	if len(collisions) > 0 {
		fmt.Printf("skipped %d collision(s): %s\n", len(collisions), strings.Join(collisions, ", "))
	}

	if len(imported) == 0 {
		fmt.Println("no new accounts imported")
		return nil
	}

	p.Accounts = append(p.Accounts, imported...)
	topo.Providers[providerName] = p

	if err := config.WriteTopology(svc.ConfigPath, topo); err != nil {
		return fmt.Errorf("write topology: %w", err)
	}

	fmt.Printf("imported %d account(s) into provider %q\n", len(imported), providerName)
	return nil
}
