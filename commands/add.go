package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v2"
)

var Add = &cli.Command{
	Name:  "add",
	Usage: "adds a new git hook",
	Subcommands: []*cli.Command{
		{
			Name:  "gitleaks",
			Usage: "pre-commit hook to run gitleaks detect",
			Action: func(c *cli.Context) error {
				return addGitLeaks()
			},
		},
	},
}

func addGitLeaks() error {
	hooksDir := filepath.Join(os.Getenv("HOME"), ".git-hooks", "pre-commit.d")

	// Create the pre-commit.d directory if it doesn't exist
	err := os.MkdirAll(hooksDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create pre-commit.d directory: %w", err)
	}

	// Create the gitleaks script
	scriptPath := filepath.Join(hooksDir, "gitleaks")
	script := `#!/bin/bash
go run github.com/zricethezav/gitleaks/v8@v8.18.4 protect --staged
`

	err = os.WriteFile(scriptPath, []byte(script), 0755)
	if err != nil {
		return fmt.Errorf("failed to create gitleaks script: %w", err)
	}

	fmt.Printf("Gitleaks pre-commit hook installed at: %s\n", scriptPath)
	return nil
}
