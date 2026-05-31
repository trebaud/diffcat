package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
)

// find_view.go renders the fuzzy file-jump picker (the `f` overlay): a query
// prompt over a ranked, scrollable list of matching changed-file paths.

// fileFindRows is how many result rows the picker shows at once.
const fileFindRows = 12

// fileFindBox builds the picker as a floating window: a prompt line, a match
// count, then up to fileFindRows results windowed around the selection — the
// selected row a full-width highlight bar, the rest with their fuzzy-matched
// characters bolded.
func (m model) fileFindBox() string {
	matches := m.fileFindMatches()
	w := m.width - 6
	if w > 60 {
		w = 60
	}
	if w < 20 {
		w = 20
	}

	sel := m.fileFindSel
	if sel >= len(matches) {
		sel = max(0, len(matches)-1)
	}
	offset := 0
	if sel >= fileFindRows {
		offset = sel - fileFindRows + 1
	}
	end := min(offset+fileFindRows, len(matches))

	lines := []string{
		padLine(selectedStyle.Render("❯ ")+m.fileFindInput+selectedStyle.Render("▌"), w),
		padLine(mutedStyle.Render(pluralMatches(len(matches))), w),
		padLine("", w),
	}
	if len(matches) == 0 {
		lines = append(lines, padLine(mutedStyle.Render("  no files match"), w))
	}
	shown := 0
	for i := offset; i < end; i++ {
		lines = append(lines, m.fileFindRow(matches[i], i == sel, w))
		shown++
	}
	// Pad to a stable height so the box doesn't jump as the result count changes.
	for ; shown < fileFindRows; shown++ {
		lines = append(lines, padLine("", w))
	}
	return floatingBox(strings.Join(lines, "\n"))
}

func pluralMatches(n int) string {
	if n == 1 {
		return "1 match"
	}
	return fmt.Sprintf("%d matches", n)
}

// fileFindRow renders one result. The selected row is a continuous highlight
// bar; unselected rows bold the fuzzy-matched characters in the path.
func (m model) fileFindRow(fm fileMatch, selected bool, width int) string {
	if selected {
		return selectedRowStyle.Width(width).Render("  " + truncatePath(fm.path, width-2))
	}
	return padLine("  "+styleFuzzyPath(fm.path, fm.hit, width-2), width)
}

// styleFuzzyPath renders a path bolding the rune offsets in hit, left-truncating
// (keeping the filename visible) with a leading ellipsis when it's too long. Hit
// offsets that fall in the trimmed-off prefix are simply not reached.
func styleFuzzyPath(path string, hit []int, max int) string {
	if max < 2 {
		max = 2
	}
	runes := []rune(path)
	hitSet := make(map[int]bool, len(hit))
	for _, h := range hit {
		hitSet[h] = true
	}
	start := 0
	if len(runes) > max {
		start = len(runes) - (max - 1) // leave a column for the ellipsis
	}
	hitStyle := lipgloss.NewStyle().Foreground(colSelect).Bold(true)
	var b strings.Builder
	if start > 0 {
		b.WriteString(mutedStyle.Render("…"))
	}
	for i := start; i < len(runes); i++ {
		if hitSet[i] {
			b.WriteString(hitStyle.Render(string(runes[i])))
		} else {
			b.WriteString(string(runes[i]))
		}
	}
	return b.String()
}
