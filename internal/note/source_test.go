// SPDX-License-Identifier: EUPL-1.2

package note

import "testing"

func TestParseSources(t *testing.T) {
	tests := []struct {
		name, line string
		want       Source
	}{
		{"legacy", "Source: docs/a.md, lines 3-9", Source{Kind: KindFile, Path: "docs/a.md", From: 3, To: 9}},
		{"legacy free text", "Source: IDEAS §1. Direction, lines 1-5", Source{Kind: KindFile, Path: "IDEAS §1. Direction", From: 1, To: 5}},
		{"legacy wins over a kind prefix", "Source: decision: x, lines 1-2", Source{Kind: KindFile, Path: "decision: x", From: 1, To: 2}},
		{"decision", "Source: decision: use Go, 2026-10-22", Source{Kind: KindDecision, Text: "use Go", Date: "2026-10-22"}},
		{"decision bad date", "Source: decision: use Go, 2026-02-30", Source{Kind: KindDecision, Malformed: true}},
		{"decision no date", "Source: decision: use Go", Source{Kind: KindDecision, Malformed: true}},
		{"url", "Source: url: https://example.com/a?b=1", Source{Kind: KindURL, URL: "https://example.com/a?b=1"}},
		{"url ftp", "Source: url: ftp://example.com/a", Source{Kind: KindURL, Malformed: true}},
		{"url space", "Source: url: https://example.com/a b", Source{Kind: KindURL, Malformed: true}},
		{"url no host", "Source: url: https:///a", Source{Kind: KindURL, Malformed: true}},
		{"url empty", "Source: url:", Source{Kind: KindURL, Malformed: true}},
		{"measurement", "Source: measurement: data/run1.csv", Source{Kind: KindMeasurement, Path: "data/run1.csv"}},
		{"measurement key", "Source: measurement: data/run1.csv, p95 latency", Source{Kind: KindMeasurement, Path: "data/run1.csv", Key: "p95 latency"}},
		{"measurement empty", "Source: measurement: ", Source{Kind: KindMeasurement, Malformed: true}},
		{"unknown kind", "Source: note: foo", Source{Kind: KindUnknown, KindName: "note"}},
		{"upper-case kind is unknown", "Source: Decision: x, 2026-10-22", Source{Kind: KindUnknown, KindName: "Decision"}},
	}
	for _, tt := range tests {
		got := ParseSources("Body\n\n" + tt.line + "\n")
		if len(got) != 1 {
			t.Fatalf("%s: got %d sources", tt.name, len(got))
		}
		g := got[0]
		g.Raw = ""
		if g != tt.want {
			t.Errorf("%s: got %+v, want %+v", tt.name, g, tt.want)
		}
	}
}

func TestParseSourcesIgnoresOtherLines(t *testing.T) {
	for _, line := range []string{"Source:", "Source: ", "Source: https://example.com", "Source: no form here", "  Source: x, lines 1-2", "source: x, lines 1-2"} {
		if got := ParseSources(line + "\n"); len(got) != 0 {
			t.Errorf("%q: got %+v", line, got)
		}
	}
}

func TestParseSourcesSeveralInOrder(t *testing.T) {
	got := ParseSources("Source: a.md, lines 1-2\nSource: url: https://x.org\n")
	if len(got) != 2 || got[0].Kind != KindFile || got[1].Kind != KindURL {
		t.Fatalf("got %+v", got)
	}
}

func TestHasSourceAcceptsTypedForms(t *testing.T) {
	if !HasSource("Source: decision: x, 2026-10-22\n") || !HasSource("Source: url: http://a.b\n") {
		t.Error("typed source not accepted")
	}
	if HasSource("Source: decision: x\n") || HasSource("Source: note: x\n") {
		t.Error("malformed source accepted")
	}
}
