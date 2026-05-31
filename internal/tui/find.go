package tui

import (
	"sort"
	"strings"
)

// find.go holds the fuzzy file-jump matcher (the `f` picker). It is pure and
// testable: fuzzyFilter ranks changed-file paths against a query. The picker UI
// (fileFindBox) lives in view.go; key handling and jumpToFile in update.go /
// model.go.

// fileFindMatches ranks the changed-file paths against the current picker query.
func (m model) fileFindMatches() []fileMatch {
	paths := make([]string, len(m.files))
	for i, f := range m.files {
		paths[i] = f.Path
	}
	return fuzzyFilter(paths, m.fileFindInput)
}

// jumpToFile selects the tree row for path: it unfolds any collapsed ancestor
// folders (so the row is present in the flattened tree), rebuilds, moves the
// cursor onto the file, focuses the diff pane, and loads its diff. No-op for an
// empty path or one not in the change list.
func (m *model) jumpToFile(path string) {
	if path == "" {
		return
	}
	if m.collapsed == nil {
		m.collapsed = map[string]bool{}
	}
	// Every directory ancestor's node path is a '/'-boundary prefix of the file
	// path, so unfolding all such prefixes clears whatever ancestor was collapsed
	// (deleting a non-existent key is harmless).
	for i := 0; i < len(path); i++ {
		if path[i] == '/' {
			delete(m.collapsed, path[:i])
		}
	}
	m.rebuildTree()
	for i, r := range m.rows {
		if r.file != nil && r.file.Path == path {
			m.cursor = i
			break
		}
	}
	m.focus = focusDiff
	m.loadDiff()
}

// fileMatch is one ranked result: the path, its score (higher = better), and
// the rune offsets within the path that the query matched (for bolding).
type fileMatch struct {
	path  string
	score int
	hit   []int
}

// fuzzyFilter ranks paths against query by subsequence match (each query rune
// appears in order, case-insensitively). An empty query returns every path in
// the original order, unscored. Non-matching paths are dropped. Results are
// sorted by score descending, then path ascending for a stable order.
func fuzzyFilter(paths []string, query string) []fileMatch {
	if strings.TrimSpace(query) == "" {
		out := make([]fileMatch, len(paths))
		for i, p := range paths {
			out[i] = fileMatch{path: p}
		}
		return out
	}
	q := []rune(strings.ToLower(query))
	var out []fileMatch
	for _, p := range paths {
		if score, hit, ok := scorePath(p, q); ok {
			out = append(out, fileMatch{path: p, score: score, hit: hit})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score > out[j].score
		}
		return out[i].path < out[j].path
	})
	return out
}

// scorePath greedily matches the lowercased query runes against path, left to
// right, and scores the match. It rewards contiguous runs (adjacent matched
// runes) and matches that land in the basename (after the last '/'), so typing
// a filename outranks an incidental match buried in a directory name. Returns
// the matched rune offsets (into the original path) and whether all query runes
// were found.
func scorePath(path string, q []rune) (int, []int, bool) {
	pr := []rune(path)
	lower := []rune(strings.ToLower(path))
	baseStart := 0
	if i := strings.LastIndexByte(path, '/'); i >= 0 {
		// Byte index → rune index: ASCII '/' means the rune index equals the count
		// of runes up to it. Recompute defensively for multibyte paths.
		baseStart = len([]rune(path[:i+1]))
	}

	score := 0
	prevMatch := -2
	var hit []int
	qi := 0
	for pi := 0; pi < len(lower) && qi < len(q); pi++ {
		if lower[pi] != q[qi] {
			continue
		}
		hit = append(hit, pi)
		score += 1
		if pi == prevMatch+1 {
			score += 5 // contiguous run — strong signal of a real prefix match
		}
		if pi >= baseStart {
			score += 3 // matched in the filename, not a parent directory
		}
		if pi == baseStart {
			score += 4 // landed on the first char of the basename
		}
		prevMatch = pi
		qi++
	}
	if qi < len(q) {
		return 0, nil, false // ran out of path before consuming the query
	}
	// Shorter paths with the same matches read as closer hits.
	score += max(0, 20-len(pr)/4)
	return score, hit, true
}
