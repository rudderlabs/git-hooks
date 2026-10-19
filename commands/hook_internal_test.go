package commands

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStandardHookPathIgnoresHooksPath(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	repo := t.TempDir()
	git := func(dir string, args ...string) {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
		require.NoError(t, err, string(out))
	}
	git(repo, "init", "-q")
	git(repo, "-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "--allow-empty", "-m", "init")
	// The global install points core.hooksPath at the wrapper directory.
	t.Setenv("GIT_CONFIG_PARAMETERS", "'core.hookspath'='"+t.TempDir()+"'")
	wt := filepath.Join(t.TempDir(), "wt")
	git(repo, "worktree", "add", "-q", wt)

	common, err := filepath.EvalSymlinks(filepath.Join(repo, ".git"))
	require.NoError(t, err)
	for _, dir := range []string{repo, wt} {
		t.Chdir(dir)
		path, ok, err := standardHookPath(context.Background(), "pre-commit")
		require.NoError(t, err)
		require.True(t, ok)
		got, err := filepath.EvalSymlinks(filepath.Dir(path))
		require.NoError(t, err)
		require.Equal(t, filepath.Join(common, "hooks"), got)
	}
}
