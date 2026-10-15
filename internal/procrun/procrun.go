//go:build darwin || linux

// Package procrun limits the resources used by repository hook stages.
package procrun

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

//go:embed cpucap.cjs
var cpuPreload []byte

// Options controls a stage's resource limits and execution environment.
type Options struct {
	Timeout time.Duration
	CPUs    int
	Nice    int
	Env     []string
	Dir     string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
}

// DefaultOptions reads hook limits from the environment, falling back on invalid values.
func DefaultOptions() Options {
	opt := Options{Timeout: 10 * time.Minute, CPUs: 2, Nice: 10}
	if d, err := time.ParseDuration(os.Getenv("GIT_HOOKS_TIMEOUT")); err == nil && d > 0 {
		opt.Timeout = d
	}
	if n, err := strconv.Atoi(os.Getenv("GIT_HOOKS_CPUS")); err == nil && n >= 0 {
		opt.CPUs = n
	}
	if n, err := strconv.Atoi(os.Getenv("GIT_HOOKS_NICE")); err == nil && n >= 0 {
		opt.Nice = n
	}
	return opt
}

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
	if opt.CPUs > 0 {
		path, err := preloadPath()
		if err != nil {
			return fmt.Errorf("prepare CPU cap: %w", err)
		}
		cmd.Env = append(cmd.Environ(), "GIT_HOOKS_CPUS="+strconv.Itoa(opt.CPUs))
		if strings.Contains(path, " ") {
			if opt.Stderr != nil {
				fmt.Fprintln(opt.Stderr, "git-hooks: warning: CPU cap skipped because the cache path contains spaces")
			}
		} else {
			if err := writePreload(path); err != nil {
				return fmt.Errorf("prepare CPU cap: %w", err)
			}
			env := cmd.Environ()
			nodeOptions := ""
			for _, entry := range env {
				if value, ok := strings.CutPrefix(entry, "NODE_OPTIONS="); ok {
					nodeOptions = value
				}
			}
			cmd.Env = append(env, "NODE_OPTIONS="+nodeOptions+" --require="+path)
		}
	}
	tty := foregroundTTY(func() (*os.File, error) {
		return os.OpenFile("/dev/tty", os.O_RDWR, 0)
	}, ttyGroup)
	if tty != nil {
		defer tty.Close()
		defer restoreTerminal(tty)
	}
	cmd, err := startStage(cmd, tty)
	if err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		reapGroup(cmd.Process.Pid)
		return err
	case <-ctx.Done():
	}
	message := "stage canceled and was killed"
	if parent.Err() == nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		message = fmt.Sprintf("stage timed out after %s and was killed", opt.Timeout)
	}
	stopErr := stopGroup(cmd.Process.Pid, done)
	return fmt.Errorf("%s: %w", message, errors.Join(ctx.Err(), stopErr))
}

func startStage(cmd *exec.Cmd, tty *os.File) (*exec.Cmd, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if tty != nil {
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
	// The parent is now in the background, where TIOCSPGRP would otherwise stop it.
	ignored := signal.Ignored(syscall.SIGTTOU)
	signal.Ignore(syscall.SIGTTOU)
	if !ignored {
		defer signal.Reset(syscall.SIGTTOU)
	}
	group := int32(syscall.Getpgrp())
	_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, tty.Fd(), syscall.TIOCSPGRP, uintptr(unsafe.Pointer(&group)))
}

func preloadPath() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(cpuPreload)
	return filepath.Join(cache, "git-hooks", fmt.Sprintf("cpucap-%x.cjs", hash[:8])), nil
}

func writePreload(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".cpucap-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(cpuPreload); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(0644); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// Concurrent hook stages must never see a partially written preload.
	return os.Rename(f.Name(), path)
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

// reapGroup kills what a finished stage left behind: background jobs of a
// non-interactive shell ignore SIGINT, so Ctrl-C alone leaves them running.
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
