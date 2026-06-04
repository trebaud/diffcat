package tui

import (
	"fmt"
	"image/color"
	"math"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	uv "github.com/charmbracelet/ultraviolet"

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

	// The overview dashboard is a single full-width pane, not the two-pane split.
	if m.mode == viewOverview {
		screen := m.overviewView()
		if m.showHelp {
			return m.floatOverlay(screen, m.helpBox())
		}
		return screen
	}

	// The sidebar width is driven by the `[`/`]` collapse-expand state. When it's
	// 0 the sidebar is collapsed and the diff fills the whole width; otherwise the
	// diff takes the remainder past the list and the one-column divider.
	listWidth := m.sidebarWidth()
	diffWidth := m.width
	if listWidth > 0 {
		diffWidth = m.width - listWidth - 1
	}

	// The body fills everything between the one-line header and footer. We pin
	// both panes and the divider to that exact height so the TUI occupies the
	// whole terminal — empty regions are still part of a full-height pane, not
	// dead space the layout leaves behind.
	bodyHeight := m.height - 2
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	fill := lipgloss.NewStyle().Height(bodyHeight).MaxHeight(bodyHeight)

	var body string
	switch {
	case m.mode == viewLog && !m.logDiffOpen:
		// History view, diff pane closed (the default): the commit list fills the
		// whole body. `l`/Tab/→ opens the diff on the right half; Esc closes it.
		body = fill.Render(m.commitListView(m.width))
	case listWidth == 0:
		// Sidebar collapsed: the diff pane is the entire body.
		if m.mode == viewLog {
			body = fill.Render(m.commitDiffView(diffWidth))
		} else {
			body = fill.Render(m.diffView(diffWidth))
		}
	default:
		var left, right string
		if m.mode == viewLog {
			left = fill.Render(m.commitListView(listWidth))
			right = fill.Render(m.commitDiffView(diffWidth))
		} else {
			left = fill.Render(m.listView(listWidth))
			right = fill.Render(m.diffView(diffWidth))
		}
		body = lipgloss.JoinHorizontal(lipgloss.Top, left, m.divider(bodyHeight), right)
	}

	// The chrome rows span the full width: truncate (never wrap) then pad.
	header := padLine(m.headerView(), m.width)
	footer := padLine(m.footerView(), m.width)
	screen := strings.Join([]string{header, body, footer}, "\n")

	// Overlays float above the dimmed screen rather than replacing it, so the
	// reader keeps their place in the background.
	if m.showHelp {
		return m.floatOverlay(screen, m.helpBox())
	}
	if m.showCommitDetails {
		return m.floatOverlay(screen, m.commitDetailsBox())
	}
	if m.fileFindActive {
		return m.floatOverlay(screen, m.fileFindBox())
	}
	return screen
}

// floatOverlay composites box as a floating window centered over screen: the
// background is dimmed to a subtle scrim, then the (solid) box is drawn on top.
func (m model) floatOverlay(screen, box string) string {
	canvas := lipgloss.NewCanvas(m.width, m.height)
	canvas.Compose(lipgloss.NewLayer(screen))
	dimCanvas(canvas)

	boxW, boxH := lipgloss.Width(box), lipgloss.Height(box)
	bx := max((m.width-boxW)/2, 0)
	by := max((m.height-boxH)/2, 0)
	uv.NewStyledString(box).Draw(canvas, uv.Rect(bx, by, boxW, boxH))
	return canvas.Render()
}

