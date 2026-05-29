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
)

// Line is one parsed diff line. Text excludes the leading +/-/space marker for
// Add/Del/Context; Hunk and Meta keep their full text. OldNum/NewNum are
// 1-based line numbers in the old/new file, or 0 when not applicable.
type Line struct {
	Kind   Kind
	Text   string
	OldNum int
	NewNum int
}

// Row is one side-by-side row. Full (non-nil) spans both columns (Hunk/Meta).
// Otherwise Left is the old side (Context or Del) and Right the new side
// (Context or Add); either may be nil to pad an unbalanced add/remove block.
type Row struct {
	Full  *Line
	Left  *Line
	Right *Line
}

// Parse splits unified diff output into typed lines, tracking line numbers.
func Parse(raw string) []Line {
	var out []Line
	oldNum, newNum := 0, 0
	for _, ln := range strings.Split(raw, "\n") {
		switch {
		case strings.HasPrefix(ln, "@@"):
			oldNum, newNum = parseHunk(ln)
			out = append(out, Line{Kind: Hunk, Text: ln})
		case isMeta(ln):
			out = append(out, Line{Kind: Meta, Text: ln})
		case strings.HasPrefix(ln, "+"):
			out = append(out, Line{Kind: Add, Text: ln[1:], NewNum: newNum})
			newNum++
		case strings.HasPrefix(ln, "-"):
			out = append(out, Line{Kind: Del, Text: ln[1:], OldNum: oldNum})
			oldNum++
		case strings.HasPrefix(ln, " "):
			out = append(out, Line{Kind: Context, Text: ln[1:], OldNum: oldNum, NewNum: newNum})
			oldNum++
			newNum++
		case ln == "":
			// Trailing newline from Split — skip.
		default:
			out = append(out, Line{Kind: Meta, Text: ln})
		}
	}
	return out
}

// SplitRows converts parsed lines into side-by-side rows, pairing each block of
// consecutive removals with the additions that follow it (GitHub's alignment).
func SplitRows(lines []Line) []Row {
	var rows []Row
	for i := 0; i < len(lines); {
		switch lines[i].Kind {
		case Hunk, Meta:
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
