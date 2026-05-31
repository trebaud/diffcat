package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/trebaud/diffcat/internal/git"
)

// overview_view.go renders the branch overview dashboard (viewOverview): a
// full-width header, a fixed summary + language block, then a scrollable
// per-file churn list with proportional add/del bars.

// overviewView composes the full screen for the dashboard: header, body padded
// to fill the height, and the shared footer. Like the two-pane render it emits
// exactly m.height lines, each m.width wide.
func (m model) overviewView() string {
	header := padLine(m.headerView(), m.width)
	footer := padLine(m.footerView(), m.width)
	bodyHeight := m.height - 2
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	lines := []string{header}
	lines = append(lines, m.overviewBody(m.width, bodyHeight)...)
	lines = append(lines, footer)
	return strings.Join(lines, "\n")
}

// overviewBody builds exactly height body lines: a fixed top block (summary +
// languages + the "Files changed" heading) followed by the scrollable file
// list, windowed around overviewCursor and padded to fill.
func (m model) overviewBody(width, height int) []string {
	files := m.overviewFiles()
	maxChurn := 0
	for _, f := range files {
		if c := churnOf(f); c > maxChurn {
			maxChurn = c
		}
	}

	top := []string{padLine(m.overviewSummary(), width)}
	if langs := languageStats(m.files); len(langs) > 0 {
		total := 0
		for _, l := range langs {
			total += l.churn
		}
		if len(langs) > 5 {
			langs = langs[:5]
		}
		top = append(top, padLine("", width), padLine(headingStyle.Render("  Languages"), width))
		for _, l := range langs {
			top = append(top, padLine(m.langRow(l, total, width), width))
		}
	}
	top = append(top, padLine("", width),
		padLine(headingStyle.Render(fmt.Sprintf("  Files changed (%d)", len(files))), width))

	// If the fixed block already fills the screen, just show what fits.
	if len(top) >= height {
		return top[:height]
	}
	region := height - len(top)

	cur := m.overviewCursor
	if cur >= len(files) {
		cur = max(0, len(files)-1)
	}
	offset := 0
	if cur >= region {
		offset = cur - region + 1
	}
	end := min(offset+region, len(files))

	out := top
	if len(files) == 0 {
		out = append(out, padLine(mutedStyle.Render("  no changes against base"), width))
	}
	for i := offset; i < end; i++ {
		out = append(out, m.overviewFileRow(files[i], i == cur, maxChurn, width))
	}
	for shown := end - offset; shown < region; shown++ {
		if len(files) == 0 && shown == 0 {
			continue // the "no changes" line already used a slot
		}
		out = append(out, padLine("", width))
	}
	// Guard the exact height (the no-files branch can be off by one).
	if len(out) > height {
		out = out[:height]
	}
	for len(out) < height {
		out = append(out, padLine("", width))
	}
	return out
}

// overviewSummary is the one-line totals row: file count, aggregate +adds/-dels,
// and the branch's commit count.
func (m model) overviewSummary() string {
	add, del := 0, 0
	for _, f := range m.files {
		if f.Added > 0 {
			add += f.Added
		}
		if f.Deleted > 0 {
			del += f.Deleted
		}
	}
	stat := addedStyle.Render(fmt.Sprintf("+%d", add)) + " " + removedStyle.Render(fmt.Sprintf("-%d", del))
	commits := mutedStyle.Render(fmt.Sprintf("%d commits", len(m.commits)))
	return "  " + headingStyle.Render(fmt.Sprintf("%d files changed", len(m.files))) +
		"   " + stat + "   " + commits
}

// langRow renders one language's share: a padded name, a fixed mini-bar of its
// fraction of total churn, and the percentage.
func (m model) langRow(l langStat, total, width int) string {
	const barW = 12
	pct, filled := 0, 0
	if total > 0 {
		pct = int(float64(l.churn)/float64(total)*100 + 0.5)
		filled = int(float64(l.churn)/float64(total)*float64(barW) + 0.5)
	}
	if filled > barW {
		filled = barW
	}
	bar := metaStyle.Render(strings.Repeat("█", filled)) + borderStyle.Render(strings.Repeat("░", barW-filled))
	name := truncateText(l.name, 16)
	return "  " + padRight(name, 16) + " " + bar + mutedStyle.Render(fmt.Sprintf(" %3d%%", pct))
}

