package tui

import (
	"charm.land/lipgloss/v2"

	"github.com/trebaud/diffcat/internal/diff"
)

// minimap.go renders the one-column density map pinned to the right edge of the
// diff pane: the whole diff scaled to pane height, each cell colored by what
// changed there (green add / red del / faint context), with the visible viewport
// lit as a thumb and search hits flagged in the accent color — a sense-of-place
// strip that reuses the heatmaps' shading vocabulary so it looks native.

// minimapW is the column budget the minimap takes from the diff pane: one column,
// but only when the pane is wide enough to spare it and there's a diff to map.
func (m model) minimapW(width int) int {
	if width >= 40 && m.totalDiffRows() > 0 {
		return 1
	}
	return 0
}

// mmCat is the coarse change category of a diff row, for coloring its minimap cell.
type mmCat int

const (
	mmEmpty mmCat = iota
	mmContext
	mmAdd
	mmDel
	mmMeta
)

func lineCat(k diff.Kind) mmCat {
	switch k {
	case diff.Add:
		return mmAdd
	case diff.Del:
		return mmDel
	case diff.Hunk, diff.Meta, diff.Expand:
		return mmMeta
	default:
		return mmContext
	}
}

// diffRowCat categorizes the diff row at index d in the current projection
// (unified viewLines or split splitRows). A split row that both adds and removes
// (a changed line) counts as an addition — green reads as "new work".
func (m model) diffRowCat(d int) mmCat {
	if m.splitView {
		if d < 0 || d >= len(m.splitRows) {
			return mmEmpty
		}
		r := m.splitRows[d]
		if r.Full != nil {
			return lineCat(r.Full.Kind)
		}
		addSide := r.Right != nil && r.Right.Kind == diff.Add
		delSide := r.Left != nil && r.Left.Kind == diff.Del
		switch {
		case addSide:
			return mmAdd
		case delSide:
			return mmDel
		default:
			return mmContext
		}
	}
	if d < 0 || d >= len(m.viewLines) {
		return mmEmpty
	}
	return lineCat(m.viewLines[d].Kind)
}

// minimapColumn builds the rows-tall minimap as a slice of single-cell styled
// strings (one per body row). Each cell aggregates a proportional band of diff
// rows: a band with additions reads green, deletions red, everything else a faint
// track. Bands overlapping the viewport get a subtle highlight bar (the thumb),
// and bands holding a search hit are flagged in the accent color so n/N has a map.
func (m model) minimapColumn(rows int) []string {
	cells := make([]string, rows)
	total := m.totalDiffRows()
	if total == 0 || rows == 0 {
		for i := range cells {
			cells[i] = borderStyle.Render(" ")
		}
		return cells
	}

	// Search hits index viewLines (unified only); map them for a quick lookup.
	hits := map[int]bool{}
	if !m.splitView {
		for _, h := range m.searchHits {
			hits[h] = true
		}
	}

	viewTop := m.diffOffset
	viewBot := m.diffOffset + m.diffViewportHeight() // exclusive
	for i := 0; i < rows; i++ {
		lo := i * total / rows
		hi := (i + 1) * total / rows
		if hi <= lo {
			hi = lo + 1
		}
		if hi > total {
			hi = total
		}
		add, del := 0, 0
		hit := false
		for d := lo; d < hi; d++ {
			switch m.diffRowCat(d) {
			case mmAdd:
				add++
			case mmDel:
				del++
			}
			if hits[d] {
				hit = true
			}
		}

		char := "░"
		st := borderStyle
		switch {
		case hit:
			char, st = "█", lipgloss.NewStyle().Foreground(colAccent).Bold(true)
		case add > 0 && add >= del:
			char, st = "█", addedStyle
		case del > 0:
			char, st = "█", removedStyle
		}
		// Light the band as a thumb when it overlaps the visible viewport.
		if lo < viewBot && hi > viewTop {
			st = st.Background(colRowBg).Bold(true)
		}
		cells[i] = st.Render(char)
	}
	return cells
}
