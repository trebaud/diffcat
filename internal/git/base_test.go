package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// run executes a git command in repo and fails the test on error.
func run(t *testing.T, repo string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Tester", "GIT_AUTHOR_EMAIL=tester@example.com",
		"GIT_COMMITTER_NAME=Tester", "GIT_COMMITTER_EMAIL=tester@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// commit writes a file and commits it, returning the new HEAD sha.
func commit(t *testing.T, repo, name string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, name), []byte(name+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	run(t, repo, "add", name)
	run(t, repo, "commit", "-m", name)
	return run(t, repo, "rev-parse", "HEAD")
}

// staleLocalRepo builds a repo shaped like the bug this resolution exists for:
// local master sits two commits behind origin/master, and the feature branch was
// cut from origin/master's tip. Returns the repo path and origin/master's sha.
func staleLocalRepo(t *testing.T) (repo, upstreamTip string) {
	t.Helper()
	repo = t.TempDir()
	run(t, repo, "init", "-b", "master")
	commit(t, repo, "a")
	commit(t, repo, "b") // local master stops here
	run(t, repo, "checkout", "-b", "upstream-sim")
	commit(t, repo, "c")
	upstreamTip = commit(t, repo, "d")
	// A remote-tracking ref and its upstream config, without needing a real remote.
	run(t, repo, "update-ref", "refs/remotes/origin/master", upstreamTip)
	run(t, repo, "config", "branch.master.remote", "origin")
	run(t, repo, "config", "branch.master.merge", "refs/heads/master")
	run(t, repo, "checkout", "-b", "feature")
	run(t, repo, "branch", "-D", "upstream-sim")
	commit(t, repo, "e")
	return repo, upstreamTip
}

// A stale local base branch must not stand in for its remote: measuring against
// it reports every commit that landed on master since the last fetch as part of
// this branch.
func TestBaseBranchRevPrefersFresherRemote(t *testing.T) {
	repo, upstreamTip := staleLocalRepo(t)

	if got := BaseBranchRev(repo, "master"); got != "origin/master" {
		t.Fatalf("BaseBranchRev = %q, want origin/master", got)
	}
	if got := BaseRef(repo, BaseBranchRev(repo, "master")); got != upstreamTip {
		t.Errorf("BaseRef = %q, want %q (origin/master tip)", got, upstreamTip)
	}
	// The whole point: one commit of our own, not three.
	own, baseStart, err := BranchHistory(repo, BaseRef(repo, BaseBranchRev(repo, "master")), 10, 0)
	if err != nil {
		t.Fatalf("BranchHistory: %v", err)
	}
	if baseStart != 1 {
		t.Errorf("baseStart = %d, want 1 (only commit e is ours)", baseStart)
	}
	if len(own) > 0 && own[0].Subject != "e" {
		t.Errorf("newest branch commit = %q, want e", own[0].Subject)
	}
}

// With the local branch level with its remote the label stays put, so the header
// and divider keep reading "master" rather than churning to "origin/master".
func TestBaseBranchRevKeepsLabelWhenLevel(t *testing.T) {
	repo, upstreamTip := staleLocalRepo(t)
	run(t, repo, "branch", "-f", "master", upstreamTip)

	if got := BaseBranchRev(repo, "master"); got != "master" {
		t.Errorf("BaseBranchRev = %q, want master", got)
	}
}

// A local base branch ahead of a stale remote-tracking ref wins: the remote is
// the wrong yardstick in that direction too.
func TestBaseBranchRevKeepsAheadLocal(t *testing.T) {
	repo, _ := staleLocalRepo(t)
	run(t, repo, "update-ref", "refs/remotes/origin/master", run(t, repo, "rev-parse", "master~1"))

	if got := BaseBranchRev(repo, "master"); got != "master" {
		t.Errorf("BaseBranchRev = %q, want master", got)
	}
}

// Nothing to resolve against: no remote-tracking ref, an already-qualified ref,
// and a commit-ish all pass through untouched.
func TestBaseBranchRevPassthrough(t *testing.T) {
	repo := t.TempDir()
	run(t, repo, "init", "-b", "master")
	sha := commit(t, repo, "a")
	run(t, repo, "checkout", "-b", "feature")
	commit(t, repo, "b")

	for _, ref := range []string{"master", sha, "HEAD~1", ""} {
		if got := BaseBranchRev(repo, ref); got != ref {
			t.Errorf("BaseBranchRev(%q) = %q, want it unchanged", ref, got)
		}
	}
	run(t, repo, "update-ref", "refs/remotes/origin/main", sha)
	if got := BaseBranchRev(repo, "origin/main"); got != "origin/main" {
		t.Errorf("BaseBranchRev(origin/main) = %q, want it unchanged", got)
	}
}

// A clone where the base branch was never checked out has only the remote ref —
// previously the bare label resolved to nothing and the diff came out empty.
func TestBaseBranchRevWithoutLocalBranch(t *testing.T) {
	repo := t.TempDir()
	run(t, repo, "init", "-b", "feature")
	sha := commit(t, repo, "a")
	commit(t, repo, "b")
	run(t, repo, "update-ref", "refs/remotes/origin/master", sha)

	if got := BaseBranchRev(repo, "master"); got != "origin/master" {
		t.Errorf("BaseBranchRev = %q, want origin/master", got)
	}
	if got := BaseRef(repo, BaseBranchRev(repo, "master")); got != sha {
		t.Errorf("BaseRef = %q, want %q", got, sha)
	}
}
