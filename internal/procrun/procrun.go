//go:build darwin || linux

package procrun

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// Run executes argv with the supplied resource limits.
//
// When git runs in a terminal, the stage stays in git's process group, so
// Ctrl-C, Ctrl-Z, fg, bg and kill %1 work as the shell expects. Without a
// terminal (IDEs, GUI apps, agents) the stage gets its own process group, so a
// timeout or failure can stop every process it started.
func Run(ctx context.Context, argv []string, opt Options) error {
	if len(argv) == 0 || argv[0] == "" {
		return errors.New("stage command is empty")
	}
	parent := ctx
	if opt.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opt.Timeout)
		defer cancel()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if opt.Nice > 0 {
		argv = append([]string{"nice", "-n", strconv.Itoa(opt.Nice)}, argv...)
	}
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env, cmd.Dir = opt.Env, opt.Dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = opt.Stdin, opt.Stdout, opt.Stderr
	applyCPUCap(cmd, opt.CPUs)
	interactive := InForegroundJob()
	if !interactive {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	pid := cmd.Process.Pid
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		// In a terminal, Ctrl-C already reached the whole job; there is no group of our own to reap.
		if err != nil && !interactive {
			reapGroup(pid)
		}
		return err
	case <-ctx.Done():
	}
	message := "stage canceled and was killed"
	if parent.Err() == nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		message = fmt.Sprintf("stage timed out after %s and was killed", opt.Timeout)
	}
	var stopErr error
	if interactive {
		stopErr = stopTree(pid, done)
	} else {
		stopErr = stopGroup(pid, done)
	}
	return fmt.Errorf("%s: %w", message, errors.Join(ctx.Err(), stopErr))
}

// InForegroundJob reports whether git runs with a controlling terminal. Then
// the stage stays in git's job, so the shell handles Ctrl-C, Ctrl-Z, fg, bg
// and kill %1, even for `git commit &` that is brought back with fg.
func InForegroundJob() bool {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	_ = tty.Close()
	return true
}

func stopGroup(pid int, done <-chan error) error {
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	var waitErr error
	select {
	case waitErr = <-done:
		// A leader can exit before its workers; keep the escalation armed for them.
		if errors.Is(syscall.Kill(-pid, 0), syscall.ESRCH) {
			return waitErr
		}
	case <-timer.C:
		_ = syscall.Kill(-pid, syscall.SIGKILL)
		return <-done
	}
	<-timer.C
	_ = syscall.Kill(-pid, syscall.SIGKILL)
	return waitErr
}

// stopTree stops a stage that shares git's process group. It signals the
// stage and its descendants by pid, because signalling the group would kill
// git and the shell's job with it.
func stopTree(pid int, done <-chan error) error {
	// A process is its pid plus its start time. A pid reused in a later
	// second never matches and is never signalled. ps reports the start to
	// the second, so a pid reused within the same second still would.
	return stopTreeWith(pid, done, proc{pid: pid}, nil)
}

