# sashi

[![Go](https://img.shields.io/badge/go-1.21%2B-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](#license)
[![Release](https://img.shields.io/github/v/release/trebaud/sashi)](https://github.com/trebaud/sashi/releases)

A terminal UI for reviewing your current branch's diff against `master` (or
`main`). It pairs a collapsible file tree in one pane with a colorized,
scrollable, syntax-highlighted diff in the other.

## Why

Reviewing a branch with `git diff master...` in a pager is a wall of text. sashi
gives you a navigable file tree, GitHub-style background tints, syntax highlighting,
and a side-by-side mode — so you can actually read what your branch changed.

## Features

- Collapsible file tree with per-folder roll-up stats
- GitHub-style diffs: green/red background tints with line-number gutters
- Syntax highlighting via [Chroma](https://github.com/alecthomas/chroma)
- Unified **and** side-by-side views (`s` to toggle)
- Light/dark theme that follows your terminal, toggleable with `t`
- Diff computed against the **merge base** — shows what your branch added, not
  unrelated changes that have since landed on the base
- Includes staged, unstaged, and untracked files
- Non-interactive `files` subcommand for scripting

## Install

```bash
go install github.com/trebaud/sashi/cmd/sashi@latest
```

Make sure `$(go env GOPATH)/bin` is on your `$PATH`.

Or build from source:

```bash
git clone https://github.com/trebaud/sashi.git
cd sashi
./scripts/install.sh
```

The script checks dependencies, builds the binary, and installs it to
`/usr/local/bin` or `~/.local/bin`.

## Usage

```bash
sashi                 # diff the current repo against auto-detected master/main
sashi /path/to/repo   # diff another repo
sashi --base develop  # diff against a specific base ref
sashi files           # print the changed-file list (non-interactive)
```

The diff is computed against the **merge base** of the base branch and your
current `HEAD`, so it shows what your branch introduced — not unrelated changes
that have since landed on the base. Staged, unstaged, and untracked files are
all included.

Diffs are rendered GitHub-style: added lines get a green background tint and
removed lines a red one, with line-number gutters and hunk headers. Press `s`
to toggle between the **unified** (inline) view and a **side-by-side** split
that puts removals on the left and additions on the right.

A nyan cat at the bottom of the diff pane tracks your scroll progress: it
marches from the left edge (top of the file) to the right edge (end of the
diff), trailing a rainbow as you read.

## Keys

Navigation is vim-style. The file tree (left) and diff (right) are separate
panes; `j`/`k`/`gg`/`G` act on whichever pane has focus.

| Key | Action |
| --- | --- |
| `h` / `l` | focus file tree / diff pane |
| `Tab` | toggle focused pane |
| `Enter` / `o` | open diff of the selected file (or fold/unfold a folder) |
| `j` / `k` | move down / up one line in the focused pane |
| `gg` / `G` | jump to top / bottom of the focused pane |
| `ctrl+d` / `ctrl+u` | scroll the diff half a page |
| `ctrl+f` / `ctrl+b` | scroll the diff a full page |
| `s` | toggle unified / side-by-side diff |
| `t` | toggle light / dark theme |
| `r` | refresh from disk |
| `?` | toggle help |
| `q` / `esc` | quit |

## Releasing

Cross-platform release artifacts and the GitHub release are produced by the
[`releaser` skill](.claude/skills/releaser/SKILL.md) and `scripts/`:

```bash
./scripts/releaser.sh v1.2.0   # build dist/*.tar.gz + .sha256, tag, push
./scripts/deploy.sh            # warm the Go module proxy for the latest tag
```

## Requirements

- Go 1.21+
- Git

## Stack

Built on the same TUI stack as [`mori`](../mori): Bubble Tea, Bubbles, and Lip
Gloss (the `charm.land/*/v2` modules), with Cobra for the CLI and Chroma for
syntax highlighting.

## License

MIT — see [LICENSE](LICENSE).
