package cli

// This file contains all command implementations migrated from the root commands.go
// Each command is kept here for now - they can be split into individual files later
// if needed for better organization.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/oniharnantyo/tinyroute/internal/auth"
	"github.com/oniharnantyo/tinyroute/internal/cli/interactive"
	"github.com/oniharnantyo/tinyroute/internal/config"
	"github.com/oniharnantyo/tinyroute/internal/core"
	"github.com/oniharnantyo/tinyroute/internal/credential"
	"github.com/oniharnantyo/tinyroute/internal/dialect"
	"github.com/oniharnantyo/tinyroute/internal/history"
	"github.com/oniharnantyo/tinyroute/internal/history/sqlite"
	"github.com/oniharnantyo/tinyroute/internal/preset"
	"github.com/oniharnantyo/tinyroute/internal/probe"
	"github.com/oniharnantyo/tinyroute/internal/proxy"
	"github.com/oniharnantyo/tinyroute/internal/route"
	"github.com/urfave/cli/v3"
)

// cmdMinimalInit performs minimal initialization without creating .env files.
// Used for auto-init scenarios where users manage their own environment variables.
func cmdMinimalInit() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	dir := filepath.Join(home, ".tinyroute")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	// Create minimal config.yaml
	configPath := filepath.Join(dir, "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		topo := config.Topology{Providers: map[string]config.Provider{}, Routes: []config.Route{}}
		if err := config.WriteTopology(configPath, topo); err != nil {
			return fmt.Errorf("write %s: %w", configPath, err)
		}
		fmt.Printf("created %s\n", configPath)
	} else {
		fmt.Printf("%s already exists, leaving it alone\n", configPath)
	}

	// Generate default API key
	keysPath := filepath.Join(dir, "keys.json")
	var plaintext string
	if _, err := os.Stat(keysPath); os.IsNotExist(err) {
		var key auth.Key
		plaintext, key, err = auth.GenerateKey("default")
		if err != nil {
			return fmt.Errorf("generate key: %w", err)
		}
		kf := auth.KeyFile{Keys: []auth.Key{key}}
		if err := auth.WriteKeyFile(keysPath, kf); err != nil {
			return fmt.Errorf("write %s: %w", keysPath, err)
		}
		fmt.Printf("created %s and minted key %q\n", keysPath, key.ID)
	} else {
		fmt.Printf("%s already exists, skipping key creation\n", keysPath)
	}

	fmt.Println()
	fmt.Println("tinyroute is ready.")
	if plaintext != "" {
		fmt.Println()
		fmt.Println("Your API key (shown once, store it now):")
		fmt.Println("  " + plaintext)
		fmt.Println()
		fmt.Println("Client environment:")
		fmt.Println("  export ANTHROPIC_BASE_URL=http://127.0.0.1:8787")
		fmt.Println("  export ANTHROPIC_AUTH_TOKEN=" + plaintext)
		fmt.Println()
		fmt.Println("Set provider credentials via environment variables or tinyroute auth set")
	}
	return nil
}

// newTabWriter returns a tabwriter configured for readable CLI tables.
func newTabWriter() *tabwriter.Writer {
	return tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
}

// ===========================================================================
// INIT
// ===========================================================================

func cmdInit(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	interactiveFlag := fs.Bool("interactive", true, "enable interactive setup wizard")
	noInteractiveFlag := fs.Bool("no-interactive", false, "disable interactive setup wizard")
	forceFlag := fs.Bool("force", false, "explicitly skip interactive wizard")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("resolve home directory: %w", err)
	}
	dir := filepath.Join(home, ".tinyroute")

	if *interactiveFlag && !*noInteractiveFlag && !*forceFlag {
		return interactive.RunInitWizard()
	}

	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create %s: %w", dir, err)
	}

	envPath := filepath.Join(dir, ".env")
	if _, err := os.Stat(envPath); os.IsNotExist(err) {
		envTemplate := `# tinyroute environment
# TINYROUTE_LISTEN=127.0.0.1:8787
# TINYROUTE_CAPTURE=full
# ANTHROPIC_API_KEY=
# OPENAI_API_KEY=
`
		if err := os.WriteFile(envPath, []byte(envTemplate), 0600); err != nil {
			return fmt.Errorf("write %s: %w", envPath, err)
		}
		fmt.Printf("created %s\n", envPath)
	} else {
		fmt.Printf("%s already exists, leaving it alone\n", envPath)
	}

	configPath := filepath.Join(dir, "config.yaml")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		topo := config.Topology{Providers: map[string]config.Provider{}, Routes: []config.Route{}}
		if err := config.WriteTopology(configPath, topo); err != nil {
			return fmt.Errorf("write %s: %w", configPath, err)
		}
		fmt.Printf("created %s\n", configPath)
	} else {
		fmt.Printf("%s already exists, leaving it alone\n", configPath)
	}

	keysPath := filepath.Join(dir, "keys.json")
	var kf auth.KeyFile
	if data, err := os.ReadFile(keysPath); err == nil {
		ks, err := auth.ParseKeyFile(data)
		if err != nil {
			return fmt.Errorf("parse existing %s: %w", keysPath, err)
		}
		kf.Keys = append([]auth.Key(nil), ks.Keys()...)
	}

	if _, err := os.Stat(".git"); err == nil {
		if err := ensureGitignore(".tinyroute/", ".env"); err != nil {
			fmt.Fprintf(os.Stderr, "warning: failed to update .gitignore: %v\n", err)
		} else {
			fmt.Println("detected .git — added .tinyroute/ and .env to .gitignore")
		}
	}

	var plaintext string
	if len(kf.Keys) == 0 {
		var key auth.Key
		plaintext, key, err = auth.GenerateKey("default")
		if err != nil {
			return fmt.Errorf("generate key: %w", err)
		}
		kf.Keys = append(kf.Keys, key)
		if err := auth.WriteKeyFile(keysPath, kf); err != nil {
			return fmt.Errorf("write %s: %w", keysPath, err)
		}
		fmt.Printf("created %s and minted key %q\n", keysPath, key.ID)
	} else {
		fmt.Printf("%s already has %d key(s), skipping key creation\n", keysPath, len(kf.Keys))
	}

	fmt.Println()
	fmt.Println("tinyroute is scaffolded. Start it with: tinyroute serve")
	if plaintext != "" {
		fmt.Println()
		fmt.Println("Your API key (shown once, store it now):")
		fmt.Println("  " + plaintext)
		fmt.Println()
		fmt.Println("Client environment:")
		fmt.Println("  export ANTHROPIC_BASE_URL=http://127.0.0.1:8787")
		fmt.Println("  export ANTHROPIC_AUTH_TOKEN=" + plaintext)
	}
	return nil
}

