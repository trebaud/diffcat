// Package git wraps the git CLI for the operations sashi needs:
// resolving the base branch, listing changed files against it, and producing
// per-file diffs. Everything shells out to git so we inherit the user's config.
package git

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// IsRepo returns true if path is inside a git work tree.
func IsRepo(path string) bool {
	err := exec.Command("git", "-C", path, "rev-parse", "--is-inside-work-tree").Run()
	return err == nil
}

// RepoRoot returns the absolute path to the top-level work tree for path.
func RepoRoot(path string) (string, error) {
	out, err := exec.Command("git", "-C", path, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return "", fmt.Errorf("not a git repository: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// CurrentBranch returns the checked-out branch name, or "" when detached.
func CurrentBranch(repo string) string {
	out, err := exec.Command("git", "-C", repo, "branch", "--show-current").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// DefaultBranch picks the branch to diff against. It prefers origin/HEAD, then
// probes the conventional names. "master" is checked before "main" since this
// tool is built around diffing against master.
func DefaultBranch(repo string) string {
	out, err := exec.Command("git", "-C", repo, "symbolic-ref", "refs/remotes/origin/HEAD").Output()
	if err == nil {
		parts := strings.Split(strings.TrimSpace(string(out)), "/")
		if len(parts) > 0 && parts[len(parts)-1] != "" {
			return parts[len(parts)-1]
		}
	}
	for _, name := range []string{"master", "main"} {
		if exec.Command("git", "-C", repo, "rev-parse", "--verify", "--quiet", name).Run() == nil {
			return name
		}
	}
	return "master"
}

// MergeBase returns the common ancestor of base and HEAD. Diffing against the
// merge base shows what the current branch added rather than every change that
// has since landed on base. Falls back to base itself if no ancestor is found.
func MergeBase(repo, base string) string {
	out, err := exec.Command("git", "-C", repo, "merge-base", base, "HEAD").Output()
	if err != nil {
		return base
	}
	if mb := strings.TrimSpace(string(out)); mb != "" {
		return mb
	}
	return base
}

// isBranch reports whether ref names a local or remote-tracking branch.
func isBranch(repo, ref string) bool {
	for _, full := range []string{"refs/heads/" + ref, "refs/remotes/" + ref} {
		if exec.Command("git", "-C", repo, "show-ref", "--verify", "--quiet", full).Run() == nil {
			return true
		}
	}
	return false
}

// BaseRef resolves the ref to diff against for the user-supplied base.
//
// For a branch, it returns the merge base with HEAD so the diff shows what the
// current branch added rather than changes that have since landed on the
// branch. For a commit, tag, or other commit-ish (e.g. a raw SHA or HEAD~3),
// the ref is returned verbatim so the diff compares directly against exactly
// that point — using a merge base there would silently compare against a
// different commit and surprise the user.
func BaseRef(repo, base string) string {
	if isBranch(repo, base) {
		return MergeBase(repo, base)
	}
	return base
}

// FileChange describes one path that differs between the base and the working tree.
type FileChange struct {
	Path    string
	Status  string // "A", "M", "D", "R", "C", "T", or "?" for untracked
	Added   int    // -1 for binary
	Deleted int    // -1 for binary
}

// Binary reports whether git could not compute line stats for the change.
func (f FileChange) Binary() bool { return f.Added < 0 || f.Deleted < 0 }

// ChangedFiles lists every path that differs between base and the working tree,
// including staged, unstaged, and (optionally) untracked files. It merges
// `git diff --numstat` (line counts) with `--name-status` (change type).
func ChangedFiles(repo, base string) ([]FileChange, error) {
	statusByPath, order, err := nameStatus(repo, base)
	if err != nil {
		return nil, err
	}

	stats := numStat(repo, base)

	changes := make([]FileChange, 0, len(order))
	for _, path := range order {
		add, del := -2, -2
		if s, ok := stats[path]; ok {
			add, del = s[0], s[1]
		}
		// numstat can miss a path that name-status reports (e.g. pure mode
		// change); default such files to 0/0 rather than leaving them binary.
		if add == -2 {
			add, del = 0, 0
		}
		changes = append(changes, FileChange{
			Path:    path,
			Status:  statusByPath[path],
			Added:   add,
			Deleted: del,
		})
	}

	for _, path := range untracked(repo) {
		if _, seen := statusByPath[path]; seen {
			continue
		}
		add, del := countLines(filepath.Join(repo, path))
		changes = append(changes, FileChange{Path: path, Status: "?", Added: add, Deleted: del})
	}

	return changes, nil
}

// nameStatus returns a path→status map and the paths in git's reported order.
func nameStatus(repo, base string) (map[string]string, []string, error) {
	out, err := exec.Command("git", "-C", repo, "diff", "--name-status", base).Output()
	if err != nil {
		return nil, nil, fmt.Errorf("git diff failed: %w", err)
	}
	statusByPath := map[string]string{}
	var order []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) < 2 {
			continue
		}
		status := fields[0][:1] // first letter; renames look like "R100"
		// For renames/copies git emits old\tnew — use the new path.
		path := fields[len(fields)-1]
		statusByPath[path] = status
		order = append(order, path)
	}
	return statusByPath, order, nil
}

// numStat returns path→[added, deleted]; -1 entries mean a binary file.
func numStat(repo, base string) map[string][2]int {
	out, err := exec.Command("git", "-C", repo, "diff", "--numstat", base).Output()
	if err != nil {
		return nil
	}
	stats := map[string][2]int{}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		add, del := -1, -1 // "-" markers (binary) stay -1
		fmt.Sscanf(fields[0], "%d", &add)
		fmt.Sscanf(fields[1], "%d", &del)
		path := strings.Join(fields[2:], " ")
		stats[path] = [2]int{add, del}
	}
	return stats
}

// untracked returns paths not yet known to git, respecting .gitignore.
func untracked(repo string) []string {
	out, err := exec.Command("git", "-C", repo, "ls-files", "--others", "--exclude-standard").Output()
	if err != nil {
		return nil
	}
	var paths []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			paths = append(paths, line)
		}
	}
	return paths
}

