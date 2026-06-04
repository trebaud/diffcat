package tui

import (
	"fmt"
	"strings"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/trebaud/diffcat/internal/git"
)

// overview_view.go renders the Stats dashboard (viewOverview): a full-width
// header and title, then a two-pane body — the scrollable per-author commit
// ranking on the left, and on the right the contribution calendar spanning the
// full right section on top with the remaining activity charts (AI adoption,
// commit timeline, day×hour heatmap, …) gridded smaller beneath it. On a short
// pane the calendar folds back into the grid. No file list — the dashboard is
// intentionally diff-free so it stays fast on a deep history.

// Layout constants for the two-pane body. The author pane is wide enough for a
// full ranking row ("name  ▇▇▇░░░  100%  NNNNNN commits"); the charts need at
// least overviewMinChartWidth for the 24-hour heatmap. Below the combined width
// the body falls back to a single full-width author column (charts dropped).
const (
	overviewAuthorPaneWidth = 54
	overviewMinChartWidth   = 30
)

// overviewTwoPane reports whether the screen is wide enough for the side-by-side
// author-ranking / charts layout (else the author ranking goes full-width).
func (m model) overviewTwoPane() bool {
	return m.width >= overviewAuthorPaneWidth+1+overviewMinChartWidth
}

// minGridChartRows is the smallest chart grid worth keeping beneath the
// contribution calendar at the top of the right pane; under it the calendar folds
// back into the grid so a short pane isn't left with a stub of charts.
const minGridChartRows = 5

// overviewCharts builds the right pane: the contribution calendar spanning the
// full pane width on top, then the remaining charts packed into the smaller grid
// below it. When the pane is too short to spare the calendar's rows plus a usable
// grid (or there's no dated activity), the calendar rides in the grid like any
// other chart instead. Returns exactly height lines.
func (m model) overviewCharts(hs git.HistoryStats, width, height int) []string {
	contrib := contributionBlock(hs, width, 1)
	if contrib == nil || height < len(contrib)+1+minGridChartRows {
		return m.chartGrid(hs, width, height, true)
	}
	out := append([]string{}, contrib...)
	out = append(out, "") // blank between the calendar and the charts below
	return append(out, m.chartGrid(hs, width, height-len(out), false)...)
}

// overviewPaneHeight is the height of the two-pane region: the body (between
// header and footer) minus the title row and the blank under it.
func (m model) overviewPaneHeight() int {
	return max(1, (m.height-2)-2)
}

// overviewAuthorViewport is how many author rows are visible at once (the pane
// height minus its heading).
func (m model) overviewAuthorViewport() int {
	return max(1, m.overviewPaneHeight()-1)
}

// overviewMaxScroll is the furthest the author ranking can scroll: enough to bring
// the last author into view, no further.
func (m model) overviewMaxScroll() int {
	return max(0, len(m.historyStats.Authors)-m.overviewAuthorViewport())
}

// scrollOverview moves the author-ranking offset by delta, clamped to range.
func (m *model) scrollOverview(delta int) {
	m.overviewScroll += delta
	if hi := m.overviewMaxScroll(); m.overviewScroll > hi {
		m.overviewScroll = hi
	}
	if m.overviewScroll < 0 {
		m.overviewScroll = 0
	}
}

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

// overviewBody builds exactly height body lines: a title row, a blank, then the
// two-pane region — the scrollable author ranking on the left and the charts
// spread down the right, divided by a vertical rule. On a narrow screen the
// ranking takes the full width and the charts are dropped.
func (m model) overviewBody(width, height int) []string {
	// The Stats come from a background `git log` walk; meaningless until it lands,
	// so show a loading body rather than an empty ranking.
	if !m.historyComputed {
		return m.overviewLoading(width, height)
	}

	hs := m.historyStats
	total := authorTotal(hs.Authors)

	out := []string{
		padLine(m.overviewTitle(), width),
		padLine("", width),
	}
	paneHeight := max(1, height-len(out))

	if total == 0 {
		out = append(out, padLine(mutedStyle.Render("  no commits"), width))
		return fitHeight(out, width, height)
	}

	if !m.overviewTwoPane() {
		// Narrow: the ranking takes the whole width; charts need more room than a
		// single column can spare, so they're dropped here.
		for _, l := range m.authorList(hs, total, paneHeight) {
			out = append(out, padLine(l, width))
		}
		return fitHeight(out, width, height)
	}

	leftW := overviewAuthorPaneWidth
	rightW := width - leftW - 1
	left := m.authorList(hs, total, paneHeight)
	right := m.overviewCharts(hs, rightW, paneHeight)
	div := borderStyle.Render("│")
	for i := 0; i < paneHeight; i++ {
		var l, r string
		if i < len(left) {
			l = left[i]
		}
		if i < len(right) {
			r = right[i]
		}
		out = append(out, padLine(padLine(l, leftW)+div+padLine(r, rightW), width))
	}
	return fitHeight(out, width, height)
}

