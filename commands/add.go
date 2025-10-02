package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	_ "embed"

	"github.com/urfave/cli/v2"
)

//go:embed hooks/gitleaks/pre-commit.sh
var gitLeaksScriptTemplate string

//go:embed hooks/gitleaks/commit-msg.sh
var gitLeaksCommitMsgTemplate string

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
	gitleaksPath, err := installLatestGitleaks()
	if err != nil {
		return fmt.Errorf("installing/updating gitleaks: %w", err)
	}

	hooksDir := filepath.Join(os.Getenv("HOME"), ".git-hooks", "pre-commit.d")

	// Create the pre-commit.d directory if it doesn't exist
	err = os.MkdirAll(hooksDir, 0755)
	if err != nil {
		return fmt.Errorf("creating pre-commit.d directory: %w", err)
	}

	// Create the gitleaks script using the template
	scriptPath := filepath.Join(hooksDir, "gitleaks")

	tmpl, err := template.New("gitleaks").Parse(gitLeaksScriptTemplate)
	if err != nil {
		return fmt.Errorf("parsing gitleaks script template: %w", err)
	}

	file, err := os.Create(scriptPath)
	if err != nil {
		return fmt.Errorf("creating gitleaks script file: %w", err)
	}
	defer file.Close()

	err = tmpl.Execute(file, map[string]string{"GitleaksPath": gitleaksPath})
	if err != nil {
		return fmt.Errorf("executing gitleaks script template: %w", err)
	}

	err = os.Chmod(scriptPath, 0755)
	if err != nil {
		return fmt.Errorf("setting permissions for gitleaks script: %w", err)
	}

	fmt.Printf("Gitleaks pre-commit hook installed at: %s\n", scriptPath)

	// Install commit-msg hook
	commitMsgDir := filepath.Join(os.Getenv("HOME"), ".git-hooks", "commit-msg.d")
	err = os.MkdirAll(commitMsgDir, 0755)
	if err != nil {
		return fmt.Errorf("creating commit-msg.d directory: %w", err)
	}

	commitMsgPath := filepath.Join(commitMsgDir, "gitleaks")

	tmpl2, err := template.New("gitleaks-commit-msg").Parse(gitLeaksCommitMsgTemplate)
	if err != nil {
		return fmt.Errorf("parsing gitleaks commit-msg template: %w", err)
	}

	file2, err := os.Create(commitMsgPath)
	if err != nil {
		return fmt.Errorf("creating gitleaks commit-msg file: %w", err)
	}
	defer file2.Close()

	err = tmpl2.Execute(file2, map[string]string{"GitleaksPath": gitleaksPath})
	if err != nil {
		return fmt.Errorf("executing gitleaks commit-msg template: %w", err)
	}

	err = os.Chmod(commitMsgPath, 0755)
	if err != nil {
		return fmt.Errorf("setting permissions for gitleaks commit-msg script: %w", err)
	}

	fmt.Printf("Gitleaks commit-msg hook installed at: %s\n", commitMsgPath)

	return nil
}

func installLatestGitleaks() (string, error) {
	// Check if Homebrew is available
	_, err := exec.LookPath("brew")
	if err == nil {
		// Homebrew is available, use it to install Gitleaks
		cmd := exec.Command("brew", "install", "gitleaks")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		fmt.Println("Installing/updating gitleaks using Homebrew...")
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("installing/updating gitleaks with Homebrew: %w", err)
		}
	} else {
		// Homebrew is not available, fall back to go install
		cmd := exec.Command("go", "install", "github.com/zricethezav/gitleaks/v8@latest")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		fmt.Println("Installing/updating gitleaks using go install...")
		if err := cmd.Run(); err != nil {
			return "", fmt.Errorf("installing/updating gitleaks with go install: %w", err)
		}
	}

	fmt.Println("Gitleaks installed/updated successfully.")

	// Find the full path of gitleaks
	gitleaksPath, err := exec.LookPath("gitleaks")
	if err != nil {
		return "", fmt.Errorf("finding gitleaks in PATH: %w", err)
	}

	return gitleaksPath, nil
}