// FileContent reads the working-tree file (the new side of the diff) split into
// lines, with the same newline and binary semantics as countLines so its length
// matches the diff's line numbers. The trailing element of a newline-terminated
// file is dropped; a final line without a newline is kept. CR bytes are left in
// place so revealed context matches the diffed text byte-for-byte.
func FileContent(repo, path string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(repo, path))
	if err != nil {
		return nil, err
	}
	if len(data) > 0 {
		head := data
		if len(head) > 8000 {
			head = head[:8000]
		}
		if strings.IndexByte(string(head), 0) >= 0 {
			return nil, fmt.Errorf("binary file: %s", path)
		}
	}
	if len(data) == 0 {
		return nil, nil
	}
	content := string(data)
	lines := strings.Split(content, "\n")
	if strings.HasSuffix(content, "\n") {
		lines = lines[:len(lines)-1] // drop the empty element after the final newline
	}
	return lines, nil
}

// countLines returns (lineCount, 0) for a new file, or (-1, -1) if unreadable
// or apparently binary (contains a NUL byte in the first chunk).
func countLines(path string) (int, int) {
	data, err := os.ReadFile(path)
	if err != nil {
		return -1, -1
	}
	if len(data) > 0 {
		head := data
		if len(head) > 8000 {
			head = head[:8000]
		}
		if strings.IndexByte(string(head), 0) >= 0 {
			return -1, -1
		}
	}
	if len(data) == 0 {
		return 0, 0
	}
	content := string(data)
	n := strings.Count(content, "\n")
	if !strings.HasSuffix(content, "\n") {
		n++ // last line has no trailing newline
	}
	return n, 0
}

// FileDiff returns the unified diff for a single path against base. Untracked
// files have no diff in git's index, so we synthesize one with --no-index
// against /dev/null.
func FileDiff(repo, base, path string, status string) string {
	if status == "?" {
		out, _ := exec.Command("git", "-C", repo, "diff", "--no-index", "--", os.DevNull, path).Output()
		return string(out)
	}
	out, err := exec.Command("git", "-C", repo, "diff", base, "--", path).Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// Shortstat returns a one-line summary of the whole diff against base, e.g.
// "5 files changed, 120 insertions(+), 30 deletions(-)". Empty when clean.
func Shortstat(repo, base string) string {
	out, err := exec.Command("git", "-C", repo, "diff", "--shortstat", base).Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
