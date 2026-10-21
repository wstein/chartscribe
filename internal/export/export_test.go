// SPDX-License-Identifier: EUPL-1.2

package export

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/wstein/chartscribe/internal/index"
)

func build(t *testing.T, start string, depth int) Result {
	t.Helper()
	return buildIn(t, "basic", start, depth, "")
}

func buildIn(t *testing.T, fixture, start string, depth int, title string) Result {
	t.Helper()
	x, err := index.Load(filepath.Join("..", "..", "testdata", "export", fixture))
	if err != nil {
		t.Fatal(err)
	}
	n, ok := x.Find(start)
	if !ok {
		t.Fatalf("no note %q", start)
	}
	return Build(x, n, depth, title)
}

func titles(r Result) string {
	var out []string
	for _, n := range r.Notes {
		out = append(out, n.Title)
	}
	return strings.Join(out, ",")
}

func TestMapOrderAndDepth(t *testing.T) {
	if got := titles(build(t, "Map - Harbor", 1)); got != "Map - Harbor,Anchor,Chain,Buoy" {
		t.Errorf("depth 1: %s", got)
	}
	if got := titles(build(t, "Map - Harbor", 0)); got != "Map - Harbor" {
		t.Errorf("depth 0: %s", got)
	}
	// Depth 2 follows the links of the members: Lighthouse is linked from Anchor.
	r := build(t, "Map - Harbor", 2)
	if got := titles(r); got != "Map - Harbor,Anchor,Lighthouse,Chain,Buoy" {
		t.Errorf("depth 2: %s", got)
	}
}

func TestRewriteLinks(t *testing.T) {
	md := build(t, "Map - Harbor", 1).Markdown
	for _, want := range []string{
		"[chain link](#chain)",                  // label kept, target in export
		"[Buoy](#buoy)",                         // plain link in export
		"See also Lighthouse and Missing note.", // not in export: plain text
		"[Hook](#anchor)",                       // alias resolves to the note
		"[Anchor](#anchor)",                     // heading part dropped
		"[[Not a link in code]]",                // fenced code is untouched
	} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
	if strings.Contains(md, "[[Anchor") {
		t.Errorf("wikilink left in output:\n%s", md)
	}
}

func TestStructure(t *testing.T) {
	r := build(t, "Map - Harbor", 1)
	md := r.Markdown
	for _, want := range []string{"# Map - Harbor\n", "\n## Anchor\n", "\n### Details\n", "\n## Sources\n"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, bad := range []string{"id:", "tags:", "\n## Links", "\nSource:", "20261021"} {
		if strings.Contains(md, bad) {
			t.Errorf("unexpected %q in:\n%s", bad, md)
		}
	}
	// The duplicated source is listed once, in first-appearance order.
	i1 := strings.Index(md, "- HARBOR.md §1. Anchors, lines 1-9")
	i2 := strings.Index(md, "- HARBOR.md §2. Buoys, lines 10-14")
	if i1 < 0 || i2 < i1 || strings.Count(md, "HARBOR.md §1") != 1 {
		t.Errorf("sources wrong:\n%s", md)
	}
	if len(r.Unresolved) != 1 || r.Unresolved[0] != (Unresolved{"Anchor", "Missing note"}) {
		t.Errorf("unresolved: %v", r.Unresolved)
	}
	if !strings.HasSuffix(md, "\n") || strings.HasSuffix(md, "\n\n") {
		t.Errorf("must end with exactly one newline")
	}
}

func TestDeterministic(t *testing.T) {
	if a, b := build(t, "Map - Harbor", 2).Markdown, build(t, "Map - Harbor", 2).Markdown; a != b {
		t.Error("two builds differ")
	}
}

func TestPlainNoteStart(t *testing.T) {
	// A non-map note is the title and intro; the notes it links to follow.
	if got := titles(build(t, "Buoy", 1)); got != "Buoy,Anchor" {
		t.Errorf("got %s", got)
	}
}

func TestSlug(t *testing.T) {
	for in, want := range map[string]string{
		"Map - Harbor": "map---harbor",
		"Anchor":       "anchor",
		"A, B & C?":    "a-b--c",
		"Überblick 2":  "überblick-2",
	} {
		if got := slug(in); got != want {
			t.Errorf("slug(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCodeAndLinksSectionAreNotMembers(t *testing.T) {
	r := buildIn(t, "code", "Start", 1, "")
	if got := titles(r); got != "Start,Real,InListItem" {
		t.Errorf("members: %s", got)
	}
	if len(r.Unresolved) != 0 {
		t.Errorf("unresolved from code: %v", r.Unresolved)
	}
	for _, keep := range []string{"`[[InInline]]`", "``a `[[InDouble]]` b``", "[[InFence]]", "[[InNested]]", "[[InTilde]]", "    [[InIndented]]", "`[[Ghost]]`"} {
		if !strings.Contains(r.Markdown, keep) {
			t.Errorf("code text %q was changed:\n%s", keep, r.Markdown)
		}
	}
	if strings.Contains(r.Markdown, "Related") {
		t.Errorf("Links section leaked:\n%s", r.Markdown)
	}
}

func TestAnchorsAreUnique(t *testing.T) {
	md := buildIn(t, "anchors", "Start", 1, "").Markdown
	// Alpha's body heading "Part" takes the slug before the note "Part".
	for _, want := range []string{"[Part](#part-1)", "[Foo Bar](#foo-bar)", "[Foo-Bar](#foo-bar-1)", "[Alpha](#alpha)"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
}

func TestBodyHeadingsEndBelowTheSection(t *testing.T) {
	md := buildIn(t, "anchors", "Start", 1, "").Markdown
	// Alpha is a level 2 section; its body "# Top" / "## Part" become ### and ####.
	for _, want := range []string{"\n## Alpha\n", "\n### Top\n", "\n#### Part\n"} {
		if !strings.Contains(md, want) {
			t.Errorf("missing %q in:\n%s", want, md)
		}
	}
}

func TestTitleOverride(t *testing.T) {
	r := buildIn(t, "anchors", "Alpha", 0, "The Report")
	if !strings.HasPrefix(r.Markdown, "# The Report\n") || r.Notes[0].Title != "Alpha" {
		t.Errorf("%s / %v", r.Markdown, r.Notes)
	}
}

func TestFenceInLinksSectionIsDropped(t *testing.T) {
	p := parse("Body\n\n## Links\n\n```\n[[X]]\n```\n\nSource: s, lines 1-2\n\n## After\n")
	var texts []string
	for _, l := range p.lines {
		texts = append(texts, l.text)
	}
	if got := strings.Join(texts, "|"); strings.Contains(got, "```") || strings.Contains(got, "[[X]]") || len(p.targets) != 0 || len(p.sources) != 1 {
		t.Errorf("lines %q targets %v sources %v", got, p.targets, p.sources)
	}
}

func TestMapTextSpans(t *testing.T) {
	up := func(s string) string { return strings.ToUpper(s) }
	for in, want := range map[string]string{
		"a `b` c":       "A `b` C",
		"a ``b ` c`` d": "A ``b ` c`` D",
		"a `b c":        "A `B C",
		"a ``b` c":      "A ``B` C",
		"`a` and `b`":   "`a` AND `b`",
	} {
		if got := mapText(in, up); got != want {
			t.Errorf("mapText(%q) = %q, want %q", in, got, want)
		}
	}
}
