# CLAUDE.md

## Commands

```bash
go build -o diff-master ./cmd/diff-master   # Build
go run ./cmd/diff-master                     # Run against the current repo
go run ./cmd/diff-master files               # Non-interactive file list
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

## Stack

Same TUI stack as the sibling `mori` project: `charm.land/bubbletea/v2`,
`charm.land/bubbles/v2`, `charm.land/lipgloss/v2`, and `github.com/spf13/cobra`.
