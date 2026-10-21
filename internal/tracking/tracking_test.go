// SPDX-License-Identifier: EUPL-1.2

package tracking

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
)

type fakeRunner struct {
	out  []byte
	err  error
	name string
	args []string
}

func (f *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	f.name, f.args = name, args
	return f.out, f.err
}

func fixtureIssues(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("../../testdata/tracking/issues.json")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestFetchQuery(t *testing.T) {
	r := &fakeRunner{out: fixtureIssues(t)}
	issues, err := Fetch(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"issue", "list", "--state", "all", "--limit", "200", "--json", "number,title,state,url,body"}
	if r.name != "gh" || !reflect.DeepEqual(r.args, want) {
		t.Fatalf("ran %s %v, want gh %v", r.name, r.args, want)
	}
	if len(issues) != 4 || issues[0].Number != 12 || issues[1].State != "CLOSED" {
		t.Fatalf("issues = %+v", issues)
	}
}

func TestFetchLimitReached(t *testing.T) {
	var sb strings.Builder
	sb.WriteString("[")
	for i := 1; i <= Limit; i++ {
		if i > 1 {
			sb.WriteString(",")
		}
		fmt.Fprintf(&sb, `{"number":%d,"title":"t","state":"OPEN","url":"u","body":""}`, i)
	}
	sb.WriteString("]")
	_, err := Fetch(context.Background(), &fakeRunner{out: []byte(sb.String())})
	if !errors.Is(err, ErrLimit) || !strings.Contains(err.Error(), "200") {
		t.Fatalf("err = %v", err)
	}
}

func TestFetchErrors(t *testing.T) {
	_, err := Fetch(context.Background(), &fakeRunner{err: ErrMissing})
	if !errors.Is(err, ErrGh) || !strings.Contains(err.Error(), "gh") {
		t.Fatalf("missing: %v", err)
	}
	_, err = Fetch(context.Background(), &fakeRunner{err: errors.New("exit status 4"), out: []byte("not logged in")})
	if !errors.Is(err, ErrGh) || !strings.Contains(err.Error(), "exit status 4") {
		t.Fatalf("failing: %v", err)
	}
	_, err = Fetch(context.Background(), &fakeRunner{out: []byte("{nope")})
	if !errors.Is(err, ErrGh) {
		t.Fatalf("bad json: %v", err)
	}
}

func TestByNote(t *testing.T) {
	issues, err := Parse(fixtureIssues(t))
	if err != nil {
		t.Fatal(err)
	}
	by := ByNote(issues)
	var nums []int
	for _, i := range by["20261021120000"] {
		nums = append(nums, i.Number)
	}
	if !reflect.DeepEqual(nums, []int{7, 12}) { // sorted by number; inline mentions do not link
		t.Fatalf("issues of the first note = %v", nums)
	}
	if got := by["20261021120100-001"]; len(got) != 1 || got[0].Number != 7 {
		t.Fatalf("suffix id = %+v", got)
	}
	if len(by) != 2 {
		t.Fatalf("ids = %v", by)
	}
}

func TestLine(t *testing.T) {
	got := Line(Issue{Number: 7, Title: "Fix\nthe  bug", State: "CLOSED", URL: "https://x/7"})
	if got != "- #7 Fix the bug (closed) https://x/7" {
		t.Fatalf("line = %q", got)
	}
}
