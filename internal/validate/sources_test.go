// SPDX-License-Identifier: EUPL-1.2

package validate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func pairs(res Result) []string {
	out := []string{}
	for _, f := range res.Findings {
		out = append(out, f.Rule+"\t"+f.Note)
	}
	sort.Strings(out)
	return out
}

// Each fixture of testdata/validate/sources has EXPECT (plain validate) and
// EXPECT_CHECKED (RunWith with CheckSources, the fixture directory as source root).
func TestSourceFixtures(t *testing.T) {
	for _, dir := range fixtureDirs(t, "sources") {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			plain, err := Run(dir)
			if err != nil {
				t.Fatal(err)
			}
			if got, want := pairs(plain), readPairs(t, filepath.Join(dir, "EXPECT")); strings.Join(got, "|") != strings.Join(want, "|") {
				t.Errorf("plain findings = %q, want %q", got, want)
			}
			abs, _ := filepath.Abs(dir)
			checked, err := RunWith(dir, nil, Options{CheckSources: true, SourceRoot: abs})
			if err != nil {
				t.Fatal(err)
			}
			if got, want := pairs(checked), readPairs(t, filepath.Join(dir, "EXPECT_CHECKED")); strings.Join(got, "|") != strings.Join(want, "|") {
				t.Errorf("checked findings = %q, want %q", got, want)
			}
		})
	}
}

