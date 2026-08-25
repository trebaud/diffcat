<div align="center">

# 🐱 diffcat

**`cat`, but for git diffs.**

[![CI](https://github.com/trebaud/diffcat/actions/workflows/ci.yml/badge.svg)](https://github.com/trebaud/diffcat/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/go-1.25%2B-00ADD8?logo=go)](https://go.dev)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](#license)
[![Release](https://img.shields.io/github/v/release/trebaud/diffcat)](https://github.com/trebaud/diffcat/releases)

![diffcat browsing a branch diff](assets/browse.gif)

</div>

## Why diffcat?

There are good diff tools already: [delta](https://github.com/dandavison/delta),
[difftastic](https://github.com/Wilfred/difftastic),
[lazygit](https://github.com/jesseduffield/lazygit), [tig](https://github.com/jonas/tig).
diffcat is narrower on purpose, its main focus is on making the git diff view experience delighful. 


## Features

- TUI with native vim keybindings
- GitHub-style diffs: green/red tints, line-number gutters, syntax highlighting, surrounding-line expansion
- Unified or side-by-side view (`s`); a theme picker (`t`) with 8 color themes, light/dark, file-type icons, and a reduce-motion toggle
- Auto-detected base, or any ref via `--base`
- `e` opens the file under the cursor in your own editor, at the line you were reading
- Commit history view with per-commit diffs
- Stats dashboard (`S`): contribution calendar, per-author ranking, activity charts, streaks, and a human-vs-AI split that names each agent — over the last week, month, 6 months, year, or all time (`1`–`5`, or `[`/`]`)

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

## Open in your editor

Press `e` on a file (or on a line in the diff) to hand the terminal to your
editor, opened at that line. diffcat suspends while the editor runs and reloads
the diff from disk when you quit it, so an edit shows up immediately.

It's vim unless you say otherwise. Name another in
`~/.config/diffcat/config.json`:

```json
{
  "editor": "nvim"
}
```

The value is a command, so flags are fine (`"code -w"`, `"emacsclient -nw"`).
Precedence is `--editor` → `DIFFCAT_EDITOR` → config → `$VISUAL` → `$EDITOR` →
`vim`.

The line number is passed the way each editor expects it: `+N` for the vim
family, nano, micro, kak and emacs; `--goto file:N` for VS Code and its forks;
`file:N` for Sublime, Helix and Zed. Anything else is handed just the path.

## Themes & appearance

Press `t` in the TUI for the theme picker: arrow through the themes with a live
preview, `T` flips light/dark, `i` cycles the file-type icon set, `m` toggles
animations, `↵` keeps the choice (persisted), `esc` reverts. Your selection is
saved to `~/.config/diffcat/config.json` and restored on the next run.

You can also set appearance up front, via flags or environment variables:

```bash
diffcat --theme dracula           # github (default), dracula, nord, catppuccin,
                                  # gruvbox, tokyonight, solarized, monochrome
diffcat --icons nerd              # ascii (default), unicode, nerd (needs a Nerd Font)
diffcat --no-anim                 # freeze the nyan cat, pulse, and shimmer

DIFFCAT_THEME=nord diffcat        # env equivalents: DIFFCAT_THEME, DIFFCAT_ICONS,
                                  # DIFFCAT_NO_ANIM
```

Precedence is flag → environment → saved config → terminal auto-detection.
[`NO_COLOR`](https://no-color.org) is honored: when set, diffcat forces the
monochrome theme and freezes all motion.

## License

MIT, see [LICENSE](LICENSE). Built on [Bubble Tea](https://github.com/charmbracelet/bubbletea), Lip Gloss, Cobra, and Chroma.
