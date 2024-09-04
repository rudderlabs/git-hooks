package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	_ "embed"

	"github.com/urfave/cli/v2"
)

//go:embed hooks/gitleaks.sh
var gitLeaksScript []byte

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
	// Ensure gitleaks is installed and up-to-date
	if err := installLatestGitleaks(); err != nil {
		return fmt.Errorf("failed to install/update gitleaks: %w", err)
	}

	hooksDir := filepath.Join(os.Getenv("HOME"), ".git-hooks", "pre-commit.d")

	// Create the pre-commit.d directory if it doesn't exist
	err := os.MkdirAll(hooksDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create pre-commit.d directory: %w", err)
	}

	// Create the gitleaks script
	scriptPath := filepath.Join(hooksDir, "gitleaks")

	err = os.WriteFile(scriptPath, gitLeaksScript, 0755)
	if err != nil {
		return fmt.Errorf("failed to create gitleaks script: %w", err)
	}

	fmt.Printf("Gitleaks pre-commit hook installed at: %s\n", scriptPath)
	return nil
}

func installLatestGitleaks() error {
	cmd := exec.Command("go", "install", "github.com/zricethezav/gitleaks/v8@latest")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Println("Installing/updating gitleaks...")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to install/update gitleaks: %w", err)
	}

	fmt.Println("Gitleaks installed/updated successfully.")
	return nil
}
