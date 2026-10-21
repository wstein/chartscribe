// SPDX-License-Identifier: EUPL-1.2

// Package collection finds a note collection, reads its notes and its
// optional .cscribe.toml.
package collection

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ConfigName is the optional configuration file at the collection root.
const ConfigName = ".cscribe.toml"

// Vocabulary lists the allowed values of the namespaced tags. `map` is always
// allowed as a type and is not listed here.
type Vocabulary struct {
	Type     []string
	Status   []string
	Category []string
}

// DefaultVocabulary is the chartroom vocabulary.
func DefaultVocabulary() Vocabulary {
	return Vocabulary{
		Type:   []string{"idea", "decision", "milestone", "design", "evidence", "question", "process"},
		Status: []string{"open", "decided", "done", "dropped", "reference"},
		Category: []string{"direction", "state", "ratings", "roadmap", "design", "credentials-network",
			"providers", "interfaces", "recovery", "decisions", "delivery", "process"},
	}
}

// LoadVocabulary returns the vocabulary of the collection at root: the keys of
// .cscribe.toml override the default one by one; without the file it is the default.
//
// The file is a TOML subset parsed by hand, because the only construct needed
// is a [tags] table of string arrays and the repository takes no TOML
// dependency for it: `#` comments, basic ("...") and literal ('...') strings,
// arrays that may span lines and end with a comma. Anything else is an error.
func LoadVocabulary(root string) (Vocabulary, error) {
	v := DefaultVocabulary()
	data, err := os.ReadFile(filepath.Join(root, ConfigName))
	if errors.Is(err, os.ErrNotExist) {
		return v, nil
	}
	if err != nil {
		return Vocabulary{}, err
	}
	if err := parseConfig(string(data), &v); err != nil {
		return Vocabulary{}, fmt.Errorf("%s: %w", ConfigName, err)
	}
	return v, nil
}

func parseConfig(text string, v *Vocabulary) error {
	lines := strings.Split(text, "\n")
	table := ""
	seen := map[string]bool{}
	for i := 0; i < len(lines); i++ {
		lineNo := i + 1
		line := strings.TrimSpace(stripComment(lines[i]))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") && !strings.Contains(line, "=") {
			table = strings.TrimSpace(line[1 : len(line)-1])
			if table != "tags" {
				return fmt.Errorf("line %d: unknown table [%s]", lineNo, table)
			}
			continue
		}
		key, rest, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("line %d: cannot parse %q", lineNo, line)
		}
		key = strings.TrimSpace(key)
		if table == "" {
			return fmt.Errorf("line %d: key %q outside a table", lineNo, key)
		}
		// Collect the array across lines until the brackets balance.
		value := strings.TrimSpace(rest)
		for !balanced(value) && i+1 < len(lines) {
			i++
			value += " " + strings.TrimSpace(stripComment(lines[i]))
		}
		if seen[key] {
			return fmt.Errorf("line %d: duplicate key %q", lineNo, key)
		}
		seen[key] = true
		var target *[]string
		switch key {
		case "type":
			target = &v.Type
		case "status":
			target = &v.Status
		case "category":
			target = &v.Category
		default:
			return fmt.Errorf("line %d: unknown key %q in [tags]", lineNo, key)
		}
		items, err := parseStringArray(value)
		if err != nil {
			return fmt.Errorf("line %d: %s: %w", lineNo, key, err)
		}
		*target = items
	}
	return nil
}

// stripComment removes a # comment that is outside a string.
func stripComment(line string) string {
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case quote == '"' && c == '\\':
			i++
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '#':
			return line[:i]
		}
	}
	return line
}

// balanced reports whether the brackets of value outside strings are closed.
func balanced(value string) bool {
	depth := 0
	var quote byte
	for i := 0; i < len(value); i++ {
		c := value[i]
		switch {
		case quote == '"' && c == '\\':
			i++
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '[':
			depth++
		case c == ']':
			depth--
		}
	}
	return depth <= 0
}

func parseStringArray(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	if !strings.HasPrefix(value, "[") {
		return nil, errors.New("expected an array of strings")
	}
	if !strings.HasSuffix(value, "]") {
		return nil, errors.New("unterminated array")
	}
	inner := strings.TrimSpace(value[1 : len(value)-1])
	items := []string{}
	for inner != "" {
		var item string
		switch inner[0] {
		case '"':
			end := 1
			for end < len(inner) && inner[end] != '"' {
				if inner[end] == '\\' {
					end++
				}
				end++
			}
			if end >= len(inner) {
				return nil, errors.New("unterminated string")
			}
			if err := json.Unmarshal([]byte(inner[:end+1]), &item); err != nil {
				return nil, errors.New("invalid string escape")
			}
			inner = inner[end+1:]
		case '\'':
			end := strings.IndexByte(inner[1:], '\'')
			if end < 0 {
				return nil, errors.New("unterminated string")
			}
			item = inner[1 : end+1]
			inner = inner[end+2:]
		default:
			return nil, errors.New("array items must be strings")
		}
		if item == "" {
			return nil, errors.New("empty value")
		}
		items = append(items, item)
		inner = strings.TrimSpace(inner)
		if inner == "" {
			break
		}
		if inner[0] != ',' {
			return nil, errors.New("expected a comma between array items")
		}
		inner = strings.TrimSpace(inner[1:])
	}
	return items, nil
}
