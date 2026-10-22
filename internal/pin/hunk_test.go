// SPDX-License-Identifier: EUPL-1.2

package pin

import "testing"

func TestParseHunkHeader(t *testing.T) {
	tests := []struct {
		line string
		want Hunk
		ok   bool
	}{
		{"@@ -3 +3 @@", Hunk{3, 1, 3, 1}, true},
		{"@@ -3,2 +4,5 @@ func x()", Hunk{3, 2, 4, 5}, true},
		{"@@ -2,0 +3,2 @@", Hunk{2, 0, 3, 2}, true},
		{"@@ -1,2 +0,0 @@", Hunk{1, 2, 0, 0}, true},
		{"@@ -0,0 +1,3 @@", Hunk{0, 0, 1, 3}, true},
		{"@@@ -1,2 -1,2 +1,2 @@@", Hunk{}, false}, // combined diff, never produced for two trees
		{"+@@ -1 +1 @@", Hunk{}, false},
		{"@@ -x +1 @@", Hunk{}, false},
		{"", Hunk{}, false},
	}
	for _, tt := range tests {
		got, ok := ParseHunkHeader(tt.line)
		if ok != tt.ok || got != tt.want {
			t.Errorf("%q: got %+v %v, want %+v %v", tt.line, got, ok, tt.want, tt.ok)
		}
	}
}

func TestHeaderRoundTrip(t *testing.T) {
	for _, line := range []string{"@@ -3 +3 @@", "@@ -3,2 +4,5 @@", "@@ -2,0 +3,2 @@", "@@ -1,2 +0,0 @@"} {
		h, ok := ParseHunkHeader(line)
		if !ok || h.Header() != line {
			t.Errorf("%q -> %+v -> %q", line, h, h.Header())
		}
	}
}

func TestMapRange(t *testing.T) {
	// The range under test is lines 5-7 of the pinned file unless the case says otherwise.
	tests := []struct {
		name     string
		hunks    []Hunk
		from, to int
		status   Status
		nf, nt   int
	}{
		{"no hunks", nil, 5, 7, Current, 5, 7},
		{"insert before", []Hunk{{2, 0, 3, 2}}, 5, 7, Moved, 7, 9},
		{"insert directly before the range", []Hunk{{4, 0, 5, 1}}, 5, 7, Moved, 6, 8},
		{"insert after line 1 of the range", []Hunk{{5, 0, 6, 1}}, 5, 7, Changed, 0, 0},
		{"insert inside", []Hunk{{6, 0, 7, 3}}, 5, 7, Changed, 0, 0},
		{"insert after the last line of the range", []Hunk{{7, 0, 8, 2}}, 5, 7, Current, 5, 7},
		{"insert after the range", []Hunk{{9, 0, 10, 2}}, 5, 7, Current, 5, 7},
		{"insert at file start", []Hunk{{0, 0, 1, 1}}, 5, 7, Moved, 6, 8},
		{"delete before", []Hunk{{1, 2, 0, 0}}, 5, 7, Moved, 3, 5},
		{"delete directly before", []Hunk{{4, 1, 3, 0}}, 5, 7, Moved, 4, 6},
		{"delete inside", []Hunk{{6, 1, 5, 0}}, 5, 7, Changed, 0, 0},
		{"delete the first line", []Hunk{{5, 1, 4, 0}}, 5, 7, Changed, 0, 0},
		{"delete the last line", []Hunk{{7, 1, 6, 0}}, 5, 7, Changed, 0, 0},
		{"delete the whole range", []Hunk{{5, 3, 4, 0}}, 5, 7, Changed, 0, 0},
		{"delete after", []Hunk{{8, 1, 7, 0}}, 5, 7, Current, 5, 7},
		{"replace inside", []Hunk{{6, 1, 6, 1}}, 5, 7, Changed, 0, 0},
		{"replace overlapping the start", []Hunk{{3, 3, 3, 1}}, 5, 7, Changed, 0, 0},
		{"replace before with more lines", []Hunk{{2, 1, 2, 4}}, 5, 7, Moved, 8, 10},
		{"replace before with fewer lines", []Hunk{{2, 3, 2, 1}}, 5, 7, Moved, 3, 5},
		{"replace before, same length", []Hunk{{2, 2, 2, 2}}, 5, 7, Current, 5, 7},
		{"replace after", []Hunk{{8, 2, 8, 5}}, 5, 7, Current, 5, 7},
		{"two hunks before add up", []Hunk{{1, 0, 2, 3}, {3, 2, 6, 0}}, 5, 7, Moved, 6, 8},
		{"two hunks before cancel out", []Hunk{{1, 0, 2, 2}, {3, 2, 5, 0}}, 5, 7, Current, 5, 7},
		{"before and after", []Hunk{{1, 0, 2, 1}, {9, 1, 10, 0}}, 5, 7, Moved, 6, 8},
		{"before then inside", []Hunk{{1, 0, 2, 1}, {6, 1, 7, 1}}, 5, 7, Changed, 0, 0},
		{"single line, insert before", []Hunk{{4, 0, 5, 1}}, 5, 5, Moved, 6, 6},
		{"single line, insert after", []Hunk{{5, 0, 6, 1}}, 5, 5, Current, 5, 5},
		{"single line, replaced", []Hunk{{5, 1, 5, 1}}, 5, 5, Changed, 0, 0},
		{"range at file start", []Hunk{{3, 0, 4, 1}}, 1, 2, Current, 1, 2},
		{"range at file start, insert at 0", []Hunk{{0, 0, 1, 1}}, 1, 2, Moved, 2, 3},
		{"whole file rewritten", []Hunk{{1, 10, 1, 12}}, 5, 7, Changed, 0, 0},
		{"four lines inserted at the start", []Hunk{{0, 0, 1, 4}}, 1, 1, Moved, 5, 5},
	}
	for _, tt := range tests {
		got := MapRange(tt.hunks, tt.from, tt.to)
		if got.Status != tt.status || got.From != tt.nf || got.To != tt.nt {
			t.Errorf("%s: got %+v, want %s %d-%d", tt.name, got, tt.status, tt.nf, tt.nt)
		}
		if got.Status == Changed && len(got.Overlap) == 0 {
			t.Errorf("%s: changed without the hunk that changed it", tt.name)
		}
	}
}

func TestMapRangeNamesTheHunksThatTouchTheRange(t *testing.T) {
	got := MapRange([]Hunk{{1, 0, 2, 1}, {6, 1, 7, 1}, {7, 0, 8, 1}, {20, 1, 21, 0}}, 5, 8)
	if got.Status != Changed || len(got.Overlap) != 2 || got.Overlap[0] != (Hunk{6, 1, 7, 1}) || got.Overlap[1] != (Hunk{7, 0, 8, 1}) {
		t.Errorf("got %+v", got)
	}
}
