package commands

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"

	cleangit "github.com/rudderlabs/git-hooks/internal/clean-local-git"
	"github.com/urfave/cli/v2"
)

// ScanLocal scans repositories for local hooksPath overrides and optionally removes them.
var ScanLocal = &cli.Command{
	Name:            "scan-local",
	SkipFlagParsing: true,
	Usage:           "Scan for repositories with local hooksPath overrides",
	ArgsUsage:       "[PATH]",
	Flags: []cli.Flag{
		&cli.IntFlag{
			Name:  "max-depth",
			Value: 10,
			Usage: "Maximum directory depth to search",
		},
		&cli.BoolFlag{
			Name:  "verbose",
			Usage: "Show detailed output",
		},
		&cli.BoolFlag{
			Name:  "auto-fix",
			Usage: "Automatically remove local hooksPath overrides",
		},
		&cli.BoolFlag{
			Name:    "yes",
			Aliases: []string{"y"},
			Usage:   "Remove overrides without prompting",
		},
	},
	Action: parseScanLocal,
}

func parseScanLocal(c *cli.Context) error {
	// urfave stops at PATH, so keep option values together before moving positional arguments.
	var flags, paths []string
	args := c.Args().Slice()
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			paths = append(paths, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			paths = append(paths, arg)
			continue
		}
		flags = append(flags, arg)
		if (arg == "--max-depth" || arg == "-max-depth") && i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	set := flag.NewFlagSet(c.Command.Name, flag.ContinueOnError)
	set.SetOutput(c.App.ErrWriter)
	for _, option := range c.Command.Flags {
		if err := option.Apply(set); err != nil {
			return err
		}
	}
	args = append(flags, "--")
	args = append(args, paths...)
	if err := set.Parse(args); err != nil {
		return err
	}
	parsed := cli.NewContext(c.App, set, c)
	parsed.Command = c.Command
	if parsed.Bool("help") {
		cli.HelpPrinter(c.App.Writer, cli.CommandHelpTemplate, c.Command)
		return nil
	}
	return scanLocalAction(parsed)
}

func scanLocalAction(c *cli.Context) error {
	if c.NArg() > 1 {
		return fmt.Errorf("expected at most one PATH")
	}
	out := c.App.Writer
	path := "."
	if c.NArg() > 0 {
		path = c.Args().First()
	}

	maxDepth := c.Int("max-depth")
	verbose := c.Bool("verbose")
	autoFix := c.Bool("auto-fix")

	ctx, stop := signal.NotifyContext(c.Context, os.Interrupt)
	defer stop()

	fmt.Fprintln(out, "🔍 Scanning for local hook overrides...")
	repos, err := cleangit.Scan(ctx, path, maxDepth)
	if err != nil {
		return fmt.Errorf("scanning repositories: %w", err)
	}

	var overrides, unreadable []cleangit.Repository
	for _, repo := range repos {
		if repo.Err != nil {
			unreadable = append(unreadable, repo)
		} else {
			overrides = append(overrides, repo)
		}
	}
	if len(unreadable) > 0 {
		// An unreadable config may hide an override; report it, but do not count it as one.
		fmt.Fprintf(out, "\n⚠️  Could not read the config of %d %s:\n\n",
			len(unreadable), pluralize("repository", "repositories", len(unreadable)))
		for _, repo := range unreadable {
			fmt.Fprintf(out, "  • %s: %v\n", repo.Path, repo.Err)
		}
	}
	repos = overrides

	if len(repos) == 0 {
		fmt.Fprintln(out, "\n✅ No repositories found with local hooksPath overrides.")
		return nil
	}

	fmt.Fprintf(out, "\nFound %d %s with local hook overrides:\n\n",
		len(repos),
		pluralize("repository", "repositories", len(repos)))

	for _, repo := range repos {
		if verbose {
			fmt.Fprintf(out, "  📁 %s\n", repo.Path)
			fmt.Fprintf(out, "     Override: core.hooksPath = %s\n", repo.CustomHooksPath)
		} else {
			fmt.Fprintf(out, "  • %s (core.hooksPath = %s)\n", repo.Path, repo.CustomHooksPath)
		}
	}

	if !autoFix {
		fmt.Fprintln(out, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Fprintf(out, "Summary: %d %s with local overrides\n",
			len(repos),
			pluralize("repository", "repositories", len(repos)))
		fmt.Fprintln(out, "\n💡 Tip: Use --auto-fix to remove these overrides")
		return nil
	}

	if !c.Bool("yes") {
		// A shell terminal check avoids treating character devices such as /dev/null as terminals.
		terminal := exec.CommandContext(ctx, "sh", "-c", "test -t 0")
		terminal.Stdin = c.App.Reader
		if err := terminal.Run(); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return fmt.Errorf("stdin is not a terminal; pass --yes with --auto-fix")
		}
		fmt.Fprint(out, "\nRemove these local overrides? (y/N): ")
		reader := bufio.NewReader(c.App.Reader)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("reading user input: %w", err)
		}
		response = strings.TrimSpace(response)
		if response != "y" && response != "Y" {
			fmt.Fprintln(out, "Cancelled.")
			return nil
		}
	}

	fmt.Fprintln(out, "\n🔧 Removing overrides...")
	summary := cleangit.Clean(ctx, repos)

	fmt.Fprintln(out)
	for _, result := range summary.Results {
		if result.Error != nil {
			fmt.Fprintf(out, "❌ %s: %v\n", result.Repository.Path, result.Error)
		} else {
			if verbose {
				fmt.Fprintf(out, "✅ %s (removed: %s)\n", result.Repository.Path, result.PreviousPath)
			} else {
				fmt.Fprintf(out, "✅ %s\n", result.Repository.Path)
			}
		}
	}

	fmt.Fprintln(out, "\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	if summary.ConfigsRemoved == summary.RepositoriesWithConfig {
		fmt.Fprintf(out, "Summary: %d %s removed successfully\n",
			summary.ConfigsRemoved,
			pluralize("override", "overrides", summary.ConfigsRemoved))
	} else {
		fmt.Fprintf(out, "Summary: %d removed, %d failed\n",
			summary.ConfigsRemoved,
			summary.RepositoriesWithConfig-summary.ConfigsRemoved)
	}
	fmt.Fprintln(out, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	if failed := summary.RepositoriesWithConfig - summary.ConfigsRemoved; failed > 0 {
		return fmt.Errorf("failed to clean %d %s", failed, pluralize("repository", "repositories", failed))
	}
	return nil
}

func pluralize(singular, plural string, count int) string {
	if count == 1 {
		return singular
	}
	return plural
}
