package commands

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/urfave/cli/v2"
)

var Hooks = &cli.Command{
	Name:  "hook",
	Usage: "",
	Action: func(c *cli.Context) error {
		if c.NArg() == 0 {
			return cli.ShowAppHelp(c)
		}

		hookName := c.Args().First()
		return executeHook(hookName)
	},
}

func executeHook(hookName string) error {
	// 1. Execute global scripts
	err := executeScriptsInDir(filepath.Join(os.Getenv("HOME"), ".git-hooks", hookName+".d"))
	if err != nil {
		return err
	}

	// 2. Execute local scripts
	err = executeScriptsInDir(filepath.Join(".git-hooks", hookName+".d"))
	if err != nil {
		return err
	}

	// 3. Execute Husky scripts (try both modern and legacy formats)
	err = executeHuskyGitHooks(hookName)
	if err != nil {
		return err
	}

	// 4. Execute standard Git hook for backwards compatibility
	gitDir := os.Getenv("GIT_DIR")
	if gitDir == "" {
		gitDir = ".git"
	}
	return executeStandardGitHook(filepath.Join(gitDir, "hooks", hookName))
}

func executeScriptsInDir(dir string) error {
	files, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // Directory doesn't exist, which is fine
		}
		return err
	}

	for _, file := range files {
		if !file.IsDir() {
			scriptPath := filepath.Join(dir, file.Name())
			if err := executeScript(scriptPath); err != nil {
				return err
			}
		}
	}

	return nil
}

func executeScript(path string) error {
	// Skip binary name (os.Args[0]), "hook" subcommand (os.Args[1]), and hook name (os.Args[2])
	// Pass only the actual hook arguments starting from os.Args[3]
	var args []string
	if len(os.Args) > 3 {
		args = os.Args[3:]
	}
	cmd := exec.Command(path, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	return cmd.Run()
}

func executeStandardGitHook(path string) error {
	if info, err := os.Stat(path); err == nil {
		// Check if it's a regular file and executable
		if info.Mode().IsRegular() && (info.Mode()&0111) != 0 {
			return executeScript(path)
		}
		// If it exists but is not executable, skip silently
		return nil
	}
	return nil // Hook doesn't exist, which is fine
}

func executeHuskyGitHooks(hookName string) error {
	// Try modern Husky format first (.husky/hookname)
	modernPath := filepath.Join(".husky", hookName)
	if err := executeHuskyGitHook(modernPath); err != nil {
		return err
	}

	// Try legacy Husky format (.husky/_/hookname)
	legacyPath := filepath.Join(".husky/_", hookName)
	if err := executeHuskyGitHook(legacyPath); err != nil {
		return err
	}

	return nil
}

func executeHuskyGitHook(path string) error {
	if info, err := os.Stat(path); err == nil {
		// Check if it's a regular file and executable
		if info.Mode().IsRegular() && (info.Mode()&0111) != 0 {
			return executeScript(path)
		}
		// If it exists but is not executable, skip silently
		return nil
	}
	return nil // Hook doesn't exist, which is fine
}
