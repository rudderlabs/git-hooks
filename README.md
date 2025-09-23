# Git Hooks

Git Hooks is a flexible and powerful tool for managing and executing Git hooks across multiple levels of your development environment. It allows for global, local repository-specific, Husky, and standard Git hook configurations, providing a hierarchical approach to Git hook management.

## Features

- Configure global Git hooks in `~/.git-hooks`
- Support for local repository-specific hooks in `$GIT_DIR/.git-hooks`
- Support for Husky hooks in `.husky` folder (both modern and legacy formats)
- Backwards compatibility with standard Git hooks
- Hierarchical execution of hooks (global → local → Husky → standard)
- Easy setup of specific hooks (e.g., gitleaks for pre-commit)

## Installation

You can install Git Hooks using either Go's `install` command or Homebrew.

### Using Go Install

To install Git Hooks using Go's `install` command:

```bash
go install github.com/lvrach/git-hooks@latest
```

This command will download the latest version of Git Hooks, compile it, and install the binary in your `$GOPATH/bin` directory. Make sure your `$GOPATH/bin` is in your system's PATH to run `git-hooks` from any location.

If you haven't set GOPATH, the binary will typically be installed in `$HOME/go/bin` on Unix systems or `%USERPROFILE%\go\bin` on Windows.

### Using Homebrew

To install Git Hooks using Homebrew:

```bash
brew install lvrach/tap/git-hooks
```

This command will install Git Hooks from the lvrach tap, making it available system-wide.

## Configuration

To configure Git to use Git Hooks:

```bash
git-hooks config
```

This command will:

- Create the `~/.git-hooks` directory
- Set up scripts for all Git hook types in this directory
- Configure Git to use this directory for hooks

### Reverting Configuration

To revert the changes made by the `git-hooks config` command:

```bash
git-hooks implode
```

## Usage

### Running Hooks

Once configured, Git Hooks will automatically handle Git hooks. When Git triggers a hook, it will run the corresponding script in `~/.git-hooks`, which in turn calls `git-hooks hook <hook-name>`.

### Adding Supported Hooks

To set up supported hooks, use the `add` command. For example, to set up the gitleaks pre-commit hook:

```bash
git-hooks add gitleaks
```

This will create a pre-commit hook on a global level that runs gitleaks to check for sensitive information in your commits.

### Custom Hook Scripts

You can add custom hook scripts in the following locations:

1. Global hooks: `~/.git-hooks/<hook-name>.d/`
2. Local repository hooks: `$GIT_DIR/.git-hooks/<hook-name>.d/`

These scripts will be executed in order when the corresponding hook is triggered.

## Hook Execution Order

When a Git hook is triggered, Git Hooks executes hooks in the following order:

1. Global hooks in `~/.git-hooks/<hook-name>.d/`
2. Local repository hooks in `$GIT_DIR/.git-hooks/<hook-name>.d/`
3. Husky hooks in `.husky/<hook-name>` (modern) or `.husky/_/<hook-name>` (legacy)
4. Standard Git hook in `$GIT_DIR/hooks/<hook-name>`

This order ensures that you can have a cascading set of hooks, from the most global to the most specific, with Husky integration for projects that use it.