// authorList is the left pane: an "Authored by" heading (with the visible range
// when the ranking scrolls) followed by the window of ranked author rows at the
// current scroll offset. Returns at most height lines (heading + viewport rows).
func (m model) authorList(hs git.HistoryStats, total, height int) []string {
	authors := hs.Authors
	viewport := max(1, height-1)
	start := m.overviewScroll
	if hi := max(0, len(authors)-viewport); start > hi {
		start = hi
	}
	if start < 0 {
		start = 0
	}
	end := min(start+viewport, len(authors))

	heading := fmt.Sprintf("  Authored by  %d", len(authors))
	if len(authors) > viewport {
		heading = fmt.Sprintf("  Authored by  %d–%d of %d", start+1, end, len(authors))
	}
	out := []string{headingStyle.Render(heading)}
	for i := start; i < end; i++ {
		out = append(out, authorRow(authors[i], total))
	}
	return out
}

// fitHeight clamps out to exactly height lines: truncating an overflow, padding a
// short body with blank width-wide lines.
func fitHeight(out []string, width, height int) []string {
	if len(out) > height {
		return out[:height]
	}
	for len(out) < height {
		out = append(out, padLine("", width))
	}
	return out
}

// overviewTitle labels the dashboard: the whole-repo Stats plus its totals.
func (m model) overviewTitle() string {
	head := "  " + titleStyle.Render("Stats") + "  " + mutedStyle.Render("entire commit history")
	if !m.historyComputed {
		return head
	}
	authors := len(m.historyStats.Authors)
	aw := "authors"
	if authors == 1 {
		aw = "author"
	}
	return head + "   " + mutedStyle.Render(fmt.Sprintf("· %d commits · %d %s", m.historyStats.Commits, authors, aw))
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

// adoptionBlock is the AI-adoption curve: two one-row sparklines (human in green,
// AI in accent) sharing one vertical scale so the AI line visibly rises from
// nothing as the human line towers, plus a headline with the first AI commit date
// and the AI share of the most recent commits. Returns nil if too narrow.
func adoptionBlock(hs git.HistoryStats, width int) []string {
	const label = 7
	cols := width - 2 - label
	if cols < 8 {
		return nil
	}
	if cols > 64 {
		cols = 64
	}
	bs := rebucket(hs.Daily, cols)
	human := make([]int, len(bs))
	ai := make([]int, len(bs))
	scale := 0
	for i, d := range bs {
		human[i], ai[i] = d.Human, d.AI
		scale = max(scale, max(d.Human, d.AI))
	}
	headline := fmt.Sprintf("  first AI commit %s · %d%% of last %d",
		hs.FirstAI.Format("2006-01-02"), sharePct(hs.RecentAI, hs.RecentTotal), hs.RecentTotal)
	return []string{
		headingStyle.Render("  AI adoption"),
		"  " + padRight("human", label) + addedStyle.Render(sparklineMax(human, scale)),
		"  " + padRight("ai", label) + titleStyle.Render(sparklineMax(ai, scale)),
		mutedStyle.Render(headline),
	}
}

// timelineBlock is the commit-activity timeline: one self-scaled sparkline of all
// commits per time bucket across the repo's life, plus a totals summary. Returns
// nil if too narrow.
func timelineBlock(hs git.HistoryStats, width int) []string {
	cols := width - 2
	if cols < 8 {
		return nil
	}
	if cols > 80 {
		cols = 80
	}
	bs := rebucket(hs.Daily, cols)
	vals := make([]int, len(bs))
	for i, d := range bs {
		vals[i] = d.Human + d.AI
	}
	span := spanLabel(len(hs.Daily))
	return []string{
		headingStyle.Render("  Commit activity"),
		"  " + addedStyle.Render(sparklineMax(vals, maxInts(vals))),
		mutedStyle.Render(fmt.Sprintf("  %d commits · %s", hs.Commits, span)),
	}
}

// heatmapBlock is the punch-card heatmap: an hour ruler plus seven Mon–Sun rows of
// 24 shaded cells, each cell's shade scaled to the busiest hour in the grid.
// Returns nil if too narrow or there's no dated activity.
func heatmapBlock(hs git.HistoryStats, width int) []string {
	const prefix = 6 // "  Mon " — 2 indent + 3 label + 1 space
	if width < prefix+24 {
		return nil
	}
	peak := 0
	for d := 0; d < 7; d++ {
		for h := 0; h < 24; h++ {
			peak = max(peak, hs.Punch[d][h])
		}
	}
	if peak == 0 {
		return nil
	}

	ruler := []rune(strings.Repeat(" ", 24))
	for _, mk := range []struct {
		at  int
		lbl string
	}{{0, "0"}, {6, "6"}, {12, "12"}, {18, "18"}} {
		for i, c := range mk.lbl {
			if mk.at+i < 24 {
				ruler[mk.at+i] = c
			}
		}
	}
	out := []string{
		headingStyle.Render("  Activity by hour"),
		strings.Repeat(" ", prefix) + mutedStyle.Render(string(ruler)),
	}

	// git weekday is Sunday=0; show Mon–Sun, the conventional punch-card order.
	for _, wd := range []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday} {
		var b strings.Builder
		b.WriteString("  " + mutedStyle.Render(padRight(wd.String()[:3], 3)) + " ")
		for h := 0; h < 24; h++ {
			b.WriteString(heatCell(hs.Punch[int(wd)][h], peak))
		}
		out = append(out, b.String())
	}
	return out
}

