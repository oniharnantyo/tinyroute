package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/oniharnantyo/tinyroute/internal/cli/interactive"
	"github.com/oniharnantyo/tinyroute/internal/clients"
	"github.com/oniharnantyo/tinyroute/internal/config"
	"github.com/urfave/cli/v3"
)

func cmdClient() *cli.Command {
	return &cli.Command{
		Name:     "clients",
		Aliases:  []string{"client"},
		Category: "Clients",
		Usage:    "Manage downstream coding agent configurations",
		Commands: []*cli.Command{
			{
				Name:      "install",
				Usage:     "Configure a coding agent to route through tinyroute",
				ArgsUsage: "[agent-id]",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "api-key",
						Usage: "existing API key to configure in agent config (default: mint new scoped key)",
					},
					&cli.StringFlag{
						Name:  "name",
						Usage: "custom key name when minting a new API key (default: agent-<id>)",
					},
					&cli.StringFlag{
						Name:  "model",
						Usage: "model name to configure for agents that require a model selection",
					},
					&cli.StringFlag{
						Name:  "base-url",
						Usage: "override base URL to write into agent configuration",
					},
					&cli.BoolFlag{
						Name:    "interactive",
						Aliases: []string{"i"},
						Usage:   "enable interactive prompts",
						Value:   true,
					},
					&cli.BoolFlag{
						Name:  "no-interactive",
						Usage: "disable interactive prompts",
					},
					&cli.BoolFlag{
						Name:  "force",
						Usage: "skip confirmation and interactive prompts",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					args := cmd.Args().Slice()
					if key := cmd.String("api-key"); key != "" {
						args = append(args, "--api-key="+key)
					}
					if name := cmd.String("name"); name != "" {
						args = append(args, "--name="+name)
					}
					if model := cmd.String("model"); model != "" {
						args = append(args, "--model="+model)
					}
					if baseURL := cmd.String("base-url"); baseURL != "" {
						args = append(args, "--base-url="+baseURL)
					}
					if cmd.IsSet("interactive") && !cmd.Bool("interactive") {
						args = append(args, "--no-interactive")
					}
					if cmd.Bool("no-interactive") {
						args = append(args, "--no-interactive")
					}
					if cmd.Bool("force") {
						args = append(args, "--force")
					}
					return cmdClientInstall(args)
				},
			},
			{
				Name:      "status",
				Usage:     "Show configuration status of supported coding agents",
				ArgsUsage: "[agent-id]",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "interactive",
						Aliases: []string{"i"},
						Usage:   "enable interactive mode",
						Value:   true,
					},
					&cli.BoolFlag{
						Name:  "no-interactive",
						Usage: "disable interactive mode",
					},
					&cli.BoolFlag{
						Name:  "force",
						Usage: "skip interactive mode",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					args := cmd.Args().Slice()
					if cmd.IsSet("interactive") && !cmd.Bool("interactive") {
						args = append(args, "--no-interactive")
					}
					if cmd.Bool("no-interactive") {
						args = append(args, "--no-interactive")
					}
					if cmd.Bool("force") {
						args = append(args, "--force")
					}
					return cmdClientStatus(args)
				},
			},
			{
				Name:      "uninstall",
				Usage:     "Remove tinyroute configuration from a coding agent",
				ArgsUsage: "[agent-id]",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "interactive",
						Aliases: []string{"i"},
						Usage:   "enable interactive prompts",
						Value:   true,
					},
					&cli.BoolFlag{
						Name:  "no-interactive",
						Usage: "disable interactive prompts",
					},
					&cli.BoolFlag{
						Name:  "force",
						Usage: "skip confirmation prompts",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					args := cmd.Args().Slice()
					if cmd.IsSet("interactive") && !cmd.Bool("interactive") {
						args = append(args, "--no-interactive")
					}
					if cmd.Bool("no-interactive") {
						args = append(args, "--no-interactive")
					}
					if cmd.Bool("force") {
						args = append(args, "--force")
					}
					return cmdClientUninstall(args)
				},
			},
		},
	}
}

