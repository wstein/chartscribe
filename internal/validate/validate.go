// SPDX-License-Identifier: EUPL-1.2

// Package validate checks a note collection against the rules of
// specs/001-note-tooling/contracts/validate-rules.md. It only reads files: no
// network, no process start, no write.
package validate

import (
	"sort"

	"github.com/wstein/chartscribe/internal/collection"
	"github.com/wstein/chartscribe/internal/note"
)

// Rules lists the rule names in contract order.
var Rules = []string{
	"frontmatter", "field-unknown", "id-missing", "id-invalid", "id-duplicate", "list-type", "tag-invalid", "tag-count",
	"title-char", "name-duplicate", "source-missing", "wikilink-unresolved", "map-unlisted", "map-multiple", "map-category",
	"map-duplicate", "category-no-map", "readme-link",
	"source-kind-unknown", "source-malformed", "source-file-missing", "source-not-file", "source-range-invalid", "source-outside-root",
}

// WarningRules lists the rules that report warnings, not findings: they never
// change the exit status. source-stale is produced by internal/stale, not by Run.
var WarningRules = []string{"source-stale"}

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
	// Sources lists the parsed Source lines of every note that is not a map and
	// whose frontmatter parsed, in file order (notes without a Source line are left out).
	Sources []NoteSources
}

// NoteSources is the parsed Source lines of one note.
type NoteSources struct {
	Note    string // file name
	Sources []note.Source
}

// Options switch on the opt-in checks of RunWith.
type Options struct {
	// CheckSources verifies that each cited file (kinds file and measurement) exists
	// below SourceRoot, is a regular file and, for lines A-B, that the range is real.
	CheckSources bool
	// SourceRoot is the directory source paths are relative to; "" means
	// collection.DefaultSourceRoot of the collection root.
	SourceRoot string
}

// Overlay replaces the text of files read from disk, so a candidate result can
// be validated without writing it. Keys are slash paths relative to the root:
// "notes/<name>.md" and "README.md". Line endings of a value are taken as given.
type Overlay map[string]string

// Run validates the collection at root (a directory with a notes/ folder).
// An error means the collection could not be read; rule violations are findings.
func Run(root string) (Result, error) { return RunOverlay(root, nil) }

// RunOverlay is Run with the files named in overlay read from it instead of from disk.
func RunOverlay(root string, overlay Overlay) (Result, error) {
	return RunWith(root, overlay, Options{})
}

// RunWith is RunOverlay with the opt-in checks of opts.
func RunWith(root string, overlay Overlay, opts Options) (Result, error) {
	if opts.CheckSources && opts.SourceRoot == "" {
		opts.SourceRoot = collection.DefaultSourceRoot(root)
	}
	vocab, err := collection.LoadVocabulary(root)
	if err != nil {
		return Result{}, err
	}
	files, err := collection.ReadNotes(root)
	if err != nil {
		return Result{}, err
	}
	for i, f := range files {
		if text, ok := overlay["notes/"+f.Name]; ok {
			files[i].Text = text
		}
	}
	c := &checker{vocab: vocab, overlay: overlay, opts: opts}
	if err := c.readme(root); err != nil {
		return Result{}, err
	}
	c.notes(files)
	c.duplicates()
	c.wikilinks()
	c.maps()
	return Result{Notes: len(files), Findings: sortUnique(c.findings), Sources: c.sources}, nil
}

type checker struct {
	vocab    collection.Vocabulary
	overlay  Overlay
	opts     Options
	sources  []NoteSources
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
