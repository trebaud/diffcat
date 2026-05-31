package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/trebaud/diffcat/internal/diff"
	"github.com/trebaud/diffcat/internal/git"
)

// tickMsg drives the nyan cat's wiggle. ~7fps is enough for a charming gait
// without busy-spinning the terminal.
type tickMsg struct{}

const tickInterval = 150 * time.Millisecond

func tickCmd() tea.Cmd {
	return tea.Tick(tickInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

// gitStateMsg carries the result of a background git-state poll. The fingerprint
// is computed in the tick goroutine (off the UI thread), so the poll never
// blocks rendering even on a slow repo.
type gitStateMsg struct{ fingerprint string }

// syncInterval is how long the poll waits between fingerprint checks. It
// self-paces: the next poll is scheduled only after the previous one returns
// (Update re-arms it), so a slow `git status` can't pile up overlapping polls.
const syncInterval = 1500 * time.Millisecond

func syncCmd(repo, baseName string) tea.Cmd {
	return tea.Tick(syncInterval, func(time.Time) tea.Msg {
		return gitStateMsg{fingerprint: git.Fingerprint(repo, baseName)}
	})
}

// Update is the Elm update function — it maps a message to the next model and
// any side effects.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clampDiffOffset()
		m.clampDiffCursor()
		m.ensureCursorVisible()
		return m, nil

	case tickMsg:
		m.animFrame++
		return m, tickCmd()

	case gitStateMsg:
		// Only do the (more expensive) refresh when the cheap fingerprint moved;
		// otherwise the poll is a no-op beyond re-arming itself.
		if msg.fingerprint != m.syncFingerprint {
			m.syncFingerprint = msg.fingerprint
			m.refresh()
		}
		return m, syncCmd(m.repo, m.baseName)

	case tea.KeyPressMsg:
		return m.handleKey(msg)
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	if m.showHelp {
		// Any key dismisses the help overlay.
		m.showHelp = false
		m.pendingG = false
		return m, nil
	}

	if m.showCommitDetails {
		// j/k (and half/full page) scroll a long body; any other key dismisses.
		switch msg.String() {
		case "j", "down":
			m.detailsScroll++
		case "k", "up":
			m.detailsScroll--
		case "ctrl+d", "ctrl+f":
			m.detailsScroll += m.diffViewportHeight() / 2
		case "ctrl+u", "ctrl+b":
			m.detailsScroll -= m.diffViewportHeight() / 2
		default:
			m.showCommitDetails = false
			m.detailsScroll = 0
			m.pendingG = false
			return m, nil
		}
		// Clamp to the scrollable range so j past the end / k past the top stick.
		if max := m.detailsMaxScroll(); m.detailsScroll > max {
			m.detailsScroll = max
		}
		if m.detailsScroll < 0 {
			m.detailsScroll = 0
		}
		m.pendingG = false
		return m, nil
	}

	key := msg.String()

	// `gg` chord: a pending `g` followed by another `g` jumps to the top of
	// the focused pane. Any other key cancels the chord and is handled below.
	if m.pendingG {
		m.pendingG = false
		if key == "g" {
			m.gotoTop()
			return m, nil
		}
	}

	switch key {
	case "q", "ctrl+c":
		return m, tea.Quit

	case "esc":
		// Esc steps back one level: a per-commit (or working-tree) drill-in → the
		// history list → the default branch diff; on the default view it quits.
		switch m.mode {
		case viewCommit:
			m.exitCommit()
			return m, nil
		case viewLog:
			m.exitLog()
			return m, nil
		}
		return m, tea.Quit

	case "L":
		// Toggle the commit-history view. From a per-commit tree, step back to
		// the history list (Esc's first stop) rather than all the way out.
		switch m.mode {
		case viewLog:
			m.exitLog()
		case viewCommit:
			m.exitCommit()
		default:
			m.enterLog()
		}
		return m, nil

	case "?":
		m.showHelp = true
		return m, nil

	case "d":
		// Inspect the in-scope commit's details. Only meaningful where a real
		// commit is in scope: the highlighted commit in the history list (not the
		// working-tree row) or the commit being drilled into.
		if m.detailsCommit() != nil {
			m.showCommitDetails = true
			m.detailsScroll = 0
		}
		return m, nil

	case "r":
		m.refresh()
		m.syncFingerprint = git.Fingerprint(m.repo, m.baseName)
		return m, nil

	case "s":
		// Toggle unified ↔ side-by-side. Row counts differ between modes, so
		// reset to the top to keep the scroll position sensible. Expansions are
		// about which lines are revealed, not layout, so they're preserved.
		m.splitView = !m.splitView
		m.diffOffset = 0
		m.diffCursor = 0
		m.rebuildView()
		return m, nil

	case "t":
		m.toggleTheme()
		return m, nil

	// --- pane focus (vim window motions) ---
	case "tab":
		m.toggleFocus()
		return m, nil
	case "h", "left":
		m.focus = focusFiles
		return m, nil
	case "l", "right":
		m.focus = focusDiff
		return m, nil
	case "enter", "o":
		// History list: enter drills into the highlighted commit's file tree, or
		// the working-tree row into the full branch view. (Scroll its preview
		// instead with l/Tab to focus the diff pane.)
		if m.mode == viewLog {
			if m.onWorkingRow() {
				m.enterWorkingTree()
			} else {
				m.enterCommit()
			}
			return m, nil
		}
		// In the diff pane, on an expand affordance: reveal hidden context.
		if m.focus == focusDiff {
			if l := m.cursorLine(); l != nil && l.Kind == diff.Expand {
				m.expandUnderCursor(*l)
			}
			return m, nil
		}
		// On a folder: fold/unfold it. On a file: open its diff and move into
		// the diff pane.
		if r := m.selectedRow(); r != nil && r.isDir {
			m.toggleCollapse()
			return m, nil
		}
		m.focus = focusDiff
		return m, nil

	// --- motions within the focused pane ---
	case "j", "down":
		m.moveDown()
		return m, nil
	case "k", "up":
		m.moveUp()
		return m, nil
	case "g":
		m.pendingG = true // wait for the second `g`
		return m, nil
	case "G", "end":
		m.gotoBottom()
		return m, nil

	// --- diff paging (always moves the diff cursor by a page) ---
	case "ctrl+d":
		m.moveDiffCursor(m.diffViewportHeight() / 2)
		return m, nil
	case "ctrl+u":
		m.moveDiffCursor(-m.diffViewportHeight() / 2)
		return m, nil
	case "ctrl+f", "pgdown", " ":
		m.moveDiffCursor(m.diffViewportHeight())
		return m, nil
	case "ctrl+b", "pgup":
		m.moveDiffCursor(-m.diffViewportHeight())
		return m, nil
	}
	return m, nil
}

// toggleTheme flips between the dark and light palettes, rebuilding the global
// style table. The highlight cache memoizes token colors from the active Chroma
// style, so it's dropped here to re-lex under the new style (scroll position is
// preserved — only the colors change).
func (m *model) toggleTheme() {
	m.dark = !m.dark
	ApplyTheme(m.dark)
	m.hlCache = map[string][]span{}
}

func (m *model) toggleFocus() {
	if m.focus == focusFiles {
		m.focus = focusDiff
	} else {
		m.focus = focusFiles
	}
}

// moveDown/moveUp dispatch j/k to the focused pane: in viewLog the left pane is
// the commit list, otherwise it's the file tree; the right pane is always the
// diff's line cursor.
func (m *model) moveDown() {
	switch {
	case m.focus != focusFiles:
		m.moveDiffCursor(1)
	case m.mode == viewLog:
		m.moveCommitCursor(1)
	default:
		m.moveCursor(1)
	}
}

func (m *model) moveUp() {
	switch {
	case m.focus != focusFiles:
		m.moveDiffCursor(-1)
	case m.mode == viewLog:
		m.moveCommitCursor(-1)
	default:
		m.moveCursor(-1)
	}
}

func (m *model) gotoTop() {
	switch {
	case m.focus != focusFiles:
		m.diffCursor = 0
		m.ensureCursorVisible()
	case m.mode == viewLog:
		m.commitCursor = 0
		m.loadCommitDiff()
	default:
		m.cursor = 0
		m.loadDiff()
	}
}

func (m *model) gotoBottom() {
	switch {
	case m.focus != focusFiles:
		m.diffCursor = m.totalDiffRows() - 1
		m.ensureCursorVisible()
	case m.mode == viewLog:
		m.commitCursor = max(0, m.logRowCount()-1)
		m.loadCommitDiff()
	default:
		m.cursor = max(0, len(m.rows)-1)
		m.loadDiff()
	}
}

// moveDiffCursor moves the diff-pane line cursor and scrolls to keep it visible.
func (m *model) moveDiffCursor(delta int) {
	m.diffCursor += delta
	m.clampDiffCursor()
	m.ensureCursorVisible()
}

// expandUnderCursor reveals the next window of hidden context for the gap under
// the cursor: downward (or "all") grows the revealed block from the top, upward
// grows it from the bottom. The view is rebuilt and the cursor held in place.
func (m *model) expandUnderCursor(l diff.Line) {
	if l.GapID < 0 || l.GapID >= len(m.gaps) {
		return
	}
	rev := m.revealed[l.GapID]
	if l.Dir == diff.ExpandUp {
		rev[1] += expandWindow
	} else {
		rev[0] += expandWindow
	}
	m.revealed[l.GapID] = rev
	m.rebuildView()
}

func (m *model) moveCursor(delta int) {
	if len(m.files) == 0 {
		return
	}
	m.cursor += delta
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.rows) {
		m.cursor = len(m.rows) - 1
	}
	m.loadDiff()
}

