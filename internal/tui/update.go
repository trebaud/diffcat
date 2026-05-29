package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/trebaud/sashi/internal/diff"
	"github.com/trebaud/sashi/internal/git"
)

// tickMsg drives the nyan cat's wiggle. ~7fps is enough for a charming gait
// without busy-spinning the terminal.
type tickMsg struct{}

const tickInterval = 150 * time.Millisecond

func tickCmd() tea.Cmd {
	return tea.Tick(tickInterval, func(time.Time) tea.Msg { return tickMsg{} })
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
		// Esc steps back one level: a per-commit tree → the history list →
		// the default branch diff; on the default view it quits.
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

	case "r":
		m.refresh()
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
		// History list: enter drills into the highlighted commit's file tree.
		// (Scroll its preview instead with l/Tab to focus the diff pane.)
		if m.mode == viewLog {
			m.enterCommit()
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
		m.commitCursor = max(0, len(m.commits)-1)
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

// refresh recomputes the base and reloads the changed-file list from disk so
// the view reflects edits made since launch.
func (m *model) refresh() {
	m.base = git.BaseRef(m.repo, m.baseName)
	if files, err := git.ChangedFiles(m.repo, m.base); err == nil {
		m.files = files
		m.rebuildTree()
	}
	m.shortstat = git.Shortstat(m.repo, m.base)
	switch m.mode {
	case viewLog:
		m.loadCommits()
		m.clampCommitCursor()
		m.loadCommitDiff()
		return
	case viewCommit:
		// A commit is immutable, so its file set can't have changed; just reload
		// the current file's diff. The stashed branch tree is refreshed on the
		// next refresh after exiting back to it.
		m.loadDiff()
		return
	}
	m.loadDiff()
}

// clampCommitCursor keeps the history cursor within the (possibly reloaded)
// commit list.
func (m *model) clampCommitCursor() {
	if m.commitCursor >= len(m.commits) {
		m.commitCursor = max(0, len(m.commits)-1)
	}
	if m.commitCursor < 0 {
		m.commitCursor = 0
	}
}
