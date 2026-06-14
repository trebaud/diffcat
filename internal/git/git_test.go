package git

import (
	"testing"
	"time"
)

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

// authorRec builds one commit record in the framing parseHistory expects: a
// leading RS (0x1e), a header of sha/date/author/email/co-authors joined by US
// (0x1f). date is a strict ISO-8601 author date (RFC3339); pass "" to simulate a
// commit with no parseable date.
func authorRec(sha, date, author, email, coauthors string) string {
	return "\x1e" + sha + "\x1f" + date + "\x1f" + author + "\x1f" + email + "\x1f" + coauthors
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

func TestParseHistory(t *testing.T) {
	// Four commits, newest-first (the order git log emits): Bob+Claude co-author
	// (classifies to Claude, not Bob), Bob, Ada, Ada.
	data := authorRec("sha4", "2026-01-04T10:00:00Z", "Bob", "bob@x.io", "Claude <noreply@anthropic.com>") + "\n" +
		authorRec("sha3", "2026-01-03T09:00:00Z", "Bob", "bob@x.io", "") + "\n" +
		authorRec("sha2", "2026-01-02T08:00:00Z", "Ada", "ada@x.io", "") + "\n" +
		authorRec("sha1", "2026-01-01T07:00:00Z", "Ada", "ada@x.io", "")
	got := parseHistory([]byte(data))

	if got.Commits != 4 {
		t.Errorf("Commits = %d, want 4", got.Commits)
	}

	// Ranked by commit count desc, ties by name: Ada(2) human, then Bob(1) human
	// and Claude(1) AI tie and sort by name. Each human keeps their own bucket.
	want := []AuthorShare{
		{Name: "Ada", Commits: 2, AI: false},
		{Name: "Bob", Commits: 1, AI: false},
		{Name: "Claude", Commits: 1, AI: true},
	}
	if len(got.Authors) != len(want) {
		t.Fatalf("got %d authors %v, want %d", len(got.Authors), got.Authors, len(want))
	}
	for i, w := range want {
		if got.Authors[i] != w {
			t.Errorf("Authors[%d] = %+v, want %+v", i, got.Authors[i], w)
		}
	}

	// Each ranked bucket also gets its own sub-stats keyed by name.
	if len(got.ByAuthor) != 3 {
		t.Fatalf("ByAuthor has %d entries, want 3", len(got.ByAuthor))
	}
	if ada, ok := got.ByAuthor["Ada"]; !ok || ada.Commits != 2 {
		t.Errorf("ByAuthor[Ada].Commits = %d (present=%v), want 2", ada.Commits, ok)
	}
	if claude, ok := got.ByAuthor["Claude"]; !ok || claude.Commits != 1 {
		t.Errorf("ByAuthor[Claude].Commits = %d (present=%v), want 1", claude.Commits, ok)
	}

	// Per-author SHA lists back the lazy module computation: Ada's two commits, and
	// the co-authored commit classifies to Claude's bucket (not Bob's).
	if ada := got.AuthorSHAs["Ada"]; len(ada) != 2 || ada[0] != "sha2" || ada[1] != "sha1" {
		t.Errorf("AuthorSHAs[Ada] = %v, want [sha2 sha1]", ada)
	}
	if claude := got.AuthorSHAs["Claude"]; len(claude) != 1 || claude[0] != "sha4" {
		t.Errorf("AuthorSHAs[Claude] = %v, want [sha4]", claude)
	}
	if bob := got.AuthorSHAs["Bob"]; len(bob) != 1 || bob[0] != "sha3" {
		t.Errorf("AuthorSHAs[Bob] = %v, want [sha3]", bob)
	}
}

func TestParseHistoryTimeSeries(t *testing.T) {
	// Same four commits as TestParseHistory: one AI (Claude) on Jan 4, humans on
	// Jan 1–3, spanning four contiguous days.
	data := authorRec("sha4", "2026-01-04T10:00:00Z", "Bob", "bob@x.io", "Claude <noreply@anthropic.com>") + "\n" +
		authorRec("sha3", "2026-01-03T09:00:00Z", "Bob", "bob@x.io", "") + "\n" +
		authorRec("sha2", "2026-01-02T08:00:00Z", "Ada", "ada@x.io", "") + "\n" +
		authorRec("sha1", "2026-01-01T07:00:00Z", "Ada", "ada@x.io", "")
	got := parseHistory([]byte(data))

	if want := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC); !got.Start.Equal(want) {
		t.Errorf("Start = %v, want %v", got.Start, want)
	}
	if want := time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC); !got.End.Equal(want) {
		t.Errorf("End = %v, want %v", got.End, want)
	}

	// Ada's sub-stats cover only her two days (Jan 1–2), not the repo's full span.
	ada := got.ByAuthor["Ada"]
	if want := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC); !ada.Start.Equal(want) {
		t.Errorf("ByAuthor[Ada].Start = %v, want %v", ada.Start, want)
	}
	if want := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC); !ada.End.Equal(want) {
		t.Errorf("ByAuthor[Ada].End = %v, want %v", ada.End, want)
	}
	if wantAda := []DayCount{{Human: 1}, {Human: 1}}; len(ada.Daily) != len(wantAda) {
		t.Errorf("ByAuthor[Ada].Daily = %v, want %v", ada.Daily, wantAda)
	}

	wantDaily := []DayCount{{Human: 1}, {Human: 1}, {Human: 1}, {AI: 1}}
	if len(got.Daily) != len(wantDaily) {
		t.Fatalf("Daily len = %d (%v), want %d", len(got.Daily), got.Daily, len(wantDaily))
	}
	for i, w := range wantDaily {
		if got.Daily[i] != w {
			t.Errorf("Daily[%d] = %+v, want %+v", i, got.Daily[i], w)
		}
	}

	// Punch must total every dated commit, placed at the commit's local hour.
	punchTotal := 0
	for _, row := range got.Punch {
		for _, n := range row {
			punchTotal += n
		}
	}
	if punchTotal != 4 {
		t.Errorf("punch total = %d, want 4", punchTotal)
	}
	// Jan 1 2026 is a Thursday; the 07:00Z human commit lands there.
	if got.Punch[int(time.Thursday)][7] != 1 {
		t.Errorf("Punch[Thu][7] = %d, want 1", got.Punch[int(time.Thursday)][7])
	}

	if !got.HasAI {
		t.Error("HasAI = false, want true")
	}
	if want := time.Date(2026, 1, 4, 10, 0, 0, 0, time.UTC); !got.FirstAI.Equal(want) {
		t.Errorf("FirstAI = %v, want %v", got.FirstAI, want)
	}
	if got.RecentTotal != 4 || got.RecentAI != 1 {
		t.Errorf("recent = %d/%d, want 1/4", got.RecentAI, got.RecentTotal)
	}
}