func dialectBaseURL(listen, dialect string) string {
	listen = strings.TrimSpace(listen)
	scheme := "http://"
	if strings.HasPrefix(listen, "http://") {
		scheme = ""
	} else if strings.HasPrefix(listen, "https://") {
		scheme = ""
	}
	if strings.HasPrefix(listen, ":") {
		listen = "127.0.0.1" + listen
	} else if strings.HasPrefix(listen, "0.0.0.0:") {
		listen = "127.0.0.1" + listen[7:]
	}
	if scheme != "" {
		listen = scheme + listen
	}
	listen = strings.TrimRight(listen, "/")

	switch dialect {
	case "openai", "openairesponses", "openai-responses":
		return listen + "/openai/v1"
	case "anthropic":
		return listen + "/anthropic"
	case "gemini":
		return listen + "/gemini"
	default:
		return listen + "/" + strings.TrimPrefix(dialect, "/")
	}
}

func discoverModelsForDialect(dialect string) []string {
	svc, err := config.LoadService()
	if err != nil {
		return nil
	}
	data, err := os.ReadFile(svc.ConfigPath)
	if err != nil {
		return nil
	}
	topo, err := config.ParseTopology(data)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var result []string
	for _, r := range topo.Routes {
		if r.From == dialect || r.From == "*" {
			for _, hop := range r.Chain {
				parts := strings.Split(hop, ":")
				if len(parts) == 2 {
					m := parts[1]
					if m != "" && m != "$model" && !seen[m] {
						seen[m] = true
						result = append(result, m)
					}
				}
			}
			if r.Match != "" && r.Match != "*" && !strings.Contains(r.Match, "*") && !seen[r.Match] {
				seen[r.Match] = true
				result = append(result, r.Match)
			}
		}
	}
	return result
}

func truncateKeyDisplay(k string) string {
	if len(k) <= 8 {
		return k
	}
	return k[:4] + "..." + k[len(k)-4:]
}

