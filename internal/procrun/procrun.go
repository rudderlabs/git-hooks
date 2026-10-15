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
	snap := snapshot()
	stage := proc{pid: pid, start: snap[pid].start}
	return stopTreeWith(pid, done, stage, stage.descendants(snap))
}

// stopTreeWith stops the stage and the processes in tree, then rescans
// below the stage at the grace deadline.
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
	signalProcs(tree, pid, syscall.SIGTERM, reaped)
	grace := time.NewTimer(5 * time.Second)
	defer grace.Stop()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
		case <-grace.C:
			// Children the stage started during the grace period are below it too.
			tree = append(tree, stage.descendants(snapshot())...)
			signalProcs(tree, pid, syscall.SIGKILL, reaped)
			if !exited {
				waitErr = <-done
			}
			return waitErr
		}
		// A failed snapshot says nothing; wait for the next tick or the grace.
		if snap := snapshot(); reaped() && snap != nil && len(live(tree, snap)) == 0 {
			return waitErr
		}
	}
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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var out []byte
	var err error
	// Some Linux images keep ps only in /usr/bin. BusyBox ps lacks lstart and
	// fails here; the callers then fall back to pids.
	for _, ps := range psPaths {
		if out, err = exec.CommandContext(ctx, ps, "-A", "-o", "pid=,ppid=,lstart=").Output(); err == nil {
			break
		}
	}
	if err != nil {
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
// snapshot it falls back to the pid alone, which can hit a reused pid but
// never leaves the hook's processes running.
func live(procs []proc, snap map[int]psEntry) []proc {
	var out []proc
	if snap == nil {
		for _, p := range procs {
			if syscall.Kill(p.pid, 0) == nil {
				out = append(out, p)
			}
		}
		return out
	}
	for _, p := range procs {
		if e, ok := snap[p.pid]; ok && p.start != "" && e.start == p.start {
			out = append(out, p)
		}
	}
	return out
}

// signalProcs signals the stage and each descendant that still runs as
// itself. The stage is our child: until cmd.Wait reaps it, its pid cannot be
// reused, so it needs no start time and is signalled even when ps fails.
func signalProcs(procs []proc, stagePid int, sig syscall.Signal, reaped func() bool) {
	for _, p := range live(procs, snapshot()) {
		if p.pid != stagePid {
			_ = syscall.Kill(p.pid, sig)
		}
	}
	if !reaped() {
		_ = syscall.Kill(stagePid, sig)
	}
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