// Without the option a legacy line that cites nothing real is not checked.
func TestLegacySourceIsNotVerifiedByDefault(t *testing.T) {
	root := newFixture(t) // cites "IDEAS_NOTES.md §1. Direction, lines 1-5", which does not exist
	assertClean(t, root)
	res, err := RunWith(root, nil, Options{CheckSources: true, SourceRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if !has(res.Findings, "source-file-missing", "First note.md") {
		t.Errorf("want source-file-missing with CheckSources, got %v", res.Findings)
	}
}

func TestTypedOnlyNoteHasNoSourceMissing(t *testing.T) {
	root := newFixture(t)
	edit(t, root, "First note", "Source: IDEAS_NOTES.md §1. Direction, lines 1-5", "Source: decision: keep it small, 2026-10-22")
	assertClean(t, root)
	edit(t, root, "First note", "decision: keep it small, 2026-10-22", "decision: keep it small")
	fs := check(t, root)
	if !has(fs, "source-malformed", "First note.md") || has(fs, "source-missing", "First note.md") {
		t.Errorf("got %v", fs)
	}
}

func TestMapsNeedNoSource(t *testing.T) {
	root := newFixture(t)
	if fs := check(t, root); has(fs, "source-missing", "Map - Direction.md") {
		t.Errorf("got %v", fs)
	}
}

func TestResultListsParsedSources(t *testing.T) {
	root := newFixture(t)
	res, err := Run(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Sources) != 2 || res.Sources[0].Note != "First note.md" || res.Sources[0].Sources[0].From != 1 {
		t.Errorf("Sources = %+v", res.Sources)
	}
}

func TestCheckSourcesDoesNotWrite(t *testing.T) {
	root := newFixture(t)
	before := treeHash(t, root)
	if _, err := RunWith(root, nil, Options{CheckSources: true, SourceRoot: root}); err != nil {
		t.Fatal(err)
	}
	if treeHash(t, root) != before {
		t.Error("RunWith changed the tree")
	}
}

const legacyLine = "Source: IDEAS_NOTES.md §1. Direction, lines 1-5"

// Reviewer repro 1: prose that looks like a kind above a valid legacy line was accepted before and still is.
func TestKindLikeProseNextToLegacyLineIsAccepted(t *testing.T) {
	root := newFixture(t)
	edit(t, root, "First note", legacyLine, "Source: Wikipedia: prose\n"+legacyLine)
	assertClean(t, root)
	edit(t, root, "First note", "Source: Wikipedia: prose\n", "Source: decision: no date\n")
	assertClean(t, root) // the earlier rule accepted this note, so nothing new is reported
}

// Reviewer repro 2: a fenced example is not a Source line, and its path is not checked.
func TestFencedSourceExamplesAreIgnored(t *testing.T) {
	root := newFixture(t)
	edit(t, root, "First note", "Body text.", "Body text.\n\n```\nSource: Foo: bar\nSource: nowhere/x.rs, lines 9-9\n```")
	assertClean(t, root)
	res, err := RunWith(root, nil, Options{CheckSources: true, SourceRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range res.Findings {
		if strings.Contains(f.Message, "nowhere") {
			t.Errorf("fenced example was checked: %v", f)
		}
	}
}

func TestUnknownKindIsReportedOnlyWhereTheNoteWasRejected(t *testing.T) {
	root := newFixture(t)
	edit(t, root, "First note", legacyLine, "Source: note: foo")
	fs := check(t, root)
	if !has(fs, "source-kind-unknown", "First note.md") || has(fs, "source-missing", "First note.md") {
		t.Errorf("got %v", fs)
	}
}

// The ticket repro cases (#15, #16).
func TestTicketCases(t *testing.T) {
	root := newFixture(t)
	if err := os.WriteFile(filepath.Join(root, "real.md"), []byte("one line\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tests := []struct{ line, rule string }{
		{"Source: does/not/exist.rs, lines 900-999", "source-file-missing"},
		{"Source: real.md, lines 500-600", "source-range-invalid"},
		{"Source: https://example.com/x, lines 1-1", "source-malformed"},
		{"Source: https://example.com/x", ""},
		{"Source: decision 2026-10-21 the owner", ""},
		{"Source: measurement cargo bench 2026-10-21", ""},
		{"Source: measurement real.md 2026-10-21", ""},
		{"Source: measurement nowhere/data.csv 2026-10-21", "source-file-missing"},
	}
	for _, tt := range tests {
		r := copyTree(t, root)
		edit(t, r, "First note", legacyLine, tt.line)
		res, err := RunWith(r, nil, Options{CheckSources: true, SourceRoot: r})
		if err != nil {
			t.Fatal(err)
		}
		fs := []string{}
		for _, f := range res.Findings {
			if f.Note == "First note.md" {
				fs = append(fs, f.Rule)
			}
		}
		want := []string{}
		if tt.rule != "" {
			want = []string{tt.rule}
		}
		if strings.Join(fs, ",") != strings.Join(want, ",") {
			t.Errorf("%q: findings %v, want %v", tt.line, fs, want)
		}
	}
	// The URL-with-range message names the reason, also without --check-sources.
	r := copyTree(t, root)
	edit(t, r, "First note", legacyLine, "Source: https://example.com/x, lines 1-1")
	for _, f := range check(t, r) {
		if f.Rule == "source-malformed" && !strings.Contains(f.Message, "url sources take no line range") {
			t.Errorf("message = %s", f.Message)
		}
	}
}

// Plain validate gives the findings that cdce0d7 (before typed sources) gave on the real chartroom
// collection of the Anchorage workspace; the golden is that binary's --json output.
func TestPlainValidateMatchesTheEarlierBinaryOnChartroom(t *testing.T) {
	root := os.Getenv("CHARTROOM_DIR")
	if root == "" {
		root = "/workspaces/Anchorage/chartroom"
	}
	if _, err := os.Stat(filepath.Join(root, "notes")); err != nil {
		t.Skip("real chartroom collection not found (set CHARTROOM_DIR)")
	}
	data, err := os.ReadFile(filepath.Join(fixtureRoot, "main-oracle-chartroom.json"))
	if err != nil {
		t.Fatal(err)
	}
	var want struct {
		Notes    int
		Findings []struct{ Rule, Note string }
	}
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatal(err)
	}
	res, err := Run(root)
	if err != nil {
		t.Fatal(err)
	}
	got := pairs(res)
	exp := []string{}
	for _, f := range want.Findings {
		exp = append(exp, f.Rule+"\t"+f.Note)
	}
	sort.Strings(exp)
	if strings.Join(got, "|") != strings.Join(exp, "|") {
		t.Errorf("findings = %q, golden %q (the collection may have changed since the golden was taken)", got, exp)
	}
}

func TestUnconfiguredCategoryMessageExplainsTheConfig(t *testing.T) {
	root := newFixture(t)
	edit(t, root, "First note", `"category/direction"`, `"category/billing"`)
	for _, f := range check(t, root) {
		if f.Rule == "tag-invalid" {
			for _, want := range []string{`"billing" is not in the configured category/ vocabulary`, ".cscribe.toml under [tags] category"} {
				if !strings.Contains(f.Message, want) {
					t.Errorf("message lacks %q: %s", want, f.Message)
				}
			}
			return
		}
	}
	t.Error("no tag-invalid finding")
}
