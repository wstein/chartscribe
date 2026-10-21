// SPDX-License-Identifier: EUPL-1.2

package note

import "testing"

func TestReplaceTracking(t *testing.T) {
	lines := []string{"- #1 A (open) https://x/1", "- #2 B (closed) https://x/2"}
	tests := []struct {
		name, body string
		lines      []string
		want       string
	}{
		{"append", "Text\n\nSource: s, lines 1-2\n", lines,
			"Text\n\nSource: s, lines 1-2\n\n## Tracking\n\n- #1 A (open) https://x/1\n- #2 B (closed) https://x/2\n"},
		{"append without final newline", "Text", lines[:1],
			"Text\n\n## Tracking\n\n- #1 A (open) https://x/1\n"},
		{"append after blank line", "Text\n\n", lines[:1],
			"Text\n\n## Tracking\n\n- #1 A (open) https://x/1\n"},
		{"replace at end", "Text\n\n## Tracking\n\n- #9 old (open) u\n", lines[:1],
			"Text\n\n## Tracking\n\n- #1 A (open) https://x/1\n"},
		{"replace before heading", "Text\n\n## Tracking\n\n- #9 old (open) u\n\n## Later\n\nmore\n", lines[:1],
			"Text\n\n## Tracking\n\n- #1 A (open) https://x/1\n\n## Later\n\nmore\n"},
		{"remove", "Text\n\n## Tracking\n\n- #9 old (open) u\n\n## Later\n", nil,
			"Text\n\n## Later\n"},
		{"remove at end", "Text\n\n## Tracking\n\n- #9 old (open) u\n", nil, "Text\n\n"},
		{"nothing to remove", "Text\n", nil, "Text\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ReplaceTracking(tt.body, tt.lines)
			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
			if again := ReplaceTracking(got, tt.lines); again != got {
				t.Fatalf("not idempotent: %q", again)
			}
		})
	}
}
