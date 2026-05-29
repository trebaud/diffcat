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

func TestParseCommitsEmpty(t *testing.T) {
	if got := parseCommits(nil); len(got) != 0 {
		t.Errorf("nil input: got %d commits, want 0", len(got))
	}
	if got := parseCommits([]byte("\n")); len(got) != 0 {
		t.Errorf("blank input: got %d commits, want 0", len(got))
	}
}
