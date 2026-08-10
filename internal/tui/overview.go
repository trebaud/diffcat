package tui

import (
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/trebaud/diffcat/internal/git"
)

// overview.go holds the Stats dashboard (viewOverview, toggled with `S`): a
// summary of the whole repo: a scrollable per-author commit ranking (each human
// author and each AI agent its own row) on the left, with the contribution
// calendar and the remaining activity charts on the right, across the commits
// reachable from HEAD that fall in the selected time window (the range selector —
// last week/month/6 months/year or all time, keys 1–5 and `[`/`]` — lives here
// too; each window is walked once and cached). It's deliberately diff-free so
// it stays fast on a deep history. Entering/leaving the mode lives here; rendering
// is in overview_view.go. Only the author ranking scrolls (j/k); the charts don't.

// statsRangeSpec is one selectable window on the Stats dashboard: the key that
// picks it, the label the selector shows, and the offset back from now that bounds
// it (all three zero = no bound, i.e. the entire history).
type statsRangeSpec struct {
	key    string
	label  string
	years  int
	months int
	days   int
}

// statsRanges are the dashboard's windows, narrowest first — the order the
// selector lays them out and the order `[`/`]` step through. Their keys are the
// number keys, so the reader can jump straight to one.
var statsRanges = []statsRangeSpec{
	{"1", "week", 0, 0, -7},
	{"2", "month", 0, -1, 0},
	{"3", "6 months", 0, -6, 0},
	{"4", "year", -1, 0, 0},
	{"5", "all time", 0, 0, 0},
}

// statsRangeAll indexes the unbounded window — the dashboard's default, so it
// opens on the whole history as it always has.
const statsRangeAll = 4

// unbounded reports whether the window covers the entire history (no cutoff).
func (s statsRangeSpec) unbounded() bool {
	return s.years == 0 && s.months == 0 && s.days == 0
}

// cutoff is the oldest author date the window admits, computed back from now. The
// zero time means unbounded (all time), which git.HistorySince reads as "no
// window".
func (s statsRangeSpec) cutoff(now time.Time) time.Time {
	if s.unbounded() {
		return time.Time{}
	}
	return now.AddDate(s.years, s.months, s.days)
}

// statsRangeSpec returns the selected window, clamped so a bad index can't panic
// a render.
func (m model) statsRangeSpec() statsRangeSpec {
	i := m.statsRange
	if i < 0 || i >= len(statsRanges) {
		i = statsRangeAll
	}
	return statsRanges[i]
}

// setStatsRange selects the window the dashboard summarizes. A window already
// walked comes straight from the cache (instant); a new one drops the stats back
// to the loading state and returns the command that computes it in the background,
// exactly like the first open. The author cursor is re-clamped, since a narrower
// window usually has fewer contributors.
func (m *model) setStatsRange(i int) tea.Cmd {
	if i < 0 || i >= len(statsRanges) || i == m.statsRange {
		return nil
	}
	m.statsRange = i
	if hs, ok := m.historyCache[i]; ok {
		m.historyStats = hs
		m.historyComputed = true
	} else {
		m.historyStats = git.HistoryStats{}
		m.historyComputed = false
	}
	m.clampOverviewCursor()
	cmd := m.ensureHistory()
	// A cached window lands synchronously, so the historyMsg that normally re-kicks
	// an open contributor's module ranking never arrives — do it here instead, or
	// their heatmap would keep showing the previous window's modules.
	if m.historyComputed && m.mode == viewAuthorDetail {
		return tea.Batch(cmd, m.ensureAuthorModules(m.detailAuthor))
	}
	return cmd
}

// cycleStatsRange steps the window selection by delta (`]` widens toward all time,
// `[` narrows), stopping at the ends rather than wrapping so repeated presses
// settle somewhere predictable.
func (m *model) cycleStatsRange(delta int) tea.Cmd {
	i := m.statsRange + delta
	if i < 0 {
		i = 0
	}
	if i >= len(statsRanges) {
		i = len(statsRanges) - 1
	}
	return m.setStatsRange(i)
}

// clampOverviewCursor keeps the author-ranking selection and scroll inside the
// current ranking, which shrinks when the window narrows.
func (m *model) clampOverviewCursor() {
	if hi := len(m.historyStats.Authors) - 1; m.overviewCursor > hi {
		m.overviewCursor = max(0, hi)
	}
	if m.overviewCursor < 0 {
		m.overviewCursor = 0
	}
	m.ensureAuthorVisible()
}

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
