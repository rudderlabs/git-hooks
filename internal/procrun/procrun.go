//go:build darwin || linux

package procrun

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"
	"unsafe"
)

// Run executes argv with the supplied resource limits.
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
	var tty *os.File
	if foregroundFiles(cmd) {
		tty = foregroundTTY(func() (*os.File, error) {
			return os.OpenFile("/dev/tty", os.O_RDWR, 0)
		}, ttyGroup)
	}
	if tty != nil {
		defer tty.Close()
		defer restoreTerminal(tty)
	}
	cmd, err := startStage(cmd, tty)
	if err != nil {
		return err
	}
	pid := cmd.Process.Pid
	done := make(chan error, 1)
	go func() {
		if cmd.SysProcAttr.Foreground {
			done <- waitForeground(cmd.Process, func() error { return relayStop(tty, pid) })
		} else {
			done <- cmd.Wait()
		}
	}()
	select {
	case err := <-done:
		if err != nil {
			reapGroup(pid)
		}
		return err
	case <-ctx.Done():
	}
	message := "stage canceled and was killed"
	if parent.Err() == nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		message = fmt.Sprintf("stage timed out after %s and was killed", opt.Timeout)
	}
	stopErr := stopGroup(pid, done)
	return fmt.Errorf("%s: %w", message, errors.Join(ctx.Err(), stopErr))
}

func startStage(cmd *exec.Cmd, tty *os.File) (*exec.Cmd, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if tty != nil && foregroundFiles(cmd) {
		// A failed Start consumes the command and its I/O pipes, so keep the fallback fresh.
		foreground := exec.Command(cmd.Args[0], cmd.Args[1:]...)
		foreground.Path, foreground.Err = cmd.Path, cmd.Err
		foreground.Env, foreground.Dir = cmd.Env, cmd.Dir
		foreground.Stdin, foreground.Stdout, foreground.Stderr = cmd.Stdin, cmd.Stdout, cmd.Stderr
		foreground.ExtraFiles = append(append([]*os.File(nil), cmd.ExtraFiles...), tty)
		// Foreground's ioctl precedes child-fd remapping; Ctty must name the parent's fd.
		foreground.SysProcAttr = &syscall.SysProcAttr{Setpgid: true, Foreground: true, Ctty: int(tty.Fd())}
		if err := foreground.Start(); err == nil {
			return foreground, nil
		}
		// Exec can fail after the foreground group has already changed.
		restoreTerminal(tty)
	}
	return cmd, cmd.Start()
}

func foregroundFiles(cmd *exec.Cmd) bool {
	// Wait4 cannot drain the I/O goroutines that exec.Cmd creates for non-file streams.
	in, inOK := cmd.Stdin.(*os.File)
	out, outOK := cmd.Stdout.(*os.File)
	err, errOK := cmd.Stderr.(*os.File)
	return inOK && in != nil && outOK && out != nil && errOK && err != nil
}

func waitForeground(process *os.Process, stopped func() error) error {
	// Wait4 owns reaping here; Release closes OS handles without a second wait.
	defer func() { _ = process.Release() }()
	pid := process.Pid
	var relayErr error
	for {
		var status syscall.WaitStatus
		_, err := syscall.Wait4(pid, &status, syscall.WUNTRACED, nil)
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if err != nil {
			return errors.Join(relayErr, err)
		}
		// Go's BSD WaitStatus labels SIGSTOP as continued; WCONTINUED is not requested here.
		if status.Stopped() || status.Continued() {
			if err := stopped(); err != nil {
				relayErr = errors.Join(relayErr, err)
				_ = syscall.Kill(-pid, syscall.SIGKILL)
			}
			continue
		}
		if status.Exited() || status.Signaled() {
			if status.Exited() && status.ExitStatus() == 0 {
				return relayErr
			}
			exitErr := &ExitError{Code: status.ExitStatus()}
			if status.Signaled() {
				exitErr.Signal = status.Signal()
			}
			return errors.Join(relayErr, exitErr)
		}
	}
}

func relayStop(tty *os.File, pid int) error {
	if err := setTerminalGroup(tty, syscall.Getpgrp()); err != nil {
		return err
	}
	// The shell must see the whole git job stop before it can restore its prompt.
	if err := syscall.Kill(0, syscall.SIGTSTP); err != nil {
		return err
	}
	if err := setTerminalGroup(tty, pid); err != nil {
		return err
	}
	return syscall.Kill(-pid, syscall.SIGCONT)
}

func foregroundTTY(open func() (*os.File, error), group func(*os.File) (int, error)) *os.File {
	tty, err := open()
	if err != nil {
		return nil
	}
	pgrp, err := group(tty)
	if err != nil || pgrp != syscall.Getpgrp() {
		_ = tty.Close()
		return nil
	}
	return tty
}

func ttyGroup(tty *os.File) (int, error) {
	var group int32
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, tty.Fd(), syscall.TIOCGPGRP, uintptr(unsafe.Pointer(&group)))
	if errno != 0 {
		return 0, errno
	}
	return int(group), nil
}

func restoreTerminal(tty *os.File) {
	_ = setTerminalGroup(tty, syscall.Getpgrp())
}

func setTerminalGroup(tty *os.File, pid int) error {
	// The parent is now in the background, where TIOCSPGRP would otherwise stop it.
	ignored := signal.Ignored(syscall.SIGTTOU)
	signal.Ignore(syscall.SIGTTOU)
	if !ignored {
		defer signal.Reset(syscall.SIGTTOU)
	}
	group := int32(pid)
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, tty.Fd(), syscall.TIOCSPGRP, uintptr(unsafe.Pointer(&group)))
	if errno != 0 {
		return errno
	}
	return nil
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
