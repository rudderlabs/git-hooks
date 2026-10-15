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
	"sync"
	"syscall"
	"time"

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
	var interrupted bool
	var intr chan os.Signal
	if procrun.InForegroundJob() {
		// Catch SIGINT so it does not kill us before the stage reports its exit.
		// Ignoring it instead would make every child inherit the ignore.
		intr = make(chan os.Signal, 1)
		signal.Notify(intr, os.Interrupt)
		defer signal.Stop(intr)
	} else {
		signals = append(signals, os.Interrupt)
	}
	ctx, stop := signal.NotifyContext(context.Background(), signals...)
	defer stop()
	afterStage := func(err error) error {
		if err != nil {
			return err
		}
		// Read the channel here, not in a goroutine, so a Ctrl-C that came in
		// during the stage is seen before the next stage starts.
		select {
		case <-intr:
			interrupted = true
		default:
		}
		if interrupted {
			return errors.New("interrupted")
		}
		return ctx.Err()
	}

	// An empty HOME would make the global path relative and silently skip gitleaks.
	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("finding global hooks: %w", err)
	}

	repoRoot, err := os.Getwd()
	if err != nil {
		return err
	}
	globalDir := filepath.Join(home, ".git-hooks", hookName+".d")
	repoDir := filepath.Join(".git-hooks", hookName+".d")
	// Only for counting: an earlier stage may run npm install, so husky is
	// resolved again right before its own stage.
	countPlan := husky.Resolve(repoRoot, hookName, args, os.Environ())

	// Nothing runs yet that could handle Ctrl-C, so it cancels the read.
	readCtx, stopRead := signal.NotifyContext(ctx, os.Interrupt)
	stdin, err := replayableStdin(readCtx, hookName, countStages(readCtx, hookName, globalDir, repoDir, countPlan))
	// Ctrl-C kills git, which closes stdin; EOF can win the race against the cancel.
	if err == nil && readCtx.Err() != nil {
		_ = stdin.Close()
		err = readCtx.Err()
	}
	stopRead()
	if err != nil {
		return err
	}
	defer stdin.Close()

	// 1. Global scripts (gitleaks) run uncapped and fail closed.
	global := func(argv, _ []string) error {
		if err := afterStage(nil); err != nil {
			return err
		}
		cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
		in, release, err := stdin.Open()
		if err != nil {
			return err
		}
		defer release()
		cmd.Stdin, cmd.Stdout, cmd.Stderr = in, os.Stdout, os.Stderr
		return afterStage(cmd.Run())
	}
	if err := executeScriptsInDir(globalDir, args, global); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	opt := procrun.DefaultOptions()
	opt.Dir = repoRoot
	opt.Stdout, opt.Stderr = os.Stdout, os.Stderr
	capped := func(argv, env []string) error {
		if err := afterStage(nil); err != nil {
			return err
		}
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
	if err := executeScriptsInDir(repoDir, args, capped); err != nil {
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
	return afterStage(executeStandardGitHook(ctx, hookName, args, capped))
}

type runner func(argv, env []string) error

// countStages counts the stages that will get the hook input. With none,
// git-hooks never reads stdin or touches disk: most reference-transaction
// calls, which git makes on every ref update, have no stage at all.
func countStages(ctx context.Context, hookName, globalDir, repoDir string, plan husky.Plan) int {
	if !bufferedHooks[hookName] {
		return 0
	}
	n := 0
	for _, dir := range []string{globalDir, repoDir} {
		entries, _ := os.ReadDir(dir)
		for _, e := range entries {
			if !e.IsDir() {
				n++
			}
		}
	}
	if plan.Action == husky.Run {
		n++
	}
	if path, ok, err := standardHookPath(ctx, hookName); err == nil && ok {
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode()&0o111 != 0 {
			n++
		}
	}
	return n
}

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
	spool string // a file holding the input; empty means share os.Stdin
}

// bufferedHooks get input from git that ends with EOF. A validator after a
// logging stage must still see every ref. proc-receive talks to git both ways
// and would deadlock, so it and unknown hooks keep stdin as is.
var bufferedHooks = map[string]bool{
	"pre-push":              true,
	"pre-receive":           true,
	"post-receive":          true,
	"post-rewrite":          true,
	"reference-transaction": true,
}

