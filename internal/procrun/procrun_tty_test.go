//go:build darwin || linux

package procrun

import (
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

func TestStopTreeKillsDescendantsOnly(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// The stage shares our process group in a terminal; stopTree must not signal the group.
	cmd := exec.Command("sh", "-c", `sh -c 'trap "" TERM; echo $$ > grandchild; exec sleep 300' & while [ ! -s grandchild ]; do :; done; sleep 300`)
	cmd.Dir = dir
	require.NoError(t, cmd.Start())
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	path := filepath.Join(dir, "grandchild")
	require.Eventually(t, func() bool { b, _ := os.ReadFile(path); return len(strings.TrimSpace(string(b))) > 0 }, 5*time.Second, 10*time.Millisecond)
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	grandchild, err := strconv.Atoi(strings.TrimSpace(string(b)))
	require.NoError(t, err)
	t.Cleanup(func() { _ = syscall.Kill(grandchild, syscall.SIGKILL) })

	require.Error(t, stopTree(cmd.Process.Pid, done))
	require.Eventually(t, func() bool { return syscall.Kill(grandchild, 0) == syscall.ESRCH }, 10*time.Second, 20*time.Millisecond)
	require.NoError(t, syscall.Kill(os.Getpid(), 0), "the test process itself must survive")
}

func TestDescendantsFollowProcessIdentity(t *testing.T) {
	t.Parallel()
	cmd := exec.Command("sh", "-c", "sleep 30 & sleep 30 & wait")
	require.NoError(t, cmd.Start())
	stage := proc{pid: cmd.Process.Pid}
	t.Cleanup(func() {
		for _, p := range stage.descendants(snapshot()) {
			_ = syscall.Kill(p.pid, syscall.SIGKILL)
		}
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	stage.start = snapshot()[stage.pid].start
	require.NotEmpty(t, stage.start)
	require.Eventually(t, func() bool { return len(stage.descendants(snapshot())) == 2 }, 5*time.Second, 20*time.Millisecond)

	// The same pid with another start time is another process: no children, no signal.
	reused := proc{pid: stage.pid, start: "Thu Jan  1 00:00:00 1970"}
	require.Empty(t, reused.descendants(snapshot()))
	require.Empty(t, live([]proc{reused}, snapshot()))
}

func TestStopTreeKillsStageWithoutPs(t *testing.T) {
	// Not parallel: it changes a package variable.
	old := psPaths
	psPaths = []string{"/nonexistent/ps"}
	t.Cleanup(func() { psPaths = old })
	cmd := exec.Command("/bin/sh", "-c", "trap '' TERM; exec /bin/sleep 300")
	require.NoError(t, cmd.Start())
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	time.Sleep(200 * time.Millisecond)
	start := time.Now()
	require.Error(t, stopTree(cmd.Process.Pid, done), "the stage dies by SIGKILL")
	require.Less(t, time.Since(start), 10*time.Second)
}

func TestStopTreeKillsTermIgnoringChildWithoutPs(t *testing.T) {
	// Not parallel: it changes a package variable.
	dir := t.TempDir()
	cmd := exec.Command("/bin/sh", "-c", `/bin/sh -c 'trap "" TERM; echo $$ > child; exec /bin/sleep 300' & while [ ! -s child ]; do :; done; wait`)
	cmd.Dir = dir
	require.NoError(t, cmd.Start())
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	path := filepath.Join(dir, "child")
	require.Eventually(t, func() bool { b, _ := os.ReadFile(path); return len(strings.TrimSpace(string(b))) > 0 }, 5*time.Second, 10*time.Millisecond)
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	child, err := strconv.Atoi(strings.TrimSpace(string(b)))
	require.NoError(t, err)
	t.Cleanup(func() { _ = syscall.Kill(child, syscall.SIGKILL) })

	// ps works for the first scan, then fails: the child must still get SIGKILL.
	old := psPaths
	t.Cleanup(func() { psPaths = old })
	snap := snapshot()
	stage := proc{pid: cmd.Process.Pid, start: snap[cmd.Process.Pid].start}
	tree := stage.descendants(snap)
	require.NotEmpty(t, tree)
	psPaths = []string{"/nonexistent/ps"}
	stopErr := stopTreeWith(cmd.Process.Pid, done, stage, tree)
	require.Error(t, stopErr)
	require.Eventually(t, func() bool { return syscall.Kill(child, 0) == syscall.ESRCH }, 10*time.Second, 20*time.Millisecond)
}
