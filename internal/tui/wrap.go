package tui

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/trebaud/diffcat/internal/diff"
)

// wrap.go is soft-wrap (`w`): long code lines continue onto extra screen rows
// instead of being cut at the pane edge. The diff cursor and scroll offset still
// index logical rows (viewLines / splitRows); rowHeight says how many screen rows
// each one takes, and the scroll math below sums those heights so the cursor row
// is always wholly on screen.

// diffPaneWidth is the diff pane's full column count, mirroring twoPaneScreen's
// split: the terminal width less the sidebar and its divider.
func (m model) diffPaneWidth() int {
	if lw := m.sidebarWidth(); lw > 0 {
		return m.width - lw - 1
	}
	return m.width
}

// diffContentWidth is the diff pane's width less the minimap column — the width
// diffWindow lays rows out at.
func (m model) diffContentWidth() int {
	w := m.diffPaneWidth()
	return w - m.minimapW(w)
}

// codeRows is how many screen rows l's code takes when the code area (renderCode's
// width, which includes its one-column left pad) is codeW wide.
func codeRows(l *diff.Line, codeW int) int {
	if l == nil {
		return 1
	}
	cols := codeW - 1
	if cols < 1 {
		return 1
	}
	return max(1, (lipgloss.Width(expandTabs(l.Text))+cols-1)/cols)
}

// splitWidths is the left/right side widths of a split row at content width cw.
func splitWidths(cw int) (leftW, rightW int) {
	leftW = (cw - 1) / 2
	return leftW, cw - 1 - leftW
}

// rowHeight is how many screen rows display row i occupies: always 1 unless
// wrapping, where a long line spans several (in split mode, the taller side).
// Hunk headers, meta, and expand rows never wrap.
func (m model) rowHeight(i int) int {
	if !m.wrap {
		return 1
	}
	cw := m.diffContentWidth()
	d := m.lineDigits
	if m.splitView {
		if i < 0 || i >= len(m.splitRows) || m.splitRows[i].Full != nil {
			return 1
		}
		r := m.splitRows[i]
		leftW, rightW := splitWidths(cw)
		return max(codeRows(r.Left, leftW-d-1), codeRows(r.Right, rightW-d-1))
	}
	if i < 0 || i >= len(m.viewLines) {
		return 1
	}
	l := &m.viewLines[i]
	if l.Kind == diff.Hunk || l.Kind == diff.Meta || l.Kind == diff.Expand {
		return 1
	}
	return codeRows(l, cw-(2*d+3))
}

// rowsHeight is the total screen height of display rows a..b inclusive.
func (m model) rowsHeight(a, b int) int {
	h := 0
	for i := a; i <= b; i++ {
		h += m.rowHeight(i)
	}
	return h
}

// lastVisibleRow is the last display row wholly on screen from the current
// offset (at least the offset row itself, however tall).
func (m model) lastVisibleRow() int {
	total := m.totalDiffRows()
	vh := m.diffViewportHeight()
	if !m.wrap {
		return max(0, min(m.diffOffset+vh, total)-1)
	}
	used := 0
	last := m.diffOffset
	for i := m.diffOffset; i < total; i++ {
		used += m.rowHeight(i)
		if used > vh && i > m.diffOffset {
			break
		}
		last = i
	}
	return last
}

// maxWrapOffset is the largest scroll offset that still fills the viewport when
// wrapping: the earliest row from which the remaining rows fit on screen.
func (m model) maxWrapOffset() int {
	total := m.totalDiffRows()
	if total == 0 {
		return 0
	}
	vh := m.diffViewportHeight()
	off := total - 1
	used := m.rowHeight(off)
	for off > 0 {
		h := m.rowHeight(off - 1)
		if used+h > vh {
			break
		}
		used += h
		off--
	}
	return off
}

