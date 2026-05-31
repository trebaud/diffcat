package tui

import "testing"

// TestFuzzyFilterEmptyQuery: an empty query returns every path, in order.
func TestFuzzyFilterEmptyQuery(t *testing.T) {
	paths := []string{"b.go", "a.go", "c.go"}
	got := fuzzyFilter(paths, "")
	if len(got) != len(paths) {
		t.Fatalf("empty query returned %d matches, want %d", len(got), len(paths))
	}
	for i, m := range got {
		if m.path != paths[i] {
			t.Errorf("empty query reordered: got %q at %d, want %q", m.path, i, paths[i])
		}
	}
}

// TestFuzzyFilterSubsequence: only paths containing the query runes in order
// match; others are dropped.
func TestFuzzyFilterSubsequence(t *testing.T) {
	paths := []string{"internal/tui/view.go", "internal/git/git.go", "README.md"}
	got := fuzzyFilter(paths, "view")
	if len(got) != 1 || got[0].path != "internal/tui/view.go" {
		t.Fatalf("query 'view' matched %+v, want only view.go", got)
	}
	if len(fuzzyFilter(paths, "zzz")) != 0 {
		t.Errorf("query 'zzz' should match nothing")
	}
}

// TestFuzzyFilterBasenameBoost: a query matching the filename outranks one that
// only matches in a directory segment.
func TestFuzzyFilterBasenameBoost(t *testing.T) {
	paths := []string{
		"model/internal.go", // "model" matches in the directory
		"internal/model.go", // "model" matches the basename — should rank higher
	}
	got := fuzzyFilter(paths, "model")
	if len(got) != 2 {
		t.Fatalf("both paths should match, got %d", len(got))
	}
	if got[0].path != "internal/model.go" {
		t.Errorf("basename match should rank first, got %q", got[0].path)
	}
}

// TestFuzzyFilterCaseInsensitive: matching ignores case, and the hit offsets
// point at the matched runes.
func TestFuzzyFilterCaseInsensitive(t *testing.T) {
	got := fuzzyFilter([]string{"View.GO"}, "vg")
	if len(got) != 1 {
		t.Fatalf("case-insensitive query should match, got %d", len(got))
	}
	if len(got[0].hit) != 2 {
		t.Errorf("expected 2 hit offsets, got %v", got[0].hit)
	}
}

// TestJumpToFileExpandsAncestors: jumping to a file inside a collapsed folder
// unfolds the ancestor and lands the cursor on the file row.
func TestJumpToFileExpandsAncestors(t *testing.T) {
	m := sampleModel()
	// Collapse the folder holding the deep file, then jump to it.
	m.collapsed["very/deep/nested/path/that/should/truncate"] = true
	m.rebuildTree()
	m.jumpToFile("very/deep/nested/path/that/should/truncate/handler.go")
	r := m.selectedRow()
	if r == nil || r.file == nil || r.file.Path != "very/deep/nested/path/that/should/truncate/handler.go" {
		t.Fatalf("jumpToFile did not land on the file row, got %+v", r)
	}
	if m.focus != focusDiff {
		t.Errorf("jumpToFile should focus the diff pane")
	}
}
