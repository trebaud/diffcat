package tui

import (
	"fmt"
	"image/color"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/alecthomas/chroma/v2"

	"github.com/trebaud/diffcat/internal/diff"
	"github.com/trebaud/diffcat/internal/git"
)

// View renders the full screen: a header bar, the file list beside the diff
// pane, and a footer. A help overlay replaces the body when toggled.
func (m model) View() tea.View {
	v := tea.NewView(m.render())
	v.AltScreen = true
	// Drive the terminal canvas from the theme so a light theme actually turns
	// the whole screen light (nil in dark mode = keep the terminal's own colors).
	v.BackgroundColor = colCanvas
	v.ForegroundColor = colText
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

	var left, right string
	if m.mode == viewLog {
		left = fill.Render(m.commitListView(listWidth))
		right = fill.Render(m.commitDiffView(diffWidth))
	} else {
		left = fill.Render(m.listView(listWidth))
		right = fill.Render(m.diffView(diffWidth))
	}
	body := lipgloss.JoinHorizontal(lipgloss.Top, left, m.divider(bodyHeight), right)

	// The chrome rows span the full width: truncate (never wrap) then pad.
	header := padLine(m.headerView(), m.width)
	footer := padLine(m.footerView(), m.width)
	return strings.Join([]string{header, body, footer}, "\n")
}

func (m model) headerView() string {
	left := titleStyle.Render("diffcat")
	if m.mode == viewLog {
		mid := mutedStyle.Render(fmt.Sprintf("  %s · history", branchLabel(m.branch)))
		count := "  " + headingStyle.Render(fmt.Sprintf("%d commits", len(m.commits)))
		return left + mid + count
	}
	if m.mode == viewCommit {
		if m.scopeWorking {
			mid := mutedStyle.Render("  history · working tree")
			return left + mid + "  " + headingStyle.Render("uncommitted changes")
		}
		c := m.scopeCommit
		sha, subject := "", ""
		if c != nil {
			sha, subject = c.Short, c.Subject
		}
		mid := mutedStyle.Render(fmt.Sprintf("  history · commit %s", sha))
		return left + mid + "  " + headingStyle.Render(subject)
	}
	base := m.baseName
	if base == "" {
		base = "base"
	}
	// Make the comparison explicit and visually distinct: the current branch in
	// bright text, an arrow, then the base branch as a blue pill (tagged "default"
	// when it's the repo default master/main), so it's never ambiguous what the
	// diff is measured against.
	mid := "  " + branchStyle.Render(branchLabel(m.branch))
	mid += mutedStyle.Render(" → ")
	mid += baseBadgeStyle.Render(" " + base + " ")
	if m.baseIsDefault {
		mid += mutedStyle.Render(" default")
	}
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

	if len(m.rows) == 0 {
		msg := "  no changes against base"
		if m.mode == viewCommit {
			msg = "  no files in this commit"
			if m.scopeWorking {
				msg = "  no uncommitted changes"
			}
		}
		b.WriteString(mutedStyle.Render(msg))
		return lipgloss.NewStyle().Width(width).Render(b.String())
	}

	// Scroll the tree to keep the cursor visible.
	offset := 0
	if m.cursor >= rows {
		offset = m.cursor - rows + 1
	}

	end := offset + rows
	if end > len(m.rows) {
		end = len(m.rows)
	}
	for i := offset; i < end; i++ {
		b.WriteString(m.treeRow(m.rows[i], i == m.cursor, width))
		b.WriteString("\n")
	}
	return lipgloss.NewStyle().Width(width).Render(b.String())
}

// commitListView renders the left pane in history mode: the branch's commits
// (base..HEAD), newest first, as a scrollable selectable list.
func (m model) commitListView(width int) string {
	rows := m.listViewportHeight()
	var b strings.Builder

	b.WriteString(m.paneHeading(fmt.Sprintf("History (%d)", len(m.commits)), focusFiles))
	b.WriteString("\n")

	total := m.logRowCount()
	if total == 0 {
		b.WriteString(mutedStyle.Render("  no commits on this branch"))
		return lipgloss.NewStyle().Width(width).Render(b.String())
	}

	// Scroll the list to keep the cursor visible (mirrors listView). The cursor
	// and offset index the displayed list, which includes the optional leading
	// working-tree row.
	offset := 0
	if m.commitCursor >= rows {
		offset = m.commitCursor - rows + 1
	}
	end := offset + rows
	if end > total {
		end = total
	}
	for i := offset; i < end; i++ {
		sel := i == m.commitCursor
		if m.logWorking && i == 0 {
			b.WriteString(m.workingRow(sel, width))
		} else {
			ci := i
			if m.logWorking {
				ci--
			}
			b.WriteString(m.commitRow(m.commits[ci], sel, width))
		}
		b.WriteString("\n")
	}
	return lipgloss.NewStyle().Width(width).Render(b.String())
}

