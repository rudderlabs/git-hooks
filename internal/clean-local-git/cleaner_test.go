package cleangit_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	cleangit "github.com/rudderlabs/git-hooks/internal/clean-local-git"
	"github.com/stretchr/testify/require"
)

// Unit Tests

func TestGetHooksPath_WithValue(t *testing.T) {
	t.Log("Testing git config reads hooksPath correctly")

	tempDir := t.TempDir()
	setupGitRepo(t, tempDir)
	setGitConfig(t, tempDir, "core.hooksPath", ".custom-hooks")

	// Scan will internally call getHooksPath
	repos, err := cleangit.Scan(context.Background(), tempDir, 1)
	require.NoError(t, err)
	require.Len(t, repos, 1)
	require.Equal(t, ".custom-hooks", repos[0].CustomHooksPath)
}

func TestGetHooksPath_WithoutValue(t *testing.T) {
	t.Log("Testing git config handles missing hooksPath")

	tempDir := t.TempDir()
	setupGitRepo(t, tempDir)

	// Should not find repos without hooksPath
	repos, err := cleangit.Scan(context.Background(), tempDir, 1)
	require.NoError(t, err)
	require.Len(t, repos, 0)
}

func TestGetHooksPath_InvalidRepo(t *testing.T) {
	t.Log("Testing git config handles non-repo directory")

	tempDir := t.TempDir()
	// Create .git directory but don't initialize it properly
	err := os.Mkdir(filepath.Join(tempDir, ".git"), 0o755)
	require.NoError(t, err)

	// A broken repository is reported without stopping the scan of its siblings.
	repos, err := cleangit.Scan(context.Background(), tempDir, 1)
	require.NoError(t, err)
	require.Len(t, repos, 1)
	require.Error(t, repos[0].Err)

	summary := cleangit.Clean(context.Background(), repos)
	require.Zero(t, summary.ConfigsRemoved)
	require.Error(t, summary.Results[0].Error)
}

func TestRemoveHooksPath_Success(t *testing.T) {
	t.Log("Testing removal of hooksPath")

	t.Log("Setting up git repository with hooksPath")
	tempDir := t.TempDir()
	setupGitRepo(t, tempDir)
	setGitConfig(t, tempDir, "core.hooksPath", ".husky")

	t.Log("Verifying hooksPath is set")
	value, exists := getGitConfig(t, tempDir, "core.hooksPath")
	require.True(t, exists)
	require.Equal(t, ".husky", value)

	t.Log("Scanning for repositories with hooksPath")
	repos, err := cleangit.Scan(context.Background(), tempDir, 1)
	require.NoError(t, err)
	require.Len(t, repos, 1)

	t.Log("Cleaning repositories")
	summary := cleangit.Clean(context.Background(), repos)
	require.Equal(t, 1, summary.ConfigsRemoved)
	require.Len(t, summary.Results, 1)
	require.True(t, summary.Results[0].Removed)
	require.NoError(t, summary.Results[0].Error)

	t.Log("Verifying hooksPath was removed")
	_, exists = getGitConfig(t, tempDir, "core.hooksPath")
	require.False(t, exists)
}

func TestRemoveHooksPath_AlreadyRemoved(t *testing.T) {
	t.Log("Testing removal when hooksPath doesn't exist")

	tempDir := t.TempDir()
	setupGitRepo(t, tempDir)

	// Manually create a repo struct with hooksPath (simulating stale data)
	repo := cleangit.Repository{
		Path:            tempDir,
		ConfigPath:      filepath.Join(tempDir, ".git", "config"),
		HasCustomHooks:  true,
		CustomHooksPath: ".husky",
	}

	// The goal state already holds, so stale scan data is not a failure.
	summary := cleangit.Clean(context.Background(), []cleangit.Repository{repo})
	require.Equal(t, 1, summary.RepositoriesWithConfig)
	require.Equal(t, 1, summary.ConfigsRemoved)
	require.Len(t, summary.Results, 1)
	require.NoError(t, summary.Results[0].Error)
}

