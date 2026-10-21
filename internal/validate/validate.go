// SPDX-License-Identifier: EUPL-1.2

// Package validate checks a note collection against the rules of
// specs/001-note-tooling/contracts/validate-rules.md. It only reads files: no
// network, no process start, no write.
package validate

import (
	"sort"

	"github.com/wstein/chartscribe/internal/collection"
)

// Rules lists the rule names in contract order.
var Rules = []string{
	"frontmatter", "field-unknown", "id-missing", "id-invalid", "id-duplicate", "list-type", "tag-invalid", "tag-count",
	"title-char", "name-duplicate", "source-missing", "wikilink-unresolved", "map-unlisted", "map-multiple", "map-category",
	"map-duplicate", "category-no-map", "readme-link",
}

// Finding is one rule violation. Note is the file name; it is empty for a
// collection-level finding.
type Finding struct {
	Rule    string
	Note    string
	Message string
}

// Result is the outcome of validating one collection.
type Result struct {
	Notes    int
	Findings []Finding
}

// Run validates the collection at root (a directory with a notes/ folder).
// An error means the collection could not be read; rule violations are findings.
func Run(root string) (Result, error) {
	vocab, err := collection.LoadVocabulary(root)
	if err != nil {
		return Result{}, err
	}
	files, err := collection.ReadNotes(root)
	if err != nil {
		return Result{}, err
	}
	c := &checker{vocab: vocab}
	if err := c.readme(root); err != nil {
		return Result{}, err
	}
	c.notes(files)
	c.duplicates()
	c.wikilinks()
	c.maps()
	return Result{Notes: len(files), Findings: sortUnique(c.findings)}, nil
}

type checker struct {
	vocab    collection.Vocabulary
	findings []Finding

	parsed []parsedNote // notes whose frontmatter parsed, in file order
	ids    orderedOwners
	names  orderedOwners
}

func (c *checker) add(rule, note, message string) {
	c.findings = append(c.findings, Finding{Rule: rule, Note: note, Message: message})
}

// sortUnique orders findings by note, rule and message and drops duplicates.
func sortUnique(in []Finding) []Finding {
	out := append([]Finding{}, in...)
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Note != b.Note {
			return a.Note < b.Note
		}
		if a.Rule != b.Rule {
			return a.Rule < b.Rule
		}
		return a.Message < b.Message
	})
	uniq := out[:0]
	for i, f := range out {
		if i == 0 || f != out[i-1] {
			uniq = append(uniq, f)
		}
	}
	return uniq
}

// orderedOwners maps a key to the owners that claim it, keeping the order of
// first appearance so messages are stable.
type orderedOwners struct {
	keys   []string
	owners map[string][]string
}

func (o *orderedOwners) add(key, owner string) {
	if o.owners == nil {
		o.owners = map[string][]string{}
	}
	if _, ok := o.owners[key]; !ok {
		o.keys = append(o.keys, key)
	}
	o.owners[key] = append(o.owners[key], owner)
}
