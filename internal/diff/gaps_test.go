package diff

import "testing"

// gapNew is a compact view of a gap's new-side range for assertions.
type gapNew struct{ start, end int }

func newRanges(gaps []Gap) []gapNew {
	out := make([]gapNew, len(gaps))
	for i, g := range gaps {
		out[i] = gapNew{g.NewStart, g.NewEnd}
	}
	return out
}

func TestGaps(t *testing.T) {
	tests := []struct {
		name     string
		raw      string
		newLines int
		want     []gapNew
	}{
		{
			name: "leading and trailing",
			raw: "diff --git a/f b/f\n--- a/f\n+++ b/f\n" +
				"@@ -10,2 +10,3 @@\n context at 10\n+inserted\n context at 11\n",
			newLines: 20,
			// hidden new lines 1..9 before the hunk, 13..20 after it
			want: []gapNew{{1, 9}, {13, 20}},
		},
		{
			name: "new file has no gaps",
			raw: "diff --git a/f b/f\nnew file mode 100644\n--- /dev/null\n+++ b/f\n" +
				"@@ -0,0 +1,3 @@\n+one\n+two\n+three\n",
			newLines: 3,
			want:     nil,
		},
		{
			name:     "no hunks",
			raw:      "diff --git a/f b/f\nBinary files a/f and b/f differ\n",
			newLines: 10,
			want:     nil,
		},
		{
			name: "between two hunks",
			raw: "diff --git a/f b/f\n--- a/f\n+++ b/f\n" +
				"@@ -1,2 +1,2 @@\n a\n-b\n+B\n" +
				"@@ -10,2 +10,2 @@\n j\n-k\n+K\n",
			newLines: 15,
			// gap between hunks (new 3..9) and trailing (new 12..15)
			want: []gapNew{{3, 9}, {12, 15}},
		},
		{
			name: "section heading with +/- tokens does not corrupt the range",
			// git appends surrounding-context text after the second @@; here it
			// ends in "+" and contains "EP-001". Neither must be misread as a
			// range, which would zero the start and drop the leading gap.
			raw: "diff --git a/f b/f\n--- a/f\n+++ b/f\n" +
				"@@ -321,2 +321,3 @@ Each epic is demoable: EP-001 signup/login +\n ctx\n+inserted\n ctx\n",
			newLines: 330,
			// leading new 1..320 before the hunk, trailing new 324..330 after it
			want: []gapNew{{1, 320}, {324, 330}},
		},
		{
			name: "hunk ending on adds keeps old/new aligned",
			raw: "diff --git a/f b/f\n--- a/f\n+++ b/f\n" +
				"@@ -5,1 +5,3 @@\n ctx5\n+x\n+y\n" +
				"@@ -20,1 +22,1 @@\n ctx20\n",
			newLines: 30,
			// after first hunk lastNew=7, lastOld=5; gap new 8..21, trailing new 23..30
			want: []gapNew{{1, 4}, {8, 21}, {23, 30}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gaps := Gaps(Parse(tt.raw), tt.newLines)
			got := newRanges(gaps)
			if len(got) != len(tt.want) {
				t.Fatalf("got %d gaps %v, want %d %v", len(got), got, len(tt.want), tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("gap %d = %v, want %v", i, got[i], tt.want[i])
				}
			}
			// Every emitted gap must have equal-length old and new spans.
			for i, g := range gaps {
				if (g.OldEnd - g.OldStart) != (g.NewEnd - g.NewStart) {
					t.Errorf("gap %d old span %d..%d != new span %d..%d", i, g.OldStart, g.OldEnd, g.NewStart, g.NewEnd)
				}
			}
		})
	}
}