func TestRemoveHooksPath_PreservesOtherCoreSettings(t *testing.T) {
	t.Log("Testing that other core.* settings are preserved")

	t.Log("Setting up git repository with multiple core settings")
	tempDir := t.TempDir()
	setupGitRepo(t, tempDir)
	setGitConfig(t, tempDir, "core.hooksPath", ".husky")
	setGitConfig(t, tempDir, "core.autocrlf", "input")
	setGitConfig(t, tempDir, "core.ignorecase", "false")

	t.Log("Scanning and cleaning repositories")
	repos, err := cleangit.Scan(context.Background(), tempDir, 1)
	require.NoError(t, err)

	summary := cleangit.Clean(context.Background(), repos)
	require.Equal(t, 1, summary.ConfigsRemoved)

	t.Log("Verifying hooksPath was removed")
	_, exists := getGitConfig(t, tempDir, "core.hooksPath")
	require.False(t, exists)

	t.Log("Verifying other core settings were preserved")
	value, exists := getGitConfig(t, tempDir, "core.autocrlf")
	require.True(t, exists)
	require.Equal(t, "input", value)

	value, exists = getGitConfig(t, tempDir, "core.ignorecase")
	require.True(t, exists)
	require.Equal(t, "false", value)
}

// Integration Tests

func TestScan_EmptyDirectory(t *testing.T) {
	t.Log("Testing Scan with empty directory")

	tempDir := t.TempDir()

	repos, err := cleangit.Scan(context.Background(), tempDir, 5)
	require.NoError(t, err)
	require.Len(t, repos, 0)
}

func TestScan_WithCustomHooksPath(t *testing.T) {
	t.Log("Testing Scan finds repository with custom hooksPath")

	tempDir := t.TempDir()

	// Create a real Git repository with custom hooksPath
	setupGitRepo(t, tempDir)
	setGitConfig(t, tempDir, "core.hooksPath", ".husky")

	repos, err := cleangit.Scan(context.Background(), tempDir, 5)
	require.NoError(t, err)
	require.Len(t, repos, 1)

	repo := repos[0]
	require.True(t, repo.HasCustomHooks)
	require.Equal(t, ".husky", repo.CustomHooksPath)
	require.Equal(t, tempDir, repo.Path)
}

func TestScan_WithoutCustomHooksPath(t *testing.T) {
	t.Log("Testing Scan ignores repository without custom hooksPath")

	tempDir := t.TempDir()

	// Create a real Git repository without custom hooksPath
	setupGitRepo(t, tempDir)

	repos, err := cleangit.Scan(context.Background(), tempDir, 5)
	require.NoError(t, err)
	require.Len(t, repos, 0)
}

func TestScan_NestedRepositories(t *testing.T) {
	t.Log("Testing Scan finds nested repositories")

	tempDir := t.TempDir()

	// Create multiple nested repos
	repo1 := filepath.Join(tempDir, "repo1")
	repo2 := filepath.Join(tempDir, "subdir", "repo2")

	for _, repoPath := range []string{repo1, repo2} {
		setupGitRepo(t, repoPath)
		setGitConfig(t, repoPath, "core.hooksPath", ".husky")
	}

	repos, err := cleangit.Scan(context.Background(), tempDir, 10)
	require.NoError(t, err)
	require.Len(t, repos, 2)
}

func TestScan_MaxDepth(t *testing.T) {
	t.Log("Testing Scan respects maxDepth")

	tempDir := t.TempDir()

	// Create a deeply nested repo
	deepRepo := filepath.Join(tempDir, "l1", "l2", "l3", "l4", "l5", "l6")
	setupGitRepo(t, deepRepo)
	setGitConfig(t, deepRepo, "core.hooksPath", ".husky")

	// Should not find with low depth
	repos, err := cleangit.Scan(context.Background(), tempDir, 3)
	require.NoError(t, err)
	require.Len(t, repos, 0)

	// Should find with sufficient depth
	repos, err = cleangit.Scan(context.Background(), tempDir, 10)
	require.NoError(t, err)
	require.Len(t, repos, 1)
}

func TestClean_ActualRemoval(t *testing.T) {
	t.Log("Testing Clean actually removes hooksPath")

	tempDir := t.TempDir()

	// Create a real Git repository
	setupGitRepo(t, tempDir)
	setGitConfig(t, tempDir, "core.hooksPath", ".husky")

	repos, err := cleangit.Scan(context.Background(), tempDir, 5)
	require.NoError(t, err)

	// Clean for real
	summary := cleangit.Clean(context.Background(), repos)

	require.Equal(t, 1, summary.ConfigsRemoved)
	require.Len(t, summary.Results, 1)

	result := summary.Results[0]
	require.True(t, result.Removed)
	require.NoError(t, result.Error)
	require.Equal(t, ".husky", result.PreviousPath)

	// Verify hooksPath was removed using git config
	value, exists := getGitConfig(t, tempDir, "core.hooksPath")
	require.False(t, exists, "hooksPath should have been removed, but still has value: %s", value)
}

