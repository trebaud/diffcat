package git

import "testing"

// record builds one `git log` record in the separator-framed format Commits asks
// for: fields joined by US (0x1f), terminated by RS (0x1e). The %D decoration
// field carries ref names (branches/tags); body is last so its newlines stay
// clear of the field separators.
func record(sha, short, author, email, date, parents, decoration, subject, body string) string {
	return sha + "\x1f" + short + "\x1f" + author + "\x1f" + email + "\x1f" + date + "\x1f" + parents + "\x1f" + decoration + "\x1f" + subject + "\x1f" + body + "\x1e"
}

func TestParseCommits(t *testing.T) {
	// git emits each record followed by a newline; the last one may lack it.
	blob := record("aaa111full", "aaa111", "Ada Lovelace", "ada@example.io", "2026-05-29", "bbb222full", "HEAD -> main, tag: v1.2.0, tag: v1.1.0", "Tighten README for end users", "Explain the why.\n\nSecond paragraph of the body.") + "\n" +
		record("ccc333full", "ccc333", "Grace Hopper", "grace@example.io", "2026-05-28", "ddd444 eee555", "", "Merge branch 'x' into main", "") + "\n" +
		record("fff666full", "fff666", "Alan Turing", "alan@example.io", "2026-05-27", "", "", "Root commit, no parent", "")

	got := parseCommits([]byte(blob), map[string]bool{"origin": true})
	if len(got) != 3 {
		t.Fatalf("got %d commits, want 3", len(got))
	}

	if got[0].SHA != "aaa111full" || got[0].Short != "aaa111" {
		t.Errorf("commit 0 sha/short = %q/%q", got[0].SHA, got[0].Short)
	}
	if got[0].Subject != "Tighten README for end users" {
		t.Errorf("commit 0 subject with spaces mangled: %q", got[0].Subject)
	}
	if got[0].Author != "Ada Lovelace" || got[0].AuthorEmail != "ada@example.io" || got[0].Date != "2026-05-29" {
		t.Errorf("commit 0 author/email/date = %q/%q/%q", got[0].Author, got[0].AuthorEmail, got[0].Date)
	}
	if got[0].Body != "Explain the why.\n\nSecond paragraph of the body." {
		t.Errorf("commit 0 multi-line body mangled: %q", got[0].Body)
	}
	if got[1].Body != "" {
		t.Errorf("commit 1 should have empty body, got %q", got[1].Body)
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

	// The decoration's "tag: " entries become Tags, in order; branches are ignored.
	if want := []string{"v1.2.0", "v1.1.0"}; len(got[0].Tags) != len(want) {
		t.Errorf("commit 0 tags = %v, want %v", got[0].Tags, want)
	} else {
		for i := range want {
			if got[0].Tags[i] != want[i] {
				t.Errorf("commit 0 tag[%d] = %q, want %q", i, got[0].Tags[i], want[i])
			}
		}
	}
	if len(got[1].Tags) != 0 {
		t.Errorf("commit 1 should have no tags, got %v", got[1].Tags)
	}

	// "HEAD -> main" becomes a local branch head; bare tag-only commits carry none.
	if want := []string{"main"}; len(got[0].Heads) != 1 || got[0].Heads[0] != want[0] {
		t.Errorf("commit 0 heads = %v, want %v", got[0].Heads, want)
	}
	if len(got[1].Heads) != 0 {
		t.Errorf("commit 1 should have no heads, got %v", got[1].Heads)
	}
}

// TestParseRefs covers the three ref kinds split out of a %D decoration: local
// branch heads (incl. the HEAD-pointed one and detached HEAD), remote-tracking
// refs, and tags — order preserved within each kind.
func TestParseRefs(t *testing.T) {
	remoteSet := map[string]bool{"origin": true}
	heads, remotes, tags := parseRefs("HEAD -> main, feature/x, origin/main, origin/HEAD, tag: v1.2.0, tag: v1.1.0", remoteSet)

	if want := []string{"main", "feature/x"}; !equalStrings(heads, want) {
		t.Errorf("heads = %v, want %v", heads, want)
	}
	if want := []string{"origin/main", "origin/HEAD"}; !equalStrings(remotes, want) {
		t.Errorf("remotes = %v, want %v", remotes, want)
	}
	if want := []string{"v1.2.0", "v1.1.0"}; !equalStrings(tags, want) {
		t.Errorf("tags = %v, want %v", tags, want)
	}

	// Detached HEAD: "HEAD" alone is a head, not a remote.
	if heads, _, _ := parseRefs("HEAD, origin/main", remoteSet); !equalStrings(heads, []string{"HEAD"}) {
		t.Errorf("detached HEAD heads = %v, want [HEAD]", heads)
	}

	// Empty decoration yields nothing.
	if h, r, tg := parseRefs("", remoteSet); len(h)+len(r)+len(tg) != 0 {
		t.Errorf("empty decoration = %v/%v/%v, want all empty", h, r, tg)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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
	if got := parseCommits(nil, nil); len(got) != 0 {
		t.Errorf("nil input: got %d commits, want 0", len(got))
	}
	if got := parseCommits([]byte("\n"), nil); len(got) != 0 {
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

// authorRec builds one authorship record in the framing parseAuthorship expects:
// a leading RS (0x1e), a header of author/email/co-authors joined by US (0x1f),
// then the commit's --numstat lines (each "added\tdeleted\tpath").
func authorRec(author, email, coauthors string, numstat ...string) string {
	rec := "\x1e" + author + "\x1f" + email + "\x1f" + coauthors
	for _, n := range numstat {
		rec += "\n" + n
	}
	return rec
}

func TestParseAuthorshipChurn(t *testing.T) {
	data := authorRec("Ada", "ada@x.io", "", "10\t2\tmain.go", "-\t-\tlogo.png", "3\t0\tutil.go") +
		"\n" + authorRec("Bob", "bob@x.io", "Claude <noreply@anthropic.com>", "5\t5\tapp.go")
	got := parseAuthorship([]byte(data))
	if len(got) != 2 {
		t.Fatalf("got %d commits, want 2", len(got))
	}
	// binary "-\t-" contributes 0; the rest sum added+deleted.
	if got[0].churn != 15 {
		t.Errorf("commit 0 churn = %d, want 15", got[0].churn)
	}
	if got[1].churn != 10 {
		t.Errorf("commit 1 churn = %d, want 10", got[1].churn)
	}
	if got[0].coauthors != "" || got[1].coauthors != "Claude <noreply@anthropic.com>" {
		t.Errorf("coauthors parsed wrong: %q / %q", got[0].coauthors, got[1].coauthors)
	}
}

func TestIsAIAuthored(t *testing.T) {
	cases := []struct {
		name string
		c    authorCommit
		want bool
	}{
		{"human", authorCommit{author: "Ada Lovelace", email: "ada@x.io"}, false},
		{"claude co-author", authorCommit{author: "Ada", email: "ada@x.io", coauthors: "Claude <noreply@anthropic.com>"}, true},
		{"copilot author", authorCommit{author: "Copilot", email: "bot@github.com"}, true},
		{"anthropic email", authorCommit{author: "Ada", email: "x@anthropic.com"}, true},
		{"unrelated bot name", authorCommit{author: "Roberto", email: "rob@x.io"}, false},
	}
	for _, tc := range cases {
		if got := tc.c.isAIAuthored(); got != tc.want {
			t.Errorf("%s: isAIAuthored() = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestCommitAIAgent(t *testing.T) {
	cases := []struct {
		name string
		c    Commit
		want string // the agent name, "" for human
	}{
		{"human", Commit{Author: "Ada Lovelace", AuthorEmail: "ada@x.io"}, ""},
		{"claude trailer", Commit{Author: "Ada", AuthorEmail: "ada@x.io", Body: "Fix nav\n\nCo-Authored-By: Claude <noreply@anthropic.com>"}, "Claude"},
		{"anthropic resolves to claude", Commit{Author: "Ada", Body: "x\n\nco-authored-by: bot <noreply@anthropic.com>"}, "Claude"},
		{"copilot author", Commit{Author: "Copilot", AuthorEmail: "bot@github.com"}, "Copilot"},
		// Prose mentioning a tool's name must not flag a human commit: "cursor"
		// here is the UI cursor, not the Cursor editor, and lives in the body, not
		// a trailer.
		{"marker word in prose", Commit{Author: "Ada", AuthorEmail: "ada@x.io", Body: "Keep the cursor on the diff when the sidebar collapses."}, ""},
	}
	for _, tc := range cases {
		if got := tc.c.AIAgent(); got != tc.want {
			t.Errorf("%s: AIAgent() = %q, want %q", tc.name, got, tc.want)
		}
		if got := tc.c.IsAIAuthored(); got != (tc.want != "") {
			t.Errorf("%s: IsAIAuthored() = %v, want %v", tc.name, got, tc.want != "")
		}
	}
}
