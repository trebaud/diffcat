# diff-master

A terminal UI for reviewing your current branch's diff against `master` (or
`main`). It lists the changed files in one pane and shows a colorized,
scrollable unified diff in the other.

## Install

```bash
go install github.com/trebaud/diff-master/cmd/diff-master@latest
```

Or build from source:

```bash
go build -o diff-master ./cmd/diff-master
```

## Usage

```bash
diff-master                 # diff the current repo against auto-detected master/main
diff-master /path/to/repo   # diff another repo
diff-master --base develop  # diff against a specific base ref
diff-master files           # print the changed-file list (non-interactive)
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

Navigation is vim-style. The file list (left) and diff (right) are separate
panes; `j`/`k`/`gg`/`G` act on whichever pane has focus.

| Key | Action |
| --- | --- |
| `h` / `l` | focus file list / diff pane |
| `Tab` | toggle focused pane |
| `Enter` | open diff of the selected file |
| `j` / `k` | move down / up one line in the focused pane |
| `gg` / `G` | jump to top / bottom of the focused pane |
| `ctrl+d` / `ctrl+u` | scroll the diff half a page |
| `ctrl+f` / `ctrl+b` | scroll the diff a full page |
| `s` | toggle unified / side-by-side diff |
| `r` | refresh from disk |
| `?` | toggle help |
| `q` / `esc` | quit |

## Stack

Built on the same TUI stack as [`mori`](../mori): Bubble Tea, Bubbles, and Lip
Gloss (the `charm.land/*/v2` modules), with Cobra for the CLI.
