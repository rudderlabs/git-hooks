package cleangit_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	cleangit "github.com/lvrach/git-hooks/internal/clean-local-git"
)

func TestScan_EmptyDirectory(t *testing.T) {
	t.Log("Testing Scan with empty directory")

	tempDir := t.TempDir()

	repos, err := cleangit.Scan(context.Background(), tempDir, 5)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(repos) != 0 {
		t.Errorf("Expected 0 repositories, got %d", len(repos))
	}
}

func TestScan_WithCustomHooksPath(t *testing.T) {
	t.Log("Testing Scan finds repository with custom hooksPath")

	tempDir := t.TempDir()

	// Create a fake Git repository with custom hooksPath
	gitDir := filepath.Join(tempDir, ".git")
	err := os.Mkdir(gitDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create .git directory: %v", err)
	}

	// Create .git/config with hooksPath
	configPath := filepath.Join(gitDir, "config")
	configContent := `[core]
	repositoryformatversion = 0
	filemode = true
	hooksPath = .husky
[remote "origin"]
	url = git@github.com:example/repo.git
`
	err = os.WriteFile(configPath, []byte(configContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	repos, err := cleangit.Scan(context.Background(), tempDir, 5)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(repos) != 1 {
		t.Fatalf("Expected 1 repository, got %d", len(repos))
	}

	repo := repos[0]
	if !repo.HasCustomHooks {
		t.Error("Expected HasCustomHooks to be true")
	}

	if repo.CustomHooksPath != ".husky" {
		t.Errorf("Expected CustomHooksPath to be '.husky', got '%s'", repo.CustomHooksPath)
	}

	if repo.Path != tempDir {
		t.Errorf("Expected Path to be %s, got %s", tempDir, repo.Path)
	}
}

func TestScan_WithoutCustomHooksPath(t *testing.T) {
	t.Log("Testing Scan ignores repository without custom hooksPath")

	tempDir := t.TempDir()

	// Create a fake Git repository without custom hooksPath
	gitDir := filepath.Join(tempDir, ".git")
	err := os.Mkdir(gitDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create .git directory: %v", err)
	}

	// Create .git/config without hooksPath
	configPath := filepath.Join(gitDir, "config")
	configContent := `[core]
	repositoryformatversion = 0
	filemode = true
[remote "origin"]
	url = git@github.com:example/repo.git
`
	err = os.WriteFile(configPath, []byte(configContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	repos, err := cleangit.Scan(context.Background(), tempDir, 5)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Should not return repos without custom hooksPath
	if len(repos) != 0 {
		t.Errorf("Expected 0 repositories, got %d", len(repos))
	}
}

func TestScan_NestedRepositories(t *testing.T) {
	t.Log("Testing Scan finds nested repositories")

	tempDir := t.TempDir()

	// Create multiple nested repos
	repo1 := filepath.Join(tempDir, "repo1")
	repo2 := filepath.Join(tempDir, "subdir", "repo2")

	for _, repoPath := range []string{repo1, repo2} {
		err := os.MkdirAll(filepath.Join(repoPath, ".git"), 0o755)
		if err != nil {
			t.Fatalf("Failed to create .git directory: %v", err)
		}

		configPath := filepath.Join(repoPath, ".git", "config")
		configContent := `[core]
	hooksPath = .husky
`
		err = os.WriteFile(configPath, []byte(configContent), 0o644)
		if err != nil {
			t.Fatalf("Failed to write config: %v", err)
		}
	}

	repos, err := cleangit.Scan(context.Background(), tempDir, 10)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(repos) != 2 {
		t.Errorf("Expected 2 repositories, got %d", len(repos))
	}
}

func TestScan_MaxDepth(t *testing.T) {
	t.Log("Testing Scan respects maxDepth")

	tempDir := t.TempDir()

	// Create a deeply nested repo
	deepRepo := filepath.Join(tempDir, "l1", "l2", "l3", "l4", "l5", "l6")
	err := os.MkdirAll(filepath.Join(deepRepo, ".git"), 0o755)
	if err != nil {
		t.Fatalf("Failed to create deep .git directory: %v", err)
	}

	configPath := filepath.Join(deepRepo, ".git", "config")
	configContent := `[core]
	hooksPath = .husky
`
	err = os.WriteFile(configPath, []byte(configContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	// Should not find with low depth
	repos, err := cleangit.Scan(context.Background(), tempDir, 3)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(repos) != 0 {
		t.Errorf("Expected 0 repositories with depth 3, got %d", len(repos))
	}

	// Should find with sufficient depth
	repos, err = cleangit.Scan(context.Background(), tempDir, 10)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(repos) != 1 {
		t.Errorf("Expected 1 repository with depth 10, got %d", len(repos))
	}
}

func TestClean_ActualRemoval(t *testing.T) {
	t.Log("Testing Clean actually removes hooksPath")

	tempDir := t.TempDir()

	// Create a fake Git repository
	gitDir := filepath.Join(tempDir, ".git")
	err := os.Mkdir(gitDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create .git directory: %v", err)
	}

	configPath := filepath.Join(gitDir, "config")
	originalContent := `[core]
	repositoryformatversion = 0
	hooksPath = .husky
[remote "origin"]
	url = git@github.com:example/repo.git
`
	err = os.WriteFile(configPath, []byte(originalContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	repos, err := cleangit.Scan(context.Background(), tempDir, 5)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Clean for real
	summary := cleangit.Clean(context.Background(), repos)

	if summary.ConfigsRemoved != 1 {
		t.Errorf("Expected 1 config removed, got %d", summary.ConfigsRemoved)
	}

	result := summary.Results[0]
	if !result.Removed {
		t.Error("Expected Removed to be true")
	}

	if result.Error != nil {
		t.Errorf("Expected no error, got %v", result.Error)
	}

	if result.PreviousPath != ".husky" {
		t.Errorf("Expected PreviousPath to be '.husky', got '%s'", result.PreviousPath)
	}

	// Verify hooksPath was removed
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}

	contentStr := string(content)
	if contains(contentStr, "hooksPath") {
		t.Error("hooksPath was not removed from config")
	}

	// Verify other content is preserved
	if !contains(contentStr, "[core]") {
		t.Error("[core] section was removed")
	}
	if !contains(contentStr, "[remote") {
		t.Error("[remote] section was removed")
	}
}

func TestClean_MultipleRepositories(t *testing.T) {
	t.Log("Testing Clean handles multiple repositories")

	tempDir := t.TempDir()

	// Create multiple repos
	for i := 1; i <= 3; i++ {
		repoPath := filepath.Join(tempDir, filepath.FromSlash("repo"), string(rune('0'+i)))
		gitDir := filepath.Join(repoPath, ".git")
		err := os.MkdirAll(gitDir, 0o755)
		if err != nil {
			t.Fatalf("Failed to create .git directory: %v", err)
		}

		configPath := filepath.Join(gitDir, "config")
		configContent := `[core]
	hooksPath = .husky
`
		err = os.WriteFile(configPath, []byte(configContent), 0o644)
		if err != nil {
			t.Fatalf("Failed to write config: %v", err)
		}
	}

	repos, err := cleangit.Scan(context.Background(), tempDir, 10)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	summary := cleangit.Clean(context.Background(), repos)

	if summary.RepositoriesWithConfig != 3 {
		t.Errorf("Expected 3 repositories, got %d", summary.RepositoriesWithConfig)
	}

	if summary.ConfigsRemoved != 3 {
		t.Errorf("Expected 3 configs removed, got %d", summary.ConfigsRemoved)
	}

	if len(summary.Results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(summary.Results))
	}
}

func TestScan_ContextCancellation(t *testing.T) {
	t.Log("Testing Scan respects context cancellation")

	tempDir := t.TempDir()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := cleangit.Scan(ctx, tempDir, 5)
	if err == nil {
		t.Error("Expected an error, got nil")
	}
}

func TestScan_ExcludedDirectories(t *testing.T) {
	t.Log("Testing Scan skips excluded directories")

	tempDir := t.TempDir()

	// Create repos in excluded directories
	excludedDirs := []string{"node_modules", "venv", ".venv", "vendor"}
	for _, dir := range excludedDirs {
		repoPath := filepath.Join(tempDir, dir, "fake-repo")
		err := os.MkdirAll(filepath.Join(repoPath, ".git"), 0o755)
		if err != nil {
			t.Fatalf("Failed to create .git directory in %s: %v", dir, err)
		}

		configPath := filepath.Join(repoPath, ".git", "config")
		configContent := `[core]
	hooksPath = .husky
`
		err = os.WriteFile(configPath, []byte(configContent), 0o644)
		if err != nil {
			t.Fatalf("Failed to write config: %v", err)
		}
	}

	// Create a valid repo
	validRepo := filepath.Join(tempDir, "valid-repo")
	err := os.MkdirAll(filepath.Join(validRepo, ".git"), 0o755)
	if err != nil {
		t.Fatalf("Failed to create valid repo: %v", err)
	}

	configPath := filepath.Join(validRepo, ".git", "config")
	configContent := `[core]
	hooksPath = .husky
`
	err = os.WriteFile(configPath, []byte(configContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	repos, err := cleangit.Scan(context.Background(), tempDir, 10)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Should only find the valid repo, not the ones in excluded dirs
	if len(repos) != 1 {
		t.Errorf("Expected 1 repository, got %d", len(repos))
		for _, repo := range repos {
			t.Logf("Found repo: %s", repo.Path)
		}
	}

	if len(repos) > 0 && !contains(repos[0].Path, "valid-repo") {
		t.Errorf("Expected to find valid-repo, got %s", repos[0].Path)
	}
}

func TestClean_PreservesOtherConfig(t *testing.T) {
	t.Log("Testing Clean preserves other git config sections")

	tempDir := t.TempDir()

	gitDir := filepath.Join(tempDir, ".git")
	err := os.Mkdir(gitDir, 0o755)
	if err != nil {
		t.Fatalf("Failed to create .git directory: %v", err)
	}

	configPath := filepath.Join(gitDir, "config")
	originalContent := `[core]
	repositoryformatversion = 0
	filemode = true
	bare = false
	logallrefupdates = true
	ignorecase = true
	precomposeunicode = true
	hooksPath = .husky
[remote "origin"]
	url = git@github.com:example/repo.git
	fetch = +refs/heads/*:refs/remotes/origin/*
[branch "main"]
	remote = origin
	merge = refs/heads/main
[user]
	name = Test User
	email = test@example.com
`
	err = os.WriteFile(configPath, []byte(originalContent), 0o644)
	if err != nil {
		t.Fatalf("Failed to write config: %v", err)
	}

	repos, err := cleangit.Scan(context.Background(), tempDir, 5)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	// Clean for real
	summary := cleangit.Clean(context.Background(), repos)

	if summary.ConfigsRemoved != 1 {
		t.Errorf("Expected 1 config removed, got %d", summary.ConfigsRemoved)
	}

	// Verify hooksPath was removed but everything else preserved
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("Failed to read config: %v", err)
	}

	contentStr := string(content)

	// Should NOT contain hooksPath
	if contains(contentStr, "hooksPath") {
		t.Error("hooksPath was not removed from config")
	}

	// Should preserve all other sections
	mustContain := []string{
		"[core]",
		"repositoryformatversion",
		"[remote \"origin\"]",
		"[branch \"main\"]",
		"[user]",
		"name = Test User",
		"email = test@example.com",
	}

	for _, str := range mustContain {
		if !contains(contentStr, str) {
			t.Errorf("Config should contain '%s' but doesn't", str)
		}
	}
}

func TestClean_EmptyResults(t *testing.T) {
	t.Log("Testing Clean with no repositories")

	summary := cleangit.Clean(context.Background(), []cleangit.Repository{})

	if summary.RepositoriesWithConfig != 0 {
		t.Errorf("Expected 0 repositories, got %d", summary.RepositoriesWithConfig)
	}

	if summary.ConfigsRemoved != 0 {
		t.Errorf("Expected 0 configs removed, got %d", summary.ConfigsRemoved)
	}

	if len(summary.Results) != 0 {
		t.Errorf("Expected 0 results, got %d", len(summary.Results))
	}
}

func TestScan_NonGitDirectory(t *testing.T) {
	t.Log("Testing Scan with non-git directories")

	tempDir := t.TempDir()

	// Create some random directories without .git
	for _, dir := range []string{"src", "docs", "build"} {
		err := os.MkdirAll(filepath.Join(tempDir, dir), 0o755)
		if err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
	}

	repos, err := cleangit.Scan(context.Background(), tempDir, 5)
	if err != nil {
		t.Fatalf("Scan failed: %v", err)
	}

	if len(repos) != 0 {
		t.Errorf("Expected 0 repositories, got %d", len(repos))
	}
}

// Helper functions
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
