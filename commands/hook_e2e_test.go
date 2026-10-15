//go:build darwin || linux

package commands_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

var binary string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "git-hooks-e2e-*")
	if err != nil {
		panic(err)
	}
	binary = filepath.Join(dir, "git-hooks")
	build := exec.Command("go", "build", "-o", binary, "..")
	build.Stdout, build.Stderr = os.Stdout, os.Stderr
	if err := build.Run(); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

type fixture struct {
	t    *testing.T
	home string
	repo string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	f := &fixture{t: t, home: t.TempDir(), repo: t.TempDir()}
	cmd := exec.Command("git", "init", "-q", f.repo)
	cmd.Env = f.env()
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	return f
}

func (f *fixture) env() []string {
	return []string{
		"HOME=" + f.home,
		"PATH=" + os.Getenv("PATH"),
		"GIT_CONFIG_GLOBAL=" + os.DevNull,
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_HOOKS_CPUS=0",
	}
}

func (f *fixture) write(path, content string) {
	f.t.Helper()
	require.NoError(f.t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(f.t, os.WriteFile(path, []byte(content), 0o755))
}

func (f *fixture) hook(ctx context.Context, name string, env []string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, binary, append([]string{"hook", name}, args...)...)
	cmd.Dir = f.repo
	cmd.Env = env
	return cmd
}

func TestHookReplaysStdinToEveryStage(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.write(filepath.Join(f.home, ".git-hooks", "pre-push.d", "global"), "#!/bin/sh\ncat > \"$PWD/global.in\"\n")
	f.write(filepath.Join(f.repo, ".git", "hooks", "pre-push"), "#!/bin/sh\ncat > \"$PWD/standard.in\"\n")
	input := "refs/heads/main 1111 refs/heads/main 2222\n"

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := f.hook(ctx, "pre-push", f.env(), "origin", "git@example.com:o/r.git")
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	for _, name := range []string{"global.in", "standard.in"} {
		got, err := os.ReadFile(filepath.Join(f.repo, name))
		require.NoError(t, err)
		require.Equal(t, input, string(got), name)
	}
}

func TestHookFailsClosedOnSIGTERM(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.write(filepath.Join(f.home, ".git-hooks", "pre-commit.d", "slow"), "#!/bin/sh\nsleep 2\n")
	f.write(filepath.Join(f.repo, ".git", "hooks", "pre-commit"), "#!/bin/sh\nexit 1\n")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := f.hook(ctx, "pre-commit", f.env())
	require.NoError(t, cmd.Start())
	time.Sleep(300 * time.Millisecond)
	require.NoError(t, cmd.Process.Signal(syscall.SIGTERM))
	err := cmd.Wait()
	require.Error(t, err, "a terminated hook must not let the commit through")
}

func TestHookFailsClosedWithoutHome(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.write(filepath.Join(f.home, ".git-hooks", "pre-commit.d", "gitleaks"), "#!/bin/sh\nexit 1\n")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	env := []string{"PATH=" + os.Getenv("PATH"), "GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_NOSYSTEM=1"}
	out, err := f.hook(ctx, "pre-commit", env).CombinedOutput()
	require.Error(t, err, string(out))
}

func TestHookStandardHookLinkedToWrapperDoesNotRecurse(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	counter := filepath.Join(f.repo, "global.count")
	f.write(filepath.Join(f.home, ".git-hooks", "pre-commit.d", "count"), "#!/bin/sh\necho x >> "+counter+"\n")
	wrapper := filepath.Join(f.home, ".git-hooks", "pre-commit")
	f.write(wrapper, "#!/bin/sh\n\""+binary+"\" hook pre-commit \"$@\"\n")
	require.NoError(t, os.MkdirAll(filepath.Join(f.repo, ".git", "hooks"), 0o755))
	require.NoError(t, os.Symlink(wrapper, filepath.Join(f.repo, ".git", "hooks", "pre-commit")))

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := f.hook(ctx, "pre-commit", f.env()).CombinedOutput()
	require.NoError(t, err, string(out))
	require.NoError(t, ctx.Err(), "the hook must terminate on its own")
	got, err := os.ReadFile(counter)
	require.NoError(t, err)
	require.Equal(t, 2, strings.Count(string(got), "x"), "outer run plus one nested run, then stop")
	require.Contains(t, string(out), "calls git-hooks again and was skipped")
}

func TestHookKeepsStdinStreamingForProcReceive(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	f.write(filepath.Join(f.repo, ".git", "hooks", "proc-receive"), "#!/bin/sh\nread -r line\necho \"got:$line\" > \"$PWD/proc.out\"\n")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := f.hook(ctx, "proc-receive", f.env())
	in, err := cmd.StdinPipe()
	require.NoError(t, err)
	require.NoError(t, cmd.Start())
	// git keeps stdin open and waits for the hook's answer; never close it here.
	_, err = in.Write([]byte("version=1\n"))
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("proc-receive deadlocked waiting for stdin EOF")
	}
	got, err := os.ReadFile(filepath.Join(f.repo, "proc.out"))
	require.NoError(t, err)
	require.Equal(t, "got:version=1\n", string(got))
	_ = in.Close()
}

