// SPDX-License-Identifier: EUPL-1.2

package note

import (
	"strings"
	"testing"
)

func TestClosingFence(t *testing.T) {
	tests := []struct {
		name, text, want string
		ok               bool
	}{
		{"closed", "```\nx\n```\n", "", false},
		{"none", "plain", "", false},
		{"top level", "a\n```go\nx", "```", true},
		{"long tilde", "~~~~~\nx", "~~~~~", true},
		{"after a closed fence", "```\nx\n```\n````\ny", "````", true},
		{"indent 1", " ```\nx", " ```", true},
		{"indent 3", "   ~~~\nx", "   ~~~", true},
		{"marker line", "- ```\n  x", "  ```", true},
		{"numbered marker line", "1. ```\n   x", "   ```", true},
		{"continuation, 2 spaces", "- item\n\n  ```\n  l", "  ```", true},
		{"continuation, 4 spaces", "- item\n\n    ```\n    l", "    ```", true},
		{"continuation, numbered", "1. item\n\n   ```\n   l", "   ```", true},
		{"quote", "> ```\n> x", "> ```", true},
		{"quote and list", "> - ```\n>   x", ">   ```", true},
		{"tab is indented code, no fence", "\t```\nx", "", false},
	}
	for _, tt := range tests {
		got, ok := ClosingFence(tt.text)
		if got != tt.want || ok != tt.ok {
			t.Errorf("%s: ClosingFence = %q, %v; want %q, %v", tt.name, got, ok, tt.want, tt.ok)
			continue
		}
		if !ok {
			continue
		}
		// The closing line closes that very fence: nothing stays open and the
		// next line is ordinary text.
		full := tt.text + "\n" + got + "\n## Next"
		fenced, unclosed := CodeLines(full)
		n := len(fenced)
		if unclosed || !fenced[n-2] || fenced[n-1] {
			t.Errorf("%s: closing line does not close the fence: %v unclosed=%v", tt.name, fenced, unclosed)
		}
		// The closer is not an opener: the text after it is not fenced even
		// when more text follows.
		if strings.Count(full, "\n") != n-1 {
			t.Errorf("%s: line count", tt.name)
		}
	}
}

func TestListContinuationFence(t *testing.T) {
	tests := []struct {
		name, text string
		want       []bool
		unclosed   bool
	}{
		{"closed in the item", "- item\n\n  ```\n  l\n  ```\ntext", []bool{false, false, true, true, true, false}, false},
		{"unclosed ends with the item", "- item\n\n  ```\n  l\n## Next\nafter", []bool{false, false, true, true, false, false}, false},
		{"unclosed to the end", "- item\n\n  ```\n  l\n  m", []bool{false, false, true, true, true}, true},
		{"closer at 4 spaces is relative 2", "- item\n\n  ```\n  l\n    ```\ntext", []bool{false, false, true, true, true, false}, false},
		{"top-level fence stays top-level", "text\n\n  ```\n  l\n```\nafter", []bool{false, false, true, true, true, false}, false},
		// After "para" the item has ended, so the fence is top level and a closer
		// indented 4 spaces is not a closer; inside the item (rel. 2) it would be.
		{"a less indented line ends the item", "- item\n\npara\n\n  ```\n  l\n    ```\ntext", []bool{false, false, false, false, true, true, true, true}, true},
		{"fence after the list ended", "- item\n\npara\n\n  ```\n  l\n  ```", []bool{false, false, false, false, true, true, true}, false},
	}
	for _, tt := range tests {
		got, unclosed := CodeLines(tt.text)
		if len(got) != len(tt.want) || unclosed != tt.unclosed {
			t.Errorf("%s: %v unclosed=%v", tt.name, got, unclosed)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("%s: line %d fenced=%v, want %v (%v)", tt.name, i, got[i], tt.want[i], got)
				break
			}
		}
	}
}
