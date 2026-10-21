// SPDX-License-Identifier: EUPL-1.2

package validate

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Parity with the reference checker chartroom/scripts/check_notes.py of the
// Anchorage workspace. The test runs both validators on the same collections and
// compares the (rule, note) pairs of their findings. It is skipped when python3
// or the reference script is absent.
//
// Where the reference script is found:
//   - CHECK_NOTES_PY: path of check_notes.py;
//   - else /workspaces/Anchorage/chartroom/scripts/check_notes.py.
//
// CHARTROOM_COLLECTION may name an extra collection root (for example a checkout
// of the real chartroom/) that is compared as well.
//
// Intentional differences, covered by the testdata/validate/diff fixtures (EXPECT is
// the Go result, PYTHON the reference result):
//   - map-multiple counts a note once per map (several links from one map, also
//     through a title and an alias, are one listing);
//   - single-quoted scalars, plain flow items, `[]`-style Obsidian output and
//     unindented block list items are accepted (the reference reports frontmatter
//     or id-invalid findings for them);
//   - the generated `## Tracking` section is skipped for wikilinks and the Source line.

var pyRules = []struct {
	re   *regexp.Regexp
	rule string
}{
	{regexp.MustCompile(`^bad frontmatter`), "frontmatter"},
	{regexp.MustCompile(`^unknown frontmatter field`), "field-unknown"},
	{regexp.MustCompile(`^missing id$`), "id-missing"},
	{regexp.MustCompile(`^invalid id`), "id-invalid"},
	{regexp.MustCompile(`^(tags|aliases) must be a list`), "list-type"},
	{regexp.MustCompile(`^invalid tag`), "tag-invalid"},
	{regexp.MustCompile(`^needs exactly one`), "tag-count"},
	{regexp.MustCompile(`^forbidden character`), "title-char"},
	{regexp.MustCompile(`^missing Source line`), "source-missing"},
	{regexp.MustCompile(`^wikilink \[\[`), "wikilink-unresolved"},
	{regexp.MustCompile(`^not listed in any map`), "map-unlisted"},
	{regexp.MustCompile(`^listed in more than one map`), "map-multiple"},
	{regexp.MustCompile(`^category .* differs from its map`), "map-category"},
	{regexp.MustCompile(`^link .* points outside`), "readme-link"},
	{regexp.MustCompile(`^broken link`), "readme-link"},
}

var pyCollection = []struct {
	re   *regexp.Regexp
	rule string
}{
	{regexp.MustCompile(`^duplicate id `), "id-duplicate"},
	{regexp.MustCompile(`^duplicate title or alias `), "name-duplicate"},
	{regexp.MustCompile(`^more than one map for category`), "map-duplicate"},
	{regexp.MustCompile(`^category \S+: no map$`), "category-no-map"},
}

var pyNotePrefix = regexp.MustCompile(`^(.+?\.md): (.*)$`)

func classifyPython(t *testing.T, line string) string {
	t.Helper()
	for _, c := range pyCollection {
		if c.re.MatchString(line) {
			return c.rule + "\t"
		}
	}
	if m := pyNotePrefix.FindStringSubmatch(line); m != nil {
		for _, r := range pyRules {
			if r.re.MatchString(m[2]) {
				return r.rule + "\t" + m[1]
			}
		}
	}
	t.Fatalf("cannot classify reference finding %q", line)
	return ""
}

func referenceScript(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not found")
	}
	candidates := []string{os.Getenv("CHECK_NOTES_PY"), "/workspaces/Anchorage/chartroom/scripts/check_notes.py"}
	for _, c := range candidates {
		if c == "" {
			continue
		}
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	t.Skip("reference check_notes.py not found (set CHECK_NOTES_PY)")
	return ""
}

func referencePairs(t *testing.T, script, root string) []string {
	t.Helper()
	cmd := exec.Command("python3", "-I", script, root)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	_ = cmd.Run() // exit status 1 means findings
	out := []string{}
	for _, line := range strings.Split(strings.TrimSpace(stderr.String()), "\n") {
		if line != "" {
			out = append(out, classifyPython(t, line))
		}
	}
	sort.Strings(out)
	return out
}

func TestParityWithReference(t *testing.T) {
	script := referenceScript(t)
	var roots []string
	for _, kind := range []string{"pass", "fail"} {
		roots = append(roots, fixtureDirs(t, kind)...)
	}
	roots = append(roots, filepath.Join(fixtureRoot, "chartroom"))
	if extra := os.Getenv("CHARTROOM_COLLECTION"); extra != "" {
		roots = append(roots, extra)
	}
	for _, root := range roots {
		t.Run(filepath.Base(root), func(t *testing.T) {
			want := referencePairs(t, script, root)
			got := goPairs(t, root)
			if strings.Join(got, "|") != strings.Join(want, "|") {
				t.Errorf("Go findings = %q\nreference findings = %q", got, want)
			}
		})
	}
}

// The documented differences: Go and the reference each give their recorded result.
func TestIntentionalDifferencesFromReference(t *testing.T) {
	script := referenceScript(t)
	for _, dir := range fixtureDirs(t, "diff") {
		t.Run(filepath.Base(dir), func(t *testing.T) {
			wantPy := readPairs(t, filepath.Join(dir, "PYTHON"))
			if got := referencePairs(t, script, dir); strings.Join(got, "|") != strings.Join(wantPy, "|") {
				t.Errorf("reference findings = %q, recorded %q", got, wantPy)
			}
			if len(wantPy) == 0 {
				t.Error("a diff fixture must differ from the reference")
			}
		})
	}
}
