//go:build darwin || linux

package procrun_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/rudderlabs/git-hooks/internal/procrun"
	"github.com/stretchr/testify/require"
)

func TestExitCode(t *testing.T) {
	t.Parallel()
	err := procrun.Run(context.Background(), []string{"sh", "-c", "exit 3"}, procrun.Options{})
	var exitErr *exec.ExitError
	require.ErrorAs(t, err, &exitErr)
	require.Equal(t, 3, exitErr.ExitCode())
}

func TestTimeoutKillsGroup(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Reap the background child on TERM so even hosts without an init reaper stay clean.
	err := procrun.Run(ctx, []string{"sh", "-c", `trap 'wait "$child"; exit 0' TERM; sleep 300 & child=$!; echo "$child" > child; echo $$ > parent; wait "$child"`}, procrun.Options{Dir: dir, Timeout: 200 * time.Millisecond})
	child := readPID(t, filepath.Join(dir, "child"))
	parent := readPID(t, filepath.Join(dir, "parent"))
	t.Cleanup(func() { _ = syscall.Kill(child, syscall.SIGKILL); _ = syscall.Kill(parent, syscall.SIGKILL) })
	require.ErrorContains(t, err, "stage timed out after 200ms and was killed")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Eventually(t, func() bool { return syscall.Kill(child, 0) == syscall.ESRCH }, time.Second, 10*time.Millisecond)
	require.ErrorIs(t, syscall.Kill(parent, 0), syscall.ESRCH)
}

func TestCancelKillsGroup(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- procrun.Run(ctx, []string{"sh", "-c", `trap 'wait "$child"; exit 0' TERM; sleep 300 & child=$!; echo "$child" > child; echo $$ > parent; wait "$child"`}, procrun.Options{Dir: dir})
	}()
	require.Eventually(t, func() bool {
		data, _ := os.ReadFile(filepath.Join(dir, "parent"))
		return strings.HasSuffix(string(data), "\n")
	}, 2*time.Second, 10*time.Millisecond)
	child := readPID(t, filepath.Join(dir, "child"))
	parent := readPID(t, filepath.Join(dir, "parent"))
	t.Cleanup(func() { _ = syscall.Kill(child, syscall.SIGKILL); _ = syscall.Kill(parent, syscall.SIGKILL) })
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(7 * time.Second):
		t.Fatal("Run did not stop after cancellation")
	}
	require.Eventually(t, func() bool { return syscall.Kill(child, 0) == syscall.ESRCH }, time.Second, 10*time.Millisecond)
	require.ErrorIs(t, syscall.Kill(parent, 0), syscall.ESRCH)
}

func readPID(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	require.NoError(t, err)
	require.Greater(t, pid, 0)
	return pid
}

func isolatedCache(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", home)
	cache, err := os.UserCacheDir()
	require.NoError(t, err)
	return cache
}

func TestNodeOptionsAndCache(t *testing.T) {
	cache := isolatedCache(t)
	t.Setenv("NODE_OPTIONS", "--trace-warnings")
	t.Setenv("GIT_HOOKS_CPUS", "99")
	var out bytes.Buffer
	opt := procrun.Options{CPUs: 2, Stdout: &out}
	argv := []string{"sh", "-c", `printf '%s\n%s' "$NODE_OPTIONS" "$GIT_HOOKS_CPUS"`}
	require.NoError(t, procrun.Run(context.Background(), argv, opt))
	lines := strings.Split(out.String(), "\n")
	require.Len(t, lines, 2)
	require.Equal(t, "2", lines[1])
	require.True(t, strings.HasPrefix(lines[0], "--trace-warnings --require="), lines[0])
	path := strings.TrimPrefix(lines[0], "--trace-warnings --require=")
	require.Equal(t, filepath.Join(cache, "git-hooks"), filepath.Dir(path))
	require.Regexp(t, `^cpucap-[a-f0-9]+\.cjs$`, filepath.Base(path))
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.Equal(t, os.FileMode(0644), info.Mode().Perm())
	out.Reset()
	require.NoError(t, procrun.Run(context.Background(), argv, opt))
	again, err := os.Stat(path)
	require.NoError(t, err)
	require.True(t, os.SameFile(info, again), "existing preload should be reused")
	entries, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, entries, 1)

	out.Reset()
	opt.Env = []string{"NODE_OPTIONS=--no-warnings", "GIT_HOOKS_CPUS=8"}
	require.NoError(t, procrun.Run(context.Background(), argv, opt))
	require.Equal(t, "--no-warnings --require="+path+"\n2", out.String())
}

