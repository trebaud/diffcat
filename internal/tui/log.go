package tui

import (
	"github.com/trebaud/sashi/internal/diff"
	"github.com/trebaud/sashi/internal/git"
)

// log.go holds the commit-history view (viewLog): entering/leaving the mode,
// the commit under the cursor, and loading that commit's diff into the shared
// diff pipeline. Rendering lives in view.go (commitListView); the right pane
// reuses the same diff renderer the branch view uses.

// selectedCommit returns the commit under the history cursor, or nil when there
// are no commits (e.g. HEAD sits on the base branch).
func (m model) selectedCommit() *git.Commit {
	if m.commitCursor >= 0 && m.commitCursor < len(m.commits) {
		return &m.commits[m.commitCursor]
	}
	return nil
}

// enterLog switches into the history view, loading the branch's commits
// (base..HEAD) on first entry and previewing the newest one.
func (m *model) enterLog() {
	m.loadCommits()
	m.mode = viewLog
	m.focus = focusFiles
	m.commitCursor = 0
	m.loadCommitDiff()
}

// exitLog returns to the default branch-vs-base view, restoring the diff for the
// file under the tree cursor.
func (m *model) exitLog() {
	m.mode = viewBranch
	m.focus = focusFiles
	m.loadDiff()
}

// loadCommits (re)reads base..HEAD and drops the per-commit diff cache so a
// refresh reflects new history.
func (m *model) loadCommits() {
	if cs, err := git.Commits(m.repo, m.base); err == nil {
		m.commits = cs
	} else {
		m.commits = nil
	}
	m.commitDiffCache = map[string][]diff.Line{}
}

// moveCommitCursor moves the history selection and previews the new commit's
// diff, clamped to the list.
func (m *model) moveCommitCursor(delta int) {
	if len(m.commits) == 0 {
		return
	}
	m.commitCursor += delta
	if m.commitCursor < 0 {
		m.commitCursor = 0
	}
	if m.commitCursor >= len(m.commits) {
		m.commitCursor = len(m.commits) - 1
	}
	m.loadCommitDiff()
}

// loadCommitDiff parses the diff for the highlighted commit into the shared diff
// state and resets scroll. The patch spans multiple files, so context expansion
// is disabled (no single backing file) and no lexer is set — a combined patch
// has no one language to highlight. Results are memoized per SHA.
func (m *model) loadCommitDiff() {
	m.diffOffset = 0
	m.diffCursor = 0
	m.diff = nil
	m.fileLines = nil
	m.gaps = nil
	m.revealed = map[int][2]int{}
	m.viewLines = nil
	m.splitRows = nil
	m.lexer = nil
	m.hlCache = map[string][]span{}

	c := m.selectedCommit()
	if c == nil {
		m.rebuildView()
		return
	}

	lines, ok := m.commitDiffCache[c.SHA]
	if !ok {
		if raw := git.CommitDiff(m.repo, c.SHA); raw != "" {
			lines = diff.Parse(raw)
		} else {
			note := "(no textual diff)"
			if c.IsMerge() {
				note = "(merge commit — no combined diff)"
			}
			lines = []diff.Line{{Kind: diff.Meta, Text: note}}
		}
		if m.commitDiffCache == nil {
			m.commitDiffCache = map[string][]diff.Line{}
		}
		m.commitDiffCache[c.SHA] = lines
	}
	m.diff = lines
	m.rebuildView()
}