func ensureGitignore(entries ...string) error {
	const path = ".gitignore"
	existing := map[string]bool{}
	if data, err := os.ReadFile(path); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			existing[strings.TrimSpace(line)] = true
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	var toAdd []string
	for _, e := range entries {
		if !existing[e] {
			toAdd = append(toAdd, e)
		}
	}
	if len(toAdd) == 0 {
		return nil
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	for _, e := range toAdd {
		if _, err := fmt.Fprintln(f, e); err != nil {
			return err
		}
	}
	return nil
}

// ===========================================================================
// VALIDATE
// ===========================================================================

func cmdValidate(args []string) error {
	fs := flag.NewFlagSet("validate", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	svc, err := config.LoadService()
	if err != nil {
		return fmt.Errorf("load service config: %w", err)
	}

	data, err := os.ReadFile(svc.ConfigPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", svc.ConfigPath, err)
	}

	topo, err := config.ParseTopology(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", svc.ConfigPath, err)
	}

	errs := config.ValidateTopology(topo, dialect.Names())
	if len(errs) == 0 {
		fmt.Printf("OK: %d provider(s), %d route(s), no errors\n", len(topo.Providers), len(topo.Routes))
		return nil
	}

	for _, e := range errs {
		fmt.Fprintf(os.Stderr, "error: %v\n", e)
	}
	return fmt.Errorf("%d validation error(s)", len(errs))
}

// ===========================================================================
// TEST
// ===========================================================================

func cmdTest(args []string) error {
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	model := fs.String("model", "", "only probe hops whose route chain targets this model")
	timeout := fs.Duration("timeout", 10*time.Second, "per-hop probe timeout")
	interactiveFlag := fs.Bool("interactive", true, "show progress spinner during probes")
	noInteractiveFlag := fs.Bool("no-interactive", false, "disable progress spinner")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	svc, err := config.LoadService()
	if err != nil {
		return fmt.Errorf("load service config: %w", err)
	}
	data, err := os.ReadFile(svc.ConfigPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", svc.ConfigPath, err)
	}
	topo, err := config.ParseTopology(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", svc.ConfigPath, err)
	}

	type hop struct{ provider, model string }
	seen := map[hop]bool{}
	var hops []hop
	for _, r := range topo.Routes {
		for _, entry := range r.Chain {
			parts := strings.SplitN(entry, ":", 2)
			if len(parts) != 2 {
				continue
			}
			h := hop{provider: parts[0], model: parts[1]}
			if h.model == "$model" {
				h.model = r.Match
			}
			if *model != "" && h.model != *model {
				continue
			}
			if !seen[h] {
				seen[h] = true
				hops = append(hops, h)
			}
		}
	}

	if len(hops) == 0 {
		fmt.Println("no matching hops to probe")
		return nil
	}

	var spinner *interactive.Spinner
	if *interactiveFlag && !*noInteractiveFlag {
		var err error
		spinner, err = interactive.StartSpinner(fmt.Sprintf("Probing %d provider hop(s)...", len(hops)))
		if err != nil {
			spinner = nil
		}
	}

	client := &http.Client{Timeout: *timeout}
	failures := 0
	for _, h := range hops {
		if spinner != nil {
			spinner.Update(fmt.Sprintf("Testing %s / %s...", h.provider, h.model))
		}
		prov, ok := topo.Providers[h.provider]
		if !ok {
			fmt.Printf("%-15s %-25s SKIP: provider %q not declared\n", h.provider, h.model, h.provider)
			continue
		}
		d, ok := dialect.ByName(prov.Dialect)
		if !ok {
			fmt.Printf("%-15s %-25s SKIP: unknown dialect %q\n", h.provider, h.model, prov.Dialect)
			continue
		}

		probeBody, err := d.RewriteModel([]byte(probeBodyFor(prov.Dialect)), h.model)
		if err != nil {
			fmt.Printf("%-15s %-25s FAIL: could not build probe request: %v\n", h.provider, h.model, err)
			failures++
			continue
		}

		outboundPaths := d.Paths()
		if len(outboundPaths) == 0 {
			fmt.Printf("%-15s %-25s SKIP: dialect declares no outbound path\n", h.provider, h.model)
			continue
		}
		url := proxy.JoinURL(prov.BaseURL, outboundPaths[0])

		req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(probeBody))
		if err != nil {
			fmt.Printf("%-15s %-25s FAIL: %v\n", h.provider, h.model, err)
			failures++
			continue
		}
		credStore, _ := credential.NewStore(svc.CredentialsPath)
		tokRes, _ := prov.BuildCredential(h.provider, credStore).Token(context.Background())
		req.Header = d.AuthHeaders(tokRes, prov.Headers)

		start := time.Now()
		resp, err := client.Do(req)
		elapsed := time.Since(start)
		if err != nil {
			fmt.Printf("%-15s %-25s FAIL: unreachable (%v) in %s\n", h.provider, h.model, err, elapsed.Round(time.Millisecond))
			failures++
			continue
		}
		resp.Body.Close()

		switch {
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			fmt.Printf("%-15s %-25s OK: %d in %s\n", h.provider, h.model, resp.StatusCode, elapsed.Round(time.Millisecond))
		case resp.StatusCode == 401 || resp.StatusCode == 403:
			fmt.Printf("%-15s %-25s FAIL: rejected credential (%d)\n", h.provider, h.model, resp.StatusCode)
			failures++
		case resp.StatusCode == 404:
			fmt.Printf("%-15s %-25s FAIL: unknown model (%d)\n", h.provider, h.model, resp.StatusCode)
			failures++
		default:
			fmt.Printf("%-15s %-25s FAIL: unexpected status %d\n", h.provider, h.model, resp.StatusCode)
			failures++
		}
	}

	if spinner != nil {
		if failures == 0 {
			spinner.Success("All provider probes completed successfully")
		} else {
			spinner.Fail(fmt.Sprintf("%d/%d provider hop(s) failed", failures, len(hops)))
		}
	}

	if failures > 0 {
		return fmt.Errorf("%d/%d hop(s) failed", failures, len(hops))
	}
	return nil
}

func probeBodyFor(dialectName string) string {
	switch dialectName {
	case "anthropic":
		return `{"model":"probe","max_tokens":1,"messages":[{"role":"user","content":"ping"}]}`
	default:
		return `{"model":"probe","messages":[{"role":"user","content":"ping"}]}`
	}
}

// ===========================================================================
// STATUS
// ===========================================================================

func cmdStatus(args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	svc, err := config.LoadService()
	if err != nil {
		return fmt.Errorf("load service config: %w", err)
	}

	data, err := os.ReadFile(svc.ConfigPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", svc.ConfigPath, err)
	}
	topo, err := config.ParseTopology(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", svc.ConfigPath, err)
	}

	clock := route.RealClock{}
	health := route.NewHealthStore(clock, svc.StatePath)
	if err := health.Load(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: failed to load state: %v\n", err)
	}

	fmt.Printf("listen:  %s\n", svc.Listen)
	fmt.Printf("config:  %s\n", svc.ConfigPath)
	fmt.Printf("capture: %s\n", svc.Capture)
	fmt.Println()

	tw := newTabWriter()
	fmt.Fprintln(tw, "PROVIDER\tDIALECT\tAUTH\tCOOLDOWN")
	names := make([]string, 0, len(topo.Providers))
	for name := range topo.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		p := topo.Providers[name]
		authState := "set"
		if p.APIKey == "" {
			authState = "MISSING"
		}
		cooldown := "-"
		if end := health.CooldownEnd(name); !end.IsZero() {
			cooldown = fmt.Sprintf("until %s", end.Format(time.RFC3339))
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", name, p.Dialect, authState, cooldown)
	}
	tw.Flush()

	fmt.Printf("\nroutes: %d\n", len(topo.Routes))

	var warnings int
	for name, p := range topo.Providers {
		if p.APIKey == "" {
			fmt.Fprintf(os.Stderr, "warning: provider %q has no api_key configured\n", name)
			warnings++
		}
	}
	if warnings > 0 {
		fmt.Printf("\n%d auth warning(s)\n", warnings)
	}
	return nil
}

// ===========================================================================
// LOG
// ===========================================================================

func cmdHistory() *cli.Command {
	return &cli.Command{
		Name:  "history",
		Usage: "Observability: request log, sessions, status, and maintenance",
		Commands: []*cli.Command{
			{
				Name:  "log",
				Usage: "Tail request history",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "f",
						Usage:   "follow the log as it grows",
						Aliases: []string{"follow"},
					},
					&cli.BoolFlag{
						Name:  "failures",
						Usage: "show only non-ok outcomes",
					},
					&cli.StringFlag{
						Name:  "since",
						Usage: "only show records at or after this RFC3339 time",
					},
					&cli.StringFlag{
						Name:  "until",
						Usage: "only show records at or before this RFC3339 time",
					},
					&cli.StringFlag{
						Name:  "session",
						Usage: "filter to a single session ID",
					},
					&cli.StringFlag{
						Name:  "key",
						Usage: "filter to a single key ID",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					var args []string
					if cmd.Bool("f") {
						args = append(args, "-f")
					}
					if cmd.Bool("failures") {
						args = append(args, "--failures")
					}
					if since := cmd.String("since"); since != "" {
						args = append(args, "--since="+since)
					}
					if until := cmd.String("until"); until != "" {
						args = append(args, "--until="+until)
					}
					if session := cmd.String("session"); session != "" {
						args = append(args, "--session="+session)
					}
					if key := cmd.String("key"); key != "" {
						args = append(args, "--key="+key)
					}
					return cmdLog(args)
				},
			},
			{
				Name:  "sessions",
				Usage: "List recorded sessions",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "key",
						Usage: "filter to a single key ID",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					var args []string
					if key := cmd.String("key"); key != "" {
						args = append(args, "--key="+key)
					}
					if cmd.NArg() > 0 {
						args = append(args, cmd.Args().First())
					}
					return cmdSessions(args)
				},
			},
			{
				Name:  "status",
				Usage: "Show current state (cooldowns, keys, routes)",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "interactive",
						Aliases: []string{"i"},
						Usage:   "enable interactive prompts (default: true in interactive terminals)",
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
					args := append([]string(nil), cmd.Args().Slice()...)
					if cmd.IsSet("interactive") && !cmd.Bool("interactive") {
						args = append(args, "--no-interactive")
					}
					if cmd.Bool("no-interactive") {
						args = append(args, "--no-interactive")
					}
					if cmd.Bool("force") {
						args = append(args, "--force")
					}
					return cmdStatus(args)
				},
			},
			{
				Name:  "compact",
				Usage: "Reclaim superseded blobs from history",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "dry-run",
						Usage: "report what would be removed without deleting",
					},
					&cli.BoolFlag{
						Name:    "interactive",
						Aliases: []string{"i"},
						Usage:   "enable interactive confirmation (default: true in interactive terminals)",
						Value:   true,
					},
					&cli.BoolFlag{
						Name:  "no-interactive",
						Usage: "disable interactive confirmation",
					},
					&cli.BoolFlag{
						Name:  "force",
						Usage: "skip interactive confirmation",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					args := append([]string(nil), cmd.Args().Slice()...)
					if cmd.Bool("dry-run") {
						args = append(args, "--dry-run")
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
					return cmdCompact(args)
				},
			},
		},
	}
}

