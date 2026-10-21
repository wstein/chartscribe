// SPDX-License-Identifier: EUPL-1.2

package note

import (
	"regexp"
	"strconv"
	"time"
)

var idPattern = regexp.MustCompile(`^[0-9]{14}(-[0-9]{3})?$`)

// ValidID reports whether id is 14 digits YYYYMMDDHHMMSS naming a real
// calendar date and time, with an optional -NNN suffix.
func ValidID(id string) bool {
	if !idPattern.MatchString(id) {
		return false
	}
	n := func(from, to int) int { v, _ := strconv.Atoi(id[from:to]); return v }
	year, month, day, hour, min, sec := n(0, 4), n(4, 6), n(6, 8), n(8, 10), n(10, 12), n(12, 14)
	if year < 1 {
		return false
	}
	t := time.Date(year, time.Month(month), day, hour, min, sec, 0, time.UTC)
	return t.Year() == year && int(t.Month()) == month && t.Day() == day &&
		t.Hour() == hour && t.Minute() == min && t.Second() == sec
}
