package tui

import (
	tea "charm.land/bubbletea/v2"
)

// overview.go holds the Stats dashboard (viewOverview, toggled with `S`): a
// summary of the whole repo: a scrollable per-author commit ranking (each human
// author and each AI agent its own row) on the left, with activity charts on the
// right, across every commit reachable from HEAD. It's deliberately diff-free so
// it stays fast on a deep history. Entering/leaving the mode lives here; rendering
// is in overview_view.go. Only the author ranking scrolls (j/k); the charts don't.

// enterOverview switches into the whole-repo Stats. The underlying `git log` walk
// can still take a moment on a very deep history, so it's computed in the
// background (see ensureHistory) and the returned command fills the dashboard in
// when ready — meanwhile the screen opens instantly on a loading state. It's only
// reachable from the commit-history view, so `S`/esc backs out there.
func (m *model) enterOverview() tea.Cmd {
	m.mode = viewOverview
	m.focus = focusFiles
	m.overviewScroll = 0
	return m.ensureHistory()
}

// exitOverview leaves the dashboard for the commit-history view it was opened
// from, restoring its diff.
func (m *model) exitOverview() {
	m.mode = viewLog
	m.focus = focusFiles
	m.loadCommitDiff()
}
