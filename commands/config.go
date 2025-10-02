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
	Usage: "Configure git-hooks",
	Action: func(c *cli.Context) error {
		return configureGitHooks()
	},
}

var Implode = &cli.Command{
	Name:  "implode",
	Usage: "Revert git-hooks configuration",
	Action: func(c *cli.Context) error {
		return implodeGitHooks()
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

	gitHooksPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	fmt.Printf("Using git-hooks binary: %s\n", gitHooksPath)

	// Parse the embedded template
	tmpl, err := template.ParseFS(hookTemplate, "hook.sh")
	if err != nil {
		return fmt.Errorf("failed to parse hook template: %w", err)
	}

	// Create a script for each Git hook
	for _, hook := range gitHooks {
		if err := createHookScript(hooksDir, hook, tmpl, gitHooksPath); err != nil {
			return err
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

func implodeGitHooks() error {
	hooksDir := filepath.Join(os.Getenv("HOME"), ".git-hooks")

	// Count and collect names of files not created by the tool
	fileCount := 0
	var filesToDelete []string
	err := filepath.Walk(hooksDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			hookName := filepath.Base(path)
			if !contains(gitHooks, hookName) {
				fileCount++
				filesToDelete = append(filesToDelete, path)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to analyze git-hooks directory: %w", err)
	}

	// Ask for user confirmation
	if fileCount > 0 {
		fmt.Printf("This will delete %d file(s) in %s:\n", fileCount, hooksDir)
		for _, file := range filesToDelete {
			fmt.Printf("  - %s\n", filepath.Base(file))
		}
		fmt.Print("Are you sure you want to proceed? (y/N): ")
		var response string
		_, err = fmt.Scanln(&response)
		if err != nil {
			return fmt.Errorf("failed to read user input: %w", err)
		}

		if response != "y" && response != "Y" {
			fmt.Println("Operation cancelled.")
			return nil
		}
	} else {
		fmt.Println("No additional files found in the git-hooks directory.")
	}

	// Remove the git-hooks directory
	err = os.RemoveAll(hooksDir)
	if err != nil {
		return fmt.Errorf("failed to remove git-hooks directory: %w", err)
	}

	// Reset Git's core.hooksPath configuration
	cmd := exec.Command("git", "config", "--global", "--unset", "core.hooksPath")
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to unset Git hooks configuration: %w", err)
	}

	fmt.Println("Git hooks configuration has been reverted.")
	fmt.Printf("Removed directory: %s\n", hooksDir)
	fmt.Println("Reset Git's core.hooksPath configuration")
	return nil
}

func createHookScript(hooksDir, hook string, tmpl *template.Template, gitHooksPath string) error {
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

	return nil
}

// Helper function to check if a string is in a slice
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
