package cleangit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

var (
	ErrPermissionDenied = errors.New("permission denied")
	ErrNotGitRepository = errors.New("not a git repository")
	ErrHooksNotFound    = errors.New("hooks directory not found")
	ErrRemovalFailed    = errors.New("removing hooks")
	ErrUserCancelled    = errors.New("operation cancelled by user")
)

// Repository represents a Git repository
type Repository struct {
	Path            string
	GitDir          string
	ConfigPath      string
	CustomHooksPath string
	HasCustomHooks  bool
}

// Result tracks the outcome of removing hooksPath config
type Result struct {
	Repository   *Repository
	Removed      bool
	PreviousPath string
	Error        error
}

// Summary contains the overall results of the clean operation
type Summary struct {
	RepositoriesWithConfig int
	ConfigsRemoved         int
	Results                []Result
}

// Scan finds Git repositories and returns those with custom hooksPath
func Scan(ctx context.Context, rootPath string, maxDepth int) ([]Repository, error) {
	repos, err := findRepositories(ctx, rootPath, maxDepth)
	if err != nil {
		return nil, fmt.Errorf("finding repositories: %w", err)
	}

	// Filter to only return repos with custom hooks
	var reposWithHooks []Repository
	for _, repo := range repos {
		if repo.HasCustomHooks {
			reposWithHooks = append(reposWithHooks, repo)
		}
	}

	return reposWithHooks, nil
}

// Clean removes hooksPath configuration from repositories
func Clean(ctx context.Context, repos []Repository) Summary {
	sum := Summary{
		RepositoriesWithConfig: len(repos),
	}

	for i := range repos {
		res := Result{
			Repository:   &repos[i],
			PreviousPath: repos[i].CustomHooksPath,
		}

		// Check context cancellation
		select {
		case <-ctx.Done():
			res.Error = ctx.Err()
			sum.Results = append(sum.Results, res)
			return sum
		default:
		}

		if err := removeHooksPath(&repos[i]); err != nil {
			res.Error = err
		} else {
			res.Removed = true
			sum.ConfigsRemoved++
		}

		sum.Results = append(sum.Results, res)
	}

	return sum
}

// findRepositories scans for Git repositories recursively
func findRepositories(ctx context.Context, rootPath string, maxDepth int) ([]Repository, error) {
	var repos []Repository

	absRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, err
	}

	err = walkDir(ctx, absRoot, 0, maxDepth, func(r Repository) {
		repos = append(repos, r)
	})

	return repos, err
}

// walkDir recursively walks directories looking for .git folders
func walkDir(ctx context.Context, path string, depth, maxDepth int, callback func(Repository)) error {
	// Check context cancellation
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	// Check max depth
	if depth > maxDepth {
		return nil
	}

	// Skip common directories
	if shouldSkip(path) {
		return nil
	}

	// Check if this is a Git repository
	gitDir := filepath.Join(path, ".git")
	if info, err := os.Stat(gitDir); err == nil && info.IsDir() {
		repo := createRepository(path, gitDir)
		callback(repo)
		return nil // Don't recurse into .git
	}

	// Read directory contents
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsPermission(err) {
			return nil // Skip permission errors
		}
		return err
	}

	// Recurse into subdirectories
	for _, entry := range entries {
		if entry.IsDir() {
			subPath := filepath.Join(path, entry.Name())
			if err := walkDir(ctx, subPath, depth+1, maxDepth, callback); err != nil {
				return err
			}
		}
	}

	return nil
}

// shouldSkip determines if a directory should be excluded
func shouldSkip(path string) bool {
	dirName := filepath.Base(path)

	// Common directories to skip
	skipDirs := []string{
		"node_modules", "venv", ".venv", "env", "__pycache__",
		".terraform", "vendor",
	}

	for _, skip := range skipDirs {
		if dirName == skip {
			return true
		}
	}

	return false
}

// createRepository creates a Repository struct with config info
func createRepository(repoPath, gitDir string) Repository {
	configPath := filepath.Join(gitDir, "config")
	repo := Repository{
		Path:       repoPath,
		GitDir:     gitDir,
		ConfigPath: configPath,
	}

	// Check for custom hooksPath in config
	customPath, hasCustom := getHooksPath(configPath)
	repo.CustomHooksPath = customPath
	repo.HasCustomHooks = hasCustom

	return repo
}

// getHooksPath extracts hooksPath from .git/config using git config command
func getHooksPath(configPath string) (string, bool) {
	// Get the repository path (parent of .git directory)
	repoPath := filepath.Dir(filepath.Dir(configPath))

	// Use git config to read the value
	cmd := exec.Command("git", "config", "--local", "core.hooksPath")
	cmd.Dir = repoPath

	output, err := cmd.Output()
	if err != nil {
		// git config returns exit code 1 if key doesn't exist
		return "", false
	}

	value := strings.TrimSpace(string(output))
	if value == "" {
		return "", false
	}

	return value, true
}

// removeHooksPath removes the hooksPath setting using git config command
func removeHooksPath(repo *Repository) error {
	// Use git config to unset the value
	cmd := exec.Command("git", "config", "--local", "--unset", "core.hooksPath")
	cmd.Dir = repo.Path

	// Run the command
	if err := cmd.Run(); err != nil {
		// Check if it's an exit error
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Exit code 5 means the key didn't exist, which is fine
			if exitErr.ExitCode() == 5 {
				return nil
			}
		}
		return fmt.Errorf("unsetting core.hooksPath: %w", err)
	}

	return nil
}