// A commit whose date doesn't parse still counts in the ranking but adds no
// time-series point — the series must stay consistent.
func TestParseHistoryUndatedCommit(t *testing.T) {
	data := authorRec("sha2", "", "Ada", "ada@x.io", "") + "\n" +
		authorRec("sha1", "2026-01-01T07:00:00Z", "Ada", "ada@x.io", "")
	got := parseHistory([]byte(data))
	if got.Commits != 2 {
		t.Errorf("Commits = %d, want 2", got.Commits)
	}
	if len(got.Daily) != 1 || got.Daily[0].Human != 1 {
		t.Errorf("Daily = %v, want one day with 1 human commit", got.Daily)
	}
	// The undated commit still belongs to the author's SHA list (it's a real commit,
	// just without a time-series point).
	if shas := got.AuthorSHAs["Ada"]; len(shas) != 2 {
		t.Errorf("AuthorSHAs[Ada] = %v, want 2 SHAs", shas)
	}
}

func TestModuleKey(t *testing.T) {
	cases := []struct{ path, want string }{
		{"internal/tui/overview.go", "internal/tui"},
		{"cmd/diffcat/main.go", "cmd/diffcat"},
		{"internal/git.go", "internal"},
		{"src/controllers/api/v1/user.go", "src/controllers/api/v1"},
		{"a/b/c/d/e/f/deep.go", "a/b/c/d"}, // capped at four directory segments
		{"README.md", ""},
		{"", ""},
	}
	for _, tc := range cases {
		if got := moduleKey(tc.path); got != tc.want {
			t.Errorf("moduleKey(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// parseAuthorModules sums added+deleted lines per area, descending, across the
// numstat rows of one contributor's commits — with binary files (numstat "-") and
// pure renames (0\t0) contributing nothing.
func TestParseAuthorModules(t *testing.T) {
	// `git log --numstat --pretty=format:` output for two of an author's commits:
	// only numstat rows and blank separators between commits.
	data := "10\t2\tinternal/tui/view.go\n" + // internal/tui: 12
		"3\t0\tinternal/git/git.go\n" + // internal/git: 3
		"-\t-\tassets/logo.png\n" + // binary → 0, must not rank
		"40\t5\tREADME.md\n" + // top-level file → no module, must not rank
		"\n" +
		"5\t1\tinternal/tui/update.go\n" + // internal/tui: +6 → 18 total
		"0\t0\tinternal/git/old.go\n" // pure rename/no-op → 0

	mods := parseAuthorModules([]byte(data))
	want := []ModuleCount{
		{Path: "internal/tui", Lines: 18},
		{Path: "internal/git", Lines: 3},
	}
	if len(mods) != len(want) {
		t.Fatalf("Modules = %+v, want %+v", mods, want)
	}
	for i, w := range want {
		if mods[i] != w {
			t.Errorf("Modules[%d] = %+v, want %+v", i, mods[i], w)
		}
	}

	if got := parseAuthorModules(nil); len(got) != 0 {
		t.Errorf("parseAuthorModules(nil) = %v, want empty", got)
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

// TestHistoryReductions checks the pure reductions the Stats dashboard charts
// draw from: hour-of-day and weekday rollups of the punch card, streak/cadence
// stats from the daily series, the human/AI split, and author concentration.
func TestHistoryReductions(t *testing.T) {
	start := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC) // a Thursday
	hs := HistoryStats{
		Commits: 10,
		Authors: []AuthorShare{
			{Name: "Claude", Commits: 6, AI: true},
			{Name: "Ada", Commits: 3},
			{Name: "Bob", Commits: 1},
		},
		Start: start,
		// Daily: a 3-day streak, a gap, then a 2-day streak ending the series.
		Daily: []DayCount{
			{Human: 1},
			{Human: 2, AI: 1}, // busiest day: 3 commits, index 1
			{Human: 1},
			{}, // gap
			{Human: 1},
			{AI: 1},
		},
	}
	// Punch: Monday(1) gets two commits at 09h, Wednesday(3) one at 14h.
	hs.Punch[1][9] = 2
	hs.Punch[3][14] = 1

	if h := hs.HourOfDay(); h[9] != 2 || h[14] != 1 || h[0] != 0 {
		t.Errorf("HourOfDay 9h/14h/0h = %d/%d/%d, want 2/1/0", h[9], h[14], h[0])
	}
	if w := hs.WeekdayTotals(); w[1] != 2 || w[3] != 1 || w[0] != 0 {
		t.Errorf("WeekdayTotals Mon/Wed/Sun = %d/%d/%d, want 2/1/0", w[1], w[3], w[0])
	}

	if longest, current := hs.Streaks(); longest != 3 || current != 2 {
		t.Errorf("Streaks = %d/%d, want longest 3, current 2", longest, current)
	}
	if day, count := hs.BusiestDay(); count != 3 || !day.Equal(start.AddDate(0, 0, 1)) {
		t.Errorf("BusiestDay = %s/%d, want 2026-01-02/3", day.Format("2006-01-02"), count)
	}
	if pw := hs.CommitsPerWeek(); pw != 10.0 { // 10 commits / (6 days -> clamped to 1 week)
		t.Errorf("CommitsPerWeek = %.2f, want 10", pw)
	}

	if human, ai := hs.HumanAICommits(); human != 4 || ai != 6 {
		t.Errorf("HumanAICommits = %d/%d, want 4/6", human, ai)
	}
	if top, bus := hs.Concentration(); top != 60 || bus != 1 {
		t.Errorf("Concentration = %d%%/%d, want 60%%/1", top, bus)
	}
}

// TestHistoryReductionsEmpty checks the reductions degrade cleanly on an empty
// history rather than panicking (e.g. dividing by a zero total).
func TestHistoryReductionsEmpty(t *testing.T) {
	var hs HistoryStats
	if h := hs.HourOfDay(); h != [24]int{} {
		t.Errorf("empty HourOfDay = %v, want zeroes", h)
	}
	if longest, current := hs.Streaks(); longest != 0 || current != 0 {
		t.Errorf("empty Streaks = %d/%d, want 0/0", longest, current)
	}
	if day, count := hs.BusiestDay(); !day.IsZero() || count != 0 {
		t.Errorf("empty BusiestDay = %s/%d, want zero/0", day, count)
	}
	if pw := hs.CommitsPerWeek(); pw != 0 {
		t.Errorf("empty CommitsPerWeek = %.2f, want 0", pw)
	}
	if top, bus := hs.Concentration(); top != 0 || bus != 0 {
		t.Errorf("empty Concentration = %d/%d, want 0/0", top, bus)
	}
}
