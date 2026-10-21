// SPDX-License-Identifier: EUPL-1.2

package fix

import (
	"strings"
	"unicode"

	"github.com/wstein/chartscribe/internal/collection"
	"github.com/wstein/chartscribe/internal/note"
	"github.com/wstein/chartscribe/internal/validate"
)

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// normalizeTags trims a tag, drops a leading #, lower-cases a type/, status/ or category/ tag when the
// lower-case value is in the vocabulary, and drops empty and repeated tags. Other tags keep their text.
func normalizeTags(in []string, v collection.Vocabulary) []string {
	namespaces := []struct {
		name    string
		allowed []string
	}{
		{"type", append(append([]string{}, v.Type...), "map")},
		{"status", v.Status},
		{"category", v.Category},
	}
	seen := map[string]bool{}
	out := []string{}
	for _, t := range in {
		t = strings.TrimPrefix(strings.TrimSpace(t), "#")
		lower := strings.ToLower(t)
		for _, ns := range namespaces {
			if value, ok := strings.CutPrefix(lower, ns.name+"/"); ok && contains(ns.allowed, value) {
				t = lower
				break
			}
		}
		if t == "" || seen[t] {
			continue
		}
		seen[t] = true
		out = append(out, t)
	}
	return out
}

// dedupeAliases drops aliases that repeat the title or an earlier alias, case-insensitively.
func dedupeAliases(in []string, title string) []string {
	seen := map[string]bool{strings.ToLower(title): true}
	out := []string{}
	for _, a := range in {
		if seen[strings.ToLower(a)] {
			continue
		}
		seen[strings.ToLower(a)] = true
		out = append(out, a)
	}
	return out
}

// scalarTag reports whether a scalar tags value is one tag certainly.
func scalarTag(s string) bool {
	return s != "" && !strings.ContainsAny(s, ",[]\"") && !strings.ContainsFunc(s, unicode.IsSpace)
}

func scalarAlias(s string) bool { return s != "" && !strings.ContainsAny(s, ",[]\"") }

// derivedID returns the id a malformed id names unambiguously: exactly 14 digits, with only the
// separators - : _ space and T between them, that form a real date and time.
func derivedID(raw string) (string, bool) {
	var digits strings.Builder
	for _, r := range raw {
		switch {
		case r >= '0' && r <= '9':
			digits.WriteRune(r)
		case strings.ContainsRune("-:_ Tt", r):
		default:
			return "", false
		}
	}
	id := digits.String()
	if len(id) != 14 || !note.ValidID(id) {
		return "", false
	}
	return id, true
}

// noteEdits plans the frontmatter fixes of one note. found holds the rules of the findings of the note.
func noteEdits(file, name, text string, vocab collection.Vocabulary, usedIDs map[string]bool, found map[string]bool) []lineEdit {
	fm, _, err := note.Parse(text)
	if err != nil {
		return nil
	}
	lay, ok := newLayout(text)
	if !ok {
		return nil
	}
	title := strings.TrimSuffix(name, ".md")
	var edits []lineEdit
	add := func(key, line, rule string) {
		if e, ok := lay.replaceKey(file, key, line, rule); ok {
			edits = append(edits, e)
		}
	}

	if v, ok := fm.Get("id"); ok && !v.IsList && !note.ValidID(v.Text) {
		if id, ok := derivedID(v.Text); ok && !usedIDs[id] {
			add("id", "id: "+id, "id-invalid")
		}
	}
	if v, ok := fm.Get("aliases"); ok && !v.NonString {
		switch {
		case v.IsList:
			if want := dedupeAliases(v.List, title); !equal(want, v.List) {
				add("aliases", flowList("aliases", want), "name-duplicate")
			}
		case scalarAlias(v.Text):
			add("aliases", flowList("aliases", dedupeAliases([]string{v.Text}, title)), "list-type")
		}
	}
	if v, ok := fm.Get("tags"); ok && !v.NonString {
		rule := "tag-count"
		if found["tag-invalid"] {
			rule = "tag-invalid"
		}
		switch {
		case v.IsList && (found["tag-invalid"] || found["tag-count"]):
			if want := normalizeTags(v.List, vocab); !equal(want, v.List) {
				add("tags", flowList("tags", want), rule)
			}
		case !v.IsList && scalarTag(v.Text):
			add("tags", flowList("tags", normalizeTags([]string{v.Text}, vocab)), "list-type")
		}
	}
	return edits
}

// noteRules returns, per note file, the rules of its findings.
func noteRules(findings []validate.Finding) map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for _, f := range findings {
		if f.Note == "" {
			continue
		}
		if out[f.Note] == nil {
			out[f.Note] = map[string]bool{}
		}
		out[f.Note][f.Rule] = true
	}
	return out
}
