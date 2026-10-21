// SPDX-License-Identifier: EUPL-1.2

package fix

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
)

var (
	itemLine    = regexp.MustCompile(`^\s*-\s+(.*)$`)
	pairLine    = regexp.MustCompile(`^([A-Za-z_][\p{L}\p{N}_-]*):\s*(.*)$`)
	trailingCmt = regexp.MustCompile(`\s+#.*$`)
)

// lineEdit replaces lines [start, end) of the original text by repl; start == end inserts.
type lineEdit struct {
	file       string
	start, end int
	repl       []string
	rule       string
}

// applyEdits applies edits (positions refer to orig) and reports false when two overlap.
func applyEdits(orig string, edits []lineEdit) (string, bool) {
	lines := strings.Split(orig, "\n")
	sorted := append([]lineEdit{}, edits...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].start < sorted[j].start })
	var out []string
	pos := 0
	for _, e := range sorted {
		if e.start < pos || e.end < e.start || e.end > len(lines) {
			return "", false
		}
		out = append(out, lines[pos:e.start]...)
		out = append(out, e.repl...)
		pos = e.end
	}
	out = append(out, lines[pos:]...)
	return strings.Join(out, "\n"), true
}

// layout locates the frontmatter lines of a note text.
type layout struct {
	lines []string
	end   int // index of the closing --- line
}

func newLayout(text string) (layout, bool) {
	lines := strings.Split(text, "\n")
	if len(lines) < 2 || lines[0] != "---" {
		return layout{}, false
	}
	for i := 1; i < len(lines)-1; i++ {
		if lines[i] == "---" {
			return layout{lines: lines, end: i}, true
		}
	}
	return layout{}, false
}

// body returns the text after the closing --- line.
func body(text string) string {
	l, ok := newLayout(text)
	if !ok {
		return text
	}
	return strings.Join(l.lines[l.end+1:], "\n")
}

// key finds the lines of a frontmatter key: the key line and the block list items that follow it.
// It reports false when the key is absent or repeated, carries a trailing comment, or is followed
// by item lines the rewrite could not keep apart from it.
func (l layout) key(name string) (start, end int, ok bool) {
	start = -1
	for i := 1; i < l.end; i++ {
		if itemLine.MatchString(l.lines[i]) {
			continue
		}
		m := pairLine.FindStringSubmatch(l.lines[i])
		if m == nil || m[1] != name {
			continue
		}
		if start >= 0 {
			return 0, 0, false
		}
		start = i
		if trailingCmt.ReplaceAllString(m[2], "") != m[2] {
			return 0, 0, false
		}
	}
	if start < 0 {
		return 0, 0, false
	}
	end = start + 1
	for end < l.end && itemLine.MatchString(l.lines[end]) {
		end++
	}
	for k := end; k < l.end; k++ {
		trimmed := strings.TrimSpace(l.lines[k])
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			if itemLine.MatchString(l.lines[k]) {
				return 0, 0, false // an orphan item would join the rewritten key
			}
			break
		}
	}
	return start, end, true
}

// replaceKey rewrites the lines of a key by one line.
func (l layout) replaceKey(file, name, newLine, rule string) (lineEdit, bool) {
	start, end, ok := l.key(name)
	if !ok {
		return lineEdit{}, false
	}
	return lineEdit{file: file, start: start, end: end, repl: []string{newLine}, rule: rule}, true
}

// flowList renders a list the way the collection writes it: ["a", "b"].
func flowList(key string, items []string) string {
	parts := make([]string, len(items))
	for i, it := range items {
		var b strings.Builder
		enc := json.NewEncoder(&b)
		enc.SetEscapeHTML(false)
		_ = enc.Encode(it)
		parts[i] = strings.TrimSpace(b.String())
	}
	return key + ": [" + strings.Join(parts, ", ") + "]"
}
