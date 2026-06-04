package tui

import (
	tea "charm.land/bubbletea/v2"
)

// overview.go holds the Stats dashboard (viewOverview, toggled with `S`): a
// full-screen summary of the whole repo — the total commit count and a per-author
// ranking by commits (each human author and each AI agent its own row), across
// every commit reachable from HEAD. It's deliberately diff-free so it stays fast
// on a deep history. Entering/leaving the mode lives here; rendering is in
// overview_view.go. The view is read-only — there's nothing to scroll or open.

// enterOverview switches into the whole-repo Stats. The underlying `git log` walk
// can still take a moment on a very deep history, so it's computed in the
// background (see ensureHistory) and the returned command fills the dashboard in
// when ready — meanwhile the screen opens instantly on a loading state. It's only
// reachable from the commit-history view, so `S`/esc backs out there.
func (m *model) enterOverview() tea.Cmd {
	m.mode = viewOverview
	m.focus = focusFiles
	return m.ensureHistory()
}

// exitOverview leaves the dashboard for the commit-history view it was opened
// from, restoring its diff.
func (m *model) exitOverview() {
	m.mode = viewLog
	m.focus = focusFiles
	m.loadCommitDiff()
}