// ===========================================================================

func cmdLog(args []string) error {
	fs := flag.NewFlagSet("log", flag.ContinueOnError)
	follow := fs.Bool("f", false, "follow the log as it grows")
	failuresOnly := fs.Bool("failures", false, "show only non-ok outcomes")
	since := fs.String("since", "", "only show records at or after this RFC3339 time")
	until := fs.String("until", "", "only show records at or before this RFC3339 time")
	session := fs.String("session", "", "filter to a single session ID")
	key := fs.String("key", "", "filter to a single key ID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	svc, err := config.LoadService()
	if err != nil {
		return fmt.Errorf("load service config: %w", err)
	}

	var sinceT, untilT time.Time
	if *since != "" {
		if sinceT, err = time.Parse(time.RFC3339, *since); err != nil {
			return fmt.Errorf("--since: %w", err)
		}
	}
	if *until != "" {
		if untilT, err = time.Parse(time.RFC3339, *until); err != nil {
			return fmt.Errorf("--until: %w", err)
		}
	}

	db, err := sqlite.Open(svc.HistoryDBPath)
	if err != nil {
		return fmt.Errorf("open history db (%s): %w", svc.HistoryDBPath, err)
	}
	defer db.Close()
	store := sqlite.NewStore(db)

	filter := history.Filter{
		From:    sinceT,
		To:      untilT,
		Session: *session,
		KeyID:   *key,
		Limit:   1000,
	}

	rows, _, err := store.List(context.Background(), filter)
	if err != nil {
		return fmt.Errorf("list history: %w", err)
	}

	matches := func(rec history.Summary) bool {
		if *failuresOnly && rec.Outcome == string(core.OutcomeOK) {
			return false
		}
		return true
	}

	printLine := func(rec history.Summary) {
		fmt.Printf("%s  %-10s  %-8s  key=%-10s session=%-8s model=%-20s outcome=%s\n",
			rec.Timestamp.Format(time.RFC3339), rec.ID, rec.Endpoint, rec.KeyID, rec.Session, rec.ModelReq, rec.Outcome)
	}

	// Store.List returns most recent first; reverse for display (chronological order)
	var filtered []history.Summary
	for i := len(rows) - 1; i >= 0; i-- {
		if matches(rows[i]) {
			filtered = append(filtered, rows[i])
			printLine(rows[i])
		}
	}

	if !*follow {
		return nil
	}

	var lastTS time.Time
	var lastID string
	if len(rows) > 0 {
		// rows[0] is the most recent
		lastTS = rows[0].Timestamp
		lastID = rows[0].ID
	}

	return pollFollowHistory(context.Background(), store, 500*time.Millisecond, lastTS, lastID, untilT, *session, *key, matches, printLine)
}

func pollFollowHistory(ctx context.Context, store *sqlite.Store, interval time.Duration, lastTS time.Time, lastID string, untilT time.Time, session, key string, matches func(history.Summary) bool, printLine func(history.Summary)) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			subFilter := history.Filter{
				From:    lastTS,
				To:      untilT,
				Session: session,
				KeyID:   key,
				Limit:   1000,
			}
			newRows, _, err := store.List(ctx, subFilter)
			if err != nil {
				continue
			}
			var fresh []history.Summary
			for _, r := range newRows {
				if r.Timestamp.Equal(lastTS) && r.ID <= lastID {
					continue
				}
				if r.Timestamp.Before(lastTS) {
					continue
				}
				if matches(r) {
					fresh = append(fresh, r)
				}
			}
			if len(newRows) > 0 {
				lastTS = newRows[0].Timestamp
				lastID = newRows[0].ID
			}
			for i := len(fresh) - 1; i >= 0; i-- {
				printLine(fresh[i])
			}
		}
	}
}

// ===========================================================================
// SESSIONS
// ===========================================================================

func cmdSessions(args []string) error {
	fs := flag.NewFlagSet("sessions", flag.ContinueOnError)
	key := fs.String("key", "", "filter to a single key ID")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if fs.NArg() >= 1 {
		return cmdSessionReplay(fs.Arg(0))
	}

	svc, err := config.LoadService()
	if err != nil {
		return fmt.Errorf("load service config: %w", err)
	}

	db, err := sqlite.Open(svc.HistoryDBPath)
	if err != nil {
		return fmt.Errorf("open history db (%s): %w", svc.HistoryDBPath, err)
	}
	defer db.Close()
	store := sqlite.NewStore(db)

	filter := history.Filter{
		KeyID: *key,
		Limit: 1000,
	}

	var allRows []history.Summary
	cursor := ""
	for {
		filter.Cursor = cursor
		rows, nextCursor, err := store.List(context.Background(), filter)
		if err != nil {
			return fmt.Errorf("list history: %w", err)
		}
		allRows = append(allRows, rows...)
		if nextCursor == "" {
			break
		}
		cursor = nextCursor
	}

	type sessionSummary struct {
		id          string
		turns       int
		keys        map[string]bool
		models      map[string]bool
		providers   map[string]bool
		inputTokens int64
		outputTok   int64
		firstSeen   time.Time
		lastSeen    time.Time
	}

	summaries := map[string]*sessionSummary{}
	var order []string
	for _, rec := range allRows {
		if rec.Session == "" {
			continue
		}
		s, ok := summaries[rec.Session]
		if !ok {
			s = &sessionSummary{
				id:        rec.Session,
				keys:      map[string]bool{},
				models:    map[string]bool{},
				providers: map[string]bool{},
				firstSeen: rec.Timestamp,
				lastSeen:  rec.Timestamp,
			}
			summaries[rec.Session] = s
			order = append(order, rec.Session)
		}
		s.turns++
		if rec.KeyID != "" {
			s.keys[rec.KeyID] = true
		}
		if rec.ModelReq != "" {
			s.models[rec.ModelReq] = true
		}
		if rec.Provider != "" {
			s.providers[rec.Provider] = true
		}
		s.inputTokens += rec.InputTokens
		s.outputTok += rec.OutputTokens
		if rec.Timestamp.Before(s.firstSeen) {
			s.firstSeen = rec.Timestamp
		}
		if rec.Timestamp.After(s.lastSeen) {
			s.lastSeen = rec.Timestamp
		}
	}

	if len(order) == 0 {
		fmt.Println("no sessions recorded yet")
		return nil
	}

	tw := newTabWriter()
	fmt.Fprintln(tw, "SESSION\tKEY\tTURNS\tMODELS\tPROVIDERS\tIN TOK\tOUT TOK\tLAST SEEN")
	for _, id := range order {
		s := summaries[id]
		fmt.Fprintf(tw, "%s\t%s\t%d\t%s\t%s\t%d\t%d\t%s\n",
			s.id, joinSet(s.keys), s.turns, joinSet(s.models), joinSet(s.providers), s.inputTokens, s.outputTok, s.lastSeen.Format(time.RFC3339))
	}
	return tw.Flush()
}

