package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/trebaud/diff-master/internal/git"
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
	case "q", "ctrl+c", "esc":
		return m, tea.Quit

	case "?":
		m.showHelp = true
		return m, nil

	case "r":
		m.refresh()
		return m, nil

	case "s":
		// Toggle unified ↔ side-by-side. Row counts differ between modes, so
		// reset to the top to keep the scroll position sensible.
		m.splitView = !m.splitView
		m.diffOffset = 0
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

	// --- diff paging (always acts on the diff pane) ---
	case "ctrl+d":
		m.scrollDiff(m.diffViewportHeight() / 2)
		return m, nil
	case "ctrl+u":
		m.scrollDiff(-m.diffViewportHeight() / 2)
		return m, nil
	case "ctrl+f", "pgdown", " ":
		m.scrollDiff(m.diffViewportHeight())
		return m, nil
	case "ctrl+b", "pgup":
		m.scrollDiff(-m.diffViewportHeight())
		return m, nil
	}
	return m, nil
}

func (m *model) toggleFocus() {
	if m.focus == focusFiles {
		m.focus = focusDiff
	} else {
		m.focus = focusFiles
	}
}

// moveDown/moveUp dispatch j/k to the focused pane: file selection on the
// left, line-wise diff scrolling on the right.
func (m *model) moveDown() {
	if m.focus == focusFiles {
		m.moveCursor(1)
	} else {
		m.scrollDiff(1)
	}
}

func (m *model) moveUp() {
	if m.focus == focusFiles {
		m.moveCursor(-1)
	} else {
		m.scrollDiff(-1)
	}
}

func (m *model) gotoTop() {
	if m.focus == focusFiles {
		m.cursor = 0
		m.loadDiff()
		return
	}
	m.diffOffset = 0
}

func (m *model) gotoBottom() {
	if m.focus == focusFiles {
		m.cursor = max(0, len(m.rows)-1)
		m.loadDiff()
		return
	}
	m.diffOffset = m.totalDiffRows()
	m.clampDiffOffset()
}

func (m *model) scrollDiff(delta int) {
	m.diffOffset += delta
	m.clampDiffOffset()
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
	m.base = git.MergeBase(m.repo, m.baseName)
	if files, err := git.ChangedFiles(m.repo, m.base); err == nil {
		m.files = files
		m.rebuildTree()
	}
	m.shortstat = git.Shortstat(m.repo, m.base)
	m.loadDiff()
}