// dimCanvas blends every cell toward the theme's canvas tone, fading the
// background into a soft scrim behind a floating window. Foreground text is
// pulled most of the way toward the scrim (so it recedes yet stays faintly
// legible); backgrounds are nudged less so colored bands just lose their punch.
func dimCanvas(canvas *lipgloss.Canvas) {
	dark := colCanvas == nil
	scrim := color.RGBA{R: 0x0d, G: 0x11, B: 0x17, A: 0xff} // github dark canvas
	defaultFg := color.RGBA{R: 0xad, G: 0xbc, B: 0xc7, A: 0xff}
	if !dark {
		scrim = color.RGBA{R: 0xff, G: 0xff, B: 0xff, A: 0xff}
		defaultFg = color.RGBA{R: 0x1f, G: 0x23, B: 0x28, A: 0xff}
	}
	w, h := canvas.Width(), canvas.Height()
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			cell := canvas.CellAt(x, y)
			if cell == nil {
				continue
			}
			fg := cell.Style.Fg
			if fg == nil {
				fg = defaultFg
			}
			cell.Style.Fg = blendColor(fg, scrim, 0.62)
			if cell.Style.Bg != nil {
				cell.Style.Bg = blendColor(cell.Style.Bg, scrim, 0.45)
			}
		}
	}
}

// blendColor linearly interpolates from a toward b by t (0 = a, 1 = b).
func blendColor(a, b color.Color, t float64) color.Color {
	ar, ag, ab, _ := a.RGBA()
	br, bg, bb, _ := b.RGBA()
	mix := func(x, y uint32) uint8 {
		// RGBA() returns 16-bit channels; /257 maps 0..65535 → 0..255.
		return uint8((float64(x)*(1-t)+float64(y)*t)/257 + 0.5)
	}
	return color.RGBA{R: mix(ar, br), G: mix(ag, bg), B: mix(ab, bb), A: 0xff}
}

func (m model) headerView() string {
	left := titleStyle.Render("diffcat")
	if m.mode == viewLog {
		mid := mutedStyle.Render(fmt.Sprintf("  %s · history", branchLabel(m.branch)))
		count := "  " + headingStyle.Render(fmt.Sprintf("%d commits", m.featureCommitCount()))
		return left + mid + count
	}
	if m.mode == viewOverview {
		if c := m.overviewCommit; c != nil {
			mid := mutedStyle.Render(fmt.Sprintf("  %s · overview", c.Short))
			return left + mid + "  " + headingStyle.Render(truncateText(c.Subject, 60))
		}
		mid := mutedStyle.Render(fmt.Sprintf("  %s · overview", branchLabel(m.branch)))
		stat := ""
		if m.shortstat != "" {
			stat = "  " + headingStyle.Render(m.shortstat)
		}
		return left + mid + stat
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
// (base..HEAD), newest first, as a scrollable selectable list. When the branch
// forked from a base, the base branch's own commits follow below a labeled
// divider so the split between "this branch" and "what it branched from" is clear.
func (m model) commitListView(width int) string {
	rows := m.listViewportHeight()
	var b strings.Builder

	// The header counts the branch's own commits; the base history below the
	// divider is context, not part of "what this branch added".
	b.WriteString(m.paneHeading(fmt.Sprintf("History (%d)", m.featureCommitCount()), focusFiles))
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
			// Draw the divider just above the first base-branch commit so the
			// branch's own commits read as one group and the base as another. The
			// rail (│) carries down through the gap and tees into the divider, so
			// the line stays connected across the two groups.
			if ci == m.baseStart {
				b.WriteString(mutedStyle.Render("│"))
				b.WriteString("\n")
				b.WriteString(commitDivider(m.baseName, width))
				b.WriteString("\n")
			}
			b.WriteString(m.commitRow(m.commits[ci], sel, width))
		}
		b.WriteString("\n")
	}
	return lipgloss.NewStyle().Width(width).Render(b.String())
}

// commitDivider renders the labeled rule that separates the branch's own commits
// from the base branch's history below them, e.g. "├─ main ──────────". The rule
// is muted so it reads as structure, while the base branch name is bold so it
// stays legible against any theme.
func commitDivider(label string, width int) string {
	lead, name := "├─ ", label+" "
	fill := width - lipgloss.Width(lead) - lipgloss.Width(name)
	if fill < 0 {
		fill = 0
	}
	return mutedStyle.Render(lead) + metaStyle.Render(label) + mutedStyle.Render(" "+strings.Repeat("─", fill))
}

