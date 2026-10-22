// SPDX-License-Identifier: EUPL-1.2

package pin

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wstein/chartscribe/internal/note"
)

func numbered(n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "line %d\n", i)
	}
	return b.String()
}

func src(path string, from, to int, pin string) note.Source {
	return note.Source{Kind: note.KindFile, Path: path, From: from, To: to, Pin: pin}
}

func classifier(t *testing.T, r *repo, sources ...note.Source) *Classifier {
	t.Helper()
	c, err := New(ExecGit{}, r.dir, sources)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestClassifyRealGit(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", numbered(10))
	r.write("b.txt", numbered(10))
	r.write("gone.txt", numbered(10))
	r.write("old.txt", numbered(20))
	r.write("crlf.txt", strings.ReplaceAll(numbered(10), "\n", "\r\n"))
	r.write("bin.dat", "a\x00b\x00\x01\x02")
	r.write("empty.txt", "")
	r.write("same.txt", numbered(10))
	r.write("sub/dir.txt", numbered(10))
	p := r.commit("first")[:7]

	r.write("a.txt", "new top\nnew top 2\n"+numbered(10)) // two lines inserted before
	r.write("b.txt", strings.Replace(numbered(10), "line 5\n", "edited\n", 1))
	if err := os.Remove(filepath.Join(r.dir, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	r.git("mv", "old.txt", "renamed.txt")
	r.write("crlf.txt", strings.ReplaceAll(numbered(10), "\n", "\r\n")+"tail\r\n")
	r.write("bin.dat", "a\x00b\x00\x01\x03")
	r.write("empty.txt", "now one line\n")
	r.commit("second")

	tests := []struct {
		name string
		s    note.Source
		want Result
	}{
		{"moved by an insert before", src("a.txt", 3, 5, p), Result{Status: Moved, From: 5, To: 7}},
		{"current when only the tail changed", src("crlf.txt", 2, 4, p), Result{Status: Current, From: 2, To: 4}},
		{"changed inside", src("b.txt", 4, 6, p), Result{Status: Changed, Reason: "the cited lines were edited since @" + p, Hunks: []string{"@@ -5 +5 @@"}}},
		{"current before the edit", src("b.txt", 1, 4, p), Result{Status: Current, From: 1, To: 4}},
		{"current after the edit", src("b.txt", 6, 10, p), Result{Status: Current, From: 6, To: 10}},
		{"deleted", src("gone.txt", 1, 2, p), Result{Status: Missing, Reason: "the file was deleted since @" + p}},
		{"renamed", src("old.txt", 1, 2, p), Result{Status: Renamed, NewPath: "renamed.txt", Reason: "the file was renamed since @" + p}},
		{"binary", src("bin.dat", 1, 1, p), Result{Status: Changed, Reason: "binary file changed since @" + p}},
		{"untouched file", src("same.txt", 2, 3, p), Result{Status: Current, From: 2, To: 3}},
		{"unknown pin", src("same.txt", 2, 3, "deadbee"), Result{Status: Unknown, Reason: "pin @deadbee is not a commit of this clone"}},
		{"range outside the pinned file", src("empty.txt", 1, 1, p), Result{Status: Unknown, Reason: "lines 1-1 lie outside the file at the pin (0 lines)"}},
		{"unpinned", src("a.txt", 1, 2, ""), Result{Status: Unpinned}},
		{"never existed", src("nowhere.txt", 1, 2, p), Result{Status: Missing, Reason: "the file is not found below the source root"}},
		{"escapes the root", src("../x.txt", 1, 2, p), Result{Status: Unknown, Reason: "the path leaves the source root"}},
	}
	var all []note.Source
	for _, tt := range tests {
		all = append(all, tt.s)
	}
	c := classifier(t, r, all...)
	for _, tt := range tests {
		got := c.Classify(tt.s)
		if got.Status != tt.want.Status || got.From != tt.want.From || got.To != tt.want.To || got.NewPath != tt.want.NewPath ||
			got.Reason != tt.want.Reason || strings.Join(got.Hunks, ";") != strings.Join(tt.want.Hunks, ";") {
			t.Errorf("%s: got %+v, want %+v", tt.name, got, tt.want)
		}
	}
}

func TestCRLFLineEndingChangeIsAChange(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", "one\ntwo\nthree\n")
	p := r.commit("first")[:7]
	r.write("a.txt", "one\r\ntwo\r\nthree\r\n")
	r.commit("second")
	if got := classifier(t, r, src("a.txt", 2, 2, p)).Classify(src("a.txt", 2, 2, p)); got.Status != Changed {
		t.Errorf("got %+v", got)
	}
}

func TestUncommittedAndUntrackedFilesAreUnknown(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", numbered(5))
	p := r.commit("first")[:7]
	r.write("a.txt", "x\n"+numbered(5))
	r.write("new.txt", numbered(5))
	for _, tt := range []struct{ path, reason string }{
		{"a.txt", "the file has uncommitted changes"},
		{"new.txt", "the file is not tracked by git"},
	} {
		s := src(tt.path, 1, 2, p)
		got := classifier(t, r, s).Classify(s)
		if got.Status != Unknown || got.Reason != tt.reason {
			t.Errorf("%s: got %+v", tt.path, got)
		}
	}
}

func TestPinnedAtHeadIsCurrent(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", numbered(5))
	p := r.commit("first")[:12]
	s := src("a.txt", 2, 3, p)
	if got := classifier(t, r, s).Classify(s); got.Status != Current || got.From != 2 || got.To != 3 {
		t.Errorf("got %+v", got)
	}
}

func TestShallowCloneIsUnknown(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", numbered(5))
	p := r.commit("first")[:7]
	r.write("a.txt", "x\n"+numbered(5))
	r.commit("second")
	clone := filepath.Join(t.TempDir(), "clone")
	r.git("clone", "-q", "--no-local", "--depth", "1", "file://"+r.dir, clone)
	s := src("a.txt", 2, 3, p)
	c, err := New(ExecGit{}, clone, []note.Source{s})
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Classify(s); got.Status != Unknown || got.Reason != "shallow clone (history incomplete)" || c.Disabled() == "" {
		t.Errorf("got %+v", got)
	}
}

func TestNewErrors(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(dir))
	if _, err := New(ExecGit{}, dir, nil); err != ErrNoRepo {
		t.Errorf("no repository: %v", err)
	}
	r := newRepo(t) // a repository without a commit
	if _, err := New(ExecGit{}, r.dir, nil); err != ErrNoRepo {
		t.Errorf("no commit: %v", err)
	}
}

func TestSourceRootBelowTheToplevel(t *testing.T) {
	r := newRepo(t)
	r.write("pkg/a.txt", numbered(8))
	p := r.commit("first")[:7]
	r.write("pkg/a.txt", "top\n"+numbered(8))
	r.commit("second")
	s := src("a.txt", 2, 3, p)
	c, err := New(ExecGit{}, filepath.Join(r.dir, "pkg"), []note.Source{s})
	if err != nil {
		t.Fatal(err)
	}
	if got := c.Classify(s); got.Status != Moved || got.From != 3 {
		t.Errorf("got %+v", got)
	}
}

func TestTarget(t *testing.T) {
	r := newRepo(t)
	r.write("a.txt", numbered(5))
	first := r.commit("first")
	r.write("a.txt", numbered(8))
	r.write("dirty.txt", "x\n")
	r.git("add", "-A")
	r.git("-c", "commit.gpgsign=false", "commit", "-q", "-m", "second")
	head := r.git("rev-parse", "HEAD")
	r.write("dirty.txt", "y\n")
	r.write("loose.txt", "z\n")
	c := classifier(t, r)
	tests := []struct {
		name   string
		s      note.Source
		rev    string
		want   string
		reason string
	}{
		{"head", src("a.txt", 2, 8, ""), "", head[:12], ""},
		{"range outside now", src("a.txt", 2, 9, ""), "", "", `Source "a.txt" lines 2-9: the file has 8 lines`},
		{"dirty", src("dirty.txt", 1, 1, ""), "", "", "the file has uncommitted changes"},
		{"untracked", src("loose.txt", 1, 1, ""), "", "", "the file is not tracked by git"},
		{"older rev", src("a.txt", 2, 5, ""), first[:7], first[:12], ""},
		{"older rev too short a file", src("a.txt", 2, 8, ""), first[:7], "", "lines 2-8 lie outside the file at " + first[:7] + " (5 lines)"},
		{"unknown rev", src("a.txt", 2, 3, ""), "deadbee", "", "revision deadbee is not a commit of this clone"},
		{"bad rev", src("a.txt", 2, 3, ""), "--output=x", "", "the revision must be 7 to 40 lower-case hex digits"},
		{"file absent at rev", src("dirty.txt", 1, 1, ""), first[:7], "", "the file does not exist at " + first[:7]},
		{"missing file", src("none.txt", 1, 1, ""), "", "", `Source file "none.txt" not found under the source root`},
	}
	for _, tt := range tests {
		got, reason := c.Target(tt.s, tt.rev)
		if got != tt.want || reason != tt.reason {
			t.Errorf("%s: got %q %q, want %q %q", tt.name, got, reason, tt.want, tt.reason)
		}
	}
}

// The core guarantee: for random edits, Current and Moved ranges hold exactly the pinned text, and a
// range whose lines survive in one piece (no line inserted into it) is never reported as Changed.
func TestRandomEditsKeepTheCoreGuarantee(t *testing.T) {
	rng := rand.New(rand.NewSource(17))
	r := newRepo(t)
	type trial struct {
		name       string
		from, to   int
		orig       []string
		now        []string
		wantStatus Status
	}
	var trials []trial
	for i := 0; i < 40; i++ {
		n := 5 + rng.Intn(30)
		orig := make([]string, n)
		for k := range orig {
			orig[k] = fmt.Sprintf("t%d-orig-%d", i, k)
		}
		// Edits on the model: ids are unique, so the expected outcome is exact.
		type ln struct {
			text string
			from int // index in orig, or -1 for a new line
		}
		cur := make([]ln, n)
		for k := range cur {
			cur[k] = ln{orig[k], k}
		}
		for e, edits := 0, rng.Intn(6); e < edits; e++ {
			pos := rng.Intn(len(cur) + 1)
			switch rng.Intn(3) {
			case 0: // insert
				var add []ln
				for k, c := 0, 1+rng.Intn(3); k < c; k++ {
					add = append(add, ln{fmt.Sprintf("t%d-new-%d-%d", i, e, k), -1})
				}
				cur = append(cur[:pos], append(add, cur[pos:]...)...)
			case 1: // delete
				if pos < len(cur) {
					c := 1 + rng.Intn(3)
					if pos+c > len(cur) {
						c = len(cur) - pos
					}
					cur = append(cur[:pos], cur[pos+c:]...)
				}
			default: // replace one line
				if pos < len(cur) {
					cur[pos] = ln{fmt.Sprintf("t%d-rep-%d", i, e), -1}
				}
			}
		}
		from := 1 + rng.Intn(n)
		to := from + rng.Intn(n-from+1)
		// Expected: the original lines from-1..to-1 all survive, in a row, with nothing between them.
		want, first := Changed, -1
		for idx, l := range cur {
			if l.from == from-1 {
				first = idx
			}
		}
		if first >= 0 && first+(to-from) < len(cur) {
			ok := true
			for k := 0; k <= to-from; k++ {
				if cur[first+k].from != from-1+k {
					ok = false
				}
			}
			if ok {
				want = Moved
				if first == from-1 {
					want = Current
				}
			}
		}
		name := fmt.Sprintf("f%d.txt", i)
		var o, c []string
		o = orig
		for _, l := range cur {
			c = append(c, l.text)
		}
		r.write(name, strings.Join(o, "\n")+"\n")
		trials = append(trials, trial{name, from, to, o, c, want})
	}
	p := r.commit("orig")[:7]
	for _, tr := range trials {
		r.write(tr.name, strings.Join(tr.now, "\n")+"\n")
	}
	r.commit("edited")
	var all []note.Source
	for _, tr := range trials {
		all = append(all, src(tr.name, tr.from, tr.to, p))
	}
	c := classifier(t, r, all...)
	seen := map[Status]int{}
	defer func() {
		if seen[Current] == 0 || seen[Moved] == 0 || seen[Changed] == 0 {
			t.Errorf("the trials must cover every outcome: %v", seen)
		}
	}()
	for _, tr := range trials {
		seen[tr.wantStatus]++
		got := c.Classify(src(tr.name, tr.from, tr.to, p))
		if got.Status != tr.wantStatus {
			t.Errorf("%s lines %d-%d: got %+v, want %s", tr.name, tr.from, tr.to, got, tr.wantStatus)
			continue
		}
		if got.Status == Current || got.Status == Moved {
			pinned := strings.Join(tr.orig[tr.from-1:tr.to], "\n")
			now := strings.Join(tr.now[got.From-1:got.To], "\n")
			if pinned != now {
				t.Errorf("%s: %s range %d-%d holds %q, pinned text is %q", tr.name, got.Status, got.From, got.To, now, pinned)
			}
		}
	}
}
