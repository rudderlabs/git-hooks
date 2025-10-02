package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v2"
)

var Remove = &cli.Command{
	Name:  "remove",
	Usage: "removes a git hook",
	Subcommands: []*cli.Command{
		{
			Name:  "gitleaks",
			Usage: "remove the gitleaks pre-commit hook",
			Action: func(c *cli.Context) error {
				return removeGitLeaks()
			},
		},
	},
}

func removeGitLeaks() error {
	hooksDir := filepath.Join(os.Getenv("HOME"), ".git-hooks", "pre-commit.d")
	scriptPath := filepath.Join(hooksDir, "gitleaks")

	err := os.Remove(scriptPath)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Println("Gitleaks pre-commit hook not found. Nothing to remove.")
			return nil
		}
		return fmt.Errorf("removing gitleaks script: %w", err)
	}

	fmt.Printf("Gitleaks pre-commit hook removed from: %s\n", scriptPath)
	return nil
}
