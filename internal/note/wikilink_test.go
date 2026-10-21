// SPDX-License-Identifier: EUPL-1.2

package note

import (
	"reflect"
	"testing"
)

func TestWikiTargets(t *testing.T) {
	body := "See [[A note]], [[B#Part|label]] and [[ C |x]].\n[[broken\nacross]] [[]] [[ok]]"
	got := WikiTargets(body)
	want := []string{"A note", "B#Part|label", " C |x", "ok"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("WikiTargets = %q, want %q", got, want)
	}
}

func TestLinkName(t *testing.T) {
	tests := map[string]string{
		"A note": "a note", "B#Part|label": "b", " C |x": "c", "D|E#F": "d", "É": "é",
	}
	for in, want := range tests {
		if got := LinkName(in); got != want {
			t.Errorf("LinkName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestStripTracking(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"no section", "A\nSource: x, lines 1-2\n", "A\nSource: x, lines 1-2\n"},
		{"to end of file", "A\nSource: x, lines 1-2\n\n## Tracking\n\n- #1 [[T]] (open) u\n", "A\nSource: x, lines 1-2\n\n"},
		{"until next heading", "A\nSource: x, lines 1-2\n## Tracking\n- #1 [[T]] (open) u\n## Later\n[[X]]\n", "A\nSource: x, lines 1-2\n## Later\n[[X]]\n"},
		{"heading before the source line is content", "## Tracking\n[[X]]\nSource: x, lines 1-2\n", "## Tracking\n[[X]]\nSource: x, lines 1-2\n"},
		{"map without source line", "[[A]]\n## Tracking\n[[X]]\n", "[[A]]\n"},
		{"not a heading prefix", "A\nSource: x, lines 1-2\n## Tracking issues\n[[X]]\n", "A\nSource: x, lines 1-2\n## Tracking issues\n[[X]]\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripTracking(tt.in); got != tt.want {
				t.Errorf("StripTracking = %q, want %q", got, tt.want)
			}
		})
	}
}