// stopTreeWith stops the stage and its descendants. tree holds processes
// already known; the stage's start time is learned from snapshots while the
// stage is unreaped.
func stopTreeWith(pid int, done <-chan error, stage proc, tree []proc) error {
	var waitErr error
	exited := false
	reaped := func() bool {
		if !exited {
			select {
			case waitErr = <-done:
				exited = true
			default:
			}
		}
		return exited
	}
	known := map[proc]bool{}
	var fresh []proc
	add := func(ps []proc) {
		for _, p := range ps {
			if !known[p] {
				known[p] = true
				tree = append(tree, p)
				fresh = append(fresh, p)
			}
		}
	}
	add(tree)
	// learn takes the stage's identity only while it is unreaped: after the
	// reap its pid, and the children below it, may be someone else's. This
	// also recovers from a ps that failed on an earlier call.
	learn := func(by time.Time) map[int]psEntry {
		snap := snapshotBy(by)
		if snap != nil && !reaped() {
			if stage.start == "" {
				stage.start = snap[pid].start
			}
			add(stage.descendants(snap))
		}
		return snap
	}
	// Every ps call below is bounded by these deadlines, so the whole stop
	// takes at most about 10 s even when ps hangs.
	start := time.Now()
	// Learn the tree before the first signal: a stage that dies on SIGTERM
	// takes the only route to its children with it. Discovery gets 2.5 s,
	// room for one full ps call and a retry; a ps that never works (BusyBox
	// has no lstart) leaves only the stage to signal.
	discover := start.Add(2500 * time.Millisecond)
	for stage.start == "" && !reaped() && time.Now().Before(discover) {
		if learn(discover) == nil {
			// A ps that fails at once must not be retried in a tight loop.
			time.Sleep(100 * time.Millisecond)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	if !reaped() {
		_ = syscall.Kill(pid, syscall.SIGTERM)
	}
	// SIGTERM goes once to each process, so a TERM trap runs once. A batch
	// that ps could not verify stays queued for the next try.
	if signalProcs(fresh, pid, syscall.SIGTERM, reaped, deadline) {
		fresh = nil
	}
	for time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
		snap := learn(deadline)
		if signalProcs(fresh, pid, syscall.SIGTERM, reaped, deadline) {
			fresh = nil
		}
		// Once the stage is gone, stop early only on a real snapshot: one
		// failed ps must not abandon a child that ignores SIGTERM.
		if reaped() && ((snap != nil && len(live(tree, snap)) == 0) || len(tree) == 0) {
			return waitErr
		}
	}
	// Freeze everything first, so nothing forks between the last scan and
	// the kill.
	final := time.Now().Add(time.Second)
	if !reaped() {
		_ = syscall.Kill(pid, syscall.SIGSTOP)
	}
	// A stopped process cannot exit, so its pid stays its own until it is
	// killed: everything stopped here is killed by pid, even if ps fails now.
	var stopped []int
	if snap := snapshotBy(final); snap != nil {
		for _, p := range live(tree, snap) {
			if p.pid != pid && syscall.Kill(p.pid, syscall.SIGSTOP) == nil {
				stopped = append(stopped, p.pid)
			}
		}
	}
	learn(final)
	signalProcs(tree, pid, syscall.SIGKILL, reaped, final)
	for _, p := range stopped {
		_ = syscall.Kill(p, syscall.SIGKILL)
	}
	if !exited {
		waitErr = <-done
	}
	return waitErr
}

// psPaths is a variable so a test can make ps fail.
var psPaths = []string{"/bin/ps", "/usr/bin/ps"}

type proc struct {
	pid   int
	start string
}

type psEntry struct {
	ppid  int
	start string
}

// snapshot reads every process with its parent and start time. ps runs by
// absolute path under a deadline, so a broken PATH cannot hang a kill.
func snapshot() map[int]psEntry {
	return snapshotBy(time.Now().Add(2 * time.Second))
}

// snapshotBy is snapshot with a caller's deadline, so a slow ps cannot push
// a stop past its grace period.
func snapshotBy(deadline time.Time) map[int]psEntry {
	if limit := time.Now().Add(2 * time.Second); deadline.After(limit) {
		deadline = limit
	}
	// A ps stuck in the kernel cannot be killed, and exec waits for it even
	// after the context ends. Read it in a goroutine and stop waiting at the
	// deadline; the goroutine ends with the process.
	result := make(chan []byte, 1)
	go func() {
		ctx, cancel := context.WithDeadline(context.Background(), deadline)
		defer cancel()
		// Some Linux images keep ps only in /usr/bin. BusyBox ps lacks lstart
		// and fails here; callers then signal only the stage.
		for _, ps := range psPaths {
			cmd := exec.CommandContext(ctx, ps, "-A", "-o", "pid=,ppid=,lstart=")
			cmd.WaitDelay = 100 * time.Millisecond
			if out, err := cmd.Output(); err == nil {
				result <- out
				return
			}
		}
		result <- nil
	}()
	var out []byte
	select {
	case out = <-result:
	case <-time.After(time.Until(deadline) + 200*time.Millisecond):
	}
	if out == nil {
		return nil
	}
	procs := map[int]psEntry{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		pid, err1 := strconv.Atoi(fields[0])
		ppid, err2 := strconv.Atoi(fields[1])
		if err1 == nil && err2 == nil {
			procs[pid] = psEntry{ppid: ppid, start: strings.Join(fields[2:], " ")}
		}
	}
	return procs
}

// descendants lists the processes below p, but only while p is still the
// same process: below a reused pid live someone else's children.
func (p proc) descendants(snap map[int]psEntry) []proc {
	if p.start == "" || snap[p.pid].start != p.start {
		return nil
	}
	children := map[int][]int{}
	for pid, e := range snap {
		children[e.ppid] = append(children[e.ppid], pid)
	}
	var all []proc
	queue := []int{p.pid}
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		for _, c := range children[next] {
			all = append(all, proc{pid: c, start: snap[c].start})
			queue = append(queue, c)
		}
	}
	return all
}

// live returns the processes that still run as themselves. Without a
// snapshot nothing can be verified, so nothing is returned: signalling a pid
// that may now belong to someone else is worse than leaving a child running.
func live(procs []proc, snap map[int]psEntry) []proc {
	var out []proc
	if snap == nil {
		return nil
	}
	for _, p := range procs {
		if e, ok := snap[p.pid]; ok && p.start != "" && e.start == p.start {
			out = append(out, p)
		}
	}
	return out
}

// signalProcs signals each descendant that still runs as itself, and on
// SIGKILL the stage too. The stage is our child: until cmd.Wait reaps it, its
// pid cannot be reused, so it needs no start time and is signalled even when
// ps fails.
func signalProcs(procs []proc, stagePid int, sig syscall.Signal, reaped func() bool, by time.Time) (verified bool) {
	verified = true
	if len(procs) > 0 {
		snap := snapshotBy(by)
		verified = snap != nil
		for _, p := range live(procs, snap) {
			if p.pid != stagePid {
				_ = syscall.Kill(p.pid, sig)
			}
		}
	}
	if sig == syscall.SIGKILL && !reaped() {
		_ = syscall.Kill(stagePid, sig)
	}
	return verified
}

// Background jobs ignore SIGINT in non-interactive shells, so failed stages need group cleanup.
func reapGroup(pid int) {
	if errors.Is(syscall.Kill(-pid, 0), syscall.ESRCH) {
		return
	}
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if errors.Is(syscall.Kill(-pid, 0), syscall.ESRCH) {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	_ = syscall.Kill(-pid, syscall.SIGKILL)
}