// workingRow renders the synthetic "working tree" entry pinned atop the history
// list: a hollow node glyph, a "local" tag in the SHA column, and a summary of
// the uncommitted changes. It mirrors commitRow's layout so the two read as one
// list, with a green glyph to flag it as the live working state.
func (m model) workingRow(selected bool, width int) string {
	glyph := "○"
	n := len(m.files)
	noun := "files"
	if n == 1 {
		noun = "file"
	}
	tag := "local"
	subj := fmt.Sprintf("uncommitted changes · %d %s", n, noun)

	head := glyph + " " + tag + " "
	avail := width - lipgloss.Width(head)
	if avail < 3 {
		avail = 3
	}
	subj = truncateText(subj, avail)

	if selected {
		return selectedRowStyle.Width(width).Render(head + subj)
	}
	return addedStyle.Render(glyph) + " " + metaStyle.Render(tag) + " " + subj
}

// commitRow renders one commit line: a node glyph (● commit, ◆ merge), the short
// SHA, and the subject. The selected row is one continuous highlight bar; an
// unselected row colorizes the glyph and SHA. Stacked glyphs form the rail.
func (m model) commitRow(c git.Commit, selected bool, width int) string {
	glyph := "●"
	if c.IsMerge() {
		glyph = "◆"
	}
	head := glyph + " " + c.Short + " "
	avail := width - lipgloss.Width(head)
	if avail < 3 {
		avail = 3
	}
	subj := truncateText(c.Subject, avail)

	if selected {
		return selectedRowStyle.Width(width).Render(head + subj)
	}

	glyphStyle := titleStyle
	if c.IsMerge() {
		glyphStyle = lipgloss.NewStyle().Foreground(colWarn).Bold(true)
	}
	return glyphStyle.Render(glyph) + " " + metaStyle.Render(c.Short) + " " + subj
}

// treeRow renders one line of the file tree — a folder ("▾ internal/tui  +42 -7")
// or a file ("M view.go  +12 -3") — with guide rails for the ancestor levels, a
// status/chevron glyph, the segment name, and stats flush-right. We lay it out in
// plain text first so the columns align exactly, then colorize per segment; a
// selected row drops the per-segment colors for one continuous highlight bar.
func (m model) treeRow(r treeRow, selected bool, width int) string {
	prefixW := 2 * len(r.guides)

	glyph := statusGlyph(r.status)
	if r.isDir {
		glyph = "▸"
		if !r.collapsed {
			glyph = "▾"
		}
	}

	// Folder names carry a trailing slash so a name collision with a file reads
	// unambiguously.
	label := r.name
	if r.isDir {
		label += "/"
	}

	statsPlain := ""
	switch {
	case r.isDir:
		if r.added != 0 || r.deleted != 0 {
			statsPlain = fmt.Sprintf("+%d -%d", r.added, r.deleted)
		}
	case r.binary:
		statsPlain = "bin"
	default:
		statsPlain = fmt.Sprintf("+%d -%d", r.added, r.deleted)
	}

	// Layout budget: prefix + glyph(1) + space(1) + name + gap + stats.
	statsW := lipgloss.Width(statsPlain)
	avail := width - prefixW - 2 - statsW - 1
	if avail < 3 {
		avail = 3
	}
	name := truncateText(label, avail)

	gap := width - prefixW - 2 - lipgloss.Width(name) - statsW
	if gap < 1 {
		gap = 1
	}

	if selected {
		row := treeGuidesPlain(r.guides) + glyph + " " + name + strings.Repeat(" ", gap) + statsPlain
		return selectedRowStyle.Width(width).Render(row)
	}

	nameStyled := name
	glyphStyled := statusStyle(r.status).Render(glyph)
	if r.isDir {
		nameStyled = dirStyle.Render(name)
		glyphStyled = dirStyle.Render(glyph)
	}
	// A file a sync changed (and the reader hasn't opened) gently breathes its
	// status glyph through the pulse ramp — a quiet dim→bright→dim ease, not a
	// blink. The name is left alone so the cue stays subtle; the pulse stops the
	// moment its diff is opened (loadDiff clears the flag).
	if !r.isDir && m.unseen[r.path] {
		glyphStyled = lipgloss.NewStyle().Foreground(pulseShade(m.animFrame)).Bold(true).Render(glyph)
	}

	stats := mutedStyle.Render(statsPlain)
	if !r.isDir && !r.binary && statsPlain != "" {
		stats = addedStyle.Render(fmt.Sprintf("+%d", r.added)) + " " +
			removedStyle.Render(fmt.Sprintf("-%d", r.deleted))
	}
	return treeGuides(r.guides) + glyphStyled + " " +
		nameStyled + strings.Repeat(" ", gap) + stats
}

