# sashi

[![Go](https://img.shields.io/badge/go-1.21%2B-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](#license)
[![Release](https://img.shields.io/github/v/release/trebaud/sashi)](https://github.com/trebaud/sashi/releases)

A terminal UI for reviewing your branch's diff against `master` (or `main`) —
a collapsible file tree on the left, a colorized, syntax-highlighted diff on the
right. *sashi* (差し) means "difference" in Japanese.

## Features

- Collapsible file tree with per-folder roll-up stats
- GitHub-style diffs: green/red tints, line-number gutters, syntax highlighting ([Chroma](https://github.com/alecthomas/chroma))
- Unified or side-by-side view (`s`), light/dark theme (`t`)
- Diffs the **merge base** — what your branch added, not what later landed on the base
- Includes staged, unstaged, and untracked files
- Non-interactive `files` subcommand for scripting

## Install

```bash
go install github.com/trebaud/sashi/cmd/sashi@latest   # needs $(go env GOPATH)/bin on $PATH
```

From source:

```bash
git clone https://github.com/trebaud/sashi.git && cd sashi && ./scripts/install.sh
```

Requires Go 1.21+ and Git.

## Quick start

```bash
cd your-repo
sashi
```

Launches the TUI against the auto-detected base branch. Navigation is vim-like
(`h`/`j`/`k`/`l`, `gg`/`G`); press `?` in-app for the full keybindings.

## Commands

```bash
sashi [path] [--base <ref>]         # launch the TUI (path defaults to .)
sashi files [path] [--base <ref>]   # print the changed-file list, non-interactive
sashi --version
```

**`--base, -b <ref>`** overrides the base. It accepts any git ref — branch, remote
branch, tag, or commit:

```bash
sashi -b develop
sashi -b origin/main
sashi -b v1.2.0
sashi -b 3f9a1c2
```

Without `--base`, the base is auto-detected: `origin/HEAD`, else `master`, else `main`.

## License

MIT — see [LICENSE](LICENSE). Built on [Bubble Tea](https://github.com/charmbracelet/bubbletea), Lip Gloss, Cobra, and Chroma.
