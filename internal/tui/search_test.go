package tui

import (
	"reflect"
	"testing"

	"github.com/trebaud/diffcat/internal/diff"
)

// TestMatchRanges checks case-insensitive substring offsets, including repeated
// and non-overlapping matches.
func TestMatchRanges(t *testing.T) {
	cases := []struct {
		text, query string
		want        [][2]int
	}{
		{"hello world", "o", [][2]int{{4, 5}, {7, 8}}},
		{"FooBarFoo", "foo", [][2]int{{0, 3}, {6, 9}}},
		{"abc", "xyz", nil},
		{"abc", "", nil},
		{"aaaa", "aa", [][2]int{{0, 2}, {2, 4}}}, // non-overlapping
	}
	for _, c := range cases {
		if got := matchRanges(c.text, c.query); !reflect.DeepEqual(got, c.want) {
			t.Errorf("matchRanges(%q, %q) = %v, want %v", c.text, c.query, got, c.want)
		}
	}
}

// TestRecomputeSearch checks that hits are the indices of matching view lines
// (Expand rows skipped) and that searchIdx is clamped into the hit list.
func TestRecomputeSearch(t *testing.T) {
	m := model{
		viewLines: []diff.Line{
			{Kind: diff.Context, Text: "the quick brown fox"},
			{Kind: diff.Add, Text: "a quick patch"},
			{Kind: diff.Expand, Text: "quick", Hidden: 3}, // skipped despite the text
			{Kind: diff.Del, Text: "slow and steady"},
			{Kind: diff.Add, Text: "QUICK uppercase"}, // case-insensitive
		},
	}
	m.searchQuery = "quick"
	m.searchIdx = 99 // out of range → should clamp
	m.recomputeSearch()

	want := []int{0, 1, 4}
	if !reflect.DeepEqual(m.searchHits, want) {
		t.Fatalf("searchHits = %v, want %v", m.searchHits, want)
	}
	if m.searchIdx != 0 {
		t.Errorf("searchIdx should clamp to 0, got %d", m.searchIdx)
	}

	// An empty query clears the hits.
	m.searchQuery = ""
	m.recomputeSearch()
	if len(m.searchHits) != 0 {
		t.Errorf("empty query should clear hits, got %v", m.searchHits)
	}
}

// TestSearchNavigation checks n/N wrap-around over the hit list and that the
// diff cursor follows the current hit (unified mode).
func TestSearchNavigation(t *testing.T) {
	m := model{
		viewLines: []diff.Line{
			{Kind: diff.Context, Text: "match here"},
			{Kind: diff.Context, Text: "no"},
			{Kind: diff.Context, Text: "match again"},
		},
		// A viewport tall enough that ensureCursorVisible doesn't fight the test.
		height: 40,
	}
	m.searchQuery = "match"
	m.recomputeSearch()
	if len(m.searchHits) != 2 {
		t.Fatalf("expected 2 hits, got %v", m.searchHits)
	}

	m.jumpToFirstHit()
	if m.diffCursor != 0 {
		t.Errorf("first hit cursor = %d, want 0", m.diffCursor)
	}
	m.nextHit()
	if m.diffCursor != 2 {
		t.Errorf("after next, cursor = %d, want 2", m.diffCursor)
	}
	m.nextHit() // wraps back to the first
	if m.diffCursor != 0 {
		t.Errorf("next should wrap to first hit (cursor 0), got %d", m.diffCursor)
	}
	m.prevHit() // wraps to the last
	if m.diffCursor != 2 {
		t.Errorf("prev should wrap to last hit (cursor 2), got %d", m.diffCursor)
	}
}