func cmdClientInstall(args []string) error {
	fs := flag.NewFlagSet("agent install", flag.ContinueOnError)
	apiKeyFlag := fs.String("api-key", "", "existing API key to configure")
	nameFlag := fs.String("name", "", "custom key name when minting a new API key")
	modelFlag := fs.String("model", "", "model name to configure")
	baseURLFlag := fs.String("base-url", "", "override base URL")
	interactiveFlag := fs.Bool("interactive", true, "enable interactive prompt")
	noInteractiveFlag := fs.Bool("no-interactive", false, "disable interactive prompt")
	forceFlag := fs.Bool("force", false, "skip interactive prompt")

	if err := parseFlags(fs, args); err != nil {
		return err
	}

	isInteractive := *interactiveFlag && !*noInteractiveFlag && !*forceFlag

	var clientID string
	if fs.NArg() >= 1 {
		clientID = fs.Arg(0)
	} else if isInteractive {
		all := clients.All()
		if len(all) == 0 {
			return fmt.Errorf("no agents registered")
		}
		options := make([]string, len(all))
		for i, a := range all {
			options[i] = fmt.Sprintf("%s (%s)", a.ID(), a.Name())
		}
		selected, err := interactive.Select("Select coding agent to install:", options)
		if err != nil {
			return err
		}
		clientID = strings.Fields(selected)[0]
	} else {
		return fmt.Errorf("usage: tinyroute agent install <agent-id>")
	}

	a, ok := clients.Get(clientID)
	if !ok {
		return fmt.Errorf("unknown agent %q", clientID)
	}

	// 1. Base URL
	baseURL := *baseURLFlag
	if baseURL == "" {
		if svc, err := config.LoadService(); err == nil {
			baseURL = clients.DialectBaseURL(svc.Listen, a.Dialect())
		}
	}

	// 2. API Key configuration decision
	apiKey := *apiKeyFlag
	keyStrategy := clients.KeyStrategyMint
	if apiKey != "" {
		keyStrategy = clients.KeyStrategyReuse
	} else if isInteractive {
		choice, err := interactive.Select("API key configuration:", []string{
			"Mint a new scoped API key (recommended)",
			"Reuse an existing API key / token",
		})
		if err != nil {
			return err
		}
		if strings.HasPrefix(choice, "Reuse") {
			entered, err := interactive.Password("Enter API key or token to configure:")
			if err != nil {
				return err
			}
			if strings.TrimSpace(entered) == "" {
				return fmt.Errorf("API key cannot be empty")
			}
			apiKey = strings.TrimSpace(entered)
			keyStrategy = clients.KeyStrategyReuse
		} else {
			keyStrategy = clients.KeyStrategyMint
		}
	} else {
		keyStrategy = clients.KeyStrategyMint
	}

	// 3. Model selections
	primaryModel := *modelFlag
	modelSlotsMap := map[string]string{}
	var selectedModels []string

	routableModels := clients.DiscoverModelsForDialect(a.Dialect())

	if a.NeedsModel() {
		slots := a.ModelSlots()
		for _, slot := range slots {
			if slot.Kind == clients.SlotSingle {
				if slot.ID == "model" && primaryModel != "" {
					modelSlotsMap[slot.ID] = primaryModel
					continue
				}
				if isInteractive {
					if len(routableModels) > 0 {
						opts := make([]string, len(routableModels))
						copy(opts, routableModels)
						if !slot.Required {
							opts = append([]string{"(skip)"}, opts...)
						}
						sel, err := interactive.Select(fmt.Sprintf("Select %s for %s:", slot.Name, a.Name()), opts)
						if err != nil {
							return err
						}
						if sel != "(skip)" {
							modelSlotsMap[slot.ID] = sel
							if slot.ID == "model" {
								primaryModel = sel
							}
						}
					} else if slot.Required {
						val, err := interactive.Input(fmt.Sprintf("Enter %s for %s:", slot.Name, a.Name()), "gpt-4o", nil)
						if err != nil {
							return err
						}
						modelSlotsMap[slot.ID] = val
						if slot.ID == "model" {
							primaryModel = val
						}
					}
				} else {
					if primaryModel == "" && slot.Required {
						primaryModel = "gpt-4o"
					}
					if primaryModel != "" && slot.ID == "model" {
						modelSlotsMap[slot.ID] = primaryModel
					}
				}
			} else if slot.Kind == clients.SlotMulti {
				if isInteractive && len(routableModels) > 0 {
					selList, err := interactive.MultiSelect(fmt.Sprintf("Select %s for %s:", slot.Name, a.Name()), routableModels)
					if err != nil {
						return err
					}
					selectedModels = selList
				} else {
					if primaryModel != "" {
						selectedModels = []string{primaryModel}
					} else {
						selectedModels = []string{"gpt-4o"}
					}
				}
			}
		}
	}

	if primaryModel == "" && len(selectedModels) > 0 {
		primaryModel = selectedModels[0]
	}

	listen := "127.0.0.1:8080"
	keysPath := ""
	if svc, err := config.LoadService(); err == nil {
		listen = svc.Listen
		keysPath = svc.KeysPath
	}

	installer := clients.NewInstaller(listen, keysPath)

	keyName := *nameFlag
	if keyName == "" {
		keyName = "agent-" + a.ID()
	}

	plan, err := installer.Plan(clients.InstallRequest{
		ClientID:    clientID,
		BaseURL:     baseURL,
		APIKey:      apiKey,
		KeyStrategy: keyStrategy,
		KeyName:     keyName,
		Model:       primaryModel,
		Models:      selectedModels,
		ModelSlots:  modelSlotsMap,
	})
	if err != nil {
		return fmt.Errorf("plan install for %s: %w", clientID, err)
	}

	// 4. Preview and Confirmation
	if isInteractive {
		fmt.Println("\nConfiguration Preview:")
		fmt.Printf("  Agent:       %s (%s)\n", plan.ClientName, plan.ClientID)
		fmt.Printf("  Base URL:    %s\n", plan.BaseURL)
		if plan.KeyStrategy == clients.KeyStrategyMint {
			fmt.Printf("  API Key:     Mint fresh key (scoped to %s:*)\n", plan.Dialect)
		} else {
			fmt.Printf("  API Key:     Use provided key (%s)\n", truncateKeyDisplay(plan.APIKey))
		}
		if plan.Model != "" {
			fmt.Printf("  Primary Model: %s\n", plan.Model)
		}
		if len(plan.Models) > 0 {
			fmt.Printf("  Model List:    %s\n", strings.Join(plan.Models, ", "))
		}
		for slotID, slotVal := range plan.ModelSlots {
			if slotID != "model" {
				fmt.Printf("  Slot [%s]:    %s\n", slotID, slotVal)
			}
		}
		fmt.Printf("  Config Path: %s\n", plan.ConfigPath)
		if plan.HasBackup {
			fmt.Printf("  Backup File: %s\n", plan.BackupPath)
		}
		fmt.Println()

		confirm, err := interactive.Confirm("Apply configuration?", true)
		if err != nil {
			return err
		}
		if !confirm {
			fmt.Println("Aborted. No changes written.")
			return nil
		}
	}

	res, err := installer.Apply(plan)
	if err != nil {
		return fmt.Errorf("apply configuration for %s: %w", clientID, err)
	}

	fmt.Printf("Successfully configured %s (%s)\n", plan.ClientName, plan.ClientID)
	fmt.Printf("  Base URL:    %s\n", plan.BaseURL)
	if plan.Model != "" {
		fmt.Printf("  Model:       %s\n", plan.Model)
	}
	for _, f := range res.Files {
		fmt.Printf("  Config File: %s\n", f)
	}
	if res.Backup != "" {
		fmt.Printf("  Backup:      %s\n", res.Backup)
	}

	if plan.KeyStrategy == clients.KeyStrategyMint && res.Key != "" {
		fmt.Println("\nMinted fresh scoped API key (shown once, store it now):")
		fmt.Println("  " + res.Key)
	}

	return nil
}

