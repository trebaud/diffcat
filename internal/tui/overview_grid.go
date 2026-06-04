package tui

import (
	"strings"

	"github.com/trebaud/diffcat/internal/git"
)

// overview_grid.go lays the Stats charts out as a responsive grid beneath the
// full-width contribution calendar. It spreads the charts across at least a couple
// of columns where width allows, then fills each column's leftover height by
// growing the flexible charts (the sparklines become taller area charts) rather
// than padding with blank gaps — so the charts get bigger to occupy the space.
// overviewMinChartWidth (in overview_view.go) is the hard floor on a column's
// width, set by the 24-hour heatmap.

// overviewMaxAreaRows caps how tall a flexible chart's area band may grow, so a
// lone sparkline in a very tall column becomes a generous block rather than an
// absurd wall; any height beyond that falls back to even gaps between charts.
const overviewMaxAreaRows = 6

// chartGrid renders the right-hand charts into a multi-column grid divided by the
// same faint rule as the author/chart split. It uses enough columns to spread the
// charts across the pane (at least two where width allows), then fills each
// column's height by growing its flexible charts before falling back to even gaps.
// Returns exactly height lines.
func (m model) chartGrid(hs git.HistoryStats, width, height int, withContribution bool) []string {
	// Probe each group's charts once at the floor width and minimum (one-row) height
	// to learn which have data (the only nils are data-driven — no AI, no commits —
	// and so are width-independent) and their floor heights. A group is a set of
	// related charts that must share a column; survivors are re-rendered at the real
	// column width and grown height once those are known.
	type chart struct {
		spec   chartSpec
		height int // floor height, at one area row
	}
	type group struct {
		charts []chart
		height int // floor total plus the blank lines between its charts
		perRow int // total lines the group gains per extra area row (growth capacity)
	}
	var groups []group
	for _, g := range overviewBlocks(withContribution) {
		var gr group
		for _, sp := range g {
			if b := sp.render(hs, overviewMinChartWidth, 1); b != nil {
				if len(gr.charts) > 0 {
					gr.height++ // blank between charts within the group
				}
				gr.charts = append(gr.charts, chart{sp, len(b)})
				gr.height += len(b)
				gr.perRow += sp.perRow
			}
		}
		if len(gr.charts) > 0 {
			groups = append(groups, gr)
		}
	}
	if len(groups) == 0 {
		return make([]string, height)
	}

	// pack assigns the groups to `cols` columns, each to the currently shortest
	// column (ties left) so the first `cols` groups seed one column each — no column
	// is ever empty — and the rest stay balanced. Group heights are width-
	// independent, so this runs without rendering.
	pack := func(cols int) [][]int {
		assign := make([][]int, cols)
		colH := make([]int, cols)
		for gi, g := range groups {
			s := 0
			for j := 1; j < cols; j++ {
				if colH[j] < colH[s] {
					s = j
				}
			}
			if colH[s] > 0 {
				colH[s]++ // blank separator before the group
			}
			colH[s] += g.height
			assign[s] = append(assign[s], gi)
		}
		return assign
	}
	colHeight := func(col []int) int {
		h := 0
		for i, gi := range col {
			if i > 0 {
				h++
			}
			h += groups[gi].height
		}
		return h
	}

	// Fewest columns whose tallest packed column still fits the floor height (so
	// nothing clips even before any growth), bounded by the width's hard floor and
	// the group count.
	maxCols := max(1, min((width+1)/(overviewMinChartWidth+1), len(groups)))
	fewest := 1
	for fewest < maxCols {
		fits := true
		for _, col := range pack(fewest) {
			if colHeight(col) > height {
				fits = false
				break
			}
		}
		if fits {
			break
		}
		fewest++
	}
	// Prefer at least two columns where width and chart count allow, so the charts
	// spread across the pane rather than stacking in one tall column.
	cols := max(fewest, min(2, maxCols, len(groups)))
	assign := pack(cols)

	colW := (width - (cols - 1)) / cols
	rem := (width - (cols - 1)) % cols // first `rem` columns get one more col
	widthOf := func(i int) int {
		if i < rem {
			return colW + 1
		}
		return colW
	}

	// Fill each column to the full height by growing its flexible charts: pick the
	// uniform area-row count whose extra lines soak up the column's slack (capped),
	// render the groups at that height, then spread any residual as even gaps.
	columns := make([][]string, cols)
	for c := 0; c < cols; c++ {
		floor, flexPerRow := 0, 0
		for i, gi := range assign[c] {
			if i > 0 {
				floor++ // separator before the group
			}
			floor += groups[gi].height
			flexPerRow += groups[gi].perRow
		}
		rows := 1
		if flexPerRow > 0 {
			if slack := height - floor; slack > 0 {
				rows = min(1+slack/flexPerRow, overviewMaxAreaRows)
			}
		}
		var blocks [][]string
		content := 0
		for _, gi := range assign[c] {
			var block []string
			for i, ch := range groups[gi].charts {
				if i > 0 {
					block = append(block, "")
				}
				block = append(block, ch.spec.render(hs, widthOf(c), rows)...)
			}
			blocks = append(blocks, block)
			content += len(block)
		}
		columns[c] = stackBlocks(blocks, content, height)
	}

	// Compose the columns side by side, each padded to its width and to height, with
	// a vertical rule between them.
	div := borderStyle.Render("│")
	out := make([]string, height)
	for row := 0; row < height; row++ {
		var b strings.Builder
		for c := 0; c < cols; c++ {
			if c > 0 {
				b.WriteString(div)
			}
			var line string
			if row < len(columns[c]) {
				line = columns[c][row]
			}
			b.WriteString(padLine(line, widthOf(c)))
		}
		out[row] = b.String()
	}
	return out
}

