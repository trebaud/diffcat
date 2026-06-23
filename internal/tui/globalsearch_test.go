package tui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/trebaud/diffcat/internal/git"
)

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

// TestGlobalSearchByAuthor checks that a query matching only an author's name
// (not any subject, body, file, or code) still surfaces that author's commits.
func TestGlobalSearchByAuthor(t *testing.T) {
	m := logSampleModel()
	m.gsFiles = []git.FileChange{}
	m.gsCode = []gsCodeLine{}

	m.gsInput = "hopper" // only Grace Hopper's commit carries this
	results := m.gsResults()
	if countCat(results, gsCommit) != 1 {
		t.Fatalf("author query 'hopper' should match exactly one commit, got %d", countCat(results, gsCommit))
	}
	if results[0].sha != "bbb222full" {
		t.Errorf("author query should surface Grace Hopper's commit, got %q", results[0].sha)
	}
}

// TestGlobalSearchPagination checks that a result set larger than one page reports
// its page count and that the rendered overlay follows the selection onto the
// last page.
func TestGlobalSearchPagination(t *testing.T) {
	m := logSampleModel()
	m.commits = nil
	for i := 0; i < gsPageSize*2+1; i++ { // spans three pages
		m.commits = append(m.commits, git.Commit{
			SHA:     fmt.Sprintf("sha%02dfull", i),
			Short:   fmt.Sprintf("sha%02d", i),
			Author:  "Ada Lovelace",
			Date:    "2026-05-29",
			Subject: fmt.Sprintf("commit number %d", i),
		})
	}
	m.baseStart = len(m.commits)
	m.gsFiles = []git.FileChange{}
	m.gsCode = []gsCodeLine{}
	m.gsInput = "ada" // matches every commit's author
	m.width, m.height = 120, 40

	results := m.gsResults()
	if len(results) != gsPageSize*2+1 {
		t.Fatalf("want %d author matches, got %d", gsPageSize*2+1, len(results))
	}
	pageCount := (len(results) + gsPageSize - 1) / gsPageSize

	if out := m.globalSearchBox(); !strings.Contains(out, fmt.Sprintf("page 1/%d", pageCount)) {
		t.Errorf("first page should report page 1/%d:\n%s", pageCount, out)
	}
	// Selecting the last result should render the last page.
	m.gsSel = len(results) - 1
	if out := m.globalSearchBox(); !strings.Contains(out, fmt.Sprintf("page %d/%d", pageCount, pageCount)) {
		t.Errorf("selecting the last result should show page %d/%d", pageCount, pageCount)
	}
}
