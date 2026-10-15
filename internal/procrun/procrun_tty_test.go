//go:build darwin || linux

package procrun

import (
	"errors"
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

func TestStopTreeNeverSignalsUnverifiedPids(t *testing.T) {
	// Not parallel: it changes a package variable.
	unrelated := exec.Command("/bin/sleep", "300")
	require.NoError(t, unrelated.Start())
	t.Cleanup(func() { _ = unrelated.Process.Kill(); _, _ = unrelated.Process.Wait() })
	stage := exec.Command("/bin/sleep", "300")
	require.NoError(t, stage.Start())
	done := make(chan error, 1)
	go func() { done <- stage.Wait() }()

	old := psPaths
	psPaths = []string{"/nonexistent/ps"}
	t.Cleanup(func() { psPaths = old })
	// The tracked pid now belongs to an unrelated process, and ps cannot tell.
	tree := []proc{{pid: unrelated.Process.Pid, start: "Thu Jan  1 00:00:00 1970"}}
	start := time.Now()
	require.Error(t, stopTreeWith(stage.Process.Pid, done, proc{pid: stage.Process.Pid}, tree), "the stage dies")
	// With a tracked process still unverified, the loop waits for ps to recover,
	// but never past the grace deadline.
	require.Less(t, time.Since(start), 11*time.Second)
	require.NoError(t, unrelated.Process.Signal(syscall.Signal(0)), "an unverified pid is never signalled")
}

func TestStopTreeRecoversFromOneFailedPs(t *testing.T) {
	// Not parallel: it changes a package variable.
	dir := t.TempDir()
	flaky := filepath.Join(dir, "ps")
	require.NoError(t, os.WriteFile(flaky, []byte("#!/bin/sh\nif [ -f \""+dir+"/once\" ]; then exec /bin/ps \"$@\"; fi\ntouch \""+dir+"/once\"\nexit 1\n"), 0o755))
	cmd := exec.Command("/bin/sh", "-c", "/bin/sleep 300 & echo $! > child; wait")
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

	old := psPaths
	psPaths = []string{flaky}
	t.Cleanup(func() { psPaths = old })
	require.Error(t, stopTree(cmd.Process.Pid, done))
	require.Eventually(t, func() bool { return syscall.Kill(child, 0) == syscall.ESRCH }, 10*time.Second, 20*time.Millisecond, "the child must not outlive the stage")
}

func TestStopTreeIsBoundedWhenPsHangs(t *testing.T) {
	// Not parallel: it changes a package variable.
	dir := t.TempDir()
	hang := filepath.Join(dir, "ps")
	require.NoError(t, os.WriteFile(hang, []byte("#!/bin/sh\nexec /bin/sleep 30\n"), 0o755))
	old := psPaths
	psPaths = []string{hang}
	t.Cleanup(func() { psPaths = old })
	cmd := exec.Command("/bin/sh", "-c", "trap '' TERM; exec /bin/sleep 300")
	require.NoError(t, cmd.Start())
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	start := time.Now()
	require.Error(t, stopTree(cmd.Process.Pid, done))
	require.Less(t, time.Since(start), 11*time.Second, "a hanging ps must not stretch the stop")
}

func TestStopTreeSurvivesOneFailedPsAfterReap(t *testing.T) {
	// Not parallel: it changes a package variable.
	dir := t.TempDir()
	flaky := filepath.Join(dir, "ps")
	// The third call fails; every other call works.
	script := "#!/bin/sh\nn=$(cat \"" + dir + "/n\" 2>/dev/null || echo 0)\nn=$((n+1))\necho $n > \"" + dir + "/n\"\n[ \"$n\" = 3 ] && exit 1\nexec /bin/ps \"$@\"\n"
	require.NoError(t, os.WriteFile(flaky, []byte(script), 0o755))
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

	old := psPaths
	psPaths = []string{flaky}
	t.Cleanup(func() { psPaths = old })
	require.Error(t, stopTree(cmd.Process.Pid, done))
	require.Eventually(t, func() bool { return syscall.Kill(child, 0) == syscall.ESRCH }, 10*time.Second, 20*time.Millisecond, "one failed ps must not save the child")
}

func TestStopTreeCatchesForksOfAnOrphanedWorker(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// The stage dies on SIGTERM; its worker ignores TERM and keeps forking.
	cmd := exec.Command("/bin/sh", "-c", `/bin/sh -c 'trap "" TERM; echo $$ > worker; while :; do /bin/sh -c "trap \"\" TERM; exec /bin/sleep 1001" & echo $! >> kids; /bin/sleep 0.05; done' & /bin/sleep 300`)
	cmd.Dir = dir
	require.NoError(t, cmd.Start())
	// The worker forks forever; kill it ourselves whatever stopTree does,
	// but only while its pid still has the start time recorded here.
	var worker proc
	require.Eventually(t, func() bool {
		b, err := os.ReadFile(filepath.Join(dir, "worker"))
		if err != nil {
			return false
		}
		pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
		if err != nil {
			return false
		}
		worker = proc{pid: pid, start: snapshot()[pid].start}
		return worker.start != ""
	}, 5*time.Second, 10*time.Millisecond)
	t.Cleanup(func() {
		if len(live([]proc{worker}, snapshot())) == 1 {
			_ = syscall.Kill(worker.pid, syscall.SIGKILL)
		}
	})
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	time.Sleep(500 * time.Millisecond)
	_ = stopTree(cmd.Process.Pid, done)

	b, err := os.ReadFile(filepath.Join(dir, "kids"))
	require.NoError(t, err)
	var leaked []int
	for _, field := range strings.Fields(string(b)) {
		pid, err := strconv.Atoi(field)
		require.NoError(t, err)
		if syscall.Kill(pid, 0) == nil {
			// A pid that is gone makes ps exit 1 with no output; anything else must work.
			out, err := exec.Command("/bin/ps", "-o", "command=", "-p", field).Output()
			var exitErr *exec.ExitError
			gone := errors.As(err, &exitErr) && exitErr.ExitCode() == 1 && len(out) == 0
			if err != nil && !gone {
				t.Fatalf("ps failed: %v", err)
			}
			if strings.Contains(string(out), "sleep 1001") {
				leaked = append(leaked, pid)
				_ = syscall.Kill(pid, syscall.SIGKILL)
			}
		}
	}
	require.Empty(t, leaked, "forks of a verified worker must die with it")
}

func TestStopTreeNeverLeavesAProcessStopped(t *testing.T) {
	// Not parallel: it changes package variables.
	dir := t.TempDir()
	// This ps shifts every start time by a year once "shift" exists, as a
	// clock step would shift what ps reports.
	ps := filepath.Join(dir, "ps")
	script := "#!/bin/sh\nif [ -f \"" + dir + "/shift\" ]; then /bin/ps \"$@\" | awk '{ $NF = $NF + 1; print }'; else exec /bin/ps \"$@\"; fi\n"
	require.NoError(t, os.WriteFile(ps, []byte(script), 0o755))
	cmd := exec.Command("/bin/sh", "-c", `/bin/sh -c 'trap "" TERM; echo $$ > child; exec /bin/sleep 300' & trap "" TERM; while [ ! -s child ]; do :; done; wait`)
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

	oldPs, oldHook := psPaths, beforeVerify
	psPaths = []string{ps}
	beforeVerify = func() { _ = os.WriteFile(filepath.Join(dir, "shift"), nil, 0o644) }
	t.Cleanup(func() { psPaths, beforeVerify = oldPs, oldHook })
	_ = stopTree(cmd.Process.Pid, done)

	out, err := exec.Command("/bin/ps", "-o", "state=", "-p", strconv.Itoa(child)).Output()
	if err == nil {
		require.NotContains(t, strings.TrimSpace(string(out)), "T", "an unverifiable process may run on, but never stay stopped")
	}
}
