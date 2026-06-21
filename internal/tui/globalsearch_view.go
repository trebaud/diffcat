package tui

import (
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
)

// globalsearch_view.go renders the global search overlay: a query prompt over the
// hits grouped under category headers (Commits / Files / Code), the selected row
// a full-width highlight bar. It mirrors the palette / file-finder pickers so the
// three read as one idiom; the extra wrinkle here is the non-selectable header
// rows (each with a category glyph + count) and the blank spacers between groups.

// gsRows is how many content rows (headers + spacers + results) the overlay shows
// at once.
const gsRows = 12

// gsRow is one rendered line in the list: a category header (header set, with cat
// naming the group), a blank spacer between groups (spacer set), or a result (res
// set, idx its position in the flat result list).
type gsRow struct {
	header string
	cat    gsCategory
	spacer bool
	res    gsResult
	idx    int
}

// gsLayout interleaves category headers (and a blank spacer before each group
// after the first) between the grouped results, yielding the flat row list the
// overlay renders and windows over.
func gsLayout(results []gsResult) []gsRow {
	rows := []gsRow{}
	last := gsCategory(-1)
	for i, r := range results {
		if i == 0 || r.cat != last {
			if i != 0 {
				rows = append(rows, gsRow{spacer: true})
			}
			rows = append(rows, gsRow{header: r.cat.label(), cat: r.cat})
			last = r.cat
		}
		rows = append(rows, gsRow{res: r, idx: i})
	}
	return rows
}

// glyph is the small marker shown beside a category header so the groups are
// scannable at a glance, not distinguished by their label alone.
func (c gsCategory) glyph() string {
	switch c {
	case gsCommit:
		return "●"
	case gsFile:
		return "◆"
	case gsCode:
		return "■"
	}
	return "•"
}

// tint is the category's accent color: brand magenta for commits, the folder cyan
// for files, the selection blue for code — each already in the palette.
func (c gsCategory) tint() color.Color {
	switch c {
	case gsCommit:
		return colAccent
	case gsFile:
		return colMeta
	case gsCode:
		return colSelect
	}
	return colMuted
}

// globalSearchBox builds the overlay as a floating window: a prompt line, a hint,
// then the windowed, grouped result list with a key-hint footer.
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

	counts := map[gsCategory]int{}
	for _, r := range results {
		counts[r.cat]++
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

	ob := lipgloss.NewStyle().Background(colOverlayBg)
	hint := "search commits · files · code"
	if len(results) > 0 {
		hint = pluralMatches(len(results))
	}
	lines := []string{
		padOverlay(ob.Foreground(colSelect).Bold(true).Render("⌕ ")+ob.Render(m.gsInput)+ob.Foreground(colSelect).Bold(true).Render("▌"), w),
		padOverlay(ob.Foreground(colMuted).Render(hint), w),
		padOverlay("", w),
	}
	if strings.TrimSpace(m.gsInput) == "" {
		lines = append(lines, padOverlay(ob.Foreground(colMuted).Render("  type to search across the repo"), w))
	} else if len(results) == 0 {
		lines = append(lines, padOverlay(ob.Foreground(colMuted).Render("  no matches"), w))
	}
	shown := 0
	for i := offset; i < end; i++ {
		r := rows[i]
		switch {
		case r.spacer:
			lines = append(lines, padOverlay("", w))
		case r.header != "":
			lines = append(lines, gsHeaderRow(r.cat, counts[r.cat], w))
		default:
			lines = append(lines, m.gsResultRow(r.res, r.idx == sel, w))
		}
		shown++
	}
	// Pad to a stable height so the box doesn't jump as the result count changes.
	for ; shown < gsRows; shown++ {
		lines = append(lines, padOverlay("", w))
	}
	// Key-hint footer, set off from the list by a blank line.
	lines = append(lines,
		padOverlay("", w),
		padOverlay(ob.Foreground(colMuted).Render("  ↵ open   ↑↓ move   esc close"), w),
	)
	return floatingBox(strings.Join(lines, "\n"))
}

// padOverlay right-pads s to width with cells painted in the overlay background,
// so a line never falls back to the terminal's default background after an inner
// style reset — the cause of the patchy backgrounds the naive padLine left behind.
func padOverlay(s string, width int) string {
	s = lipgloss.NewStyle().MaxWidth(width).Render(s)
	if gap := width - lipgloss.Width(s); gap > 0 {
		s += lipgloss.NewStyle().Background(colOverlayBg).Render(strings.Repeat(" ", gap))
	}
	return s
}

// gsHeaderRow renders a category header: a tinted glyph, the label, and the
// group's match count flush-right — so each group reads as a labelled section.
func gsHeaderRow(cat gsCategory, count, width int) string {
	ob := lipgloss.NewStyle().Background(colOverlayBg)
	glyph := ob.Foreground(cat.tint()).Render(cat.glyph())
	label := ob.Foreground(colMuted).Bold(true).Render(cat.label())
	cnt := ob.Foreground(colMuted).Render(strconv.Itoa(count))
	leftW := 2 + lipgloss.Width(cat.glyph()) + 1 + lipgloss.Width(cat.label())
	gap := width - leftW - lipgloss.Width(strconv.Itoa(count))
	if gap < 1 {
		gap = 1
	}
	row := ob.Render("  ") + glyph + ob.Render(" ") + label + ob.Render(strings.Repeat(" ", gap)) + cnt
	return padOverlay(row, width)
}

// selectedDisplayRow returns the index of the selected result within the
// interleaved row list (headers and spacers shift result positions down), or 0.
func selectedDisplayRow(rows []gsRow, sel int) int {
	for i, r := range rows {
		if r.header == "" && !r.spacer && r.idx == sel {
			return i
		}
	}
	return 0
}

// gsResultRow renders one hit: a caret column (filled ▸ only when selected, so
// rows align and the selection never relies on color alone), the title (matched
// runes emphasised), and the muted context sub-label flush-right. The selected
// row is one continuous highlight bar.
func (m model) gsResultRow(r gsResult, selected bool, width int) string {
	caret := "  "
	if selected {
		caret = "▸ "
	}
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
		row := "  " + caret + title + strings.Repeat(" ", gap)
		if subW > 0 {
			row += r.sub
		}
		return selectedRowStyle.Width(width).Render(row)
	}
	ob := lipgloss.NewStyle().Background(colOverlayBg)
	styled := gsEmphasise(title, r.hit)
	gap := width - 4 - lipgloss.Width(title) - subW
	if gap < 1 {
		gap = 1
	}
	row := ob.Render("  "+caret) + styled + ob.Render(strings.Repeat(" ", gap))
	if subW > 0 {
		row += ob.Foreground(colMuted).Render(r.sub)
	}
	return padOverlay(row, width)
}

// gsEmphasise bolds the [start,end) matched rune range within text, painting every
// rune on the overlay background so the title never falls back to the terminal's
// default background. A zero range (no match in the visible title, e.g. a commit
// matched only in its body) renders plain. Offsets past the truncation point are
// simply not reached.
func gsEmphasise(text string, hit [2]int) string {
	base := lipgloss.NewStyle().Background(colOverlayBg)
	hitStyle := base.Foreground(colSelect).Bold(true)
	if hit[1] <= hit[0] {
		return base.Render(text)
	}
	runes := []rune(text)
	lo := min(max(hit[0], 0), len(runes))
	hi := min(max(hit[1], lo), len(runes))
	var b strings.Builder
	b.WriteString(base.Render(string(runes[:lo])))
	b.WriteString(hitStyle.Render(string(runes[lo:hi])))
	b.WriteString(base.Render(string(runes[hi:])))
	return b.String()
}