func joinSet(m map[string]bool) string {
	names := make([]string, 0, len(m))
	for k := range m {
		names = append(names, k)
	}
	sort.Strings(names)
	return strings.Join(names, ",")
}

func cmdSessionReplay(sessionID string) error {
	svc, err := config.LoadService()
	if err != nil {
		return fmt.Errorf("load service config: %w", err)
	}

	db, err := sqlite.Open(svc.HistoryDBPath)
	if err != nil {
		return fmt.Errorf("open history db (%s): %w", svc.HistoryDBPath, err)
	}
	defer db.Close()
	store := sqlite.NewStore(db)

	rows, _, err := store.List(context.Background(), history.Filter{
		Session: sessionID,
		Limit:   1000,
	})
	if err != nil {
		return fmt.Errorf("list session history: %w", err)
	}

	if len(rows) == 0 {
		return fmt.Errorf("no records found for session %q", sessionID)
	}

	// Reverse to display in chronological order
	found := 0
	for i := len(rows) - 1; i >= 0; i-- {
		rec := rows[i]
		found++
		keyStr := rec.KeyID
		if keyStr == "" {
			keyStr = "-"
		}
		fmt.Printf("--- turn %d: %s (%s) key=%s outcome=%s ---\n", found, rec.Timestamp.Format(time.RFC3339), rec.ModelReq, keyStr, rec.Outcome)

		var attempts []core.Attempt
		if rec.Attempts != "" {
			_ = json.Unmarshal([]byte(rec.Attempts), &attempts)
		}
		for _, a := range attempts {
			fmt.Printf("  hop: %s/%s status=%d elapsed=%s\n", a.Provider, a.Model, a.Status, a.Elapsed.Round(time.Millisecond))
		}
	}

	return nil
}

func truncateForDisplay(b []byte, n int) string {
	s := string(b)
	if len(s) > n {
		return s[:n] + "...(truncated)"
	}
	return s
}

// ===========================================================================
// KEYS
// ===========================================================================

func cmdKeys() *cli.Command {
	return &cli.Command{
		Name:  "keys",
		Usage: "Manage API keys",
		Commands: []*cli.Command{
			{
				Name:  "create",
				Usage: "Generate a new API key",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "name",
						Usage: "Display name for the new key",
					},
					&cli.StringFlag{
						Name:  "expires",
						Usage: "Expiry duration (e.g. 168h) or RFC3339 timestamp",
					},
					&cli.StringFlag{
						Name:  "rate",
						Usage: "Rate limit as requests/interval (e.g. 60/1m)",
					},
					&cli.BoolFlag{
						Name:    "interactive",
						Aliases: []string{"i"},
						Usage:   "enable interactive prompt for key name",
						Value:   true,
					},
					&cli.BoolFlag{
						Name:  "no-interactive",
						Usage: "disable interactive prompt",
					},
					&cli.BoolFlag{
						Name:  "force",
						Usage: "skip interactive prompt",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					args := []string{}
					if name := cmd.String("name"); name != "" {
						args = append(args, "--name="+name)
					}
					if expires := cmd.String("expires"); expires != "" {
						args = append(args, "--expires="+expires)
					}
					if rate := cmd.String("rate"); rate != "" {
						args = append(args, "--rate="+rate)
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
					return cmdKeysCreate(args)
				},
			},
			{
				Name:  "list",
				Usage: "List all keys with last-use timestamps",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return cmdKeysList(nil)
				},
			},
			{
				Name:  "revoke",
				Usage: "Revoke a key",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "interactive",
						Aliases: []string{"i"},
						Usage:   "enable confirmation prompt before revoking key",
						Value:   true,
					},
					&cli.BoolFlag{
						Name:  "no-interactive",
						Usage: "disable confirmation prompt",
					},
					&cli.BoolFlag{
						Name:  "force",
						Usage: "skip confirmation prompt",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					if cmd.NArg() < 1 {
						return fmt.Errorf("usage: tinyroute keys revoke <key-id>")
					}
					args := []string{cmd.Args().First()}
					if cmd.IsSet("interactive") && !cmd.Bool("interactive") {
						args = append(args, "--no-interactive")
					}
					if cmd.Bool("no-interactive") {
						args = append(args, "--no-interactive")
					}
					if cmd.Bool("force") {
						args = append(args, "--force")
					}
					return cmdKeysRevoke(args)
				},
			},
		},
	}
}

func cmdKeysCreate(args []string) error {
	fs := flag.NewFlagSet("keys create", flag.ContinueOnError)
	name := fs.String("name", "", "display name for the new key")
	expiresFlag := fs.String("expires", "", "expiry duration (e.g. 168h) or RFC3339 timestamp")
	rateFlag := fs.String("rate", "", "rate limit as requests/interval (e.g. 60/1m)")
	interactiveFlag := fs.Bool("interactive", true, "enable interactive prompt for key name")
	noInteractiveFlag := fs.Bool("no-interactive", false, "disable interactive prompt")
	forceFlag := fs.Bool("force", false, "skip interactive prompt")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	keyName := *name
	if *interactiveFlag && !*noInteractiveFlag && !*forceFlag && keyName == "" {
		var err error
		keyName, err = interactive.Input("Enter display name for the new API key:", "default", nil)
		if err != nil {
			return err
		}
	}

	var opts []auth.KeyOpt
	if *expiresFlag != "" {
		expStr := *expiresFlag
		if d, err := time.ParseDuration(expStr); err == nil {
			exp := time.Now().Add(d)
			opts = append(opts, auth.WithExpires(&exp))
		} else if t, err := time.Parse(time.RFC3339, expStr); err == nil {
			opts = append(opts, auth.WithExpires(&t))
		} else {
			return fmt.Errorf("invalid --expires format: must be duration (e.g. 168h) or RFC3339 timestamp")
		}
	}

	if *rateFlag != "" {
		parts := strings.Split(*rateFlag, "/")
		if len(parts) != 2 {
			return fmt.Errorf("invalid --rate format (expected count/interval, e.g. 60/1m)")
		}
		requests, err := strconv.Atoi(parts[0])
		if err != nil || requests <= 0 {
			return fmt.Errorf("invalid --rate requests count: %s", parts[0])
		}
		interval := parts[1]
		if _, err := time.ParseDuration(interval); err != nil {
			return fmt.Errorf("invalid --rate interval %q: %w", interval, err)
		}
		opts = append(opts, auth.WithRate(&auth.RateSpec{
			Requests: requests,
			Interval: interval,
		}))
	}

	svc, err := config.LoadService()
	if err != nil {
		return fmt.Errorf("load service config: %w", err)
	}

	kf, err := loadKeyFile(svc.KeysPath)
	if err != nil {
		return err
	}

	plaintext, key, err := auth.GenerateKey(keyName, opts...)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}
	kf.Keys = append(kf.Keys, key)
	if err := auth.WriteKeyFile(svc.KeysPath, kf); err != nil {
		return fmt.Errorf("write %s: %w", svc.KeysPath, err)
	}

	fmt.Printf("created key %s (%s)\n\n", key.ID, key.Name)
	fmt.Println("Your API key (shown once, store it now):")
	fmt.Println("  " + plaintext)
	fmt.Println()
	fmt.Println("Client environment:")
	fmt.Println("  export ANTHROPIC_BASE_URL=" + "http://" + svc.Listen)
	fmt.Println("  export ANTHROPIC_AUTH_TOKEN=" + plaintext)
	return nil
}

