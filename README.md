# diffcat

[![Go](https://img.shields.io/badge/go-1.21%2B-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](#license)
[![Release](https://img.shields.io/github/v/release/trebaud/diffcat)](https://github.com/trebaud/diffcat/releases)

A terminal UI for reviewing your branch's diff against `master` (or `main`) —
a collapsible file tree on the left, a colorized, syntax-highlighted diff on the
right. *diffcat* is `cat` for diffs — point it at a repo and it shows what your
branch changed.

## Features

- Collapsible file tree with per-folder roll-up stats
- GitHub-style diffs: green/red tints, line-number gutters, syntax highlighting ([Chroma](https://github.com/alecthomas/chroma))
- Unified or side-by-side view (`s`), light/dark theme (`t`)
- Diffs the **merge base** — what your branch added, not what later landed on the base
- Includes staged, unstaged, and untracked files
- Non-interactive `files` subcommand for scripting

## Install

```bash
go install github.com/trebaud/diffcat/cmd/diffcat@latest   # needs $(go env GOPATH)/bin on $PATH
```

From source:

```bash
git clone https://github.com/trebaud/diffcat.git && cd diffcat && ./scripts/install.sh
```

Requires Go 1.21+ and Git.

## Quick start

```bash
cd your-repo
diffcat
```

Launches the TUI against the auto-detected base branch. Navigation is vim-like
(`h`/`j`/`k`/`l`, `gg`/`G`); press `?` in-app for the full keybindings.

## Commands

```bash
diffcat [path] [--base <ref>]         # launch the TUI (path defaults to .)
diffcat files [path] [--base <ref>]   # print the changed-file list, non-interactive
diffcat --version
```

**`--base, -b <ref>`** overrides the base. It accepts any git ref — branch, remote
branch, tag, or commit:

```bash
diffcat -b develop
diffcat -b origin/main
diffcat -b v1.2.0
diffcat -b 3f9a1c2
```

Without `--base`, the base is auto-detected: `origin/HEAD`, else `master`, else `main`.

## License

MIT — see [LICENSE](LICENSE). Built on [Bubble Tea](https://github.com/charmbracelet/bubbletea), Lip Gloss, Cobra, and Chroma.
