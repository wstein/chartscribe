// SPDX-License-Identifier: EUPL-1.2

package validate

import (
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
