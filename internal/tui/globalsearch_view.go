package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// globalsearch_view.go renders the global search overlay: a query prompt over the
// hits grouped under category headers (Commits / Files / Code), the selected row
// a full-width highlight bar. It mirrors the palette / file-finder pickers so the
// three read as one idiom; the extra wrinkle here is the non-selectable header
// rows interleaved between groups.

// gsRows is how many content rows (headers + results) the overlay shows at once.
const gsRows = 14

// gsRow is one rendered line in the list: a category header (header set, not
// selectable) or a result (res set, idx its position in the flat result list).
type gsRow struct {
	header string
	res    gsResult
	idx    int
}

// gsLayout interleaves category headers between the grouped results, yielding the
// flat row list the overlay renders and windows over.
func gsLayout(results []gsResult) []gsRow {
	rows := []gsRow{}
	last := gsCategory(-1)
	for i, r := range results {
		if i == 0 || r.cat != last {
			rows = append(rows, gsRow{header: r.cat.label()})
			last = r.cat
		}
		rows = append(rows, gsRow{res: r, idx: i})
	}
	return rows
}

// globalSearchBox builds the overlay as a floating window: a prompt line, a hint,
// then the windowed, grouped result list.
func (m model) globalSearchBox() string {
	results := m.gsResults()
	rows := gsLayout(results)
	w := m.width - 6
	if w > 64 {
		w = 64
	}
	if w < 28 {
		w = 28
	}

	sel := m.gsSel
	if sel >= len(results) {
		sel = max(0, len(results)-1)
	}

	// Window the rows around the selected result so it stays visible as the list
	// scrolls past gsRows.
	selRow := selectedDisplayRow(rows, sel)
	offset := 0
	if selRow >= gsRows {
		offset = selRow - gsRows + 1
	}
	end := min(offset+gsRows, len(rows))

	hint := mutedStyle.Render("search commits · files · code")
	if len(results) > 0 {
		hint = mutedStyle.Render(pluralMatches(len(results)))
	}
	lines := []string{
		padLine(selectedStyle.Render("⌕ ")+m.gsInput+selectedStyle.Render("▌"), w),
		padLine(hint, w),
		padLine("", w),
	}
	if strings.TrimSpace(m.gsInput) == "" {
		lines = append(lines, padLine(mutedStyle.Render("  type to search across the repo"), w))
	} else if len(results) == 0 {
		lines = append(lines, padLine(mutedStyle.Render("  no matches"), w))
	}
	shown := 0
	for i := offset; i < end; i++ {
		if r := rows[i]; r.header != "" {
			lines = append(lines, padLine("  "+headingStyle.Render(r.header), w))
		} else {
			lines = append(lines, m.gsResultRow(r.res, r.idx == sel, w))
		}
		shown++
	}
	// Pad to a stable height so the box doesn't jump as the result count changes.
	for ; shown < gsRows; shown++ {
		lines = append(lines, padLine("", w))
	}
	return floatingBox(strings.Join(lines, "\n"))
}

// selectedDisplayRow returns the index of the selected result within the
// interleaved row list (headers shift result positions down), or 0.
func selectedDisplayRow(rows []gsRow, sel int) int {
	for i, r := range rows {
		if r.header == "" && r.idx == sel {
			return i
		}
	}
	return 0
}

// gsResultRow renders one hit: its title (matched runes emphasised) on the first
// part of the line with the muted context sub-label flush-right. The selected row
// is one continuous highlight bar.
func (m model) gsResultRow(r gsResult, selected bool, width int) string {
	subW := lipgloss.Width(r.sub)
	titleW := width - 4 - subW - 1
	if titleW < 6 {
		titleW = 6
		subW = 0
	}
	title := truncateText(r.title, titleW)
	if selected {
		gap := width - 4 - lipgloss.Width(title) - subW
		if gap < 1 {
			gap = 1
		}
		row := "    " + title + strings.Repeat(" ", gap)
		if subW > 0 {
			row += r.sub
		}
		return selectedRowStyle.Width(width).Render(row)
	}
	styled := gsEmphasise(title, r.hit)
	gap := width - 4 - lipgloss.Width(title) - subW
	if gap < 1 {
		gap = 1
	}
	row := "    " + styled + strings.Repeat(" ", gap)
	if subW > 0 {
		row += mutedStyle.Render(r.sub)
	}
	return padLine(row, width)
}

// gsEmphasise bolds the [start,end) matched rune range within text. A zero range
// (no match in the visible title, e.g. a commit matched only in its body) renders
// plain. Offsets past the truncation point are simply not reached.
func gsEmphasise(text string, hit [2]int) string {
	if hit[1] <= hit[0] {
		return text
	}
	runes := []rune(text)
	hitStyle := lipgloss.NewStyle().Foreground(colSelect).Bold(true)
	var b strings.Builder
	for i, r := range runes {
		if i >= hit[0] && i < hit[1] {
			b.WriteString(hitStyle.Render(string(r)))
		} else {
			b.WriteString(string(r))
		}
	}
	return b.String()
}
