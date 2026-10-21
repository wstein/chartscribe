// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wstein/chartscribe/internal/note"
	"github.com/wstein/chartscribe/internal/tracking"
)

type stubRunner struct {
	out  string
	err  error
	args []string
}

func (s *stubRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	s.args = append([]string{name}, args...)
	return []byte(s.out), s.err
}

func useRunner(t *testing.T, r tracking.Runner) {
	t.Helper()
	old := ghRunner
	ghRunner = r
	t.Cleanup(func() { ghRunner = old })
}

const trackingIssues = `[
 {"number": 12, "title": "Chartroom: 20261021120000 First note", "state": "OPEN", "url": "https://github.com/o/r/issues/12", "body": ""},
 {"number": 7, "title": "Ghost", "state": "CLOSED", "url": "https://github.com/o/r/issues/7", "body": "Chartroom: 20261021120000 First note"}
]`

const noteHead = "---\nid: %s\ntags: [\"type/idea\"]\n---\n"

// trackingCollection writes a collection with a note that gets a section, one
// that has a stale section, one with a stale section and a heading after it,
// and one without issues.
func trackingCollection(t *testing.T) (root string, files map[string]string) {
	t.Helper()
	root = t.TempDir()
	files = map[string]string{
		"First note.md":  strings.Replace(noteHead, "%s", "20261021120000", 1) + "Body [[Second note]]\n\nSource: s, lines 1-2\n",
		"Second note.md": strings.Replace(noteHead, "%s", "20261021120100", 1) + "Body\n\nSource: s, lines 1-2\n\n## Tracking\n\n- #99 gone (open) u\n\n## Later\n\nkeep\n",
		"Third note.md":  strings.Replace(noteHead, "%s", "20261021120200", 1) + "Body\n",
	}
	if err := os.MkdirAll(filepath.Join(root, "notes"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(root, "notes", name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root, files
}

func readAll(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	entries, _ := os.ReadDir(filepath.Join(root, "notes"))
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(root, "notes", e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[e.Name()] = string(data)
	}
	return out
}

func TestTrackingDryRunChangesNothing(t *testing.T) {
	root, before := trackingCollection(t)
	r := &stubRunner{out: trackingIssues}
	useRunner(t, r)
	code, out, errOut := run("--root", root, "tracking")
	if code != 0 || errOut != "" {
		t.Fatalf("code %d, stderr %q", code, errOut)
	}
	for name, text := range readAll(t, root) {
		if text != before[name] {
			t.Errorf("%s changed in a dry run", name)
		}
	}
	want := "notes/First note.md\tadd\t2\nnotes/Second note.md\tremove\t0\ntracking: 3 notes, 2 to change (dry run; --write applies)\n"
	if out != want {
		t.Fatalf("stdout = %q, want %q", out, want)
	}
	if got := strings.Join(r.args, " "); !strings.Contains(got, "--state all --limit 200") {
		t.Fatalf("gh args = %q", got)
	}
}

func TestTrackingWriteChangesOnlyTheSection(t *testing.T) {
	root, before := trackingCollection(t)
	useRunner(t, &stubRunner{out: trackingIssues})
	code, out, errOut := run("--root", root, "tracking", "--write")
	if code != 0 || errOut != "" {
		t.Fatalf("code %d, stderr %q", code, errOut)
	}
	if !strings.HasSuffix(out, "tracking: 3 notes, 2 changed\n") {
		t.Fatalf("stdout = %q", out)
	}
	after := readAll(t, root)
	wantFirst := before["First note.md"] + "\n## Tracking\n\n- #7 Ghost (closed) https://github.com/o/r/issues/7\n- #12 Chartroom: 20261021120000 First note (open) https://github.com/o/r/issues/12\n"
	if after["First note.md"] != wantFirst {
		t.Fatalf("First note = %q", after["First note.md"])
	}
	if want := strings.Replace(before["Second note.md"], "## Tracking\n\n- #99 gone (open) u\n\n", "", 1); after["Second note.md"] != want {
		t.Fatalf("Second note = %q", after["Second note.md"])
	}
	if after["Third note.md"] != before["Third note.md"] {
		t.Fatal("a note without issues changed")
	}
	for name := range before { // the bytes outside the section hash the same (up to the blank line before an appended one)
		bb := bodyOf(t, before[name])
		ab := bodyOf(t, after[name])
		if sha256.Sum256([]byte(strings.TrimRight(note.StripTracking(bb), "\n"))) != sha256.Sum256([]byte(strings.TrimRight(note.StripTracking(ab), "\n"))) {
			t.Errorf("%s: bytes outside the section changed", name)
		}
		if !strings.HasPrefix(after[name], before[name][:len(before[name])-len(bb)]) {
			t.Errorf("%s: frontmatter changed", name)
		}
	}
	// A second run has nothing to do.
	code, out, _ = run("--root", root, "tracking", "--write")
	if code != 0 || !strings.HasSuffix(out, "tracking: 3 notes, 0 changed\n") {
		t.Fatalf("second run: code %d, stdout %q", code, out)
	}
}

func bodyOf(t *testing.T, text string) string {
	t.Helper()
	_, body, err := note.Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func TestTrackingJSON(t *testing.T) {
	root, _ := trackingCollection(t)
	useRunner(t, &stubRunner{out: trackingIssues})
	code, out, errOut := run("--root", root, "--json", "tracking")
	if code != 0 || errOut != "" {
		t.Fatalf("code %d, stderr %q", code, errOut)
	}
	var doc struct {
		Version int    `json:"version"`
		Command string `json:"command"`
		OK      bool   `json:"ok"`
		Write   bool   `json:"write"`
		Issues  int    `json:"issues"`
		Changed int    `json:"changed"`
		Plan    []struct {
			ID     string `json:"id"`
			Path   string `json:"path"`
			Action string `json:"action"`
			Issues []struct {
				Number int    `json:"number"`
				State  string `json:"state"`
			} `json:"issues"`
		} `json:"plan"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatal(err)
	}
	if doc.Version != 1 || doc.Command != "tracking" || !doc.OK || doc.Write || doc.Issues != 2 || doc.Changed != 2 || len(doc.Plan) != 2 {
		t.Fatalf("doc = %+v", doc)
	}
	n := doc.Plan[0]
	if n.ID != "20261021120000" || n.Path != "notes/First note.md" || n.Action != "add" || len(n.Issues) != 2 || n.Issues[0].Number != 7 || n.Issues[0].State != "closed" {
		t.Fatalf("note = %+v", n)
	}
}

func TestTrackingEnvironmentErrors(t *testing.T) {
	root, before := trackingCollection(t)
	tests := []struct {
		name   string
		runner tracking.Runner
		errHas string
	}{
		{"gh failing", &stubRunner{err: errors.New("exit status 4"), out: "not logged in"}, "exit status 4"},
		{"limit reached", &stubRunner{out: "[" + strings.TrimSuffix(strings.Repeat(`{"number":1,"title":"t","state":"OPEN","url":"u","body":""},`, 200), ",") + "]"}, "limit of 200"},
		{"bad output", &stubRunner{out: "{"}, "unreadable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useRunner(t, tt.runner)
			code, out, errOut := run("--root", root, "tracking", "--write")
			if code != 3 || out != "" || !strings.Contains(errOut, tt.errHas) {
				t.Fatalf("code %d, stdout %q, stderr %q", code, out, errOut)
			}
			code, out, _ = run("--root", root, "--json", "tracking")
			if code != 3 || !strings.Contains(out, `"ok": false`) {
				t.Fatalf("json: code %d, stdout %q", code, out)
			}
		})
	}
	for name, text := range readAll(t, root) {
		if text != before[name] {
			t.Errorf("%s changed after a failure", name)
		}
	}
}

func TestTrackingGhMissing(t *testing.T) {
	root, _ := trackingCollection(t)
	useRunner(t, tracking.ExecRunner{})
	t.Setenv("PATH", t.TempDir()) // no gh here; the real gh is never started
	code, out, errOut := run("--root", root, "tracking")
	if code != 3 || out != "" || !strings.Contains(errOut, "gh not found") {
		t.Fatalf("code %d, stdout %q, stderr %q", code, out, errOut)
	}
}

func TestTrackingNoCollectionAndUsage(t *testing.T) {
	useRunner(t, &stubRunner{out: "[]"})
	if code, _, _ := run("--root", t.TempDir(), "tracking"); code != 3 {
		t.Fatalf("no collection: code %d", code)
	}
	if code, _, _ := run("tracking", "extra"); code != 2 {
		t.Fatalf("argument: code %d", code)
	}
}

func TestTrackingLeavesCRLFNotesAlone(t *testing.T) {
	root, _ := trackingCollection(t)
	path := filepath.Join(root, "notes", "First note.md")
	crlf := strings.ReplaceAll(readAll(t, root)["First note.md"], "\n", "\r\n")
	if err := os.WriteFile(path, []byte(crlf), 0o644); err != nil {
		t.Fatal(err)
	}
	useRunner(t, &stubRunner{out: trackingIssues})
	code, _, errOut := run("--root", root, "tracking", "--write")
	data, _ := os.ReadFile(path)
	if code != 0 || string(data) != crlf || !strings.Contains(errOut, "First note.md") {
		t.Fatalf("code %d, stderr %q, changed %v", code, errOut, string(data) != crlf)
	}
}
