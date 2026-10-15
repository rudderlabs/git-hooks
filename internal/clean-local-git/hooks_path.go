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
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", repoDir}, args...)...)
	if abs, err := filepath.Abs(repoDir); err == nil {
		cmd.Env = append(os.Environ(), "GIT_CEILING_DIRECTORIES="+filepath.Dir(abs))
	}
	return cmd
}
