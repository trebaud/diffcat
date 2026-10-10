package tui

import (
	"strconv"

	tea "charm.land/bubbletea/v2"

	"github.com/trebaud/diffcat/internal/diff"
)

// nav.go holds the review motions layered over the basic j/k keymap: vim count
// prefixes, stepping between change blocks (`]c` / `[c`) and files (`]f` / `[f`,
// `J` / `K`), the Esc back-stack for the branch diff, copying what's under the
// cursor (`y`), and mouse-wheel scrolling.

// takeCount consumes the pending count prefix, returning it (1 when none was
// typed) and whether one was given.
func (m *model) takeCount() (int, bool) {
	s := m.count
	m.count = ""
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return 1, false
	}
	return n, true
}

// isChange reports whether l is an added or removed line.
func isChange(l *diff.Line) bool {
	return l != nil && (l.Kind == diff.Add || l.Kind == diff.Del)
}

// rowIsChange reports whether display row i (in the active view mode) carries an
// added or removed line on either side.
func (m model) rowIsChange(i int) bool {
	if m.splitView {
		if i < 0 || i >= len(m.splitRows) {
			return false
		}
		r := m.splitRows[i]
		return isChange(r.Left) || isChange(r.Right)
	}
	if i < 0 || i >= len(m.viewLines) {
		return false
	}
	return isChange(&m.viewLines[i])
}

// changeStart reports whether row i opens a contiguous block of changed lines —
// the stops `]c` / `[c` step between.
func (m model) changeStart(i int) bool {
	return m.rowIsChange(i) && !m.rowIsChange(i-1)
}

// fileStart reports whether row i is the first row of a file's section in a
// combined multi-file patch (the history preview). Rows synthesized by BuildView
// carry no path and never start a section.
func (m model) fileStart(i int) bool {
	l := m.rowLine(i)
	if l == nil || l.Path == "" {
		return false
	}
	for j := i - 1; j >= 0; j-- {
		if p := m.rowLine(j); p != nil && p.Path != "" {
			return p.Path != l.Path
		}
	}
	return true
}

// stepRows moves the diff cursor n stops in direction dir (±1), where a stop is
// any row for which isStop holds. It reports whether the cursor moved; when it
// did, the diff pane takes focus and the stop is scrolled into view.
func (m *model) stepRows(dir, n int, isStop func(int) bool) bool {
	total := m.totalDiffRows()
	i := m.diffCursor
	for ; n > 0; n-- {
		j := i + dir
		for j >= 0 && j < total && !isStop(j) {
			j += dir
		}
		if j < 0 || j >= total {
			break
		}
		i = j
	}
	if i == m.diffCursor {
		return false
	}
	m.focus = focusDiff
	m.revealRow(i)
	return true
}

// revealRow puts the diff cursor on row i. A row already on screen just gets the
// cursor; one off screen is scrolled to a quarter of the way down the pane, so a
// jump lands with a little leading context rather than pinned to an edge.
func (m *model) revealRow(i int) {
	m.diffCursor = i
	m.clampDiffCursor()
	if m.diffCursor < m.diffOffset || m.diffCursor > m.lastVisibleRow() {
		m.diffOffset = m.diffCursor - m.diffViewportHeight()/4
		m.clampDiffOffset()
	}
	m.ensureCursorVisible()
}

// ensureDiffShown makes sure there's a diff pane to step through: the history
// view's pane is closed by default, so a change/file motion there opens it.
// Returns false where there's no diff at all (the Stats pages).
func (m *model) ensureDiffShown() bool {
	switch m.mode {
	case viewOverview, viewAuthorDetail:
		return false
	case viewLog:
		if !m.logDiffOpen {
			m.openLogDiff()
		}
	}
	return true
}

