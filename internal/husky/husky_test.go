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
	for _, layout := range []string{"absent", "regular file", "v9 runtime", "pnp directories"} {
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
			case "pnp directories":
				require.NoError(t, os.Mkdir(filepath.Join(root, ".pnp.cjs"), 0755))
				require.NoError(t, os.Mkdir(filepath.Join(root, ".pnp.js"), 0755))
			}
			plan := husky.Resolve(root, "pre-commit", nil, nil)
			require.Equal(t, husky.Skip, plan.Action)
			require.Contains(t, plan.Reason, "node_modules")
			require.Contains(t, plan.Reason, "install")
			require.Empty(t, plan.Argv)
		})
	}
}

func TestResolveInstallLayouts(t *testing.T) {
	t.Parallel()
	for _, layout := range []string{".pnp.cjs", ".pnp.js", "node_modules", "node_modules/.bin"} {
		t.Run(layout, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFile(t, root, ".husky/pre-commit", "printf '%s\\n' \"$PATH\"\n", 0644)
			if strings.HasPrefix(layout, ".pnp.") {
				writeFile(t, root, layout, "", 0644)
			} else {
				require.NoError(t, os.MkdirAll(filepath.Join(root, layout), 0755))
			}
			plan := husky.Resolve(root, "pre-commit", nil, []string{"PATH=/usr/bin:/bin"})
			require.Equal(t, husky.Run, plan.Action)
			wantPath := "/usr/bin:/bin"
			if layout == "node_modules/.bin" {
				wantPath = filepath.Join(root, layout) + string(os.PathListSeparator) + wantPath
			}
			require.Contains(t, plan.Env, "PATH="+wantPath)
			cmd := exec.Command(plan.Argv[0], plan.Argv[1:]...)
			cmd.Dir, cmd.Env = root, plan.Env
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", output)
			require.Equal(t, wantPath+"\n", string(output))
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
		{"split quotes", `. "$(dirname "$0")"/_/husky.sh`},
		{"split quotes with double dash", `. "$(dirname -- "$0")"/_/husky.sh`},
		{"single quotes", `. '.husky/_/husky.sh'`},
		{"unquoted", `. .husky/_/husky.sh`},
		{"source", `source "$(dirname "$0")/_/husky.sh"`},
		{"source single quotes", `source '.husky/_/husky.sh'`},
		{"semicolon", `. '.husky/_/husky.sh'; exit 0`},
		{"CRLF", "#!/bin/sh\r\n. \"$(dirname \"$0\")\"/_/husky.sh\r\nexit 0\r\n"},
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
			writeFile(t, root, ".husky/pre-commit", "printf 'hook\\n' >> \"$COUNTER\"\nprintf '%s\\n' \"$INIT_MARKER\" \"$@\" > \"$MARKER\"\n", tc.mode)
			writeFile(t, root, ".husky/_/pre-commit", ". \"$(dirname \"$0\")/h\"\n", 0644)
			writeFile(t, root, ".husky/_/h", v9Helper, 0644)
			home := t.TempDir()
			writeFile(t, home, ".huskyrc", "exit 99\n", 0644)
			writeFile(t, home, ".config/husky/init.sh", "export INIT_MARKER=loaded\n", 0644)
			require.NoError(t, os.Mkdir(filepath.Join(root, "node_modules"), 0755))
			env := []string{"HOME=" + home, "PATH=/usr/bin:/bin", "COUNTER=" + counter, "MARKER=" + marker}
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
			require.Contains(t, string(output), "DEPRECATED")
			content, err := os.ReadFile(counter)
			require.NoError(t, err)
			require.Equal(t, 1, strings.Count(string(content), "\n"))
			content, err = os.ReadFile(marker)
			require.NoError(t, err)
			require.Equal(t, "loaded\n"+strings.Join(args, "\n")+"\n", string(content))
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
		{name: "indented comment", prefix: " \t# source '.husky/_/husky.sh'\r\n", paths: []string{"/usr/bin:/bin"}},
		{name: "unrelated source", prefix: ". /dev/null # .husky/_/husky.sh\n", paths: []string{"/usr/bin:/bin"}},
		{name: "quoted mention", prefix: ": '. .husky/_/husky.sh'\n", paths: []string{"/usr/bin:/bin"}},
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
			if len(tc.paths) == 0 || tc.paths[len(tc.paths)-1] == "" {
				require.NoError(t, os.Symlink("/bin/sh", filepath.Join(root, "node_modules/.bin/sh")))
			}
			if tc.helper {
				writeFile(t, root, ".husky/_/husky.sh", legacyHelper, 0644)
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
			argv := []string{"sh", "-c", v9Init, filepath.Join(root, ".husky/pre-commit")}
			if tc.helper {
				argv = []string{"sh", filepath.Join(root, ".husky/pre-commit")}
			}
			require.Equal(t, append(argv, args...), plan.Argv)
			require.Empty(t, plan.Reason)
			require.Equal(t, originalEnv, env)
			wantEnv := []string{"MARKER=" + marker, "HUSKY=1"}
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

const legacyHelper = `if [ -z "$husky_skip_init" ]; then
  [ "$HUSKY" = "0" ] && exit 0
  [ -f "$HOME/.huskyrc" ] && . "$HOME/.huskyrc"
  export husky_skip_init=1
  sh -e "$0" "$@"
  exit $?
fi
`

const v9Init = `i="${XDG_CONFIG_HOME:-$HOME/.config}/husky/init.sh"; [ -f "$i" ] && . "$i"; [ "${HUSKY-}" = "0" ] && exit 0; exec sh -e "$0" "$@"`

// v9Helper is Husky 9.1.7's runtime so the test exercises its real dispatch behavior.
const v9Helper = `#!/usr/bin/env sh
[ "$HUSKY" = "2" ] && set -x
n=$(basename "$0")
s=$(dirname "$(dirname "$0")")/$n

[ ! -f "$s" ] && exit 0

if [ -f "$HOME/.huskyrc" ]; then
	echo "husky - '~/.huskyrc' is DEPRECATED, please move your code to ~/.config/husky/init.sh"
fi
i="${XDG_CONFIG_HOME:-$HOME/.config}/husky/init.sh"
[ -f "$i" ] && . "$i"

[ "${HUSKY-}" = "0" ] && exit 0

export PATH="node_modules/.bin:$PATH"
sh -e "$s" "$@"
c=$?

[ $c != 0 ] && echo "husky - $n script failed (code $c)"
[ $c = 127 ] && echo "husky - command not found in PATH=$PATH"
exit $c
`

func TestResolveV9FallbackInit(t *testing.T) {
	t.Parallel()
	for _, config := range []string{"xdg", "home", "empty xdg"} {
		t.Run(config, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			home := t.TempDir()
			env := []string{"HOME=" + home, "PATH=/usr/bin:/bin"}
			configRoot := filepath.Join(home, ".config")
			switch config {
			case "xdg":
				configRoot = filepath.Join(t.TempDir(), "config with spaces")
				env = append(env, "XDG_CONFIG_HOME="+configRoot)
			case "empty xdg":
				env = append(env, "XDG_CONFIG_HOME=")
			}
			writeFile(t, home, ".huskyrc", "exit 99\n", 0644)
			writeFile(t, configRoot, "husky/init.sh", "export INIT_MARKER=loaded\n", 0644)
			writeFile(t, root, ".husky/pre-commit", "printf '%s\\n' \"$INIT_MARKER\" \"$@\"\n", 0644)
			require.NoError(t, os.Mkdir(filepath.Join(root, "node_modules"), 0755))
			plan := husky.Resolve(root, "pre-commit", []string{"arg with spaces", "--flag"}, env)
			require.Equal(t, husky.Run, plan.Action)
			cmd := exec.Command(plan.Argv[0], plan.Argv[1:]...)
			cmd.Dir, cmd.Env = root, plan.Env
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", output)
			require.Equal(t, "loaded\narg with spaces\n--flag\n", string(output))
		})
	}
}

func TestResolveLegacyInit(t *testing.T) {
	t.Parallel()
	for _, source := range []string{`. "$(dirname "$0")/_/husky.sh"`, `. "$(dirname -- "$0")/_/husky.sh"`} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			home := t.TempDir()
			writeFile(t, home, ".huskyrc", "export INIT_MARKER=loaded\n", 0644)
			writeFile(t, root, ".husky/_/husky.sh", legacyHelper, 0644)
			writeFile(t, root, ".husky/pre-commit", source+"\nprintf '%s\\n' \"$INIT_MARKER\" \"$@\"\n", 0644)
			require.NoError(t, os.Mkdir(filepath.Join(root, "node_modules"), 0755))
			plan := husky.Resolve(root, "pre-commit", []string{"arg with spaces", "--flag"}, []string{
				"HOME=" + home, "PATH=/usr/bin:/bin", "husky_skip_init=0", "husky_skip_init=1",
			})
			require.Equal(t, husky.Run, plan.Action)
			cmd := exec.Command(plan.Argv[0], plan.Argv[1:]...)
			cmd.Dir, cmd.Env = root, plan.Env
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", output)
			require.Equal(t, "loaded\narg with spaces\n--flag\n", string(output))
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

func TestResolveLastHuskyEntry(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		env    []string
		action husky.Action
		value  string
	}{
		{"enabled last", []string{"HUSKY=0", "HUSKY=1"}, husky.Run, "1"},
		{"empty last", []string{"HUSKY=0", "HUSKY="}, husky.Run, ""},
		{"disabled last", []string{"HUSKY=1", "HUSKY=0"}, husky.Skip, ""},
		{"unrelated variable", []string{"HUSKY=0", "OTHER_HUSKY=1"}, husky.Skip, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			writeFile(t, root, ".husky/pre-commit", "printf '%s' \"$HUSKY\"\n", 0644)
			require.NoError(t, os.Mkdir(filepath.Join(root, "node_modules"), 0755))
			env := append([]string{"PATH=/usr/bin:/bin"}, tc.env...)
			plan := husky.Resolve(root, "pre-commit", nil, env)
			require.Equal(t, tc.action, plan.Action)
			if tc.action == husky.Skip {
				require.Contains(t, plan.Reason, "HUSKY=0")
				require.Empty(t, plan.Argv)
				return
			}
			cmd := exec.Command(plan.Argv[0], plan.Argv[1:]...)
			cmd.Dir, cmd.Env = root, plan.Env
			output, err := cmd.CombinedOutput()
			require.NoError(t, err, "%s", output)
			require.Equal(t, tc.value, string(output))
		})
	}
}

func TestResolveV9FallbackInitDisablesHusky(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	home := t.TempDir()
	// husky v9 checks HUSKY after sourcing init.sh, so init.sh can turn hooks off.
	writeFile(t, home, ".config/husky/init.sh", "export HUSKY=0\n", 0644)
	writeFile(t, root, ".husky/pre-commit", "echo HOOK-RAN\nexit 7\n", 0644)
	require.NoError(t, os.Mkdir(filepath.Join(root, "node_modules"), 0755))
	plan := husky.Resolve(root, "pre-commit", nil, []string{"HOME=" + home, "PATH=/usr/bin:/bin"})
	require.Equal(t, husky.Run, plan.Action)
	cmd := exec.Command(plan.Argv[0], plan.Argv[1:]...)
	cmd.Dir, cmd.Env = root, plan.Env
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", output)
	require.NotContains(t, string(output), "HOOK-RAN")
}

func TestResolveV9DeprecationStubKeepsErrexit(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	// husky v9 writes this stub to .husky/_/husky.sh; it does not re-run the hook with sh -e.
	writeFile(t, root, ".husky/_/husky.sh", "echo \"husky - DEPRECATED\"\n", 0644)
	writeFile(t, root, ".husky/pre-commit", ". \"$(dirname -- \"$0\")/_/husky.sh\"\nfalse\necho after-false\n", 0644)
	require.NoError(t, os.Mkdir(filepath.Join(root, "node_modules"), 0755))
	plan := husky.Resolve(root, "pre-commit", nil, []string{"HOME=" + t.TempDir(), "PATH=/usr/bin:/bin"})
	require.Equal(t, husky.Run, plan.Action)
	cmd := exec.Command(plan.Argv[0], plan.Argv[1:]...)
	cmd.Dir, cmd.Env = root, plan.Env
	output, err := cmd.CombinedOutput()
	require.Error(t, err, "%s", output)
	require.NotContains(t, string(output), "after-false")
}