// heatCell renders one punch-card cell: a faint dot for an empty hour, else one of
// four shade blocks (green) scaled to peak, the busiest hour in the grid.
func heatCell(v, peak int) string {
	return heatCellWidth(v, peak, 1)
}

// heatCellWidth is heatCell drawn w columns wide — the contribution hero uses w=2
// so its cells read as chunky GitHub-style squares rather than thin slivers.
func heatCellWidth(v, peak, w int) string {
	if w < 1 {
		w = 1
	}
	if v <= 0 {
		return borderStyle.Render(strings.Repeat("·", w))
	}
	shades := []rune("░▒▓█")
	lvl := v * len(shades) / peak
	if lvl < 1 {
		lvl = 1
	}
	if lvl > len(shades) {
		lvl = len(shades)
	}
	return addedStyle.Render(strings.Repeat(string(shades[lvl-1]), w))
}

// contributionBlock is the GitHub-style contribution calendar: weeks as columns,
// seven weekday rows (Sun–Sat), each cell shaded by that day's commit count via
// heatCellWidth, scaled to the busiest day in view. Each cell is cellW columns
// wide (the full-width hero uses 2 for chunky squares; the grid fallback uses 1).
// Columns align to the week of the first commit (Start.Weekday()); only the
// trailing weeks that fit width are shown. A month-abbreviation ruler labels where
// each month begins. Returns nil if too narrow or there's no dated activity.
func contributionBlock(hs git.HistoryStats, width, cellW int) []string {
	const prefix = 6 // "  Sun " — 2 indent + 3 label + 1 space
	if cellW < 1 {
		cellW = 1
	}
	avail := (width - prefix) / cellW
	if avail < 6 || len(hs.Daily) == 0 {
		return nil
	}
	offset := int(hs.Start.Weekday()) // column 0 begins on the week containing Start
	totalWeeks := (offset + len(hs.Daily) + 6) / 7
	if totalWeeks < 1 {
		return nil
	}
	if avail > totalWeeks {
		avail = totalWeeks
	}
	startWeek := totalWeeks - avail // show the trailing `avail` weeks

	// grid holds each visible day's commit count; present marks which cells map to a
	// real calendar day (so the ragged first/last weeks render as blanks, not dots).
	var grid, present [7][]int
	for wd := 0; wd < 7; wd++ {
		grid[wd] = make([]int, avail)
		present[wd] = make([]int, avail)
	}
	peak := 0
	for i, d := range hs.Daily {
		col := (offset+i)/7 - startWeek
		if col < 0 || col >= avail {
			continue
		}
		wd := (offset + i) % 7
		grid[wd][col] = d.Human + d.AI
		present[wd][col] = 1
		peak = max(peak, grid[wd][col])
	}

	// Month ruler: a 3-letter abbreviation at the column where each month first
	// appears across the visible week starts. Collect candidates first, then drop
	// any whose label would overrun the next one — which only happens for a leading
	// partial week (e.g. a Dec 28 column one week before Jan), so the throwaway
	// partial-month label is the one dropped.
	type monthMark struct {
		col   int
		label string
	}
	var marks []monthMark
	prevMonth := time.Month(0)
	for col := 0; col < avail; col++ {
		ws := hs.Start.AddDate(0, 0, (startWeek+col)*7-offset)
		if m := ws.Month(); m != prevMonth {
			prevMonth = m
			marks = append(marks, monthMark{col, ws.Format("Jan")})
		}
	}
	ruler := []rune(strings.Repeat(" ", avail*cellW))
	for i, mk := range marks {
		if i+1 < len(marks) && (marks[i+1].col-mk.col)*cellW < len(mk.label) {
			continue // its label would collide with the next month's — drop it
		}
		for k, c := range mk.label {
			if pos := mk.col*cellW + k; pos < len(ruler) {
				ruler[pos] = c
			}
		}
	}

	out := []string{
		headingStyle.Render("  Contribution graph"),
		strings.Repeat(" ", prefix) + mutedStyle.Render(string(ruler)),
	}
	labels := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	for wd := 0; wd < 7; wd++ {
		var b strings.Builder
		b.WriteString("  " + mutedStyle.Render(labels[wd]) + " ")
		for col := 0; col < avail; col++ {
			if present[wd][col] == 0 {
				b.WriteString(strings.Repeat(" ", cellW))
				continue
			}
			b.WriteString(heatCellWidth(grid[wd][col], peak, cellW))
		}
		out = append(out, b.String())
	}
	return out
}

