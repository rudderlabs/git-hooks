#!/bin/sh
# Run git-hooks with the full path
"{{.GitHooksPath}}" hook {{.HookName}} "$@"