func cmdKeysList(args []string) error {
	fs := flag.NewFlagSet("keys list", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

	svc, err := config.LoadService()
	if err != nil {
		return fmt.Errorf("load service config: %w", err)
	}

	kf, err := loadKeyFile(svc.KeysPath)
	if err != nil {
		return err
	}

	var activeKeys []auth.Key
	for _, k := range kf.Keys {
		if !k.Disabled {
			activeKeys = append(activeKeys, k)
		}
	}

	if len(activeKeys) == 0 {
		if len(kf.Keys) == 0 {
			fmt.Println("no keys created yet — run: tinyroute keys create")
		} else {
			fmt.Println("no active keys found (all keys have been revoked) — run: tinyroute keys create")
		}
		return nil
	}

	lastUse := deriveLastUse(svc.HistoryDBPath)

	tw := newTabWriter()
	fmt.Fprintln(tw, "ID\tNAME\tPREFIX\tCREATED\tEXPIRES\tRATE\tLAST USE")
	for _, k := range activeKeys {
		expires := "-"
		if k.Expires != nil {
			expires = k.Expires.Format(time.RFC3339)
		}
		rate := "-"
		if k.Rate != nil {
			rate = fmt.Sprintf("%d/%s", k.Rate.Requests, k.Rate.Interval)
		}
		last := "never"
		if t, ok := lastUse[k.ID]; ok {
			last = t.Format(time.RFC3339)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			k.ID, k.Name, k.Prefix, k.Created.Format(time.RFC3339), expires, rate, last)
	}
	return tw.Flush()
}

func cmdKeysRevoke(args []string) error {
	fs := flag.NewFlagSet("keys revoke", flag.ContinueOnError)
	interactiveFlag := fs.Bool("interactive", true, "enable confirmation prompt before revoking key")
	noInteractiveFlag := fs.Bool("no-interactive", false, "disable confirmation prompt")
	forceFlag := fs.Bool("force", false, "skip confirmation prompt")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("usage: tinyroute keys revoke <key-id>")
	}
	id := fs.Arg(0)

	if *interactiveFlag && !*noInteractiveFlag && !*forceFlag {
		confirmed, err := interactive.Confirm(fmt.Sprintf("Are you sure you want to revoke key %s?", id), false)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Println("Revocation cancelled.")
			return nil
		}
	}

	svc, err := config.LoadService()
	if err != nil {
		return fmt.Errorf("load service config: %w", err)
	}

	if err := auth.RevokeKey(svc.KeysPath, id); err != nil {
		return err
	}

	fmt.Printf("revoked key %s\n", id)
	return nil
}

func loadKeyFile(path string) (auth.KeyFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return auth.KeyFile{}, nil
		}
		return auth.KeyFile{}, fmt.Errorf("read %s: %w", path, err)
	}
	ks, err := auth.ParseKeyFile(data)
	if err != nil {
		return auth.KeyFile{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return auth.KeyFile{Keys: append([]auth.Key(nil), ks.Keys()...)}, nil
}

func deriveLastUse(dbPath string) map[string]time.Time {
	db, err := sqlite.Open(dbPath)
	if err != nil {
		return map[string]time.Time{}
	}
	defer db.Close()

	store := sqlite.NewStore(db)
	res, err := store.LastUseByKey(context.Background())
	if err != nil {
		return map[string]time.Time{}
	}
	return res
}

// ===========================================================================
// AUTH
// ===========================================================================

func cmdProviders() *cli.Command {
	return &cli.Command{
		Name:  "providers",
		Usage: "Manage providers, credentials, and models (add, auth, model)",
		Commands: []*cli.Command{
			{
				Name:  "add",
				Usage: "Add a provider from a preset",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:  "list",
						Usage: "list available presets and exit",
					},
					&cli.StringFlag{
						Name:  "name",
						Usage: "custom provider instance name in config.yaml",
					},
					&cli.StringFlag{
						Name:  "dialect",
						Usage: "select the alt dialect for dual-protocol presets",
					},
					&cli.StringFlag{
						Name:  "base-url",
						Usage: "override base URL for the provider",
					},
					&cli.BoolFlag{
						Name:    "interactive",
						Aliases: []string{"i"},
						Usage:   "enable interactive preset selection",
						Value:   true,
					},
					&cli.BoolFlag{
						Name:  "no-interactive",
						Usage: "disable interactive preset selection",
					},
					&cli.BoolFlag{
						Name:  "force",
						Usage: "skip interactive selection",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					var args []string
					if cmd.Bool("list") {
						args = append(args, "--list")
					}
					if name := cmd.String("name"); name != "" {
						args = append(args, "--name="+name)
					}
					if dialect := cmd.String("dialect"); dialect != "" {
						args = append(args, "--dialect="+dialect)
					}
					if baseURL := cmd.String("base-url"); baseURL != "" {
						args = append(args, "--base-url="+baseURL)
					}
					if cmd.NArg() > 0 {
						args = append(args, cmd.Args().Slice()...)
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
					return cmdAdd(args)
				},
			},
			{
				Name:     "auth",
				Usage:    "Manage provider credentials and OAuth authentication",
				Commands: cmdAuth().Commands,
			},
			{
				Name:  "model",
				Usage: "Manage provider model whitelists (add, list, remove, test)",
				Commands: []*cli.Command{
					{
						Name:  "add",
						Usage: "Add models to provider whitelist",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:  "models",
								Usage: "comma-separated list of models to add",
							},
							&cli.BoolFlag{
								Name:  "no-interactive",
								Usage: "disable interactive model selection",
							},
							&cli.BoolFlag{
								Name:  "force",
								Usage: "skip interactive selection",
							},
						},
						Action: func(ctx context.Context, cmd *cli.Command) error {
							var args []string
							if models := cmd.String("models"); models != "" {
								args = append(args, "--models="+models)
							}
							if cmd.Bool("no-interactive") {
								args = append(args, "--no-interactive")
							}
							if cmd.Bool("force") {
								args = append(args, "--force")
							}
							if cmd.NArg() > 0 {
								args = append(args, cmd.Args().Slice()...)
							}
							return cmdProviderModelAdd(args)
						},
					},
					{
						Name:    "list",
						Aliases: []string{"ls"},
						Usage:   "List whitelisted models for provider",
						Action: func(ctx context.Context, cmd *cli.Command) error {
							return cmdProviderModelList(cmd.Args().Slice())
						},
					},
					{
						Name:    "remove",
						Aliases: []string{"rm"},
						Usage:   "Remove model from provider whitelist",
						Flags: []cli.Flag{
							&cli.BoolFlag{
								Name:  "no-interactive",
								Usage: "disable interactive selection",
							},
							&cli.BoolFlag{
								Name:  "force",
								Usage: "skip interactive selection",
							},
						},
						Action: func(ctx context.Context, cmd *cli.Command) error {
							var args []string
							if cmd.Bool("no-interactive") {
								args = append(args, "--no-interactive")
							}
							if cmd.Bool("force") {
								args = append(args, "--force")
							}
							if cmd.NArg() > 0 {
								args = append(args, cmd.Args().Slice()...)
							}
							return cmdProviderModelRemove(args)
						},
					},
					{
						Name:  "test",
						Usage: "Test health probe for specific provider model",
						Flags: []cli.Flag{
							&cli.DurationFlag{
								Name:  "timeout",
								Usage: "probe timeout",
								Value: 10 * time.Second,
							},
							&cli.BoolFlag{
								Name:  "no-interactive",
								Usage: "disable interactive selection",
							},
							&cli.BoolFlag{
								Name:  "force",
								Usage: "skip interactive selection",
							},
						},
						Action: func(ctx context.Context, cmd *cli.Command) error {
							var args []string
							if timeout := cmd.Duration("timeout"); timeout != 10*time.Second {
								args = append(args, "--timeout="+timeout.String())
							}
							if cmd.Bool("no-interactive") {
								args = append(args, "--no-interactive")
							}
							if cmd.Bool("force") {
								args = append(args, "--force")
							}
							if cmd.NArg() > 0 {
								args = append(args, cmd.Args().Slice()...)
							}
							return cmdProviderModelTest(args)
						},
					},
				},
			},
			cmdProvidersAccount(),
		},
	}
}

