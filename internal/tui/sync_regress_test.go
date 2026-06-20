package tui

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/trebaud/diffcat/internal/git"
)

// initSyncRepo builds a throwaway git repo with two commits on `main` and returns
// its path. The author/committer identity is forced via env on each commit so the
// test needs no global git config.
func initSyncRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	run := func(args ...string) {
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
	run("init", "-b", "main")
	run("commit", "--allow-empty", "-m", "first")
	run("commit", "--allow-empty", "-m", "second")
	return repo
}

// commitInRepo lands a new empty commit and returns its full SHA.
func commitInRepo(t *testing.T, repo, msg string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", repo, "commit", "--allow-empty", "-m", msg)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Tester", "GIT_AUTHOR_EMAIL=tester@example.com",
		"GIT_COMMITTER_NAME=Tester", "GIT_COMMITTER_EMAIL=tester@example.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
	sha, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		t.Fatalf("rev-parse HEAD: %v", err)
	}
	return strings.TrimSpace(string(sha))
}

// newSyncModel builds the real model against repo, mirroring tui.Run's setup, and
// parks it in the commit-history view.
func newSyncModel(t *testing.T, repo string) model {
	t.Helper()
	base := git.BaseRef(repo, "main")
	files, err := git.ChangedFiles(repo, base)
	if err != nil {
		t.Fatalf("ChangedFiles: %v", err)
	}
	m := newModel(repo, base, "main", true, "main", files, git.Shortstat(repo, base), resolved{reduceMotion: true})
	m.mode = viewLog
	return m
}

// hasCommit reports whether sha is present in the loaded history list.
func hasCommit(m model, sha string) bool {
	for _, c := range m.commits {
		if c.SHA == sha {
			return true
		}
	}
	return false
}

// poll feeds one background git-state message — exactly what syncCmd delivers —
// and returns the updated model.
func poll(m model) model {
	next, _ := m.Update(gitStateMsg{fingerprint: git.Fingerprint(m.repo, m.baseName)})
	return next.(model)
}

// TestSyncShowsCommitMadeWhileInDashboard reproduces the bug: a commit that lands
// while the Stats dashboard covers the list must appear once we return to the log,
// even though refresh skipped the list reload while the dashboard was up.
func TestSyncShowsCommitMadeWhileInDashboard(t *testing.T) {
	repo := initSyncRepo(t)
	m := newSyncModel(t, repo)
	m.enterOverview()

	sha := commitInRepo(t, repo, "made while in dashboard")
	m = poll(m) // refresh runs but skips the list reload for viewOverview

	m.exitOverview()
	m = poll(m) // a second poll: fingerprint already matches, so no delta — no help here

	if !hasCommit(m, sha) {
		t.Fatalf("commit %s made while in the dashboard never appeared in the history", sha)
	}
}

// TestSyncShowsCommitMadeWhileDrilledIntoCommit is the same for a commit drill-in.
func TestSyncShowsCommitMadeWhileDrilledIntoCommit(t *testing.T) {
	repo := initSyncRepo(t)
	m := newSyncModel(t, repo)
	if c := m.selectedCommit(); c == nil {
		t.Fatalf("no commit under the cursor to drill into")
	}
	m.enterCommit()

	sha := commitInRepo(t, repo, "made while drilled in")
	m = poll(m) // refresh skips the list reload for viewCommit

	m.exitCommit()
	m = poll(m) // fingerprint unchanged on the second poll

	if !hasCommit(m, sha) {
		t.Fatalf("commit %s made while drilled in never appeared in the history", sha)
	}
}

// TestSyncShowsCommitMadeWhileInLog is the control: the common path (committing
// while sitting in the plain log) still surfaces the new commit on the next poll.
func TestSyncShowsCommitMadeWhileInLog(t *testing.T) {
	repo := initSyncRepo(t)
	m := newSyncModel(t, repo)

	sha := commitInRepo(t, repo, "made while in log")
	m = poll(m)

	if !hasCommit(m, sha) {
		t.Fatalf("commit %s made while in the log never appeared in the history", sha)
	}
}