// treeGuides draws the ancestor rails: a faint "│ " where a sibling still follows
// at that level, blank where the branch has ended.
func treeGuides(guides []bool) string {
	var b strings.Builder
	for _, hasNext := range guides {
		if hasNext {
			b.WriteString(treeGuideStyle.Render("│ "))
		} else {
			b.WriteString("  ")
		}
	}
	return b.String()
}

// treeGuidesPlain is treeGuides without styling, for the selection bar (which
// owns the whole row's color).
func treeGuidesPlain(guides []bool) string {
	var b strings.Builder
	for _, hasNext := range guides {
		if hasNext {
			b.WriteString("│ ")
		} else {
			b.WriteString("  ")
		}
	}
	return b.String()
}

// pulseShade picks the breathing color for an unopened changed file's glyph,
// walking the pulse ramp up and back down as a triangle wave so it eases dim→
// bright→dim. It steps every other frame (~3.3fps off the nyan tick) for a calm
// ~2.4s breath rather than a flicker.
func pulseShade(frame int) color.Color {
	n := len(pulseRamp)
	if n < 2 {
		return colMeta
	}
	period := 2 * (n - 1)
	p := (frame / 2) % period
	if p >= n {
		p = period - p // fold the back half into a triangle
	}
	return pulseRamp[p]
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
	f := m.selectedFile()

	// Heading + body depend on what's under the cursor: a file shows its diff, a
	// folder shows a roll-up, and an empty tree shows a hint.
	if f != nil {
		title := truncatePath(f.Path, width-2-lipgloss.Width(m.splitTag())) + m.splitTag()
		return m.diffPane(width, title, m.diffWindow(width, rows))
	}
	title, body := m.noFileSelection(width)
	return m.diffPane(width, title, []string{body})
}

// commitDiffView renders the right pane in history mode: the highlighted
// commit's combined diff, headed by its short SHA and subject.
func (m model) commitDiffView(width int) string {
	rows := m.diffViewportHeight()
	if m.onWorkingRow() {
		title := truncateText("working tree · uncommitted changes", width-2-lipgloss.Width(m.splitTag())) + m.splitTag()
		return m.diffPane(width, title, m.diffWindow(width, rows))
	}
	c := m.selectedCommit()
	if c == nil {
		return m.diffPane(width, "commit", []string{mutedStyle.Render("  No commit selected.")})
	}
	title := truncateText(c.Short+"  "+c.Subject, width-2-lipgloss.Width(m.splitTag())) + m.splitTag()
	return m.diffPane(width, title, m.diffWindow(width, rows))
}

// splitTag is the " [split]" badge appended to a diff-pane title in split mode.
func (m model) splitTag() string {
	if m.splitView {
		return " [split]"
	}
	return ""
}

// diffPane lays out the right pane: a focus-aware heading, the body lines, blank
// padding so the pane fills its height, and the nyan progress bar pinned to the
// bottom. Shared by the file diff and the commit diff.
func (m model) diffPane(width int, title string, lines []string) string {
	rows := m.diffViewportHeight()
	var b strings.Builder
	b.WriteString(m.paneHeading(title, focusDiff))
	b.WriteString("\n")
	for _, l := range lines {
		b.WriteString(l)
		b.WriteString("\n")
	}
	for shown := len(lines); shown < rows; shown++ {
		b.WriteString("\n")
	}
	b.WriteString(m.nyanProgress(width))
	return lipgloss.NewStyle().Width(width).Render(b.String())
}

// noFileSelection produces the diff-pane heading and body for when the cursor is
// on a folder (a roll-up summary) or on nothing (an empty-tree hint).
func (m model) noFileSelection(width int) (title, body string) {
	r := m.selectedRow()
	if r != nil && r.isDir {
		title = truncatePath(r.path+"/", width-2)
		return title, mutedStyle.Render(fmt.Sprintf("  folder — +%d -%d across its files", r.added, r.deleted))
	}
	return "diff", mutedStyle.Render("  Select a file to view its diff.")
}

