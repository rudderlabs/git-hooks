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

func TestDescendants(t *testing.T) {
	t.Parallel()
	cmd := exec.Command("sh", "-c", "sleep 30 & sleep 30 & wait")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		for _, p := range descendants(cmd.Process.Pid) {
			_ = syscall.Kill(p, syscall.SIGKILL)
		}
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	require.Eventually(t, func() bool { return len(descendants(cmd.Process.Pid)) == 2 }, 5*time.Second, 20*time.Millisecond)
}