// maxHookInput bounds the spool. A mirror fetch of a million refs fits; past
// that the hook fails closed, because a validator must not see a cut list.
const maxHookInput = 512 << 20

// replayableStdin spools the input to a file, even for one stage. git then
// never blocks writing to a stage that left a child holding stdin, exec never
// waits on a feeding goroutine, and a child that reads late still gets every
// byte. If no directory takes the file, the hook fails rather than hand a
// stage an empty or cut input.
func replayableStdin(ctx context.Context, hookName string, stages int) (*stdinSource, error) {
	if !bufferedHooks[hookName] || stages == 0 {
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
	var data []byte
	select {
	case r := <-read:
		if r.err != nil {
			return nil, fmt.Errorf("reading %s input: %w", hookName, r.err)
		}
		if len(r.data) > maxHookInput {
			return nil, fmt.Errorf("%s input is larger than %d bytes", hookName, maxHookInput)
		}
		data = r.data
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	type spooled struct {
		path string
		err  error
	}
	// A write to a stalled filesystem must not outlast a cancel.
	var current spoolPath
	wrote := make(chan spooled, 1)
	go func() {
		path, err := writeSpool(ctx, data, &current)
		wrote <- spooled{path, err}
	}()
	select {
	case w := <-wrote:
		if w.err != nil {
			return nil, fmt.Errorf("buffering %s input: %w", hookName, w.err)
		}
		return &stdinSource{spool: w.path}, nil
	case <-ctx.Done():
		// Remove the file now: the process exits right after this return.
		// Unlinking is safe while the writer still holds it open.
		removeBounded(current.take())
		return nil, ctx.Err()
	}
}

// spoolPath shares the spool file name between the writer and a cancel.
type spoolPath struct {
	mu     sync.Mutex
	path   string
	closed bool
}

// set records the file being written. It reports false once the cancel has
// taken the path, so the writer removes a file it created too late.
func (p *spoolPath) set(path string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed {
		return false
	}
	p.path = path
	return true
}

// take returns the current file and stops any later set.
func (p *spoolPath) take() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.closed = true
	return p.path
}

// writeSpool tries the temp dir, /tmp, then the git dir, which is writable
// whenever git can update refs. A failed write (a full disk, a quota) moves
// on to the next directory too.
func writeSpool(ctx context.Context, data []byte, current *spoolPath) (string, error) {
	dirs := []func() string{
		os.TempDir,
		func() string { return "/tmp" },
		func() string {
			out, err := exec.CommandContext(ctx, "git", "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
			if err != nil {
				return ""
			}
			return strings.TrimSpace(string(out))
		},
	}
	var errs []error
	for _, dir := range dirs {
		// After a cancel the caller has returned; a new file would never be removed.
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		d := dir()
		if d == "" {
			continue
		}
		f, err := os.CreateTemp(d, "git-hooks-stdin-*")
		if err != nil {
			errs = append(errs, err)
			continue
		}
		if !current.set(f.Name()) {
			_ = f.Close()
			_ = os.Remove(f.Name())
			return "", ctx.Err()
		}
		_, werr := f.Write(data)
		cerr := f.Close()
		if err := errors.Join(werr, cerr); err != nil {
			removeBounded(f.Name())
			errs = append(errs, err)
			continue
		}
		return f.Name(), nil
	}
	return "", errors.Join(errs...)
}

// Open returns a file that reads the hook input from the start, and a release
// func to call after the stage.
func (s *stdinSource) Open() (*os.File, func(), error) {
	if s.spool == "" {
		return os.Stdin, func() {}, nil
	}
	f, err := os.Open(s.spool)
	if err != nil {
		return nil, nil, fmt.Errorf("replaying hook input: %w", err)
	}
	return f, func() { _ = f.Close() }, nil
}

// Close removes the spool. A child that still holds it open keeps reading.
func (s *stdinSource) Close() error {
	removeBounded(s.spool)
	return nil
}

// removeBounded unlinks path but gives up after a second: a stalled
// filesystem can block unlink too, and git must not hang on it.
func removeBounded(path string) {
	if path == "" {
		return
	}
	removed := make(chan struct{})
	go func() {
		_ = os.Remove(path)
		close(removed)
	}()
	select {
	case <-removed:
	case <-time.After(time.Second):
	}
}