func cmdAuthSet(args []string) error {
	fs := flag.NewFlagSet("auth set", flag.ContinueOnError)
	accountFlag := fs.String("account", "", "account name under provider")
	interactiveFlag := fs.Bool("interactive", true, "prompt for masked credential input")
	noInteractiveFlag := fs.Bool("no-interactive", false, "disable masked credential prompt")
	forceFlag := fs.Bool("force", false, "read credential from stdin without prompt")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return errors.New("usage: tinyroute auth set <provider>  (credential is read from stdin)")
	}
	providerName := fs.Arg(0)

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

	p, ok := topo.Providers[providerName]
	if !ok {
		return fmt.Errorf("unknown provider %q (add it first with: tinyroute add)", providerName)
	}

	var cred string
	if *interactiveFlag && !*noInteractiveFlag && !*forceFlag {
		var err error
		promptMsg := fmt.Sprintf("Enter API key for provider %q:", providerName)
		if *accountFlag != "" {
			promptMsg = fmt.Sprintf("Enter API key for provider %q account %q:", providerName, *accountFlag)
		}
		cred, err = interactive.Password(promptMsg)
		if err != nil {
			return fmt.Errorf("read credential: %w", err)
		}
	} else {
		fmt.Fprintln(os.Stderr, "reading credential from stdin...")
		var err error
		cred, err = readSecretFromStdin()
		if err != nil {
			return fmt.Errorf("read credential: %w", err)
		}
	}

	if cred == "" {
		return errors.New("empty credential, aborting")
	}

	accountName := *accountFlag
	if accountName != "" {
		found := false
		for i, acc := range p.Accounts {
			if acc.Name == accountName {
				p.Accounts[i].APIKey = cred
				p.Accounts[i].Type = "static"
				found = true
				break
			}
		}
		if !found {
			p.Accounts = append(p.Accounts, config.Account{
				Name:   accountName,
				Type:   "static",
				APIKey: cred,
			})
		}
	} else {
		p.APIKey = cred
	}

	topo.Providers[providerName] = p
	if err := config.WriteTopology(svc.ConfigPath, topo); err != nil {
		return fmt.Errorf("write %s: %w", svc.ConfigPath, err)
	}

	if accountName != "" {
		fmt.Printf("updated credential for provider %q account %q\n", providerName, accountName)
	} else {
		fmt.Printf("updated credential for provider %q\n", providerName)
	}
	return nil
}

func readSecretFromStdin() (string, error) {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func cmdAuthList(args []string) error {
	fs := flag.NewFlagSet("auth list", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}

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

	names := make([]string, 0, len(topo.Providers))
	for name := range topo.Providers {
		names = append(names, name)
	}
	sort.Strings(names)

	tw := newTabWriter()
	fmt.Fprintln(tw, "PROVIDER\tDIALECT\tBASE URL\tCREDENTIAL")
	for _, name := range names {
		p := topo.Providers[name]
		cred := "(unset)"
		if p.APIKey != "" {
			cred = maskCredential(p.APIKey)
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", name, p.Dialect, p.BaseURL, cred)
	}
	return tw.Flush()
}

func maskCredential(s string) string {
	if len(s) <= 4 {
		return "****"
	}
	return strings.Repeat("*", len(s)-4) + s[len(s)-4:]
}

// ===========================================================================
// ADD
// ===========================================================================

func cmdAdd(args []string) error {
	fs := flag.NewFlagSet("add", flag.ContinueOnError)
	list := fs.Bool("list", false, "list available presets and exit")
	nameFlag := fs.String("name", "", "custom provider instance name in config.yaml")
	dialectFlag := fs.String("dialect", "", "select the alt dialect for dual-protocol presets")
	baseURLFlag := fs.String("base-url", "", "override base URL for the provider")
	interactiveFlag := fs.Bool("interactive", true, "enable interactive preset selection")
	noInteractiveFlag := fs.Bool("no-interactive", false, "disable interactive preset selection")
	forceFlag := fs.Bool("force", false, "skip interactive selection and overwrite prompts")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	if *list {
		tw := newTabWriter()
		fmt.Fprintln(tw, "NAME\tDIALECT\tAUTH\tTIER\tBASE URL\tCREDENTIAL VAR")
		for _, p := range preset.All() {
			authTag := "api key"
			if p.OAuthCapable {
				authTag = "oauth"
			} else if p.CredentialVar == "" {
				authTag = "-"
			}
			tierTag := "-"
			if p.Tier != "" {
				tierTag = p.Tier
				if p.FreeNote != "" {
					tierTag = fmt.Sprintf("%s (%s)", p.Tier, p.FreeNote)
				}
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n", p.Name, p.Dialect, authTag, tierTag, p.BaseURL, p.CredentialVar)
		}
		return tw.Flush()
	}

	svc, err := config.LoadService()
	if err != nil {
		return fmt.Errorf("load service config: %w", err)
	}

	data, err := os.ReadFile(svc.ConfigPath)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("read %s: %w", svc.ConfigPath, err)
		}
	}
	var topo config.Topology
	if len(data) > 0 {
		if topo, err = config.ParseRawTopology(data); err != nil {
			return fmt.Errorf("parse %s: %w", svc.ConfigPath, err)
		}
	}
	if topo.Providers == nil {
		topo.Providers = map[string]config.Provider{}
	}

	isInteractive := *interactiveFlag && !*noInteractiveFlag && !*forceFlag && interactive.CanPrompt()

	var presetName string
	var provKey string

	if fs.NArg() >= 1 {
		presetName = fs.Arg(0)
		if fs.NArg() >= 2 {
			provKey = fs.Arg(1)
		}
	} else if *interactiveFlag && !*noInteractiveFlag && !*forceFlag {
		presets := preset.All()
		options := make([]string, len(presets))
		for i, pr := range presets {
			authTag := "api key"
			if pr.OAuthCapable {
				authTag = "oauth"
			} else if pr.CredentialVar == "" {
				authTag = "—"
			}

			tierTag := ""
			if pr.Tier != "" {
				tierTag = pr.Tier
				if pr.FreeNote != "" {
					tierTag = fmt.Sprintf("%s (%s)", pr.Tier, pr.FreeNote)
				}
			}

			var tagDesc string
			if tierTag != "" {
				tagDesc = fmt.Sprintf("[%s, %s]", authTag, tierTag)
			} else {
				tagDesc = fmt.Sprintf("[%s]", authTag)
			}
			if pr.RiskNotice != "" {
				tagDesc += " [ToS notice]"
			}

			alreadyStr := ""
			if _, exists := topo.Providers[pr.Name]; exists {
				alreadyStr = " (already configured)"
			}

			options[i] = fmt.Sprintf("%-20s %-25s%s", pr.Name, tagDesc, alreadyStr)
		}
		selectedOption, err := interactive.Select("Select a provider preset to add:", options)
		if err != nil {
			return err
		}
		parts := strings.Fields(selectedOption)
		if len(parts) > 0 {
			presetName = parts[0]
		}
	} else {
		return fmt.Errorf("usage: tinyroute add <preset> [instance-name] (available: %s)", strings.Join(preset.Names(), ", "))
	}

	if *nameFlag != "" {
		provKey = *nameFlag
	}

	p := preset.Get(presetName)
	if p == nil {
		return fmt.Errorf("unknown preset %q (available: %s)", presetName, strings.Join(preset.Names(), ", "))
	}

	if provKey == "" {
		provKey = p.Name
		if isInteractive {
			inputName, err := interactive.Input(fmt.Sprintf("Provider instance name in config.yaml for %q:", p.Name), provKey, nil)
			if err != nil {
				return err
			}
			if strings.TrimSpace(inputName) != "" {
				provKey = strings.TrimSpace(inputName)
			}
		}
	}

	existing, alreadyConfigured := topo.Providers[provKey]
	if alreadyConfigured {
		if isInteractive {
			confirm, err := interactive.Confirm(fmt.Sprintf("Provider %q is already configured in %s. Do you want to update it?", provKey, svc.ConfigPath), false)
			if err != nil {
				return err
			}
			if !confirm {
				fmt.Printf("provider %q left unchanged.\n", provKey)
				return nil
			}
		} else if !*forceFlag {
			return fmt.Errorf("provider %q already configured (edit %s directly to change it, or use --force to overwrite)", provKey, svc.ConfigPath)
		}
	}

	dialectName, baseURL := p.Dialect, p.BaseURL
	if *dialectFlag != "" {
		switch *dialectFlag {
		case p.Dialect:
			// already selected
		case p.AltDialect:
			if p.AltDialect == "" {
				return fmt.Errorf("preset %q has no alternate dialect", p.Name)
			}
			dialectName, baseURL = p.AltDialect, p.AltBaseURL
		default:
			return fmt.Errorf("preset %q does not support dialect %q", p.Name, *dialectFlag)
		}
	}

	if *baseURLFlag != "" {
		baseURL = *baseURLFlag
	} else if isInteractive {
		customURL, err := interactive.Input(fmt.Sprintf("Base URL for provider %q:", provKey), baseURL, nil)
		if err != nil {
			return err
		}
		if strings.TrimSpace(customURL) != "" {
			baseURL = strings.TrimSpace(customURL)
		}
	}

	apiKey := ""
	shouldPerformLogin := false

	if p.OAuthCapable {
		if isInteractive {
			confirmLogin, err := interactive.Confirm(fmt.Sprintf("Provider %q supports OAuth (%s flow). Log in now?", provKey, p.FlowType), true)
			if err != nil {
				return err
			}
			shouldPerformLogin = confirmLogin
		}
	} else {
		if envVal := os.Getenv(p.CredentialVar); envVal != "" {
			apiKey = envVal
		} else if envVal := os.Getenv(strings.ToUpper(strings.ReplaceAll(provKey, "-", "_")) + "_API_KEY"); envVal != "" {
			apiKey = envVal
		} else if alreadyConfigured && existing.APIKey != "" {
			apiKey = existing.APIKey
		}

		if isInteractive {
			var promptMsg string
			if apiKey != "" && !strings.HasPrefix(apiKey, "${") {
				promptMsg = fmt.Sprintf("Enter API key for %q (press Enter to keep current key):", provKey)
			} else if envVal := os.Getenv(p.CredentialVar); envVal != "" {
				promptMsg = fmt.Sprintf("Enter API key for %q (press Enter to hardcode key from %s):", provKey, p.CredentialVar)
			} else {
				promptMsg = fmt.Sprintf("Enter API key for %q (optional, press Enter to skip):", provKey)
			}
			inputKey, err := interactive.Password(promptMsg)
			if err != nil {
				return err
			}
			if strings.TrimSpace(inputKey) != "" {
				apiKey = strings.TrimSpace(inputKey)
			}
		}
	}

	if p.OAuthCapable && shouldPerformLogin {
		if err := cmdAuthLogin(context.Background(), []string{provKey}, true); err != nil {
			fmt.Printf("\nOAuth login failed or cancelled: %v\n", err)
			fmt.Printf("Provider configuration was NOT saved to config.yaml.\n")
			return fmt.Errorf("oauth login failed: %w", err)
		}
	}

	topo.Providers[provKey] = config.Provider{
		Dialect:   dialectName,
		BaseURL:   baseURL,
		Transport: p.Transport,
		APIKey:    apiKey,
	}

	if err := config.WriteTopology(svc.ConfigPath, topo); err != nil {
		return fmt.Errorf("write %s: %w", svc.ConfigPath, err)
	}

	if alreadyConfigured {
		fmt.Printf("updated provider %q (preset=%s dialect=%s base_url=%s)\n", provKey, p.Name, dialectName, baseURL)
	} else {
		fmt.Printf("added provider %q (preset=%s dialect=%s base_url=%s)\n", provKey, p.Name, dialectName, baseURL)
	}

	if p.RiskNotice != "" {
		fmt.Printf("Notice: %s\n", p.RiskNotice)
	}

	if p.OAuthCapable {
		if !shouldPerformLogin {
			fmt.Printf("Run 'tinyroute auth login %s' to authenticate via OAuth.\n", provKey)
		}
	} else if apiKey != "" && !strings.HasPrefix(apiKey, "${") {
		fmt.Printf("credential updated for provider %q\n", provKey)
	} else if strings.HasPrefix(apiKey, "${") {
		fmt.Printf("set its credential with: tinyroute auth set %s\n", provKey)
		fmt.Printf("or export %s before starting tinyroute\n", p.CredentialVar)
	} else {
		fmt.Printf("no credential set for provider %q (optional)\n", provKey)
	}
	return nil
}

// ===========================================================================
// COMPACT
// ===========================================================================

func cmdCompact(args []string) error {
	fs := flag.NewFlagSet("compact", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "report what would be removed without deleting")
	interactiveFlag := fs.Bool("interactive", true, "show progress bar during compaction")
	noInteractiveFlag := fs.Bool("no-interactive", false, "disable progress bar")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	_, _, _ = dryRun, interactiveFlag, noInteractiveFlag

	fmt.Println("nothing to compact (payload blob storage is disabled)")
	return nil
}

func parseFlags(fs *flag.FlagSet, args []string) error {
	var flags, nonFlags []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
		} else {
			nonFlags = append(nonFlags, a)
		}
	}
	return fs.Parse(append(flags, nonFlags...))
}

