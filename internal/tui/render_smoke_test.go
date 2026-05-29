package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/trebaud/diff-master/internal/diff"
	"github.com/trebaud/diff-master/internal/git"
)

const sampleRaw = `diff --git a/view.go b/view.go
index 1111111..2222222 100644
--- a/view.go
+++ b/view.go
@@ -10,5 +10,6 @@ func main() {
 a long context line that definitely exceeds a narrow diff pane width by a lot
-removed line one
-removed line two
+added line one
+added line two
+added line three
 trailing context
`

func sampleModel() model {
	m := model{
		baseName:  "master",
		branch:    "feature/long-branch-name",
		shortstat: "3 files changed, 42 insertions(+), 7 deletions(-)",
		files: []git.FileChange{
			{Path: "internal/tui/view.go", Status: "M", Added: 42, Deleted: 7},
			{Path: "very/deep/nested/path/that/should/truncate/handler.go", Status: "A", Added: 120, Deleted: 0},
			{Path: "assets/logo.png", Status: "A", Added: -1, Deleted: -1},
			{Path: "old.txt", Status: "D", Added: 0, Deleted: 9},
		},
		focus: focusFiles,
	}
	m.rebuildTree()
	// Park the cursor on a file row (not a folder) so the diff pane renders a
	// real diff in the focused-diff tests.
	for i, r := range m.rows {
		if r.file != nil {
			m.cursor = i
			break
		}
	}
	m.diff = diff.Parse(sampleRaw)
	m.splitRows = diff.SplitRows(m.diff)
	m.lineDigits = lineDigits(m.diff)
	return m
}

// TestRenderNoWrap guards the invariant that no rendered line is wider than the
// terminal — a line that overflows wraps and shoves the whole layout down.
func TestRenderNoWrap(t *testing.T) {
	m := sampleModel()
	for _, sz := range [][2]int{{200, 50}, {120, 40}, {100, 18}, {80, 24}, {60, 12}} {
		m.width, m.height = sz[0], sz[1]
		for i, line := range strings.Split(m.render(), "\n") {
			if w := lipgloss.Width(line); w > m.width {
				t.Errorf("%dx%d line %d width %d exceeds %d: %q", sz[0], sz[1], i, w, m.width, line)
			}
		}
	}
}

// TestFullScreenFill guards that the TUI occupies the entire terminal: exactly
// one rendered line per row, and every line spans the full width — in both the
// unified and side-by-side diff modes, with the diff pane focused so its rows
// are exercised.
func TestFullScreenFill(t *testing.T) {
	m := sampleModel()
	m.focus = focusDiff
	for _, split := range []bool{false, true} {
		m.splitView = split
		for _, sz := range [][2]int{{200, 50}, {120, 40}, {100, 24}, {80, 24}, {70, 16}, {60, 12}} {
			m.width, m.height = sz[0], sz[1]
			lines := strings.Split(m.render(), "\n")
			if len(lines) != sz[1] {
				t.Errorf("split=%v %dx%d: %d lines, want %d", split, sz[0], sz[1], len(lines), sz[1])
			}
			for i, line := range lines {
				if w := lipgloss.Width(line); w != sz[0] {
					t.Errorf("split=%v %dx%d line %d width %d, want %d", split, sz[0], sz[1], i, w, sz[0])
				}
			}
		}
	}
}

// TestTooSmallGate verifies the resize hint replaces the body below the minimum.
func TestTooSmallGate(t *testing.T) {
	m := sampleModel()
	m.width, m.height = 40, 8
	if out := m.render(); !strings.Contains(out, "too small") {
		t.Errorf("expected resize hint below minimum size, got:\n%s", out)
	}
}
