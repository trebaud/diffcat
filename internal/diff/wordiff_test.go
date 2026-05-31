package diff

import "testing"

// emphOf returns the Emph ranges of the first line of the given kind.
func emphOf(lines []Line, kind Kind) [][2]int {
	for _, l := range lines {
		if l.Kind == kind {
			return l.Emph
		}
	}
	return nil
}

// substr extracts the runes a range covers from a line's text, so a test can
// assert on the changed word rather than bare offsets.
func substr(s string, r [2]int) string {
	rs := []rune(s)
	return string(rs[r[0]:r[1]])
}

func TestWordDiffMarksChangedWord(t *testing.T) {
	raw := "diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -1,1 +1,1 @@\n" +
		"-the quick brown fox\n+the slow brown fox\n"
	lines := Parse(raw)

	del := emphOf(lines, Del)
	add := emphOf(lines, Add)
	if len(del) != 1 || len(add) != 1 {
		t.Fatalf("want one emph range each, got del=%v add=%v", del, add)
	}
	if got := substr("the quick brown fox", del[0]); got != "quick" {
		t.Errorf("del emph = %q, want %q", got, "quick")
	}
	if got := substr("the slow brown fox", add[0]); got != "slow" {
		t.Errorf("add emph = %q, want %q", got, "slow")
	}
}

func TestWordDiffSkipsUnrelatedLines(t *testing.T) {
	// Two lines that share almost nothing should not be word-highlighted — the
	// flat full-line tint reads better than lighting both up end to end.
	raw := "diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -1,1 +1,1 @@\n" +
		"-alpha beta gamma\n+xx yy zz qq\n"
	lines := Parse(raw)
	if e := emphOf(lines, Del); e != nil {
		t.Errorf("expected no del emph for unrelated lines, got %v", e)
	}
	if e := emphOf(lines, Add); e != nil {
		t.Errorf("expected no add emph for unrelated lines, got %v", e)
	}
}

func TestWordDiffPureAddHasNoEmph(t *testing.T) {
	// A line with no removed counterpart has nothing to diff against.
	raw := "diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -1,1 +1,2 @@\n" +
		" context\n+brand new line\n"
	lines := Parse(raw)
	if e := emphOf(lines, Add); e != nil {
		t.Errorf("expected no emph on a pure addition, got %v", e)
	}
}

func TestWordDiffCoalescesAdjacentChanges(t *testing.T) {
	// Consecutive changed tokens collapse into one continuous range.
	raw := "diff --git a/f b/f\n--- a/f\n+++ b/f\n@@ -1,1 +1,1 @@\n" +
		"-value = compute(a, b)\n+value = compute(a, b, c)\n"
	lines := Parse(raw)
	add := emphOf(lines, Add)
	if len(add) != 1 {
		t.Fatalf("want a single coalesced range, got %v", add)
	}
	if got := substr("value = compute(a, b, c)", add[0]); got != ", c" {
		t.Errorf("add emph = %q, want %q", got, ", c")
	}
}
