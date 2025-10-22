package commands

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"

	cleangit "github.com/rudderlabs/git-hooks/internal/clean-local-git"
	"github.com/urfave/cli/v2"
)

var ScanLocal = &cli.Command{
	Name:      "scan-local",
	Usage:     "Scan for repositories with local hooksPath overrides",
	ArgsUsage: "[PATH]",
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
	},
	Action: scanLocalAction,
}

func scanLocalAction(c *cli.Context) error {
	// Get starting path
	path := "."
	if c.NArg() > 0 {
		path = c.Args().First()
	}

	maxDepth := c.Int("max-depth")
	verbose := c.Bool("verbose")
	autoFix := c.Bool("auto-fix")

	// Create context that can be cancelled with CTRL+C
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// Scan for repositories with custom hooksPath
	fmt.Println("🔍 Scanning for local hook overrides...")
	repos, err := cleangit.Scan(ctx, path, maxDepth)
	if err != nil {
		return fmt.Errorf("scanning repositories: %w", err)
	}

	if len(repos) == 0 {
		fmt.Println("\n✅ No repositories found with local hooksPath overrides.")
		return nil
	}

	// Show found repositories
	fmt.Printf("\nFound %d %s with local hook overrides:\n\n",
		len(repos),
		pluralize("repository", "repositories", len(repos)))

	for _, repo := range repos {
		if verbose {
			fmt.Printf("  📁 %s\n", repo.Path)
			fmt.Printf("     Override: core.hooksPath = %s\n", repo.CustomHooksPath)
		} else {
			fmt.Printf("  • %s (core.hooksPath = %s)\n", repo.Path, repo.CustomHooksPath)
		}
	}

	// If not auto-fix mode, just show summary and exit
	if !autoFix {
		fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
		fmt.Printf("Summary: %d %s with local overrides\n",
			len(repos),
			pluralize("repository", "repositories", len(repos)))
		fmt.Println("\n💡 Tip: Use --auto-fix to remove these overrides")
		return nil
	}

	// Auto-fix mode: show confirmation prompt
	fmt.Print("\nRemove these local overrides? (y/N): ")

	// Check for context cancellation before reading input
	select {
	case <-ctx.Done():
		fmt.Println("\nOperation cancelled.")
		return nil
	default:
	}

	// Read user input with better error handling
	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return fmt.Errorf("reading user input: %w", err)
	}

	response = strings.TrimSpace(response)
	if response != "y" && response != "Y" {
		fmt.Println("Cancelled.")
		return nil
	}

	// Remove overrides
	fmt.Println("\n🔧 Removing overrides...")
	summary := cleangit.Clean(ctx, repos)

	// Show individual results
	fmt.Println()
	for _, result := range summary.Results {
		if result.Error != nil {
			fmt.Printf("❌ %s: %v\n", result.Repository.Path, result.Error)
		} else {
			if verbose {
				fmt.Printf("✅ %s (removed: %s)\n", result.Repository.Path, result.PreviousPath)
			} else {
				fmt.Printf("✅ %s\n", result.Repository.Path)
			}
		}
	}

	// Show summary
	fmt.Println("\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	if summary.ConfigsRemoved == summary.RepositoriesWithConfig {
		fmt.Printf("Summary: %d %s removed successfully\n",
			summary.ConfigsRemoved,
			pluralize("override", "overrides", summary.ConfigsRemoved))
	} else {
		fmt.Printf("Summary: %d removed, %d failed\n",
			summary.ConfigsRemoved,
			summary.RepositoriesWithConfig-summary.ConfigsRemoved)
	}
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return nil
}

func pluralize(singular, plural string, count int) string {
	if count == 1 {
		return singular
	}
	return plural
}
