package tui

import (
	"github.com/alecthomas/chroma/v2"

	"github.com/trebaud/diffcat/internal/diff"
	"github.com/trebaud/diffcat/internal/git"
)

// log.go holds the commit-history view (viewLog): entering/leaving the mode,
// the commit under the cursor, and loading that commit's diff into the shared
// diff pipeline. Rendering lives in view.go (commitListView); the right pane
// reuses the same diff renderer the branch view uses.

// logRowCount is the number of selectable rows in the history list: the commits
// plus the optional leading "working tree" entry.
func (m model) logRowCount() int {
	if m.logWorking {
		return len(m.commits) + 1
	}
	return len(m.commits)
}

// onWorkingRow reports whether the history cursor sits on the synthetic
// working-tree entry (always row 0 when present).
func (m model) onWorkingRow() bool {
	return m.logWorking && m.commitCursor == 0
}

// commitIndex maps the history cursor onto an index into m.commits, accounting
// for the leading working-tree row; it is -1 when the cursor is on that row.
func (m model) commitIndex() int {
	if m.logWorking {
		return m.commitCursor - 1
	}
	return m.commitCursor
}

// selectedCommit returns the commit under the history cursor, or nil when the
// cursor is on the working-tree row or there are no commits (e.g. HEAD sits on
// the base branch).
func (m model) selectedCommit() *git.Commit {
	i := m.commitIndex()
	if i >= 0 && i < len(m.commits) {
		return &m.commits[i]
	}
	return nil
}

// enterLog switches into the history view, loading the branch's commits
// (base..HEAD) on first entry. The working tree (staged + unstaged changes) is
// pinned as the first row when there's anything uncommitted, and the cursor
// starts there — that's the most likely thing the reader wants to look at.
func (m *model) enterLog() {
	m.loadCommits()
	m.logWorking = len(m.files) > 0
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

// enterWorkingTree drills the history list's working-tree row into a file tree of
// just the uncommitted changes (staged + unstaged + untracked — the diff against
// HEAD), reusing the per-commit drill-in layout (viewCommit). Like enterCommit it
// stashes the branch tree so exitCommit restores it verbatim; scopeWorking scopes
// loadDiff/refresh to `git diff HEAD` rather than the branch base.
func (m *model) enterWorkingTree() {
	m.branchFiles = m.files
	m.branchRows = m.rows
	m.branchCursor = m.cursor

	m.scopeWorking = true
	m.scopeCommit = nil
	if files, err := git.ChangedFiles(m.repo, "HEAD"); err == nil {
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

// enterCommit drills from the history list into a per-commit file tree: it
// stashes the branch tree, loads the highlighted commit's changed files into the
// shared tree fields, and shows the first file's diff scoped to that commit. A
// commit with no per-file entries (e.g. a merge) still enters — the tree is just
// empty.
func (m *model) enterCommit() {
	c := m.selectedCommit()
	if c == nil {
		return
	}
	// Stash the branch tree so leaving restores it untouched.
	m.branchFiles = m.files
	m.branchRows = m.rows
	m.branchCursor = m.cursor

	m.scopeCommit = c
	if files, err := git.CommitFiles(m.repo, c.SHA); err == nil {
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

// exitCommit returns from a per-commit tree to the history list, restoring the
// stashed branch tree and re-previewing the commit's combined diff.
func (m *model) exitCommit() {
	m.scopeCommit = nil
	m.scopeWorking = false
	m.files = m.branchFiles
	m.rows = m.branchRows
	m.cursor = m.branchCursor
	m.branchFiles = nil
	m.branchRows = nil
	m.mode = viewLog
	m.focus = focusFiles
	m.loadCommitDiff()
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
	if m.logRowCount() == 0 {
		return
	}
	m.commitCursor += delta
	if m.commitCursor < 0 {
		m.commitCursor = 0
	}
	if m.commitCursor >= m.logRowCount() {
		m.commitCursor = m.logRowCount() - 1
	}
	m.loadCommitDiff()
}

// loadCommitDiff parses the diff for the highlighted commit into the shared diff
// state and resets scroll. The patch spans multiple files, so context expansion
// is disabled (no single backing file) and m.lexer stays nil — instead each line
// is highlighted per file via lineLexer/pathLexers, keyed on the Path the diff
// parser carries from the patch's `+++` headers. Results are memoized per SHA.
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
	m.pathLexers = map[string]chroma.Lexer{}

	// The working-tree row previews the combined branch diff against base. Like a
	// commit's combined diff it spans multiple files, so it rides the same
	// per-path highlighting (m.lexer nil, pathLexers keyed on each line's Path).
	if m.onWorkingRow() {
		if raw := git.WorkingDiff(m.repo); raw != "" {
			m.diff = diff.Parse(raw)
		} else {
			m.diff = []diff.Line{{Kind: diff.Meta, Text: "(no textual changes)"}}
		}
		m.rebuildView()
		return
	}

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
