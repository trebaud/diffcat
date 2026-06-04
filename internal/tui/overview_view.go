package tui

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/trebaud/diffcat/internal/git"
)

// overview_view.go renders the Stats dashboard (viewOverview): a full-width
// header, a title + commit-count summary, and a per-author ranking by commit
// count (each human author and each AI agent its own bar). No file list — the
// dashboard is intentionally diff-free so it stays fast on a deep history.

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

// overviewBody builds exactly height body lines: the title, the commit-count
// summary, an "Authored by" heading, and one ranked bar per author. With more
// authors than fit, the last line becomes a "+N more" roll-up; the rest is padded
// blank to fill the height.
func (m model) overviewBody(width, height int) []string {
	// The Stats come from a background `git log` walk; meaningless until it lands,
	// so show a loading body rather than an empty ranking.
	if !m.historyComputed {
		return m.overviewLoading(width, height)
	}

	out := []string{
		padLine(m.overviewTitle(), width),
		padLine("", width),
		padLine(m.overviewSummary(), width),
		padLine("", width),
		padLine(headingStyle.Render("  Authored by"), width),
	}

	authors := m.historyStats.Authors
	total := authorTotal(authors)
	avail := height - len(out)
	if avail < 0 {
		avail = 0
	}
	switch {
	case total == 0:
		out = append(out, padLine(mutedStyle.Render("  no commits"), width))
	case avail > 0:
		overflow := 0
		if len(authors) > avail {
			// Reserve the last visible line for the "+N more" roll-up.
			authors = authors[:max(0, avail-1)]
			overflow = len(m.historyStats.Authors) - len(authors)
		}
		for _, s := range authors {
			out = append(out, padLine(authorRow(s, total), width))
		}
		if overflow > 0 {
			out = append(out, padLine(mutedStyle.Render(fmt.Sprintf("  +%d more", overflow)), width))
		}
	}

	if len(out) > height {
		out = out[:height]
	}
	for len(out) < height {
		out = append(out, padLine("", width))
	}
	return out
}

// overviewTitle labels the dashboard: the whole-repo Stats.
func (m model) overviewTitle() string {
	return "  " + titleStyle.Render("Stats") + "  " + mutedStyle.Render("entire commit history")
}

// overviewSummary is the one-line totals row: the non-merge commit count and how
// many distinct authors contributed.
func (m model) overviewSummary() string {
	authors := len(m.historyStats.Authors)
	aw := "authors"
	if authors == 1 {
		aw = "author"
	}
	return "  " + headingStyle.Render(fmt.Sprintf("%d commits", m.historyStats.Commits)) +
		"   " + mutedStyle.Render(fmt.Sprintf("%d %s", authors, aw))
}

// overviewLoading is the placeholder body shown while the stats are still being
// gathered in the background: the title, a muted progress line, and blank fill to
// the exact body height.
func (m model) overviewLoading(width, height int) []string {
	out := []string{
		padLine(m.overviewTitle(), width),
		padLine("", width),
		padLine(mutedStyle.Render("  analyzing commit history…"), width),
	}
	if len(out) > height {
		return out[:height]
	}
	for len(out) < height {
		out = append(out, padLine("", width))
	}
	return out
}

// sharePct is value as a rounded whole percentage of total (0 when total is 0).
func sharePct(value, total int) int {
	if total <= 0 {
		return 0
	}
	return int(float64(value)/float64(total)*100 + 0.5)
}

// authorTotal sums the commit counts across authorship buckets — the denominator
// for the ranking bars (and equal to the non-merge commit count).
func authorTotal(shares []git.AuthorShare) int {
	total := 0
	for _, s := range shares {
		total += s.Commits
	}
	return total
}

// authorRow renders one ranked author line: a padded name, a fixed-width bar of
// their share of all commits (accent-colored for an AI agent, green for a human),
// the rounded percentage, and the commit count.
func authorRow(s git.AuthorShare, total int) string {
	const barW = 12
	pct, filled := sharePct(s.Commits, total), 0
	if total > 0 {
		filled = int(float64(s.Commits)/float64(total)*float64(barW) + 0.5)
	}
	if filled > barW {
		filled = barW
	}
	fill := addedStyle // human
	if s.AI {
		fill = titleStyle
	}
	bar := fill.Render(strings.Repeat("█", filled)) + borderStyle.Render(strings.Repeat("░", barW-filled))
	word := "commits"
	if s.Commits == 1 {
		word = "commit"
	}
	return "  " + padRight(truncateText(s.Name, 18), 18) + " " + bar +
		mutedStyle.Render(fmt.Sprintf(" %3d%%  %d %s", pct, s.Commits, word))
}

// padRight pads s with trailing spaces to exactly n display columns, truncating
// if it's already wider.
func padRight(s string, n int) string {
	if w := lipgloss.Width(s); w < n {
		return s + strings.Repeat(" ", n-w)
	}
	return truncateText(s, n)
}
