// SPDX-License-Identifier: EUPL-1.2

package note

import "testing"

func TestClosingFence(t *testing.T) {
	tests := []struct {
		name, text, want string
		ok               bool
	}{
		{"closed", "```\nx\n```\n", "", false},
		{"backticks", "a\n```go\nx", "```", true},
		{"long tilde", "~~~~~\nx", "~~~~~", true},
		{"after a closed fence", "```\nx\n```\n````\ny", "````", true},
		{"list item", "- ```\n  x", "  ```", true},
		{"indented fence", "  ```\n  x", "```", true},
		{"quote", "> ```\n> x", "> ```", true},
		{"none", "plain", "", false},
	}
	for _, tt := range tests {
		got, ok := ClosingFence(tt.text)
		if got != tt.want || ok != tt.ok {
			t.Errorf("%s: ClosingFence = %q, %v; want %q, %v", tt.name, got, ok, tt.want, tt.ok)
		}
	}
}
