//go:build darwin || linux

package procrun

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestForegroundRequiresFileIO(t *testing.T) {
	t.Parallel()
	f, err := os.Create(filepath.Join(t.TempDir(), "io"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })
	for _, tc := range []struct {
		name           string
		stdin          io.Reader
		stdout, stderr io.Writer
		want           bool
	}{
		{"files", f, f, f, true},
		{"reader", strings.NewReader("input"), f, f, false},
		{"stdout writer", f, &bytes.Buffer{}, f, false},
		{"stderr writer", f, f, &bytes.Buffer{}, false},
		{"nil stdin", nil, f, f, false},
		{"nil stdout", f, nil, f, false},
		{"nil stderr", f, f, nil, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cmd := exec.Command("sh", "-c", "exit 0")
			cmd.Stdin, cmd.Stdout, cmd.Stderr = tc.stdin, tc.stdout, tc.stderr
			require.Equal(t, tc.want, foregroundFiles(cmd))
		})
	}
}

func TestWaitForegroundExit(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		command string
		code    int
		signal  syscall.Signal
	}{
		{"exit 0", 0, 0},
		{"exit 7", 7, 0},
		{"kill -INT $$", -1, syscall.SIGINT},
	} {
		t.Run(tc.command, func(t *testing.T) {
			t.Parallel()
			cmd := startWaitStage(t, tc.command)
			done := make(chan error, 1)
			go func() { done <- waitForeground(cmd.Process, func() error { return errors.New("unexpected stop") }) }()
			err := foregroundResult(t, done)
			if tc.code == 0 {
				require.NoError(t, err)
				return
			}
			var exitErr *ExitError
			require.ErrorAs(t, err, &exitErr)
			require.Equal(t, tc.code, exitErr.ExitCode())
			require.Equal(t, tc.signal, exitErr.Signal)
			require.NotEmpty(t, exitErr.Error())
		})
	}
}

func TestWaitForegroundStopsAndContinues(t *testing.T) {
	t.Parallel()
	cmd := startWaitStage(t, "kill -TSTP $$; kill -STOP $$; exit 7")
	pid := cmd.Process.Pid
	stops := 0
	done := make(chan error, 1)
	// The real shell exercises WUNTRACED without stopping the test runner's own process group.
	go func() {
		done <- waitForeground(cmd.Process, func() error {
			stops++
			return syscall.Kill(-pid, syscall.SIGCONT)
		})
	}()
	var exitErr *ExitError
	require.ErrorAs(t, foregroundResult(t, done), &exitErr)
	require.Equal(t, 7, exitErr.ExitCode())
	require.Equal(t, 2, stops)
}

func TestWaitForegroundRelayFailure(t *testing.T) {
	t.Parallel()
	cmd := startWaitStage(t, "kill -STOP $$; exit 0")
	done := make(chan error, 1)
	go func() { done <- waitForeground(cmd.Process, func() error { return syscall.ENOTTY }) }()
	require.ErrorIs(t, foregroundResult(t, done), syscall.ENOTTY)
}

func TestStopGroupWithForegroundWait(t *testing.T) {
	t.Parallel()
	cmd := startWaitStage(t, "exec sleep 300")
	pid := cmd.Process.Pid
	done := make(chan error, 1)
	go func() { done <- waitForeground(cmd.Process, func() error { return errors.New("unexpected stop") }) }()
	stopped := make(chan error, 1)
	go func() { stopped <- stopGroup(pid, done) }()
	var exitErr *ExitError
	require.ErrorAs(t, foregroundResult(t, stopped), &exitErr)
	require.Equal(t, syscall.SIGTERM, exitErr.Signal)
	require.ErrorIs(t, syscall.Kill(pid, 0), syscall.ESRCH)
}

func startWaitStage(t *testing.T, command string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = t.TempDir()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	f, err := os.Create(filepath.Join(cmd.Dir, "io"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = f.Close() })
	cmd.Stdin, cmd.Stdout, cmd.Stderr = f, f, f
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	t.Cleanup(func() { _ = syscall.Kill(-pid, syscall.SIGKILL) })
	return cmd
}

func foregroundResult(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(3 * time.Second):
		t.Fatal("foreground wait did not finish")
		return nil
	}
}

func TestStageTTYFallback(t *testing.T) {
	// Terminal restoration changes the process-wide SIGTTOU disposition.
	dir := t.TempDir()
	f, err := os.Create(filepath.Join(dir, "not-a-tty"))
	require.NoError(t, err)
	defer f.Close()
	var out, stderr bytes.Buffer
	cmd := exec.Command("sh", "-c", `read -r value; printf '%s:%s' "$CUSTOM" "$value"; printf problem >&2; echo once >> started; exit 3`)
	cmd.Dir, cmd.Env = dir, []string{"CUSTOM=custom"}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = strings.NewReader("input\n"), &out, &stderr
	started, err := startStage(cmd, f)
	require.NoError(t, err)
	var exitErr *exec.ExitError
	require.ErrorAs(t, started.Wait(), &exitErr)
	require.Equal(t, 3, exitErr.ExitCode())
	require.Equal(t, "custom:input", out.String())
	require.Equal(t, "problem", stderr.String())
	data, err := os.ReadFile(filepath.Join(dir, "started"))
	require.NoError(t, err)
	require.Equal(t, "once\n", string(data))
}

func TestForegroundTTY(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name  string
		group int
		err   error
		want  bool
	}{
		{"foreground", syscall.Getpgrp(), nil, true},
		{"background", syscall.Getpgrp() + 1, nil, false},
		{"ioctl failure", syscall.Getpgrp(), syscall.ENOTTY, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f, err := os.Create(filepath.Join(t.TempDir(), "tty"))
			require.NoError(t, err)
			t.Cleanup(func() { _ = f.Close() })
			got := foregroundTTY(func() (*os.File, error) { return f, nil }, func(file *os.File) (int, error) {
				require.Same(t, f, file)
				return tc.group, tc.err
			})
			if tc.want {
				require.Same(t, f, got)
				_, err = f.Stat()
				require.NoError(t, err)
			} else {
				require.Nil(t, got)
				_, err = f.Stat()
				require.ErrorIs(t, err, os.ErrClosed)
			}
		})
	}
	t.Run("open failure", func(t *testing.T) {
		t.Parallel()
		require.Nil(t, foregroundTTY(func() (*os.File, error) { return nil, syscall.ENXIO }, func(*os.File) (int, error) {
			t.Fatal("must not query an unavailable tty")
			return 0, nil
		}))
	})
}
