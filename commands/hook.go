package commands

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/rudderlabs/git-hooks/internal/husky"
	"github.com/rudderlabs/git-hooks/internal/procrun"
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
	// Skip binary name (os.Args[0]), "hook" subcommand (os.Args[1]), and hook name (os.Args[2])
	var args []string
	if len(os.Args) > 3 {
		args = os.Args[3:]
	}

	// Repo stages run in their own process group, so Ctrl-C must reach them through us.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 1. Global scripts (gitleaks) run uncapped and fail closed.
	err := executeScriptsInDir(filepath.Join(os.Getenv("HOME"), ".git-hooks", hookName+".d"), args, nil)
	if err != nil {
		return err
	}

	repoRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	opt := procrun.DefaultOptions()
	opt.Dir = repoRoot
	opt.Stdin, opt.Stdout, opt.Stderr = os.Stdin, os.Stdout, os.Stderr
	opt.Env = os.Environ()
	capped := func(argv []string, env []string) error {
		o := opt
		if env != nil {
			o.Env = env
		}
		return procrun.Run(ctx, argv, o)
	}

	// 2. Repo scripts.
	err = executeScriptsInDir(filepath.Join(".git-hooks", hookName+".d"), args, capped)
	if err != nil {
		return err
	}

	// 3. Husky: exactly one entry point per hook.
	plan := husky.Resolve(repoRoot, hookName, args, os.Environ())
	switch plan.Action {
	case husky.Skip:
		fmt.Fprintf(os.Stderr, "git-hooks: WARNING: husky %s skipped: %s\n", hookName, plan.Reason)
	case husky.Run:
		if err := capped(plan.Argv, plan.Env); err != nil {
			return fmt.Errorf("husky %s: %w", hookName, err)
		}
	}

	// 4. Standard Git hook for backwards compatibility.
	return executeStandardGitHook(ctx, hookName, args, capped)
}

type runner func(argv []string, env []string) error

func executeScriptsInDir(dir string, args []string, run runner) error {
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
			if err := executeScript(scriptPath, args, run); err != nil {
				return err
			}
		}
	}

	return nil
}

func executeScript(path string, args []string, run runner) error {
	if run != nil {
		abs, err := filepath.Abs(path)
		if err != nil {
			return err
		}
		return run(append([]string{abs}, args...), nil)
	}
	cmd := exec.Command(path, args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	return cmd.Run()
}

func executeStandardGitHook(ctx context.Context, hookName string, args []string, run runner) error {
	path, ok := standardHookPath(ctx, hookName)
	if !ok {
		return nil
	}
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("checking %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil
	}
	if info.Mode()&0o111 == 0 {
		// Git itself ignores such hooks; say so, because people expect it to run.
		fmt.Fprintf(os.Stderr, "git-hooks: WARNING: %s is not executable and was skipped\n", path)
		return nil
	}
	return executeScript(path, args, run)
}

// standardHookPath returns <common git dir>/hooks/<hook>. It must not use
// `git rev-parse --git-path hooks/...`: that follows core.hooksPath, which points
// back at the global git-hooks wrapper and makes the hook call itself forever.
func standardHookPath(ctx context.Context, hookName string) (string, bool) {
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		return "", false // Not inside a git repository, so there is no standard hook.
	}
	return filepath.Join(strings.TrimSpace(string(out)), "hooks", hookName), true
}
