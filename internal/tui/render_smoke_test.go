package tui

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"charm.land/lipgloss/v2"

	"github.com/trebaud/diffcat/internal/diff"
	"github.com/trebaud/diffcat/internal/git"
)

// sampleHistory is a populated HistoryStats for the Stats dashboard: a per-author
// ranking plus the time-series the AI-adoption curve, activity timeline, and
// punch-card heatmap draw from — so the render invariants exercise those sections.
func sampleHistory() git.HistoryStats {
	daily := make([]git.DayCount, 200)
	var punch [7][24]int
	for i := range daily {
		daily[i].Human = i%5 + 1
		if i > 120 { // AI ramps up in the back half of the history
			daily[i].AI = (i - 120) % 7
		}
		punch[i%7][i%24] += daily[i].Human + daily[i].AI
	}
	// Enough authors (with one very long name) that the ranking scrolls on a short
	// terminal — exercising the scroll-offset window and the range heading.
	authors := []git.AuthorShare{
		{Name: "Claude", Commits: 82, AI: true},
		{Name: "Ada Lovelace", Commits: 31},
		{Name: "A Contributor With A Very Long Display Name", Commits: 18},
	}
	for i := 0; i < 20; i++ {
		authors = append(authors, git.AuthorShare{Name: fmt.Sprintf("contributor-%d", i), Commits: 20 - i})
	}
	// Per-author sub-stats for the contributor detail page: their own (shorter) daily
	// series, a punch card, and a module ranking. Claude (AI) and Ada (human) exercise
	// both badges; the long-named contributor exercises the card/heatmap truncation.
	authorDaily := make([]git.DayCount, 60)
	var authorPunch [7][24]int
	for i := range authorDaily {
		authorDaily[i].Human = i%4 + 1
		authorPunch[i%7][(i*3)%24] += authorDaily[i].Human
	}
	mkAuthor := func(commits int, ai bool, mods []git.ModuleCount) git.HistoryStats {
		d := authorDaily
		if ai {
			d = make([]git.DayCount, len(authorDaily))
			for i := range d {
				d[i].AI = authorDaily[i].Human
			}
		}
		return git.HistoryStats{
			Commits: commits,
			Start:   time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
			End:     time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC),
			Daily:   d,
			Punch:   authorPunch,
			Modules: mods,
		}
	}
	byAuthor := map[string]git.HistoryStats{
		"Claude": mkAuthor(82, true, []git.ModuleCount{
			{Path: "internal/tui", Lines: 4200},
			{Path: "internal/git", Lines: 1800},
			{Path: "cmd/diffcat", Lines: 300},
			{Path: "scripts", Lines: 90},
		}),
		"Ada Lovelace": mkAuthor(31, false, []git.ModuleCount{
			{Path: "internal/git", Lines: 900},
			{Path: "a/very/long/module/path/that/truncates", Lines: 120},
		}),
		"A Contributor With A Very Long Display Name": mkAuthor(18, false, nil),
	}
	// A few commit SHAs per author back the lazy module computation (ensureAuthorModules
	// only kicks the load when the author has SHAs).
	authorSHAs := map[string][]string{
		"Claude":       {"aaa111", "bbb222", "ccc333"},
		"Ada Lovelace": {"ddd444", "eee555"},
		"A Contributor With A Very Long Display Name": {"fff666"},
	}
	return git.HistoryStats{
		Commits:     137,
		Authors:     authors,
		Start:       time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		End:         time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC),
		Daily:       daily,
		Punch:       punch,
		FirstAI:     time.Date(2026, 3, 12, 9, 0, 0, 0, time.UTC),
		HasAI:       true,
		RecentAI:    11,
		RecentTotal: 30,
		ByAuthor:    byAuthor,
		AuthorSHAs:  authorSHAs,
	}
}

const sampleRaw = `diff --git a/view.go b/view.go
index 1111111..2222222 100644
--- a/view.go
+++ b/view.go
@@ -10,5 +10,6 @@ func main() {
 a long context line that definitely exceeds a narrow diff pane width by a lot
-removed line one
-removed line two
+added line one
+added line two
+added line three
 trailing context
`

