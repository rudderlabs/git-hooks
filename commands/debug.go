package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/urfave/cli/v2"
)

var Debug = &cli.Command{
	Name:  "debug",
	Usage: "debug git hooks setup and execution",
	Subcommands: []*cli.Command{
		{
			Name:  "hook",
			Usage: "debug a specific hook execution",
			Action: func(c *cli.Context) error {
				if c.NArg() == 0 {
					return fmt.Errorf("hook name required")
				}
				return debugHook(c.Args().First())
			},
		},
		{
			Name:  "config",
			Usage: "debug git hooks configuration",
			Action: func(c *cli.Context) error {
				return debugConfig()
			},
		},
	},
}

func debugHook(hookName string) error {
	fmt.Printf("=== Debugging hook: %s ===\n\n", hookName)
	
	// 1. Check global scripts
	globalDir := filepath.Join(os.Getenv("HOME"), ".git-hooks", hookName+".d")
	fmt.Printf("1. Global hooks directory: %s\n", globalDir)
	if files, err := os.ReadDir(globalDir); err == nil {
		fmt.Printf("   Found %d files:\n", len(files))
		for _, file := range files {
			if !file.IsDir() {
				scriptPath := filepath.Join(globalDir, file.Name())
				info, _ := os.Stat(scriptPath)
				fmt.Printf("   - %s (mode: %v)\n", file.Name(), info.Mode())
			}
		}
	} else {
		fmt.Printf("   Error or not found: %v\n", err)
	}
	
	// 2. Check local scripts
	localDir := filepath.Join(".git-hooks", hookName+".d")
	fmt.Printf("\n2. Local hooks directory: %s\n", localDir)
	if files, err := os.ReadDir(localDir); err == nil {
		fmt.Printf("   Found %d files:\n", len(files))
		for _, file := range files {
			if !file.IsDir() {
				scriptPath := filepath.Join(localDir, file.Name())
				info, _ := os.Stat(scriptPath)
				fmt.Printf("   - %s (mode: %v)\n", file.Name(), info.Mode())
			}
		}
	} else {
		fmt.Printf("   Error or not found: %v\n", err)
	}
	
	// 3. Check Husky scripts (both formats)
	fmt.Printf("\n3. Husky hooks:\n")
	
	// Check new format (.husky/hookname)
	newHuskyPath := filepath.Join(".husky", hookName)
	fmt.Printf("   New format: %s\n", newHuskyPath)
	if info, err := os.Stat(newHuskyPath); err == nil {
		fmt.Printf("     Found: mode %v, size %d bytes\n", info.Mode(), info.Size())
		if info.Mode().IsRegular() && (info.Mode()&0111) != 0 {
			fmt.Printf("     Status: Executable ✓\n")
		} else {
			fmt.Printf("     Status: Not executable ✗\n")
		}
	} else {
		fmt.Printf("     Error or not found: %v\n", err)
	}
	
	// Check old format (.husky/_/hookname)
	oldHuskyPath := filepath.Join(".husky/_", hookName)
	fmt.Printf("   Old format: %s\n", oldHuskyPath)
	if info, err := os.Stat(oldHuskyPath); err == nil {
		fmt.Printf("     Found: mode %v, size %d bytes\n", info.Mode(), info.Size())
		if info.Mode().IsRegular() && (info.Mode()&0111) != 0 {
			fmt.Printf("     Status: Executable ✓\n")
		} else {
			fmt.Printf("     Status: Not executable ✗\n")
		}
	} else {
		fmt.Printf("     Error or not found: %v\n", err)
	}
	
	// 4. Check standard Git hook
	gitDir := os.Getenv("GIT_DIR")
	if gitDir == "" {
		gitDir = ".git"
	}
	standardPath := filepath.Join(gitDir, "hooks", hookName)
	fmt.Printf("\n4. Standard Git hook: %s\n", standardPath)
	if info, err := os.Stat(standardPath); err == nil {
		fmt.Printf("   Found: mode %v, size %d bytes\n", info.Mode(), info.Size())
		if info.Mode().IsRegular() && (info.Mode()&0111) != 0 {
			fmt.Printf("   Status: Executable ✓\n")
		} else {
			fmt.Printf("   Status: Not executable ✗\n")
		}
	} else {
		fmt.Printf("   Error or not found: %v\n", err)
	}
	
	return nil
}

func debugConfig() error {
	fmt.Println("=== Git Hooks Configuration Debug ===\n")
	
	// Check global configuration
	fmt.Println("1. Git configuration:")
	cmd := exec.Command("git", "config", "--global", "core.hooksPath")
	if output, err := cmd.Output(); err == nil {
		fmt.Printf("   core.hooksPath = %s\n", string(output))
	} else {
		fmt.Printf("   core.hooksPath not set or error: %v\n", err)
	}
	
	// Check git-hooks binary
	fmt.Println("\n2. git-hooks binary:")
	if gitHooksPath, err := exec.LookPath("git-hooks"); err == nil {
		fmt.Printf("   Found at: %s\n", gitHooksPath)
		if info, err := os.Stat(gitHooksPath); err == nil {
			fmt.Printf("   Mode: %v\n", info.Mode())
		}
	} else {
		fmt.Printf("   Not found in PATH: %v\n", err)
	}
	
	// Check global hooks directory
	globalDir := filepath.Join(os.Getenv("HOME"), ".git-hooks")
	fmt.Printf("\n3. Global hooks directory: %s\n", globalDir)
	if info, err := os.Stat(globalDir); err == nil {
		fmt.Printf("   Exists: mode %v\n", info.Mode())
		
		// Check hook scripts
		fmt.Println("   Hook scripts:")
		for _, hook := range gitHooks {
			hookPath := filepath.Join(globalDir, hook)
			if info, err := os.Stat(hookPath); err == nil {
				fmt.Printf("   - %s: mode %v\n", hook, info.Mode())
			}
		}
	} else {
		fmt.Printf("   Error or not found: %v\n", err)
	}
	
	// Check current repository
	fmt.Println("\n4. Current repository:")
	if gitDir := os.Getenv("GIT_DIR"); gitDir != "" {
		fmt.Printf("   GIT_DIR = %s\n", gitDir)
	} else {
		fmt.Println("   GIT_DIR not set, using .git")
	}
	
	if _, err := os.Stat(".git"); err == nil {
		fmt.Println("   Git repository detected ✓")
	} else {
		fmt.Printf("   Not a git repository or error: %v\n", err)
	}
	
	return nil
}
