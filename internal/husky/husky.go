// Package husky plans a single invocation across Husky versions.
package husky

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Action determines whether the caller should execute a hook or report a skip.
type Action int

const (
	// None means there is no repository Husky hook.
	None Action = iota
	// Run means the caller should execute the plan.
	Run
	// Skip means the hook cannot run or was explicitly disabled.
	Skip
)

// Plan supplies one command and its environment for execution from the repository root.
type Plan struct {
	Action Action
	Argv   []string
	Env    []string
	Reason string
}

var legacySource = regexp.MustCompile(`(?m)^[\t ]*(?:\.|source)[\t ]+[^#\r\n]*_/husky\.sh["']*[\t ]*(?:\r?$|#|;)`)

// Resolve inspects the repository without executing hooks or changing its environment.
func Resolve(repoRoot, hookName string, args []string, environ []string) Plan {
	repoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return Plan{Action: Skip, Reason: "Cannot resolve repository root: " + err.Error()}
	}
	hook := filepath.Join(repoRoot, ".husky", hookName)
	info, err := os.Stat(hook)
	if err != nil || !info.Mode().IsRegular() {
		return Plan{Action: None}
	}
	var husky string
	for _, entry := range environ {
		if strings.HasPrefix(entry, "HUSKY=") {
			husky = strings.TrimPrefix(entry, "HUSKY=")
		}
	}
	if husky == "0" {
		return Plan{Action: Skip, Reason: "Husky is disabled by HUSKY=0"}
	}
	info, err = os.Stat(filepath.Join(repoRoot, "node_modules"))
	if (err != nil || !info.IsDir()) && !regularFile(filepath.Join(repoRoot, ".pnp.cjs")) && !regularFile(filepath.Join(repoRoot, ".pnp.js")) {
		return Plan{Action: Skip, Reason: "Husky needs node_modules; run your package manager's install command"}
	}
	content, err := os.ReadFile(hook)
	if err != nil {
		return Plan{Action: Skip, Reason: "Cannot read Husky hook: " + err.Error()}
	}
	legacy := legacySource.Match(content)
	if legacy {
		helperPath := filepath.Join(repoRoot, ".husky", "_", "husky.sh")
		info, err := os.Stat(helperPath)
		if errors.Is(err, fs.ErrNotExist) {
			return Plan{Action: Skip, Reason: "Missing .husky/_/husky.sh; run npm install or migrate to husky v9"}
		}
		// husky v9 leaves a deprecation stub here that never re-runs the hook with
		// sh -e; only the real v7/v8 helper does. Anything odd (unreadable, not a
		// small regular file) keeps the legacy path, so the hook runs and can fail.
		if err == nil && info.Mode().IsRegular() && info.Size() < 1<<20 {
			if helper, err := os.ReadFile(helperPath); err == nil {
				legacy = bytes.Contains(helper, []byte("husky_skip_init"))
			}
		}
	}
	stub := filepath.Join(repoRoot, ".husky", "_", hookName)
	if regularFile(filepath.Join(repoRoot, ".husky", "_", "h")) && regularFile(stub) {
		return Plan{Action: Run, Argv: append([]string{"sh", stub}, args...), Env: slices.Clone(environ)}
	}
	env := make([]string, 0, len(environ)+2)
	var path string
	for _, entry := range environ {
		switch {
		case strings.HasPrefix(entry, "PATH="):
			path = strings.TrimPrefix(entry, "PATH=")
		case strings.HasPrefix(entry, "husky_skip_init="):
		default:
			env = append(env, entry)
		}
	}
	bin := filepath.Join(repoRoot, "node_modules", ".bin")
	if info, err := os.Stat(bin); err == nil && info.IsDir() {
		if path != "" {
			bin += string(os.PathListSeparator) + path
		}
		path = bin
	}
	env = append(env, "PATH="+path)
	if legacy {
		// The helper loads ~/.huskyrc before restarting the hook with sh -e.
		return Plan{Action: Run, Argv: append([]string{"sh", hook}, args...), Env: env}
	}
	init := `i="${XDG_CONFIG_HOME:-$HOME/.config}/husky/init.sh"; [ -f "$i" ] && . "$i"; [ "${HUSKY-}" = "0" ] && exit 0; exec sh -e "$0" "$@"`
	return Plan{Action: Run, Argv: append([]string{"sh", "-c", init, hook}, args...), Env: env}
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
