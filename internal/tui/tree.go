package tui

import (
	"sort"
	"strings"

	"github.com/trebaud/diffcat/internal/git"
)

// treeRow is one rendered line of the file tree: either a directory (isDir) or a
// changed file. The flattened slice of treeRows respects collapsed folders, so
// the cursor can index straight into it.
type treeRow struct {
	depth int    // nesting level; drives indentation and the guide rails
	isDir bool   // folder vs. file leaf
	name  string // the segment shown on this row ("internal/tui" when compressed)
	path  string // full repo-relative path of the dir or file

	file *git.FileChange // non-nil for file rows

	added, deleted int  // aggregate for dirs, the file's own counts for files
	binary         bool // file rows only
	status         string

	collapsed bool   // dir rows only: is this folder folded shut
	guides    []bool // per ancestor level: draw a vertical rail (more siblings below)
}

// treeNode is the intermediate tree built from the flat file list before it is
// flattened into display rows. Children are keyed by segment for fast walking
// during construction, then sorted (dirs first, alphabetical) for display.
type treeNode struct {
	name     string
	path     string
	isDir    bool
	file     *git.FileChange
	children map[string]*treeNode

	added, deleted int
}

// buildTree turns the flat changed-file list into a directory tree, compressing
// single-child folder chains (so "internal/tui/view.go" nests as "internal/tui"
// → "view.go" rather than three levels) and summing line counts up to folders.
func buildTree(files []git.FileChange) *treeNode {
	root := &treeNode{children: map[string]*treeNode{}, isDir: true}
	for i := range files {
		f := &files[i]
		parts := strings.Split(f.Path, "/")
		cur := root
		for j, seg := range parts {
			child, ok := cur.children[seg]
			if !ok {
				child = &treeNode{
					name:     seg,
					path:     strings.Join(parts[:j+1], "/"),
					children: map[string]*treeNode{},
				}
				cur.children[seg] = child
			}
			if j == len(parts)-1 {
				child.file = f
				child.isDir = false
			} else {
				child.isDir = true
			}
			cur = child
		}
	}
	compress(root)
	aggregate(root)
	return root
}

// compress folds chains of single-child directories into one node so deep paths
// read as "a/b/c" on a single row instead of nesting three times — the GitHub /
// VS Code explorer behavior, which keeps the tree shallow and the names short.
func compress(n *treeNode) {
	for _, c := range n.children {
		for c.isDir && len(c.children) == 1 {
			var only *treeNode
			for _, v := range c.children {
				only = v
			}
			if !only.isDir {
				break
			}
			c.name += "/" + only.name
			c.path = only.path
			c.children = only.children
		}
		compress(c)
	}
}

// aggregate sums each directory's descendant line counts (binaries contribute 0)
// so a folder row can show a roll-up "+a -d".
func aggregate(n *treeNode) (added, deleted int) {
	if !n.isDir && n.file != nil {
		if n.file.Binary() {
			return 0, 0
		}
		return n.file.Added, n.file.Deleted
	}
	for _, c := range n.children {
		a, d := aggregate(c)
		n.added += a
		n.deleted += d
		added += a
		deleted += d
	}
	return added, deleted
}

// flattenTree walks the tree depth-first into the visible row slice, skipping the
// children of collapsed folders. guides carries, for each ancestor level, whether
// a sibling still follows below (so the renderer can draw a continuing rail).
func flattenTree(n *treeNode, collapsed map[string]bool, depth int, guides []bool, out *[]treeRow) {
	kids := sortedChildren(n)
	for i, c := range kids {
		hasNext := i < len(kids)-1
		if c.isDir {
			fold := collapsed[c.path]
			*out = append(*out, treeRow{
				depth:     depth,
				isDir:     true,
				name:      c.name,
				path:      c.path,
				added:     c.added,
				deleted:   c.deleted,
				collapsed: fold,
				guides:    append([]bool(nil), guides...),
			})
			if !fold {
				flattenTree(c, collapsed, depth+1, append(guides, hasNext), out)
			}
			continue
		}
		*out = append(*out, treeRow{
			depth:   depth,
			name:    c.name,
			path:    c.path,
			file:    c.file,
			added:   c.file.Added,
			deleted: c.file.Deleted,
			binary:  c.file.Binary(),
			status:  c.file.Status,
			guides:  append([]bool(nil), guides...),
		})
	}
}

// sortedChildren orders a node's children for display: directories first, then
// files, each group alphabetical (case-insensitive) — matching how source hosts
// present file trees.
func sortedChildren(n *treeNode) []*treeNode {
	out := make([]*treeNode, 0, len(n.children))
	for _, c := range n.children {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].isDir != out[j].isDir {
			return out[i].isDir // dirs before files
		}
		return strings.ToLower(out[i].name) < strings.ToLower(out[j].name)
	})
	return out
}
