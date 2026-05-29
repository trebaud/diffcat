package tui

import (
	"testing"

	"github.com/trebaud/diff-master/internal/diff"
)

// firstExpand returns the index of the first expand affordance in the unified
// view, or -1.
func firstExpand(lines []diff.Line) int {
	for i, l := range lines {
		if l.Kind == diff.Expand {
			return i
		}
	}
	return -1
}

func TestExpandUnderCursorRevealsContext(t *testing.T) {
	m := sampleModel()
	m.focus = focusDiff

	idx := firstExpand(m.viewLines)
	if idx < 0 {
		t.Fatal("expected at least one expand row in the sample view")
	}
	m.diffCursor = idx

	l := m.cursorLine()
	if l == nil || l.Kind != diff.Expand {
		t.Fatalf("cursorLine = %+v, want an Expand row", l)
	}

	before := len(m.viewLines)
	m.expandUnderCursor(*l)
	after := len(m.viewLines)

	if after <= before {
		t.Errorf("view did not grow after expansion: before=%d after=%d", before, after)
	}
	if m.diffCursor < 0 || m.diffCursor >= m.totalDiffRows() {
		t.Errorf("cursor %d out of range after expansion (rows=%d)", m.diffCursor, m.totalDiffRows())
	}
}

func TestExpandAllRevealsWholeGap(t *testing.T) {
	m := sampleModel()
	m.focus = focusDiff
	idx := firstExpand(m.viewLines)
	if idx < 0 {
		t.Fatal("expected an expand row")
	}
	l := m.viewLines[idx]
	if l.Dir != diff.ExpandAll {
		t.Skip("leading gap is larger than the window; not a single-press case")
	}
	hidden := l.Hidden
	m.diffCursor = idx
	m.expandUnderCursor(l)

	// One press on an "expand all" affordance should reveal every hidden line and
	// remove that gap's affordance entirely.
	g := m.gaps[l.GapID]
	rev := m.revealed[l.GapID]
	if rev[0] < g.NewEnd-g.NewStart+1 {
		t.Errorf("revealed %d of %d hidden lines after expand-all", rev[0], hidden)
	}
}
