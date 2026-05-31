package tui

import (
	"strings"

	"github.com/trebaud/diffcat/internal/diff"
)

// search.go holds the diff-search state machine (the vim-style `/` over the
// open diff). Matching is a case-insensitive substring over each visible line's
// Text; hits are tracked at line granularity for n/N navigation, and the exact
// matched rune ranges are recomputed per line at render time (searchRanges).
// Rendering lives in view.go (renderCode paints the highlight); key handling in
// update.go.

// resetSearch clears all search state — called whenever the diff is (re)loaded,
// so switching files or commits drops any active query.
func (m *model) resetSearch() {
	m.searchActive = false
	m.searchInput = ""
	m.searchQuery = ""
	m.searchHits = nil
	m.searchIdx = 0
}

// recomputeSearch rebuilds the hit list from the committed query against the
// current viewLines. Called at the end of rebuildView (so expansion and split
// toggles keep matches correct) and whenever the query changes. Expand
// affordance rows are skipped — they carry no source text. searchIdx is clamped
// into the new hit list.
func (m *model) recomputeSearch() {
	m.searchHits = nil
	if m.searchQuery == "" {
		return
	}
	q := strings.ToLower(m.searchQuery)
	for i, l := range m.viewLines {
		if l.Kind == diff.Expand {
			continue
		}
		if strings.Contains(strings.ToLower(l.Text), q) {
			m.searchHits = append(m.searchHits, i)
		}
	}
	if m.searchIdx >= len(m.searchHits) {
		m.searchIdx = 0
	}
	if m.searchIdx < 0 {
		m.searchIdx = 0
	}
}

// matchRanges returns the [start,end) rune ranges within text where query
// occurs, case-insensitively. Offsets index the original (non-lowered) runes so
// they line up with the emphasis-mask math in renderCode. Returns nil for an
// empty query or no match.
func matchRanges(text, query string) [][2]int {
	if query == "" {
		return nil
	}
	t := []rune(strings.ToLower(text))
	q := []rune(strings.ToLower(query))
	if len(q) == 0 || len(q) > len(t) {
		return nil
	}
	var out [][2]int
	for i := 0; i+len(q) <= len(t); {
		if runesHavePrefix(t[i:], q) {
			out = append(out, [2]int{i, i + len(q)})
			i += len(q) // non-overlapping matches, like a reader scanning left→right
		} else {
			i++
		}
	}
	return out
}

// runesHavePrefix reports whether s starts with the runes in prefix.
func runesHavePrefix(s, prefix []rune) bool {
	if len(prefix) > len(s) {
		return false
	}
	for i, r := range prefix {
		if s[i] != r {
			return false
		}
	}
	return true
}

// searchRanges returns the matched rune ranges for one view line when a search
// is active, or nil. Only Add/Del/Context lines carry searchable text.
func (m model) searchRanges(l diff.Line) [][2]int {
	if m.searchQuery == "" || l.Kind == diff.Expand || l.Kind == diff.Hunk || l.Kind == diff.Meta {
		return nil
	}
	return matchRanges(l.Text, m.searchQuery)
}

// jumpToFirstHit moves the diff cursor to the first hit at or after the current
// position (so `/foo<CR>` jumps forward naturally), wrapping to the first hit if
// none follow. No-op when there are no hits.
func (m *model) jumpToFirstHit() {
	if len(m.searchHits) == 0 {
		return
	}
	m.searchIdx = 0
	for i, line := range m.searchHits {
		if line >= m.diffCursor {
			m.searchIdx = i
			break
		}
	}
	m.gotoCurrentHit()
}

// nextHit / prevHit cycle the current hit and scroll to it. They wrap around the
// hit list. No-op when there are no hits.
func (m *model) nextHit() {
	if len(m.searchHits) == 0 {
		return
	}
	m.searchIdx = (m.searchIdx + 1) % len(m.searchHits)
	m.gotoCurrentHit()
}

func (m *model) prevHit() {
	if len(m.searchHits) == 0 {
		return
	}
	m.searchIdx = (m.searchIdx - 1 + len(m.searchHits)) % len(m.searchHits)
	m.gotoCurrentHit()
}

// gotoCurrentHit scrolls the diff pane to the line of the current hit, focusing
// the diff pane so the cursor is shown. In split mode the diff cursor indexes
// splitRows, so the viewLines hit index is mapped through viewToSplitRow.
func (m *model) gotoCurrentHit() {
	if m.searchIdx < 0 || m.searchIdx >= len(m.searchHits) {
		return
	}
	m.focus = focusDiff
	line := m.searchHits[m.searchIdx]
	if m.splitView {
		if r := m.viewToSplitRow(line); r >= 0 {
			m.diffCursor = r
		}
	} else {
		m.diffCursor = line
	}
	m.clampDiffCursor()
	m.ensureCursorVisible()
}

// currentHitIndex is the viewLines index of the current search hit, or -1 when
// there is no active search.
func (m model) currentHitIndex() int {
	if m.searchQuery == "" || m.searchIdx < 0 || m.searchIdx >= len(m.searchHits) {
		return -1
	}
	return m.searchHits[m.searchIdx]
}

// currentHitLine returns a pointer to the viewLine of the current hit (so split
// rendering can identify it by pointer), or nil.
func (m model) currentHitLine() *diff.Line {
	i := m.currentHitIndex()
	if i >= 0 && i < len(m.viewLines) {
		return &m.viewLines[i]
	}
	return nil
}

// viewToSplitRow maps an index into viewLines to the split row that carries that
// line (SplitRows stores pointers into viewLines), or -1 if not present.
func (m model) viewToSplitRow(i int) int {
	if i < 0 || i >= len(m.viewLines) {
		return -1
	}
	target := &m.viewLines[i]
	for r, row := range m.splitRows {
		if row.Full == target || row.Left == target || row.Right == target {
			return r
		}
	}
	return -1
}
