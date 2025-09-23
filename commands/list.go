package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/urfave/cli/v2"
)

var List = &cli.Command{
	Name:  "list",
	Usage: "list installed git hooks",
	Action: func(c *cli.Context) error {
		return listInstalledHooks()
	},
}

func listInstalledHooks() error {
	fmt.Println("=== Installed Git Hooks ===")
	
	// Check global hooks
	globalHooksDir := filepath.Join(os.Getenv("HOME"), ".git-hooks")
	fmt.Printf("\nGlobal hooks directory: %s\n", globalHooksDir)
	
	if _, err := os.Stat(globalHooksDir); os.IsNotExist(err) {
		fmt.Println("  Global hooks directory not found. Run 'git-hooks config' to set up.")
		return nil
	}
	
	// List all hook directories with scripts
	for _, hookName := range gitHooks {
		hookDir := filepath.Join(globalHooksDir, hookName+".d")
		if files, err := os.ReadDir(hookDir); err == nil && len(files) > 0 {
			fmt.Printf("  %s: %d script(s)\n", hookName, len(files))
			for _, file := range files {
				if !file.IsDir() {
					fmt.Printf("    - %s\n", file.Name())
				}
			}
		}
	}
	
	// Check current repository hooks
	fmt.Printf("\nLocal repository hooks:\n")
	
	// Check .git-hooks directory
	localHooksDir := ".git-hooks"
	if _, err := os.Stat(localHooksDir); err == nil {
		fmt.Printf("  Local .git-hooks directory found\n")
		for _, hookName := range gitHooks {
			hookDir := filepath.Join(localHooksDir, hookName+".d")
			if files, err := os.ReadDir(hookDir); err == nil && len(files) > 0 {
				fmt.Printf("    %s: %d script(s)\n", hookName, len(files))
				for _, file := range files {
					if !file.IsDir() {
						fmt.Printf("      - %s\n", file.Name())
					}
				}
			}
		}
	} else {
		fmt.Println("  No local .git-hooks directory found")
	}
	
	// Check Husky hooks (both old and new formats)
	huskyDir := ".husky"
	if _, err := os.Stat(huskyDir); err == nil {
		fmt.Printf("  Husky directory found\n")
		for _, hookName := range gitHooks {
			// Check new format (.husky/hookname)
			newHuskyPath := filepath.Join(huskyDir, hookName)
			// Check old format (.husky/_/hookname)
			oldHuskyPath := filepath.Join(huskyDir, "_", hookName)
			
			newExists := false
			oldExists := false
			
			if _, err := os.Stat(newHuskyPath); err == nil {
				newExists = true
			}
			if _, err := os.Stat(oldHuskyPath); err == nil {
				oldExists = true
			}
			
			if newExists && oldExists {
				fmt.Printf("    %s: husky hook found (both new and old format)\n", hookName)
			} else if newExists {
				fmt.Printf("    %s: husky hook found (new format)\n", hookName)
			} else if oldExists {
				fmt.Printf("    %s: husky hook found (old format)\n", hookName)
			}
		}
	} else {
		fmt.Println("  No Husky directory found")
	}
	
	// Check standard Git hooks
	gitDir := os.Getenv("GIT_DIR")
	if gitDir == "" {
		gitDir = ".git"
	}
	standardHooksDir := filepath.Join(gitDir, "hooks")
	if _, err := os.Stat(standardHooksDir); err == nil {
		fmt.Printf("  Standard Git hooks directory found\n")
		for _, hookName := range gitHooks {
			standardHookPath := filepath.Join(standardHooksDir, hookName)
			if info, err := os.Stat(standardHookPath); err == nil && (info.Mode()&0111) != 0 {
				fmt.Printf("    %s: standard hook found (executable)\n", hookName)
			}
		}
	}
	
	return nil
}
