//go:build darwin || linux

package commands_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
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
