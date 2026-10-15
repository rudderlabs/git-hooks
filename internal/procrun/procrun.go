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
	"unsafe"
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
	interactive := inForegroundJob()
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

// inForegroundJob reports whether we own the controlling terminal's foreground.
func inForegroundJob() bool {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return false
	}
	defer tty.Close()
	group, err := ttyGroup(tty)
	return err == nil && group == syscall.Getpgrp()
}

func ttyGroup(tty *os.File) (int, error) {
	var group int32
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, tty.Fd(), syscall.TIOCGPGRP, uintptr(unsafe.Pointer(&group)))
	if errno != 0 {
		return 0, errno
	}
	return int(group), nil
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
	tree := descendants(pid)
	signalAll(append([]int{pid}, tree...), syscall.SIGTERM)
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case err := <-done:
		// Children that outlived the stage were captured before it exited.
		signalAll(tree, syscall.SIGKILL)
		return err
	case <-timer.C:
	}
	signalAll(append([]int{pid}, append(tree, descendants(pid)...)...), syscall.SIGKILL)
	return <-done
}

func signalAll(pids []int, sig syscall.Signal) {
	for _, p := range pids {
		_ = syscall.Kill(p, sig)
	}
}

// descendants lists every process below pid, using ps because neither
// darwin nor linux offers a portable child list in the standard library.
func descendants(pid int) []int {
	out, err := exec.Command("ps", "-A", "-o", "pid=,ppid=").Output()
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
