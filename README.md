<div align="center">

# 🐱 diffcat

**`cat`, but for git diffs.**

[![Go](https://img.shields.io/badge/go-1.21%2B-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](#license)
[![Release](https://img.shields.io/github/v/release/trebaud/diffcat)](https://github.com/trebaud/diffcat/releases)

<!-- TODO: record a demo. Drop a ~10s asciinema/GIF showing: launch → browse tree →
     toggle split (s) → open commit history (L). Save to docs/demo.gif and uncomment:
![diffcat demo](docs/demo.gif)
-->

</div>

## Features

- TUI with vim native keybindings
- Collapsible file tree with per-folder roll-up stats
- GitHub-style diffs: green/red tints, line-number gutters, syntax highlighting
- Unified or side-by-side view (`s`), light/dark theme (`t`)
- Diffs against the **merge base**, so you see what your branch added rather than what later landed on it
- Includes staged, unstaged, and untracked files

## Install

```bash
go install github.com/trebaud/diffcat/cmd/diffcat@latest   # needs $(go env GOPATH)/bin on $PATH
```

From source:

```bash
git clone https://github.com/trebaud/diffcat.git && cd diffcat && ./scripts/install.sh
```

## Commands

```bash
diffcat [path] [--base <ref>]         # launch the TUI (path defaults to .)
diffcat files [path] [--base <ref>]   # print the changed-file list, non-interactive
```

**`--base, -b <ref>`** overrides the base. It accepts any git ref: a branch, remote
branch, tag, or commit.

```bash
diffcat -b develop
diffcat -b origin/main
diffcat -b v1.2.0
diffcat -b 3f9a1c2
```

Without `--base`, the base is auto-detected: `origin/HEAD`, else `master`, else `main`.

## License

MIT — see [LICENSE](LICENSE). Built on [Bubble Tea](https://github.com/charmbracelet/bubbletea), Lip Gloss, Cobra, and Chroma.