func sampleModel() model {
	m := model{
		sidebar:       sidebarNormal,
		baseName:      "master",
		baseIsDefault: true,
		branch:        "feature/long-branch-name",
		shortstat:     "3 files changed, 42 insertions(+), 7 deletions(-)",
		files: []git.FileChange{
			{Path: "internal/tui/view.go", Status: "M", Added: 42, Deleted: 7},
			{Path: "very/deep/nested/path/that/should/truncate/handler.go", Status: "A", Added: 120, Deleted: 0},
			{Path: "assets/logo.png", Status: "A", Added: -1, Deleted: -1},
			{Path: "old.txt", Status: "D", Added: 0, Deleted: 9},
		},
		focus: focusFiles,
	}
	m.rebuildTree()
	// Park the cursor on a file row (not a folder) so the diff pane renders a
	// real diff in the focused-diff tests.
	for i, r := range m.rows {
		if r.file != nil {
			m.cursor = i
			break
		}
	}
	m.diff = diff.Parse(sampleRaw)
	// A working-tree file longer than the hunk so there are hidden regions both
	// before (lines 1..9) and after the change — exercising the Expand rows.
	fileLines := make([]string, 30)
	for i := range fileLines {
		fileLines[i] = "a context line of source code that is quite long, definitely wider than a narrow pane"
	}
	m.fileLines = fileLines
	m.gaps = diff.Gaps(m.diff, len(fileLines))
	m.revealed = map[int][2]int{}
	m.rebuildView()
	return m
}

// logSampleModel is sampleModel switched into the commit-history view with a few
// injected commits (one normal, one merge, one root) and a long subject to
// exercise truncation. The right pane reuses the parsed sample diff.
func logSampleModel() model {
	m := sampleModel()
	m.mode = viewLog
	m.commits = []git.Commit{
		{SHA: "aaa111full", Short: "aaa1111", Author: "Ada Lovelace", Date: "2026-05-29", Parents: []string{"p1"}, Subject: "Tighten README for end users with a subject long enough to need truncation in a narrow pane"},
		{SHA: "bbb222full", Short: "bbb2222", Author: "Grace Hopper", Date: "2026-05-28", Parents: []string{"p1", "p2"}, Subject: "Merge branch 'feature' into main"},
		{SHA: "ccc333full", Short: "ccc3333", Author: "Alan Turing", Date: "2026-05-27", Parents: nil, Subject: "Initial commit"},
	}
	m.commitCursor = 0
	m.commitDiffCache = map[string][]diff.Line{}
	// Reuse the diff sampleModel already parsed as the highlighted commit's diff.
	m.commitDiffCache["aaa111full"] = m.diff
	m.rebuildView()
	return m
}

// commitSampleModel is sampleModel drilled into a single commit's file tree
// (viewCommit): the file tree is reused as the commit's changed files and the
// right pane shows the parsed sample diff scoped to that commit.
func commitSampleModel() model {
	m := sampleModel()
	m.mode = viewCommit
	m.scopeCommit = &git.Commit{
		SHA:     "aaa111full",
		Short:   "aaa1111",
		Subject: "Tighten README for end users with a subject long enough to need truncation",
	}
	return m
}

// detailSampleModel is sampleModel switched into a contributor's Stats detail page
// (viewAuthorDetail) for the named author, which must be present in sampleHistory's
// ByAuthor map. The lazy module cache is seeded as if the background compute already
// landed, so the "Top modules" heatmap renders.
func detailSampleModel(name string) model {
	m := sampleModel()
	m.mode = viewAuthorDetail
	m.commits = logSampleModel().commits
	m.historyComputed = true
	m.historyStats = sampleHistory()
	m.detailAuthor = name
	m.authorModules = map[string][]git.ModuleCount{name: m.historyStats.ByAuthor[name].Modules}
	m.authorModulesComputing = map[string]bool{}
	return m
}

