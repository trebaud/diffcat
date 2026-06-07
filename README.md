<div align="center">

# 🐱 diffcat

**`cat`, but for git diffs.**

A terminal diff viewer that also tells you how much of your code was written by AI.

[![CI](https://github.com/trebaud/diffcat/actions/workflows/ci.yml/badge.svg)](https://github.com/trebaud/diffcat/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/go-1.25%2B-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](#license)
[![Release](https://img.shields.io/github/v/release/trebaud/diffcat)](https://github.com/trebaud/diffcat/releases)

![diffcat browsing a branch diff](assets/browse.gif)

</div>

## What it does

Run `diffcat` in any repo and it opens on your branch's commit history, GitHub-style, in
the terminal, with vim keys. Press `D` for the aggregated branch-vs-base diff, `S` for a
whole-repo dashboard that ranks every contributor and shows how much of the code was
written by AI agents (Claude, Copilot, Cursor, Devin, Aider, Codex, ChatGPT), detected
from author and `Co-authored-by` trailers.

## Why diffcat?

There are good diff tools already: [delta](https://github.com/dandavison/delta),
[difftastic](https://github.com/Wilfred/difftastic),
[lazygit](https://github.com/jesseduffield/lazygit), [tig](https://github.com/jonas/tig).
diffcat is narrower on purpose.

It opens with no arguments, auto-detecting the base to diff against (`origin/HEAD`, then
`master`, then `main`). No flags, no gitconfig pager setup.

The stats dashboard is the part the others don't have: a contribution calendar,
per-author ranking, hour-of-day and weekday activity, streaks, and a human-vs-AI commit
split. As more code gets written by agents, "how much of this did a human write?" is
worth being able to answer, and diffcat answers it from your git history, per agent.

It ships as a single static binary with no runtime dependencies.

## Features

- TUI with native vim keybindings
- GitHub-style diffs: green/red tints, line-number gutters, syntax highlighting, surrounding-line expansion
- Unified or side-by-side view (`s`), light/dark theme (`t`)
- Auto-detected base, or any ref via `--base`
- Commit history view with per-commit diffs
- Stats dashboard (`S`): contribution calendar, per-author ranking, activity charts, streaks, and a human-vs-AI split that names each agent

## Demo

![commit details modal and the whole-repo stats dashboard](assets/commit-details-stats.gif)

![fuzzy find and in-diff search](assets/find.gif)

## Install

Prebuilt binary (no Go required). Download the tarball for your platform from the
[latest release](https://github.com/trebaud/diffcat/releases/latest)
(`darwin-amd64`, `darwin-arm64`, `linux-amd64`, `linux-arm64`), then:

```bash
tar -xzf diffcat-*-darwin-arm64.tar.gz   # match the file you downloaded
sudo mv diffcat /usr/local/bin/
```

With Go (1.25+):

```bash
go install github.com/trebaud/diffcat/cmd/diffcat@latest
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

`--base, -b <ref>` overrides the base. It accepts any git ref: a branch, remote
branch, tag, or commit.

```bash
diffcat -b develop
diffcat -b origin/main
diffcat -b v1.2.0
diffcat -b 3f9a1c2
```

Without `--base`, the base is auto-detected: `origin/HEAD`, else `master`, else `main`.

## License

MIT, see [LICENSE](LICENSE). Built on [Bubble Tea](https://github.com/charmbracelet/bubbletea), Lip Gloss, Cobra, and Chroma.