// diffWindowWrapped is diffWindow for soft-wrap: whole logical rows from the
// scroll offset, each expanded to its screen rows, until the pane is full.
func (m model) diffWindowWrapped(width, rows, cursor int) []string {
	var out []string
	if m.splitView {
		leftW, rightW := splitWidths(width)
		for i := m.diffOffset; i < len(m.splitRows) && len(out) < rows; i++ {
			out = append(out, m.renderSplitRowWrapped(m.splitRows[i], leftW, rightW, i == cursor)...)
		}
	} else {
		curHit := m.currentHitIndex()
		for i := m.diffOffset; i < len(m.viewLines) && len(out) < rows; i++ {
			out = append(out, m.renderUnifiedWrapped(m.viewLines[i], width, i == cursor, i == curHit)...)
		}
	}
	if len(out) > rows {
		out = out[:rows]
	}
	return out
}

// renderUnifiedWrapped is renderUnifiedLine across as many screen rows as the
// code needs. Continuation rows keep the gutter's tint but leave the numbers and
// marker blank, so the line reads as one.
func (m model) renderUnifiedWrapped(l diff.Line, width int, sel, searchCur bool) []string {
	if l.Kind == diff.Hunk || l.Kind == diff.Meta || l.Kind == diff.Expand {
		return []string{m.renderUnifiedLine(l, width, sel, searchCur)}
	}
	numStyle, bg, emphBg, marker := lineStyles(l.Kind)
	if sel {
		numStyle, bg, emphBg = selectedRowStyle, colRowBg, nil
	}
	d := m.lineDigits
	gut := numStyle.Render(numField(l.OldNum, d) + " " + numField(l.NewNum, d) + " " + marker)
	cont := numStyle.Render(strings.Repeat(" ", 2*d+3))
	avail := max(1, width-(2*d+3))
	n := codeRows(&l, avail)
	out := make([]string, n)
	for k := 0; k < n; k++ {
		g := gut
		if k > 0 {
			g = cont
		}
		out[k] = g + m.renderCodeAt(l.Text, m.lineLexer(l), avail, k*(avail-1), false, bg, emphBg, l.Emph, m.searchRanges(l), searchCur)
	}
	return out
}

// renderSplitRowWrapped is renderSplitRow across the row's wrapped height, the
// shorter side padded with blank rows in its own tint.
func (m model) renderSplitRowWrapped(r diff.Row, leftW, rightW int, sel bool) []string {
	if r.Full != nil {
		return []string{m.renderSplitRow(r, leftW, rightW, sel)}
	}
	d := m.lineDigits
	h := max(codeRows(r.Left, leftW-d-1), codeRows(r.Right, rightW-d-1))
	left := m.renderSplitSideWrapped(r.Left, leftW, false, sel, h)
	right := m.renderSplitSideWrapped(r.Right, rightW, true, sel, h)
	out := make([]string, h)
	for k := range out {
		out[k] = left[k] + borderStyle.Render("│") + right[k]
	}
	return out
}

// renderSplitSideWrapped renders one half of a split row as exactly h screen rows.
func (m model) renderSplitSideWrapped(l *diff.Line, width int, newSide, sel bool, h int) []string {
	out := make([]string, h)
	if width < 1 {
		return out
	}
	if l == nil {
		st := fillerStyle
		if sel {
			st = selectedRowStyle
		}
		for k := range out {
			out[k] = st.Width(width).Render("")
		}
		return out
	}
	numStyle, bg, emphBg, _ := lineStyles(l.Kind)
	if sel {
		numStyle, bg, emphBg = selectedRowStyle, colRowBg, nil
	}
	num := l.OldNum
	if newSide {
		num = l.NewNum
	}
	d := m.lineDigits
	gut := numStyle.Render(numField(num, d) + " ")
	cont := numStyle.Render(strings.Repeat(" ", d+1))
	avail := max(1, width-(d+1))
	n := codeRows(l, avail)
	searchCur := l == m.currentHitLine()
	for k := range out {
		g, text := gut, l.Text
		if k > 0 {
			g = cont
		}
		if k >= n {
			text = "" // pad below a side that wrapped onto fewer rows
		}
		out[k] = g + m.renderCodeAt(text, m.lineLexer(*l), avail, k*(avail-1), false, bg, emphBg, l.Emph, m.searchRanges(*l), searchCur)
	}
	return out
}
