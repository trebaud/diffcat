// Package git wraps the git CLI for the operations diffcat needs:
// resolving the base branch, listing changed files against it, and producing
// per-file diffs. Everything shells out to git so we inherit the user's config.
package git

import (
	"crypto/sha1"
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

// Commit is one commit's metadata for the history view. Short is the abbreviated
// SHA, Date is the author date (YYYY-MM-DD), and Parents lists parent SHAs —
// more than one means a merge commit.
type Commit struct {
	SHA     string
	Short   string
	Author  string
	Date    string
	Subject string
	Parents []string
}

// IsMerge reports whether the commit has more than one parent.
func (c Commit) IsMerge() bool { return len(c.Parents) > 1 }

// Commits lists the branch's history newest-first. On a feature branch it shows
// the commits added on top of base (base..HEAD); when that range is empty — e.g.
// HEAD sits on the base/default branch itself — it falls back to HEAD's full
// history so the view always has something to show.
func Commits(repo, base string) ([]Commit, error) {
	commits, err := commitLog(repo, base+"..HEAD")
	if err != nil {
		return nil, err
	}
	if len(commits) == 0 {
		return commitLog(repo, "HEAD")
	}
	return commits, nil
}

// commitLog runs `git log` over revRange and parses the result.
func commitLog(repo, revRange string) ([]Commit, error) {
	// Unit/record separators (US 0x1f / RS 0x1e) frame the fields so subjects
	// with spaces or punctuation parse unambiguously.
	const format = "--pretty=format:%H%x1f%h%x1f%an%x1f%ad%x1f%P%x1f%s%x1e"
	out, err := exec.Command("git", "-C", repo, "log", "--no-color", "--date=short", format, revRange).Output()
	if err != nil {
		return nil, fmt.Errorf("git log failed: %w", err)
	}
	return parseCommits(out), nil
}

// parseCommits turns the separator-framed `git log` output into Commits. It is a
// pure function so it can be tested without a repository.
func parseCommits(data []byte) []Commit {
	var commits []Commit
	for _, rec := range strings.Split(string(data), "\x1e") {
		rec = strings.Trim(rec, "\n")
		if rec == "" {
			continue
		}
		f := strings.Split(rec, "\x1f")
		if len(f) < 6 {
			continue
		}
		var parents []string
		if p := strings.TrimSpace(f[4]); p != "" {
			parents = strings.Fields(p)
		}
		commits = append(commits, Commit{
			SHA:     f[0],
			Short:   f[1],
			Author:  f[2],
			Date:    f[3],
			Parents: parents,
			Subject: f[5],
		})
	}
	return commits
}

// CommitDiff returns the patch a single commit introduced, ready for diff.Parse.
// `git show --format=` drops the commit-message header and emits a plain unified
// diff; it handles root commits and merges (combined diff) without special-casing.
func CommitDiff(repo, sha string) string {
	out, err := exec.Command("git", "-C", repo, "show", "--no-color", "--format=", "--patch", sha).Output()
	if err != nil {
		return ""
	}
	return strings.TrimLeft(string(out), "\n")
}

// CommitFiles lists the files a single commit changed, with per-file line stats —
// the backing list for the per-commit file tree (viewCommit). It mirrors
// ChangedFiles but scopes to one commit via `git show` instead of a base diff;
// a merge shown with the default combined format yields no per-file entries, so
// the tree comes up empty (the combined diff still previews via CommitDiff).
func CommitFiles(repo, sha string) ([]FileChange, error) {
	nameOut, err := exec.Command("git", "-C", repo, "show", "--no-color", "--format=", "--name-status", sha).Output()
	if err != nil {
		return nil, fmt.Errorf("git show --name-status failed: %w", err)
	}
	statusByPath, order := parseNameStatus(nameOut)

	numOut, _ := exec.Command("git", "-C", repo, "show", "--no-color", "--format=", "--numstat", sha).Output()
	stats := parseNumStat(numOut)

	return mergeChanges(statusByPath, order, stats), nil
}

// CommitFileDiff returns one commit's patch for a single path, ready for
// diff.Parse. Like CommitDiff it strips the message header via --format=.
func CommitFileDiff(repo, sha, path string) string {
	out, err := exec.Command("git", "-C", repo, "show", "--no-color", "--format=", "--patch", sha, "--", path).Output()
	if err != nil {
		return ""
	}
	return strings.TrimLeft(string(out), "\n")
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

// Fingerprint returns a cheap hash of the repository state diffcat renders, for
// polling in the background to tell whether the on-screen view has drifted from
// the working tree. It folds together HEAD and the base tip (so commits,
// checkouts, rebases, and the base branch advancing all move it), the porcelain
// status (the set of changed paths and their staged/unstaged state), and each
// changed path's size+mtime (so an in-place edit to an already-modified file —
// which leaves the status letters unchanged — still moves it). It deliberately
// avoids recomputing the diff: a couple of git calls plus a handful of lstats,
// far cheaper than ChangedFiles. An unchanged fingerprint means nothing diffcat
// cares about has changed, so a poll can skip the refresh entirely.
func Fingerprint(repo, baseName string) string {
	h := sha1.New()
	for _, ref := range []string{"HEAD", baseName} {
		out, _ := exec.Command("git", "-C", repo, "rev-parse", "--verify", "--quiet", ref).Output()
		h.Write(out)
	}
	statusOut, _ := exec.Command("git", "-C", repo, "status", "--porcelain", "--untracked-files=all").Output()
	h.Write(statusOut)
	for _, p := range statusPaths(statusOut) {
		if fi, err := os.Stat(filepath.Join(repo, p)); err == nil {
			fmt.Fprintf(h, "\x00%s\x00%d\x00%d", p, fi.Size(), fi.ModTime().UnixNano())
		}
	}
	return fmt.Sprintf("%x", h.Sum(nil))
}

// statusPaths extracts the changed paths from `git status --porcelain` output.
// Each line is "XY <path>" (columns 0-1 are the staged/unstaged codes, the path
// starts at column 3); a rename is "XY <orig> -> <new>", from which we take the
// new path. Pure, so it can be tested without a repo.
func statusPaths(porcelain []byte) []string {
	var paths []string
	for _, line := range strings.Split(string(porcelain), "\n") {
		if len(line) < 4 {
			continue
		}
		p := line[3:]
		if i := strings.Index(p, " -> "); i >= 0 {
			p = p[i+len(" -> "):]
		}
		paths = append(paths, strings.Trim(p, "\""))
	}
	return paths
}

// ChangedFiles lists every path that differs between base and the working tree,
// including staged, unstaged, and (optionally) untracked files. It merges
// `git diff --numstat` (line counts) with `--name-status` (change type).
func ChangedFiles(repo, base string) ([]FileChange, error) {
	nameOut, err := exec.Command("git", "-C", repo, "diff", "--name-status", base).Output()
	if err != nil {
		return nil, fmt.Errorf("git diff failed: %w", err)
	}
	statusByPath, order := parseNameStatus(nameOut)

	numOut, _ := exec.Command("git", "-C", repo, "diff", "--numstat", base).Output()
	stats := parseNumStat(numOut)

	changes := mergeChanges(statusByPath, order, stats)

	for _, path := range untracked(repo) {
		if _, seen := statusByPath[path]; seen {
			continue
		}
		add, del := countLines(filepath.Join(repo, path))
		changes = append(changes, FileChange{Path: path, Status: "?", Added: add, Deleted: del})
	}

	return changes, nil
}

// mergeChanges joins the name-status (change type) and numstat (line counts)
// views of the same diff into FileChanges, preserving git's reported order.
func mergeChanges(statusByPath map[string]string, order []string, stats map[string][2]int) []FileChange {
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
	return changes
}

// parseNameStatus turns `--name-status` output into a path→status map and the
// paths in git's reported order. Pure, so it can be tested without a repo.
func parseNameStatus(out []byte) (map[string]string, []string) {
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
	return statusByPath, order
}

// parseNumStat turns `--numstat` output into path→[added, deleted]; -1 entries
// mean a binary file. Pure, so it can be tested without a repo.
func parseNumStat(out []byte) map[string][2]int {
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

// FileContentAt reads the file at a specific commit (the new side of a
// commit-scoped diff) via `git show <sha>:<path>`, split into lines with the
// same newline and binary semantics as FileContent so its length matches the
// commit's diff line numbers. This backs context expansion in the per-commit
// drill-in, where the new side is the file as of that commit rather than the
// working tree.
func FileContentAt(repo, sha, path string) ([]string, error) {
	out, err := exec.Command("git", "-C", repo, "show", sha+":"+path).Output()
	if err != nil {
		return nil, fmt.Errorf("git show %s:%s failed: %w", sha, path, err)
	}
	if len(out) > 0 {
		head := out
		if len(head) > 8000 {
			head = head[:8000]
		}
		if strings.IndexByte(string(head), 0) >= 0 {
			return nil, fmt.Errorf("binary file: %s", path)
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	content := string(out)
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