func cmdProviderModelAdd(args []string) error {
	fs := flag.NewFlagSet("provider model add", flag.ContinueOnError)
	modelsFlag := fs.String("models", "", "comma-separated list of models to add")
	interactiveFlag := fs.Bool("interactive", true, "enable interactive model selection")
	noInteractiveFlag := fs.Bool("no-interactive", false, "disable interactive model selection")
	forceFlag := fs.Bool("force", false, "skip interactive selection")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	isInteractive := *interactiveFlag && !*noInteractiveFlag && !*forceFlag && interactive.CanPrompt()

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

	if len(topo.Providers) == 0 {
		return fmt.Errorf("no providers configured in %s", svc.ConfigPath)
	}

	var provName string
	if fs.NArg() >= 1 {
		provName = fs.Arg(0)
	} else {
		var provList []string
		for name := range topo.Providers {
			provList = append(provList, name)
		}
		sort.Strings(provList)

		if len(provList) == 1 {
			provName = provList[0]
			fmt.Printf("Auto-selected provider %q\n", provName)
		} else if isInteractive {
			chosen, err := interactive.Select("Select provider to add models to:", provList)
			if err != nil {
				return err
			}
			provName = chosen
		} else {
			return errors.New("provider argument is required (usage: tinyroute provider model add <provider> [--models=m1,m2])")
		}
	}

	prov, ok := topo.Providers[provName]
	if !ok {
		return fmt.Errorf("provider %q not found in %s", provName, svc.ConfigPath)
	}

	var selectedModels []string
	if *modelsFlag != "" {
		for _, m := range strings.Split(*modelsFlag, ",") {
			if trimmed := strings.TrimSpace(m); trimmed != "" {
				selectedModels = append(selectedModels, trimmed)
			}
		}
	} else if fs.NArg() > 1 {
		selectedModels = fs.Args()[1:]
	} else {
		var available []string
		apiKey := prov.APIKey
		if apiKey == "" {
			if store, err := credential.NewStore(svc.CredentialsPath); err == nil {
				if rec, ok := store.Get(provName); ok {
					if rec.AccessToken != "" {
						apiKey = rec.AccessToken
					} else if rec.RefreshToken != "" {
						apiKey = rec.RefreshToken
					}
				}
			}
		}

		if fetched, err := config.FetchProviderModels(prov.BaseURL, apiKey, prov.Dialect); err == nil && len(fetched) > 0 {
			available = fetched
		} else {
			cat, err := config.LoadOrRefreshCatalog("", "")
			if err == nil && cat != nil {
				if m, ok := cat.Providers[strings.ToLower(provName)]; ok {
					available = m
				} else if p := preset.Get(provName); p != nil {
					if m, ok := cat.Providers[strings.ToLower(p.Name)]; ok {
						available = m
					}
				}
				if len(available) == 0 && prov.Dialect != "openai" {
					if m, ok := cat.Providers[strings.ToLower(prov.Dialect)]; ok {
						available = m
					}
				}
			}
		}

		if len(available) == 0 {
			if isInteractive {
				inputModel, err := interactive.Input(fmt.Sprintf("No catalog models found. Enter model ID to whitelist for provider %q:", provName), "", nil)
				if err != nil {
					return err
				}
				if strings.TrimSpace(inputModel) != "" {
					selectedModels = []string{strings.TrimSpace(inputModel)}
				} else {
					return fmt.Errorf("no models specified for provider %q", provName)
				}
			} else {
				return fmt.Errorf("no catalog models found for provider %q (dialect: %s)", provName, prov.Dialect)
			}
		}

		if len(available) == 1 {
			selectedModels = available
			fmt.Printf("Auto-selected single catalog model %q for provider %q\n", available[0], provName)
		} else if isInteractive {
			choices, err := interactive.MultiSelect(fmt.Sprintf("Select models to whitelist for provider %q:", provName), available)
			if err != nil {
				return err
			}
			selectedModels = choices
		} else {
			return errors.New("model argument or --models flag is required in non-interactive mode")
		}
	}

	if len(selectedModels) == 0 {
		fmt.Println("No models selected.")
		return nil
	}

	seen := make(map[string]bool)
	for _, m := range prov.Models {
		seen[m] = true
	}
	for _, m := range selectedModels {
		if !seen[m] {
			seen[m] = true
			prov.Models = append(prov.Models, m)
		}
	}
	sort.Strings(prov.Models)
	topo.Providers[provName] = prov

	if err := config.WriteTopology(svc.ConfigPath, topo); err != nil {
		return fmt.Errorf("write %s: %w", svc.ConfigPath, err)
	}

	fmt.Printf("updated provider %q whitelist: %v\n", provName, prov.Models)
	return nil
}

