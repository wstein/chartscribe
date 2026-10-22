// SPDX-License-Identifier: EUPL-1.2

package validate

import (
	"fmt"
	"strings"

	"github.com/wstein/chartscribe/internal/collection"
	"github.com/wstein/chartscribe/internal/note"
	"github.com/wstein/chartscribe/internal/sourcecheck"
)

// parsedNote is a note whose frontmatter parsed.
type parsedNote struct {
	name    string // file name
	title   string
	tags    []string
	aliases []string
	body    string // body without the generated tracking section
	isMap   bool
}

// category returns the single category tag value of the note, if it has exactly one.
func (n parsedNote) category() (string, bool) {
	var found []string
	for _, t := range n.tags {
		if rest, ok := strings.CutPrefix(t, "category/"); ok {
			found = append(found, rest)
		}
	}
	if len(found) == 1 {
		return found[0], true
	}
	return "", false
}

func (c *checker) notes(files []collection.File) {
	for _, f := range files {
		title := f.Title()
		if note.ForbiddenTitle.MatchString(title) {
			c.add("title-char", f.Name, "forbidden character in the title (one of "+note.ForbiddenTitleChars+")")
		}
		fm, rawBody, err := note.Parse(f.Text)
		if err != nil {
			c.add("frontmatter", f.Name, "bad frontmatter: "+err.Error())
			continue
		}
		for _, key := range fm.Keys() {
			if key != "id" && key != "aliases" && key != "tags" {
				c.add("field-unknown", f.Name, fmt.Sprintf("unknown frontmatter field %q", key))
			}
		}
		c.checkID(f.Name, fm)

		aliasVal, hasAliases := fm.Get("aliases")
		tagVal, hasTags := fm.Get("tags")
		if hasAliases && (!aliasVal.IsList || aliasVal.NonString) {
			c.add("list-type", f.Name, "aliases must be a list of strings")
		}
		if !hasTags || !tagVal.IsList || tagVal.NonString {
			c.add("list-type", f.Name, "tags must be a list of strings")
		}
		n := parsedNote{name: f.Name, title: title, body: note.StripTracking(rawBody)}
		if aliasVal.IsList {
			n.aliases = aliasVal.List
		}
		if tagVal.IsList {
			n.tags = tagVal.List
		}
		c.names.add(strings.ToLower(title), f.Name)
		for _, a := range n.aliases {
			c.names.add(strings.ToLower(a), f.Name+" (alias)")
		}
		for _, t := range n.tags {
			if t == "type/map" {
				n.isMap = true
			}
		}
		c.checkTags(n)
		if !n.isMap {
			c.checkSources(f.Name, n.body)
		}
		c.parsed = append(c.parsed, n)
	}
}

func (c *checker) checkID(name string, fm note.Frontmatter) {
	v, ok := fm.Get("id")
	if !ok {
		c.add("id-missing", name, "missing id")
		return
	}
	id := v.Text
	if v.IsList {
		id = "[" + strings.Join(v.List, ", ") + "]"
	}
	if v.IsList || !note.ValidID(id) {
		c.add("id-invalid", name, fmt.Sprintf("invalid id %q; use 14 digits YYYYMMDDHHMMSS (optional -NNN)", id))
		return
	}
	c.ids.add(id, name)
}

func (c *checker) checkTags(n parsedNote) {
	namespaces := []struct {
		name    string
		allowed []string
	}{
		{"type", append(append([]string{}, c.vocab.Type...), "map")},
		{"status", c.vocab.Status},
		{"category", c.vocab.Category},
	}
	for _, ns := range namespaces {
		count := 0
		for _, t := range n.tags {
			value, ok := strings.CutPrefix(t, ns.name+"/")
			if !ok {
				continue
			}
			count++
			if !contains(ns.allowed, value) {
				c.add("tag-invalid", n.name, fmt.Sprintf("invalid tag %q; allowed %s/: %s", t, ns.name, strings.Join(ns.allowed, ", ")))
			}
		}
		if n.isMap && ns.name == "status" {
			continue
		}
		if count != 1 {
			c.add("tag-count", n.name, fmt.Sprintf("needs exactly one %s/ tag, found %d", ns.name, count))
		}
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func (c *checker) duplicates() {
	for _, id := range c.ids.keys {
		if owners := c.ids.owners[id]; len(owners) > 1 {
			c.add("id-duplicate", "", fmt.Sprintf("duplicate id %s: %s", id, strings.Join(owners, ", ")))
		}
	}
	for _, name := range c.names.keys {
		if owners := c.names.owners[name]; len(owners) > 1 {
			c.add("name-duplicate", "", fmt.Sprintf("duplicate title or alias %q: %s", name, strings.Join(owners, ", ")))
		}
	}
}

// checkSources applies the source rules to the Source lines of one note. The
// syntax rules always apply; the file checks only with Options.CheckSources.
// A note with no well-formed Source line has source-missing, unless a line
// with a kind prefix already has its own finding.
func (c *checker) checkSources(name, body string) {
	srcs := note.ParseSources(body)
	valid, flagged := false, false
	for _, s := range srcs {
		switch {
		case s.Kind == note.KindUnknown:
			flagged = true
			c.add("source-kind-unknown", name, fmt.Sprintf("unknown Source kind %q (known: %s, or \"<path>, lines A-B\")",
				s.KindName, strings.Join(note.Kinds, ", ")))
		case s.Malformed:
			flagged = true
			c.add("source-malformed", name, fmt.Sprintf("malformed Source line %q; use %s", s.Raw, sourceForm[s.Kind]))
		default:
			valid = true
			if c.opts.CheckSources {
				if p := sourcecheck.Check(c.opts.SourceRoot, s); p != nil {
					c.add(p.Rule, name, p.Message)
				}
			}
		}
	}
	if !valid && !flagged {
		c.add("source-missing", name, "missing Source line at the end of the body")
	}
	if len(srcs) > 0 {
		c.sources = append(c.sources, NoteSources{Note: name, Sources: srcs})
	}
}

var sourceForm = map[string]string{
	note.KindDecision:    `"decision: <text>, <YYYY-MM-DD>"`,
	note.KindURL:         `"url: <http or https URL>"`,
	note.KindMeasurement: `"measurement: <path>[, <key>]"`,
}