// stackBlocks joins a column's group blocks into height lines, distributing the
// slack (height − content) as even gaps between the blocks and below the last one,
// with at least one blank between blocks. The trailing gap is left to the caller's
// pad-to-height, so a column reads as evenly-spaced cards filling the pane rather
// than a tight stack over a blank void. content is the summed block line count.
func stackBlocks(blocks [][]string, content, height int) []string {
	if len(blocks) == 0 {
		return nil
	}
	slots := len(blocks) // (n−1) gaps between blocks + 1 trailing gap
	slack := max(0, height-content)
	gap := func(i int) int {
		g := slack / slots
		if i < slack%slots {
			g++
		}
		return g
	}
	var out []string
	for i, block := range blocks {
		if i > 0 {
			for k := 0; k < max(1, gap(i-1)); k++ {
				out = append(out, "")
			}
		}
		out = append(out, block...)
	}
	return out
}

// chartSpec is one chart in the grid: render draws it at a given column width and
// area-row count (fixed-height charts ignore the rows), and perRow is how many
// extra lines it gains per added area row — the grid's growth budget. perRow 0
// marks a fixed-height chart.
type chartSpec struct {
	render func(hs git.HistoryStats, width, rows int) []string
	perRow int
}

// overviewBlocks is the priority-ordered list of chart groups the grid fills from.
// Each group is a set of related charts the packer keeps contiguous in one column.
// AI-only charts return nil when the repo has no AI activity, so they self-skip
// (and an all-nil group drops out entirely).
//
// The AI story (adoption curve + human/AI split) is bundled into one group so the
// two always sit together in the same column; the activity rhythm (timeline, time
// of day, weekday, hour heatmap) and the summary cards (streak, concentration)
// follow as individual charts the packer balances across the remaining space. The
// three sparkline charts are flexible (perRow > 0) so they grow to fill height.
// withContribution prepends the contribution calendar — set only when it isn't
// already shown full-width above the grid.
func overviewBlocks(withContribution bool) [][]chartSpec {
	fixed := func(f func(git.HistoryStats, int) []string) chartSpec {
		return chartSpec{render: func(h git.HistoryStats, w, _ int) []string { return f(h, w) }}
	}
	adoption := chartSpec{
		render: func(h git.HistoryStats, w, r int) []string {
			if !h.HasAI {
				return nil
			}
			return adoptionBlock(h, w, r)
		},
		perRow: 2, // human and AI bands grow together
	}
	humanAI := fixed(func(h git.HistoryStats, w int) []string {
		if !h.HasAI {
			return nil
		}
		return humanAIBlock(h, w)
	})
	timeline := chartSpec{
		render: func(h git.HistoryStats, w, r int) []string { return timelineBlock(h, w, r) },
		perRow: 1,
	}
	timeOfDay := chartSpec{
		render: func(h git.HistoryStats, w, r int) []string { return timeOfDayBlock(h, w, r) },
		perRow: 1,
	}
	groups := [][]chartSpec{
		{adoption, humanAI}, // AI story — kept together in one column
		{timeline},
		{timeOfDay},
		{fixed(weekdayBlock)},
		{fixed(heatmapBlock)},
		{fixed(streakBlock)},
		{fixed(concentrationBlock)},
	}
	if withContribution {
		contrib := fixed(func(h git.HistoryStats, w int) []string { return contributionBlock(h, w, 1, 1) })
		groups = append([][]chartSpec{{contrib}}, groups...)
	}
	return groups
}
