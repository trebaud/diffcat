// Package diff parses unified `git diff` output into typed, line-numbered rows
// so the TUI can render it GitHub-style: full-row add/remove tints in a unified
// view, or removed-left / added-right in a side-by-side split.
package diff

import (
	"strconv"
	"strings"
)

// Kind classifies a diff line.
type Kind int

const (
	Context Kind = iota // unchanged line, present on both sides
	Add                 // added line (+)
	Del                 // removed line (-)
	Hunk                // @@ -a,b +c,d @@ header
	Meta                // diff/index/file headers, "\ No newline", etc.
	Expand              // synthetic affordance row to reveal hidden context
)

// Dir is the direction an Expand row reveals hidden context.
type Dir int

const (
	ExpandAll  Dir = iota // small gap: one press reveals all of it
	ExpandUp              // reveal lines just above the next change
	ExpandDown            // reveal lines just below the previous change
)

// Line is one parsed diff line. Text excludes the leading +/-/space marker for
// Add/Del/Context; Hunk and Meta keep their full text. OldNum/NewNum are
// 1-based line numbers in the old/new file, or 0 when not applicable. Path is
// the file the line belongs to (carried from the surrounding `diff --git`/`+++`
// headers) so a combined multi-file patch can be syntax-highlighted per file.
// Dir/GapID/Hidden are only meaningful on Expand rows.
type Line struct {
	Kind   Kind
	Text   string
	OldNum int
	NewNum int
	Path   string

	// Emph are the rune ranges within Text that changed at the word level,
	// computed by pairing this line with its counterpart (a Del with the Add that
	// replaced it). Each is a [start, end) half-open rune offset into Text. Only
	// set on Add/Del lines that have a sufficiently similar counterpart; nil
	// otherwise (so the renderer falls back to a flat full-line tint). This is
	// GitHub's two-tone diff: a soft tint on the whole line, a stronger one on the
	// exact words that differ.
	Emph [][2]int

	Dir    Dir // Expand only: which direction this affordance reveals
	GapID  int // Expand only: index into the gap list it belongs to
	Hidden int // Expand only: count of still-hidden lines it covers
}

// Gap is a run of unchanged lines git omitted from the diff: before the first
// hunk, between hunks, or after the last one. Ranges are 1-based inclusive in
// both old and new coordinates (equal length). At is the index in the pristine
// line slice where the affordance belongs — right before that line, or
// len(lines) for the trailing gap.
type Gap struct {
	OldStart, OldEnd int
	NewStart, NewEnd int
	At               int
}

// Row is one side-by-side row. Full (non-nil) spans both columns (Hunk/Meta).
// Otherwise Left is the old side (Context or Del) and Right the new side
// (Context or Add); either may be nil to pad an unbalanced add/remove block.
type Row struct {
	Full  *Line
	Left  *Line
	Right *Line
}

// Parse splits unified diff output into typed lines, tracking line numbers and
// the file each line belongs to. path follows the `diff --git`/`---`/`+++`
// headers so a combined multi-file patch (e.g. `git show`) carries a per-line
// Path the caller can highlight by.
func Parse(raw string) []Line {
	var out []Line
	oldNum, newNum := 0, 0
	path := ""
	for _, ln := range strings.Split(raw, "\n") {
		switch {
		case strings.HasPrefix(ln, "diff --git"):
			path = "" // new file section; resolved from the --- / +++ headers
		case strings.HasPrefix(ln, "--- "), strings.HasPrefix(ln, "+++ "):
			if p := pathFromHeader(ln); p != "" {
				path = p
			}
		}
		switch {
		case strings.HasPrefix(ln, "@@"):
			oldNum, newNum = parseHunk(ln)
			out = append(out, Line{Kind: Hunk, Text: ln, Path: path})
		case isMeta(ln):
			out = append(out, Line{Kind: Meta, Text: ln, Path: path})
		case strings.HasPrefix(ln, "+"):
			out = append(out, Line{Kind: Add, Text: ln[1:], NewNum: newNum, Path: path})
			newNum++
		case strings.HasPrefix(ln, "-"):
			out = append(out, Line{Kind: Del, Text: ln[1:], OldNum: oldNum, Path: path})
			oldNum++
		case strings.HasPrefix(ln, " "):
			out = append(out, Line{Kind: Context, Text: ln[1:], OldNum: oldNum, NewNum: newNum, Path: path})
			oldNum++
			newNum++
		case ln == "":
			// Trailing newline from Split — skip.
		default:
			out = append(out, Line{Kind: Meta, Text: ln, Path: path})
		}
	}
	markWordDiff(out)
	return out
}