func TestClean_MultipleRepositories(t *testing.T) {
	t.Log("Testing Clean handles multiple repositories")

	tempDir := t.TempDir()

	// Create multiple repos
	for i := 1; i <= 3; i++ {
		repoPath := filepath.Join(tempDir, filepath.FromSlash("repo"), string(rune('0'+i)))
		setupGitRepo(t, repoPath)
		setGitConfig(t, repoPath, "core.hooksPath", ".husky")
	}

	repos, err := cleangit.Scan(context.Background(), tempDir, 10)
	require.NoError(t, err)

	summary := cleangit.Clean(context.Background(), repos)

	require.Equal(t, 3, summary.RepositoriesWithConfig)
	require.Equal(t, 3, summary.ConfigsRemoved)
	require.Len(t, summary.Results, 3)
}

func TestScan_ContextCancellation(t *testing.T) {
	t.Log("Testing Scan respects context cancellation")

	tempDir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := cleangit.Scan(ctx, tempDir, 5)
	require.Error(t, err)
}

func TestScan_ExcludedDirectories(t *testing.T) {
	t.Log("Testing Scan skips excluded directories")

	tempDir := t.TempDir()

	// Create repos in excluded directories
	excludedDirs := []string{"node_modules", "venv", ".venv", "vendor"}
	for _, dir := range excludedDirs {
		repoPath := filepath.Join(tempDir, dir, "fake-repo")
		setupGitRepo(t, repoPath)
		setGitConfig(t, repoPath, "core.hooksPath", ".husky")
	}

	// Create a valid repo
	validRepo := filepath.Join(tempDir, "valid-repo")
	setupGitRepo(t, validRepo)
	setGitConfig(t, validRepo, "core.hooksPath", ".husky")

	repos, err := cleangit.Scan(context.Background(), tempDir, 10)
	require.NoError(t, err)

	// Should only find the valid repo, not the ones in excluded dirs
	require.Len(t, repos, 1)
	require.Contains(t, repos[0].Path, "valid-repo")
}

func TestClean_PreservesOtherConfig(t *testing.T) {
	t.Log("Testing Clean preserves other git config sections")

	tempDir := t.TempDir()

	// Create real git repo and set multiple config values
	setupGitRepo(t, tempDir)
	setGitConfig(t, tempDir, "core.hooksPath", ".husky")
	setGitConfig(t, tempDir, "user.name", "Test User")
	setGitConfig(t, tempDir, "user.email", "test@example.com")
	setGitConfig(t, tempDir, "remote.origin.url", "git@github.com:example/repo.git")

	repos, err := cleangit.Scan(context.Background(), tempDir, 5)
	require.NoError(t, err)

	// Clean for real
	summary := cleangit.Clean(context.Background(), repos)
	require.Equal(t, 1, summary.ConfigsRemoved)

	// Verify hooksPath was removed
	value, exists := getGitConfig(t, tempDir, "core.hooksPath")
	require.False(t, exists, "hooksPath should have been removed, but still has value: %s", value)

	// Verify other settings are preserved
	value, exists = getGitConfig(t, tempDir, "user.name")
	require.True(t, exists)
	require.Equal(t, "Test User", value)

	value, exists = getGitConfig(t, tempDir, "user.email")
	require.True(t, exists)
	require.Equal(t, "test@example.com", value)

	value, exists = getGitConfig(t, tempDir, "remote.origin.url")
	require.True(t, exists)
	require.Equal(t, "git@github.com:example/repo.git", value)
}

func TestClean_EmptyResults(t *testing.T) {
	t.Log("Testing Clean with no repositories")

	summary := cleangit.Clean(context.Background(), []cleangit.Repository{})

	require.Equal(t, 0, summary.RepositoriesWithConfig)
	require.Equal(t, 0, summary.ConfigsRemoved)
	require.Len(t, summary.Results, 0)
}

