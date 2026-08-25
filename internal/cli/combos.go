package cli

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/oniharnantyo/tinyroute/internal/cli/interactive"
	"github.com/oniharnantyo/tinyroute/internal/config"
	"github.com/oniharnantyo/tinyroute/internal/dialect"
	"github.com/pterm/pterm"
	"github.com/urfave/cli/v3"
)

// ValidateComboName validates the charset, forbids colons, and checks uniqueness.
func ValidateComboName(name string, existingCombos []config.Combo) error {
	return config.ValidateComboName(name, existingCombos)
}

// GetMemberCandidates derives provider:model candidates from parsed topology provider whitelists.
// Providers with no whitelisted models are skipped.
func GetMemberCandidates(topo config.Topology) []string {
	return config.GetMemberCandidates(topo)
}

// AddComboCore validates inputs and appends a new combo to the topology.
func AddComboCore(topo config.Topology, name string, members []string, mode string, capabilities []string) (config.Topology, error) {
	name = strings.TrimSpace(name)
	if mode == "" {
		mode = "ordered"
	}
	validModes := map[string]bool{"ordered": true, "pool": true, "fused": true}
	if !validModes[mode] {
		return topo, fmt.Errorf("invalid mode %q: must be ordered, pool, or fused", mode)
	}

	if err := ValidateComboName(name, topo.Combos); err != nil {
		return topo, err
	}

	if len(members) < 1 {
		return topo, fmt.Errorf("at least 1 member is required")
	}
	for _, m := range members {
		if strings.TrimSpace(m) == "" {
			return topo, fmt.Errorf("member cannot be empty")
		}
	}

	topo.Combos = append(topo.Combos, config.Combo{
		Name:         name,
		Members:      members,
		Mode:         mode,
		Capabilities: capabilities,
	})

	if errs := config.ValidateTopology(topo, dialect.Names()); len(errs) > 0 {
		var errMsgs []string
		for _, e := range errs {
			errMsgs = append(errMsgs, e.Error())
		}
		return topo, fmt.Errorf("config validation failed: %s", strings.Join(errMsgs, "; "))
	}

	return topo, nil
}

// RunComboAddWizard guides the user through the 5-step combo creation process.
func RunComboAddWizard(topo config.Topology, initialName string) (string, []string, string, []string, error) {
	pterm.DefaultHeader.WithFullWidth().WithBackgroundStyle(pterm.NewStyle(pterm.BgDarkGray)).Println("tinyroute Combo Wizard")
	pterm.Println("Create a multi-model combo (ordered, pool, or fused).")
	pterm.Println()

	// Step 1: Name
	pterm.DefaultSection.Println("Step 1 of 5: Name")
	name, err := interactive.Input("Enter combo name:", initialName, func(val string) error {
		return ValidateComboName(val, topo.Combos)
	})
	if err != nil {
		return "", nil, "", nil, err
	}

	// Step 2: Members
	pterm.DefaultSection.Println("Step 2 of 5: Members")
	pterm.Println("Pick provider@account:model to draw from a specific connection, or provider:model to use any connection.")
	candidates := GetMemberCandidates(topo)
	var members []string
	ordinals := []string{"FIRST", "SECOND", "THIRD", "FOURTH", "FIFTH", "SIXTH", "SEVENTH", "EIGHTH", "NINTH", "TENTH"}
	for {
		ord := fmt.Sprintf("#%d", len(members)+1)
		if len(members) < len(ordinals) {
			ord = ordinals[len(members)]
		}

		var available []string
		chosenSet := make(map[string]bool)
		for _, m := range members {
			chosenSet[m] = true
		}
		for _, c := range candidates {
			if !chosenSet[c] {
				available = append(available, c)
			}
		}

		if len(members) >= 1 {
			available = append(available, "Done — enough members")
		}

		if len(available) == 0 {
			if len(members) >= 1 {
				break
			}
			return "", nil, "", nil, fmt.Errorf("no more models available to select")
		}

		sel, err := interactive.Select(fmt.Sprintf("Select the %s member:", ord), available)
		if err != nil {
			return "", nil, "", nil, err
		}
		if sel == "Done — enough members" {
			break
		}
		members = append(members, sel)
	}

	// Step 3: Mode
	pterm.DefaultSection.Println("Step 3 of 5: Mode")
	modeOptions := []string{
		"ordered - try members in sequence until one succeeds (default)",
		"pool - load-balance requests across healthy members",
		"fused - combine responses from multiple models",
	}
	selectedModeStr, err := interactive.Select("Select execution mode:", modeOptions)
	if err != nil {
		return "", nil, "", nil, err
	}
	mode := "ordered"
	if strings.HasPrefix(selectedModeStr, "pool") {
		mode = "pool"
	} else if strings.HasPrefix(selectedModeStr, "fused") {
		mode = "fused"
	}

	// Step 4: Capabilities
	pterm.DefaultSection.Println("Step 4 of 5: Capabilities (optional)")
	capOptions := []string{"vision", "pdf", "audio", "video"}
	selectedCaps, err := interactive.MultiSelect("Select supported capabilities (empty selection allowed):", capOptions)
	if err != nil {
		selectedCaps = nil
	}

	// Step 5: Review & Confirm
	pterm.DefaultSection.Println("Step 5 of 5: Review & Confirm")
	pterm.Printf("  • Name: %s\n", name)
	pterm.Printf("  • Mode: %s\n", mode)
	pterm.Println("  • Members (in order):")
	for i, m := range members {
		pterm.Printf("    %d. %s\n", i+1, m)
	}
	if len(selectedCaps) > 0 {
		pterm.Printf("  • Capabilities: %s\n", strings.Join(selectedCaps, ", "))
	} else {
		pterm.Println("  • Capabilities: none")
	}
	pterm.Println()

	confirmed, err := interactive.Confirm("Save combo configuration?", true)
	if err != nil || !confirmed {
		pterm.Info.Println("Setup cancelled; no changes saved.")
		return "", nil, "", nil, fmt.Errorf("aborted")
	}

	return name, members, mode, selectedCaps, nil
}

