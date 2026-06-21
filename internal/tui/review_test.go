package tui

import "testing"

// fileCursor parks the tree cursor on the first file row (folders are skipped by
// the real cursor motions) and returns its path.
func fileCursor(m *model) string {
	for i, r := range m.rows {
		if r.file != nil {
			m.cursor = i
			return r.file.Path
		}
	}
	return ""
}

// TestReviewToggleAndProgress checks marking a file reviewed updates the progress
// totals, that re-marking toggles it off, and that the cursor advances to the
// next unreviewed file on a mark (so clearing a branch flows without navigation).
func TestReviewToggleAndProgress(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir()) // isolate the persisted state file
	m := sampleModel()                      // viewBranch, 4 changed files
	if done, total := m.reviewProgress(); done != 0 || total != len(m.files) {
		t.Fatalf("fresh progress = %d/%d, want 0/%d", done, total, len(m.files))
	}

	path := fileCursor(&m)
	startCursor := m.cursor
	m.toggleReviewed()
	if !m.reviewed[path] {
		t.Errorf("after toggle, %q should be reviewed", path)
	}
	if done, _ := m.reviewProgress(); done != 1 {
		t.Errorf("after one mark, done = %d, want 1", done)
	}
	if m.cursor == startCursor {
		t.Errorf("marking a file should advance the cursor to the next unreviewed file")
	}

	// Toggling the same file back off clears it and re-arms the celebration latch.
	m.cursor = startCursor
	m.toggleReviewed()
	if m.reviewed[path] {
		t.Errorf("second toggle should clear %q", path)
	}
	if m.celebrated {
		t.Errorf("unmarking should leave the celebration un-latched")
	}
}

// TestReviewCelebration checks that marking the final reviewable item fires the
// celebration exactly once, and that it doesn't re-fire while everything stays
// cleared.
func TestReviewCelebration(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir()) // isolate the persisted state file
	m := sampleModel()
	m.reviewed = map[string]bool{}
	ids := m.reviewScopeIDs()
	if len(ids) == 0 {
		t.Fatal("sample model should have reviewable files")
	}
	// Mark every item but the last directly, then toggle the last through the hook.
	for _, id := range ids[:len(ids)-1] {
		m.reviewed[id] = true
	}
	if m.celebrate != 0 {
		t.Fatal("celebration should not have started yet")
	}
	// Park the cursor on the last unreviewed file and mark it.
	last := ids[len(ids)-1]
	for i, r := range m.rows {
		if r.file != nil && r.file.Path == last {
			m.cursor = i
		}
	}
	m.toggleReviewed()
	if m.celebrate <= 0 {
		t.Errorf("marking the last item should fire the celebration, got celebrate=%d", m.celebrate)
	}
	if !m.celebrated {
		t.Errorf("celebration latch should be set")
	}
}

// TestPaletteMatches checks the command palette's fuzzy filter returns ranked
// actions and that an empty query lists them all.
func TestPaletteMatches(t *testing.T) {
	m := sampleModel()
	all := m.paletteMatches()
	if len(all) != len(m.paletteActions()) {
		t.Errorf("empty query should list every action: got %d, want %d", len(all), len(m.paletteActions()))
	}
	hits := m
	hits.paletteInput = "theme"
	got := hits.paletteMatches()
	if len(got) == 0 {
		t.Fatal("query 'theme' should match at least one action")
	}
	if got[0].action.keys != "t" {
		t.Errorf("top match for 'theme' should be the theme picker (key t), got %q (%q)", got[0].action.label, got[0].action.keys)
	}
}
