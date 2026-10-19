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

// LocalHooksPath returns hooksPath values from local and enabled worktree config, excluding global config.
func LocalHooksPath(ctx context.Context, repoDir string) ([]string, error) {
	values, err := configValues(ctx, repoDir, "--local", "core.hooksPath")
	if err != nil {
		return nil, err
	}
	enabled, err := readConfig(ctx, repoDir, "--local", "--type=bool", "--get", "extensions.worktreeConfig")
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(enabled)) == "true" {
		worktree, err := configValues(ctx, repoDir, "--worktree", "core.hooksPath")
		if err != nil {
			return nil, err
		}
		values = append(values, worktree...)
	}
	return values, nil
}

func configValues(ctx context.Context, repoDir, scope, key string) ([]string, error) {
	output, err := readConfig(ctx, repoDir, scope, "--null", "--get-all", key)
	if err != nil || len(output) == 0 {
		return nil, err
	}
	// NUL separators preserve empty values and paths containing newlines.
	return strings.Split(strings.TrimSuffix(string(output), "\x00"), "\x00"), nil
}

func readConfig(ctx context.Context, repoDir string, args ...string) ([]byte, error) {
	cmd := gitCommand(ctx, repoDir, append([]string{"config"}, args...)...)
	output, err := cmd.Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// Git distinguishes an absent key from a failed config read.
			if exitErr.ExitCode() == 1 {
				return nil, nil
			}
			return nil, fmt.Errorf("reading git config in %s: %w: %s", repoDir, err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return nil, fmt.Errorf("reading git config in %s: %w", repoDir, err)
	}
	return output, nil
}

// gitCommand runs git in repoDir without discovery above it. A broken .git
// would otherwise make git read, or unset, the parent repository's config.
func gitCommand(ctx context.Context, repoDir string, args ...string) *exec.Cmd {
	// GIT_DIR below must not be resolved again relative to -C.
	if abs, err := filepath.Abs(repoDir); err == nil {
		repoDir = abs
	}
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repoDir}, args...)...)
	// GIT_DIR and friends override discovery and the ceiling; a scan run from
	// inside a hook inherits them and would edit the hook's repository.
	env := make([]string, 0, len(os.Environ())+1)
	for _, entry := range os.Environ() {
		name, _, _ := strings.Cut(entry, "=")
		switch name {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY",
			"GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_NAMESPACE", "GIT_CONFIG", "GIT_CONFIG_PARAMETERS",
			"GIT_CONFIG_COUNT", "GIT_CEILING_DIRECTORIES", "GIT_DISCOVERY_ACROSS_FILESYSTEM":
			continue
		}
		// `git -c` in the calling hook arrives as GIT_CONFIG_KEY_n/VALUE_n and would read as local config.
		if strings.HasPrefix(name, "GIT_CONFIG_KEY_") || strings.HasPrefix(name, "GIT_CONFIG_VALUE_") {
			continue
		}
		env = append(env, entry)
	}
	// A .git directory is named outright, so git never discovers upward.
	// GIT_CEILING_DIRECTORIES cannot hold a path with ':' in it.
	if info, err := os.Stat(filepath.Join(repoDir, ".git")); err == nil && info.IsDir() {
		env = append(env, "GIT_DIR="+filepath.Join(repoDir, ".git"))
	} else if real, err := filepath.EvalSymlinks(repoDir); err == nil {
		if abs, err := filepath.Abs(real); err == nil {
			env = append(env, "GIT_CEILING_DIRECTORIES="+filepath.Dir(abs))
		}
	}
	cmd.Env = env
	return cmd
}
