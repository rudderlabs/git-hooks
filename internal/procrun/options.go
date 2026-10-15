// Package procrun limits the resources used by repository hook stages.
package procrun

import (
	"crypto/sha256"
	_ "embed"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

//go:embed cpucap.cjs
var cpuPreload []byte

// Options controls a stage's resource limits and execution environment.
type Options struct {
	Timeout time.Duration
	CPUs    int
	Nice    int
	Env     []string
	Dir     string
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
}

// DefaultOptions reads hook limits from the environment, falling back on invalid values.
func DefaultOptions() Options {
	opt := Options{Timeout: 10 * time.Minute, CPUs: 2, Nice: 10}
	if d, err := time.ParseDuration(os.Getenv("GIT_HOOKS_TIMEOUT")); err == nil && d >= 0 {
		opt.Timeout = d
	}
	if n, err := strconv.Atoi(os.Getenv("GIT_HOOKS_CPUS")); err == nil && n >= 0 {
		opt.CPUs = n
	}
	if n, err := strconv.Atoi(os.Getenv("GIT_HOOKS_NICE")); err == nil && n >= 0 {
		opt.Nice = n
	}
	return opt
}

func applyCPUCap(cmd *exec.Cmd, cpus int) {
	if cpus <= 0 {
		return
	}
	cmd.Env = append(cmd.Environ(), "GIT_HOOKS_CPUS="+strconv.Itoa(cpus))
	if err := prepareCPUCap(cmd); err != nil && cmd.Stderr != nil {
		fmt.Fprintf(cmd.Stderr, "git-hooks: warning: CPU cap skipped: %s\n", strings.ReplaceAll(err.Error(), "\n", " "))
	}
}

func prepareCPUCap(cmd *exec.Cmd) error {
	path, err := preloadPath()
	if err != nil {
		return err
	}
	if strings.Contains(path, " ") {
		return errors.New("cache path contains spaces")
	}
	if err := writePreload(path); err != nil {
		return err
	}
	env := cmd.Environ()
	nodeOptions := ""
	for _, entry := range env {
		if value, ok := strings.CutPrefix(entry, "NODE_OPTIONS="); ok {
			nodeOptions = value
		}
	}
	cmd.Env = append(env, "NODE_OPTIONS="+nodeOptions+" --require="+path)
	return nil
}

func preloadPath() (string, error) {
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(cpuPreload)
	return filepath.Join(cache, "git-hooks", fmt.Sprintf("cpucap-%x.cjs", hash[:8])), nil
}

func writePreload(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".cpucap-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(cpuPreload); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(0o644); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	// Concurrent hook stages must never see a partially written preload.
	return os.Rename(f.Name(), path)
}
