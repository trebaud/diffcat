package tui

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/trebaud/diff-master/internal/diff"
	"github.com/trebaud/diff-master/internal/git"
)

// View renders the full screen: a header bar, the file list beside the diff
// pane, and a footer. A help overlay replaces the body when toggled.
func (m model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	return v
}

// Minimum usable terminal size. Below this the two-pane layout can't render
// legibly, so we show a resize hint instead of a broken screen.
const (
	minWidth  = 60
	minHeight = 12
)

func (m model) render() string {
	if m.width == 0 || m.height == 0 {
		return "loading…"
	}
	if m.width < minWidth || m.height < minHeight {
		return m.tooSmallView()
	}
	if m.showHelp {
		return m.helpView()
	}

	// Proportional split with a cap: the file list takes ~35% but never grows
	// past 40 cols (lists don't benefit from more) nor shrinks below 22.
	listWidth := m.width * 35 / 100
	if listWidth > 40 {
		listWidth = 40
	}
	if listWidth < 22 {
		listWidth = 22
	}
	diffWidth := m.width - listWidth - 1

	// The body fills everything between the one-line header and footer. We pin
	// both panes and the divider to that exact height so the TUI occupies the
	// whole terminal — empty regions are still part of a full-height pane, not
	// dead space the layout leaves behind.
	bodyHeight := m.height - 2
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	fill := lipgloss.NewStyle().Height(bodyHeight).MaxHeight(bodyHeight)

	list := fill.Render(m.listView(listWidth))
	diff := fill.Render(m.diffView(diffWidth))
	body := lipgloss.JoinHorizontal(lipgloss.Top, list, m.divider(bodyHeight), diff)

	// The chrome rows span the full width: truncate (never wrap) then pad.
	header := padLine(m.headerView(), m.width)
	footer := padLine(m.footerView(), m.width)
	return strings.Join([]string{header, body, footer}, "\n")
}

func (m model) headerView() string {
	base := m.baseName
	if base == "" {
		base = "base"
	}
	left := titleStyle.Render("diff-master")
	mid := mutedStyle.Render(fmt.Sprintf("  %s ← %s", branchLabel(m.branch), base))
	stat := ""
	if m.shortstat != "" {
		stat = "  " + headingStyle.Render(m.shortstat)
	}
	return left + mid + stat
}

// padLine truncates a single styled line to width (without wrapping) and pads
// it with trailing spaces so it spans exactly width columns.
func padLine(s string, width int) string {
	s = lipgloss.NewStyle().MaxWidth(width).Render(s)
	if gap := width - lipgloss.Width(s); gap > 0 {
		s += strings.Repeat(" ", gap)
	}
	return s
}

// divider is a full-height vertical rule between the two panes so the split is
// visible all the way down the screen, not just where content happens to reach.
func (m model) divider(height int) string {
	if height < 1 {
		height = 1
	}
	bar := borderStyle.Render("│")
	rows := make([]string, height)
	for i := range rows {
		rows[i] = bar
	}
	return strings.Join(rows, "\n")
}

func branchLabel(b string) string {
	if b == "" {
		return "(working tree)"
	}
	return b
}

func (m model) listView(width int) string {
	rows := m.listViewportHeight()
	var b strings.Builder

	b.WriteString(m.paneHeading(fmt.Sprintf("Changed files (%d)", len(m.files)), focusFiles))
	b.WriteString("\n")

	if len(m.files) == 0 {
		b.WriteString(mutedStyle.Render("  no changes against base"))
		return lipgloss.NewStyle().Width(width).Render(b.String())
	}

	// Scroll the list to keep the cursor visible.
	offset := 0
	if m.cursor >= rows {
		offset = m.cursor - rows + 1
	}

	end := offset + rows
	if end > len(m.files) {
		end = len(m.files)
	}
	for i := offset; i < end; i++ {
		b.WriteString(m.fileRow(m.files[i], i == m.cursor, width))
		b.WriteString("\n")
	}
	return lipgloss.NewStyle().Width(width).Render(b.String())
}

// fileRow renders one entry: "▸ M path/to/file.go        +12 -3", with the
// stats flush-right and (when selected) a full-width highlight bar. We measure
// with plain text so alignment is exact, then colorize per segment — except a
// selected row, where the selection color intentionally overrides the syntax
// colors so the cursor reads unambiguously.
func (m model) fileRow(f git.FileChange, selected bool, width int) string {
	glyph := statusGlyph(f.Status)

	statsPlain := "bin"
	if !f.Binary() {
		statsPlain = fmt.Sprintf("+%d -%d", f.Added, f.Deleted)
	}

	caret := "  "
	if selected {
		caret = "▸ "
	}

	// Layout budget: caret(2) + glyph(1) + space(1) + name + gap + stats.
	avail := width - 2 - 1 - 1 - lipgloss.Width(statsPlain) - 1
	if avail < 4 {
		avail = 4
	}
	name := truncatePath(f.Path, avail)

	gap := width - 2 - 1 - 1 - lipgloss.Width(name) - lipgloss.Width(statsPlain)
	if gap < 1 {
		gap = 1
	}

	if selected {
		row := caret + glyph + " " + name + strings.Repeat(" ", gap) + statsPlain
		return selectedRowStyle.Width(width).Render(row)
	}

	stats := mutedStyle.Render(statsPlain)
	if !f.Binary() {
		stats = addedStyle.Render(fmt.Sprintf("+%d", f.Added)) + " " +
			removedStyle.Render(fmt.Sprintf("-%d", f.Deleted))
	}
	return caret + statusStyle(f.Status).Render(glyph) + " " +
		name + strings.Repeat(" ", gap) + stats
}

