package tui

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// gitIn runs a git command in repo with a fixed identity, failing the test on error.
func gitIn(t *testing.T, repo string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Tester", "GIT_AUTHOR_EMAIL=tester@example.com",
		"GIT_COMMITTER_NAME=Tester", "GIT_COMMITTER_EMAIL=tester@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// TestBranchSwitchRefreshesHistory covers the bug where switching branches while
// diffcat was open left the commit history showing the old branch. The poll must
// reload the history against the new HEAD and re-label the branch.
func TestBranchSwitchRefreshesHistory(t *testing.T) {
	repo := initSyncRepo(t) // main with two commits
	gitIn(t, repo, "checkout", "-b", "feature")
	shaFeature := commitInRepo(t, repo, "feature work")

	m := newSyncModel(t, repo) // launches on `feature`, viewLog
	if m.branch != "feature" {
		t.Fatalf("expected to launch on feature, got %q", m.branch)
	}
	if !hasCommit(m, shaFeature) {
		t.Fatalf("feature commit %s missing at launch", shaFeature)
	}

	gitIn(t, repo, "checkout", "main")
	m = poll(m)

	if m.branch != "main" {
		t.Errorf("branch label not updated after checkout: got %q, want main", m.branch)
	}
	if hasCommit(m, shaFeature) {
		t.Errorf("feature commit %s still shown after switching to main", shaFeature)
	}
	if len(m.commits) == 0 {
		t.Fatal("commit history empty after switching to main")
	}
}

// TestRefreshForcesRepaintOnHeadMove locks in the rendering fix: when HEAD moves
// (a checkout, commit, or rebase) refresh returns a repaint command so the screen
// is fully redrawn — without it, bubbletea's scroll-diff optimization can leave
// stale commit rows on screen. A working-tree-only change must NOT force a repaint
// (it would flicker on every save).
func TestRefreshForcesRepaintOnHeadMove(t *testing.T) {
	repo := initSyncRepo(t)
	gitIn(t, repo, "checkout", "-b", "feature")
	commitInRepo(t, repo, "feature work")
	m := newSyncModel(t, repo)

	// A checkout moves HEAD → expect a repaint command.
	gitIn(t, repo, "checkout", "main")
	if cmd := m.refresh(); cmd == nil {
		t.Error("refresh after a branch switch returned no repaint command")
	}

	// A pure working-tree edit (HEAD unchanged) → no repaint.
	if err := os.WriteFile(repo+"/scratch.txt", []byte("dirty\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if cmd := m.refresh(); cmd != nil {
		t.Error("refresh on a working-tree-only change should not force a repaint")
	}
}