// jumpChange steps n change blocks in direction dir (`]c` / `[c`).
func (m *model) jumpChange(dir, n int) {
	if !m.ensureDiffShown() {
		return
	}
	if m.stepRows(dir, n, m.changeStart) {
		return
	}
	switch {
	case m.totalDiffRows() == 0:
	case dir > 0 && (m.mode == viewBranch || m.mode == viewCommit):
		m.flash = "last change in this file — ]f / J for the next file"
	case dir > 0:
		m.flash = "no more changes below"
	default:
		m.flash = "no more changes above"
	}
}

// jumpFile steps n files in direction dir (`]f` / `[f` / `J` / `K`). In the
// file-tree views that moves the tree cursor (loading the next file's diff, and
// keeping focus where it was); in the history preview, whose diff is one
// multi-file patch, it moves between the files' sections.
func (m *model) jumpFile(dir, n int) {
	if !m.ensureDiffShown() {
		return
	}
	if m.mode == viewLog {
		if !m.stepRows(dir, n, m.fileStart) {
			m.flash = "no more files in this commit"
		}
		return
	}
	prev := m.cursor
	focus := m.focus
	m.moveCursor(dir * n)
	m.focus = focus
	if m.cursor == prev {
		if dir > 0 {
			m.flash = "last file"
		} else {
			m.flash = "first file"
		}
	}
}

// gotoLine is `{count}G`: jump to line N of the focused pane. In the diff that's
// the first row at or past new-side line N (or, when the new side never gets
// there — a trailing deletion — old-side line N); in a list it's the Nth entry.
func (m *model) gotoLine(n int) {
	switch {
	case m.mode == viewOverview:
		m.overviewCursor = 0
		m.moveOverviewCursor(n - 1)
	case m.focus == focusDiff:
		for _, side := range []func(*diff.Line) int{
			func(l *diff.Line) int { return l.NewNum },
			func(l *diff.Line) int { return l.OldNum },
		} {
			for i := 0; i < m.totalDiffRows(); i++ {
				if l := m.rowLine(i); l != nil && side(l) >= n {
					m.revealRow(i)
					return
				}
			}
		}
		m.gotoBottom()
	case m.mode == viewLog:
		m.moveCommitCursor(n - 1 - m.commitCursor)
	default:
		seen := 0
		for i, r := range m.rows {
			if r.file == nil {
				continue
			}
			seen++
			if seen == n {
				m.cursor = i
				m.loadDiff()
				return
			}
		}
		m.gotoBottom()
	}
}

// navBack is a snapshot of where the branch diff was opened from, enough to put
// the reader back: the history row (by SHA, since the list may reload), whether
// its diff pane was open, any visual selection, and the drill-in they were in.
type navBack struct {
	from        viewMode
	cursorSHA   string // "" = the working-tree row
	logDiffOpen bool
	visual      bool
	anchorSHA   string // visual anchor row by SHA ("" = the working-tree row)
	working     bool   // drilled into the working tree
	rng         bool   // drilled into a visual range
}

// snapshotBack records the reader's place before `D` leaves the history view or
// a drill-in for the branch diff.
func (m *model) snapshotBack() {
	if m.mode != viewLog && m.mode != viewCommit {
		m.branchBack = nil
		return
	}
	shaAt := func(row int) string {
		if c := m.rowCommit(row); c != nil {
			return c.SHA
		}
		return ""
	}
	m.branchBack = &navBack{
		from:        m.mode,
		cursorSHA:   shaAt(m.commitCursor),
		logDiffOpen: m.logDiffOpen,
		visual:      m.logVisual,
		anchorSHA:   shaAt(m.logAnchor),
		working:     m.scopeWorking,
		rng:         m.scopeRange != nil,
	}
}