func TestHookReplaysStdinWithoutTempDir(t *testing.T) {
	t.Parallel()
	for _, hook := range []string{"pre-push", "post-rewrite", "reference-transaction"} {
		t.Run(hook, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			// The global stage reads everything first, as a gitleaks wrapper may.
			f.write(filepath.Join(f.home, ".git-hooks", hook+".d", "global"), "#!/bin/sh\ncat > /dev/null\n")
			f.write(filepath.Join(f.repo, ".git", "hooks", hook), "#!/bin/sh\ncat > \"$PWD/standard.in\"\n")
			input := "0000 1111 refs/heads/main\n"

			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			cmd := f.hook(ctx, hook, append(f.env(), "TMPDIR=/nonexistent-git-hooks-tmp"), "prepared")
			cmd.Stdin = strings.NewReader(input)
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, string(out))
			got, err := os.ReadFile(filepath.Join(f.repo, "standard.in"))
			require.NoError(t, err)
			require.Equal(t, input, string(got), "a later validator must see the same input")
		})
	}
}

func TestHookDoesNotWaitForChildHoldingStdin(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	pidFile := filepath.Join(f.repo, "child.pid")
	// The child inherits stdin, never reads it, and outlives the hook.
	f.write(filepath.Join(f.home, ".git-hooks", "pre-push.d", "leaves-child"), "#!/bin/sh\nsleep 60 <&0 >/dev/null 2>&1 &\necho $! > "+pidFile+"\nexit 0\n")
	// A second stage makes git-hooks replay the input; one stage reads git's stdin directly.
	f.write(filepath.Join(f.repo, ".git", "hooks", "pre-push"), "#!/bin/sh\ncat > /dev/null\n")
	input := strings.Repeat("0000000000000000000000000000000000000000 1111111111111111111111111111111111111111 refs/heads/main\n", 4000)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := f.hook(ctx, "pre-push", f.env(), "origin", "url")
	cmd.Stdin = strings.NewReader(input)
	start := time.Now()
	out, err := cmd.CombinedOutput()
	t.Cleanup(func() {
		if b, err := os.ReadFile(pidFile); err == nil {
			if pid, err := strconv.Atoi(strings.TrimSpace(string(b))); err == nil {
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	})
	require.NoError(t, err, string(out))
	require.Less(t, time.Since(start), 20*time.Second, "the hook must not wait for the child that holds stdin")
}

func TestHookBackgroundChildReadsAllReplayedInput(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	out := filepath.Join(f.repo, "bg.count")
	// The child reads after the stage has exited.
	f.write(filepath.Join(f.home, ".git-hooks", "pre-push.d", "bg"), "#!/bin/sh\n(sleep 1; wc -c > "+out+") <&0 >/dev/null 2>&1 &\nexit 0\n")
	f.write(filepath.Join(f.repo, ".git", "hooks", "pre-push"), "#!/bin/sh\ncat > /dev/null\n")
	input := strings.Repeat("0000000000000000000000000000000000000000 1111111111111111111111111111111111111111 refs/heads/main\n", 4000)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := f.hook(ctx, "pre-push", f.env(), "origin", "url")
	cmd.Stdin = strings.NewReader(input)
	res, err := cmd.CombinedOutput()
	require.NoError(t, err, string(res))
	require.Eventually(t, func() bool {
		b, err := os.ReadFile(out)
		return err == nil && strings.TrimSpace(string(b)) == strconv.Itoa(len(input))
	}, 10*time.Second, 50*time.Millisecond)
}

func TestHookSingleStageReadsStdinDirectly(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	tmp := t.TempDir()
	// The only stage lists the temp dir while it runs: no spool may exist.
	f.write(filepath.Join(f.repo, ".git", "hooks", "reference-transaction"), "#!/bin/sh\nls \"$TMPDIR\" > \"$PWD/tmp.list\"\ncat > \"$PWD/standard.in\"\n")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := f.hook(ctx, "reference-transaction", append(f.env(), "TMPDIR="+tmp), "prepared")
	cmd.Stdin = strings.NewReader("0000 1111 refs/heads/main\n")
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
	listing, err := os.ReadFile(filepath.Join(f.repo, "tmp.list"))
	require.NoError(t, err)
	require.Empty(t, strings.TrimSpace(string(listing)), "one stage needs no spool file")
	got, err := os.ReadFile(filepath.Join(f.repo, "standard.in"))
	require.NoError(t, err)
	require.Equal(t, "0000 1111 refs/heads/main\n", string(got))
}
