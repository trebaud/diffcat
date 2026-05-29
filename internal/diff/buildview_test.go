package diff

import (
	"fmt"
	"testing"
)

// leadingTrailingRaw is a one-hunk diff with hidden context both before (new
// lines 1..9) and after (10/11 onward) the change.
const leadingTrailingRaw = "diff --git a/f b/f\n--- a/f\n+++ b/f\n" +
	"@@ -10,2 +10,3 @@\n context at 10\n+inserted\n context at 11\n"

func buildFor(t *testing.T, revealed map[int][2]int, window int) []Line {
	t.Helper()
	lines := Parse(leadingTrailingRaw)
	fileLines := make([]string, 20)
	for i := range fileLines {
		fileLines[i] = fmt.Sprintf("line %d", i+1)
	}
	gaps := Gaps(lines, len(fileLines))
	if len(gaps) != 2 {
		t.Fatalf("setup: want 2 gaps, got %d", len(gaps))
	}
	return BuildView(lines, fileLines, gaps, revealed, window)
}

func countKind(lines []Line, k Kind) int {
	n := 0
	for _, l := range lines {
		if l.Kind == k {
			n++
		}
	}
	return n
}

func TestBuildViewCollapsed(t *testing.T) {
	view := buildFor(t, map[int][2]int{}, 20)
	// Both gaps (9 and 8 lines) are below the window, so each is a single
	// "expand all" row and no context is revealed yet.
	if got := countKind(view, Expand); got != 2 {
		t.Errorf("want 2 expand rows, got %d", got)
	}
	if got := countKind(view, Context); got != 2 { // the two in-hunk context lines
		t.Errorf("want 2 context rows, got %d", got)
	}
	for _, l := range view {
		if l.Kind == Expand && l.Dir != ExpandAll {
			t.Errorf("small gap should be ExpandAll, got dir %d", l.Dir)
		}
	}
}

func TestBuildViewPartialReveal(t *testing.T) {
	// Reveal 3 lines from the top of the leading gap (gap 0, 9 hidden total).
	view := buildFor(t, map[int][2]int{0: {3, 0}}, 20)

	// The leading gap sits after the diff/---/+++ meta lines, so the revealed
	// context (numbered 1..3) starts at index 3, followed by the residual expand
	// row (6 still hidden).
	const base = 3
	for i := 0; i < 3; i++ {
		row := view[base+i]
		if row.Kind != Context || row.NewNum != i+1 || row.OldNum != i+1 {
			t.Fatalf("row %d = %+v, want context new/old %d", base+i, row, i+1)
		}
	}
	if row := view[base+3]; row.Kind != Expand || row.Hidden != 6 {
		t.Errorf("row %d = %+v, want expand with 6 hidden", base+3, row)
	}
}

func TestBuildViewFullyRevealedDropsButton(t *testing.T) {
	// Reveal the whole 9-line leading gap; its expand row must disappear, leaving
	// only the trailing gap's button.
	view := buildFor(t, map[int][2]int{0: {9, 0}}, 20)
	if got := countKind(view, Expand); got != 1 {
		t.Errorf("want 1 expand row after fully revealing gap 0, got %d", got)
	}
}

func TestBuildViewTwoButtonsWhenLarge(t *testing.T) {
	// With a tiny window, the 9-line leading gap exceeds it and shows both an
	// up and a down affordance.
	view := buildFor(t, map[int][2]int{}, 2)
	var up, down int
	for _, l := range view {
		if l.Kind != Expand {
			continue
		}
		switch l.Dir {
		case ExpandUp:
			up++
		case ExpandDown:
			down++
		}
	}
	if up < 1 || down < 1 {
		t.Errorf("want at least one up and one down affordance, got up=%d down=%d", up, down)
	}
}