// timeOfDayBlock is the time-of-day rhythm: a 24-hour sparkline of commits summed
// across every weekday, an hour ruler, and a caption naming the peak hour and the
// share of commits landing after midnight. Returns nil if too narrow or empty.
func timeOfDayBlock(hs git.HistoryStats, width int) []string {
	if width-2 < 24 {
		return nil
	}
	hours := hs.HourOfDay()
	total, peak, peakHr, night := 0, 0, 0, 0
	for h, v := range hours {
		total += v
		if v > peak {
			peak, peakHr = v, h
		}
		if h < 6 {
			night += v
		}
	}
	if total == 0 {
		return nil
	}
	ruler := []rune(strings.Repeat(" ", 24))
	for _, mk := range []struct {
		at  int
		lbl string
	}{{0, "0"}, {6, "6"}, {12, "12"}, {18, "18"}} {
		for i, c := range mk.lbl {
			if mk.at+i < 24 {
				ruler[mk.at+i] = c
			}
		}
	}
	return []string{
		headingStyle.Render("  Time of day"),
		"  " + addedStyle.Render(sparklineMax(hours[:], peak)),
		strings.Repeat(" ", 2) + mutedStyle.Render(string(ruler)),
		mutedStyle.Render(fmt.Sprintf("  peak %dh · %d%% after midnight", peakHr, sharePct(night, total))),
	}
}

// weekdayBlock is the per-weekday rollup: seven horizontal bars (Mon–Sun) of
// commits summed across hours, scaled to the busiest weekday, with the count
// suffixed. Returns nil if too narrow or there's no activity.
func weekdayBlock(hs git.HistoryStats, width int) []string {
	totals := hs.WeekdayTotals()
	peak := 0
	for _, v := range totals {
		peak = max(peak, v)
	}
	if peak == 0 {
		return nil
	}
	const prefix = 6 // "  Mon "
	barW := width - prefix - len(fmt.Sprintf(" %d", peak))
	if barW < 6 {
		return nil
	}
	if barW > 18 {
		barW = 18
	}
	out := []string{headingStyle.Render("  By weekday")}
	for _, wd := range []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday} {
		v := totals[int(wd)]
		filled := v * barW / peak
		if v > 0 && filled == 0 {
			filled = 1
		}
		bar := addedStyle.Render(strings.Repeat("█", filled)) + borderStyle.Render(strings.Repeat("░", barW-filled))
		out = append(out, "  "+mutedStyle.Render(wd.String()[:3])+" "+bar+mutedStyle.Render(fmt.Sprintf(" %d", v)))
	}
	return out
}

