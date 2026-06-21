package tui

import (
	"strings"

	"charm.land/lipgloss/v2"
)

// palette_view.go renders the command palette overlay: a query prompt over a
// ranked, scrollable list of actions, each with its keybinding flush-right. It
// mirrors the file-finder picker (find_view.go) so the two read as one idiom.

// paletteRows is how many action rows the palette shows at once.
const paletteRows = 10

// paletteBox builds the palette as a floating window: a prompt line, a hint, then
// the windowed action list with the selected row as a full-width highlight bar.
func (m model) paletteBox() string {
	matches := m.paletteMatches()
	w := m.width - 6
	if w > 56 {
		w = 56
	}
	if w < 24 {
		w = 24
	}

	sel := m.paletteSel
	if sel >= len(matches) {
		sel = max(0, len(matches)-1)
	}
	offset := 0
	if sel >= paletteRows {
		offset = sel - paletteRows + 1
	}
	end := min(offset+paletteRows, len(matches))

	lines := []string{
		padLine(selectedStyle.Render("⌘ ")+m.paletteInput+selectedStyle.Render("▌"), w),
		padLine(mutedStyle.Render("run a command"), w),
		padLine("", w),
	}
	if len(matches) == 0 {
		lines = append(lines, padLine(mutedStyle.Render("  no commands match"), w))
	}
	shown := 0
	for i := offset; i < end; i++ {
		lines = append(lines, m.paletteRow(matches[i], i == sel, w))
		shown++
	}
	for ; shown < paletteRows; shown++ {
		lines = append(lines, padLine("", w))
	}
	return floatingBox(strings.Join(lines, "\n"))
}

// paletteRow renders one action: its label (fuzzy-matched runes bolded) with the
// keybinding hint flush-right. The selected row is one continuous highlight bar.
func (m model) paletteRow(pm paletteMatch, selected bool, width int) string {
	keys := pm.action.keys
	keyW := lipgloss.Width(keys)
	labelW := width - 2 - keyW - 1
	if labelW < 3 {
		labelW = 3
	}
	label := truncateText(pm.action.label, labelW)
	gap := width - 2 - lipgloss.Width(label) - keyW
	if gap < 1 {
		gap = 1
	}
	if selected {
		return selectedRowStyle.Width(width).Render("  " + label + strings.Repeat(" ", gap) + keys)
	}
	row := "  " + styleFuzzyPath(label, pm.hit, labelW) + strings.Repeat(" ", gap)
	if keys != "" {
		row += mutedStyle.Render(keys)
	}
	return padLine(row, width)
}
