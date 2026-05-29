package tui

import (
	"testing"

	"github.com/trebaud/diff-master/internal/git"
)

func treeFiles() []git.FileChange {
	return []git.FileChange{
		{Path: "internal/tui/view.go", Status: "M", Added: 42, Deleted: 7},
		{Path: "internal/tui/model.go", Status: "M", Added: 5, Deleted: 1},
		{Path: "internal/git/git.go", Status: "M", Added: 3, Deleted: 0},
		{Path: "README.md", Status: "A", Added: 10, Deleted: 0},
		{Path: "assets/logo.png", Status: "A", Added: -1, Deleted: -1},
	}
}

func flatten(files []git.FileChange, collapsed map[string]bool) []treeRow {
	var rows []treeRow
	flattenTree(buildTree(files), collapsed, 0, nil, &rows)
	return rows
}

// TestTreeStructure checks the shape: dirs sort before files, single-child dir
// chains compress ("internal/git" not "internal" → "git"), and folders roll up
// their descendants' line counts.
func TestTreeStructure(t *testing.T) {
	rows := flatten(treeFiles(), map[string]bool{})

	// dirs first (assets, internal), then files (README.md).
	if rows[0].name != "assets" || !rows[0].isDir {
		t.Fatalf("row0 = %+v, want dir assets", rows[0])
	}
	if rows[1].name != "logo.png" || rows[1].depth != 1 {
		t.Fatalf("row1 = %+v, want logo.png at depth 1", rows[1])
	}
	if rows[2].name != "internal" || !rows[2].isDir {
		t.Fatalf("row2 = %+v, want dir internal", rows[2])
	}

	// internal has two children (git, tui) so it does NOT compress. Its first
	// child "git" has a single file, so "git" compresses with nothing (a dir with
	// one file stays a dir). Roll-up on internal = 42+7? counts: 42+5 add on tui,
	// 3 on git → added 50, deleted 8.
	if rows[2].added != 50 || rows[2].deleted != 8 {
		t.Errorf("internal roll-up = +%d -%d, want +50 -8", rows[2].added, rows[2].deleted)
	}

	// Last row is the top-level file README.md.
	last := rows[len(rows)-1]
	if last.name != "README.md" || last.isDir {
		t.Errorf("last row = %+v, want file README.md", last)
	}
}

// TestTreeCompressesChains verifies a deep single-child path collapses to one
// folder row showing the joined segments.
func TestTreeCompressesChains(t *testing.T) {
	files := []git.FileChange{{Path: "a/b/c/deep.go", Status: "A", Added: 1, Deleted: 0}}
	rows := flatten(files, map[string]bool{})
	if len(rows) != 2 {
		t.Fatalf("got %d rows, want 2 (one folder, one file)", len(rows))
	}
	if rows[0].name != "a/b/c" || !rows[0].isDir {
		t.Errorf("folder row = %q (dir=%v), want compressed a/b/c", rows[0].name, rows[0].isDir)
	}
	if rows[1].name != "deep.go" {
		t.Errorf("file row = %q, want deep.go", rows[1].name)
	}
}

// TestTreeCollapse hides a folder's children when its path is in the collapsed
// set, and the folder row reflects the folded state.
func TestTreeCollapse(t *testing.T) {
	full := flatten(treeFiles(), map[string]bool{})
	folded := flatten(treeFiles(), map[string]bool{"internal": true})

	if len(folded) >= len(full) {
		t.Fatalf("collapsing internal should drop rows: full=%d folded=%d", len(full), len(folded))
	}
	for _, r := range folded {
		if r.path == "internal" && !r.collapsed {
			t.Error("internal row should be marked collapsed")
		}
		if len(r.path) > len("internal/") && r.path[:9] == "internal/" {
			t.Errorf("collapsed folder leaked child row %q", r.path)
		}
	}
}