// truncatePath keeps the filename visible by trimming the left (directory) side.
func truncatePath(path string, max int) string {
	if lipgloss.Width(path) <= max || max < 2 {
		return path
	}
	r := []rune(path)
	return "…" + string(r[len(r)-(max-1):])
}

func (m model) tooSmallView() string {
	msg := fmt.Sprintf("terminal too small (%d×%d) — need at least %d×%d",
		m.width, m.height, minWidth, minHeight)
	pad := (m.height - 1) / 2
	if pad < 0 {
		pad = 0
	}
	return strings.Repeat("\n", pad) + " " + mutedStyle.Render(msg)
}

func (m model) diffView(width int) string {
	rows := m.diffViewportHeight()
	var b strings.Builder

	f := m.selectedFile()
	if f == nil {
		return lipgloss.NewStyle().Width(width).Render(mutedStyle.Render("Select a file to view its diff."))
	}

	// Heading: file path + a mode tag. paneHeading prepends a 2-col marker.
	mode := ""
	if m.splitView {
		mode = " [split]"
	}
	title := truncatePath(f.Path, width-2-lipgloss.Width(mode)) + mode
	b.WriteString(m.paneHeading(title, focusDiff))
	b.WriteString("\n")

	lines := m.diffWindow(width, rows)
	for _, l := range lines {
		b.WriteString(l)
		b.WriteString("\n")
	}
	// Pad to fill the pane so the nyan progress bar always pins to the bottom.
	for shown := len(lines); shown < rows; shown++ {
		b.WriteString("\n")
	}

	b.WriteString(m.nyanProgress(width))

	return lipgloss.NewStyle().Width(width).Render(b.String())
}

// diffWindow renders the visible slice of diff rows for the current view mode.
func (m model) diffWindow(width, rows int) []string {
	var out []string
	if m.splitView {
		leftW := (width - 1) / 2
		rightW := width - 1 - leftW
		end := min(m.diffOffset+rows, len(m.splitRows))
		for i := m.diffOffset; i < end; i++ {
			out = append(out, m.renderSplitRow(m.splitRows[i], leftW, rightW))
		}
		return out
	}
	end := min(m.diffOffset+rows, len(m.diff))
	for i := m.diffOffset; i < end; i++ {
		out = append(out, m.renderUnifiedLine(m.diff[i], width))
	}
	return out
}

// renderUnifiedLine renders one inline-diff row: "old new ± code" with the whole
// row tinted green (add) or red (del), GitHub-style.
func (m model) renderUnifiedLine(l diff.Line, width int) string {
	switch l.Kind {
	case diff.Hunk:
		return hunkLineStyle.Width(width).Render(truncateText(l.Text, width))
	case diff.Meta:
		return metaLineStyle.Width(width).Render(truncateText(l.Text, width))
	}

	numStyle, bodyStyle, marker := lineStyles(l.Kind)
	d := m.lineDigits
	gut := numStyle.Render(numField(l.OldNum, d) + " " + numField(l.NewNum, d) + " " + marker)
	avail := width - lipgloss.Width(gut)
	if avail < 1 {
		avail = 1
	}
	body := bodyStyle.Width(avail).Render(truncateText(" "+l.Text, avail))
	return gut + body
}

// renderSplitRow renders one side-by-side row: old/del on the left, new/add on
// the right, divided by a vertical rule. Hunk/Meta rows span the full width.
func (m model) renderSplitRow(r diff.Row, leftW, rightW int) string {
	if r.Full != nil {
		w := leftW + 1 + rightW
		if r.Full.Kind == diff.Hunk {
			return hunkLineStyle.Width(w).Render(truncateText(r.Full.Text, w))
		}
		return metaLineStyle.Width(w).Render(truncateText(r.Full.Text, w))
	}
	left := m.renderSplitSide(r.Left, leftW, false)
	right := m.renderSplitSide(r.Right, rightW, true)
	return left + borderStyle.Render("│") + right
}