func cmdProviderModelList(args []string) error {
	fs := flag.NewFlagSet("provider model list", flag.ContinueOnError)
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	svc, err := config.LoadService()
	if err != nil {
		return fmt.Errorf("load service config: %w", err)
	}
	data, err := os.ReadFile(svc.ConfigPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", svc.ConfigPath, err)
	}
	topo, err := config.ParseTopology(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", svc.ConfigPath, err)
	}

	w := newTabWriter()
	defer w.Flush()

	if fs.NArg() > 0 {
		provName := fs.Arg(0)
		prov, ok := topo.Providers[provName]
		if !ok {
			return fmt.Errorf("provider %q not found in %s", provName, svc.ConfigPath)
		}
		fmt.Fprintf(w, "PROVIDER\tMODELS\n")
		modelsStr := strings.Join(prov.Models, ", ")
		if modelsStr == "" {
			modelsStr = "-"
		}
		fmt.Fprintf(w, "%s\t%s\n", provName, modelsStr)
		return nil
	}

	fmt.Fprintf(w, "PROVIDER\tMODELS\n")
	var names []string
	for k := range topo.Providers {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		p := topo.Providers[k]
		modelsStr := strings.Join(p.Models, ", ")
		if modelsStr == "" {
			modelsStr = "-"
		}
		fmt.Fprintf(w, "%s\t%s\n", k, modelsStr)
	}
	return nil
}

func cmdProviderModelRemove(args []string) error {
	fs := flag.NewFlagSet("provider model remove", flag.ContinueOnError)
	interactiveFlag := fs.Bool("interactive", true, "enable interactive model removal")
	noInteractiveFlag := fs.Bool("no-interactive", false, "disable interactive model removal")
	forceFlag := fs.Bool("force", false, "skip interactive selection")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	isInteractive := *interactiveFlag && !*noInteractiveFlag && !*forceFlag && interactive.CanPrompt()

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

	var provName string
	if fs.NArg() >= 1 {
		provName = fs.Arg(0)
	} else {
		var provList []string
		for name, prov := range topo.Providers {
			if len(prov.Models) > 0 {
				provList = append(provList, name)
			}
		}
		sort.Strings(provList)

		if len(provList) == 0 {
			fmt.Println("No providers have whitelisted models to remove.")
			return nil
		} else if len(provList) == 1 {
			provName = provList[0]
			fmt.Printf("Auto-selected provider %q\n", provName)
		} else if isInteractive {
			chosen, err := interactive.Select("Select provider to remove model from:", provList)
			if err != nil {
				return err
			}
			provName = chosen
		} else {
			return errors.New("provider argument is required (usage: tinyroute provider model remove <provider> <model>)")
		}
	}

	prov, ok := topo.Providers[provName]
	if !ok {
		return fmt.Errorf("provider %q not found in %s", provName, svc.ConfigPath)
	}

	var targetModel string
	if fs.NArg() >= 2 {
		targetModel = fs.Arg(1)
	} else {
		if len(prov.Models) == 0 {
			fmt.Printf("Provider %q has no whitelisted models to remove.\n", provName)
			return nil
		} else if len(prov.Models) == 1 {
			targetModel = prov.Models[0]
			fmt.Printf("Auto-selected model %q for removal\n", targetModel)
		} else if isInteractive {
			chosen, err := interactive.Select(fmt.Sprintf("Select model to remove from provider %q:", provName), prov.Models)
			if err != nil {
				return err
			}
			targetModel = chosen
		} else {
			return errors.New("model argument is required in non-interactive mode")
		}
	}

	var updated []string
	removed := false
	for _, m := range prov.Models {
		if m == targetModel {
			removed = true
			continue
		}
		updated = append(updated, m)
	}

	if !removed {
		return fmt.Errorf("model %q was not found in whitelist for provider %q", targetModel, provName)
	}

	prov.Models = updated
	topo.Providers[provName] = prov

	if err := config.WriteTopology(svc.ConfigPath, topo); err != nil {
		return fmt.Errorf("write %s: %w", svc.ConfigPath, err)
	}

	fmt.Printf("removed model %q from provider %q whitelist\n", targetModel, provName)
	return nil
}

func cmdProviderModelTest(args []string) error {
	fs := flag.NewFlagSet("provider model test", flag.ContinueOnError)
	timeout := fs.Duration("timeout", 10*time.Second, "probe timeout")
	interactiveFlag := fs.Bool("interactive", true, "enable interactive model test selection")
	noInteractiveFlag := fs.Bool("no-interactive", false, "disable interactive model test selection")
	forceFlag := fs.Bool("force", false, "skip interactive selection")
	if err := parseFlags(fs, args); err != nil {
		return err
	}

	isInteractive := *interactiveFlag && !*noInteractiveFlag && !*forceFlag && interactive.CanPrompt()

	svc, err := config.LoadService()
	if err != nil {
		return fmt.Errorf("load service config: %w", err)
	}
	data, err := os.ReadFile(svc.ConfigPath)
	if err != nil {
		return fmt.Errorf("read %s: %w", svc.ConfigPath, err)
	}
	topo, err := config.ParseTopology(data)
	if err != nil {
		return fmt.Errorf("parse %s: %w", svc.ConfigPath, err)
	}

	if len(topo.Providers) == 0 {
		return fmt.Errorf("no providers configured in %s", svc.ConfigPath)
	}

	var provName string
	if fs.NArg() >= 1 {
		provName = fs.Arg(0)
	} else {
		var provList []string
		for name, p := range topo.Providers {
			if len(p.Models) > 0 {
				provList = append(provList, name)
			}
		}
		if len(provList) == 0 {
			for name := range topo.Providers {
				provList = append(provList, name)
			}
		}
		sort.Strings(provList)

		if len(provList) == 1 {
			provName = provList[0]
			fmt.Printf("Auto-selected provider %q\n", provName)
		} else if isInteractive {
			chosen, err := interactive.Select("Select provider to test:", provList)
			if err != nil {
				return err
			}
			provName = chosen
		} else {
			return errors.New("provider argument is required (usage: tinyroute provider model test <provider> <model>)")
		}
	}

	prov, ok := topo.Providers[provName]
	if !ok {
		return fmt.Errorf("provider %q not found in %s", provName, svc.ConfigPath)
	}

	var targetModel string
	if fs.NArg() >= 2 {
		targetModel = fs.Arg(1)
	} else {
		if len(prov.Models) == 0 {
			fmt.Printf("Provider %q has no whitelisted models to test.\n", provName)
			return nil
		} else if len(prov.Models) == 1 {
			targetModel = prov.Models[0]
			fmt.Printf("Auto-selected model %q for testing\n", targetModel)
		} else if isInteractive {
			chosen, err := interactive.Select(fmt.Sprintf("Select model to test for provider %q:", provName), prov.Models)
			if err != nil {
				return err
			}
			targetModel = chosen
		} else {
			return errors.New("model argument is required in non-interactive mode")
		}
	}

	status, elapsed, err := probe.TestModel(context.Background(), provName, prov, targetModel, svc.CredentialsPath, *timeout)
	if err != nil {
		return err
	}

	if status >= 200 && status < 300 {
		fmt.Printf("%-15s %-25s OK: %d in %s\n", provName, targetModel, status, elapsed.Round(time.Millisecond))
		return nil
	}
	return fmt.Errorf("probe failed with status %d", status)
}
