package tui

import "testing"

// TestGlobalSearchGrouping checks that one query is matched across commits, files,
// and code, that the hits come back grouped in category order, and that each
// category's cap is respected. The search corpora are seeded directly so the test
// doesn't shell out to git.
func TestGlobalSearchGrouping(t *testing.T) {
	m := logSampleModel()
	m.gsFiles = m.files
	m.gsCode = []gsCodeLine{
		{path: "internal/tui/view.go", text: "renderEverything(width, height)"},
		{path: "old.txt", text: "the end of the file"},
	}

	// Empty query returns nothing — the overlay shows its prompt, not the repo.
	if got := m.gsResults(); got != nil {
		t.Errorf("empty query should return no results, got %d", len(got))
	}

	m.gsInput = "e"
	results := m.gsResults()
	if len(results) == 0 {
		t.Fatal("query 'e' should match across categories")
	}

	// Results must be grouped: every commit before every file before every code hit.
	var sawFile, sawCode bool
	for _, r := range results {
		switch r.cat {
		case gsCommit:
			if sawFile || sawCode {
				t.Error("commit hit appeared after a later category")
			}
		case gsFile:
			if sawCode {
				t.Error("file hit appeared after a code hit")
			}
			sawFile = true
		case gsCode:
			sawCode = true
		}
	}

	// At least one hit of each category should be present for this query.
	if countCat(results, gsCommit) == 0 {
		t.Error("expected a commit hit for 'e'")
	}
	if countCat(results, gsFile) == 0 {
		t.Error("expected a file hit for 'e'")
	}
	if countCat(results, gsCode) == 0 {
		t.Error("expected a code hit for 'e'")
	}

	// Caps are honoured.
	if n := countCat(results, gsCommit); n > gsCommitCap {
		t.Errorf("commit hits %d exceed cap %d", n, gsCommitCap)
	}
	if n := countCat(results, gsFile); n > gsFileCap {
		t.Errorf("file hits %d exceed cap %d", n, gsFileCap)
	}
	if n := countCat(results, gsCode); n > gsCodeCap {
		t.Errorf("code hits %d exceed cap %d", n, gsCodeCap)
	}
}