// humanAIBlock is the overall human-vs-AI split as one proportion bar (green human
// run, accent AI run) with a percentage caption. Returns nil with no commits; the
// caller only shows it when the repo has AI activity.
func humanAIBlock(hs git.HistoryStats, width int) []string {
	human, ai := hs.HumanAICommits()
	total := human + ai
	if total == 0 {
		return nil
	}
	barW := width - 2
	if barW < 8 {
		return nil
	}
	if barW > 40 {
		barW = 40
	}
	h := human * barW / total
	if human > 0 && h == 0 {
		h = 1
	}
	if h > barW {
		h = barW
	}
	bar := addedStyle.Render(strings.Repeat("█", h)) + titleStyle.Render(strings.Repeat("█", barW-h))
	return []string{
		headingStyle.Render("  Human vs AI"),
		"  " + bar,
		mutedStyle.Render(fmt.Sprintf("  human %d%% · ai %d%%", sharePct(human, total), sharePct(ai, total))),
	}
}

// streakBlock is the streak & cadence card: longest and current commit streaks,
// the busiest single day, and the average commits per week — all from the daily
// series. Returns nil if there's no dated activity.
func streakBlock(hs git.HistoryStats, width int) []string {
	if len(hs.Daily) == 0 {
		return nil
	}
	longest, current := hs.Streaks()
	day, busiest := hs.BusiestDay()
	dayWord := "days"
	if longest == 1 {
		dayWord = "day"
	}
	out := []string{
		headingStyle.Render("  Streak & cadence"),
		"  " + addedStyle.Render(fmt.Sprintf("longest %d %s", longest, dayWord)) +
			mutedStyle.Render(fmt.Sprintf(" · now %d", current)),
	}
	if busiest > 0 {
		out = append(out, mutedStyle.Render(fmt.Sprintf("  busiest %s · %d commits", day.Format("Jan 2"), busiest)))
	}
	return append(out, mutedStyle.Render(fmt.Sprintf("  ~%.0f commits / week", hs.CommitsPerWeek())))
}

// concentrationBlock is the authorship-concentration card: a bar of the top
// author's share plus the bus factor (authors making up half the commits).
// Returns nil if there are no authors.
func concentrationBlock(hs git.HistoryStats, width int) []string {
	if len(hs.Authors) == 0 {
		return nil
	}
	topPct, busFactor := hs.Concentration()
	barW := width - 2
	if barW < 8 {
		return nil
	}
	if barW > 24 {
		barW = 24
	}
	filled := topPct * barW / 100
	if topPct > 0 && filled == 0 {
		filled = 1
	}
	if filled > barW {
		filled = barW
	}
	bar := addedStyle.Render(strings.Repeat("█", filled)) + borderStyle.Render(strings.Repeat("░", barW-filled))
	word := "authors"
	if busFactor == 1 {
		word = "author"
	}
	return []string{
		headingStyle.Render("  Concentration"),
		"  " + bar + mutedStyle.Render(fmt.Sprintf(" %d%%", topPct)),
		mutedStyle.Render(fmt.Sprintf("  %d %s = half of commits", busFactor, word)),
	}
}

// rebucket aggregates a daily commit series into at most cols evenly-spaced
// buckets, summing the days that fall in each. Returns the series unchanged when
// it's already no wider than cols, so short histories aren't stretched.
func rebucket(daily []git.DayCount, cols int) []git.DayCount {
	if cols < 1 {
		cols = 1
	}
	if len(daily) <= cols {
		return daily
	}
	out := make([]git.DayCount, cols)
	for i, d := range daily {
		b := i * cols / len(daily)
		if b >= cols {
			b = cols - 1
		}
		out[b].Human += d.Human
		out[b].AI += d.AI
	}
	return out
}

// sparklineMax renders values as a row of block runes scaled to max: a space for a
// zero bucket, otherwise the lowest block at minimum so any activity is visible.
func sparklineMax(values []int, max int) string {
	blocks := []rune("▁▂▃▄▅▆▇█")
	var b strings.Builder
	for _, v := range values {
		if v <= 0 {
			b.WriteRune(' ')
			continue
		}
		lvl := 0
		if max > 0 {
			lvl = v * (len(blocks) - 1) / max
		}
		if lvl > len(blocks)-1 {
			lvl = len(blocks) - 1
		}
		b.WriteRune(blocks[lvl])
	}
	return b.String()
}

// spanLabel describes a history of days days in the largest sensible unit: days
// under two weeks, else whole weeks — with correct singular/plural.
func spanLabel(days int) string {
	if days < 14 {
		if days == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", days)
	}
	weeks := (days + 3) / 7 // round to nearest week
	return fmt.Sprintf("%d weeks", weeks)
}

// maxInts is the largest value in xs, or 0 when empty.
func maxInts(xs []int) int {
	m := 0
	for _, x := range xs {
		m = max(m, x)
	}
	return m
}
