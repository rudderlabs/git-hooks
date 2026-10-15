package husky_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rudderlabs/git-hooks/internal/husky"
	"github.com/stretchr/testify/require"
)

func TestResolveNone(t *testing.T) {
	t.Parallel()
	for _, layout := range []string{"no husky directory", "stubs only", "hook is directory"} {
		t.Run(layout, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			switch layout {
			case "stubs only":
				writeFile(t, root, ".husky/_/pre-commit", "exit 0\n", 0644)
			case "hook is directory":
				require.NoError(t, os.MkdirAll(filepath.Join(root, ".husky/pre-commit"), 0755))
			}
			require.Equal(t, husky.Plan{Action: husky.None}, husky.Resolve(root, "pre-commit", nil, []string{"HUSKY=0"}))
		})
	}
}

func TestResolveMissingNodeModules(t *testing.T) {
	t.Parallel()
	for _, layout := range []string{"absent", "regular file", "v9 runtime"} {
		t.Run(layout, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFile(t, root, ".husky/pre-commit", "exit 0\n", 0644)
			switch layout {
			case "regular file":
				writeFile(t, root, "node_modules", "", 0644)
			case "v9 runtime":
				writeFile(t, root, ".husky/_/h", "exit 0\n", 0644)
				writeFile(t, root, ".husky/_/pre-commit", "exit 0\n", 0644)
			}
			plan := husky.Resolve(root, "pre-commit", nil, nil)
			require.Equal(t, husky.Skip, plan.Action)
			require.Contains(t, plan.Reason, "node_modules")
			require.Contains(t, plan.Reason, "install")
			require.Empty(t, plan.Argv)
		})
	}
}

func TestResolveMissingLegacyHelper(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		hook string
	}{
		{"v7", "#!/bin/sh\n. \"$(dirname \"$0\")/_/husky.sh\"\nexit 0\n"},
		{"v8", "#!/usr/bin/env sh\n. \"$(dirname -- \"$0\")/_/husky.sh\"\nexit 0\n"},
		{"whitespace", "\t.\t\"$(dirname -- \"$0\")/_/husky.sh\" # initialize\nexit 0\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFile(t, root, ".husky/pre-commit", tc.hook, 0755)
			require.NoError(t, os.Mkdir(filepath.Join(root, "node_modules"), 0755))
			plan := husky.Resolve(root, "pre-commit", nil, nil)
			require.Equal(t, husky.Skip, plan.Action)
			require.Contains(t, plan.Reason, ".husky/_/husky.sh")
			require.Contains(t, plan.Reason, "npm install")
			require.Contains(t, plan.Reason, "husky v9")
			require.Empty(t, plan.Argv)
		})
	}
}

func TestResolveV9Runtime(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		mode os.FileMode
	}{
		{"non executable", 0644},
		{"executable", 0755},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			counter := filepath.Join(root, "counter")
			marker := filepath.Join(root, "marker")
			writeFile(t, root, ".husky/pre-commit", "printf '%s\\n' \"$@\" > \"$MARKER\"\n", tc.mode)
			writeFile(t, root, ".husky/_/pre-commit", "printf 'stub\\n' >> \"$COUNTER\"\n. \"$(dirname \"$0\")/h\"\n", 0644)
			writeFile(t, root, ".husky/_/h", "sh -e .husky/pre-commit \"$@\"\n", 0644)
			require.NoError(t, os.Mkdir(filepath.Join(root, "node_modules"), 0755))
			env := []string{"PATH=/usr/bin:/bin", "COUNTER=" + counter, "MARKER=" + marker}
			args := []string{"message with spaces", "--flag"}
			plan := husky.Resolve(root, "pre-commit", args, env)
			require.Equal(t, husky.Run, plan.Action)
			require.Equal(t, append([]string{"sh", filepath.Join(root, ".husky/_/pre-commit")}, args...), plan.Argv)
			require.Equal(t, env, plan.Env)
			require.Empty(t, plan.Reason)
			require.NoFileExists(t, counter)
			require.NoFileExists(t, marker)
			cmd := exec.Command(plan.Argv[0], plan.Argv[1:]...)
			cmd.Dir, cmd.Env = root, plan.Env
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", output)
			content, err := os.ReadFile(counter)
			require.NoError(t, err)
			require.Equal(t, 1, strings.Count(string(content), "\n"))
			content, err = os.ReadFile(marker)
			require.NoError(t, err)
			require.Equal(t, strings.Join(args, "\n")+"\n", string(content))
		})
	}
}