// TestRenderNoWrap guards the invariant that no rendered line is wider than the
// terminal — a line that overflows wraps and shoves the whole layout down.
func TestRenderNoWrap(t *testing.T) {
	emptyLog := logSampleModel()
	emptyLog.commits = nil // exercise the "no commits" empty state too
	workingLog := logSampleModel()
	workingLog.logWorking = true // exercise the pinned working-tree row + cursor on it
	workingCommit := commitSampleModel()
	workingCommit.scopeCommit, workingCommit.scopeWorking = nil, true // working-tree drill-in
	emptyCommit := commitSampleModel()
	emptyCommit.files, emptyCommit.rows = nil, nil // "no files in this commit" state
	emptyWorking := commitSampleModel()
	emptyWorking.scopeCommit, emptyWorking.scopeWorking = nil, true
	emptyWorking.files, emptyWorking.rows = nil, nil // "no uncommitted changes" state
	shimmer := sampleModel()                         // every file flagged unseen → rainbow-shimmer tree rows
	shimmer.unseen = map[string]bool{}
	for _, f := range shimmer.files {
		shimmer.unseen[f.Path] = true
	}
	shimmer.animFrame = 5 // a mid-cycle frame so the sparkle/wave is actually styled
	// The commit-details modal over a commit with a long email and a multi-line
	// body that must word-wrap rather than overflow the box.
	details := logSampleModel()
	details.commits[0].AuthorEmail = "ada.lovelace.with.a.very.long.email.address@example-domain.io"
	details.commits[0].Body = "This commit message body is intentionally quite long and contains a supercalifragilisticexpialidociousunbreakableword to exercise the hard-break path.\n\nA second paragraph follows the blank line and keeps going past the bottom of a short terminal so the scroll window is exercised too."
	details.showCommitDetails = true
	detailsScrolled := details
	detailsScrolled.detailsScroll = 99 // past the end → clamps
	// The whole-repo Stats (per-author commit ranking). Its data is the background
	// git.History result, so populate historyStats and mark it computed; otherwise
	// it renders the loading placeholder (overviewLoading below). Several authors
	// (one AI, several humans, plus a long name) exercise the ranking layout.
	overview := sampleModel()
	overview.mode = viewOverview
	overview.commits = logSampleModel().commits
	overview.historyComputed = true
	overview.historyStats = sampleHistory()
	overviewScrolled := overview
	overviewScrolled.overviewScroll = 999 // clamps to the last page
	overviewEmpty := overview
	overviewEmpty.historyStats = git.HistoryStats{} // "no commits" state
	// The Stats while their background computation is still in flight.
	overviewLoading := overview
	overviewLoading.historyComputed = false
	// Contributor detail pages: an AI agent (module heatmap + AI-tinted charts), a
	// human, and one with no module data (the "Top modules" block must self-skip).
	detailAI := detailSampleModel("Claude")
	detailHuman := detailSampleModel("Ada Lovelace")
	detailNoMods := detailSampleModel("A Contributor With A Very Long Display Name")
	// The detail page before the lazy module load lands: cache empty, so the heatmap
	// is absent and the "analyzing…" note shows in the card.
	detailLoading := detailSampleModel("Claude")
	detailLoading.authorModules = map[string][]git.ModuleCount{}
	detailLoading.authorModulesComputing = map[string]bool{"Claude": true}
	// A committed diff search, the active search prompt, and the fuzzy file picker.
	searched := sampleModel()
	searched.focus = focusDiff
	searched.searchQuery = "line"
	searched.recomputeSearch()
	searchPrompt := sampleModel()
	searchPrompt.searchActive = true
	searchPrompt.searchInput = "a very long query that should be truncated by the footer padding logic"
	finding := sampleModel()
	finding.fileFindActive = true
	finding.fileFindInput = "view"
	// Sidebar collapse/expand states: the file view with the sidebar hidden (diff
	// fills the width) and the history view widened (commit rows grow author/date/
	// tag columns, including a tagged commit).
	hiddenSidebar := sampleModel()
	hiddenSidebar.sidebar = sidebarHidden
	hiddenSidebar.focus = focusDiff
	wideLog := logSampleModel()
	wideLog.sidebar = sidebarWide
	wideLog.commits[0].Tags = []string{"v1.2.0", "v1.1.0"}
	// The history view with the diff pane opened on the right half (the default is
	// closed, exercised by logSampleModel above).
	openLog := logSampleModel()
	openLog.logDiffOpen = true
	openLog.focus = focusDiff
	for _, m := range []model{sampleModel(), logSampleModel(), openLog, workingLog, emptyLog, commitSampleModel(), workingCommit, emptyCommit, emptyWorking, shimmer, details, detailsScrolled, overview, overviewScrolled, overviewEmpty, overviewLoading, detailAI, detailHuman, detailNoMods, detailLoading, searched, searchPrompt, finding, hiddenSidebar, wideLog} {
		for _, sz := range [][2]int{{200, 50}, {120, 40}, {100, 18}, {80, 24}, {60, 12}} {
			m.width, m.height = sz[0], sz[1]
			for i, line := range strings.Split(m.render(), "\n") {
				if w := lipgloss.Width(line); w > m.width {
					t.Errorf("mode=%d %dx%d line %d width %d exceeds %d: %q", m.mode, sz[0], sz[1], i, w, m.width, line)
				}
			}
		}
	}
}