func cmdCombos() *cli.Command {
	return &cli.Command{
		Name:  "combos",
		Usage: "Manage combos (multi-model panels: ordered, pool, fused)",
		Commands: []*cli.Command{
			{
				Name:      "add",
				Usage:     "Add a combo to config",
				ArgsUsage: "<name>",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "members",
						Usage: "comma-separated list of provider[:model] or provider@account[:model] members",
					},
					&cli.StringFlag{
						Name:  "mode",
						Usage: "combo mode: ordered (default), pool, or fused",
						Value: "ordered",
					},
					&cli.StringFlag{
						Name:  "capabilities",
						Usage: "comma-separated capability list (e.g. vision,pdf,audio)",
					},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					var args []string
					if members := cmd.String("members"); members != "" {
						args = append(args, "--members="+members)
					}
					if cmd.IsSet("mode") {
						if mode := cmd.String("mode"); mode != "" {
							args = append(args, "--mode="+mode)
						}
					}
					if caps := cmd.String("capabilities"); caps != "" {
						args = append(args, "--capabilities="+caps)
					}
					if cmd.NArg() > 0 {
						args = append(args, cmd.Args().Slice()...)
					}
					return cmdCombosAdd(args)
				},
			},
			{
				Name:    "list",
				Aliases: []string{"ls"},
				Usage:   "List all combos",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return cmdCombosList(cmd.Args().Slice())
				},
			},
			{
				Name:      "remove",
				Aliases:   []string{"rm"},
				Usage:     "Remove a combo from config",
				ArgsUsage: "<name>",
				Flags: []cli.Flag{
					&cli.BoolFlag{
						Name:    "interactive",
						Aliases: []string{"i"},
						Usage:   "enable confirmation prompt before removing (default: true in interactive terminals)",
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
					return cmdCombosRemove(args)
				},
			},
		},
	}
}

