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

	// 3. Execute Husky scripts
	err = executeHuskyGitHook(filepath.Join(".husky/_", hookName))
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
	cmd := exec.Command(path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func executeStandardGitHook(path string) error {
	if _, err := os.Stat(path); err == nil {
		return executeScript(path)
	}
	return nil // Hook doesn't exist, which is fine
}

func executeHuskyGitHook(path string) error {
	if _, err := os.Stat(path); err == nil {
		return executeScript(path)
	}
	return nil // Hook doesn't exist, which is fine
}