// workingRow renders the synthetic "working tree" entry pinned atop the history
// list: a hollow node glyph, a "local" tag in the SHA column, and a summary of
// the uncommitted changes. It mirrors commitRow's layout so the two read as one
// list, with a green glyph to flag it as the live working state.
func (m model) workingRow(selected bool, width int) string {
	glyph := "○"
	n := m.workingCount
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
	if m.sidebar == sidebarWide || m.mode == viewLog {
		return m.commitRowWide(c, glyph, selected, width)
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

// commitRowWide renders one commit line for the widened sidebar: the same node
// glyph + short SHA + subject, plus a subtle gold tag badge after the SHA and the
// author/date right-aligned. The right metadata is dropped first when a row gets
// too narrow to keep the subject legible. Like commitRow it lays out plain text
// for exact alignment, then colorizes per segment (the selected row is one bar).
func (m model) commitRowWide(c git.Commit, glyph string, selected bool, width int) string {
	head := glyph + " " + c.Short + "  "

	refsPlain, refsStyled := refBadges(c)
	refPart := ""
	if refsPlain != "" {
		refPart = refsPlain + "  "
	}

	meta := truncateText(c.Author, 16) + "  " + c.Date

	fixed := lipgloss.Width(head) + lipgloss.Width(refPart)
	subjAvail := width - fixed - lipgloss.Width(meta) - 2
	if subjAvail < 12 {
		// Too cramped for the right metadata — give the room back to the subject.
		meta = ""
		subjAvail = width - fixed - 1
	}
	if subjAvail < 3 {
		subjAvail = 3
	}
	subj := truncateText(c.Subject, subjAvail)

	gap := width - fixed - lipgloss.Width(subj) - lipgloss.Width(meta)
	if gap < 1 {
		gap = 1
	}

	if selected {
		row := head + refPart + subj + strings.Repeat(" ", gap) + meta
		return selectedRowStyle.Width(width).Render(row)
	}

	glyphStyle := titleStyle
	if c.IsMerge() {
		glyphStyle = lipgloss.NewStyle().Foreground(colWarn).Bold(true)
	}
	var b strings.Builder
	b.WriteString(glyphStyle.Render(glyph))
	b.WriteString(" ")
	b.WriteString(metaStyle.Render(c.Short))
	b.WriteString("  ")
	if refsStyled != "" {
		b.WriteString(refsStyled)
		b.WriteString("  ")
	}
	b.WriteString(subj)
	b.WriteString(strings.Repeat(" ", gap))
	if meta != "" {
		b.WriteString(mutedStyle.Render(meta))
	}
	return b.String()
}

// refBadges builds a commit's ref decoration the way tig and `git log --decorate`
// do: local branches as [main] (green, the checked-out one bold), remote-tracking
// refs as {origin/main} (red), and tags bare in gold. It returns the plain text
// (for width math) and the colorized version in lockstep, both empty when the
// commit carries no refs. The selected row uses the plain form inside its single
// highlight bar; unselected rows use the styled form.
func refBadges(c git.Commit) (plain, styled string) {
	var plainParts, styledParts []string
	for _, h := range c.Heads {
		badge := "[" + h + "]"
		plainParts = append(plainParts, badge)
		styledParts = append(styledParts, headStyle.Render(badge))
	}
	for _, r := range c.Remotes {
		badge := "{" + r + "}"
		plainParts = append(plainParts, badge)
		styledParts = append(styledParts, remoteStyle.Render(badge))
	}
	for _, t := range c.Tags {
		plainParts = append(plainParts, t)
		styledParts = append(styledParts, tagStyle.Render(t))
	}
	if len(plainParts) == 0 {
		return "", ""
	}
	return strings.Join(plainParts, " "), strings.Join(styledParts, " ")
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
	// blink. The pulse persists until its diff is opened (loadDiff clears the flag).
	// On top of that, the instant a change lands a bright highlight sweeps once
	// across the name — a clear "this just changed" cue that then clears itself,
	// leaving the quieter glyph pulse to carry on.
	if !r.isDir && m.unseen[r.path] {
		glyphStyled = lipgloss.NewStyle().Foreground(pulseShade(m.animFrame)).Bold(true).Render(glyph)
		if swept, live := shimmerName(name, m.animFrame-m.unseenAt[r.path]); live {
			nameStyled = swept
		}
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

// shimmerName paints a one-shot highlight that sweeps left→right across a freshly
// changed file's name, then leaves the text plain. age is animFrames since the
// file was flagged unseen; a soft bright band rides the pulse ramp and travels a
// little past the last rune so the tail flashes too, after which the sweep is done
// — the glyph keeps breathing, but the name rests. Only ever brightens (never
// dims below the plain text), so the cue reads as a passing glint. Returns
// (styled, true) while the sweep is live, and (_, false) once it has cleared.
func shimmerName(name string, age int) (string, bool) {
	if age < 0 {
		age = 0
	}
	runes := []rune(name)
	n := len(runes)
	if n == 0 {
		return name, false
	}
	const (
		band  = 3.0 // half-width of the bright sweep, in cells (wide enough to overlap frame-to-frame)
		speed = 1.4 // cells the head advances per animFrame (~6.7fps → a ~1.5s glint)
	)
	head := float64(age) * speed
	if head-band > float64(n-1) {
		return name, false // the band has cleared the last rune
	}
	var b strings.Builder
	for i, r := range runes {
		t := 1 - math.Abs(float64(i)-head)/band // 1 at the head, 0 at the band edge
		if t < 0.2 {
			b.WriteRune(r) // outside the band (and its faint edge) → plain, no dark dip
			continue
		}
		b.WriteString(shimmerStyle(t).Render(string(r)))
	}
	return b.String(), true
}

// shimmerStyle maps a sweep intensity (0=edge, 1=head) onto the brighter half of
// the pulse ramp so the band only lifts the name above its resting tone, bolding
// the crest for a touch more pop.
func shimmerStyle(t float64) lipgloss.Style {
	n := len(pulseRamp)
	if n == 0 {
		return lipgloss.NewStyle().Foreground(colMeta)
	}
	lo := n / 2 // never reach for the dim low stops — those would darken the name
	idx := lo + int(t*float64(n-1-lo)+0.5)
	if idx < lo {
		idx = lo
	}
	if idx >= n {
		idx = n - 1
	}
	s := lipgloss.NewStyle().Foreground(pulseRamp[idx])
	if t > 0.6 {
		s = s.Bold(true)
	}
	return s
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
	curHit := m.currentHitIndex()
	for i := m.diffOffset; i < end; i++ {
		out = append(out, m.renderUnifiedLine(m.viewLines[i], width, i == cursor, i == curHit))
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
func (m model) renderUnifiedLine(l diff.Line, width int, sel, searchCur bool) string {
	switch l.Kind {
	case diff.Hunk:
		return fullRowStyle(hunkLineStyle, sel).Width(width).Render(truncateText(l.Text, width))
	case diff.Meta:
		return fullRowStyle(metaLineStyle, sel).Width(width).Render(truncateText(l.Text, width))
	case diff.Expand:
		return fullRowStyle(expandLineStyle, sel).Width(width).Render(truncateText(expandLabel(l), width))
	}

	numStyle, bg, emphBg, marker := lineStyles(l.Kind)
	if sel {
		numStyle, bg, emphBg = selectedRowStyle, colRowBg, nil
	}
	d := m.lineDigits
	gut := numStyle.Render(numField(l.OldNum, d) + " " + numField(l.NewNum, d) + " " + marker)
	avail := width - lipgloss.Width(gut)
	if avail < 1 {
		avail = 1
	}
	// Search highlight survives the selection tint: the current match is normally
	// the cursor row, so suppressing it on selection would hide the n/N target.
	return gut + m.renderCode(l.Text, m.lineLexer(l), avail, bg, emphBg, l.Emph, m.searchRanges(l), searchCur)
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
	numStyle, bg, emphBg, _ := lineStyles(l.Kind)
	if sel {
		numStyle, bg, emphBg = selectedRowStyle, colRowBg, nil
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
	return gut + m.renderCode(l.Text, m.lineLexer(*l), avail, bg, emphBg, l.Emph, m.searchRanges(*l), l == m.currentHitLine())
}

// lineStyles returns the gutter style, the code-body background tint (nil for
// context), the stronger word-level emphasis tint (nil for context), and the
// marker for a line kind.
func lineStyles(kind diff.Kind) (num lipgloss.Style, bg, emphBg color.Color, marker string) {
	switch kind {
	case diff.Add:
		return addNumStyle, diffAddBg, diffAddEmphBg, "+"
	case diff.Del:
		return delNumStyle, diffDelBg, diffDelEmphBg, "-"
	default:
		return ctxNumStyle, nil, nil, " "
	}
}

// renderCode renders a line of code into exactly width columns: a leading gutter
// space, the syntax-highlighted tokens (lexed with lexer), an ellipsis if it
// overflows, then padding — all sharing bg so the diff row tint reads as one
// continuous band beneath the colored tokens. The rune ranges in emph (over the
// raw text) are painted with emphBg instead — the stronger word-level tint that
// marks exactly what changed within the line.
func (m model) renderCode(text string, lexer chroma.Lexer, width int, bg, emphBg color.Color, emph, search [][2]int, searchCur bool) string {
	if width <= 0 {
		return ""
	}
	base := lipgloss.NewStyle()
	if bg != nil {
		base = base.Background(bg)
	}
	emphStyle := base
	if emphBg != nil {
		emphStyle = lipgloss.NewStyle().Background(emphBg)
	}
	// A search hit overrides both the body and word-emphasis tints; the current
	// match (n/N target) reads brighter than the rest.
	searchHitBg := colSearchBg
	if searchCur {
		searchHitBg = colSearchCurBg
	}
	searchStyle := lipgloss.NewStyle().Background(searchHitBg)

	expanded := expandTabs(text)
	var mask, searchMask []bool
	if len(emph) > 0 && emphBg != nil {
		mask = expandedMask(text, emph)
	}
	if len(search) > 0 {
		searchMask = expandedMask(text, search)
	}

	var b strings.Builder
	used := 0
	b.WriteString(base.Render(" ")) // left padding, matching the gutter space
	used++

	ri := 0 // rune offset into the expanded text, to index the masks
	for _, sp := range m.highlight(lexer, expanded) {
		runes := []rune(sp.text)
		k := 0
		for k < len(runes) {
			if used >= width {
				break
			}
			// Group the longest run of runes that share both an emphasis and a
			// search state so each span is rendered with a single background.
			em := maskAt(mask, ri+k)
			hit := maskAt(searchMask, ri+k)
			j := k + 1
			for j < len(runes) && maskAt(mask, ri+j) == em && maskAt(searchMask, ri+j) == hit {
				j++
			}
			st := base
			if em {
				st = emphStyle
			}
			if hit {
				st = searchStyle
			}
			if sp.fg != nil {
				st = st.Foreground(sp.fg)
			}
			seg := string(runes[k:j])
			segW := lipgloss.Width(seg)
			remaining := width - used
			if segW <= remaining {
				b.WriteString(st.Render(seg))
				used += segW
				k = j
				continue
			}
			// Doesn't fit: cut leaving one column for the ellipsis, then stop.
			cut := cutToWidth(seg, remaining-1)
			b.WriteString(st.Render(cut))
			b.WriteString(base.Render("…"))
			used += lipgloss.Width(cut) + 1
			ri = -1 // sentinel: done
			break
		}
		if ri < 0 || used >= width {
			break
		}
		ri += len(runes)
	}
	if used < width {
		b.WriteString(base.Render(strings.Repeat(" ", width-used)))
	}
	return b.String()
}

// maskAt reads an emphasis mask defensively — out-of-range indices (no mask, or
// width math that drifted past it) read as un-emphasized.
func maskAt(mask []bool, i int) bool {
	return i >= 0 && i < len(mask) && mask[i]
}

// expandedMask projects per-rune emphasis ranges over the raw text onto the
// tab-expanded text renderCode actually paints, replicating each tab's flag
// across the spaces it expands to so the mask stays aligned with the rendered
// runes. It mirrors expandTabs's column math exactly.
func expandedMask(text string, emph [][2]int) []bool {
	runes := []rune(text)
	raw := make([]bool, len(runes))
	for _, r := range emph {
		for i := r[0]; i < r[1] && i < len(raw); i++ {
			if i >= 0 {
				raw[i] = true
			}
		}
	}
	var mask []bool
	col := 0
	for i, r := range runes {
		if r == '\t' {
			n := tabWidth - col%tabWidth
			for k := 0; k < n; k++ {
				mask = append(mask, raw[i])
			}
			col += n
			continue
		}
		mask = append(mask, raw[i])
		col++
	}
	return mask
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

// nyanProgress renders the diff-reading position as a nyan cat marching from the
// top of the file (left edge) to the end (right edge), trailing a rainbow. The
// cat tracks the cursor line, not the scroll offset, so it advances as you read
// through the diff even when the whole file fits on screen. It only appears once
// the diff pane is focused — there's no reading position to show otherwise.
func (m model) nyanProgress(width int) string {
	if m.focus != focusDiff {
		return strings.Repeat(" ", max(0, width))
	}
	return nyanBar(width, m.nyanPos, m.animFrame, m.nyanSettle)
}

// targetFrac is the cat's destination: the cursor's position through the diff,
// in [0,1]. The rendered position (m.nyanPos) springs toward this each tick.
func (m model) targetFrac() float64 {
	if last := m.totalDiffRows() - 1; last > 0 {
		return float64(m.diffCursor) / float64(last)
	}
	return 0
}

// nyanMoving reports whether the position spring still has meaningful distance or
// velocity left — i.e. whether the fast tick rate is worth paying for.
func (m model) nyanMoving(target float64) bool {
	d := target - m.nyanPos
	return d > 1e-3 || d < -1e-3 || m.nyanVel > 1e-3 || m.nyanVel < -1e-3
}

func nyanBar(width int, frac float64, frame, settle int) string {
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
	// Arrival blink: on settling after a glide, the eyes pop wide for a beat, then
	// hold a content squint (the main, clearly-visible beat) before the gait
	// resumes. All faces are 5 wide so the layout never shifts. (settle counts down
	// from settleFrames ≈ 800ms: ~265ms wide, then ~530ms squint.)
	switch {
	case settle > 16:
		cat = "=^o^=" // just arrived — eyes wide
	case settle > 0:
		cat = "=^_^=" // happy squint, held
	}
	catW := lipgloss.Width(cat)

	maxX := width - catW
	if maxX < 1 {
		maxX = 1
	}
	// The spring-eased frac (m.nyanPos) lands the cat on the nearest whole cell —
	// the thin ━ trail can only grow in whole cells, so the smoothness comes from
	// the spring gliding the cat cell-by-cell on jumps, not from sub-cell rendering.
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

// onBaseBranch reports whether the current branch is the base it would be
// diffed against. There the branch diff is degenerate — the merge base
// collapses to HEAD, so "D" would only ever show working-tree-vs-HEAD (already
// reachable as the history's ○ local entry). Most commonly: sitting on
// main/master. When true the "D" affordance is hidden.
func (m model) onBaseBranch() bool {
	return m.branch != "" && m.branch == m.baseName
}

// branchDiffLabel names the D shortcut after the actual base branch, e.g.
// "diff vs main", falling back to a generic label when the base is unnamed.
func (m model) branchDiffLabel() string {
	if m.baseName == "" {
		return "branch diff"
	}
	return "diff vs " + m.baseName
}

func (m model) footerView() string {
	// While typing a query the footer becomes the search prompt.
	if m.searchActive {
		return selectedStyle.Render("/") + m.searchInput + selectedStyle.Render("▌")
	}
	var keys []string
	switch m.mode {
	case viewOverview:
		keys = []string{
			"j/k move", "↵ open file", "S/esc back", "t theme", "? help", "q quit",
		}
		return mutedStyle.Render(strings.Join(keys, "  ·  "))
	case viewLog:
		if m.logDiffOpen {
			keys = []string{"j/k select", "h/l ⇄ pane", "Tab hide diff", "↵ open commit"}
		} else {
			keys = []string{"j/k select", "Tab diff", "↵ open commit"}
		}
		if !m.onBaseBranch() {
			keys = append(keys, "D "+m.branchDiffLabel())
		}
		keys = append(keys, "d details", "S summary", "t theme", "? help", "q quit")
	case viewCommit:
		keys = []string{
			"j/k move", "h/l ⇄ pane", "↵ open", "f find", "/ search", "d details", "S summary", "[ ] sidebar", "s split", "t theme", "esc back", "? help", "q quit",
		}
	default:
		keys = []string{
			"j/k move", "h/l ⇄ pane", "↵ open/expand", "f find", "/ search", "[ ] sidebar", "S overview", "s split", "t theme", "L history", "? help", "q quit",
		}
	}
	footer := mutedStyle.Render(strings.Join(keys, "  ·  "))
	// A committed query shows its match position (or "no matches") ahead of the
	// keybindings so the reader knows where n/N will land.
	if m.searchQuery != "" {
		status := "/" + m.searchQuery + " "
		if len(m.searchHits) == 0 {
			status += "no matches"
		} else {
			status += fmt.Sprintf("%d/%d", m.searchIdx+1, len(m.searchHits))
		}
		footer = selectedStyle.Render(status) + mutedStyle.Render("  ·  ") + footer
	}
	return footer
}

func (m model) helpBox() string {
	lines := []string{
		titleStyle.Render("diffcat — vim keybindings"),
		"",
		headingStyle.Render("  panes"),
		"  h / l        focus file list / diff pane",
		"  Tab          toggle focused pane",
		"  Enter / o    open file's diff / expand context",
		"  f            fuzzy-jump to a changed file by name",
		"  [ / ]        collapse / widen the sidebar (full-width diff ↔ wide list)",
		"",
		headingStyle.Render("  motions (act on the focused pane)"),
		"  j / k        move cursor down / up one line",
		"  gg / G       jump to top / bottom",
		"  ctrl+d / u   half page down / up",
		"  ctrl+f / b   full page down / up",
		"",
		headingStyle.Render("  diff"),
		"  ↵ / o on  ↕  expand hidden context (↓ below, ↑ above)",
		"  /            search the open diff (n / N jump between matches)",
		"",
		headingStyle.Render("  history (the default view)"),
		"  j / k        move between entries (the list fills the screen)",
		"  Tab          toggle the selected diff on the right half on / off",
		"  Enter        open the commit's (or working tree's) files",
		"  d            inspect the selected commit (author, date, full message)",
		"  ○ local      the working tree: staged + unstaged changes",
		"  L            return to the history view from anywhere",
		"  Esc          close the diff pane, then step back (commit tree → history)",
		"",
		headingStyle.Render("  branch diff"),
	}
	// On the base branch the branch diff is degenerate, so "D" is hidden in the
	// footer; mirror that here rather than advertise a key that shows nothing.
	if !m.onBaseBranch() {
		label := m.branchDiffLabel()
		lines = append(lines, "  D            aggregated "+label+" (file tree + diff)")
	}
	lines = append(lines,
		"  S            toggle the overview (churn bars + languages) — branch-wide,",
		"               or scoped to the selected commit from the history",
		"",
		headingStyle.Render("  view"),
		"  s            toggle unified / side-by-side",
		"  t            toggle light / dark theme",
		"  r            refresh from disk (also auto-syncs in the background)",
		"  ? / q        toggle help / quit",
		"",
		mutedStyle.Render("  press any key to dismiss"),
	)
	return floatingBox(strings.Join(lines, "\n"))
}

// floatingBox wraps content in the shared floating-window chrome: a rounded
// border over a solid, slightly elevated panel background so it reads as a
// window sitting above the dimmed scrim.
func floatingBox(content string) string {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colBorder).
		BorderBackground(colOverlayBg).
		Background(colOverlayBg).
		Padding(1, 2).
		Render(content)
}

// detailsContentWidth is the column budget for the details modal's inner text:
// it stays comfortably readable but never overflows narrow terminals (the box
// adds a 1-col border + 2-col padding on each side, hence the -6).
func (m model) detailsContentWidth() int {
	w := m.width - 6
	if w > 72 {
		w = 72
	}
	if w < 1 {
		w = 1
	}
	return w
}

// detailsContent assembles the full set of inner lines for the commit-details
// modal (header, message, dismiss hint). The body is word-wrapped to the content
// width so it never wraps past the terminal edge.
func (m model) detailsContent() []string {
	c := m.detailsCommit()
	if c == nil {
		return nil
	}
	w := m.detailsContentWidth()

	author := c.Author
	if c.AuthorEmail != "" {
		author += " <" + c.AuthorEmail + ">"
	}
	lines := []string{
		titleStyle.Render(truncateText("commit "+c.SHA, w)),
		"",
		mutedStyle.Render("Author  ") + truncateText(author, w-8),
		mutedStyle.Render("Date    ") + truncateText(c.Date, w-8),
	}
	if len(c.Heads) > 0 {
		lines = append(lines, mutedStyle.Render("Branch  ")+headStyle.Render(truncateText(strings.Join(c.Heads, ", "), w-8)))
	}
	if len(c.Remotes) > 0 {
		lines = append(lines, mutedStyle.Render("Remote  ")+remoteStyle.Render(truncateText(strings.Join(c.Remotes, ", "), w-8)))
	}
	if len(c.Tags) > 0 {
		lines = append(lines, mutedStyle.Render("Tags    ")+tagStyle.Render(truncateText(strings.Join(c.Tags, ", "), w-8)))
	}
	if len(c.Parents) > 0 {
		parents := strings.Join(c.Parents, "  ")
		if c.IsMerge() {
			parents += "  (merge)"
		}
		lines = append(lines, mutedStyle.Render("Parents ")+truncateText(parents, w-8))
	}
	lines = append(lines, "")
	for _, sl := range wrapText(c.Subject, w) {
		lines = append(lines, headingStyle.Render(sl))
	}
	if strings.TrimSpace(c.Body) != "" {
		lines = append(lines, "")
		lines = append(lines, wrapText(c.Body, w)...)
	}
	lines = append(lines, "", mutedStyle.Render("j/k scroll · any other key to dismiss"))
	return lines
}

// detailsMaxScroll is how far the modal can scroll given its content and the
// available height — shared by the view (to window) and update (to clamp).
func (m model) detailsMaxScroll() int {
	rows := m.height - 4 // rounded border (2) + vertical padding (2)
	if rows < 1 {
		rows = 1
	}
	if over := len(m.detailsContent()) - rows; over > 0 {
		return over
	}
	return 0
}

// commitDetailsBox builds the commit-details floating window, windowed by
// detailsScroll when the message is taller than the screen.
func (m model) commitDetailsBox() string {
	content := m.detailsContent()
	rows := m.height - 4
	if rows < 1 {
		rows = 1
	}
	off := m.detailsScroll
	if max := m.detailsMaxScroll(); off > max {
		off = max
	}
	end := off + rows
	if end > len(content) {
		end = len(content)
	}
	if off > len(content) {
		off = len(content)
	}
	return floatingBox(strings.Join(content[off:end], "\n"))
}

// wrapText word-wraps s to width cols, preserving existing newlines (so blank
// lines between paragraphs survive). Words longer than width are hard-broken.
func wrapText(s string, width int) []string {
	if width < 1 {
		width = 1
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		if para == "" {
			out = append(out, "")
			continue
		}
		line := ""
		for _, word := range strings.Fields(para) {
			for lipgloss.Width(word) > width {
				// Hard-break a word too long to ever fit.
				if line != "" {
					out = append(out, line)
					line = ""
				}
				r := []rune(word)
				cut := len(r)
				for cut > 0 && lipgloss.Width(string(r[:cut])) > width {
					cut--
				}
				out = append(out, string(r[:cut]))
				word = string(r[cut:])
			}
			switch {
			case line == "":
				line = word
			case lipgloss.Width(line)+1+lipgloss.Width(word) <= width:
				line += " " + word
			default:
				out = append(out, line)
				line = word
			}
		}
		out = append(out, line)
	}
	return out
}
