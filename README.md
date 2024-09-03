# Git Hooks

Git Hooks is a flexible and powerful tool for managing and executing Git hooks across multiple levels of your development environment. It allows for global, local repository-specific, and standard Git hook configurations, providing a hierarchical approach to Git hook management.

## Features

- Configure global Git hooks in `~/.git-hooks`
- Support for local repository-specific hooks in `$GIT_DIR/.git-hooks`
- Backwards compatibility with standard Git hooks
- Hierarchical execution of hooks (global → local → standard)
- Easy setup of specific hooks (e.g., gitleaks for pre-commit)

## Installation

To install Git Hooks, you can use Go's `install` command:

```bash
go install github.com/lvrach/git-hooks@latest
```

This command will download the latest version of Git Hooks, compile it, and install the binary in your `$GOPATH/bin` directory. Make sure your `$GOPATH/bin` is in your system's PATH to run `git-hooks` from any location.

If you haven't set GOPATH, the binary will typically be installed in `$HOME/go/bin` on Unix systems or `%USERPROFILE%\go\bin` on Windows.

## Configuration

To configure Git to use Git Hooks:

```
git-hooks --configure
```

This command will:
- Create the `~/.git-hooks` directory
- Set up scripts for all Git hook types in this directory
- Configure Git to use this directory for hooks

## Usage

### Running Hooks

Once configured, Git Hooks will automatically handle Git hooks. When Git triggers a hook, it will run the corresponding script in `~/.git-hooks`, which in turn calls `git-hooks hook <hook-name>`.

### Setting Up Specific Hooks

To set up specific hooks, use the `setup` command. For example, to set up the gitleaks pre-commit hook:

```bash
git-hooks setup gitleaks
```

This will create a pre-commit hook that runs gitleaks to check for sensitive information in your commits.

### Custom Hook Scripts

You can add custom hook scripts in the following locations:

1. Global hooks: `~/.git-hooks/<hook-name>.d/`
2. Local repository hooks: `$GIT_DIR/.git-hooks/<hook-name>.d/`

These scripts will be executed in order when the corresponding hook is triggered.

## Hook Execution Order

When a Git hook is triggered, Git Hooks executes hooks in the following order:

1. Global hooks in `~/.git-hooks/<hook-name>.d/`
2. Local repository hooks in `$GIT_DIR/.git-hooks/<hook-name>.d/`
3. Standard Git hook in `$GIT_DIR/hooks/<hook-name>`