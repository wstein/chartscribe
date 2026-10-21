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

func TestTrackingIgnoresFencedHeading(t *testing.T) {
	fenced := "Text\n\n```md\n## Tracking\n\n- #1 x (open) u\n```\n\nMore\n"
	if got := StripTracking(fenced); got != fenced {
		t.Fatalf("fenced heading stripped: %q", got)
	}
	want := fenced + "\n## Tracking\n\n- #2 B (open) u\n"
	if got := ReplaceTracking(fenced, []string{"- #2 B (open) u"}); got != want {
		t.Fatalf("got %q", got)
	}
	tests := []struct{ name, body, want string }{
		{"tilde and longer closer", "~~~~\n## Tracking\n~~~~~\n\n## Tracking\n\n- a\n", "~~~~\n## Tracking\n~~~~~\n\n"},
		{"shorter closer does not close", "````\n```\n## Tracking\n````\n", "````\n```\n## Tracking\n````\n"},
		{"other char does not close", "```\n~~~\n## Tracking\n```\n", "```\n~~~\n## Tracking\n```\n"},
		{"unclosed fence runs to the end", "```\n## Tracking\n", "```\n## Tracking\n"},
		{"fenced heading ends nothing", "## Tracking\n\n- a\n```\n## Other\n```\n\n## Later\n", "## Later\n"},
		{"indented four spaces is no fence", "    ```\n## Tracking\n\n- a\n", "    ```\n"},
	}
	for _, tt := range tests {
		if got := StripTracking(tt.body); got != tt.want {
			t.Errorf("%s: got %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestStaleTracking(t *testing.T) {
	if !StaleTracking("## Tracking\n\n- a\n\n## Tracking\n\n- b\n") {
		t.Fatal("second section not reported")
	}
	if StaleTracking("## Tracking\n\n- a\n\n```\n## Tracking\n```\n") || StaleTracking("text\n") {
		t.Fatal("false report")
	}
	got := ReplaceTracking("## Tracking\n\n- a\n\n## Tracking\n\n- b\n", []string{"- c"})
	if got != "## Tracking\n\n- c\n\n## Tracking\n\n- b\n" {
		t.Fatalf("first section not replaced alone: %q", got)
	}
}

func TestStripCode(t *testing.T) {
	got := StripCode("a\n```\nChartroom: 1\n```\nb\n")
	if got != "a\nb\n" {
		t.Fatalf("got %q", got)
	}
}
