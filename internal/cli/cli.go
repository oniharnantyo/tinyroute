// Package cli provides the CLI command structure for tinyroute.
package cli

import (
	"context"

	"github.com/oniharnantyo/tinyroute/internal/config"
	"github.com/urfave/cli/v3"
)

// New creates and returns the root CLI command for tinyroute. Running
// `tinyroute` with no subcommand prints the command menu (help).
func New() *cli.Command {
	return &cli.Command{
		Name:    "tinyroute",
		Usage:   "A tiny multi-provider LLM proxy (run with no arguments to see the command menu)",
		Version: "dev",
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			_ = config.LoadDotenv("")
			return ctx, nil
		},
		Commands: []*cli.Command{
			cmdServe(),
			wrapCommand("init", "Scaffold config and create the first key", cmdInit),
			cmdKeys(),
			cmdProviders(),
			cmdCombos(),
			cmdClient(),
			cmdHistory(),
		},
	}
}

// wrapCommand converts a func([]string) error to a cli.Command.
func wrapCommand(name, usage string, fn func([]string) error) *cli.Command {
	return &cli.Command{
		Name:  name,
		Usage: usage,
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:    "interactive",
				Aliases: []string{"i"},
				Usage:   "enable interactive prompts and progress indicators (default: true in interactive terminals)",
				Value:   true,
			},
			&cli.BoolFlag{
				Name:  "no-interactive",
				Usage: "disable interactive prompts and progress indicators",
			},
			&cli.BoolFlag{
				Name:  "force",
				Usage: "explicitly skip interactive prompts",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			args := append([]string(nil), cmd.Args().Slice()...)
			if cmd.IsSet("interactive") {
				if cmd.Bool("interactive") {
					args = append(args, "--interactive")
				} else {
					args = append(args, "--no-interactive")
				}
			}
			if cmd.Bool("no-interactive") {
				args = append(args, "--no-interactive")
			}
			if cmd.Bool("force") {
				args = append(args, "--force")
			}
			return fn(args)
		},
	}
}
