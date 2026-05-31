package tui

import (
	"strings"
	"testing"

	"charm.land/lipgloss/v2"

	"github.com/trebaud/diffcat/internal/diff"
	"github.com/trebaud/diffcat/internal/git"
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
		baseName:      "master",
		baseIsDefault: true,
		branch:        "feature/long-branch-name",
		shortstat:     "3 files changed, 42 insertions(+), 7 deletions(-)",
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
	// A working-tree file longer than the hunk so there are hidden regions both
	// before (lines 1..9) and after the change — exercising the Expand rows.
	fileLines := make([]string, 30)
	for i := range fileLines {
		fileLines[i] = "a context line of source code that is quite long, definitely wider than a narrow pane"
	}
	m.fileLines = fileLines
	m.gaps = diff.Gaps(m.diff, len(fileLines))
	m.revealed = map[int][2]int{}
	m.rebuildView()
	return m
}

// logSampleModel is sampleModel switched into the commit-history view with a few
// injected commits (one normal, one merge, one root) and a long subject to
// exercise truncation. The right pane reuses the parsed sample diff.
func logSampleModel() model {
	m := sampleModel()
	m.mode = viewLog
	m.commits = []git.Commit{
		{SHA: "aaa111full", Short: "aaa1111", Author: "Ada Lovelace", Date: "2026-05-29", Parents: []string{"p1"}, Subject: "Tighten README for end users with a subject long enough to need truncation in a narrow pane"},
		{SHA: "bbb222full", Short: "bbb2222", Author: "Grace Hopper", Date: "2026-05-28", Parents: []string{"p1", "p2"}, Subject: "Merge branch 'feature' into main"},
		{SHA: "ccc333full", Short: "ccc3333", Author: "Alan Turing", Date: "2026-05-27", Parents: nil, Subject: "Initial commit"},
	}
	m.commitCursor = 0
	m.commitDiffCache = map[string][]diff.Line{}
	// Reuse the diff sampleModel already parsed as the highlighted commit's diff.
	m.commitDiffCache["aaa111full"] = m.diff
	m.rebuildView()
	return m
}

// commitSampleModel is sampleModel drilled into a single commit's file tree
// (viewCommit): the file tree is reused as the commit's changed files and the
// right pane shows the parsed sample diff scoped to that commit.
func commitSampleModel() model {
	m := sampleModel()
	m.mode = viewCommit
	m.scopeCommit = &git.Commit{
		SHA:     "aaa111full",
		Short:   "aaa1111",
		Subject: "Tighten README for end users with a subject long enough to need truncation",
	}
	return m
}

// TestRenderNoWrap guards the invariant that no rendered line is wider than the
// terminal — a line that overflows wraps and shoves the whole layout down.
func TestRenderNoWrap(t *testing.T) {
	emptyLog := logSampleModel()
	emptyLog.commits = nil // exercise the "no commits" empty state too
	workingLog := logSampleModel()
	workingLog.logWorking = true // exercise the pinned working-tree row + cursor on it
	workingCommit := commitSampleModel()
	workingCommit.scopeCommit, workingCommit.scopeWorking = nil, true // working-tree drill-in
	emptyCommit := commitSampleModel()
	emptyCommit.files, emptyCommit.rows = nil, nil // "no files in this commit" state
	emptyWorking := commitSampleModel()
	emptyWorking.scopeCommit, emptyWorking.scopeWorking = nil, true
	emptyWorking.files, emptyWorking.rows = nil, nil // "no uncommitted changes" state
	shimmer := sampleModel()                         // every file flagged unseen → rainbow-shimmer tree rows
	shimmer.unseen = map[string]bool{}
	for _, f := range shimmer.files {
		shimmer.unseen[f.Path] = true
	}
	shimmer.animFrame = 5 // a mid-cycle frame so the sparkle/wave is actually styled
	for _, m := range []model{sampleModel(), logSampleModel(), workingLog, emptyLog, commitSampleModel(), workingCommit, emptyCommit, emptyWorking, shimmer} {
		for _, sz := range [][2]int{{200, 50}, {120, 40}, {100, 18}, {80, 24}, {60, 12}} {
			m.width, m.height = sz[0], sz[1]
			for i, line := range strings.Split(m.render(), "\n") {
				if w := lipgloss.Width(line); w > m.width {
					t.Errorf("mode=%d %dx%d line %d width %d exceeds %d: %q", m.mode, sz[0], sz[1], i, w, m.width, line)
				}
			}
		}
	}
}

// TestFullScreenFill guards that the TUI occupies the entire terminal: exactly
// one rendered line per row, and every line spans the full width — in both the
// unified and side-by-side diff modes, with the diff pane focused so its rows
// are exercised.
func TestFullScreenFill(t *testing.T) {
	workingLog := logSampleModel()
	workingLog.logWorking = true
	for _, m := range []model{sampleModel(), logSampleModel(), workingLog, commitSampleModel()} {
		m.focus = focusDiff
		for _, split := range []bool{false, true} {
			m.splitView = split
			for _, sz := range [][2]int{{200, 50}, {120, 40}, {100, 24}, {80, 24}, {70, 16}, {60, 12}} {
				m.width, m.height = sz[0], sz[1]
				lines := strings.Split(m.render(), "\n")
				if len(lines) != sz[1] {
					t.Errorf("mode=%d split=%v %dx%d: %d lines, want %d", m.mode, split, sz[0], sz[1], len(lines), sz[1])
				}
				for i, line := range lines {
					if w := lipgloss.Width(line); w != sz[0] {
						t.Errorf("mode=%d split=%v %dx%d line %d width %d, want %d", m.mode, split, sz[0], sz[1], i, w, sz[0])
					}
				}
			}
		}
	}
}

