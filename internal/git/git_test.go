package git

import "testing"

// record builds one `git log` record in the separator-framed format Commits asks
// for: fields joined by US (0x1f), terminated by RS (0x1e).
func record(sha, short, author, date, parents, subject string) string {
	return sha + "\x1f" + short + "\x1f" + author + "\x1f" + date + "\x1f" + parents + "\x1f" + subject + "\x1e"
}

func TestParseCommits(t *testing.T) {
	// git emits each record followed by a newline; the last one may lack it.
	blob := record("aaa111full", "aaa111", "Ada Lovelace", "2026-05-29", "bbb222full", "Tighten README for end users") + "\n" +
		record("ccc333full", "ccc333", "Grace Hopper", "2026-05-28", "ddd444 eee555", "Merge branch 'x' into main") + "\n" +
		record("fff666full", "fff666", "Alan Turing", "2026-05-27", "", "Root commit, no parent")

	got := parseCommits([]byte(blob))
	if len(got) != 3 {
		t.Fatalf("got %d commits, want 3", len(got))
	}

	if got[0].SHA != "aaa111full" || got[0].Short != "aaa111" {
		t.Errorf("commit 0 sha/short = %q/%q", got[0].SHA, got[0].Short)
	}
	if got[0].Subject != "Tighten README for end users" {
		t.Errorf("commit 0 subject with spaces mangled: %q", got[0].Subject)
	}
	if got[0].Author != "Ada Lovelace" || got[0].Date != "2026-05-29" {
		t.Errorf("commit 0 author/date = %q/%q", got[0].Author, got[0].Date)
	}
	if len(got[0].Parents) != 1 || got[0].IsMerge() {
		t.Errorf("commit 0 should have one parent, got %v", got[0].Parents)
	}

	if len(got[1].Parents) != 2 || !got[1].IsMerge() {
		t.Errorf("commit 1 should be a merge with two parents, got %v", got[1].Parents)
	}

	if len(got[2].Parents) != 0 || got[2].IsMerge() {
		t.Errorf("root commit should have no parents, got %v", got[2].Parents)
	}
}

// TestMergeChanges covers the name-status + numstat join used by both
// ChangedFiles and CommitFiles: order preservation, rename new-paths, binary
// (-1) stats, and the pure-mode-change fallback (numstat missing → 0/0).
func TestMergeChanges(t *testing.T) {
	nameOut := "M\tinternal/tui/view.go\n" +
		"A\tdocs/new.md\n" +
		"R100\told/path.go\tnew/path.go\n" +
		"M\tassets/logo.png\n" +
		"M\tmode_only.txt\n"
	numOut := "12\t3\tinternal/tui/view.go\n" +
		"40\t0\tdocs/new.md\n" +
		"5\t5\tnew/path.go\n" +
		"-\t-\tassets/logo.png\n" // binary
	// mode_only.txt is intentionally absent from numstat.

	statusByPath, order := parseNameStatus([]byte(nameOut))
	stats := parseNumStat([]byte(numOut))
	got := mergeChanges(statusByPath, order, stats)

	if len(got) != 5 {
		t.Fatalf("got %d changes, want 5", len(got))
	}
	if got[0].Path != "internal/tui/view.go" || got[0].Status != "M" || got[0].Added != 12 || got[0].Deleted != 3 {
		t.Errorf("change 0 = %+v", got[0])
	}
	// Rename: name-status reports old\tnew; the new path wins, status first letter.
	if got[2].Path != "new/path.go" || got[2].Status != "R" {
		t.Errorf("rename change = %+v", got[2])
	}
	if !got[3].Binary() {
		t.Errorf("logo.png should be binary, got %+v", got[3])
	}
	if got[4].Path != "mode_only.txt" || got[4].Added != 0 || got[4].Deleted != 0 || got[4].Binary() {
		t.Errorf("mode-only change should default to 0/0 (not binary), got %+v", got[4])
	}
}

func TestParseCommitsEmpty(t *testing.T) {
	if got := parseCommits(nil); len(got) != 0 {
		t.Errorf("nil input: got %d commits, want 0", len(got))
	}
	if got := parseCommits([]byte("\n")); len(got) != 0 {
		t.Errorf("blank input: got %d commits, want 0", len(got))
	}
}

func TestStatusPaths(t *testing.T) {
	porcelain := " M internal/git/git.go\n" + // unstaged modification
		"A  cmd/diffcat/new.go\n" + // staged add
		"?? notes.txt\n" + // untracked
		"R  old/path.go -> new/path.go\n" + // rename: new path wins
		"\n" // trailing blank line is ignored

	got := statusPaths([]byte(porcelain))
	want := []string{"internal/git/git.go", "cmd/diffcat/new.go", "notes.txt", "new/path.go"}
	if len(got) != len(want) {
		t.Fatalf("got %d paths %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("path[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
