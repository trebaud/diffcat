# CLAUDE.md

## Commands

```bash
go build -o diffcat ./cmd/diffcat   # Build
go run ./cmd/diffcat                     # Run against the current repo
go run ./cmd/diffcat files               # Non-interactive file list
go test ./...                                # Run tests (TUI render invariants)
go vet ./...                                 # Vet
```

## Architecture

A CLI/TUI that visualizes the current branch's diff against master (or main).

Entry point is `cmd/diffcat/main.go` — Cobra root command launches the TUI;
the `files` subcommand prints a non-interactive list. `--base` overrides the
auto-detected base branch.

**`internal/`** — Core logic:
- `git/git.go` — Thin wrappers around the git CLI: base-branch detection,
  merge-base resolution, `ChangedFiles` (merges `--numstat` + `--name-status`,
  plus untracked files), and per-file `FileDiff`. The diff is computed against
  the **merge base** of the base branch and HEAD, so it shows what the branch
  added rather than every change since landed on the base. `BaseRef` resolves
  the diff endpoint: a branch goes through the merge base, but a raw commit/tag
  is used verbatim so `--base <sha>` compares against exactly that commit.
  `Commits` lists the branch's history (`base..HEAD`, newest first, with parent
  SHAs for merge detection and any git `Tags` pointing at each commit, parsed from
  the `%D` ref decoration by the pure `parseTags`) via a separator-framed `git log`
  parsed by the pure
  `parseCommits` — and falls back to HEAD's full history when that range is empty
  (e.g. sitting on the default branch); `CommitDiff` returns one commit's patch
  (`git show --format=`). `CommitFiles`/`CommitFileDiff` are the per-commit
  analogues of `ChangedFiles`/`FileDiff` — they scope a single commit's file list
  and per-file patch via `git show` (reusing the pure `parseNameStatus`/
  `parseNumStat`/`mergeChanges` join), backing the per-commit drill-in.
- `diff/diff.go` — Pure parser of unified `git diff` output. `Parse` yields
  typed, line-numbered `Line`s (Context/Add/Del/Hunk/Meta); `SplitRows` pairs
  each removal block with the additions that follow for the side-by-side view.
  No rendering — the TUI styles these. `Parse` also runs `markWordDiff`: it pairs
  the k-th `Del` with the k-th `Add` in each consecutive Del/Add run and computes
  a token-level LCS diff (`tokenize`/`wordRanges`) between them, recording the
  changed rune ranges on each line's `Emph` field — GitHub's intra-line word
  highlight. A similarity gate leaves `Emph` nil when the two lines share too
  little (a wholesale replacement reads better as a flat full-line tint).