// refresh recomputes the base and reloads the changed-file list from disk so the
// view reflects edits made since launch. It is driven both by the manual `r` key
// and by the background sync poll, so it preserves the reader's position: the
// same file stays selected by path (the same commit by SHA in history), and diff
// scroll/cursor/expansion are kept when the visible content didn't actually
// change — only genuinely new content resets the view.
func (m *model) refresh() {
	m.base = git.BaseRef(m.repo, m.baseName)
	m.shortstat = git.Shortstat(m.repo, m.base)

	if m.mode == viewCommit {
		if m.scopeWorking {
			// The working-tree drill-in is mutable (edits/staging change its file
			// set), so rebuild it from `git diff HEAD`, holding the selected file
			// and scroll where possible. The stashed branch tree re-syncs on exit.
			prevPath := ""
			if f := m.selectedFile(); f != nil {
				prevPath = f.Path
			}
			if files, err := git.ChangedFiles(m.repo, "HEAD"); err == nil {
				m.files = files
				m.rebuildTree()
				m.reselectPath(prevPath)
			}
			m.preserveDiffView(m.loadDiff)
			return
		}
		// A real commit's file set is immutable, so it (and the stashed branch
		// tree) are left untouched; just reload the visible file's diff, holding
		// scroll. The branch tree re-syncs once you exit back to it.
		m.preserveDiffView(m.loadDiff)
		return
	}

	prevPath := ""
	if f := m.selectedFile(); f != nil {
		prevPath = f.Path
	}
	if files, err := git.ChangedFiles(m.repo, m.base); err == nil {
		m.files = files
		m.rebuildTree()
		m.reselectPath(prevPath)
		m.flagChangedFiles()
	}

	if m.mode == viewLog {
		wasWorking := m.onWorkingRow()
		prevSHA := ""
		if c := m.selectedCommit(); c != nil {
			prevSHA = c.SHA
		}
		m.loadCommits()
		m.logWorking = len(m.files) > 0
		// Keep the reader on the working-tree row across a sync when it's still
		// present; otherwise re-find the same commit by SHA.
		if wasWorking && m.logWorking {
			m.commitCursor = 0
		} else {
			m.reselectCommit(prevSHA)
		}
		m.preserveDiffView(m.loadCommitDiff)
		return
	}
	m.preserveDiffView(m.loadDiff)
}

