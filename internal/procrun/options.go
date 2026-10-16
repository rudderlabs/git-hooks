// Package procrun runs repository hook stages with a timeout.
package procrun

import (
	"io"
	"os"
	"time"
)

// Options controls a stage's timeout and execution environment.
type Options struct {
	Timeout time.Duration
	Env     []string
	Dir     string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
}

// DefaultOptions reads the timeout from GIT_HOOKS_TIMEOUT, falling back on invalid values.
func DefaultOptions() Options {
	opt := Options{Timeout: 10 * time.Minute}
	if d, err := time.ParseDuration(os.Getenv("GIT_HOOKS_TIMEOUT")); err == nil && d >= 0 {
		opt.Timeout = d
	}
	return opt
}