- `tui/` — Bubble Tea (Elm architecture) viewer. Two panes: file tree (left),
  diff (right). Diff scrolling is tracked manually via `diffOffset` rather than
  a viewport component.
  - `model.go` state/helpers, `update.go` key handling, `view.go` rendering,
    `theme.go` semantic colors with light/dark detection, `tree.go` the file-tree
    model, `log.go` the commit-history mode.
  - Three view modes (`viewMode` in `model.go`): `viewBranch` (default),
    `viewLog`, and `viewCommit`. `L` toggles history; launching on the default
    branch (no working-tree changes) opens straight into `viewLog`. In `viewLog`
    (`log.go`) the left pane is the branch's commit list (`base..HEAD`, newest
    first; `●` nodes, `◆` for merges) and the right pane live-previews the
    highlighted commit's full combined diff — `j/k` move the commit cursor
    (`loadCommitDiff`, memoized per SHA in `commitDiffCache`), `Enter` drills into
    that commit (`enterCommit` → `viewCommit`), `l`/Tab focus the preview to
    scroll it, `Esc`/`L` return to the branch view.
    When the working tree is dirty, a synthetic **working-tree row** (`○ local`,
    `workingRow`) is pinned above the commits and the cursor starts on it. It
    represents the uncommitted delta sitting *above* HEAD, so everything about it
    is scoped to `git diff HEAD` (NOT the branch base — diffing against base would
    fold in the changes the individual commit rows already show). It previews the
    combined `git.WorkingDiff` (HEAD) patch, and `Enter` (`enterWorkingTree`)
    drills into the per-commit layout (`viewCommit`) with `scopeWorking` set:
    `CommitFiles`-style the tree comes from `ChangedFiles(repo, "HEAD")` (so it
    includes untracked files) and each file's diff is `git diff HEAD -- path`
    (`loadDiff` branches on `scopeWorking`). Unlike a real commit, that drill-in
    is mutable, so `refresh` rebuilds its file list from `git diff HEAD`. The row
    shifts commit indexing by one — `logWorking`/`logRowCount`/`onWorkingRow`/
    `commitIndex` mediate every cursor↔commit mapping, so `selectedCommit` is nil
    while the cursor sits on it.
    Context-expansion is disabled
    there (the multi-file patch has no single backing file). The combined diff
    *is* syntax-highlighted, per file: `m.lexer` stays nil, and `renderCode`
    instead lexes each line with `lineLexer`, which picks a lexer from the line's
    `Path` (carried by the diff parser from the patch's `+++` headers) and
    memoizes it in `pathLexers`.
  - `viewCommit` is the per-commit drill-in (GitHub's commit page): the left pane
    is the file tree of just that one commit's changes (`CommitFiles`) and the
    right pane shows the selected file's diff scoped to the commit
    (`CommitFileDiff`) — the same file-tree/diff layout as `viewBranch`, so it
    reuses `listView`/`diffView`/`loadDiff` (which branches on `scopeCommit`).
    Because the tree fields are shared, `enterCommit` stashes the branch tree in
    `branchFiles`/`branchRows`/`branchCursor` and `exitCommit` restores it
    verbatim. Context-expansion works here too: the new side is loaded via
    `git.FileContentAt` (`git show <sha>:<path>`), so revealed context matches
    the file as of that commit rather than the working tree, which may have moved
    past it. `Esc` steps back one level: `viewCommit` → `viewLog` → quit; it's
    context-sensitive throughout.
    The right pane reuses the same diff renderer via the shared `diffPane`; only
    the heading differs (commit SHA + subject vs file path).
  - The left pane is a collapsible file tree (`tree.go`): the flat `files` list
    builds a `treeNode` tree, single-child folder chains are compressed
    (`internal/tui/` on one row), and it flattens into `rows []treeRow` — the
    visible lines (folders + files, collapsed branches omitted) the `cursor`
    indexes into. Folders show chevrons, guide rails, and roll-up stats; `enter`/
    `o` folds/unfolds the folder under the cursor (`collapsed` map survives a
    refresh). `treeRow` (view.go) renders one line; selecting a folder shows a
    roll-up in the diff pane instead of a diff.
  - Diffs render GitHub-style: full-row green/red background tints with
    line-number gutters (`renderUnifiedLine`). The exact words that changed within
    a line get a stronger tint (`diffAddEmphBg`/`diffDelEmphBg`, the brighter
    gutter tone): `renderCode` projects each line's `diff.Emph` ranges through tab
    expansion (`expandedMask`) and paints those runs with the emphasis background
    over the soft body band, so a line reads as a soft base tint with the changed
    span popping out. `s` toggles `splitView` for a
    side-by-side layout (`renderSplitRow`/`renderSplitSide`) fed by
    `diff.SplitRows`. `lineDigits` sizes the gutter; row counts differ per mode
    so `totalDiffRows` drives scroll clamping and the nyan progress.
  - Code bodies are syntax-highlighted (`highlight.go`): a Chroma lexer chosen
    from the file path (`lexerFor`) tokenizes each line into colored `span`s,
    memoized per line in `hlCache` (reset on `loadDiff`). Token colors come from
    `tokenColor`, a curated GitHub-flavored palette keyed on Chroma's universal
    token *categories* (keyword/string/number/comment/function/type/…) rather
    than a per-style table — so coverage is uniform across languages (numbers
    stay distinct from strings, identifiers don't collapse to a flat default) and
    `pick` selects the light/dark variant via `applyHighlightTheme`. Adjacent
    spans sharing a color coalesce. `renderCode` (view.go)
    lays the highlighted tokens over the row's background tint and pads to an
    exact width; `expandTabs` normalizes tabs so the width math holds, and
    `lineStyles` returns the per-kind tint that `renderCode` paints beneath the
    tokens.
  - Layout is responsive: proportional list/diff split (list capped at 40 cols),
    a minimum-size gate (`minWidth`/`minHeight` in `view.go`), and all chrome is
    width-clamped so nothing wraps. `render_smoke_test.go` guards the no-wrap
    invariant — every rendered line must be ≤ terminal width. The left pane's
    width is a tri-state `sidebar` (`sidebarHidden`/`sidebarNormal`/`sidebarWide`
    in `model.go`) cycled with `[` (narrow) / `]` (widen): `sidebarWidth()` returns
    0 when hidden (the diff fills the whole width, no divider, focus forced to the
    diff), the normal ~35%/40-cap split, or a ~55%/72-cap wide list that still
    leaves the diff ≥28 cols. In `viewLog` a wide sidebar switches `commitRow` to
    `commitRowWide`, which adds the author/date right-aligned and a subtle-gold tag
    badge (`tagStyle`) after the SHA, dropping the right metadata first when the
    row gets too narrow.
  - `nyanBar` (view.go) pins a nyan-cat scroll-progress indicator to the bottom
    of the diff pane: position = scroll fraction, rainbow trail behind. A
    `tickMsg` loop (~7fps, `update.go`) wiggles the cat's face — the main idle
    repaint driver.
  - Background git sync (`update.go`): a second, slower self-pacing tick
    (`syncInterval`, ~1.5s) runs `git.Fingerprint` in the tick goroutine (off the
    UI thread) and returns a `gitStateMsg`. `Fingerprint` is a cheap state hash —
    HEAD + base tip + porcelain status + each changed path's size/mtime — that
    moves on commits, checkouts, staging, and in-place edits without recomputing
    the diff; the (heavier) `refresh()` runs only when it differs from the last
    poll. `refresh()` (shared with the manual `r` key) is position-preserving:
    `reselectPath`/`reselectCommit` keep the same file/commit selected across the
    rebuild, and `preserveDiffView` holds scroll/cursor/expansion whenever the
    reloaded diff is byte-identical (`diffLinesEqual`), so a sync that didn't
    touch the visible file leaves the reader exactly where they were. In
    `viewCommit` the immutable commit tree is left untouched.
