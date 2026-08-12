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
	"github.com/urfave/cli/v3"
)

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
					if mode := cmd.String("mode"); mode != "" {
						args = append(args, "--mode="+mode)
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
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: tinyroute combos add <name> --members=... [--mode=...] [--capabilities=...]")
	}
	name := fs.Arg(0)
	if *membersStr == "" {
		return fmt.Errorf("--members is required (comma-separated provider[:model] entries)")
	}

	validModes := map[string]bool{"ordered": true, "pool": true, "fused": true}
	if !validModes[*mode] {
		return fmt.Errorf("invalid mode %q: must be ordered, pool, or fused", *mode)
	}

	members := splitAndTrim(*membersStr)
	var capabilities []string
	if *capsStr != "" {
		capabilities = splitAndTrim(*capsStr)
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

	for _, c := range topo.Combos {
		if c.Name == name {
			return fmt.Errorf("combo %q already exists", name)
		}
	}

	topo.Combos = append(topo.Combos, config.Combo{
		Name:         name,
		Members:      members,
		Mode:         *mode,
		Capabilities: capabilities,
	})

	if errs := config.ValidateTopology(topo, nil); len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintf(os.Stderr, "  - %v\n", e)
		}
		return fmt.Errorf("config validation failed after adding combo %q", name)
	}

	if err := config.WriteTopology(svc.ConfigPath, topo); err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	fmt.Printf("added combo %q (mode=%s, %d members)\n", name, *mode, len(members))
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