// TestFullScreenFill guards that the TUI occupies the entire terminal: exactly
// one rendered line per row, and every line spans the full width — in both the
// unified and side-by-side diff modes, with the diff pane focused so its rows
// are exercised.
func TestFullScreenFill(t *testing.T) {
	workingLog := logSampleModel()
	workingLog.logWorking = true
	overview := sampleModel()
	overview.mode = viewOverview
	overview.commits = logSampleModel().commits
	overview.historyComputed = true
	overview.historyStats = sampleHistory()
	overviewLoading := overview
	overviewLoading.historyComputed = false
	detailAI := detailSampleModel("Claude")
	detailHuman := detailSampleModel("Ada Lovelace")
	hiddenSidebar := sampleModel()
	hiddenSidebar.sidebar = sidebarHidden
	wideLog := logSampleModel()
	wideLog.sidebar = sidebarWide
	wideLog.commits[0].Tags = []string{"v1.2.0"}
	openLog := logSampleModel()
	openLog.logDiffOpen = true
	for _, m := range []model{sampleModel(), logSampleModel(), openLog, workingLog, commitSampleModel(), overview, overviewLoading, detailAI, detailHuman, hiddenSidebar, wideLog} {
		m.focus = focusDiff
		for _, split := range []bool{false, true} {
			m.splitView = split
			for _, sz := range [][2]int{{200, 50}, {120, 40}, {100, 24}, {80, 24}, {70, 16}, {60, 12}} {
				m.width, m.height = sz[0], sz[1]
				lines := strings.Split(m.render(), "\n")
				if len(lines) != sz[1] {
					t.Errorf("mode=%d split=%v %dx%d: %d lines, want %d", m.mode, split, sz[0], sz[1], len(lines), sz[1])
				}
				for i, line := range lines {
					if w := lipgloss.Width(line); w != sz[0] {
						t.Errorf("mode=%d split=%v %dx%d line %d width %d, want %d", m.mode, split, sz[0], sz[1], i, w, sz[0])
					}
				}
			}
		}
	}
}

// TestOverlayFloats checks that a floating overlay (help, commit details) fills
// the screen exactly and keeps the background visible behind it (it composites a
// dimmed scrim rather than replacing the screen), at a range of sizes.
func TestOverlayFloats(t *testing.T) {
	help := sampleModel()
	help.showHelp = true

	details := logSampleModel()
	details.commits[0].Body = "A body paragraph that explains the change in enough words to span a couple of lines inside the floating window."
	details.showCommitDetails = true

	for _, m := range []model{help, details} {
		for _, sz := range [][2]int{{200, 50}, {120, 40}, {100, 24}, {80, 20}, {60, 14}} {
			m.width, m.height = sz[0], sz[1]
			out := m.render()
			lines := strings.Split(out, "\n")
			if len(lines) != sz[1] {
				t.Errorf("showHelp=%v %dx%d: %d lines, want %d", m.showHelp, sz[0], sz[1], len(lines), sz[1])
			}
			for i, line := range lines {
				if w := lipgloss.Width(line); w > sz[0] {
					t.Errorf("showHelp=%v %dx%d line %d width %d exceeds %d", m.showHelp, sz[0], sz[1], i, w, sz[0])
				}
			}
			// With ample margin the box can't cover the whole screen, so the
			// background header must survive behind the scrim — proof the overlay
			// floats instead of replacing the screen. (At tiny sizes the box
			// legitimately fills everything, so only assert this when there's room.)
			if sz[0] >= 120 && sz[1] >= 40 && !strings.Contains(out, "diffcat") {
				t.Errorf("showHelp=%v %dx%d: background 'diffcat' header missing — overlay replaced the screen", m.showHelp, sz[0], sz[1])
			}
		}
	}
}

