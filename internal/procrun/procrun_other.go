//go:build !darwin && !linux

package procrun

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
)

// Run executes argv with the supplied timeout and environment.
func Run(ctx context.Context, argv []string, opt Options) error {
	if len(argv) == 0 || argv[0] == "" {
		return errors.New("stage command is empty")
	}
	parent := ctx
	if opt.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opt.Timeout)
		defer cancel()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env, cmd.Dir = opt.Env, opt.Dir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = opt.Stdin, opt.Stdout, opt.Stderr
	applyCPUCap(cmd, opt.CPUs)
	err := cmd.Run()
	if ctx.Err() != nil {
		message := "stage canceled and was killed"
		if parent.Err() == nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
			message = fmt.Sprintf("stage timed out after %s and was killed", opt.Timeout)
		}
		return fmt.Errorf("%s: %w", message, errors.Join(ctx.Err(), err))
	}
	return err
}

// InForegroundJob reports false: there is no job control on this platform.
func InForegroundJob() bool { return false }
