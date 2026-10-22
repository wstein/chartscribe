// SPDX-License-Identifier: EUPL-1.2

package pin

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// repo is a throw-away git repository whose commits carry an identity from the environment of the command.
type repo struct {
	t   *testing.T
	dir string
	n   int
}

func newRepo(t *testing.T) *repo {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r := &repo{t: t, dir: dir}
	r.git("init", "-q")
	return r
}

func (r *repo) git(args ...string) string {
	r.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.org", "GIT_COMMITTER_NAME=t",
		"GIT_COMMITTER_EMAIL=t@example.org", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (r *repo) write(name, text string) {
	r.t.Helper()
	path := filepath.Join(r.dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		r.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		r.t.Fatal(err)
	}
}

// commit stages everything and returns the full id of the new commit.
func (r *repo) commit(msg string) string {
	r.t.Helper()
	r.n++
	r.git("add", "-A")
	r.git("-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", msg)
	return r.git("rev-parse", "HEAD")
}

func TestExecGitOutputAndStream(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", "one\ntwo\n")
	id := r.commit("first")
	g := ExecGit{}
	out, err := g.Output(r.dir, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || strings.TrimSpace(out) != id {
		t.Fatalf("got %q %v", out, err)
	}
	var recs []string
	if err := g.Stream(r.dir, []string{"ls-files", "-z"}, 0, func(s string) { recs = append(recs, s) }); err != nil || len(recs) != 1 || recs[0] != "a.txt" {
		t.Fatalf("got %q %v", recs, err)
	}
	if _, err := g.Output(r.dir, "rev-parse", "--verify", "nope^{commit}"); err == nil {
		t.Error("an unknown revision must fail")
	}
	if err := g.Stream(r.dir, []string{"cat-file", "blob", "0000000"}, '\n', func(string) {}); err == nil {
		t.Error("a failing command must fail")
	}
}

func TestStreamPassesLongRecordsTruncated(t *testing.T) {
	r := newRepo(t)
	long := strings.Repeat("x", 600*1024)
	r.write("a.txt", long+"\nshort\n")
	r.commit("first")
	var recs []string
	err := ExecGit{}.Stream(r.dir, []string{"cat-file", "blob", "HEAD:a.txt"}, '\n', func(s string) { recs = append(recs, s) })
	if err != nil || len(recs) != 2 || len(recs[0]) > 256*1024 || recs[1] != "short" {
		t.Fatalf("%v: %d records, first %d bytes", err, len(recs), len(recs[0]))
	}
}