// diffWindow renders the visible slice of diff rows for the current view mode.
func (m model) diffWindow(width, rows int) []string {
	var out []string
	cursor := m.diffCursor
	if m.focus != focusDiff {
		cursor = -1 // only mark the cursor row while the diff pane has focus
	}
	if m.splitView {
		leftW := (width - 1) / 2
		rightW := width - 1 - leftW
		end := min(m.diffOffset+rows, len(m.splitRows))
		for i := m.diffOffset; i < end; i++ {
			out = append(out, m.renderSplitRow(m.splitRows[i], leftW, rightW, i == cursor))
		}
		return out
	}
	end := min(m.diffOffset+rows, len(m.viewLines))
	for i := m.diffOffset; i < end; i++ {
		out = append(out, m.renderUnifiedLine(m.viewLines[i], width, i == cursor))
	}
	return out
}

// expandLabel is the text shown on an expand affordance row.
func expandLabel(l diff.Line) string {
	switch l.Dir {
	case diff.ExpandUp:
		return fmt.Sprintf("  ↑ expand (%d hidden)", l.Hidden)
	case diff.ExpandDown:
		return fmt.Sprintf("  ↓ expand (%d hidden)", l.Hidden)
	default:
		return fmt.Sprintf("  ↕ expand %d hidden lines", l.Hidden)
	}
}

// renderUnifiedLine renders one inline-diff row: "old new ± code" with the whole
// row tinted green (add) or red (del), GitHub-style. sel marks the cursor row,
// which is painted with the selection background instead of its kind tint.
func (m model) renderUnifiedLine(l diff.Line, width int, sel bool) string {
	switch l.Kind {
	case diff.Hunk:
		return fullRowStyle(hunkLineStyle, sel).Width(width).Render(truncateText(l.Text, width))
	case diff.Meta:
		return fullRowStyle(metaLineStyle, sel).Width(width).Render(truncateText(l.Text, width))
	case diff.Expand:
		return fullRowStyle(expandLineStyle, sel).Width(width).Render(truncateText(expandLabel(l), width))
	}

	numStyle, bg, marker := lineStyles(l.Kind)
	if sel {
		numStyle, bg = selectedRowStyle, colRowBg
	}
	d := m.lineDigits
	gut := numStyle.Render(numField(l.OldNum, d) + " " + numField(l.NewNum, d) + " " + marker)
	avail := width - lipgloss.Width(gut)
	if avail < 1 {
		avail = 1
	}
	return gut + m.renderCode(l.Text, m.lineLexer(l), avail, bg)
}

// fullRowStyle returns the style for a full-width row, swapping in the selection
// background when the row is under the cursor.
func fullRowStyle(base lipgloss.Style, sel bool) lipgloss.Style {
	if sel {
		return base.Background(colRowBg)
	}
	return base
}

// renderSplitRow renders one side-by-side row: old/del on the left, new/add on
// the right, divided by a vertical rule. Hunk/Meta/Expand rows span the full
// width. sel marks the cursor row.
func (m model) renderSplitRow(r diff.Row, leftW, rightW int, sel bool) string {
	if r.Full != nil {
		w := leftW + 1 + rightW
		switch r.Full.Kind {
		case diff.Hunk:
			return fullRowStyle(hunkLineStyle, sel).Width(w).Render(truncateText(r.Full.Text, w))
		case diff.Expand:
			return fullRowStyle(expandLineStyle, sel).Width(w).Render(truncateText(expandLabel(*r.Full), w))
		default:
			return fullRowStyle(metaLineStyle, sel).Width(w).Render(truncateText(r.Full.Text, w))
		}
	}
	left := m.renderSplitSide(r.Left, leftW, false, sel)
	right := m.renderSplitSide(r.Right, rightW, true, sel)
	return left + borderStyle.Render("│") + right
}

// renderSplitSide renders one half of a split row. A nil line is an empty paired
// slot, filled with a faint background so the gap reads as intentional.
func (m model) renderSplitSide(l *diff.Line, width int, newSide, sel bool) string {
	if width < 1 {
		return ""
	}
	if l == nil {
		if sel {
			return selectedRowStyle.Width(width).Render("")
		}
		return fillerStyle.Width(width).Render("")
	}
	numStyle, bg, _ := lineStyles(l.Kind)
	if sel {
		numStyle, bg = selectedRowStyle, colRowBg
	}
	num := l.OldNum
	if newSide {
		num = l.NewNum
	}
	gut := numStyle.Render(numField(num, m.lineDigits) + " ")
	avail := width - lipgloss.Width(gut)
	if avail < 1 {
		avail = 1
	}
	return gut + m.renderCode(l.Text, m.lineLexer(*l), avail, bg)
}

