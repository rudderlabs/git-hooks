package cleangit

import (
	"context"
	"errors"
	"fmt"
	"os"
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

// Exit codes
const (
	ExitSuccess        = 0
	ExitPartialSuccess = 1
	ExitFailure        = 2
	ExitUserCancelled  = 3
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

		if res.Removed {
			if err := removeHooksPath(&repos[i]); err != nil {
				res.Error = err
			} else {
				res.Removed = true
				sum.ConfigsRemoved++
			}
		} else {
			res.Removed = true // Would be removed
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

// getHooksPath extracts hooksPath from .git/config
func getHooksPath(configPath string) (string, bool) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return "", false
	}

	lines := strings.Split(string(data), "\n")
	inCore := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "[core]" {
			inCore = true
			continue
		}

		if strings.HasPrefix(trimmed, "[") {
			inCore = false
			continue
		}

		if inCore && strings.Contains(trimmed, "hooksPath") {
			parts := strings.SplitN(trimmed, "=", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]), true
			}
		}
	}

	return "", false
}

// removeHooksPath removes the hooksPath line from .git/config
func removeHooksPath(repo *Repository) error {
	data, err := os.ReadFile(repo.ConfigPath)
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}

	// Remove hooksPath line
	lines := strings.Split(string(data), "\n")
	var newLines []string
	inCore := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "[core]" {
			inCore = true
			newLines = append(newLines, line)
			continue
		}

		if strings.HasPrefix(trimmed, "[") {
			inCore = false
			newLines = append(newLines, line)
			continue
		}

		// Skip hooksPath line
		if inCore && strings.Contains(trimmed, "hooksPath") {
			continue
		}

		newLines = append(newLines, line)
	}

	newContent := strings.Join(newLines, "\n")

	// Write back
	err = os.WriteFile(repo.ConfigPath, []byte(newContent), 0o644)
	if err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	return nil
}
