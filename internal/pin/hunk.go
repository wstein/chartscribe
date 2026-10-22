// SPDX-License-Identifier: EUPL-1.2

// Package pin classifies the line range of a pinned file Source (`Source: <path>, lines A-B @<commit>`)
// against the current file: it asks read-only git for the changes since the pinned commit, maps the
// range through them and checks the cited text. See specs/001-note-tooling/contracts/sources.md.
package pin

import (
	"fmt"
	"regexp"
	"strconv"
)

// Status is the state of one pinned range against the file now.
type Status string

// The states. Current, Moved and Changed come from the hunk mapping; the others are decided before it.
const (
	Current  Status = "current"  // the range is where it was and holds the same text
	Moved    Status = "moved"    // the same text, at another offset
	Changed  Status = "changed"  // the text of the range was edited: manual review
	Renamed  Status = "renamed"  // the file has another path now
	Missing  Status = "missing"  // the file does not exist now
	Unknown  Status = "unknown"  // git could not tell; Reason says why
	Unpinned Status = "unpinned" // the source has no pin
)

// Hunk is one `@@ -OldStart,OldCount +NewStart,NewCount @@` header of `git diff -U0`.
// With a count of 0 the start is the line after which the change happens.
type Hunk struct{ OldStart, OldCount, NewStart, NewCount int }

var hunkHeader = regexp.MustCompile(`^@@ -([0-9]+)(?:,([0-9]+))? \+([0-9]+)(?:,([0-9]+))? @@`)

// ParseHunkHeader parses a hunk header line of a two-tree diff.
func ParseHunkHeader(line string) (Hunk, bool) {
	m := hunkHeader.FindStringSubmatch(line)
	if m == nil {
		return Hunk{}, false
	}
	num := func(s string, def int) int {
		if s == "" {
			return def
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return -1
		}
		return n
	}
	h := Hunk{num(m[1], 0), num(m[2], 1), num(m[3], 0), num(m[4], 1)}
	if h.OldStart < 0 || h.OldCount < 0 || h.NewStart < 0 || h.NewCount < 0 {
		return Hunk{}, false
	}
	return h, true
}

// Header renders the hunk the way git does (a count of 1 is left out).
func (h Hunk) Header() string {
	side := func(start, count int) string {
		if count == 1 {
			return strconv.Itoa(start)
		}
		return fmt.Sprintf("%d,%d", start, count)
	}
	return "@@ -" + side(h.OldStart, h.OldCount) + " +" + side(h.NewStart, h.NewCount) + " @@"
}

// Mapping is the outcome of MapRange. From and To are the range in the new file (Current, Moved);
// Overlap lists the hunks that touch the range (Changed).
type Mapping struct {
	Status   Status
	From, To int
	Overlap  []Hunk
}

// MapRange maps the old lines from..to through hunks (ascending, as git writes them):
//
//	A hunk with old lines (OldCount > 0) that include a line of the range changes it.
//	A pure insertion after old line x lies inside the range when from <= x < to; before it when x < from.
//	Every hunk that ends before the range shifts it by NewCount - OldCount; hunks after it do not matter.
//
// No hunk touching the range and no shift is Current; a shift is Moved.
func MapRange(hunks []Hunk, from, to int) Mapping {
	shift := 0
	var overlap []Hunk
	for _, h := range hunks {
		if h.OldCount == 0 {
			switch {
			case h.OldStart < from:
				shift += h.NewCount
			case h.OldStart < to:
				overlap = append(overlap, h)
			}
			continue
		}
		last := h.OldStart + h.OldCount - 1
		switch {
		case last < from:
			shift += h.NewCount - h.OldCount
		case h.OldStart <= to:
			overlap = append(overlap, h)
		}
	}
	if len(overlap) > 0 {
		return Mapping{Status: Changed, Overlap: overlap}
	}
	if shift == 0 {
		return Mapping{Status: Current, From: from, To: to}
	}
	return Mapping{Status: Moved, From: from + shift, To: to + shift}
}