// lineStyles returns the gutter style, the code-body background tint (nil for
// context), and the marker for a line kind.
func lineStyles(kind diff.Kind) (num lipgloss.Style, bg color.Color, marker string) {
	switch kind {
	case diff.Add:
		return addNumStyle, diffAddBg, "+"
	case diff.Del:
		return delNumStyle, diffDelBg, "-"
	default:
		return ctxNumStyle, nil, " "
	}
}

// renderCode renders a line of code into exactly width columns: a leading gutter
// space, the syntax-highlighted tokens (lexed with lexer), an ellipsis if it
// overflows, then padding — all sharing bg so the diff row tint reads as one
// continuous band beneath the colored tokens.
func (m model) renderCode(text string, lexer chroma.Lexer, width int, bg color.Color) string {
	if width <= 0 {
		return ""
	}
	base := lipgloss.NewStyle()
	if bg != nil {
		base = base.Background(bg)
	}

	var b strings.Builder
	used := 0
	b.WriteString(base.Render(" ")) // left padding, matching the gutter space
	used++

	for _, sp := range m.highlight(lexer, expandTabs(text)) {
		if used >= width {
			break
		}
		st := base
		if sp.fg != nil {
			st = st.Foreground(sp.fg)
		}
		remaining := width - used
		if lipgloss.Width(sp.text) <= remaining {
			b.WriteString(st.Render(sp.text))
			used += lipgloss.Width(sp.text)
			continue
		}
		// Doesn't fit: cut leaving one column for the ellipsis, then stop.
		seg := cutToWidth(sp.text, remaining-1)
		b.WriteString(st.Render(seg))
		b.WriteString(base.Render("…"))
		used += lipgloss.Width(seg) + 1
		break
	}
	if used < width {
		b.WriteString(base.Render(strings.Repeat(" ", width-used)))
	}
	return b.String()
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

	// Perceptually-even rainbow: constant OKLCH L=0.72 C=0.155, hue rotating, so
	// every band has identical perceived brightness (raw ANSI indices banded
	// unevenly and went dull on muted terminal themes). Degrades to the nearest
	// ANSI on 16-color terminals.
	trail := []color.Color{
		lipgloss.Color("#f67972"), lipgloss.Color("#e19005"),
		lipgloss.Color("#5ebd64"), lipgloss.Color("#00c1c2"),
		lipgloss.Color("#69a3ff"), lipgloss.Color("#d480da"),
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
	b.WriteString(catStyle.Render(cat)) // brand-magenta cat (kept off the selection blue)
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
	var keys []string
	switch m.mode {
	case viewLog:
		keys = []string{
			"j/k select", "h/l ⇄ pane", "↵ open", "gg/G top/bot", "s split", "t theme", "L/esc back", "? help", "q quit",
		}
	case viewCommit:
		keys = []string{
			"j/k move", "h/l ⇄ pane", "↵ open/fold", "gg/G top/bot", "s split", "t theme", "esc back", "? help", "q quit",
		}
	default:
		keys = []string{
			"j/k move", "h/l ⇄ pane", "↵ open/fold/expand", "gg/G top/bot", "C-d/C-u half", "s split", "t theme", "L history", "? help", "q quit",
		}
	}
	return mutedStyle.Render(strings.Join(keys, "  ·  "))
}

func (m model) helpView() string {
	lines := []string{
		titleStyle.Render("diffcat — vim keybindings"),
		"",
		headingStyle.Render("  panes"),
		"  h / l        focus file list / diff pane",
		"  Tab          toggle focused pane",
		"  Enter / o    open file's diff / fold a folder / expand context",
		"",
		headingStyle.Render("  motions (act on the focused pane)"),
		"  j / k        move cursor down / up one line",
		"  gg / G       jump to top / bottom",
		"  ctrl+d / u   half page down / up",
		"  ctrl+f / b   full page down / up",
		"",
		headingStyle.Render("  diff"),
		"  ↵ / o on  ↕  expand hidden context (↓ below, ↑ above)",
		"",
		headingStyle.Render("  history"),
		"  L            toggle the commit-history view",
		"  j / k        (in history) move between entries, preview each diff",
		"  Enter        (in history) open the commit's (or working tree's) files",
		"  ○ local      (in history) the working tree: staged + unstaged changes",
		"  Esc          step back: commit tree → history → branch diff",
		"",
		headingStyle.Render("  view"),
		"  s            toggle unified / side-by-side",
		"  t            toggle light / dark theme",
		"  r            refresh from disk (also auto-syncs in the background)",
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