func TestCPUCapDisabled(t *testing.T) {
	cache := isolatedCache(t)
	var out bytes.Buffer
	err := procrun.Run(context.Background(), []string{"sh", "-c", `printf '%s' "$NODE_OPTIONS"`}, procrun.Options{Env: []string{"NODE_OPTIONS=--trace-warnings"}, Stdout: &out})
	require.NoError(t, err)
	require.Equal(t, "--trace-warnings", out.String())
	_, err = os.Stat(filepath.Join(cache, "git-hooks"))
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestCPUCapCacheWithSpaces(t *testing.T) {
	home := filepath.Join(t.TempDir(), "has space")
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", home)
	var out, stderr bytes.Buffer
	err := procrun.Run(context.Background(), []string{"sh", "-c", `printf '%s\n%s' "$NODE_OPTIONS" "$GIT_HOOKS_CPUS"`}, procrun.Options{CPUs: 2, Env: []string{"NODE_OPTIONS=--trace-warnings"}, Stdout: &out, Stderr: &stderr})
	require.NoError(t, err)
	require.Equal(t, "--trace-warnings\n2", out.String())
	require.Contains(t, stderr.String(), "warning")
	require.Equal(t, 1, strings.Count(stderr.String(), "\n"))
}

func TestKillEscalatesAfterLeaderExits(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- procrun.Run(ctx, []string{"sh", "-c", `trap 'echo term > terminated; exit 0' TERM; echo $$ > parent; sh -c 'trap "" TERM; echo $$ > child; exec sleep 300' & wait`}, procrun.Options{Dir: dir})
	}()
	require.Eventually(t, func() bool {
		data, _ := os.ReadFile(filepath.Join(dir, "child"))
		return strings.HasSuffix(string(data), "\n")
	}, 2*time.Second, 10*time.Millisecond)
	child := readPID(t, filepath.Join(dir, "child"))
	parent := readPID(t, filepath.Join(dir, "parent"))
	t.Cleanup(func() { _ = syscall.Kill(child, syscall.SIGKILL); _ = syscall.Kill(parent, syscall.SIGKILL) })
	start := time.Now()
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(8 * time.Second):
		t.Fatal("Run did not kill the remaining worker")
	}
	require.GreaterOrEqual(t, time.Since(start), 5*time.Second)
	require.FileExists(t, filepath.Join(dir, "terminated"))
	require.Eventually(t, func() bool { return syscall.Kill(child, 0) == syscall.ESRCH }, time.Second, 10*time.Millisecond)
	require.ErrorIs(t, syscall.Kill(parent, 0), syscall.ESRCH)
}

func TestInvalidCommand(t *testing.T) {
	t.Parallel()
	require.Error(t, procrun.Run(context.Background(), nil, procrun.Options{}))
	require.Error(t, procrun.Run(context.Background(), []string{""}, procrun.Options{}))
	require.Error(t, procrun.Run(context.Background(), []string{filepath.Join(t.TempDir(), "missing")}, procrun.Options{}))
}

func TestCanceledBeforeStart(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := procrun.Run(ctx, []string{"sh", "-c", "touch started"}, procrun.Options{Dir: dir})
	require.ErrorIs(t, err, context.Canceled)
	require.NoFileExists(t, filepath.Join(dir, "started"))
}

func TestCPUCapCacheFailure(t *testing.T) {
	for _, name := range []string{"HOME unset", "read-only cache", "not a directory"} {
		t.Run(name, func(t *testing.T) {
			cache := isolatedCache(t)
			switch name {
			case "HOME unset":
				t.Setenv("HOME", "")
				t.Setenv("XDG_CACHE_HOME", "")
			case "read-only cache":
				require.NoError(t, os.MkdirAll(cache, 0755))
				require.NoError(t, os.Chmod(cache, 0555))
				t.Cleanup(func() { require.NoError(t, os.Chmod(cache, 0755)) })
			case "not a directory":
				require.NoError(t, os.MkdirAll(cache, 0755))
				require.NoError(t, os.WriteFile(filepath.Join(cache, "git-hooks"), []byte("not a directory"), 0644))
			}
			var out, stderr bytes.Buffer
			opt := procrun.Options{CPUs: 2, Env: []string{"NODE_OPTIONS=--trace-warnings"}, Stdout: &out, Stderr: &stderr}
			argv := []string{"sh", "-c", `printf '%s' "$NODE_OPTIONS"`}
			require.NoError(t, procrun.Run(context.Background(), argv, opt))
			require.Equal(t, "--trace-warnings", out.String())
			require.True(t, strings.HasPrefix(stderr.String(), "git-hooks: warning: CPU cap skipped: "), stderr.String())
			require.Equal(t, 1, strings.Count(stderr.String(), "\n"))
			opt.Stderr = nil
			require.NoError(t, procrun.Run(context.Background(), argv, opt))
		})
	}
}

