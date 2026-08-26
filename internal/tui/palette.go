package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/trebaud/diffcat/internal/git"
)

// palette.go is the command palette (`:`): a fuzzy-searchable list of
// every action, each shown with its keybinding so the palette doubles as a way to
// discover and reinforce the shortcuts. Each action's `do` runs the same mutation
// the corresponding key handler does — reusing the extracted methods (goHistory,
// toggleStats, …) where the logic is shared. Rendering is in palette_view.go.

// paletteAction is one entry: a human label, the keybinding hint shown beside it,
// and the mutation to run when chosen. do may return a command (e.g. tea.Quit, a
// background recompute) just like a key handler.
type paletteAction struct {
	label string
	keys  string
	do    func(m *model) tea.Cmd
}

// paletteActions is the full action list. Order here is the order shown for an
// empty query; the fuzzy filter reorders by score once the reader types.
func (m model) paletteActions() []paletteAction {
	return []paletteAction{
		{"Toggle unified / side-by-side diff", "s", func(m *model) tea.Cmd {
			if m.mode == viewLog {
				return nil
			}
			m.splitView = !m.splitView
			m.diffOffset = 0
			m.diffCursor = 0
			m.rebuildView()
			m.clampDiffHOffset()
			return nil
		}},
		{"Search the diff", "/", func(m *model) tea.Cmd {
			if m.mode != viewOverview {
				m.searchActive = true
				m.searchInput = ""
			}
			return nil
		}},
		{"Find a changed file", "f", func(m *model) tea.Cmd {
			if m.mode == viewBranch || m.mode == viewCommit {
				m.fileFindActive = true
				m.fileFindInput = ""
				m.fileFindSel = 0
			}
			return nil
		}},
		{"Open the file in your editor", "e", func(m *model) tea.Cmd { return m.openEditor() }},
		{"Go to commit history", "L", func(m *model) tea.Cmd { return m.goHistory() }},
		{"Open branch diff vs base", "D", func(m *model) tea.Cmd { return m.goBranchDiff() }},
		{"Open stats dashboard", "S", func(m *model) tea.Cmd { return m.toggleStats() }},
		{"Stats: last week", "1", statsRangeAction(0)},
		{"Stats: last month", "2", statsRangeAction(1)},
		{"Stats: last 6 months", "3", statsRangeAction(2)},
		{"Stats: last year", "4", statsRangeAction(3)},
		{"Stats: all time", "5", statsRangeAction(statsRangeAll)},
		{"Cycle sidebar width", "[ ]", func(m *model) tea.Cmd {
			if m.mode != viewOverview && m.mode != viewLog {
				m.sidebar = (m.sidebar + 1) % 3
				if m.sidebar == sidebarHidden {
					m.focus = focusDiff
				}
			}
			return nil
		}},
		{"Choose theme (colors · light/dark · icons)", "t", func(m *model) tea.Cmd {
			m.showThemePicker = true
			m.themeSel = m.themeIdx
			m.snapTheme = m.themeIdx
			m.snapDark = m.dark
			return nil
		}},
		{"Toggle reduce motion", "", func(m *model) tea.Cmd {
			m.reduceMotion = !m.reduceMotion
			if !m.reduceMotion && !m.ticking {
				m.ticking = true
				return tickCmd(tickSlow)
			}
			return nil
		}},
		{"Refresh from disk", "r", func(m *model) tea.Cmd {
			repaint := m.refresh()
			m.syncFingerprint = git.Fingerprint(m.repo, m.baseRev)
			if m.mode == viewOverview || m.mode == viewAuthorDetail {
				return tea.Batch(repaint, m.ensureHistory())
			}
			return repaint
		}},
		{"Show keybindings help", "?", func(m *model) tea.Cmd { m.showHelp = true; return nil }},
		{"Quit diffcat", "q", func(m *model) tea.Cmd { return tea.Quit }},
	}
}

// statsRangeAction builds the palette entry that selects one Stats window. From
// the history view it also opens the dashboard on that window, so picking a range
// is a one-step way in; from a diff view it just sets the window (matching `S`,
// which only enters Stats from the history).
func statsRangeAction(rng int) func(*model) tea.Cmd {
	return func(m *model) tea.Cmd {
		set := m.setStatsRange(rng)
		if m.mode == viewLog {
			return tea.Batch(set, m.toggleStats())
		}
		return set
	}
}

// paletteMatch is one ranked palette result: the action and the rune offsets in
// its label the query matched (for bolding).
type paletteMatch struct {
	action paletteAction
	hit    []int
}

// paletteMatches ranks the actions against the current palette query, reusing the
// file-finder's fuzzy matcher over the action labels.
func (m model) paletteMatches() []paletteMatch {
	acts := m.paletteActions()
	labels := make([]string, len(acts))
	byLabel := make(map[string]paletteAction, len(acts))
	for i, a := range acts {
		labels[i] = a.label
		byLabel[a.label] = a
	}
	ranked := fuzzyFilter(labels, m.paletteInput)
	out := make([]paletteMatch, 0, len(ranked))
	for _, r := range ranked {
		out = append(out, paletteMatch{action: byLabel[r.path], hit: r.hit})
	}
	return out
}
