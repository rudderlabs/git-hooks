package procrun

import (
	"fmt"
	"syscall"
)

// ExitError preserves a foreground stage's exit code or terminating signal.
type ExitError struct {
	Code   int
	Signal syscall.Signal
}

// Error describes how the stage terminated.
func (e *ExitError) Error() string {
	if e.Signal != 0 {
		return "signal: " + e.Signal.String()
	}
	return fmt.Sprintf("exit status %d", e.Code)
}

// ExitCode returns the exit status, or -1 when a signal terminated the stage.
func (e *ExitError) ExitCode() int {
	return e.Code
}