func TestCPUCapNode(t *testing.T) {
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node is not on PATH")
	}
	isolatedCache(t)
	var out, stderr bytes.Buffer
	err := procrun.Run(context.Background(), []string{"node", "-e", `console.log(require('os').availableParallelism?.() ?? require('os').cpus().length, require('os').cpus().length)`}, procrun.Options{CPUs: 2, Env: []string{}, Stdout: &out, Stderr: &stderr})
	require.NoError(t, err, stderr.String())
	require.Equal(t, "2 2\n", out.String())
}

func TestDefaultOptions(t *testing.T) {
	for _, tc := range []struct {
		name, timeout, cpus, nice string
		wantTimeout               time.Duration
		wantCPUs, wantNice        int
	}{
		{"unset", "", "", "", 10 * time.Minute, 2, 10},
		{"custom", "3s", "4", "5", 3 * time.Second, 4, 5},
		{"disabled", "1m", "0", "0", time.Minute, 0, 0},
		{"bad", "soon", "many", "kind", 10 * time.Minute, 2, 10},
		{"negative", "-1s", "-1", "-1", 10 * time.Minute, 2, 10},
		{"zero timeout", "0", "2", "10", 0, 2, 10},
		{"zero duration", "0s", "2", "10", 0, 2, 10},
		{"overflow", "999999999999999999999h", "999999999999999999999", "999999999999999999999", 10 * time.Minute, 2, 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GIT_HOOKS_TIMEOUT", tc.timeout)
			t.Setenv("GIT_HOOKS_CPUS", tc.cpus)
			t.Setenv("GIT_HOOKS_NICE", tc.nice)
			opt := procrun.DefaultOptions()
			require.Equal(t, tc.wantTimeout, opt.Timeout)
			require.Equal(t, tc.wantCPUs, opt.CPUs)
			require.Equal(t, tc.wantNice, opt.Nice)
		})
	}
}

func TestNice(t *testing.T) {
	t.Parallel()
	readNice := func(n int) int {
		var out bytes.Buffer
		err := procrun.Run(context.Background(), []string{"sh", "-c", "ps -o nice= -p $$"}, procrun.Options{Nice: n, Stdout: &out, Stderr: &out})
		require.NoError(t, err, out.String())
		value, err := strconv.Atoi(strings.TrimSpace(out.String()))
		require.NoError(t, err)
		return value
	}
	base := readNice(0)
	require.Equal(t, min(base+5, 19), readNice(5))
}

func TestEnvironmentAndIO(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var out, stderr bytes.Buffer
	err := procrun.Run(context.Background(), []string{"sh", "-c", `read value; printf '%s:%s' "$CUSTOM" "$value"; printf problem >&2; pwd > cwd`}, procrun.Options{
		Env: append(os.Environ(), "CUSTOM=custom"), Dir: dir,
		Stdin: strings.NewReader("input\n"), Stdout: &out, Stderr: &stderr,
	})
	require.NoError(t, err)
	require.Equal(t, "custom:input", out.String())
	require.Equal(t, "problem", stderr.String())
	data, err := os.ReadFile(dir + "/cwd")
	require.NoError(t, err)
	dir, err = filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	require.Equal(t, dir, strings.TrimSpace(string(data)))
}

func TestSuccessfulStageLeavesBackgroundJobs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	err := procrun.Run(context.Background(), []string{"sh", "-c", `sleep 300 & echo $! > child; exit 0`}, procrun.Options{Dir: dir})
	child := readPID(t, filepath.Join(dir, "child"))
	t.Cleanup(func() { _ = syscall.Kill(child, syscall.SIGKILL) })
	require.NoError(t, err)
	require.NoError(t, syscall.Kill(child, 0), "successful hooks may intentionally start background work")
}

func TestFailedStageReapsBackgroundJobs(t *testing.T) {
	t.Parallel()
	for _, ending := range []string{"exit 1", "kill -INT $$"} {
		t.Run(ending, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			// One job ignores TERM, so the reaper must escalate to KILL.
			err := procrun.Run(context.Background(), []string{"sh", "-c", `sleep 300 & echo $! > a; sh -c 'trap "" TERM; echo $$ > b; exec sleep 300' & while [ ! -s b ]; do :; done; ` + ending}, procrun.Options{Dir: dir})
			a := readPID(t, filepath.Join(dir, "a"))
			b := readPID(t, filepath.Join(dir, "b"))
			t.Cleanup(func() { _ = syscall.Kill(a, syscall.SIGKILL); _ = syscall.Kill(b, syscall.SIGKILL) })
			require.Error(t, err)
			require.Eventually(t, func() bool {
				return syscall.Kill(a, 0) == syscall.ESRCH && syscall.Kill(b, 0) == syscall.ESRCH
			}, 5*time.Second, 20*time.Millisecond)
		})
	}
}
