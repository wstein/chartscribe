// SPDX-License-Identifier: EUPL-1.2

package pin

import (
	"strings"
	"testing"

	"github.com/wstein/chartscribe/internal/gittest"
)

type repo = gittest.Repo

func newRepo(t *testing.T) *repo { return gittest.New(t) }

func TestExecGitOutputAndStream(t *testing.T) {
	r := newRepo(t)
	r.Write("a.txt", "one\ntwo\n")
	id := r.Commit("first")
	g := ExecGit{}
	out, err := g.Output(r.Dir, "rev-parse", "--verify", "HEAD^{commit}")
	if err != nil || strings.TrimSpace(out) != id {
		t.Fatalf("got %q %v", out, err)
	}
	var recs []string
	if err := g.Stream(r.Dir, []string{"ls-files", "-z"}, 0, func(s string) { recs = append(recs, s) }); err != nil || len(recs) != 1 || recs[0] != "a.txt" {
		t.Fatalf("got %q %v", recs, err)
	}
	if _, err := g.Output(r.Dir, "rev-parse", "--verify", "nope^{commit}"); err == nil {
		t.Error("an unknown revision must fail")
	}
	if err := g.Stream(r.Dir, []string{"cat-file", "blob", "0000000"}, '\n', func(string) {}); err == nil {
		t.Error("a failing command must fail")
	}
}

func TestStreamPassesLongRecordsTruncated(t *testing.T) {
	r := newRepo(t)
	long := strings.Repeat("x", 600*1024)
	r.Write("a.txt", long+"\nshort\n")
	r.Commit("first")
	var recs []string
	err := ExecGit{}.Stream(r.Dir, []string{"cat-file", "blob", "HEAD:a.txt"}, '\n', func(s string) { recs = append(recs, s) })
	if err != nil || len(recs) != 2 || len(recs[0]) > 256*1024 || recs[1] != "short" {
		t.Fatalf("%v: %d records, first %d bytes", err, len(recs), len(recs[0]))
	}
}
