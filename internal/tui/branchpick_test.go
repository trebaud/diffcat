package tui

import (
	"os"
	"strings"
	"testing"
)

// TestBranchPickerSwitchUpdatesHistory drives the switcher end to end: the
// picker lists local branches minus the current one, choosing one returns the
// background switch command, and feeding its message through Update reloads
// the commit history onto the new branch immediately — no poll needed.
func TestBranchPickerSwitchUpdatesHistory(t *testing.T) {
	repo := initSyncRepo(t) // main with two commits
	gitIn(t, repo, "checkout", "-b", "feature")
	shaFeature := commitInRepo(t, repo, "feature work")

	m := newSyncModel(t, repo) // launches on `feature`, viewLog
	m.openBranchPicker()
	if !m.branchPickActive {
		t.Fatal("openBranchPicker should open the picker")
	}
	for _, b := range m.branchPickList {
		if b.Name == "feature" {
			t.Fatal("picker offered the checked-out branch")
		}
	}

	// Choose "main" the way the Enter handler does.
	matches := m.branchPickMatches()
	if len(matches) != 1 || matches[0].path != "main" {
		t.Fatalf("picker entries = %+v, want just main", matches)
	}
	msg, ok := switchBranchCmd(m.repo, matches[0].path)().(branchSwitchMsg)
	if !ok || msg.err != nil {
		t.Fatalf("switch failed: %+v", msg)
	}
	next, _ := m.Update(msg)
	m = next.(model)

	// The history must already reflect the new branch — from this Update alone.
	if m.branch != "main" {
		t.Errorf("branch after switch = %q, want main", m.branch)
	}
	if hasCommit(m, shaFeature) {
		t.Errorf("feature commit %s still in the history after switching to main", shaFeature)
	}
	if len(m.commits) == 0 {
		t.Fatal("commit history empty after the switch")
	}
	if !strings.Contains(m.flash, "switched to main") {
		t.Errorf("flash = %q, want a switched-to confirmation", m.flash)
	}
}

// TestBranchPickerSwitchRefusalFlashes checks the failure path: when git
// refuses the switch (here: an uncommitted change it would clobber), HEAD
// stays put and git's reason lands in the footer flash.
func TestBranchPickerSwitchRefusalFlashes(t *testing.T) {
	repo := initSyncRepo(t)
	if err := os.WriteFile(repo+"/f.txt", []byte("main version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo, "add", "f.txt")
	commitInRepo(t, repo, "add f.txt on main")
	gitIn(t, repo, "checkout", "-b", "feature")
	if err := os.WriteFile(repo+"/f.txt", []byte("feature version\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitIn(t, repo, "add", "f.txt")
	commitInRepo(t, repo, "change f.txt on feature")
	// A dirty f.txt differs across the two branches, so git must refuse.
	if err := os.WriteFile(repo+"/f.txt", []byte("dirty\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	m := newSyncModel(t, repo)
	msg, ok := switchBranchCmd(m.repo, "main")().(branchSwitchMsg)
	if !ok || msg.err == nil {
		t.Fatalf("switch over a clobbering dirty file should fail, got %+v", msg)
	}
	next, _ := m.Update(msg)
	m = next.(model)
	if m.branch != "feature" {
		t.Errorf("branch = %q after a refused switch, want feature", m.branch)
	}
	if !strings.Contains(m.flash, "switch failed") {
		t.Errorf("flash = %q, want the switch-failed message", m.flash)
	}
}
