package commands

import (
	"context"
	"fmt"

	cleangit "github.com/lvrach/git-hooks/internal/clean-local-git"
	"github.com/urfave/cli/v2"
)

var CleanLocalHooks = &cli.Command{
	Name:      "clean-local-hooks",
	Usage:     "Remove hooksPath configuration from Git repositories",
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
			Name:  "force",
			Usage: "Skip confirmation prompt",
		},
	},
	Action: cleanLocalHooksAction,
}

func cleanLocalHooksAction(c *cli.Context) error {
	// Get starting path
	path := "."
	if c.NArg() > 0 {
		path = c.Args().First()
	}

	maxDepth := c.Int("max-depth")
	verbose := c.Bool("verbose")
	force := c.Bool("force")

	ctx := context.Background()

	// Scan for repositories with custom hooksPath
	repos, err := cleangit.Scan(ctx, path, maxDepth)
	if err != nil {
		return fmt.Errorf("scanning repositories: %w", err)
	}

	if verbose {
		fmt.Printf("Found %d repositories with custom hooksPath\n\n", len(repos))
	}

	if len(repos) == 0 {
		fmt.Println("No repositories found with custom hooksPath configuration.")
		return nil
	}

	// Show repos if verbose
	if verbose {
		for _, repo := range repos {
			fmt.Printf("📁 %s\n", repo.Path)
			fmt.Printf("   hooksPath: %s\n", repo.CustomHooksPath)
			fmt.Println()
		}
	}

	// Show confirmation prompt if not force
	if !force {
		fmt.Printf("This will remove hooksPath configuration from %d repositories:\n", len(repos))
		for _, repo := range repos {
			fmt.Printf("  - %s\n", repo.Path)
			fmt.Printf("    hooksPath: %s\n", repo.CustomHooksPath)
		}
		fmt.Print("\nContinue? (y/N): ")
		var response string
		_, err := fmt.Scanln(&response)
		if err != nil {
			return fmt.Errorf("reading user input: %w", err)
		}

		if response != "y" && response != "Y" {
			fmt.Println("Cancelled.")
			return nil
		}
		fmt.Println()
	}

	// Clean repositories
	summary := cleangit.Clean(ctx, repos)

	// Show individual results if verbose
	if verbose {
		for _, result := range summary.Results {
			fmt.Printf("📁 %s\n", result.Repository.Path)
			if result.Error != nil {
				fmt.Printf("   ❌ Error: %v\n", result.Error)
			} else {
				fmt.Printf("   ✅ Removed: %s\n", result.PreviousPath)
			}
			fmt.Println()
		}
	}

	// Show summary
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Printf("Summary:\n")
	fmt.Printf("  Repositories with custom hooksPath: %d\n", summary.RepositoriesWithConfig)
	fmt.Printf("  Removed: %d\n", summary.ConfigsRemoved)
	if summary.ConfigsRemoved < summary.RepositoriesWithConfig {
		fmt.Printf("  Failed: %d\n", summary.RepositoriesWithConfig-summary.ConfigsRemoved)
	}
	fmt.Println("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")

	return nil
}