// TestLogModeNavigation checks the history view's cursor movement, merge
// detection, clamping, and that leaving restores the branch view.
func TestLogModeNavigation(t *testing.T) {
	m := logSampleModel()
	if m.mode != viewLog {
		t.Fatalf("logSampleModel should be in viewLog, got %d", m.mode)
	}
	if c := m.selectedCommit(); c == nil || c.Short != "aaa1111" {
		t.Fatalf("first selected commit = %+v", c)
	}

	m.moveCommitCursor(1)
	if m.commitCursor != 1 {
		t.Errorf("after move down, cursor = %d, want 1", m.commitCursor)
	}
	if c := m.selectedCommit(); c == nil || !c.IsMerge() {
		t.Errorf("commit at index 1 should be a merge, got %+v", c)
	}

	m.moveCommitCursor(99)
	if m.commitCursor != len(m.commits)-1 {
		t.Errorf("cursor should clamp to last (%d), got %d", len(m.commits)-1, m.commitCursor)
	}
	m.moveCommitCursor(-99)
	if m.commitCursor != 0 {
		t.Errorf("cursor should clamp to 0, got %d", m.commitCursor)
	}

	m.exitLog()
	if m.mode != viewBranch {
		t.Errorf("exitLog should return to viewBranch, got %d", m.mode)
	}
}

// TestCommitDrillInRestore checks that leaving a per-commit tree restores the
// stashed branch tree verbatim: the same files, rows, and cursor position the
// branch view had before drilling in, with the commit scope cleared.
func TestCommitDrillInRestore(t *testing.T) {
	m := logSampleModel()

	// Capture the branch tree as it was before drilling in (logSampleModel was
	// built from sampleModel, so it carries a real file tree underneath).
	wantFiles := m.files
	wantRows := m.rows
	wantCursor := m.cursor

	// Simulate enterCommit's effect without shelling out to git: stash the branch
	// tree, then repurpose the shared fields for the commit's (different) files.
	m.branchFiles, m.branchRows, m.branchCursor = m.files, m.rows, m.cursor
	m.scopeCommit = &m.commits[0]
	m.files = []git.FileChange{{Path: "only/in/commit.go", Status: "A", Added: 3, Deleted: 0}}
	m.cursor = 0
	m.rebuildTree()
	m.mode = viewCommit

	if f := m.selectedFile(); f == nil && len(m.files) > 0 {
		// Park on the file row (the compressed "only/in" chain puts a folder first).
		for i, r := range m.rows {
			if r.file != nil {
				m.cursor = i
				break
			}
		}
	}
	if len(m.files) != 1 || m.files[0].Path != "only/in/commit.go" {
		t.Fatalf("commit tree not loaded, files = %+v", m.files)
	}

	m.exitCommit()

	if m.mode != viewLog {
		t.Errorf("exitCommit should return to viewLog, got %d", m.mode)
	}
	if m.scopeCommit != nil {
		t.Errorf("scopeCommit should be cleared, got %+v", m.scopeCommit)
	}
	if len(m.files) != len(wantFiles) || m.cursor != wantCursor {
		t.Errorf("branch tree not restored: files=%d (want %d), cursor=%d (want %d)",
			len(m.files), len(wantFiles), m.cursor, wantCursor)
	}
	if len(m.rows) != len(wantRows) {
		t.Errorf("branch rows not restored: got %d, want %d", len(m.rows), len(wantRows))
	}
}