func cmdCombosAdd(args []string) error {
	fs := flag.NewFlagSet("combos add", flag.ContinueOnError)
	membersStr := fs.String("members", "", "comma-separated list of provider[:model] or provider@account[:model] members")
	mode := fs.String("mode", "ordered", "combo mode: ordered, pool, or fused")
	capsStr := fs.String("capabilities", "", "comma-separated capability list")

	var positional []string
	var flagArgs []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			flagArgs = append(flagArgs, a)
		} else {
			positional = append(positional, a)
		}
	}

	if err := fs.Parse(flagArgs); err != nil {
		return err
	}

	name := ""
	if len(positional) > 0 {
		name = positional[0]
	}

	svc, err := config.LoadService()
	if err != nil {
		return fmt.Errorf("load service config: %w", err)
	}

	data, err := os.ReadFile(svc.ConfigPath)
	if err != nil {
		return fmt.Errorf("read config (%s): %w", svc.ConfigPath, err)
	}
	topo, err := config.ParseTopology(data)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	// Fully-typed shortcut path
	if name != "" && *membersStr != "" {
		members := splitAndTrim(*membersStr)
		var capabilities []string
		if *capsStr != "" {
			capabilities = splitAndTrim(*capsStr)
		}

		updatedTopo, err := AddComboCore(topo, name, members, *mode, capabilities)
		if err != nil {
			return err
		}

		if err := config.WriteTopology(svc.ConfigPath, updatedTopo); err != nil {
			return fmt.Errorf("write config: %w", err)
		}

		fmt.Printf("added combo %q (mode=%s, %d members)\n", name, *mode, len(members))
		return nil
	}

	// Interactive / missing arguments path
	candidates := GetMemberCandidates(topo)
	if len(candidates) == 0 {
		fmt.Println("No providers or whitelisted models configured.")
		fmt.Println("Add a provider first with: tinyroute provider add")
		return nil
	}

	if !interactive.CanPrompt() {
		return fmt.Errorf("missing required arguments; usage: tinyroute combos add <name> --members=provider:model1,provider:model2 [--mode=ordered] [--capabilities=vision,pdf]")
	}

	wizardName, members, wizardMode, caps, err := RunComboAddWizard(topo, name)
	if err != nil {
		if err.Error() == "aborted" {
			return nil
		}
		return err
	}

	updatedTopo, err := AddComboCore(topo, wizardName, members, wizardMode, caps)
	if err != nil {
		return err
	}

	if err := config.WriteTopology(svc.ConfigPath, updatedTopo); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	fmt.Printf("added combo %q (mode=%s, %d members)\n", wizardName, wizardMode, len(members))
	fmt.Printf("Clients can now request model: %s\n", wizardName)
	return nil
}

func cmdCombosList(_ []string) error {
	svc, err := config.LoadService()
	if err != nil {
		return fmt.Errorf("load service config: %w", err)
	}

	data, err := os.ReadFile(svc.ConfigPath)
	if err != nil {
		return fmt.Errorf("read config (%s): %w", svc.ConfigPath, err)
	}
	topo, err := config.ParseTopology(data)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	if len(topo.Combos) == 0 {
		fmt.Println("No combos configured.")
		fmt.Println("Add one with: tinyroute combos add <name> --members=provider1:model1,provider2:model2")
		return nil
	}

	sort.Slice(topo.Combos, func(i, j int) bool {
		return topo.Combos[i].Name < topo.Combos[j].Name
	})

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "NAME\tMODE\tMEMBERS\tCAPABILITIES")
	for _, c := range topo.Combos {
		mode := c.Mode
		if mode == "" {
			mode = "ordered"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", c.Name, mode, strings.Join(c.Members, ", "), strings.Join(c.Capabilities, ", "))
	}
	return w.Flush()
}

func cmdCombosRemove(args []string) error {
	fs := flag.NewFlagSet("combos remove", flag.ContinueOnError)
	noInteractive := fs.Bool("no-interactive", false, "disable confirmation prompt")
	force := fs.Bool("force", false, "skip confirmation prompt")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: tinyroute combos remove <name>")
	}
	name := fs.Arg(0)

	svc, err := config.LoadService()
	if err != nil {
		return fmt.Errorf("load service config: %w", err)
	}

	data, err := os.ReadFile(svc.ConfigPath)
	if err != nil {
		return fmt.Errorf("read config (%s): %w", svc.ConfigPath, err)
	}
	topo, err := config.ParseTopology(data)
	if err != nil {
		return fmt.Errorf("parse config: %w", err)
	}

	idx := -1
	for i, c := range topo.Combos {
		if c.Name == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return fmt.Errorf("combo %q not found", name)
	}

	if !*noInteractive && !*force {
		confirmed, err := interactive.Confirm(fmt.Sprintf("Remove combo %q?", name), true)
		if err != nil {
			return err
		}
		if !confirmed {
			fmt.Println("aborted")
			return nil
		}
	}

	topo.Combos = append(topo.Combos[:idx], topo.Combos[idx+1:]...)

	if err := config.WriteTopology(svc.ConfigPath, topo); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	fmt.Printf("removed combo %q\n", name)
	return nil
}

func splitAndTrim(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