// overviewFileRow renders one file's churn line: status glyph, path, +adds/-dels,
// and a proportional add/del bar scaled to the largest file's churn. The selected
// row is a continuous highlight bar (with a plain bar so it reads on the tint).
func (m model) overviewFileRow(f git.FileChange, selected bool, maxChurn, width int) string {
	glyph := statusGlyph(f.Status)
	stats := fmt.Sprintf("+%d -%d", f.Added, f.Deleted)
	if f.Binary() {
		stats = "bin"
	}
	statsW := lipgloss.Width(stats)

	barW := width / 5
	if barW > 20 {
		barW = 20
	}
	if barW < 6 {
		barW = 6
	}

	prefixW := 4 // "  " + glyph + " "
	avail := width - prefixW - statsW - 2 - barW - 1
	if avail < 3 {
		avail = 3
	}
	name := truncatePath(f.Path, avail)
	gap := width - prefixW - lipgloss.Width(name) - statsW - 2 - barW
	if gap < 1 {
		gap = 1
	}

	if selected {
		row := "  " + glyph + " " + name + strings.Repeat(" ", gap) + stats + "  " +
			plainBar(f.Added, f.Deleted, maxChurn, barW)
		return selectedRowStyle.Width(width).Render(row)
	}

	statsStyled := mutedStyle.Render(stats)
	if !f.Binary() {
		statsStyled = addedStyle.Render(fmt.Sprintf("+%d", f.Added)) + " " +
			removedStyle.Render(fmt.Sprintf("-%d", f.Deleted))
	}
	row := "  " + statusStyle(f.Status).Render(glyph) + " " + name + strings.Repeat(" ", gap) +
		statsStyled + "  " + diffstatBar(f.Added, f.Deleted, maxChurn, barW)
	return padLine(row, width)
}

// barCells splits a width-wide bar into green (added), red (deleted), and faint
// track (unfilled) cells. The filled length is the file's churn relative to the
// largest file's churn, so longer bars mean more change; within the filled span,
// the green/red split mirrors the add/delete ratio.
func barCells(added, deleted, maxChurn, width int) (green, red, track int) {
	churn := 0
	if added > 0 {
		churn += added
	}
	if deleted > 0 {
		churn += deleted
	}
	filled := 0
	if maxChurn > 0 {
		filled = int(float64(churn)/float64(maxChurn)*float64(width) + 0.5)
	}
	if filled > width {
		filled = width
	}
	if churn > 0 && filled == 0 {
		filled = 1 // any change earns at least one cell
	}
	if churn > 0 {
		green = int(float64(added)/float64(churn)*float64(filled) + 0.5)
	}
	if green > filled {
		green = filled
	}
	red = filled - green
	track = width - filled
	return green, red, track
}

// diffstatBar renders the colored churn bar (green adds, red dels, faint track).
func diffstatBar(added, deleted, maxChurn, width int) string {
	g, r, t := barCells(added, deleted, maxChurn, width)
	return addedStyle.Render(strings.Repeat("█", g)) +
		removedStyle.Render(strings.Repeat("█", r)) +
		borderStyle.Render(strings.Repeat("░", t))
}

// plainBar is diffstatBar without color, for the selected row whose highlight
// bar owns the whole row's tint.
func plainBar(added, deleted, maxChurn, width int) string {
	g, r, t := barCells(added, deleted, maxChurn, width)
	return strings.Repeat("█", g+r) + strings.Repeat("░", t)
}

// padRight pads s with trailing spaces to exactly n display columns, truncating
// if it's already wider.
func padRight(s string, n int) string {
	if w := lipgloss.Width(s); w < n {
		return s + strings.Repeat(" ", n-w)
	}
	return truncateText(s, n)
}
