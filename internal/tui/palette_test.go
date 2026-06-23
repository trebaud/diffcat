package tui

import "testing"

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
