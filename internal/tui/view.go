package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

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

	header := m.headerView()
	footer := m.footerView()

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

	list := m.listView(listWidth)
	diff := m.diffView(diffWidth)
	body := lipgloss.JoinHorizontal(lipgloss.Top, list, borderStyle.Render(" "), diff)

	// Clamp the single-line chrome so it can never wrap and shove the body down.
	clamp := lipgloss.NewStyle().MaxWidth(m.width)
	return strings.Join([]string{clamp.Render(header), body, clamp.Render(footer)}, "\n")
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

	// paneHeading prepends a 2-col focus marker, so reserve that before trimming.
	b.WriteString(m.paneHeading(truncatePath(f.Path, width-2), focusDiff))
	b.WriteString("\n")

	end := m.diffOffset + rows
	if end > len(m.diffLines) {
		end = len(m.diffLines)
	}
	for i := m.diffOffset; i < end; i++ {
		b.WriteString(colorizeDiffLine(m.diffLines[i], width))
		b.WriteString("\n")
	}

	// Scroll indicator when there's more below.
	if len(m.diffLines) > rows {
		pct := 100
		if max := len(m.diffLines) - rows; max > 0 {
			pct = m.diffOffset * 100 / max
		}
		b.WriteString(mutedStyle.Render(fmt.Sprintf("  %d%%", pct)))
	}

	return lipgloss.NewStyle().Width(width).MaxWidth(width).Render(b.String())
}

// colorizeDiffLine styles a single unified-diff line by its leading marker.
func colorizeDiffLine(line string, width int) string {
	if width > 2 && len(line) > width {
		line = line[:width-1] + "…"
	}
	switch {
	case strings.HasPrefix(line, "@@"):
		return metaStyle.Render(line)
	case strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "---"):
		return headingStyle.Render(line)
	case strings.HasPrefix(line, "diff "), strings.HasPrefix(line, "index "),
		strings.HasPrefix(line, "new file"), strings.HasPrefix(line, "deleted file"),
		strings.HasPrefix(line, "rename "), strings.HasPrefix(line, "similarity "):
		return mutedStyle.Render(line)
	case strings.HasPrefix(line, "+"):
		return addedStyle.Render(line)
	case strings.HasPrefix(line, "-"):
		return removedStyle.Render(line)
	default:
		return contextStyle.Render(line)
	}
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
		"j/k move", "h/l ⇄ pane", "gg/G top/bot", "C-d/C-u half", "C-f/C-b page", "r refresh", "? help", "q quit",
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
		headingStyle.Render("  other"),
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
