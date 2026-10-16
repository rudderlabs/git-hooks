package commands_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rudderlabs/git-hooks/commands"
	cleangit "github.com/rudderlabs/git-hooks/internal/clean-local-git"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

func scanLocalRepo(t *testing.T) string {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root := t.TempDir()
	dir := filepath.Join(root, "repo")
	require.NoError(t, os.Mkdir(dir, 0o700))
	for _, args := range [][]string{
		{"-C", dir, "init"},
		{"-C", dir, "config", "--local", "--add", "core.hooksPath", "first"},
		{"-C", dir, "config", "--local", "--add", "core.hooksPath", "second"},
	} {
		output, err := exec.Command("git", args...).CombinedOutput()
		require.NoError(t, err, "%s", output)
	}
	return root
}

func runScanLocal(args ...string) (string, error) {
	var output bytes.Buffer
	command := *commands.ScanLocal
	app := &cli.App{
		Commands:  []*cli.Command{&command},
		Reader:    strings.NewReader("y\n"),
		Writer:    &output,
		ErrWriter: &output,
	}
	err := app.Run(append([]string{"git-hooks", "scan-local"}, args...))
	return output.String(), err
}

func TestScanLocalFlagOrders(t *testing.T) {
	for _, name := range []string{"before", "after", "alias", "mixed"} {
		t.Run(name, func(t *testing.T) {
			root := scanLocalRepo(t)
			var args []string
			switch name {
			case "before":
				args = []string{"--auto-fix", "--yes", root}
			case "after":
				args = []string{root, "--auto-fix", "--yes", "--max-depth", "2"}
			case "alias":
				args = []string{root, "--auto-fix", "-y"}
			case "mixed":
				args = []string{"--auto-fix", root, "--yes", "--verbose"}
			}
			output, err := runScanLocal(args...)
			require.NoError(t, err, "%s", output)
			values, err := cleangit.LocalHooksPath(context.Background(), filepath.Join(root, "repo"))
			require.NoError(t, err)
			require.Empty(t, values)
			require.Contains(t, output, "removed successfully")
			require.NotContains(t, output, "(y/N)")
		})
	}
}

func TestScanLocalNonTerminalNeedsYes(t *testing.T) {
	root := scanLocalRepo(t)
	_, err := runScanLocal("--auto-fix", root)
	require.ErrorContains(t, err, "--yes")
	values, err := cleangit.LocalHooksPath(context.Background(), filepath.Join(root, "repo"))
	require.NoError(t, err)
	require.Equal(t, []string{"first", "second"}, values)
}

func TestScanLocalCleaningFailure(t *testing.T) {
	root := scanLocalRepo(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "repo", ".git", "config.lock"), nil, 0o600))
	output, err := runScanLocal("--auto-fix", "--yes", root)
	require.ErrorContains(t, err, "failed")
	require.Contains(t, output, "0 removed, 1 failed")
	values, err := cleangit.LocalHooksPath(context.Background(), filepath.Join(root, "repo"))
	require.NoError(t, err)
	require.Equal(t, []string{"first", "second"}, values)
}

func TestScanLocalShowsAllValues(t *testing.T) {
	root := scanLocalRepo(t)
	output, err := runScanLocal(root)
	require.NoError(t, err)
	require.Contains(t, output, "first")
	require.Contains(t, output, "second")
}

func TestScanLocalArgumentErrors(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		message string
	}{
		{"unknown flag", []string{"--unknown"}, "flag provided but not defined"},
		{"invalid depth", []string{"--max-depth", "bad"}, "invalid value"},
		{"extra path", []string{"extra"}, "one PATH"},
		{"end of flags", []string{"--", "--yes"}, "one PATH"},
		{"explicit no", []string{"--auto-fix", "--yes=false"}, "--yes"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := scanLocalRepo(t)
			_, err := runScanLocal(append([]string{root}, tc.args...)...)
			require.ErrorContains(t, err, tc.message)
			values, err := cleangit.LocalHooksPath(context.Background(), filepath.Join(root, "repo"))
			require.NoError(t, err)
			require.Equal(t, []string{"first", "second"}, values)
		})
	}
}

func TestScanLocalHelpAfterPath(t *testing.T) {
	root := scanLocalRepo(t)
	output, err := runScanLocal(root, "--help")
	require.NoError(t, err)
	require.Contains(t, output, "--yes")
	require.NotContains(t, output, "Scanning")
}

func TestScanLocalReportsUnreadableSeparately(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "broken", ".git"), 0o755))
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	var out bytes.Buffer
	app := &cli.App{Commands: []*cli.Command{commands.ScanLocal}, Writer: &out, ErrWriter: &out, Reader: strings.NewReader("")}
	err := app.Run([]string{"git-hooks", "scan-local", root, "--auto-fix", "--yes"})
	require.NoError(t, err, out.String())
	require.Contains(t, out.String(), "Could not read the config of 1 repository")
	require.Contains(t, out.String(), "No repositories found with local hooksPath overrides")
}