// TestAuthorDetailNavigation checks that selecting a contributor in the Stats
// ranking opens their detail page, that the cursor scrolls the ranking, and that
// backing out returns to the overview.
func TestAuthorDetailNavigation(t *testing.T) {
	m := sampleModel()
	m.mode = viewOverview
	m.historyComputed = true
	m.historyStats = sampleHistory()
	m.authorModules = map[string][]git.ModuleCount{}
	m.authorModulesComputing = map[string]bool{}
	m.width, m.height = 120, 40

	// j moves the ranking cursor; enter opens the author under it.
	m.moveDown()
	if m.overviewCursor != 1 {
		t.Fatalf("after moveDown, overviewCursor = %d, want 1", m.overviewCursor)
	}
	wantName := m.historyStats.Authors[1].Name
	cmd := m.enterAuthorDetail()
	if m.mode != viewAuthorDetail {
		t.Fatalf("enterAuthorDetail should switch to viewAuthorDetail, got %d", m.mode)
	}
	if m.detailAuthor != wantName {
		t.Errorf("detailAuthor = %q, want %q", m.detailAuthor, wantName)
	}
	// The author has SHAs in sampleHistory, so the lazy module load is kicked.
	if cmd == nil {
		t.Error("enterAuthorDetail should return a command to load the contributor's modules")
	}
	if !m.authorModulesComputing[wantName] {
		t.Error("enterAuthorDetail should mark the contributor's modules as computing")
	}

	m.exitAuthorDetail()
	if m.mode != viewOverview {
		t.Errorf("exitAuthorDetail should return to viewOverview, got %d", m.mode)
	}
	// The ranking cursor (and its selection) is preserved across the round trip.
	if m.overviewCursor != 1 {
		t.Errorf("overviewCursor = %d after returning, want 1 (preserved)", m.overviewCursor)
	}

	// enterAuthorDetail is a no-op before the background stats land.
	notReady := sampleModel()
	notReady.mode = viewOverview
	notReady.enterAuthorDetail()
	if notReady.mode == viewAuthorDetail {
		t.Error("enterAuthorDetail should be a no-op when stats haven't been computed")
	}
}

// TestLogWorkingTreeRow checks that the pinned working-tree row shifts the
// commit indexing by one: row 0 is the working tree (no commit), and the
// commits follow beneath it.
func TestLogWorkingTreeRow(t *testing.T) {
	m := logSampleModel()
	m.logWorking = true

	if !m.onWorkingRow() {
		t.Fatalf("cursor 0 with a working row should be the working row")
	}
	if c := m.selectedCommit(); c != nil {
		t.Errorf("working row should select no commit, got %+v", c)
	}
	if got, want := m.logRowCount(), len(m.commits)+1; got != want {
		t.Errorf("logRowCount = %d, want %d", got, want)
	}

	m.moveCommitCursor(1)
	if m.onWorkingRow() {
		t.Errorf("after one move down the cursor should be off the working row")
	}
	if c := m.selectedCommit(); c == nil || c.Short != "aaa1111" {
		t.Errorf("row 1 should be the first commit, got %+v", c)
	}

	// Clamp at the bottom: the last row is the last commit, not past it.
	m.moveCommitCursor(99)
	if m.commitCursor != m.logRowCount()-1 {
		t.Errorf("cursor should clamp to last row %d, got %d", m.logRowCount()-1, m.commitCursor)
	}
	if c := m.selectedCommit(); c == nil || c.Short != "ccc3333" {
		t.Errorf("last row should be the last commit, got %+v", c)
	}
}

// TestTooSmallGate verifies the resize hint replaces the body below the minimum.
func TestTooSmallGate(t *testing.T) {
	m := sampleModel()
	m.width, m.height = 40, 8
	if out := m.render(); !strings.Contains(out, "too small") {
		t.Errorf("expected resize hint below minimum size, got:\n%s", out)
	}
}
