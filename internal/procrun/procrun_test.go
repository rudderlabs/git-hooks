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

func TestDefaultOptions(t *testing.T) {
	for _, tc := range []struct {
		name, timeout string
		want          time.Duration
	}{
		{"unset", "", 10 * time.Minute},
		{"custom", "3s", 3 * time.Second},
		{"bad", "soon", 10 * time.Minute},
		{"negative", "-1s", 10 * time.Minute},
		{"zero", "0", 0},
		{"zero duration", "0s", 0},
		{"overflow", "999999999999999999999h", 10 * time.Minute},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("GIT_HOOKS_TIMEOUT", tc.timeout)
			require.Equal(t, tc.want, procrun.DefaultOptions().Timeout)
		})
	}
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
