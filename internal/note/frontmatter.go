// SPDX-License-Identifier: EUPL-1.2

// Package note parses the parts of a note that cscribe checks: the frontmatter
// subset, ids and wikilinks.
package note

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// Value is a frontmatter value: a scalar or a list of strings. A list that
// held an element that is not a string (a JSON number, object, nested list)
// has NonString set; its List still carries a text form of each element.
type Value struct {
	IsList    bool
	Text      string
	List      []string
	NonString bool
}

// Scalar returns a scalar Value.
func Scalar(s string) Value { return Value{Text: s} }

// ListOf returns a list Value.
func ListOf(items ...string) Value { return Value{IsList: true, List: append([]string{}, items...)} }

// Frontmatter holds the parsed fields in order of first appearance.
type Frontmatter struct {
	keys   []string
	values map[string]Value
}

// Keys returns the keys in order of first appearance.
func (f Frontmatter) Keys() []string { return f.keys }

// Get returns the value of key.
func (f Frontmatter) Get(key string) (Value, bool) {
	v, ok := f.values[key]
	return v, ok
}

var (
	itemLine    = regexp.MustCompile(`^\s*-\s+(.*)$`)
	pairLine    = regexp.MustCompile(`^([A-Za-z_][\p{L}\p{N}_-]*):\s*(.*)$`)
	trailingCmt = regexp.MustCompile(`\s+#.*$`)
)

// Parse splits text into frontmatter and body. The frontmatter is the YAML
// subset of the validate contract: `key: value` lines with plain, double- and
// single-quoted scalars, flow lists and block lists, and `#` comments.
func Parse(text string) (Frontmatter, string, error) {
	if !strings.HasPrefix(text, "---\n") {
		return Frontmatter{}, "", errors.New("no frontmatter")
	}
	end := strings.Index(text[3:], "\n---\n")
	if end < 0 {
		return Frontmatter{}, "", errors.New("unterminated frontmatter")
	}
	end += 3
	f := Frontmatter{values: map[string]Value{}}
	key := ""
	block := ""
	if end > 4 {
		block = text[4:end]
	}
	for _, line := range strings.Split(block, "\n") {
		before, _, _ := strings.Cut(line, "#")
		if strings.TrimSpace(before) == "" && !strings.HasPrefix(strings.TrimSpace(line), "-") {
			continue
		}
		if m := itemLine.FindStringSubmatch(line); m != nil {
			if cur, ok := f.values[key]; key != "" && ok && cur.IsList {
				raw := strings.TrimSpace(m[1])
				if strings.HasPrefix(raw, "[") {
					// As in the reference, an item starting with [ must be JSON; it is not a string.
					var nested []any
					if err := json.Unmarshal([]byte(raw), &nested); err != nil {
						return Frontmatter{}, "", fmt.Errorf("invalid list item %s", raw)
					}
					cur.NonString = true
					cur.List = append(cur.List, raw)
				} else {
					item, err := parseScalarOrList(raw)
					if err != nil {
						return Frontmatter{}, "", err
					}
					cur.List = append(cur.List, item.Text)
				}
				f.values[key] = cur
			}
			continue
		}
		m := pairLine.FindStringSubmatch(line)
		if m == nil {
			return Frontmatter{}, "", fmt.Errorf("cannot parse frontmatter line: %q", line)
		}
		key = m[1]
		raw := trailingCmt.ReplaceAllString(m[2], "")
		var v Value
		if raw == "" {
			v = ListOf()
		} else {
			var err error
			if v, err = parseScalarOrList(raw); err != nil {
				return Frontmatter{}, "", err
			}
		}
		if _, seen := f.values[key]; !seen {
			f.keys = append(f.keys, key)
		}
		f.values[key] = v
	}
	return f, text[end+5:], nil
}

func parseScalarOrList(raw string) (Value, error) {
	raw = strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(raw, `"`):
		var s string
		if err := json.Unmarshal([]byte(raw), &s); err != nil {
			return Value{}, fmt.Errorf("invalid quoted value %s", raw)
		}
		return Scalar(s), nil
	case strings.HasPrefix(raw, `'`):
		if s, ok := unquoteSingle(raw); ok {
			return Scalar(s), nil
		}
		return Scalar(raw), nil
	case strings.HasPrefix(raw, "["):
		return parseFlowList(raw)
	}
	return Scalar(raw), nil
}

func unquoteSingle(raw string) (string, bool) {
	if len(raw) < 2 || raw[0] != '\'' || raw[len(raw)-1] != '\'' {
		return "", false
	}
	inner := raw[1 : len(raw)-1]
	for i := 0; i < len(inner); i++ {
		if inner[i] == '\'' {
			if i+1 < len(inner) && inner[i+1] == '\'' {
				i++
				continue
			}
			return "", false
		}
	}
	return strings.ReplaceAll(inner, "''", "'"), true
}

// parseFlowList parses a JSON array first (the reference behaviour); when that
// fails it accepts the YAML flow style Obsidian writes: plain and
// single-quoted items.
func parseFlowList(raw string) (Value, error) {
	var items []any
	if err := json.Unmarshal([]byte(raw), &items); err == nil {
		v := ListOf()
		for _, it := range items {
			if s, ok := it.(string); ok {
				v.List = append(v.List, s)
				continue
			}
			v.NonString = true
			b, _ := json.Marshal(it)
			v.List = append(v.List, string(b))
		}
		return v, nil
	}
	if !strings.HasSuffix(raw, "]") {
		return Value{}, fmt.Errorf("invalid flow list %s", raw)
	}
	inner := strings.TrimSpace(raw[1 : len(raw)-1])
	v := ListOf()
	if inner == "" {
		return v, nil
	}
	var cur strings.Builder
	var quote byte
	flush := func() error {
		item := strings.TrimSpace(cur.String())
		cur.Reset()
		if item == "" {
			return fmt.Errorf("invalid flow list %s: empty item", raw)
		}
		switch item[0] {
		case '"':
			var s string
			if err := json.Unmarshal([]byte(item), &s); err != nil {
				return fmt.Errorf("invalid flow list %s: bad quoted item", raw)
			}
			v.List = append(v.List, s)
		case '\'':
			s, ok := unquoteSingle(item)
			if !ok {
				return fmt.Errorf("invalid flow list %s: bad quoted item", raw)
			}
			v.List = append(v.List, s)
		default:
			if strings.ContainsAny(item, `[]"`) {
				return fmt.Errorf("invalid flow list %s: bad item %q", raw, item)
			}
			v.List = append(v.List, item)
		}
		return nil
	}
	for i := 0; i < len(inner); i++ {
		c := inner[i]
		switch {
		case quote != 0:
			cur.WriteByte(c)
			if quote == '"' && c == '\\' && i+1 < len(inner) {
				i++
				cur.WriteByte(inner[i])
			} else if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			if strings.TrimSpace(cur.String()) == "" {
				quote = c
			}
			cur.WriteByte(c)
		case c == ',':
			if err := flush(); err != nil {
				return Value{}, err
			}
		default:
			cur.WriteByte(c)
		}
	}
	if quote != 0 {
		return Value{}, fmt.Errorf("invalid flow list %s: unterminated quote", raw)
	}
	if err := flush(); err != nil {
		return Value{}, err
	}
	return v, nil
}
