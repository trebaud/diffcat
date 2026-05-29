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
- `diff/diff.go` — Pure parser of unified `git diff` output. `Parse` yields
  typed, line-numbered `Line`s (Context/Add/Del/Hunk/Meta); `SplitRows` pairs
  each removal block with the additions that follow for the side-by-side view.
  No rendering — the TUI styles these.
- `tui/` — Bubble Tea (Elm architecture) viewer. Two panes: file tree (left),
  diff (right). Diff scrolling is tracked manually via `diffOffset` rather than
  a viewport component.
  - `model.go` state/helpers, `update.go` key handling, `view.go` rendering,
    `theme.go` semantic colors with light/dark detection, `tree.go` the file-tree
    model.
  - The left pane is a collapsible file tree (`tree.go`): the flat `files` list
    builds a `treeNode` tree, single-child folder chains are compressed
    (`internal/tui/` on one row), and it flattens into `rows []treeRow` — the
    visible lines (folders + files, collapsed branches omitted) the `cursor`
    indexes into. Folders show chevrons, guide rails, and roll-up stats; `enter`/
    `o` folds/unfolds the folder under the cursor (`collapsed` map survives a
    refresh). `treeRow` (view.go) renders one line; selecting a folder shows a
    roll-up in the diff pane instead of a diff.
  - Diffs render GitHub-style: full-row green/red background tints with
    line-number gutters (`renderUnifiedLine`). `s` toggles `splitView` for a
    side-by-side layout (`renderSplitRow`/`renderSplitSide`) fed by
    `diff.SplitRows`. `lineDigits` sizes the gutter; row counts differ per mode
    so `totalDiffRows` drives scroll clamping and the nyan progress.
  - Code bodies are syntax-highlighted (`highlight.go`): a Chroma lexer chosen
    from the file path (`lexerFor`) tokenizes each line into colored `span`s,
    memoized per line in `hlCache` (reset on `loadDiff`). `renderCode` (view.go)
    lays the highlighted tokens over the row's background tint and pads to an
    exact width; `expandTabs` normalizes tabs so the width math holds. The Chroma
    style follows light/dark (`applyHighlightTheme`: github / github-dark), and
    `lineStyles` returns the per-kind tint that `renderCode` paints beneath the
    tokens.
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