// reselectPath moves the tree cursor back onto the row for path after a rebuild,
// so a sync keeps the same file selected even when the change list reordered or
// grew. Falls back to the (already clamped) cursor when the path is gone — e.g.
// its changes were reverted.
func (m *model) reselectPath(path string) {
	if path == "" {
		return
	}
	for i, r := range m.rows {
		if r.file != nil && r.file.Path == path {
			m.cursor = i
			return
		}
	}
}

// reselectCommit moves the history cursor back onto the commit with sha after the
// list reloads, so a sync that added newer commits on top doesn't shift the
// reader onto a different commit. Falls back to the clamped cursor if it's gone.
func (m *model) reselectCommit(sha string) {
	if sha != "" {
		for i, c := range m.commits {
			if c.SHA == sha {
				m.commitCursor = i
				if m.logWorking {
					m.commitCursor++ // account for the leading working-tree row
				}
				return
			}
		}
	}
	m.clampCommitCursor()
}

// preserveDiffView runs reload (loadDiff or loadCommitDiff) and, when the diff it
// produces is byte-for-byte identical to what was on screen, restores the prior
// scroll, cursor, and expansion state — so a background sync that didn't touch
// the visible file leaves the reader exactly where they were. A changed diff is
// left reset to the top by reload, since the old position no longer maps onto it.
func (m *model) preserveDiffView(reload func()) {
	prevDiff := m.diff
	prevOffset, prevCursor := m.diffOffset, m.diffCursor
	prevRevealed := m.revealed
	reload()
	if diffLinesEqual(prevDiff, m.diff) {
		m.revealed = prevRevealed
		m.rebuildView()
		m.diffOffset = prevOffset
		m.diffCursor = prevCursor
		m.clampDiffOffset()
		m.clampDiffCursor()
	}
}

// diffLinesEqual reports whether two pristine diffs are identical line-for-line.
// diff.Line is all comparable fields, so a plain == suffices per element.
func diffLinesEqual(a, b []diff.Line) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// clampCommitCursor keeps the history cursor within the (possibly reloaded)
// commit list.
func (m *model) clampCommitCursor() {
	if m.commitCursor >= m.logRowCount() {
		m.commitCursor = max(0, m.logRowCount()-1)
	}
	if m.commitCursor < 0 {
		m.commitCursor = 0
	}
}
