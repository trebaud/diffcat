package git

import (
	"reflect"
	"testing"
)

// TestParseLocalBranches checks the for-each-ref parse: the upstream, author,
// and date fields land on the right Branch, and blank lines are skipped.
func TestParseLocalBranches(t *testing.T) {
	out := []byte(
		"feature/x\x1f\x1fAda Lovelace\x1f2026-08-27\n" +
			"main\x1forigin/main\x1fGrace Hopper\x1f2026-08-20\n" +
			"\n")
	got := parseLocalBranches(out)
	want := []Branch{
		{Name: "feature/x", Author: "Ada Lovelace", Date: "2026-08-27"},
		{Name: "main", Upstream: "origin/main", Author: "Grace Hopper", Date: "2026-08-20"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseLocalBranches = %+v, want %+v", got, want)
	}
}

// TestBranchRemote checks the remote-name derivation from the upstream ref.
func TestBranchRemote(t *testing.T) {
	if got := (Branch{Upstream: "origin/feature/x"}).Remote(); got != "origin" {
		t.Errorf("Remote = %q, want origin", got)
	}
	if got := (Branch{}).Remote(); got != "" {
		t.Errorf("Remote of a purely local branch = %q, want empty", got)
	}
}
