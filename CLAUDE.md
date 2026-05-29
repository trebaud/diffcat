# CLAUDE.md

## Commands

```bash
go build -o diff-master ./cmd/diff-master   # Build
go run ./cmd/diff-master                     # Run against the current repo
go run ./cmd/diff-master files               # Non-interactive file list
go test ./...                                # Run tests (TUI render invariants)
go vet ./...                                 # Vet
```

## Architecture

A CLI/TUI that visualizes the current branch's diff against master (or main).

Entry point is `cmd/diff-master/main.go` — Cobra root command launches the TUI;
the `files` subcommand prints a non-interactive list. `--base` overrides the
auto-detected base branch.

**`internal/`** — Core logic:
- `git/git.go` — Thin wrappers around the git CLI: base-branch detection,
  merge-base resolution, `ChangedFiles` (merges `--numstat` + `--name-status`,
  plus untracked files), and per-file `FileDiff`. The diff is computed against
  the **merge base** of the base branch and HEAD, so it shows what the branch
  added rather than every change since landed on the base.
- `tui/` — Bubble Tea (Elm architecture) viewer. Two panes: file list (left),
  colorized unified diff (right). Diff scrolling is tracked manually via
  `diffOffset` rather than a viewport component.
  - `model.go` state/helpers, `update.go` key handling, `view.go` rendering,
    `theme.go` semantic colors (added/removed/meta) with light/dark detection.
  - Layout is responsive: proportional list/diff split (list capped at 40 cols),
    a minimum-size gate (`minWidth`/`minHeight` in `view.go`), and all chrome is
    width-clamped so nothing wraps. `render_smoke_test.go` guards the no-wrap
    invariant — every rendered line must be ≤ terminal width.
  - `nyanBar` (view.go) pins a nyan-cat scroll-progress indicator to the bottom
    of the diff pane: position = scroll fraction, rainbow trail behind. A
    `tickMsg` loop (~7fps, `update.go`) wiggles the cat's face; it's the only
    thing driving repaints, so there's no idle redraw beyond it.

## Stack

Same TUI stack as the sibling `mori` project: `charm.land/bubbletea/v2`,
`charm.land/bubbles/v2`, `charm.land/lipgloss/v2`, and `github.com/spf13/cobra`.
