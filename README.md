# Git Hooks

Git Hooks is a flexible and powerful tool for managing and executing Git hooks across multiple levels of your development environment. It allows for global, local repository-specific, Husky, and standard Git hook configurations, providing a hierarchical approach to Git hook management.

## Compatibility

Git Hooks must be configured as your Git hook manager (via `core.hooksPath`), but is designed to execute hooks from other tools in a hierarchical order:

- **[Husky](https://typicode.github.io/husky/)**: Fully compatible with both modern (`.husky/<hook-name>`) and legacy (`.husky/_/<hook-name>`) Husky formats. Once git-hooks is configured, it will automatically detect and execute your Husky hooks. Your project-specific Husky hooks continue to work without modification.

- **[pre-commit](https://pre-commit.com/)**: Compatible with pre-commit framework. You can use pre-commit for project-specific hooks while using git-hooks for global hooks across all repositories.

- **Standard Git Hooks**: Maintains backwards compatibility with traditional `.git/hooks/` scripts.

**Important**: Git Hooks uses `core.hooksPath` to intercept hook execution. If a repository has a local `core.hooksPath` override (e.g., from Husky's `husky install`), it will bypass git-hooks. Use `git-hooks scan-local --auto-fix` to remove local overrides and ensure git-hooks manages all hook execution.

## Features

- Configure global Git hooks in `~/.git-hooks`
- Support for local repository-specific hooks in `$GIT_DIR/.git-hooks`
- Support for Husky hooks in `.husky` folder (both modern and legacy formats)
- Backwards compatibility with standard Git hooks and pre-commit framework
- Hierarchical execution of hooks (global → local → Husky → standard)
- Easy setup of specific hooks (e.g., gitleaks for pre-commit)
- Scan and manage local `core.hooksPath` overrides

## Installation

You can install Git Hooks using either Go's `install` command or Homebrew.

### Using Go Install

To install Git Hooks using Go's `install` command:

```bash
go install github.com/rudderlabs/git-hooks@latest
```

This command will download the latest version of Git Hooks, compile it, and install the binary in your `$GOPATH/bin` directory. Make sure your `$GOPATH/bin` is in your system's PATH to run `git-hooks` from any location.

If you haven't set GOPATH, the binary will typically be installed in `$HOME/go/bin` on Unix systems or `%USERPROFILE%\go\bin` on Windows.

### Using Homebrew

To install Git Hooks using Homebrew:

```bash
brew install rudderlabs/tap/git-hooks
```

This command will install Git Hooks from `rudderlabs/tap` tap, making it available system-wide.

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

### Scanning for Local Hook Overrides

To scan for repositories with local `core.hooksPath` overrides that may conflict with global hooks:

```bash
git-hooks scan-local [PATH]
```

This will scan the specified directory (or current directory if not specified) for Git repositories that have local `core.hooksPath` configuration, which overrides the global hook setup.

**Options:**

- `--max-depth <num>` - Maximum directory depth to search (default: 10)
- `--verbose` - Show detailed output
- `--auto-fix` - Automatically remove local hooksPath overrides after confirmation

**Example:**

```bash
# Scan current directory
git-hooks scan-local

# Scan a specific directory
git-hooks scan-local ~/projects

# Scan and automatically fix
git-hooks scan-local ~/projects --auto-fix
```

This is useful when you've configured global hooks and want to ensure no repositories have local overrides that would bypass your global hook configuration.

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
