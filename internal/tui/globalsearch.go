package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/trebaud/diffcat/internal/diff"
	"github.com/trebaud/diffcat/internal/git"
)

// globalsearch.go is the global search overlay (ctrl+k): one query run across
// several sources at once — commit messages, changed-file paths, and the code in
// the branch diff — with the hits grouped under category headers. It's the
// content-search counterpart to the command palette (`:`, palette.go), which
// searches actions rather than content. Selecting a hit navigates to it (jumping
// to the commit, opening the file, or scrolling the diff to the matched line).
// Rendering is in globalsearch_view.go.

// gsCategory groups results in the overlay; the iota order is the order the
// groups are shown.
type gsCategory int

const (
	gsCommit gsCategory = iota // commit subject / body matches
	gsFile                     // changed-file path matches
	gsCode                     // matches in the branch diff's changed lines
)

// label is the category header shown above its group of results.
func (c gsCategory) label() string {
	switch c {
	case gsCommit:
		return "Commits"
	case gsFile:
		return "Files"
	case gsCode:
		return "Code"
	}
	return ""
}

// gsResult is one hit. title is the primary line (hit marks the matched rune
// range for emphasis); sub is the muted context line beside it. sha/path carry
// what gsActivate needs to navigate to the hit.
type gsResult struct {
	cat   gsCategory
	title string
	sub   string
	hit   [2]int // [start,end) matched rune range in title, or {0,0} when none
	sha   string // commit hit: the commit to jump to
	path  string // file / code hit: the changed file to open
}

// Per-category caps keep the overlay a glanceable summary rather than an
// unbounded dump; the most useful hits (commits, then files) lead.
const (
	gsCommitCap = 5
	gsFileCap   = 5
	gsCodeCap   = 8
)

// gsCodeLine is one searchable changed line in the branch diff: the file it
// belongs to and the line's text (already stripped of its +/- marker by the
// parser). Built once and cached in m.gsCode.
type gsCodeLine struct {
	path string
	text string
}

// gsResults runs the current query across every category and returns the ranked,
// grouped hits (commits, then files, then code), each capped. An empty query
// returns nil so the overlay shows its prompt rather than the whole repo.
func (m *model) gsResults() []gsResult {
	q := strings.TrimSpace(m.gsInput)
	if q == "" {
		return nil
	}
	lq := strings.ToLower(q)
	var out []gsResult

	// Commits — subject or body. The visible title is the subject, so only a
	// subject hit can be emphasised; a body-only match still lists, unmarked.
	for i := range m.commits {
		if countCat(out, gsCommit) >= gsCommitCap {
			break
		}
		c := &m.commits[i]
		if strings.Contains(strings.ToLower(c.Subject), lq) || strings.Contains(strings.ToLower(c.Body), lq) {
			r := gsResult{cat: gsCommit, title: c.Subject, sub: c.Short + " · " + c.Author + " · " + c.Date, sha: c.SHA}
			if rg := matchRanges(c.Subject, q); len(rg) > 0 {
				r.hit = [2]int{rg[0][0], rg[0][1]}
			}
			out = append(out, r)
		}
	}

	// Files — changed-file paths in the branch diff.
	for _, f := range m.branchFileSet() {
		if countCat(out, gsFile) >= gsFileCap {
			break
		}
		if strings.Contains(strings.ToLower(f.Path), lq) {
			r := gsResult{cat: gsFile, title: f.Path, sub: statusWord(f.Status), path: f.Path}
			if rg := matchRanges(f.Path, q); len(rg) > 0 {
				r.hit = [2]int{rg[0][0], rg[0][1]}
			}
			out = append(out, r)
		}
	}

	// Code — changed lines across the branch diff.
	for _, cl := range m.gsCodeLines() {
		if countCat(out, gsCode) >= gsCodeCap {
			break
		}
		if strings.Contains(strings.ToLower(cl.text), lq) {
			title := strings.TrimSpace(cl.text)
			r := gsResult{cat: gsCode, title: title, sub: cl.path, path: cl.path}
			if rg := matchRanges(title, q); len(rg) > 0 {
				r.hit = [2]int{rg[0][0], rg[0][1]}
			}
			out = append(out, r)
		}
	}
	return out
}

// countCat counts how many results already collected fall in category c, so each
// category's cap can be enforced while they're appended to one flat slice.
func countCat(rs []gsResult, c gsCategory) int {
	n := 0
	for _, r := range rs {
		if r.cat == c {
			n++
		}
	}
	return n
}

// gsActivate navigates to the chosen hit and returns any follow-up command. A
// commit hit jumps the history list to it and opens its diff; a file hit opens
// the branch diff on that file; a code hit does the same and then drives the
// in-diff search (`/`) to scroll to and highlight the matched line.
func (m *model) gsActivate(r gsResult) tea.Cmd {
	switch r.cat {
	case gsCommit:
		cmd := m.goHistory()
		m.reselectCommit(r.sha)
		m.openLogDiff()
		return cmd
	case gsFile:
		cmd := m.goBranchDiff()
		m.reselectPath(r.path)
		m.loadDiff()
		return cmd
	case gsCode:
		cmd := m.goBranchDiff()
		m.reselectPath(r.path)
		m.loadDiff()
		q := strings.TrimSpace(m.gsInput)
		m.searchInput = q
		m.searchQuery = q
		m.recomputeSearch()
		m.jumpToFirstHit()
		return cmd
	}
	return nil
}

// branchFileSet returns the branch-vs-base changed files for search, cached for
// the session. It reads them fresh (rather than m.files, which a per-commit
// drill-in repurposes) so search covers the whole branch regardless of the view
// the overlay was opened from. Invalidated by refresh.
func (m *model) branchFileSet() []git.FileChange {
	if m.gsFiles == nil {
		if files, err := git.ChangedFiles(m.repo, m.base); err == nil {
			m.gsFiles = files
		} else {
			m.gsFiles = []git.FileChange{}
		}
	}
	return m.gsFiles
}

// gsCodeLines builds (once, then caches) the flat index of every changed line in
// the branch diff — the added and removed lines across all changed files — that
// the Code category searches. Context lines are skipped: the signal in a diff is
// what changed. Binary files have no textual diff to index. Invalidated by
// refresh.
func (m *model) gsCodeLines() []gsCodeLine {
	if m.gsCode != nil {
		return m.gsCode
	}
	idx := []gsCodeLine{}
	for _, f := range m.branchFileSet() {
		if f.Binary() {
			continue
		}
		raw := git.FileDiff(m.repo, m.base, f.Path, f.Status)
		if raw == "" {
			continue
		}
		for _, l := range diff.Parse(raw) {
			if l.Kind != diff.Add && l.Kind != diff.Del {
				continue
			}
			if strings.TrimSpace(l.Text) == "" {
				continue
			}
			idx = append(idx, gsCodeLine{path: f.Path, text: l.Text})
		}
	}
	m.gsCode = idx
	return idx
}

// statusWord spells out a change's status letter for the Files sub-label.
func statusWord(status string) string {
	switch status {
	case "A":
		return "added"
	case "D":
		return "deleted"
	case "R":
		return "renamed"
	case "C":
		return "copied"
	case "T":
		return "type changed"
	case "?":
		return "untracked"
	default:
		return "modified"
	}
}