func TestScan_NonGitDirectory(t *testing.T) {
	t.Log("Testing Scan with non-git directories")

	tempDir := t.TempDir()

	// Create some random directories without .git
	for _, dir := range []string{"src", "docs", "build"} {
		err := os.MkdirAll(filepath.Join(tempDir, dir), 0o755)
		require.NoError(t, err)
	}

	repos, err := cleangit.Scan(context.Background(), tempDir, 5)
	require.NoError(t, err)
	require.Len(t, repos, 0)
}

// Helper functions

// setupGitRepo creates a real git repository using git init
func setupGitRepo(t *testing.T, dir string) {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")

	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("Failed to create directory: %v", err)
	}

	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to init git repo: %v\nOutput: %s", err, output)
	}
}

// setGitConfig sets a git config value using git config command
func setGitConfig(t *testing.T, repoPath, key, value string) {
	t.Helper()

	cmd := exec.Command("git", "config", "--local", key, value)
	cmd.Dir = repoPath
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to set git config %s=%s: %v\nOutput: %s", key, value, err, output)
	}
}

// getGitConfig gets a git config value using git config command
func getGitConfig(t *testing.T, repoPath, key string) (string, bool) {
	t.Helper()

	cmd := exec.Command("git", "config", "--local", key)
	cmd.Dir = repoPath
	output, err := cmd.Output()
	if err != nil {
		// Exit code 1 means key doesn't exist
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return "", false
		}
		t.Fatalf("Failed to get git config %s: %v", key, err)
	}

	value := string(output)
	// Trim trailing newline
	if len(value) > 0 && value[len(value)-1] == '\n' {
		value = value[:len(value)-1]
	}

	return value, value != ""
}

func TestCleanMultipleHooksPaths(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	for _, value := range []string{"first", "second"} {
		cmd := exec.Command("git", "-C", dir, "config", "--local", "--add", "core.hooksPath", value)
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", output)
	}
	repos, err := cleangit.Scan(context.Background(), dir, 1)
	require.NoError(t, err)
	require.Len(t, repos, 1)
	require.Contains(t, repos[0].CustomHooksPath, "first")
	require.Contains(t, repos[0].CustomHooksPath, "second")
	summary := cleangit.Clean(context.Background(), repos)
	require.Equal(t, 1, summary.ConfigsRemoved)
	require.NoError(t, summary.Results[0].Error)
	_, exists := getGitConfig(t, dir, "core.hooksPath")
	require.False(t, exists)
}

func TestCleanLockedConfig(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	setGitConfig(t, dir, "core.hooksPath", "first")
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "config.lock"), nil, 0o600))
	summary := cleangit.Clean(context.Background(), []cleangit.Repository{{Path: dir}})
	require.Zero(t, summary.ConfigsRemoved)
	require.Len(t, summary.Results, 1)
	require.False(t, summary.Results[0].Removed)
	require.Error(t, summary.Results[0].Error)
	value, exists := getGitConfig(t, dir, "core.hooksPath")
	require.True(t, exists)
	require.Equal(t, "first", value)
}

func TestLocalHooksPath(t *testing.T) {
	for _, tc := range []struct {
		name     string
		local    []string
		worktree bool
		want     []string
	}{
		{name: "none"},
		{name: "one", local: []string{"first"}, want: []string{"first"}},
		{name: "two", local: []string{"first", "second"}, want: []string{"first", "second"}},
		{name: "empty value", local: []string{""}, want: []string{""}},
		{name: "worktree", local: []string{"first"}, worktree: true, want: []string{"first", "worktree"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			setupGitRepo(t, dir)
			global := filepath.Join(t.TempDir(), "global")
			require.NoError(t, os.WriteFile(global, []byte("[core]\n hooksPath = global\n"), 0o600))
			t.Setenv("GIT_CONFIG_GLOBAL", global)
			for _, value := range tc.local {
				output, err := exec.Command("git", "-C", dir, "config", "--local", "--add", "core.hooksPath", value).CombinedOutput()
				require.NoError(t, err, "%s", output)
			}
			if tc.worktree {
				setGitConfig(t, dir, "extensions.worktreeConfig", "true")
				output, err := exec.Command("git", "-C", dir, "config", "--worktree", "core.hooksPath", "worktree").CombinedOutput()
				require.NoError(t, err, "%s", output)
			}
			values, err := cleangit.LocalHooksPath(context.Background(), dir)
			require.NoError(t, err)
			require.Equal(t, tc.want, values)
		})
	}
}