// pathFromHeader extracts the file path from a `--- a/path` or `+++ b/path`
// header line, stripping the a//b/ prefix and any trailing tab-delimited
// timestamp. Returns "" for /dev/null (an added or deleted side).
func pathFromHeader(ln string) string {
	s := ln[4:] // after "--- " / "+++ "
	if i := strings.IndexByte(s, '\t'); i >= 0 {
		s = s[:i]
	}
	if s == "/dev/null" {
		return ""
	}
	s = strings.TrimPrefix(s, "a/")
	s = strings.TrimPrefix(s, "b/")
	return s
}

// SplitRows converts parsed lines into side-by-side rows, pairing each block of
// consecutive removals with the additions that follow it (GitHub's alignment).
func SplitRows(lines []Line) []Row {
	var rows []Row
	for i := 0; i < len(lines); {
		switch lines[i].Kind {
		case Hunk, Meta, Expand:
			rows = append(rows, Row{Full: &lines[i]})
			i++
		case Context:
			rows = append(rows, Row{Left: &lines[i], Right: &lines[i]})
			i++
		default: // a run of Del/Add
			var dels, adds []*Line
			for ; i < len(lines) && (lines[i].Kind == Del || lines[i].Kind == Add); i++ {
				if lines[i].Kind == Del {
					dels = append(dels, &lines[i])
				} else {
					adds = append(adds, &lines[i])
				}
			}
			n := len(dels)
			if len(adds) > n {
				n = len(adds)
			}
			for k := 0; k < n; k++ {
				var row Row
				if k < len(dels) {
					row.Left = dels[k]
				}
				if k < len(adds) {
					row.Right = adds[k]
				}
				rows = append(rows, row)
			}
		}
	}
	return rows
}

// markWordDiff fills in the per-line Emph ranges by pairing each removed line
// with the added line that replaced it (the k-th Del with the k-th Add in a
// consecutive Del/Add block — the same pairing SplitRows uses) and running a
// token-level diff between the two. The tokens absent from their longest common
// subsequence are the words that changed, and their rune ranges become Emph.
func markWordDiff(lines []Line) {
	for i := 0; i < len(lines); {
		if lines[i].Kind != Del && lines[i].Kind != Add {
			i++
			continue
		}
		start := i
		for i < len(lines) && (lines[i].Kind == Del || lines[i].Kind == Add) {
			i++
		}
		markBlock(lines[start:i])
	}
}

// markBlock pairs the removals and additions in one Del/Add run positionally and
// marks each pair's intra-line word diff.
func markBlock(block []Line) {
	var dels, adds []*Line
	for k := range block {
		switch block[k].Kind {
		case Del:
			dels = append(dels, &block[k])
		case Add:
			adds = append(adds, &block[k])
		}
	}
	n := len(dels)
	if len(adds) < n {
		n = len(adds)
	}
	for k := 0; k < n; k++ {
		markPair(dels[k], adds[k])
	}
}

// markPair computes the word-level diff between a removed and an added line. It
// skips lines that share too little — a wholesale replacement reads better as a
// plain full-line tint than as two lines lit up almost end to end.
func markPair(del, add *Line) {
	oldTok := tokenize(del.Text)
	newTok := tokenize(add.Text)
	oldR, newR := wordRanges(oldTok, newTok)

	// Similarity gate: if the unchanged remainder is a tiny fraction of the
	// longer line, the two lines are effectively unrelated — leave Emph nil.
	oldLen, newLen := len([]rune(del.Text)), len([]rune(add.Text))
	longer := oldLen
	if newLen > longer {
		longer = newLen
	}
	common := oldLen - sumRanges(oldR)
	if longer > 0 && float64(common)/float64(longer) < 0.25 {
		return
	}
	del.Emph, add.Emph = oldR, newR
}