func cmdClientStatus(args []string) error {
	fs := flag.NewFlagSet("agent status", flag.ContinueOnError)
	_ = fs.Bool("interactive", true, "enable interactive prompt")
	_ = fs.Bool("no-interactive", false, "disable interactive prompt")
	_ = fs.Bool("force", false, "skip interactive prompt")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	var agents []clients.Client
	if len(args) > 0 {
		clientID := fs.Arg(0)
		a, ok := clients.Get(clientID)
		if !ok {
			return fmt.Errorf("unknown client %q", clientID)
		}
		agents = []clients.Client{a}
	} else {
		agents = clients.All()
	}

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "AGENT\tINSTALLED\tPOINTED-AT-TINYROUTE\tCONFIG PATH")

	for _, a := range agents {
		st, err := a.Detect()
		if err != nil {
			fmt.Fprintf(tw, "%s\terror\terror\t%s\n", a.ID(), st.ConfigPath)
			continue
		}
		instStr := "no"
		if st.Installed {
			instStr = "yes"
		}
		pointedStr := "no"
		if st.PointedAtTinyRoute {
			pointedStr = "yes"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", a.ID(), instStr, pointedStr, st.ConfigPath)
	}

	return tw.Flush()
}

func cmdClientUninstall(args []string) error {
	fs := flag.NewFlagSet("agent uninstall", flag.ContinueOnError)
	interactiveFlag := fs.Bool("interactive", true, "enable interactive prompt")
	noInteractiveFlag := fs.Bool("no-interactive", false, "disable interactive prompt")
	forceFlag := fs.Bool("force", false, "skip interactive prompt")

	if err := parseFlags(fs, args); err != nil {
		return err
	}

	isInteractive := *interactiveFlag && !*noInteractiveFlag && !*forceFlag

	var clientID string
	if fs.NArg() >= 1 {
		clientID = fs.Arg(0)
	} else if isInteractive {
		all := clients.All()
		if len(all) == 0 {
			return fmt.Errorf("no agents registered")
		}
		options := make([]string, len(all))
		for i, a := range all {
			options[i] = fmt.Sprintf("%s (%s)", a.ID(), a.Name())
		}
		selected, err := interactive.Select("Select coding agent to uninstall:", options)
		if err != nil {
			return err
		}
		clientID = strings.Fields(selected)[0]
	} else {
		return fmt.Errorf("usage: tinyroute agent uninstall <agent-id>")
	}

	a, ok := clients.Get(clientID)
	if !ok {
		return fmt.Errorf("unknown agent %q", clientID)
	}

	if isInteractive {
		confirm, err := interactive.Confirm(fmt.Sprintf("Reset tinyroute configuration for %s (%s)?", a.Name(), a.ID()), false)
		if err != nil {
			return err
		}
		if !confirm {
			fmt.Println("Aborted.")
			return nil
		}
	}

	if err := a.Reset(); err != nil {
		return fmt.Errorf("reset agent %s: %w", a.ID(), err)
	}

	fmt.Printf("Successfully removed tinyroute configuration for %s (%s)\n", a.Name(), a.ID())
	return nil
}