// TestLogModeNavigation checks the history view's cursor movement, merge
// detection, clamping, and that leaving restores the branch view.
func TestLogModeNavigation(t *testing.T) {
	m := logSampleModel()
	if m.mode != viewLog {
		t.Fatalf("logSampleModel should be in viewLog, got %d", m.mode)
	}
	if c := m.selectedCommit(); c == nil || c.Short != "aaa1111" {
		t.Fatalf("first selected commit = %+v", c)
	}

	m.moveCommitCursor(1)
	if m.commitCursor != 1 {
		t.Errorf("after move down, cursor = %d, want 1", m.commitCursor)
	}
	if c := m.selectedCommit(); c == nil || !c.IsMerge() {
		t.Errorf("commit at index 1 should be a merge, got %+v", c)
	}

	m.moveCommitCursor(99)
	if m.commitCursor != len(m.commits)-1 {
		t.Errorf("cursor should clamp to last (%d), got %d", len(m.commits)-1, m.commitCursor)
	}
	m.moveCommitCursor(-99)
	if m.commitCursor != 0 {
		t.Errorf("cursor should clamp to 0, got %d", m.commitCursor)
	}

	m.exitLog()
	if m.mode != viewBranch {
		t.Errorf("exitLog should return to viewBranch, got %d", m.mode)
	}
}

// TestCommitDrillInRestore checks that leaving a per-commit tree restores the
// stashed branch tree verbatim: the same files, rows, and cursor position the
// branch view had before drilling in, with the commit scope cleared.
func TestCommitDrillInRestore(t *testing.T) {
	m := logSampleModel()

	// Capture the branch tree as it was before drilling in (logSampleModel was
	// built from sampleModel, so it carries a real file tree underneath).
	wantFiles := m.files
	wantRows := m.rows
	wantCursor := m.cursor

	// Simulate enterCommit's effect without shelling out to git: stash the branch
	// tree, then repurpose the shared fields for the commit's (different) files.
	m.branchFiles, m.branchRows, m.branchCursor = m.files, m.rows, m.cursor
	m.scopeCommit = &m.commits[0]
	m.files = []git.FileChange{{Path: "only/in/commit.go", Status: "A", Added: 3, Deleted: 0}}
	m.cursor = 0
	m.rebuildTree()
	m.mode = viewCommit

	if f := m.selectedFile(); f == nil && len(m.files) > 0 {
		// Park on the file row (the compressed "only/in" chain puts a folder first).
		for i, r := range m.rows {
			if r.file != nil {
				m.cursor = i
				break
			}
		}
	}
	if len(m.files) != 1 || m.files[0].Path != "only/in/commit.go" {
		t.Fatalf("commit tree not loaded, files = %+v", m.files)
	}

	m.exitCommit()

	if m.mode != viewLog {
		t.Errorf("exitCommit should return to viewLog, got %d", m.mode)
	}
	if m.scopeCommit != nil {
		t.Errorf("scopeCommit should be cleared, got %+v", m.scopeCommit)
	}
	if len(m.files) != len(wantFiles) || m.cursor != wantCursor {
		t.Errorf("branch tree not restored: files=%d (want %d), cursor=%d (want %d)",
			len(m.files), len(wantFiles), m.cursor, wantCursor)
	}
	if len(m.rows) != len(wantRows) {
		t.Errorf("branch rows not restored: got %d, want %d", len(m.rows), len(wantRows))
	}
}

// TestLogWorkingTreeRow checks that the pinned working-tree row shifts the
// commit indexing by one: row 0 is the working tree (no commit), and the
// commits follow beneath it.
func TestLogWorkingTreeRow(t *testing.T) {
	m := logSampleModel()
	m.logWorking = true

	if !m.onWorkingRow() {
		t.Fatalf("cursor 0 with a working row should be the working row")
	}
	if c := m.selectedCommit(); c != nil {
		t.Errorf("working row should select no commit, got %+v", c)
	}
	if got, want := m.logRowCount(), len(m.commits)+1; got != want {
		t.Errorf("logRowCount = %d, want %d", got, want)
	}

	m.moveCommitCursor(1)
	if m.onWorkingRow() {
		t.Errorf("after one move down the cursor should be off the working row")
	}
	if c := m.selectedCommit(); c == nil || c.Short != "aaa1111" {
		t.Errorf("row 1 should be the first commit, got %+v", c)
	}

	// Clamp at the bottom: the last row is the last commit, not past it.
	m.moveCommitCursor(99)
	if m.commitCursor != m.logRowCount()-1 {
		t.Errorf("cursor should clamp to last row %d, got %d", m.logRowCount()-1, m.commitCursor)
	}
	if c := m.selectedCommit(); c == nil || c.Short != "ccc3333" {
		t.Errorf("last row should be the last commit, got %+v", c)
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
