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
	"sync/atomic"
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

	// Without a terminal, repo stages run in their own process group, so a
	// signal must reach them through us. In a terminal, Ctrl-C reaches every
	// stage directly; cancelling as well would cut short its own SIGINT cleanup.
	// We still stop after the current stage, because git has died by then.
	signals := []os.Signal{syscall.SIGTERM, syscall.SIGHUP}
	var interrupted atomic.Bool
	if procrun.InForegroundJob() {
		// Catch SIGINT so it does not kill us before the stage reports its exit.
		// Ignoring it instead would make every child inherit the ignore.
		intr := make(chan os.Signal, 1)
		signal.Notify(intr, os.Interrupt)
		defer signal.Stop(intr)
		go func() {
			for range intr {
				interrupted.Store(true)
			}
		}()
	} else {
		signals = append(signals, os.Interrupt)
	}
	ctx, stop := signal.NotifyContext(context.Background(), signals...)
	defer stop()
	afterStage := func(err error) error {
		if err != nil {
			return err
		}
		if interrupted.Load() {
			return errors.New("interrupted")
		}
		return ctx.Err()
	}

	// An empty HOME would make the global path relative and silently skip gitleaks.
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("finding global hooks: %w", err)
	}

	// Nothing runs yet that could handle Ctrl-C, so it cancels the read.
	readCtx, stopRead := signal.NotifyContext(ctx, os.Interrupt)
	stdin, err := replayableStdin(readCtx, hookName)
	stopRead()
	if err != nil {
		return err
	}

	// 1. Global scripts (gitleaks) run uncapped and fail closed.
	global := func(argv []string, _ []string) error {
		cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
		in, release, err := stdin.Open()
		if err != nil {
			return err
		}
		defer release()
		cmd.Stdin, cmd.Stdout, cmd.Stderr = in, os.Stdout, os.Stderr
		return afterStage(cmd.Run())
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
		return afterStage(procrun.Run(ctx, argv, o))
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
// otherwise consume it all, so a later validator would see no refs.
type stdinSource struct {
	data     []byte
	buffered bool
}

// bufferedHooks get a short input from git that ends with EOF. proc-receive
// talks to git both ways and would deadlock. reference-transaction can carry
// every ref of a mirror fetch. Those and unknown hooks keep stdin as is.
var bufferedHooks = map[string]bool{
	"pre-push":     true,
	"pre-receive":  true,
	"post-receive": true,
	"post-rewrite": true,
}

// maxHookInput bounds the buffer. Ref lists are small; a larger input means
// something other than git is feeding the hook.
const maxHookInput = 64 << 20

func replayableStdin(ctx context.Context, hookName string) (*stdinSource, error) {
	if !bufferedHooks[hookName] {
		return &stdinSource{}, nil
	}
	info, err := os.Stdin.Stat()
	if err != nil {
		return nil, fmt.Errorf("checking %s input: %w", hookName, err)
	}
	if info.Mode()&os.ModeCharDevice != 0 {
		return &stdinSource{}, nil
	}
	type result struct {
		data []byte
		err  error
	}
	read := make(chan result, 1)
	go func() {
		data, err := io.ReadAll(io.LimitReader(os.Stdin, maxHookInput+1))
		read <- result{data, err}
	}()
	select {
	case r := <-read:
		if r.err != nil {
			return nil, fmt.Errorf("reading %s input: %w", hookName, r.err)
		}
		if len(r.data) > maxHookInput {
			return nil, fmt.Errorf("%s input is larger than %d bytes", hookName, maxHookInput)
		}
		return &stdinSource{data: r.data, buffered: true}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Open returns a file that reads the hook input from the start, and a release
// func to call after the stage. Buffered input goes through a pipe we feed
// ourselves: a child the stage leaves behind may hold the read end without
// reading, and exec must not wait for that.
func (s *stdinSource) Open() (*os.File, func(), error) {
	if !s.buffered {
		return os.Stdin, func() {}, nil
	}
	r, w, err := os.Pipe()
	if err != nil {
		return nil, nil, fmt.Errorf("replaying hook input: %w", err)
	}
	go func() {
		_, _ = w.Write(s.data)
		_ = w.Close()
	}()
	return r, func() {
		_ = r.Close()
		// Unblocks the writer when the stage never read its input.
		_ = w.Close()
	}, nil
}
