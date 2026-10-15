package commands

import (
	"context"
	"errors"
	"fmt"
	"io"
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

// runningStandardHookEnv names the standard hook a git-hooks run is executing.
// A nested run that resolves to the same file skips it instead of recursing.
const runningStandardHookEnv = "GIT_HOOKS_RUNNING_STANDARD_HOOK"

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

	// An empty HOME would make the global path relative and silently skip gitleaks.
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("finding global hooks: %w", err)
	}

	stdin, err := replayableStdin(ctx, hookName)
	if err != nil {
		return err
	}
	defer stdin.Close()

	// 1. Global scripts (gitleaks) run uncapped and fail closed.
	global := func(argv []string, _ []string) error {
		in, release, err := stdin.Open()
		if err != nil {
			return err
		}
		defer release()
		cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = in, os.Stdout, os.Stderr
		return cmd.Run()
	}
	if err := executeScriptsInDir(filepath.Join(home, ".git-hooks", hookName+".d"), args, global); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	repoRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	opt := procrun.DefaultOptions()
	opt.Dir = repoRoot
	opt.Stdout, opt.Stderr = os.Stdout, os.Stderr
	capped := func(argv []string, env []string) error {
		o := opt
		o.Env = os.Environ()
		if env != nil {
			o.Env = env
		}
		in, release, err := stdin.Open()
		if err != nil {
			return err
		}
		defer release()
		o.Stdin = in
		if err := procrun.Run(ctx, argv, o); err != nil {
			return err
		}
		return ctx.Err()
	}

	// 2. Repo scripts.
	if err := executeScriptsInDir(filepath.Join(".git-hooks", hookName+".d"), args, capped); err != nil {
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
			scriptPath, err := filepath.Abs(filepath.Join(dir, file.Name()))
			if err != nil {
				return err
			}
			if err := run(append([]string{scriptPath}, args...), nil); err != nil {
				return err
			}
		}
	}

	return nil
}

func executeStandardGitHook(ctx context.Context, hookName string, args []string, run runner) error {
	path, ok, err := standardHookPath(ctx, hookName)
	if err != nil || !ok {
		return err
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
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("resolving %s: %w", path, err)
	}
	if os.Getenv(runningStandardHookEnv) == resolved {
		// A .git/hooks file that links to the global wrapper would call us forever.
		fmt.Fprintf(os.Stderr, "git-hooks: WARNING: %s calls git-hooks again and was skipped\n", path)
		return nil
	}
	return run(append([]string{path}, args...), append(os.Environ(), runningStandardHookEnv+"="+resolved))
}

// standardHookPath returns <common git dir>/hooks/<hook>. It must not use
// `git rev-parse --git-path hooks/...`: that follows core.hooksPath, which points
// back at the global git-hooks wrapper and makes the hook call itself forever.
func standardHookPath(ctx context.Context, hookName string) (string, bool, error) {
	out, err := exec.CommandContext(ctx, "git", "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", false, ctxErr
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return "", false, nil // Not inside a git repository, so there is no standard hook.
	}
	if err != nil {
		return "", false, fmt.Errorf("finding the git directory: %w", err)
	}
	return filepath.Join(strings.TrimSpace(string(out)), "hooks", hookName), true, nil
}

// stdinSource gives every stage its own reader over the same hook input.
// pre-push and post-rewrite pass data on stdin, and the first stage would
// otherwise consume it all.
type stdinSource struct {
	file  *os.File // the terminal or /dev/null, shared as is
	spool string   // a temp file holding piped input
}

// spooledHooks get input from git that ends with EOF. Other hooks keep their
// stdin as is: proc-receive talks to git both ways and would deadlock, and
// reference-transaction runs on every ref update, too often to spool.
var spooledHooks = map[string]bool{
	"pre-push":     true,
	"pre-receive":  true,
	"post-receive": true,
	"post-rewrite": true,
}

func replayableStdin(ctx context.Context, hookName string) (*stdinSource, error) {
	info, err := os.Stdin.Stat()
	if !spooledHooks[hookName] || err != nil || info.Mode()&os.ModeCharDevice != 0 {
		return &stdinSource{file: os.Stdin}, nil
	}
	f, err := os.CreateTemp("", "git-hooks-stdin-*")
	if err != nil {
		// Only later stages miss the input; blocking every push is worse.
		fmt.Fprintf(os.Stderr, "git-hooks: WARNING: cannot buffer %s input, only the first stage sees it: %v\n", hookName, err)
		return &stdinSource{file: os.Stdin}, nil
	}
	defer f.Close()
	copied := make(chan error, 1)
	go func() {
		_, err := io.Copy(f, os.Stdin)
		copied <- err
	}()
	select {
	case err = <-copied:
	case <-ctx.Done():
		err = ctx.Err()
	}
	if err != nil {
		_ = os.Remove(f.Name())
		return nil, fmt.Errorf("buffering hook input: %w", err)
	}
	return &stdinSource{spool: f.Name()}, nil
}

// Open returns a reader positioned at the start of the hook input and a
// release func. The shared terminal is never closed: it is our own stdin.
func (s *stdinSource) Open() (*os.File, func(), error) {
	if s.spool == "" {
		return s.file, func() {}, nil
	}
	f, err := os.Open(s.spool)
	if err != nil {
		return nil, nil, err
	}
	return f, func() { _ = f.Close() }, nil
}

func (s *stdinSource) Close() error {
	if s.spool == "" {
		return nil
	}
	return os.Remove(s.spool)
}
