package commands

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"

	"github.com/urfave/cli/v2"
)

//go:embed hook.sh
var hookTemplate embed.FS

var Config = &cli.Command{
	Name:  "config",
	Usage: "",
	Action: func(c *cli.Context) error {
		return configureGitHooks()
	},
}

var gitHooks = []string{
	"applypatch-msg", "pre-applypatch", "post-applypatch",
	"pre-commit", "pre-merge-commit", "prepare-commit-msg", "commit-msg", "post-commit",
	"pre-rebase", "post-checkout", "post-merge", "pre-push", "pre-receive",
	"update", "proc-receive", "post-receive", "post-update", "reference-transaction",
	"push-to-checkout", "pre-auto-gc", "post-rewrite", "sendemail-validate",
	"fsmonitor-watchman", "p4-changelist", "p4-prepare-changelist", "p4-post-changelist", "p4-pre-submit",
	"post-index-change",
}

func configureGitHooks() error {
	hooksDir := filepath.Join(os.Getenv("HOME"), ".git-hooks")

	// Create the directory if it doesn't exist
	err := os.MkdirAll(hooksDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create hooks directory: %w", err)
	}

	gitHooksPath, err := exec.LookPath("git-hooks")
	if err != nil {
		return fmt.Errorf("git-hooks not found in PATH: %w", err)
	}

	// Parse the embedded template
	tmpl, err := template.ParseFS(hookTemplate, "hook.sh")
	if err != nil {
		return fmt.Errorf("failed to parse hook template: %w", err)
	}

	// Create a script for each Git hook
	for _, hook := range gitHooks {
		scriptPath := filepath.Join(hooksDir, hook)
		file, err := os.Create(scriptPath)
		if err != nil {
			return fmt.Errorf("failed to create script file for %s: %w", hook, err)
		}
		defer file.Close()

		err = tmpl.Execute(file, struct {
			GitHooksPath string
			HookName     string
		}{
			GitHooksPath: gitHooksPath,
			HookName:     hook,
		})
		if err != nil {
			return fmt.Errorf("failed to execute template for %s: %w", hook, err)
		}

		if err := file.Chmod(0755); err != nil {
			return fmt.Errorf("failed to set permissions for %s: %w", hook, err)
		}
	}

	// Configure Git to use the directory
	cmd := exec.Command("git", "config", "--global", "core.hooksPath", hooksDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to configure Git hooks: %w", err)
	}

	fmt.Printf("Git hooks configured to use directory: %s\n", hooksDir)
	fmt.Println("Scripts created for all Git hooks")
	return nil
}
