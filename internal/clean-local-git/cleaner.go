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
	// Err is set when the repo's config could not be read; the scan reports it and goes on.
	Err error
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
		if repo.HasCustomHooks || repo.Err != nil {
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

		if repos[i].Err != nil {
			res.Error = repos[i].Err
		} else if err := removeHooksPath(ctx, &repos[i]); err != nil {
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
		repo, err := createRepository(ctx, path, gitDir)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			repo.Err = err
		}
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
func createRepository(ctx context.Context, repoPath, gitDir string) (Repository, error) {
	configPath := filepath.Join(gitDir, "config")
	repo := Repository{
		Path:       repoPath,
		GitDir:     gitDir,
		ConfigPath: configPath,
	}

	values, err := LocalHooksPath(ctx, repoPath)
	if err != nil {
		return repo, err
	}
	repo.CustomHooksPath = strings.Join(values, "\n")
	repo.HasCustomHooks = len(values) > 0
	return repo, nil
}

func removeHooksPath(ctx context.Context, repo *Repository) error {
	scopes := []string{"--local"}
	if enabled, err := readConfig(ctx, repo.Path, "--local", "--type=bool", "--get", "extensions.worktreeConfig"); err == nil && strings.TrimSpace(string(enabled)) == "true" {
		scopes = append(scopes, "--worktree")
	}
	for _, scope := range scopes {
		cmd := exec.CommandContext(ctx, "git", "-C", repo.Path, "config", scope, "--unset-all", "core.hooksPath")
		output, err := cmd.CombinedOutput()
		if err == nil {
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		var exitErr *exec.ExitError
		// Exit 5 means this scope has no value; another scope may hold it.
		if errors.As(err, &exitErr) && exitErr.ExitCode() == 5 {
			continue
		}
		return fmt.Errorf("unsetting core.hooksPath (%s) in %s: %w: %s", scope, repo.Path, err, strings.TrimSpace(string(output)))
	}
	remaining, err := LocalHooksPath(ctx, repo.Path)
	if err != nil {
		return err
	}
	if len(remaining) > 0 {
		return fmt.Errorf("core.hooksPath still set in %s: %s", repo.Path, strings.Join(remaining, ", "))
	}
	return nil
}
