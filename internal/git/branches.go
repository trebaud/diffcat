package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// Branch is one local branch plus what the switcher displays alongside it.
// Git records nothing at branch creation, so the tip commit stands in: Author
// and Date are the branch's newest commit's author name and author date.
type Branch struct {
	Name     string
	Upstream string // remote-tracking counterpart, e.g. "origin/main"; "" when purely local
	Author   string
	Date     string // YYYY-MM-DD
}

// Remote is the remote the branch tracks ("origin" for "origin/main"), or ""
// for a purely local branch.
func (b Branch) Remote() string {
	if i := strings.IndexByte(b.Upstream, '/'); i > 0 {
		return b.Upstream[:i]
	}
	return b.Upstream
}

// LocalBranches lists the repo's local branches, most recently committed first
// — the order the branch switcher shows for an empty query. Nil on error.
func LocalBranches(repo string) []Branch {
	// Unit separators frame the fields (for-each-ref spells the hex escape %1f,
	// unlike git log's %x1f) so nothing in a ref or author name can split one.
	out, err := exec.Command("git", "-C", repo, "for-each-ref",
		"--sort=-committerdate",
		"--format=%(refname:short)%1f%(upstream:short)%1f%(authorname)%1f%(authordate:format:%Y-%m-%d)",
		"refs/heads").Output()
	if err != nil {
		return nil
	}
	return parseLocalBranches(out)
}

// parseLocalBranches turns the for-each-ref output into Branches. Pure, so it
// can be tested without a repository.
func parseLocalBranches(out []byte) []Branch {
	var branches []Branch
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Split(line, "\x1f")
		if len(f) < 4 || f[0] == "" {
			continue
		}
		branches = append(branches, Branch{Name: f[0], Upstream: f[1], Author: f[2], Date: f[3]})
	}
	return branches
}

// Switch checks out the named branch via `git switch`. Git itself refuses a
// switch that would clobber uncommitted changes; on failure the error carries
// the first line of git's own stderr so the UI can say why.
func Switch(repo, name string) error {
	out, err := exec.Command("git", "-C", repo, "switch", name).CombinedOutput()
	if err == nil {
		return nil
	}
	msg := strings.TrimSpace(string(out))
	if msg == "" {
		return err
	}
	if i := strings.IndexByte(msg, '\n'); i >= 0 {
		msg = msg[:i]
	}
	return fmt.Errorf("%s", msg)
}
