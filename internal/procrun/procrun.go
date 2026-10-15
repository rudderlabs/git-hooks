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
	var waitErr error
	exited := false
	// reaped reports whether cmd.Wait has returned. After that the stage pid
	// may belong to another process, so it is checked right before each signal.
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
	var tree []int
	if !reaped() {
		tree = descendants(pid)
		if !reaped() {
			_ = syscall.Kill(pid, syscall.SIGTERM)
		}
	}
	signalAll(tree, syscall.SIGTERM)
	grace := time.NewTimer(5 * time.Second)
	defer grace.Stop()
	tick := time.NewTicker(50 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-tick.C:
		case <-grace.C:
			// Children the stage started during the grace period are below it
			// too. Scan only while the stage is unreaped, or a reused pid's
			// children would join the list; check again right before the signal,
			// because ps can take a while. Signalling by pid still leaves a
			// window of microseconds that only a pidfd could close.
			if !reaped() {
				scanned := descendants(pid)
				// If the stage was reaped during the scan, the pid may already
				// be another process's, and so may its children: drop the scan.
				if !reaped() {
					tree = append(tree, scanned...)
					_ = syscall.Kill(pid, syscall.SIGKILL)
				}
			}
			signalAll(alive(tree), syscall.SIGKILL)
			if !exited {
				waitErr = <-done
			}
			return waitErr
		}
		if reaped() && len(alive(tree)) == 0 {
			return waitErr
		}
	}
}

func alive(pids []int) []int {
	var out []int
	for _, p := range pids {
		if syscall.Kill(p, 0) == nil {
			out = append(out, p)
		}
	}
	return out
}

func signalAll(pids []int, sig syscall.Signal) {
	for _, p := range pids {
		_ = syscall.Kill(p, sig)
	}
}

// descendants lists every process below pid, using ps because neither
// darwin nor linux offers a portable child list in the standard library.
// The absolute path and the deadline keep a broken PATH from hanging a kill.
func descendants(pid int) []int {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/bin/ps", "-A", "-o", "pid=,ppid=").Output()
	if err != nil {
		return nil
	}
	children := map[int][]int{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		child, err1 := strconv.Atoi(fields[0])
		parent, err2 := strconv.Atoi(fields[1])
		if err1 == nil && err2 == nil {
			children[parent] = append(children[parent], child)
		}
	}
	var all []int
	queue := []int{pid}
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		for _, c := range children[next] {
			all = append(all, c)
			queue = append(queue, c)
		}
	}
	return all
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
