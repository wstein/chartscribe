// SPDX-License-Identifier: EUPL-1.2

package validate

import (
	"github.com/wstein/chartscribe/internal/fixtures"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const fixtureRoot = "../../testdata/validate"

// fixtureDirs returns the scenario directories under testdata/validate/<kind>.
func fixtureDirs(t *testing.T, kind string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(fixtureRoot, kind))
	if err != nil {
		t.Fatal(err)
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, filepath.Join(fixtureRoot, kind, e.Name()))
		}
	}
	return dirs
}

// readPairs reads a file of `rule<TAB>note` lines.
func readPairs(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	sort.Strings(out)
	return out
}

func goPairs(t *testing.T, root string) []string {
	t.Helper()
	res, err := Run(root)
	if err != nil {
		t.Fatalf("%s: %v", root, err)
	}
	out := []string{}
	for _, f := range res.Findings {
		out = append(out, f.Rule+"\t"+f.Note)
	}
	sort.Strings(out)
	return out
}

func TestFixtures(t *testing.T) {
	for _, kind := range []string{"pass", "fail", "diff"} {
		for _, dir := range fixtureDirs(t, kind) {
			t.Run(kind+"/"+filepath.Base(dir), func(t *testing.T) {
				want := readPairs(t, filepath.Join(dir, "EXPECT"))
				got := goPairs(t, fixtures.Materialize(t, dir))
				if strings.Join(got, "|") != strings.Join(want, "|") {
					t.Errorf("findings = %q, want %q", got, want)
				}
				if kind == "pass" && len(got) != 0 {
					t.Errorf("pass fixture has findings: %q", got)
				}
				if kind == "fail" && len(got) == 0 {
					t.Error("fail fixture has no findings")
				}
			})
		}
	}
}

// The synthetic collection shaped like chartroom/ is clean (SC-001).
func TestChartroomShapedSnapshotIsClean(t *testing.T) {
	root := fixtures.Materialize(t, filepath.Join(fixtureRoot, "chartroom"))
	if got := goPairs(t, root); len(got) != 0 {
		t.Fatalf("findings = %q", got)
	}
	res, _ := Run(root)
	if res.Notes < 30 {
		t.Errorf("snapshot has only %d notes", res.Notes)
	}
}

// copyTree copies the snapshot so that a probe can mutate it.
func copyTree(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

// The five mutation probes of SC-001 each give a finding.
func TestMutationProbes(t *testing.T) {
	snapshot := fixtures.Materialize(t, filepath.Join(fixtureRoot, "chartroom"))
	const note = "Scope and ground rules"
	probes := []struct {
		name, rule string
		mutate     func(t *testing.T, root string)
	}{
		{"broken wikilink", "wikilink-unresolved", func(t *testing.T, root string) {
			edit(t, root, note, "[[Lighthouse principles]]", "[[Nonexistent principles]]")
		}},
		{"duplicate id", "id-duplicate", func(t *testing.T, root string) {
			data, _ := os.ReadFile(filepath.Join(root, "notes", "Principles of the Lighthouse project.md"))
			id := strings.SplitN(strings.SplitN(string(data), "id: ", 2)[1], "\n", 2)[0]
			cur, _ := os.ReadFile(filepath.Join(root, "notes", note+".md"))
			old := strings.SplitN(strings.SplitN(string(cur), "id: ", 2)[1], "\n", 2)[0]
			edit(t, root, note, "id: "+old, "id: "+id)
		}},
		{"renamed category", "tag-invalid", func(t *testing.T, root string) {
			edit(t, root, note, `"category/direction"`, `"category/bogus"`)
		}},
		{"category swapped to another valid one", "map-category", func(t *testing.T, root string) {
			edit(t, root, note, `"category/direction"`, `"category/state"`)
		}},
		{"note removed from its map", "map-unlisted", func(t *testing.T, root string) {
			edit(t, root, "Map - Direction", "- ["+"[Scope and ground rules]]: one-line summary\n", "")
		}},
	}
	for _, p := range probes {
		t.Run(p.name, func(t *testing.T) {
			root := copyTree(t, snapshot)
			p.mutate(t, root)
			if fs := check(t, root); !has(fs, p.rule, func() string {
				if p.rule == "id-duplicate" {
					return ""
				}
				return note + ".md"
			}()) {
				t.Errorf("want %s, got %v", p.rule, fs)
			}
		})
	}
}