// token is one unit of the word diff: a run of word characters, a run of
// whitespace, or a single other rune. start/end are its rune offsets in the line.
type token struct {
	text       string
	start, end int
}

// tokenize splits a line into word/whitespace/punctuation tokens so the diff
// aligns on word boundaries rather than individual characters.
func tokenize(s string) []token {
	runes := []rune(s)
	var toks []token
	for i := 0; i < len(runes); {
		j := i
		switch {
		case isWordRune(runes[i]):
			for j < len(runes) && isWordRune(runes[j]) {
				j++
			}
		case runes[i] == ' ' || runes[i] == '\t':
			for j < len(runes) && (runes[j] == ' ' || runes[j] == '\t') {
				j++
			}
		default:
			j = i + 1
		}
		toks = append(toks, token{text: string(runes[i:j]), start: i, end: j})
		i = j
	}
	return toks
}

func isWordRune(r rune) bool {
	return r == '_' ||
		(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') || r >= 0x80
}

// wordRanges runs a longest-common-subsequence diff over two token slices and
// returns the rune ranges of the tokens that fall outside the LCS on each side —
// the deletions (oldR) and insertions (newR). Adjacent ranges are merged.
func wordRanges(old, new []token) (oldR, newR [][2]int) {
	a, b := len(old), len(new)
	// dp[i][j] = LCS length of old[i:] and new[j:].
	dp := make([][]int, a+1)
	for i := range dp {
		dp[i] = make([]int, b+1)
	}
	for i := a - 1; i >= 0; i-- {
		for j := b - 1; j >= 0; j-- {
			if old[i].text == new[j].text {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}
	i, j := 0, 0
	for i < a && j < b {
		switch {
		case old[i].text == new[j].text:
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			oldR = appendRange(oldR, old[i])
			i++
		default:
			newR = appendRange(newR, new[j])
			j++
		}
	}
	for ; i < a; i++ {
		oldR = appendRange(oldR, old[i])
	}
	for ; j < b; j++ {
		newR = appendRange(newR, new[j])
	}
	return oldR, newR
}

// appendRange adds a token's range, coalescing with the previous range when they
// abut so consecutive changed tokens render as one continuous band.
func appendRange(rs [][2]int, t token) [][2]int {
	if n := len(rs); n > 0 && rs[n-1][1] == t.start {
		rs[n-1][1] = t.end
		return rs
	}
	return append(rs, [2]int{t.start, t.end})
}

// sumRanges totals the rune length covered by a set of ranges.
func sumRanges(rs [][2]int) int {
	n := 0
	for _, r := range rs {
		n += r[1] - r[0]
	}
	return n
}

// Gaps finds the runs of unchanged lines git omitted: before the first hunk,
// between hunks, and after the last one. newFileLineCount is the length of the
// new-side (working-tree) file. Returns nil when there are no hunks (binary or
// empty diff) or no new-side content (a pure deletion).
func Gaps(lines []Line, newFileLineCount int) []Gap {
	if newFileLineCount <= 0 {
		return nil
	}
	var gaps []Gap
	lastOld, lastNew := 0, 0
	seenHunk := false
	for i, l := range lines {
		switch l.Kind {
		case Hunk:
			seenHunk = true
			nextOld, nextNew := parseHunk(l.Text)
			addGap(&gaps, lastOld+1, nextOld-1, lastNew+1, nextNew-1, i)
		case Context:
			lastOld, lastNew = l.OldNum, l.NewNum
		case Add:
			lastNew = l.NewNum
		case Del:
			lastOld = l.OldNum
		}
	}
	if seenHunk {
		// The trailing unchanged region keeps a constant old↔new offset, so the
		// old end follows from the new end.
		offset := lastNew - lastOld
		addGap(&gaps, lastOld+1, newFileLineCount-offset, lastNew+1, newFileLineCount, len(lines))
	}
	return gaps
}

// addGap appends a gap only if it is a non-empty, well-formed hidden range. It
// drops new-file leading regions (old start ≤ 0) and parse anomalies where the
// old and new spans disagree in length.
func addGap(gaps *[]Gap, oldStart, oldEnd, newStart, newEnd, at int) {
	if newStart < 1 || newStart > newEnd {
		return
	}
	if oldStart < 1 || oldEnd-oldStart != newEnd-newStart {
		return
	}
	*gaps = append(*gaps, Gap{OldStart: oldStart, OldEnd: oldEnd, NewStart: newStart, NewEnd: newEnd, At: at})
}

// BuildView interleaves the pristine diff with revealed context lines and the
// residual expand affordances. revealed[i] = [fromTop, fromBottom] records how
// many lines of gaps[i] have been revealed from each end; window is the
// lines-per-press step and the threshold below which a gap collapses to a single
// "expand all" row. fileLines is the new-side file, indexed 1-based via [n-1].
func BuildView(lines []Line, fileLines []string, gaps []Gap, revealed map[int][2]int, window int) []Line {
	if len(gaps) == 0 {
		return lines
	}
	var out []Line
	gi := 0
	for i := 0; i <= len(lines); i++ {
		for gi < len(gaps) && gaps[gi].At == i {
			out = append(out, expandRows(gaps[gi], gi, fileLines, revealed[gi], window)...)
			gi++
		}
		if i < len(lines) {
			out = append(out, lines[i])
		}
	}
	return out
}

// expandRows renders one gap into revealed context lines (top + bottom) framing
// the residual affordance row(s) for whatever stays hidden in the middle.
func expandRows(g Gap, id int, fileLines []string, rev [2]int, window int) []Line {
	total := g.NewEnd - g.NewStart + 1
	top, bottom := clampReveal(rev[0], rev[1], total)
	offset := g.NewStart - g.OldStart // new = old + offset within the gap

	ctx := func(n int) Line {
		text := ""
		if n-1 >= 0 && n-1 < len(fileLines) {
			text = fileLines[n-1]
		}
		return Line{Kind: Context, Text: text, OldNum: n - offset, NewNum: n}
	}

	var out []Line
	for n := g.NewStart; n < g.NewStart+top; n++ { // revealed below the previous change
		out = append(out, ctx(n))
	}
	if remaining := total - top - bottom; remaining > 0 {
		if remaining <= window {
			out = append(out, Line{Kind: Expand, Dir: ExpandAll, GapID: id, Hidden: remaining})
		} else {
			out = append(out, Line{Kind: Expand, Dir: ExpandDown, GapID: id, Hidden: remaining})
			out = append(out, Line{Kind: Expand, Dir: ExpandUp, GapID: id, Hidden: remaining})
		}
	}
	for n := g.NewEnd - bottom + 1; n <= g.NewEnd; n++ { // revealed above the next change
		out = append(out, ctx(n))
	}
	return out
}

// clampReveal keeps the revealed counts non-negative and within the gap, giving
// the top reveal priority if the two ends would overlap.
func clampReveal(top, bottom, total int) (int, int) {
	if top < 0 {
		top = 0
	}
	if bottom < 0 {
		bottom = 0
	}
	if top > total {
		top = total
	}
	if top+bottom > total {
		bottom = total - top
	}
	return top, bottom
}

func parseHunk(s string) (oldStart, newStart int) {
	oldStart, newStart = 1, 1
	for _, p := range strings.Fields(s) {
		switch {
		case strings.HasPrefix(p, "-"):
			oldStart = leadingInt(p[1:])
		case strings.HasPrefix(p, "+"):
			newStart = leadingInt(p[1:])
		}
	}
	return
}

// leadingInt parses the start from a "start,count" or "start" hunk range.
func leadingInt(s string) int {
	if i := strings.IndexByte(s, ','); i >= 0 {
		s = s[:i]
	}
	n, _ := strconv.Atoi(s)
	return n
}

func isMeta(ln string) bool {
	prefixes := []string{
		"diff ", "index ", "--- ", "+++ ", "new file", "deleted file",
		"rename ", "copy ", "similarity ", "dissimilarity ",
		"old mode", "new mode", "Binary files", "\\ No newline",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(ln, p) {
			return true
		}
	}
	return false
}
