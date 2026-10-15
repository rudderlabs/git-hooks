//go:build darwin || linux

package procrun

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

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
