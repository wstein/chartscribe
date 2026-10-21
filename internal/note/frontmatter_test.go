// SPDX-License-Identifier: EUPL-1.2

package note

import (
	"reflect"
	"strings"
	"testing"
)

func fm(lines ...string) string { return "---\n" + strings.Join(lines, "\n") + "\n---\nBody\n" }

func TestParseValues(t *testing.T) {
	tests := []struct {
		name string
		in   string
		key  string
		want Value
	}{
		{"plain scalar", fm("id: 20261021120000"), "id", Scalar("20261021120000")},
		{"double quoted", fm(`id: "20261021120000"`), "id", Scalar("20261021120000")},
		{"single quoted", fm(`id: '20261021120000'`), "id", Scalar("20261021120000")},
		{"single quoted escape", fm(`id: 'it''s'`), "id", Scalar("it's")},
		{"comment after value", fm("id: 2026 # note"), "id", Scalar("2026")},
		{"flow list json", fm(`tags: ["a", "b"]`), "tags", ListOf("a", "b")},
		{"flow list plain", fm(`tags: [a, b/c]`), "tags", ListOf("a", "b/c")},
		{"flow list single quoted", fm(`tags: ['a', "b"]`), "tags", ListOf("a", "b")},
		{"flow list mixed", fm(`tags: ["a", b]`), "tags", ListOf("a", "b")},
		{"empty flow list", fm(`aliases: []`), "aliases", ListOf()},
		{"empty value is an empty list", fm(`aliases:`), "aliases", ListOf()},
		{"block list", fm("tags:", "  - a", `  - "b c"`, "  - 'd'"), "tags", ListOf("a", "b c", "d")},
		{"block list without indent", fm("tags:", "- a", "- b"), "tags", ListOf("a", "b")},
		{"block list with blank and comment lines", fm("tags:", "  - a", "", "# c", "  - b"), "tags", ListOf("a", "b")},
		{"non string json item", fm(`tags: ["a", 1]`), "tags", Value{List: []string{"a", "1"}, IsList: true, NonString: true}},
		{"quoted scalar tags", fm(`tags: "type/idea" #`), "tags", Scalar("type/idea")},
		{"list items under a scalar are ignored", fm("id: 1", "  - x"), "id", Scalar("1")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, body, err := Parse(tt.in)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			got, ok := f.Get(tt.key)
			if !ok {
				t.Fatalf("key %q missing", tt.key)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("%s = %#v, want %#v", tt.key, got, tt.want)
			}
			if body != "Body\n" {
				t.Errorf("body = %q", body)
			}
		})
	}
}

func TestParseKeyOrderAndDuplicates(t *testing.T) {
	f, _, err := Parse(fm("tags: [a]", "id: 1", "tags: [b]"))
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Keys(); !reflect.DeepEqual(got, []string{"tags", "id"}) {
		t.Errorf("keys = %v", got)
	}
	if v, _ := f.Get("tags"); !reflect.DeepEqual(v, ListOf("b")) {
		t.Errorf("last value wins, got %#v", v)
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct{ name, in, want string }{
		{"no frontmatter", "just text\n", "no frontmatter"},
		{"empty file", "", "no frontmatter"},
		{"unterminated", "---\nid: 1\n", "unterminated frontmatter"},
		{"closing fence without newline", "---\nid: 1\n---", "unterminated frontmatter"},
		{"bad line", fm("id 1"), "cannot parse frontmatter line"},
		{"bad json string", fm(`id: "abc`), "invalid"},
		{"bad flow list", fm(`tags: ["a", `), "flow list"},
		{"empty flow item", fm(`tags: [a,,b]`), "flow list"},
		{"trailing text after quoted", fm(`id: "a" b`), "invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := Parse(tt.in)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want it to contain %q", err, tt.want)
			}
		})
	}
}

func TestParseEmptyFrontmatter(t *testing.T) {
	f, body, err := Parse("---\n---\nBody\n")
	if err != nil || len(f.Keys()) != 0 || body != "Body\n" {
		t.Errorf("got %v %q %v", f.Keys(), body, err)
	}
}
