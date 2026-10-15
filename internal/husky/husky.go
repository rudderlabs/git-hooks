// Package husky plans a single invocation across Husky versions.
package husky

import (
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

var legacySource = regexp.MustCompile(`(?m)^[\t ]*\.[\t ]+"\$\(dirname[\t ]+(?:--[\t ]+)?"\$0"\)/_/husky\.sh"[\t ]*(?:\r?$|#|;)`)

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
	if slices.Contains(environ, "HUSKY=0") {
		return Plan{Action: Skip, Reason: "Husky is disabled by HUSKY=0"}
	}
	info, err = os.Stat(filepath.Join(repoRoot, "node_modules"))
	if err != nil || !info.IsDir() {
		return Plan{Action: Skip, Reason: "Husky needs node_modules; run your package manager's install command"}
	}
	content, err := os.ReadFile(hook)
	if err != nil {
		return Plan{Action: Skip, Reason: "Cannot read Husky hook: " + err.Error()}
	}
	if legacySource.Match(content) {
		if _, err := os.Stat(filepath.Join(repoRoot, ".husky", "_", "husky.sh")); err != nil {
			return Plan{Action: Skip, Reason: "Missing .husky/_/husky.sh; run npm install or migrate to husky v9"}
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
	if path != "" {
		bin += string(os.PathListSeparator) + path
	}
	env = append(env, "husky_skip_init=1", "PATH="+bin)
	return Plan{Action: Run, Argv: append([]string{"sh", "-e", hook}, args...), Env: env}
}

func regularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