func TestResolveFallback(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		prefix   string
		helper   bool
		runtime  string
		paths    []string
		relative bool
		exitCode int
	}{
		{name: "v7 with helper", prefix: "#!/bin/sh\n. \"$(dirname \"$0\")/_/husky.sh\"\n", helper: true, paths: []string{"/usr/bin:/bin"}},
		{name: "v8 with helper", prefix: "#!/usr/bin/env sh\n. \"$(dirname -- \"$0\")/_/husky.sh\"\n", helper: true, paths: []string{"/usr/bin:/bin"}},
		{name: "v9 without runtime", paths: []string{"/usr/bin:/bin"}},
		{name: "runtime helper only", runtime: "h", paths: []string{"/usr/bin:/bin"}},
		{name: "runtime stub only", runtime: "pre-commit", paths: []string{"/usr/bin:/bin"}},
		{name: "commented source", prefix: "# . \"$(dirname \"$0\")/_/husky.sh\"\n", paths: []string{"/usr/bin:/bin"}},
		{name: "no PATH"},
		{name: "empty PATH", paths: []string{""}},
		{name: "duplicate PATH", paths: []string{"/unused", "/usr/bin:/bin"}},
		{name: "relative root", relative: true, paths: []string{"/usr/bin:/bin"}},
		{name: "exit 3", prefix: "exit 3\n", exitCode: 3, paths: []string{"/usr/bin:/bin"}},
		{name: "stop on failure", prefix: "sh -c 'exit 3'\n", exitCode: 3, paths: []string{"/usr/bin:/bin"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := filepath.Join(t.TempDir(), "repo with spaces")
			marker := filepath.Join(root, "marker")
			writeFile(t, root, ".husky/pre-commit", tc.prefix+"lint-staged \"$@\"\n", 0644)
			writeFile(t, root, "node_modules/.bin/lint-staged", "#!/bin/sh\nprintf '%s\\n' \"$@\" > \"$MARKER\"\n", 0755)
			if tc.helper {
				writeFile(t, root, ".husky/_/husky.sh", "if [ \"$husky_skip_init\" != 1 ]; then exit 99; fi\n", 0644)
			}
			if tc.runtime != "" {
				writeFile(t, root, ".husky/_/"+tc.runtime, "exit 99\n", 0644)
			}
			env := []string{"MARKER=" + marker, "HUSKY=1", "husky_skip_init=0"}
			for _, path := range tc.paths {
				env = append(env, "PATH="+path)
			}
			originalEnv := append([]string(nil), env...)
			args := []string{"message with spaces", "--flag"}
			inputRoot := root
			if tc.relative {
				cwd, err := os.Getwd()
				require.NoError(t, err)
				inputRoot, err = filepath.Rel(cwd, root)
				require.NoError(t, err)
			}
			plan := husky.Resolve(inputRoot, "pre-commit", args, env)
			require.Equal(t, husky.Run, plan.Action)
			require.Equal(t, append([]string{"sh", "-e", filepath.Join(root, ".husky/pre-commit")}, args...), plan.Argv)
			require.Empty(t, plan.Reason)
			require.Equal(t, originalEnv, env)
			wantEnv := []string{"MARKER=" + marker, "HUSKY=1", "husky_skip_init=1"}
			path := filepath.Join(root, "node_modules/.bin")
			if len(tc.paths) > 0 && tc.paths[len(tc.paths)-1] != "" {
				path += string(os.PathListSeparator) + tc.paths[len(tc.paths)-1]
			}
			wantEnv = append(wantEnv, "PATH="+path)
			require.ElementsMatch(t, wantEnv, plan.Env)
			require.NoFileExists(t, marker)
			cmd := exec.Command(plan.Argv[0], plan.Argv[1:]...)
			cmd.Dir, cmd.Env = root, plan.Env
			output, err := cmd.CombinedOutput()
			if tc.exitCode != 0 {
				var exitErr *exec.ExitError
				require.ErrorAs(t, err, &exitErr, "%s", output)
				require.Equal(t, tc.exitCode, exitErr.ExitCode())
				require.NoFileExists(t, marker)
				return
			}
			require.NoError(t, err, "%s", output)
			content, err := os.ReadFile(marker)
			require.NoError(t, err)
			require.Equal(t, strings.Join(args, "\n")+"\n", string(content))
		})
	}
}

func writeFile(t *testing.T, root, name, content string, mode os.FileMode) {
	t.Helper()
	path := filepath.Join(root, name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, []byte(content), mode))
}

func TestResolveDisabled(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"pre-commit", "commit-msg"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFile(t, root, ".husky/"+name, "exit 0\n", 0644)
			plan := husky.Resolve(root, name, nil, []string{"HUSKY=0"})
			require.Equal(t, husky.Skip, plan.Action)
			require.Contains(t, plan.Reason, "HUSKY=0")
			require.Empty(t, plan.Argv)
		})
	}
}
