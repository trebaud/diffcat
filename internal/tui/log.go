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

// featureCommitCount is the number of the branch's own commits (base..HEAD) —
// the ones above the base-branch divider. It excludes the base history shown
// below the divider for context, so headings count "what this branch added".
func (m model) featureCommitCount() int {
	if m.baseStart >= 0 {
		return m.baseStart
	}
	return len(m.commits)
}

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

// detailsCommit returns the commit the details modal should describe: the one
// being drilled into (viewCommit) or the one highlighted in the history list
// (viewLog). It is nil outside those modes, on the working-tree row, and while
// drilled into the working tree — none of which is a real commit.
func (m model) detailsCommit() *git.Commit {
	switch m.mode {
	case viewCommit:
		return m.scopeCommit
	case viewLog:
		return m.selectedCommit()
	}
	return nil
}

// enterLog switches into the history view, loading the branch's commits
// (base..HEAD) on first entry. The working tree (staged + unstaged changes) is
// pinned as the first row when there's anything uncommitted, and the cursor
// starts there — that's the most likely thing the reader wants to look at.
func (m *model) enterLog() {
	m.loadCommits()
	m.refreshWorkingCount()
	m.mode = viewLog
	m.focus = focusFiles
	m.commitCursor = 0
	// The diff pane starts closed, so the commit list fills the screen. Its diff
	// is loaded lazily when the reader opens the pane (openLogDiff), not here.
	m.logDiffOpen = false
}

// openLogDiff reveals the history view's diff pane on the right half and moves
// focus to it, loading the highlighted commit's diff (deferred while the pane
// was closed). It's the keymap target for `l`/→/Tab from the full-screen list.
func (m *model) openLogDiff() {
	m.logDiffOpen = true
	m.focus = focusDiff
	m.loadCommitDiff()
}

// closeLogDiff hides the diff pane, handing the whole width back to the commit
// list and returning focus there.
func (m *model) closeLogDiff() {
	m.logDiffOpen = false
	m.focus = focusFiles
}

// refreshWorkingCount recomputes the uncommitted-change count (against HEAD) and
// pins/unpins the working-tree row accordingly. It counts the same files the row
// drills into (git.ChangedFiles against HEAD: staged + unstaged + untracked), so
// the row appears only when the working tree actually has changes — not merely
// because the feature branch differs from its base.
func (m *model) refreshWorkingCount() {
	if files, err := git.ChangedFiles(m.repo, "HEAD"); err == nil {
		m.workingCount = len(files)
	} else {
		m.workingCount = 0
	}
	m.logWorking = m.workingCount > 0
}

// exitLog leaves the history view for the aggregated branch-vs-base diff (the
// `D` view), restoring the diff for the file under the tree cursor.
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
	m.scopeToCommit(c)
	m.loadDiff()
}

// scopeToCommit stashes the branch tree and repurposes the shared tree fields for
// commit c: it loads c's changed files and scopes the diff pipeline to that
// commit (viewCommit). It's the shared setup behind drilling in from the history
// list (enterCommit). The caller loads the diff afterward — directly, or via
// jumpToFile to a chosen path.
func (m *model) scopeToCommit(c *git.Commit) {
	// Stash the branch tree so leaving restores it untouched.
	m.branchFiles = m.files
	m.branchRows = m.rows
	m.branchCursor = m.cursor

	m.scopeCommit = c
	m.scopeWorking = false
	if files, err := git.CommitFiles(m.repo, c.SHA); err == nil {
		m.files = files
	} else {
		m.files = nil
	}
	m.cursor = 0
	m.rebuildTree()
	m.mode = viewCommit
	m.focus = focusFiles
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
	// A commit can land while drilled in (refresh skips the list reload for
	// viewCommit, having only advanced the sync fingerprint), so reload the list on
	// the way back rather than waiting for an unrelated change to move it again.
	m.reloadCommitList()
	// Re-preview only when the diff pane is open; closed, the list is full-screen.
	if m.logDiffOpen {
		m.loadCommitDiff()
	}
}

// baseHistoryLimit caps how many of the base branch's commits are shown below
// the branch's own commits — enough to give context for where the branch forked
// from without dragging in the whole history of a long-lived base like main.
const baseHistoryLimit = 30

// loadCommits (re)reads the branch's commits plus a slice of the base branch's
// history below them (recording the boundary in baseStart) and drops the
// per-commit diff cache so a refresh reflects new history.
func (m *model) loadCommits() {
	if cs, start, err := git.BranchHistory(m.repo, m.base, baseHistoryLimit); err == nil {
		m.commits = cs
		m.baseStart = start
	} else {
		m.commits = nil
		m.baseStart = -1
	}
	m.commitDiffCache = map[string][]diff.Line{}
}

// reloadCommitList re-reads the history list, holding the selected commit by SHA
// (and the working-tree row when it's still present). It is the list-only half of
// a refresh, shared by refresh's viewLog branch and by the returns-to-log exits
// (exitOverview, exitCommit). Those exits leave a mode where the background sync
// already advanced syncFingerprint but refresh deliberately skipped the list
// reload — so without reloading here, a commit made while that mode covered the
// list would never appear, since later polls see no fingerprint change to act on.
func (m *model) reloadCommitList() {
	wasWorking := m.onWorkingRow()
	prevSHA := ""
	if c := m.selectedCommit(); c != nil {
		prevSHA = c.SHA
	}
	m.loadCommits()
	m.refreshWorkingCount()
	// Keep the reader on the working-tree row when it's still present; otherwise
	// re-find the same commit by SHA across any newer commits added on top.
	if wasWorking && m.logWorking {
		m.commitCursor = 0
	} else {
		m.reselectCommit(prevSHA)
	}
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
	// Skip the per-commit diff load while the pane is closed — there's nothing to
	// show, and openLogDiff loads it on demand when the reader opens the pane.
	if m.logDiffOpen {
		m.loadCommitDiff()
	}
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
	m.resetSearch()

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
