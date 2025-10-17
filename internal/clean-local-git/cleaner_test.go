package cleangit_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	cleangit "github.com/lvrach/git-hooks/internal/clean-local-git"
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

	// Should not find invalid repos
	repos, err := cleangit.Scan(context.Background(), tempDir, 1)
	require.NoError(t, err)
	require.Len(t, repos, 0)
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

	summary := cleangit.Clean(context.Background(), []cleangit.Repository{repo})
	require.Equal(t, 1, summary.RepositoriesWithConfig)
	// Should handle gracefully (git config --unset returns exit code 5 for non-existent key)
	require.Len(t, summary.Results, 1)
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
