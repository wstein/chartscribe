// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wstein/chartscribe/internal/gittest"
)

func numberedLines(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	return b.String()
}

func srcNote(id, sources string) string {
	return "---\nid: " + id + "\ntags: [\"type/idea\", \"status/open\", \"category/direction\"]\n---\nBody.\n\n" + sources
}

// sourcesRepo is a collection inside a git repository: two cited files, three notes, one commit,
// then an insert in docs/a.md and an edit in docs/b.md.
func sourcesRepo(t *testing.T) (*gittest.Repo, string, string) {
	t.Helper()
	r := gittest.New(t)
	r.Write("docs/a.md", numberedLines(20))
	r.Write("docs/b.md", numberedLines(20))
	r.Write("notes/Map - Direction.md", "---\nid: 20261021120100\ntags: [\"type/map\", \"category/direction\"]\n---\nMap.\n\n- [[Alpha]]\n- [[Beta]]\n- [[Gamma]]\n")
	r.Write("notes/Alpha.md", srcNote("20261021120000", "Source: docs/a.md, lines 5-7 @PIN\n"))
	r.Write("notes/Beta.md", srcNote("20261021120001", "Source: docs/b.md, lines 2-4 @PIN\nSource: decision: kept, 2026-10-22\n"))
	r.Write("notes/Gamma.md", srcNote("20261021120002", "Source: docs/a.md, lines 1-2\n"))
	first := r.Commit("first")
	pin := first[:12]
	for _, n := range []string{"Alpha.md", "Beta.md"} {
		p := filepath.Join(r.Dir, "notes", n)
		data, _ := os.ReadFile(p)
		if err := os.WriteFile(p, []byte(strings.ReplaceAll(string(data), "@PIN", "@"+pin)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r.Write("docs/a.md", "new\nnew\n"+numberedLines(20))
	r.Write("docs/b.md", strings.Replace(numberedLines(20), "line 3\n", "edited\n", 1))
	head := r.Commit("edit")
	return r, pin, head[:12]
}

func placeholders(s, pin, head string) string {
	return strings.NewReplacer(pin, "{PIN}", head, "{HEAD}").Replace(s)
}

func TestSourcesCheckHumanAndJSON(t *testing.T) {
	r, pin, head := sourcesRepo(t)
	code, out, errOut := run("--root", r.Dir, "sources", "check")
	if code != 1 || errOut != "" {
		t.Fatalf("code=%d err=%q", code, errOut)
	}
	golden(t, "sources-check.stdout", placeholders(out, pin, head))
	code, out, _ = run("--root", r.Dir, "--json", "sources", "check")
	if code != 1 {
		t.Fatalf("code=%d", code)
	}
	golden(t, "sources-check.json", placeholders(out, pin, head))
	var doc struct{ OK bool }
	if err := json.Unmarshal([]byte(out), &doc); err != nil || doc.OK {
		t.Errorf("ok = %v %v", doc.OK, err)
	}
}

func TestSourcesCheckCleanExitsZero(t *testing.T) {
	r, _, _ := sourcesRepo(t)
	if code, out, _ := run("--root", r.Dir, "sources", "check", "Gamma.md"); code != 0 || !strings.Contains(out, "0 moved, 0 changed") {
		t.Errorf("code=%d out=%q", code, out)
	}
}

func TestSourcesPinPlanThenApply(t *testing.T) {
	r, _, head := sourcesRepo(t)
	gamma := filepath.Join(r.Dir, "notes", "Gamma.md")
	before, _ := os.ReadFile(gamma)
	code, out, errOut := run("--root", r.Dir, "sources", "pin", "--all")
	if code != 0 || errOut != "" || !strings.Contains(out, "1 edit in 1 file planned; --apply writes them") ||
		!strings.Contains(out, "+ Source: docs/a.md, lines 1-2 @"+head) {
		t.Fatalf("code=%d out=%q err=%q", code, out, errOut)
	}
	if after, _ := os.ReadFile(gamma); string(after) != string(before) {
		t.Fatal("the plan wrote")
	}
	code, out, _ = run("--root", r.Dir, "--json", "sources", "pin", "--all", "--apply")
	var doc struct {
		Command string
		OK      bool
		Plan    struct {
			Mode    string
			Applied bool
			Edits   []struct{ File, Rule, New string }
		}
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil || code != 0 || doc.Command != "sources pin" || doc.Plan.Mode != "apply" ||
		!doc.Plan.Applied || len(doc.Plan.Edits) != 1 || doc.Plan.Edits[0].Rule != "source-pin" {
		t.Fatalf("code=%d doc=%+v err=%v\n%s", code, doc, err, out)
	}
	after, _ := os.ReadFile(gamma)
	if string(after) != strings.Replace(string(before), "lines 1-2\n", "lines 1-2 @"+head+"\n", 1) {
		t.Errorf("got %q", after)
	}
	if code, _, errOut := run("--root", r.Dir, "validate", "--check-sources"); code != 0 {
		t.Errorf("validate: code=%d err=%q", code, errOut)
	}
}

func TestSourcesUpdatePlanThenApply(t *testing.T) {
	r, pin, head := sourcesRepo(t)
	code, out, _ := run("--root", r.Dir, "sources", "update", "Alpha.md", "notes/Beta.md")
	if code != 1 || !strings.Contains(out, "1 edit in 1 file planned") || !strings.Contains(out, "+ Source: docs/a.md, lines 7-9 @"+head) ||
		!strings.Contains(out, "changed") {
		t.Fatalf("code=%d out=%q", code, out)
	}
	golden(t, "sources-update-plan.stdout", placeholders(out, pin, head))
	// After the write the moved line is current; the changed one still needs review (exit 1).
	code, out, _ = run("--root", r.Dir, "sources", "update", "--all", "--apply")
	if code != 1 || !strings.Contains(out, "applied 1 edit in 1 file") {
		t.Fatalf("code=%d out=%q", code, out)
	}
	alpha, _ := os.ReadFile(filepath.Join(r.Dir, "notes", "Alpha.md"))
	if !strings.Contains(string(alpha), "Source: docs/a.md, lines 7-9 @"+head+"\n") {
		t.Errorf("got %q", alpha)
	}
	beta, _ := os.ReadFile(filepath.Join(r.Dir, "notes", "Beta.md"))
	if !strings.Contains(string(beta), "lines 2-4 @"+pin+"\n") {
		t.Errorf("the changed line must stay: %q", beta)
	}
	if code, _, errOut := run("--root", r.Dir, "validate", "--check-sources"); code != 0 {
		t.Errorf("validate: code=%d err=%q", code, errOut)
	}
}

func TestSourcesUsageAndEnvironmentErrors(t *testing.T) {
	r, _, _ := sourcesRepo(t)
	for _, args := range [][]string{
		{"sources", "pin"},                         // neither notes nor --all
		{"sources", "update"},                      // neither
		{"sources", "pin", "--all", "Alpha.md"},    // both
		{"sources", "check", "../../etc/passwd"},   // not a note
		{"sources", "check", "NoSuch.md"},          // not a note
		{"sources", "check", "--apply"},            // check never writes
		{"sources", "pin", "--all", "--rev", "-x"}, // handled below as a plan reason, not usage
	} {
		code, _, _ := run(append([]string{"--root", r.Dir}, args...)...)
		want := 2
		if args[len(args)-1] == "-x" {
			want = 0
		}
		if code != want {
			t.Errorf("%v: code %d, want %d", args, code, want)
		}
	}
	code, out, _ := run("--root", r.Dir, "--json", "sources", "pin")
	var doc struct{ Command, Error string }
	if err := json.Unmarshal([]byte(out), &doc); err != nil || code != 2 || doc.Command != "sources pin" || doc.Error != "usage" {
		t.Errorf("code=%d out=%q", code, out)
	}
	// Not a repository: exit 3.
	plain := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(plain))
	if err := os.MkdirAll(filepath.Join(plain, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	if code, _, errOut := run("--root", plain, "sources", "check"); code != 3 || !strings.Contains(errOut, "git repository") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
	// git missing: exit 3.
	t.Setenv("PATH", t.TempDir())
	if code, _, errOut := run("--root", r.Dir, "sources", "check"); code != 3 || !strings.Contains(errOut, "git not found") {
		t.Errorf("code=%d err=%q", code, errOut)
	}
}