// leaveBranchDiff is Esc from the branch diff: back to the history view, and —
// when `D` was pressed from there — onto the same row, selection, and drill-in.
func (m *model) leaveBranchDiff() {
	b := m.branchBack
	m.branchBack = nil
	m.enterLog()
	if b == nil {
		return
	}
	row := func(sha string) int {
		if sha == "" {
			if m.logWorking {
				return 0
			}
			return -1
		}
		return m.rowOf(sha)
	}
	if r := row(b.cursorSHA); r >= 0 {
		m.commitCursor = r
	}
	if b.visual {
		if a := row(b.anchorSHA); a >= 0 {
			m.logVisual = true
			m.logAnchor = a
		}
	}
	if b.from == viewCommit {
		switch {
		case b.rng && m.selectionRange() != nil:
			m.enterRange()
		case b.working && m.onWorkingRow():
			m.enterWorkingTree()
		case !b.working && !b.rng && m.selectedCommit() != nil:
			m.enterCommit()
		}
		m.logDiffOpen = b.logDiffOpen
		return
	}
	if b.logDiffOpen {
		m.openLogDiff()
	}
}

// copyTarget is what `y` copies: the commit SHA on a history row, the path of
// the file under the tree cursor, or path:line for the diff line under the
// cursor. Empty when nothing copyable is in scope.
func (m model) copyTarget() string {
	switch m.mode {
	case viewOverview, viewAuthorDetail:
		return ""
	case viewLog:
		if m.focus == focusFiles || !m.logDiffOpen {
			if c := m.selectedCommit(); c != nil {
				return c.SHA
			}
			return ""
		}
	}
	path, line := m.editorTarget()
	if path == "" {
		return ""
	}
	if line > 0 {
		return path + ":" + strconv.Itoa(line)
	}
	return path
}

// copyUnderCursor is `y`: put copyTarget on the system clipboard via OSC 52,
// which works over ssh and inside tmux (with set-clipboard on).
func (m *model) copyUnderCursor() tea.Cmd {
	s := m.copyTarget()
	if s == "" {
		m.flash = "y: nothing to copy here"
		return nil
	}
	m.flash = "copied " + s
	return tea.SetClipboard(s)
}

// wheelStep is how many rows one mouse-wheel notch scrolls.
const wheelStep = 3

// handleWheel scrolls the pane under the mouse pointer. Overlays other than help
// swallow the wheel, so it never moves the screen behind a picker.
func (m model) handleWheel(msg tea.MouseWheelMsg) (tea.Model, tea.Cmd) {
	if m.loading {
		return m, nil
	}
	delta := 0
	switch msg.Button {
	case tea.MouseWheelDown:
		delta = wheelStep
	case tea.MouseWheelUp:
		delta = -wheelStep
	default:
		return m, nil
	}
	if m.showHelp {
		m.scrollHelp(delta)
		return m, nil
	}
	if m.anyOverlay() {
		return m, nil
	}
	switch m.mode {
	case viewOverview:
		m.moveOverviewCursor(delta)
		return m, nil
	case viewAuthorDetail:
		return m, nil
	}
	inList := false
	switch {
	case m.mode == viewLog && !m.logDiffOpen:
		inList = true // the commit list fills the screen
	case m.mode == viewLog, m.sidebar != sidebarHidden:
		inList = msg.X < m.sidebarWidth()
	}
	switch {
	case inList && m.mode == viewLog:
		m.moveCommitCursor(delta)
	case inList:
		m.moveCursor(delta)
	default:
		m.scrollDiff(delta)
	}
	return m, nil
}

// scrollDiff scrolls the diff viewport by delta rows, dragging the cursor along
// only as far as needed to keep it on screen (like vim's ctrl+e / ctrl+y).
func (m *model) scrollDiff(delta int) {
	m.diffOffset += delta
	m.clampDiffOffset()
	if m.diffCursor < m.diffOffset {
		m.diffCursor = m.diffOffset
	}
	if last := m.lastVisibleRow(); m.diffCursor > last {
		m.diffCursor = last
	}
	m.clampDiffCursor()
}

// anyOverlay reports whether a floating window has the keyboard.
func (m model) anyOverlay() bool {
	return m.showHelp || m.showCommitDetails || m.fileFindActive || m.branchPickActive ||
		m.showThemePicker || m.showPalette || m.showGlobalSearch || m.searchActive
}
