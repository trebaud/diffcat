package tui

import (
	tea "charm.land/bubbletea/v2"
)

// overview.go holds the Stats dashboard (viewOverview, toggled with `S`): a
// summary of the whole repo: a scrollable per-author commit ranking (each human
// author and each AI agent its own row) on the left, with the contribution
// calendar and the remaining activity charts on the right, across every commit
// reachable from HEAD. It's deliberately diff-free so
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
	m.overviewCursor = 0
	m.overviewScroll = 0
	return m.ensureHistory()
}

// exitOverview leaves the dashboard for the commit-history view it was opened
// from, restoring its diff.
func (m *model) exitOverview() {
	m.mode = viewLog
	m.focus = focusFiles
	// A commit can land while the dashboard covers the list (refresh skips the list
	// reload for viewOverview, having only advanced the sync fingerprint), so reload
	// it now rather than waiting for an unrelated change to move the fingerprint
	// again — otherwise the new commit silently never appears in the history.
	m.reloadCommitList()
	m.loadCommitDiff()
}

// enterAuthorDetail opens the per-contributor Stats page for the author under the
// ranking cursor. The page's summary card and activity charts reuse that author's
// sub-stats (historyStats.ByAuthor), already computed by the cheap background walk;
// the one heavier piece — their module ranking — is kicked off lazily here (the
// returned command), so the page itself opens instantly. A no-op (nil) when the
// stats haven't landed yet or the cursor doesn't map to a known author.
func (m *model) enterAuthorDetail() tea.Cmd {
	if !m.historyComputed {
		return nil
	}
	authors := m.historyStats.Authors
	if m.overviewCursor < 0 || m.overviewCursor >= len(authors) {
		return nil
	}
	name := authors[m.overviewCursor].Name
	if _, ok := m.historyStats.ByAuthor[name]; !ok {
		return nil
	}
	m.detailAuthor = name
	m.mode = viewAuthorDetail
	m.focus = focusFiles
	return m.ensureAuthorModules(name)
}

// exitAuthorDetail backs out of a contributor's detail page to the ranking it was
// opened from, leaving the cursor and scroll where they were.
func (m *model) exitAuthorDetail() {
	m.mode = viewOverview
	m.focus = focusFiles
}
