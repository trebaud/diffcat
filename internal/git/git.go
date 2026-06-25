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
	"sort"
	"strconv"
	"strings"
	"time"
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

// HeadSHA returns the full SHA of HEAD, or "" on error. It's a cheap way to tell
// whether committed history has moved (a commit, checkout, reset, or rebase)
// since some earlier point — used to decide when the whole-history Summary's
// cached stats need recomputing, so a mere working-tree edit doesn't trigger one.
func HeadSHA(repo string) string {
	out, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
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
// SHA, Date is the author date (YYYY-MM-DD), Body is the commit message after the
// subject (may be empty), Parents lists parent SHAs — more than one means a
// merge commit — and Tags are the git tags pointing at this commit (if any).
type Commit struct {
	SHA         string
	Short       string
	Author      string
	AuthorEmail string
	Date        string
	Subject     string
	Body        string
	Parents     []string
	Heads       []string // local branches pointing here (the HEAD branch first)
	Remotes     []string // remote-tracking refs pointing here (e.g. origin/main)
	Tags        []string
}

// IsMerge reports whether the commit has more than one parent.
func (c Commit) IsMerge() bool { return len(c.Parents) > 1 }

// AIAgent names the AI coding agent that authored or assisted the commit (e.g.
// "Claude", "Copilot"), or "" if it reads as human-authored. It inspects the
// author name/email and the Co-authored-by trailers only — never the rest of the
// message — so ordinary prose can't trip a marker (e.g. "cursor" describing the UI
// cursor must not flag a human commit as AI). It's the single-commit counterpart
// to the per-commit classification History runs across the whole history, using
// the same signals — used to label a commit's Stats.
func (c Commit) AIAgent() string {
	return aiAgent(c.Author + "\x00" + c.AuthorEmail + "\x00" + c.coAuthorTrailers())
}

// IsAIAuthored reports whether any AI agent marker is present (see AIAgent).
func (c Commit) IsAIAuthored() bool { return c.AIAgent() != "" }

// coAuthorTrailers returns the commit body's Co-authored-by trailer lines joined
// by newlines (empty if none) — the only part of the message authorship scans, so
// a marker in prose can't be mistaken for an agent signature.
func (c Commit) coAuthorTrailers() string {
	var b strings.Builder
	for _, line := range strings.Split(c.Body, "\n") {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), "co-authored-by:") {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// Commits lists the branch's history newest-first. On a feature branch it shows
// the commits added on top of base (base..HEAD); when that range is empty — e.g.
// HEAD sits on the base/default branch itself — it falls back to HEAD's full
// history so the view always has something to show.
func Commits(repo, base string) ([]Commit, error) {
	remotes := remoteNames(repo)
	commits, err := commitLog(repo, remotes, base+"..HEAD")
	if err != nil {
		return nil, err
	}
	if len(commits) == 0 {
		return commitLog(repo, remotes, "HEAD")
	}
	return commits, nil
}

// BranchHistory returns the history to show in the log view as a single slice
// newest-first, together with baseStart, the index at which the base branch's
// own commits begin so the view can draw a delineation there.
//
// On a feature branch the slice is the branch's commits (base..HEAD) followed by
// the base branch's commits at and before the fork point (base and older, capped
// at baseLimit), and baseStart marks the first of those — the boundary between
// "what this branch added" and "what it branched from". When HEAD sits on the base
// branch itself there is nothing to delineate: the slice is HEAD's history and
// baseStart is -1.
//
// headLimit caps the HEAD-side walk (the branch's own commits, or HEAD's full
// history in the base-branch case) — 0 means walk it all. On a long-lived base
// branch the uncapped HEAD walk dominates startup (tens of thousands of commits
// with the heavy per-commit format), so the launch path passes a cap for a fast
// first list and backfills the rest in the background; baseLimit caps only the
// short base-context tail.
func BranchHistory(repo, base string, baseLimit, headLimit int) (commits []Commit, baseStart int, err error) {
	// Resolve the configured remotes once and thread them through every walk —
	// commitLog would otherwise shell `git remote` afresh on each of the (up to
	// two) calls below.
	remotes := remoteNames(repo)
	own, err := commitLog(repo, remotes, headRev(headLimit, base+"..HEAD")...)
	if err != nil {
		return nil, -1, err
	}
	if len(own) == 0 {
		full, err := commitLog(repo, remotes, headRev(headLimit, "HEAD")...)
		return full, -1, err
	}
	baseHist, err := commitLog(repo, remotes, fmt.Sprintf("--max-count=%d", baseLimit), base)
	if err != nil || len(baseHist) == 0 {
		return own, -1, err
	}
	return append(own, baseHist...), len(own), nil
}

// headRev builds the trailing args for a HEAD-side history walk: the rev,
// preceded by a --max-count cap when limit > 0 (0 walks the whole history).
func headRev(limit int, rev string) []string {
	if limit > 0 {
		return []string{fmt.Sprintf("--max-count=%d", limit), rev}
	}
	return []string{rev}
}

// commitLog runs `git log` with the given trailing args (a rev-range and any
// limiting flags) and parses the result. remotes (the configured remote names)
// is passed in so a multi-call walk resolves it once rather than per call.
func commitLog(repo string, remotes map[string]bool, args ...string) ([]Commit, error) {
	// Unit/record separators (US 0x1f / RS 0x1e) frame the fields so subjects
	// with spaces or punctuation parse unambiguously. The body (%b) is last so
	// its embedded newlines can't be mistaken for a field separator. %D carries
	// the ref decoration (branches/tags) — the ref-name placeholders populate
	// regardless of --decorate, so tags resolve without an extra git call.
	const format = "--pretty=format:%H%x1f%h%x1f%an%x1f%ae%x1f%ad%x1f%P%x1f%D%x1f%s%x1f%b%x1e"
	argv := append([]string{"-C", repo, "log", "--no-color", "--date=format:%Y-%m-%d %H:%M", format}, args...)
	out, err := exec.Command("git", argv...).Output()
	if err != nil {
		return nil, fmt.Errorf("git log failed: %w", err)
	}
	return parseCommits(out, remotes), nil
}

// remoteNames returns the set of configured remote names (e.g. {"origin"}), used
// to tell a remote-tracking ref like "origin/main" apart from a local branch that
// merely contains a slash like "feature/x" — the %D decoration formats both the
// same, so only the remote list disambiguates them. Nil on error (no repo / no
// remotes), which simply classifies every slashed ref as a local branch.
func remoteNames(repo string) map[string]bool {
	out, err := exec.Command("git", "-C", repo, "remote").Output()
	if err != nil {
		return nil
	}
	names := map[string]bool{}
	for _, n := range strings.Fields(string(out)) {
		names[n] = true
	}
	return names
}

// parseCommits turns the separator-framed `git log` output into Commits. It is a
// pure function so it can be tested without a repository.
func parseCommits(data []byte, remotes map[string]bool) []Commit {
	var commits []Commit
	for _, rec := range strings.Split(string(data), "\x1e") {
		rec = strings.Trim(rec, "\n")
		if rec == "" {
			continue
		}
		f := strings.Split(rec, "\x1f")
		if len(f) < 8 {
			continue
		}
		var parents []string
		if p := strings.TrimSpace(f[5]); p != "" {
			parents = strings.Fields(p)
		}
		body := ""
		if len(f) > 8 {
			body = strings.Trim(f[8], "\n")
		}
		heads, remoteRefs, tags := parseRefs(f[6], remotes)
		commits = append(commits, Commit{
			SHA:         f[0],
			Short:       f[1],
			Author:      f[2],
			AuthorEmail: f[3],
			Date:        f[4],
			Parents:     parents,
			Heads:       heads,
			Remotes:     remoteRefs,
			Tags:        tags,
			Subject:     f[7],
			Body:        body,
		})
	}
	return commits
}

// parseRefs splits a `%D` ref decoration into the three kinds diffcat badges
// separately. The decoration is a comma-separated list like
// "HEAD -> main, origin/main, origin/HEAD, tag: v1.2.0, tag: v1.1.0":
//   - "tag: <name>" entries become tags.
//   - "HEAD -> <branch>" yields the branch HEAD is on; a bare "HEAD" (detached)
//     and any other name without a slash is a local branch head. The HEAD branch
//     lands first so callers can flag the checked-out tip.
//   - a name whose first path segment is a configured remote (origin/main,
//     origin/HEAD) is a remote-tracking ref. Slashes alone don't make a ref
//     remote — local branches like feature/x carry them too — so the caller's
//     remote set is what disambiguates.
//
// Pure (given the remote set), so it can be tested without a repo.
func parseRefs(decoration string, remotes map[string]bool) (heads, remoteRefs, tags []string) {
	for _, ref := range strings.Split(decoration, ",") {
		ref = strings.TrimSpace(ref)
		switch {
		case ref == "":
			continue
		case strings.HasPrefix(ref, "tag: "):
			tags = append(tags, strings.TrimPrefix(ref, "tag: "))
		case strings.Contains(ref, " -> "):
			// "HEAD -> main": the branch the working tree is checked out on.
			heads = append(heads, ref[strings.Index(ref, " -> ")+len(" -> "):])
		case isRemoteRef(ref, remotes):
			remoteRefs = append(remoteRefs, ref)
		default:
			heads = append(heads, ref)
		}
	}
	return heads, remoteRefs, tags
}

// isRemoteRef reports whether ref's leading path segment names a configured
// remote — the test that separates "origin/main" from a local "feature/x".
func isRemoteRef(ref string, remotes map[string]bool) bool {
	i := strings.Index(ref, "/")
	return i > 0 && remotes[ref[:i]]
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

// WorkingDiff returns the combined diff of the working tree against HEAD — every
// uncommitted change, staged and unstaged, as one multi-file patch ready for
// diff.Parse. It backs the history view's "working tree" entry, which sits above
// HEAD as the not-yet-committed delta, so it must diff against HEAD, not the
// branch base: diffing against base would fold in the committed changes the
// individual commit rows already account for. Untracked files have no diff in
// the index so they're omitted from this combined view; the per-file drill-in
// still shows them via FileDiff's --no-index path.
func WorkingDiff(repo string) string {
	out, err := exec.Command("git", "-C", repo, "diff", "--no-color", "HEAD").Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// aiAgentMarkers pairs each case-insensitive substring that identifies an AI
// coding agent in a commit's author name/email or its Co-authored-by trailers with
// the agent's display name. Coding agents stamp themselves here: Claude Code adds a
// "Co-Authored-By: Claude <noreply@anthropic.com>" trailer, and the others sign in
// kind. The first pair whose marker is present names the agent, so order matters
// (e.g. "anthropic" and "claude" both resolve to "Claude"). Extend this list to
// recognize more agents.
var aiAgentMarkers = []struct{ marker, name string }{
	{"claude", "Claude"},
	{"anthropic", "Claude"},
	{"copilot", "Copilot"},
	{"cursor", "Cursor"},
	{"devin", "Devin"},
	{"aider", "Aider"},
	{"codex", "Codex"},
	{"chatgpt", "ChatGPT"},
	{"openai", "ChatGPT"},
	{"gpt-", "ChatGPT"},
}

// aiAgent returns the display name of the first AI agent whose marker appears in
// signals (case-insensitive), or "" if none — i.e. the work reads as human. The
// caller passes only authorship signals (author/email/co-author trailers), never
// the free-form message body, so ordinary prose can't trip a marker.
func aiAgent(signals string) string {
	hay := strings.ToLower(signals)
	for _, a := range aiAgentMarkers {
		if strings.Contains(hay, a.marker) {
			return a.name
		}
	}
	return ""
}

// authorCommit carries one non-merge commit's authorship signals plus its churn
// (added+deleted lines) — the unit parseHistory aggregates into the AI/human split.
type authorCommit struct {
	author    string
	email     string
	coauthors string
	churn     int
}

// agent names the AI coding agent behind the commit (see aiAgent), or "" if it
// reads as human-authored. NUL joins the fields so a marker can't straddle a
// boundary and match across two unrelated values.
func (c authorCommit) agent() string {
	return aiAgent(c.author + "\x00" + c.email + "\x00" + c.coauthors)
}

// isAIAuthored reports whether any AI agent marker appears in the commit's author
// or co-author fields.
func (c authorCommit) isAIAuthored() bool { return c.agent() != "" }

// authorshipFormat frames one line per commit: the commit SHA, the author date
// (strict ISO-8601), author, email, and the (possibly multiple, %x1f-joined)
// Co-authored-by values. Leading %x1e (RS) marks each commit so records split
// unambiguously regardless of trailing newlines. The date is its own %x1f field, so
// the spaces inside it don't disturb the field split; the SHA leads (the trailers
// blob, which may itself contain %x1f, must stay the last field).
const authorshipFormat = "--pretty=format:%x1e%H%x1f%aI%x1f%an%x1f%ae%x1f%(trailers:key=Co-authored-by,valueonly,separator=%x1f)"

// AuthorShare is one bucket of the commit count: either a named AI coding agent
// (e.g. "Claude", detected via aiAgentMarkers) or an individual human author (by
// their git author name), with the number of non-merge commits attributed to it.
// AI is true for an agent bucket so the dashboard can color it apart from humans.
type AuthorShare struct {
	Name    string
	Commits int
	AI      bool
}

// sortAuthorShares orders authorship buckets by commit count descending, ties by
// name — the order the Stats dashboard ranks them in.
func sortAuthorShares(shares []AuthorShare) {
	sort.Slice(shares, func(i, j int) bool {
		if shares[i].Commits != shares[j].Commits {
			return shares[i].Commits > shares[j].Commits
		}
		return shares[i].Name < shares[j].Name
	})
}

// DayCount is one calendar day's non-merge commit count, split into human- and
// AI-authored. The Daily series is indexed by day-offset from HistoryStats.Start.
type DayCount struct {
	Human int
	AI    int
}

// ModuleCount is one codebase area (a path prefix; see moduleKey) and the number of
// lines an author changed across it (added+deleted), summed over their commits — the
// unit behind the per-contributor "Top modules" heatmap.
type ModuleCount struct {
	Path  string
	Lines int
}

// sortModuleCounts orders module buckets by lines changed descending, ties by path —
// the order the per-author module heatmap ranks them in.
func sortModuleCounts(mods []ModuleCount) {
	sort.Slice(mods, func(i, j int) bool {
		if mods[i].Lines != mods[j].Lines {
			return mods[i].Lines > mods[j].Lines
		}
		return mods[i].Path < mods[j].Path
	})
}

// moduleKey groups a changed file path into a codebase "area" for the per-author
// module ranking: the file's directory, capped to its first four segments (e.g.
// "internal/tui", "src/controllers/api/v1"). Top-level files have no meaningful
// area, so they return "" and are dropped from the ranking.
func moduleKey(path string) string {
	parts := strings.Split(path, "/")
	dirs := len(parts) - 1 // directory segments, dropping the filename
	if dirs < 1 {
		return "" // top-level file — no meaningful area
	}
	if dirs > 4 {
		dirs = 4
	}
	return strings.Join(parts[:dirs], "/")
}

// HistoryStats summarizes everything reachable from a revision: the non-merge
// commit count, a per-author ranking by commit count (each human author and each
// AI agent its own bucket), and time-series the Stats dashboard visualizes. It
// backs the whole-repo Stats dashboard.
//
// Deliberately churn-free: it's gathered from a plain `git log` with no diff
// stat, so git never has to diff any commit — the cheap commit walk stays fast
// even on a deep history, where `--numstat`/`--shortstat` (which diff every
// commit) would be the bottleneck. The time-series below are derived from the
// author date that the same single walk already formats, so they cost nothing
// extra; render re-buckets the daily resolution down to the screen width. The
// per-contributor module ranking is the one thing that needs per-file line
// counts, so it's computed lazily and scoped to one author's commits by
// AuthorModules — never on this whole-repo open path.
type HistoryStats struct {
	Commits int
	Authors []AuthorShare

	// Start/End are the days of the earliest and latest dated commits (zero if none
	// parsed); Daily is the per-day human/AI commit split indexed by day-offset from
	// Start.
	Start time.Time
	End   time.Time
	Daily []DayCount

	// Punch is the commit count by author-local weekday (0=Sun..6=Sat) × hour
	// (0..23) — the punch-card heatmap.
	Punch [7][24]int

	// FirstAI is the date of the earliest AI-authored commit (HasAI false if none).
	// RecentAI/RecentTotal give the AI share of the most recent RecentTotal commits.
	FirstAI     time.Time
	HasAI       bool
	RecentAI    int
	RecentTotal int

	// Modules ranks the codebase areas by lines changed (added+deleted), descending.
	// The whole-repo walk leaves it nil everywhere — it's filled in lazily, per
	// contributor, by AuthorModules (which diffs only that author's commits) so the
	// expensive numstat work stays off this open path. The contributor "Top modules"
	// heatmap reads it after the lazy load lands; an empty ranking self-skips.
	Modules []ModuleCount

	// ByAuthor maps each ranking bucket's Name (see Authors) to that author's own
	// sub-stats: their Commits, Start/End, Daily series, and Punch card.
	// Authors/HasAI are left zero on these entries so the AI-adoption, human-vs-AI,
	// and concentration charts self-skip when a single contributor's page reuses the
	// dashboard renderers. Computed from the same single walk, so it costs no extra
	// git calls. Nil on the per-author entries themselves (no recursion).
	ByAuthor map[string]HistoryStats

	// AuthorSHAs lists each author's non-merge commit SHAs (keyed by the same Name as
	// Authors/ByAuthor), captured cheaply by the walk. AuthorModules pipes one
	// author's set to git to compute their module ranking on demand — correct even
	// for AI-agent buckets, which a `git log --author` filter could never match.
	AuthorSHAs map[string][]string
}

// recentWindow is how many of the newest commits the AI-share headline summarizes.
const recentWindow = 30

// History gathers HistoryStats for everything reachable from rev (e.g. "HEAD")
// from a single `git log` that only formats author/co-author fields (plus the
// commit SHA, for AuthorModules) — no diff, so it stays fast on large histories.
// Merges are excluded (a merge isn't authored work to rank). Still run off the UI
// thread, since even a bare walk of a very deep history isn't instant. Returns a
// zero HistoryStats on error.
func History(repo, rev string) HistoryStats {
	out, err := exec.Command("git", "-C", repo, "log", "--no-color", "--no-merges", authorshipFormat, rev).Output()
	if err != nil {
		return HistoryStats{}
	}
	return parseHistory(out)
}

// parseHistory aggregates `git log --numstat` output (framed by authorshipFormat)
// into HistoryStats: the non-merge commit count, a per-author commit ranking, the
// repo-wide time-series, and per-author sub-stats (time-series + module line
// ranking). Each commit is classified once — to the AI agent that signed it
// (author or Co-authored-by trailer), else to its human author by name — and
// increments both the repo-wide and that author's accumulators. The numstat lines
// trailing each record give the per-file line counts behind the module ranking.
// Pure, so it's testable without a repository.
func parseHistory(data []byte) HistoryStats {
	// Each author accumulates their own copy of the time-series plus the list of
	// their commit SHAs, so the per-contributor page reuses the dashboard renderers
	// and AuthorModules can scope a numstat pass to just that author's commits.
	type acc struct {
		commits int
		ai      bool
		byDay   map[time.Time]*DayCount
		punch   [7][24]int
		shas    []string
	}
	byName := map[string]*acc{}
	commits := 0

	// Time-series accumulators. byDay keys on midnight-local day; punch counts
	// weekday×hour. firstAI tracks the earliest AI commit. The log arrives
	// newest-first, so the first recentWindow records are the most recent commits.
	byDay := map[time.Time]*DayCount{}
	var punch [7][24]int
	var firstAI time.Time
	hasAI := false
	recentAI, recentTotal := 0, 0

	for _, rec := range strings.Split(string(data), "\x1e") {
		rec = strings.Trim(rec, "\n")
		if rec == "" {
			continue
		}
		commits++
		// The record is a single formatted line (no diff body): sha, date, author,
		// email, co-authors.
		header := rec
		if i := strings.IndexByte(rec, '\n'); i >= 0 {
			header = rec[:i]
		}
		f := strings.SplitN(header, "\x1f", 5)
		var c authorCommit
		var sha, dateStr string
		if len(f) > 0 {
			sha = f[0]
		}
		if len(f) > 1 {
			dateStr = f[1]
		}
		if len(f) > 2 {
			c.author = f[2]
		}
		if len(f) > 3 {
			c.email = f[3]
		}
		if len(f) > 4 {
			c.coauthors = f[4]
		}
		name := c.agent()
		ai := name != ""
		if !ai {
			if name = strings.TrimSpace(c.author); name == "" {
				name = "Unknown"
			}
		}
		a := byName[name]
		if a == nil {
			a = &acc{ai: ai, byDay: map[time.Time]*DayCount{}}
			byName[name] = a
		}
		a.commits++
		if sha != "" {
			a.shas = append(a.shas, sha)
		}

		if recentTotal < recentWindow {
			recentTotal++
			if ai {
				recentAI++
			}
		}

		// A commit with an unparseable/empty author date still counts toward the
		// ranking above, but contributes no time-series point.
		t, err := time.Parse(time.RFC3339, dateStr)
		if err != nil {
			continue
		}
		if ai {
			hasAI = true
			if firstAI.IsZero() || t.Before(firstAI) {
				firstAI = t
			}
		}
		// Punch card reads in the commit's own (author-local) time; the daily
		// timeline keys on the UTC calendar day so differing offsets can't split
		// one day across two map buckets. Each point lands in both the repo-wide and
		// the author's own series.
		punch[int(t.Weekday())][t.Hour()]++
		a.punch[int(t.Weekday())][t.Hour()]++
		u := t.UTC()
		day := time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
		bump := func(m map[time.Time]*DayCount) {
			d := m[day]
			if d == nil {
				d = &DayCount{}
				m[day] = d
			}
			if ai {
				d.AI++
			} else {
				d.Human++
			}
		}
		bump(byDay)
		bump(a.byDay)
	}

	authors := make([]AuthorShare, 0, len(byName))
	byAuthor := make(map[string]HistoryStats, len(byName))
	authorSHAs := make(map[string][]string, len(byName))
	for name, a := range byName {
		authors = append(authors, AuthorShare{Name: name, Commits: a.commits, AI: a.ai})
		aStart, aEnd, aDaily := buildDaily(a.byDay)
		byAuthor[name] = HistoryStats{
			Commits: a.commits,
			Start:   aStart,
			End:     aEnd,
			Daily:   aDaily,
			Punch:   a.punch,
		}
		authorSHAs[name] = a.shas
	}
	sortAuthorShares(authors)

	start, end, daily := buildDaily(byDay)
	return HistoryStats{
		Commits:     commits,
		Authors:     authors,
		Start:       start,
		End:         end,
		Daily:       daily,
		Punch:       punch,
		FirstAI:     firstAI,
		HasAI:       hasAI,
		RecentAI:    recentAI,
		RecentTotal: recentTotal,
		ByAuthor:    byAuthor,
		AuthorSHAs:  authorSHAs,
	}
}

// parseNumstat reads one numstat count field: a decimal line count, or "-" (git's
// marker for a binary file) which yields 0. Any unparseable value is treated as 0.
func parseNumstat(s string) int {
	if s == "-" || s == "" {
		return 0
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// AuthorModules computes one contributor's "Top modules" ranking — codebase areas by
// lines changed (added+deleted), descending — by diffing only their commits. The
// SHAs (from HistoryStats.AuthorSHAs) are fed on stdin, so an arbitrarily prolific
// author can't overflow the argument list, and the set is exactly what the cheap
// walk classified to that bucket (correct even for AI agents). This is the expensive,
// numstat-diffing work kept off the dashboard's whole-repo open path: it runs lazily,
// once per contributor opened. Returns nil for an empty set or on error.
func AuthorModules(repo string, shas []string) []ModuleCount {
	if len(shas) == 0 {
		return nil
	}
	// --no-walk treats each fed SHA individually (no ancestry traversal); the empty
	// pretty-format leaves only the numstat rows in the output.
	cmd := exec.Command("git", "-C", repo, "log", "--no-color", "--no-walk", "--stdin", "--numstat", "--pretty=format:")
	cmd.Stdin = strings.NewReader(strings.Join(shas, "\n"))
	out, err := cmd.Output()
	if err != nil {
		return nil
	}
	return parseAuthorModules(out)
}

// parseAuthorModules aggregates `git log --numstat` output into a module ranking by
// lines changed. It reads every "added\tdeleted\tpath" row (the only non-blank lines
// the empty pretty-format leaves), summing added+deleted into the file's module
// bucket; binary files (numstat "-\t-") and pure renames (0\t0) contribute 0 and so
// don't rank. Pure, so it's testable without a repository.
func parseAuthorModules(data []byte) []ModuleCount {
	byModule := map[string]int{}
	for _, line := range strings.Split(string(data), "\n") {
		if line == "" {
			continue
		}
		ns := strings.SplitN(line, "\t", 3)
		if len(ns) < 3 {
			continue
		}
		key := moduleKey(ns[2])
		if key == "" { // top-level file — no meaningful module, skip it
			continue
		}
		byModule[key] += parseNumstat(ns[0]) + parseNumstat(ns[1])
	}
	mods := make([]ModuleCount, 0, len(byModule))
	for p, n := range byModule {
		if n > 0 {
			mods = append(mods, ModuleCount{Path: p, Lines: n})
		}
	}
	sortModuleCounts(mods)
	return mods
}

// buildDaily flattens the per-day commit map into a contiguous, chronological
// series indexed by day-offset from the earliest day (gaps filled with zero
// days), plus that start and end day. Returns zero times and a nil series when
// empty.
func buildDaily(byDay map[time.Time]*DayCount) (start, end time.Time, daily []DayCount) {
	if len(byDay) == 0 {
		return time.Time{}, time.Time{}, nil
	}
	days := make([]time.Time, 0, len(byDay))
	for d := range byDay {
		days = append(days, d)
	}
	sort.Slice(days, func(i, j int) bool { return days[i].Before(days[j]) })
	start, end = days[0], days[len(days)-1]
	span := int(end.Sub(start).Hours()/24) + 1
	if span < 1 {
		span = 1
	}
	daily = make([]DayCount, span)
	for d, c := range byDay {
		idx := int(d.Sub(start).Hours() / 24)
		if idx >= 0 && idx < span {
			daily[idx] = *c
		}
	}
	return start, end, daily
}

// HourOfDay collapses the punch card into per-hour commit counts summed across
// every weekday — the "what time of day do commits land" distribution.
func (h HistoryStats) HourOfDay() [24]int {
	var hours [24]int
	for d := 0; d < 7; d++ {
		for hr := 0; hr < 24; hr++ {
			hours[hr] += h.Punch[d][hr]
		}
	}
	return hours
}

// WeekdayTotals collapses the punch card into per-weekday commit counts summed
// across every hour, indexed git-style (0=Sunday..6=Saturday) — callers map to
// Mon–Sun for display.
func (h HistoryStats) WeekdayTotals() [7]int {
	var days [7]int
	for d := 0; d < 7; d++ {
		for hr := 0; hr < 24; hr++ {
			days[d] += h.Punch[d][hr]
		}
	}
	return days
}

// Streaks reports the longest run of consecutive calendar days with at least one
// commit, and the current run still alive at the most recent day in the series.
// Both are zero for an empty history.
func (h HistoryStats) Streaks() (longest, current int) {
	run := 0
	for _, d := range h.Daily {
		if d.Human+d.AI > 0 {
			run++
			if run > longest {
				longest = run
			}
		} else {
			run = 0
		}
	}
	return longest, run // run is the trailing streak at the last day
}

// BusiestDay returns the calendar day with the most commits (human+AI) and that
// count. Returns a zero time and 0 for a history with no dated days.
func (h HistoryStats) BusiestDay() (day time.Time, count int) {
	best := -1
	for i, d := range h.Daily {
		if t := d.Human + d.AI; t > count {
			count, best = t, i
		}
	}
	if best < 0 {
		return time.Time{}, 0
	}
	return h.Start.AddDate(0, 0, best), count
}

// CommitsPerWeek is the mean commits per week across the history's span (the
// daily series length), 0 when there are no dated days.
func (h HistoryStats) CommitsPerWeek() float64 {
	if len(h.Daily) == 0 {
		return 0
	}
	weeks := float64(len(h.Daily)) / 7
	if weeks < 1 {
		weeks = 1
	}
	return float64(h.Commits) / weeks
}

// HumanAICommits splits the author ranking into total human- and AI-authored
// commit counts.
func (h HistoryStats) HumanAICommits() (human, ai int) {
	for _, a := range h.Authors {
		if a.AI {
			ai += a.Commits
		} else {
			human += a.Commits
		}
	}
	return human, ai
}

// Concentration measures how unevenly commits spread across authors: the top
// author's share as a rounded percentage, and how many of the top-ranked authors
// it takes to account for more than half of all commits (the "bus factor"). Both
// are 0 for an empty history. Authors is assumed sorted by commit count desc.
func (h HistoryStats) Concentration() (topPct, authorsForMajority int) {
	total := 0
	for _, a := range h.Authors {
		total += a.Commits
	}
	if total == 0 {
		return 0, 0
	}
	topPct = int(float64(h.Authors[0].Commits)/float64(total)*100 + 0.5)
	cum := 0
	for _, a := range h.Authors {
		cum += a.Commits
		authorsForMajority++
		if cum*2 > total {
			break
		}
	}
	return topPct, authorsForMajority
}

// ShortstatOf renders the one-line diff summary — the same shape as
// `git diff --shortstat`, e.g. "5 files changed, 120 insertions(+), 30
// deletions(-)" — from an already-computed change list, so the header summary
// costs no extra git call (the counts ride the numstat ChangedFiles already
// ran). Binary files (Added/Deleted < 0) count toward the file total but
// contribute no line counts, matching git. Empty when there are no changes.
func ShortstatOf(files []FileChange) string {
	if len(files) == 0 {
		return ""
	}
	ins, del := 0, 0
	for _, f := range files {
		if f.Added > 0 {
			ins += f.Added
		}
		if f.Deleted > 0 {
			del += f.Deleted
		}
	}
	parts := []string{plural(len(files), "file") + " changed"}
	if ins > 0 {
		parts = append(parts, plural(ins, "insertion")+"(+)")
	}
	if del > 0 {
		parts = append(parts, plural(del, "deletion")+"(-)")
	}
	return strings.Join(parts, ", ")
}

// plural formats "1 file" / "3 files" — the singular/plural shape git's
// shortstat uses for its file, insertion, and deletion counts.
func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return strconv.Itoa(n) + " " + word + "s"
}

// WorkingCount returns how many files differ from HEAD (staged + unstaged +
// untracked) — the same set ChangedFiles(repo, "HEAD") enumerates, but counted
// with `diff --name-only` plus the untracked listing alone, so it skips the
// second (numstat) diff and the content read of every untracked file that a
// full ChangedFiles incurs. It backs the history view's working-tree row, which
// only needs the count, not the files themselves.
func WorkingCount(repo string) int {
	n := 0
	if out, err := exec.Command("git", "-C", repo, "diff", "--name-only", "HEAD").Output(); err == nil {
		n += nonEmptyLines(out)
	}
	if out, err := exec.Command("git", "-C", repo, "ls-files", "--others", "--exclude-standard").Output(); err == nil {
		n += nonEmptyLines(out)
	}
	return n
}

// nonEmptyLines counts the non-blank newline-separated lines in git output —
// i.e. one per path in a `--name-only` / `ls-files` listing.
func nonEmptyLines(out []byte) int {
	n := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line != "" {
			n++
		}
	}
	return n
}
