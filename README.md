# hopper

Fast Go CLI/TUI to jump across projects.

`hopper` discovers projects from `~/projects` (by default) plus any configured roots, keeps an index cache for near-instant startup, and gives you a compact Bubble Tea interactive picker (not fullscreen).

## Install

```bash
go install ./...
```

This installs `hopper` into your `$GOBIN` (or `$GOPATH/bin`).

Or use the installer script (build + install + zsh wiring), which installs in `/usr/local/bin/` by default:

```bash
scripts/install.zsh
```

For bash (build + install + bash wiring):

```bash
scripts/install.bash
```

## Quick start

```bash
# build index now (optional; hopper auto-builds when needed)
hopper index rebuild

# open compact picker
hopper
```

To make selection change your current shell directory, add zsh integration:

```bash
hopper init zsh >> ~/.zshrc
source ~/.zshrc
```

For bash:

```bash
hopper init bash >> ~/.bashrc
source ~/.bashrc
```

Then use:

```bash
h
```

`h` (shell function) opens picker and runs `cd` in your current shell. The binary is available via `command hopper`.

You can also fuzzy-jump directly:

```bash
h hopper
```

If an argument is provided, `h` uses fuzzy matching and jumps directly. With no argument, it opens the dropdown picker.

Manage picker entries quickly:

```bash
ha ~/projects/new-repo
hr new-repo
```

`ha` calls `hopper add` and `hr` calls `hopper remove`.

Picker keys include arrows + enter/esc and Vim-style keys: `j/k` (move), `ctrl-u/ctrl-d` (page), `gg`/`G` (top/bottom), `q` (cancel), `l` (select), and `d`/`Delete` (remove highlighted entry).

When removing from picker, existing folders ask for confirmation. Missing pinned entries remove immediately.

Picker removal uses the same behavior as `hopper remove`: it unpins the path and adds it to `hidden_projects`.

## Commands

```text
hopper [pick] [--project PATH ...]
hopper add <path>
hopper remove <path-or-query>
hopper list [--project PATH ...]
hopper query <text> [--project PATH ...]
hopper recent [-n N]
hopper index rebuild
hopper init zsh
hopper init bash
```

## Config

Config path: `~/.config/hopper/config.toml`

`hopper` auto-creates this file on first run.

Example:

```toml
roots = ["~/projects", "~/work", "~/oss"]
exclude = [
  "**/.git",
  "**/node_modules",
  "**/dist",
  "**/build",
  "**/target",
]
project_markers = [".git", "go.mod", "package.json", "pyproject.toml", "Cargo.toml", "*.go", "*.py", "*.ts"]
pinned = ["~/projects/hopper"]
hidden_projects = []

fuzzy_confidence_min_score = 25
fuzzy_confidence_min_delta = 10

max_depth = 4
follow_symlinks = false
scan_hidden = false
picker_height = 12
stale_after_mins = 30
```

## Notes on speed

- Cache file: `~/.cache/hopper/index-v1.json`
- `hopper` loads index first, and refreshes only when stale.
- Open-count and recency are tracked to rank frequent projects higher.
- `hopper remove` hides a project globally by adding it to `hidden_projects`.
- `fuzzy_confidence_min_score` and `fuzzy_confidence_min_delta` control auto-pick confidence for fuzzy `query` and `remove` before chooser fallback.