// renderSplitSide renders one half of a split row. A nil line is an empty paired
// slot, filled with a faint background so the gap reads as intentional.
func (m model) renderSplitSide(l *diff.Line, width int, newSide bool) string {
	if width < 1 {
		return ""
	}
	if l == nil {
		return fillerStyle.Width(width).Render("")
	}
	numStyle, bodyStyle, _ := lineStyles(l.Kind)
	num := l.OldNum
	if newSide {
		num = l.NewNum
	}
	gut := numStyle.Render(numField(num, m.lineDigits) + " ")
	avail := width - lipgloss.Width(gut)
	if avail < 1 {
		avail = 1
	}
	body := bodyStyle.Width(avail).Render(truncateText(" "+l.Text, avail))
	return gut + body
}

// lineStyles returns the gutter style, body style, and marker for a line kind.
func lineStyles(kind diff.Kind) (num, body lipgloss.Style, marker string) {
	switch kind {
	case diff.Add:
		return addNumStyle, addBodyStyle, "+"
	case diff.Del:
		return delNumStyle, delBodyStyle, "-"
	default:
		return ctxNumStyle, ctxBodyStyle, " "
	}
}

// numField right-aligns a line number in a fixed-width field, blank for 0.
func numField(n, digits int) string {
	if n <= 0 {
		return strings.Repeat(" ", digits)
	}
	return fmt.Sprintf("%*d", digits, n)
}

// truncateText trims a line to max display columns, adding an ellipsis.
func truncateText(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= max {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && lipgloss.Width(string(r)) > max-1 {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}

// nyanProgress renders the diff scroll position as a nyan cat marching from the
// top of the file (left edge) to the end (right edge), trailing a rainbow. The
// cat's face/legs wiggle on each tick; the diff content fully read is the
// rainbow length behind it.
func (m model) nyanProgress(width int) string {
	rows := m.diffViewportHeight()
	maxOff := m.totalDiffRows() - rows
	frac := 1.0 // whole diff fits on screen → already at the end
	if maxOff > 0 {
		frac = float64(m.diffOffset) / float64(maxOff)
	}
	return nyanBar(width, frac, m.animFrame)
}

func nyanBar(width int, frac float64, frame int) string {
	if width < 8 {
		return strings.Repeat(" ", max(0, width))
	}
	switch {
	case frac < 0:
		frac = 0
	case frac > 1:
		frac = 1
	}

	// 2-frame gait. The cat is the pink pop-tart nyan.
	cat := "=^.^="
	if frame%2 == 1 {
		cat = "=^-^="
	}
	catW := lipgloss.Width(cat)

	maxX := width - catW
	if maxX < 1 {
		maxX = 1
	}
	catX := int(frac*float64(maxX) + 0.5)

	// ANSI rainbow: red, yellow, green, cyan, blue, magenta.
	trail := []color.Color{
		lipgloss.Color("1"), lipgloss.Color("3"), lipgloss.Color("2"),
		lipgloss.Color("6"), lipgloss.Color("4"), lipgloss.Color("5"),
	}
	n := len(trail)

	var b strings.Builder
	// Rainbow trail behind the cat, in 2-char blocks that shimmer per frame.
	for blk := 0; blk*2 < catX; blk++ {
		start := blk * 2
		segEnd := start + 2
		if segEnd > catX {
			segEnd = catX
		}
		s := lipgloss.NewStyle().Foreground(trail[(blk+frame)%n])
		b.WriteString(s.Render(strings.Repeat("━", segEnd-start)))
	}
	b.WriteString(selectedStyle.Render(cat)) // accent-pink cat
	// Faint track ahead of the cat shows how far is left.
	if right := width - catX - catW; right > 0 {
		b.WriteString(borderStyle.Render(strings.Repeat("─", right)))
	}
	return b.String()
}

// paneHeading renders a pane title, accented with a bar when that pane holds
// focus so the current target of j/k/gg/G is obvious.
func (m model) paneHeading(text string, pane focusPane) string {
	if m.focus == pane {
		return selectedStyle.Render("▌ " + text)
	}
	return headingStyle.Render("  " + text)
}

func (m model) footerView() string {
	keys := []string{
		"j/k move", "h/l ⇄ pane", "gg/G top/bot", "C-d/C-u half", "s split", "r refresh", "? help", "q quit",
	}
	return mutedStyle.Render(strings.Join(keys, "  ·  "))
}

func (m model) helpView() string {
	lines := []string{
		titleStyle.Render("diff-master — vim keybindings"),
		"",
		headingStyle.Render("  panes"),
		"  h / l        focus file list / diff pane",
		"  Tab          toggle focused pane",
		"  Enter        open diff of selected file",
		"",
		headingStyle.Render("  motions (act on the focused pane)"),
		"  j / k        down / up one line",
		"  gg / G       jump to top / bottom",
		"  ctrl+d / u   half page down / up",
		"  ctrl+f / b   full page down / up",
		"",
		headingStyle.Render("  view"),
		"  s            toggle unified / side-by-side",
		"  r            refresh from disk",
		"  ? / q        toggle help / quit",
		"",
		mutedStyle.Render("  press any key to dismiss"),
	}
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colBorder).
		Padding(1, 2).
		Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, box)
}
