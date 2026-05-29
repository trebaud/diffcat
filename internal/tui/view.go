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

func (m model) render() string {
	if m.width == 0 || m.height == 0 {
		return "loading…"
	}
	if m.showHelp {
		return m.helpView()
	}

	header := m.headerView()
	footer := m.footerView()

	const listWidth = 34
	diffWidth := m.width - listWidth - 1
	if diffWidth < 20 {
		diffWidth = m.width // narrow terminal: drop to single column-ish
	}

	list := m.listView(listWidth)
	diff := m.diffView(diffWidth)
	body := lipgloss.JoinHorizontal(lipgloss.Top, list, borderStyle.Render(" "), diff)

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

func (m model) fileRow(f git.FileChange, selected bool, width int) string {
	glyph := statusStyle(f.Status).Render(statusGlyph(f.Status))

	name := f.Path
	// Budget: 2 (glyph+space) + name + stats. Truncate the path's left side so
	// the filename stays visible.
	maxName := width - 14
	if maxName < 6 {
		maxName = 6
	}
	if len(name) > maxName {
		name = "…" + name[len(name)-maxName+1:]
	}

	stats := ""
	if f.Binary() {
		stats = mutedStyle.Render("bin")
	} else {
		stats = addedStyle.Render(fmt.Sprintf("+%d", f.Added)) + " " +
			removedStyle.Render(fmt.Sprintf("-%d", f.Deleted))
	}

	line := fmt.Sprintf("%s %s", glyph, name)
	if selected {
		line = selectedStyle.Render("▸ " + name)
		line = glyph + " " + line
	} else {
		line = "  " + line
	}
	return line + "  " + stats
}

func (m model) diffView(width int) string {
	rows := m.diffViewportHeight()
	var b strings.Builder

	f := m.selectedFile()
	if f == nil {
		return lipgloss.NewStyle().Width(width).Render(mutedStyle.Render("Select a file to view its diff."))
	}

	b.WriteString(m.paneHeading(f.Path, focusDiff))
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