func TestLocalHooksPathErrors(t *testing.T) {
	t.Parallel()
	_, err := cleangit.LocalHooksPath(context.Background(), t.TempDir())
	require.Error(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = cleangit.LocalHooksPath(ctx, t.TempDir())
	require.ErrorIs(t, err, context.Canceled)
}

func TestLocalHooksPathConfigErrors(t *testing.T) {
	for _, tc := range []struct {
		name      string
		extension string
		config    string
	}{
		{"invalid extension", "invalid", ""},
		{"invalid worktree config", "true", "[broken"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			setupGitRepo(t, dir)
			setGitConfig(t, dir, "extensions.worktreeConfig", tc.extension)
			if tc.config != "" {
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "config.worktree"), []byte(tc.config), 0o600))
			}
			_, err := cleangit.LocalHooksPath(context.Background(), dir)
			require.Error(t, err)
			require.Contains(t, err.Error(), dir)
		})
	}
}

func TestCleanContinuesAfterFailure(t *testing.T) {
	root := t.TempDir()
	var repos []cleangit.Repository
	for _, name := range []string{"locked", "writable"} {
		dir := filepath.Join(root, name)
		setupGitRepo(t, dir)
		setGitConfig(t, dir, "core.hooksPath", "hooks")
		repos = append(repos, cleangit.Repository{Path: dir})
	}
	require.NoError(t, os.WriteFile(filepath.Join(repos[0].Path, ".git", "config.lock"), nil, 0o600))
	summary := cleangit.Clean(context.Background(), repos)
	require.Equal(t, 1, summary.ConfigsRemoved)
	require.Len(t, summary.Results, 2)
	require.Error(t, summary.Results[0].Error)
	require.False(t, summary.Results[0].Removed)
	require.NoError(t, summary.Results[1].Error)
	require.True(t, summary.Results[1].Removed)
}

func TestScanContinuesPastBrokenRepo(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "broken", ".git"), 0o755))
	good := filepath.Join(root, "good")
	require.NoError(t, os.Mkdir(good, 0o755))
	setupGitRepo(t, good)
	setGitConfig(t, good, "core.hooksPath", ".husky")

	repos, err := cleangit.Scan(context.Background(), root, 2)
	require.NoError(t, err)
	require.Len(t, repos, 2)
	byPath := map[string]cleangit.Repository{}
	for _, r := range repos {
		byPath[filepath.Base(r.Path)] = r
	}
	require.Error(t, byPath["broken"].Err)
	require.NoError(t, byPath["good"].Err)
	require.Equal(t, ".husky", byPath["good"].CustomHooksPath)
}

func TestCleanWorktreeConfig(t *testing.T) {
	dir := t.TempDir()
	setupGitRepo(t, dir)
	setGitConfig(t, dir, "extensions.worktreeConfig", "true")
	cmd := exec.Command("git", "-C", dir, "config", "--worktree", "core.hooksPath", ".husky/_")
	require.NoError(t, cmd.Run())

	repos, err := cleangit.Scan(context.Background(), dir, 1)
	require.NoError(t, err)
	require.Len(t, repos, 1)
	require.Equal(t, ".husky/_", repos[0].CustomHooksPath)

	summary := cleangit.Clean(context.Background(), repos)
	require.NoError(t, summary.Results[0].Error)
	values, err := cleangit.LocalHooksPath(context.Background(), dir)
	require.NoError(t, err)
	require.Empty(t, values)
}

func TestBrokenNestedRepoNeverTouchesParent(t *testing.T) {
	parent := t.TempDir()
	setupGitRepo(t, parent)
	setGitConfig(t, parent, "core.hooksPath", ".husky")
	require.NoError(t, os.MkdirAll(filepath.Join(parent, "child", ".git"), 0o755))

	repos, err := cleangit.Scan(context.Background(), filepath.Join(parent, "child"), 1)
	require.NoError(t, err)
	require.Len(t, repos, 1)
	require.Error(t, repos[0].Err, "the broken child must not borrow the parent's config")

	cleangit.Clean(context.Background(), repos)
	value, exists := getGitConfig(t, parent, "core.hooksPath")
	require.True(t, exists)
	require.Equal(t, ".husky", value)
}
