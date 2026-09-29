package tui

import (
	"fmt"

	"github.com/trebaud/diffcat/internal/diff"
	"github.com/trebaud/diffcat/internal/git"
)

// visual.go holds the history list's visual selection (vim `v`): marking a
// contiguous run of commits and viewing their combined change, either previewed
// in the diff pane or drilled into as a file tree (viewCommit, scopeRange).
//
// Reconciliation: the range is shown as its net diff — the oldest selected
// commit's first parent against the newest selected commit (or the working tree
// when that row is selected too). When several selected commits touch the same
// lines, only the final state appears, so the latest commit's version wins and a
// change that a later commit reverts cancels out. That is exactly what the range
// contributes, with no overlapping hunks to reconcile.

// commitRange is a resolved visual selection: diff from..to, where an empty to
// is the working tree.
type commitRange struct {
	from, to string
	oldest   *git.Commit // the oldest selected commit (whose parent is from)
	newest   *git.Commit // the newest selected commit; nil when the working tree is the tip
	commits  int         // selected commits, not counting the working-tree row
	working  bool        // the working-tree row is part of the selection
}

// label summarizes the range for pane titles and the header, e.g.
// "3 commits · a1b2c3d..e4f5a6b" or "working tree + 2 commits · a1b2c3d..".
func (r commitRange) label() string {
	noun := "commits"
	if r.commits == 1 {
		noun = "commit"
	}
	if r.working {
		return fmt.Sprintf("working tree + %d %s · %s..", r.commits, noun, r.oldest.Short)
	}
	return fmt.Sprintf("%d %s · %s..%s", r.commits, noun, r.oldest.Short, r.newest.Short)
}

// visualBounds returns the selection's first and last rows (lo ≤ hi) in the
// history list's row indexing. lo is the newest end, since the list runs
// newest-first.
func (m model) visualBounds() (lo, hi int) {
	lo, hi = m.logAnchor, m.commitCursor
	if lo > hi {
		lo, hi = hi, lo
	}
	return lo, hi
}

// inVisual reports whether history row i is inside the active visual selection.
func (m model) inVisual(i int) bool {
	if !m.logVisual {
		return false
	}
	lo, hi := m.visualBounds()
	return i >= lo && i <= hi
}

// rowCommit returns the commit on history row i, or nil for the working-tree row
// (or an out-of-range row).
func (m model) rowCommit(i int) *git.Commit {
	if m.logWorking {
		i--
	}
	if i >= 0 && i < len(m.commits) {
		return &m.commits[i]
	}
	return nil
}

// selectionRange resolves the visual selection into a commitRange. It is nil
// unless the selection spans more than one row — a single-row selection behaves
// exactly like the plain cursor, so the regular single-commit paths handle it.
func (m model) selectionRange() *commitRange {
	if !m.logVisual {
		return nil
	}
	lo, hi := m.visualBounds()
	if lo == hi {
		return nil
	}
	oldest := m.rowCommit(hi)
	if oldest == nil {
		return nil
	}
	r := &commitRange{
		from:    git.RangeBase(m.repo, *oldest),
		oldest:  oldest,
		commits: hi - lo + 1,
	}
	if m.logWorking && lo == 0 {
		r.working = true
		r.commits--
	} else {
		r.newest = m.rowCommit(lo)
		r.to = r.newest.SHA
	}
	return r
}

// toggleVisual starts a visual selection anchored at the cursor, or ends the
// active one. The selection is driven from the list, so focus moves there.
func (m *model) toggleVisual() {
	if m.logVisual {
		m.exitVisual()
		return
	}
	if m.logRowCount() == 0 {
		return
	}
	m.logVisual = true
	m.logAnchor = m.commitCursor
	m.focus = focusFiles
}

// exitVisual drops the visual selection, re-previewing the single commit under
// the cursor when the diff pane is open.
func (m *model) exitVisual() {
	m.logVisual = false
	if m.logDiffOpen {
		m.loadCommitDiff()
	}
}

// loadRangeDiff fills the shared diff state with the selection's net diff (the
// caller has already reset it). Commit-only ranges are immutable, so they're
// memoized in commitDiffCache alongside the per-commit diffs; a range ending at
// the working tree is re-read every time.
func (m *model) loadRangeDiff(r *commitRange) {
	key := "range:" + r.from + ".." + r.to
	if !r.working {
		if lines, ok := m.commitDiffCache[key]; ok {
			m.diff = lines
			m.rebuildView()
			return
		}
	}
	var lines []diff.Line
	if raw := git.RangeDiff(m.repo, r.from, r.to); r.from != "" && raw != "" {
		lines = diff.Parse(raw)
	} else {
		lines = []diff.Line{{Kind: diff.Meta, Text: "(no net textual changes across the selection)"}}
	}
	if !r.working {
		if m.commitDiffCache == nil {
			m.commitDiffCache = map[string][]diff.Line{}
		}
		m.commitDiffCache[key] = lines
	}
	m.diff = lines
	m.rebuildView()
}

// enterRange drills the visual selection into a file tree of the range's net
// change (viewCommit, scoped by scopeRange), mirroring enterCommit. The
// selection is kept, so Esc returns to the list with the same range marked.
func (m *model) enterRange() {
	r := m.selectionRange()
	if r == nil || r.from == "" {
		return
	}
	m.stashBranchTree()
	m.scopeRange = r
	m.scopeCommit = nil
	m.scopeWorking = false
	if files, err := git.RangeFiles(m.repo, r.from, r.to); err == nil {
		m.files = files
	} else {
		m.files = nil
	}
	m.cursor = 0
	m.rebuildTree()
	m.mode = viewCommit
	m.focus = focusFiles
	m.loadDiff()
}

// rowOf returns the history row showing the commit with sha, or -1.
func (m model) rowOf(sha string) int {
	for i, c := range m.commits {
		if c.SHA == sha {
			if m.logWorking {
				return i + 1
			}
			return i
		}
	}
	return -1
}
