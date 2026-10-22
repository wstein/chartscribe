// SPDX-License-Identifier: EUPL-1.2

package stale

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wstein/chartscribe/internal/note"
	"github.com/wstein/chartscribe/internal/validate"
)

func notes(name string, srcs ...note.Source) []validate.NoteSources {
	return []validate.NoteSources{{Note: name, Sources: srcs}}
}

func fileSrc(p string) note.Source { return note.Source{Kind: note.KindFile, Path: p, From: 1, To: 1} }

// fakeGit answers rev-parse with top and log with the time of the last path component.
func fakeGit(top string, times map[string]string, calls *[][]string) Runner {
	return func(dir string, args ...string) (string, error) {
		*calls = append(*calls, args)
		if args[0] == "rev-parse" {
			return top + "\n", nil
		}
		path := strings.TrimPrefix(args[len(args)-1], ":(literal)")
		return times[path] + "\n", nil
	}
}

func layout(t *testing.T) (root, src string) {
	t.Helper()
	root = t.TempDir()
	for _, p := range []string{"notes/N.md", "docs/a.md", "data/m.csv"} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, p)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, p), []byte("x\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root, root
}

func TestWarnsWhenTheSourceIsNewer(t *testing.T) {
	root, src := layout(t)
	top, _ := filepath.EvalSymlinks(root)
	var calls [][]string
	run := fakeGit(top, map[string]string{"notes/N.md": "1700000000", "docs/a.md": "1800000000", "data/m.csv": "1600000000"}, &calls)
	srcs := notes("N.md", fileSrc("docs/a.md"), note.Source{Kind: note.KindMeasurement, Path: "data/m.csv"},
		note.Source{Kind: note.KindURL, URL: "https://x.org"})
	res := Check(root, src, srcs, run)
	if res.Skipped != "" || len(res.Warnings) != 1 {
		t.Fatalf("got %+v", res)
	}
	w := res.Warnings[0]
	if w.Rule != "source-stale" || w.Note != "N.md" || !strings.Contains(w.Message, `"docs/a.md"`) ||
		!strings.Contains(w.Message, "2027-01-15") || !strings.Contains(w.Message, "2023-11-14") {
		t.Errorf("warning = %+v", w)
	}
	for _, c := range calls {
		if c[0] != "rev-parse" && c[0] != "log" {
			t.Errorf("unexpected git call %v", c)
		}
		if c[0] == "log" && !(c[1] == "-1" && c[2] == "--format=%ct" && c[3] == "--") {
			t.Errorf("log args %v", c)
		}
	}
}

func TestEqualTimeIsNotStale(t *testing.T) {
	root, src := layout(t)
	top, _ := filepath.EvalSymlinks(root)
	var calls [][]string
	run := fakeGit(top, map[string]string{"notes/N.md": "1700000000", "docs/a.md": "1700000000"}, &calls)
	if res := Check(root, src, notes("N.md", fileSrc("docs/a.md")), run); len(res.Warnings) != 0 {
		t.Errorf("got %+v", res)
	}
}

func TestUntrackedFilesAreSkipped(t *testing.T) {
	root, src := layout(t)
	top, _ := filepath.EvalSymlinks(root)
	var calls [][]string
	// the note has no commit: nothing to compare
	run := fakeGit(top, map[string]string{"docs/a.md": "1800000000"}, &calls)
	if res := Check(root, src, notes("N.md", fileSrc("docs/a.md")), run); len(res.Warnings) != 0 || res.Skipped != "" {
		t.Errorf("got %+v", res)
	}
}

func TestMissingSourceIsLeftToValidate(t *testing.T) {
	root, src := layout(t)
	top, _ := filepath.EvalSymlinks(root)
	var calls [][]string
	run := fakeGit(top, map[string]string{"notes/N.md": "1"}, &calls)
	if res := Check(root, src, notes("N.md", fileSrc("docs/none.md")), run); len(res.Warnings) != 0 {
		t.Errorf("got %+v", res)
	}
}

func TestGitMissingOrNoRepoSkips(t *testing.T) {
	root, src := layout(t)
	for name, err := range map[string]error{"missing": exec.ErrNotFound, "norepo": errors.New("fatal: not a git repository")} {
		run := func(string, ...string) (string, error) { return "", err }
		res := Check(root, src, notes("N.md", fileSrc("docs/a.md")), run)
		if len(res.Warnings) != 0 || !strings.HasPrefix(res.Skipped, "source-stale skipped: ") {
			t.Errorf("%s: got %+v", name, res)
		}
	}
}

// One test with the real git in a temporary repository.
func TestRealGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root, src := layout(t)
	git := func(date string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.org", "GIT_COMMITTER_NAME=t",
			"GIT_COMMITTER_EMAIL=t@example.org", "GIT_AUTHOR_DATE="+date, "GIT_COMMITTER_DATE="+date,
			"GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	git("2026-01-01T00:00:00Z", "init", "-q")
	git("2026-01-01T00:00:00Z", "add", "notes/N.md")
	git("2026-01-01T00:00:00Z", "commit", "-q", "-m", "note")
	git("2026-02-01T00:00:00Z", "add", "docs/a.md")
	git("2026-02-01T00:00:00Z", "commit", "-q", "-m", "source")
	res := Check(root, src, notes("N.md", fileSrc("docs/a.md"), fileSrc("data/m.csv")), ExecRunner)
	if res.Skipped != "" || len(res.Warnings) != 1 || !strings.Contains(res.Warnings[0].Message, "2026-02-01") {
		t.Errorf("got %+v", res)
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "index.lock")); err == nil {
		t.Error("lock file left behind")
	}
}
