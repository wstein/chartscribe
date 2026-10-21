// SPDX-License-Identifier: EUPL-1.2

package export

import (
	"regexp"
	"strings"

	"github.com/wstein/chartscribe/internal/note"
)

type kind int

const (
	kText    kind = iota // prose: links are rewritten outside inline code
	kCode                // fenced or indented code, copied as it is
	kHeading             // an ATX heading outside code
)

// pline is one kept body line. For a heading, text is the heading text and
// level its level in the note.
type pline struct {
	text  string
	kind  kind
	level int
}

// parsed is a note body split once for member discovery and for rendering.
type parsed struct {
	lines      []pline
	targets    []string // wikilink targets in prose, in order, not in the Links section
	sources    []string // Source: texts in order
	shallowest int      // level of the shallowest kept heading
}

var (
	atx      = regexp.MustCompile(`^ {0,3}(#{1,6})[ \t]+(.*?)[ \t]*$`)
	source   = regexp.MustCompile(`^Source: (.+)$`)
	listItem = regexp.MustCompile(`^ {0,3}([-*+]|[0-9]{1,9}[.)])([ \t]|$)`)
)

// parse applies the Markdown code rules once: fences (note.CodeLines),
// indented code (4 spaces or a tab after a blank line, not within a list) and,
// in mapText, inline code spans. The "## Links" section is dropped.
func parse(body string) *parsed {
	p := &parsed{}
	indented, inList, skipLinks := false, false, false
	prevBlank := true
	keep := func(l pline) {
		if !skipLinks {
			p.lines = append(p.lines, l)
		}
	}
	text := strings.TrimRight(body, "\n")
	// Fenced code is found by the scanner that validate and tracking share; an
	// unclosed fence runs to the end of the body.
	fenced, _ := note.CodeLines(text)
	for i, raw := range strings.Split(text, "\n") {
		if fenced[i] {
			indented, prevBlank = false, false
			keep(pline{text: raw, kind: kCode})
			continue
		}
		if strings.TrimSpace(raw) == "" {
			prevBlank = true
			if !skipLinks {
				p.lines = append(p.lines, pline{text: raw, kind: kText})
			}
			continue
		}
		if strings.HasPrefix(raw, "    ") || strings.HasPrefix(raw, "\t") {
			if indented || (prevBlank && !inList) {
				indented, prevBlank = true, false
				keep(pline{text: raw, kind: kCode})
				continue
			}
			// A lazy continuation or list content: prose.
		} else {
			indented = false
			inList = listItem.MatchString(raw) || (inList && !prevBlank)
		}
		prevBlank = false
		if m := atx.FindStringSubmatch(raw); m != nil && !strings.HasPrefix(raw, "    ") && raw[0] != '\t' {
			skipLinks = len(m[1]) == 2 && m[2] == "Links"
			if skipLinks {
				continue
			}
			if p.shallowest == 0 || len(m[1]) < p.shallowest {
				p.shallowest = len(m[1])
			}
			p.lines = append(p.lines, pline{text: m[2], kind: kHeading, level: len(m[1])})
			continue
		}
		if m := source.FindStringSubmatch(raw); m != nil {
			p.sources = append(p.sources, strings.TrimSpace(m[1]))
			continue
		}
		if skipLinks {
			continue
		}
		mapText(raw, func(text string) string {
			for _, m := range wikilink.FindAllStringSubmatch(text, -1) {
				p.targets = append(p.targets, m[1])
			}
			return text
		})
		p.lines = append(p.lines, pline{text: raw, kind: kText})
	}
	return p
}

// mapText applies f to the parts of a prose line outside inline code spans
// (a backtick run closed by a run of the same length) and returns the line.
func mapText(line string, f func(string) string) string {
	var out strings.Builder
	rest := line
	for {
		i := strings.IndexByte(rest, '`')
		if i < 0 {
			out.WriteString(f(rest))
			return out.String()
		}
		n := 0
		for i+n < len(rest) && rest[i+n] == '`' {
			n++
		}
		end := closingRun(rest[i+n:], n)
		if end < 0 {
			// No closing run: the backticks are literal text.
			out.WriteString(f(rest[:i+n]))
			rest = rest[i+n:]
			continue
		}
		out.WriteString(f(rest[:i]))
		stop := i + n + end + n
		out.WriteString(rest[i:stop])
		rest = rest[stop:]
	}
}

// closingRun returns the index in s of the first backtick run of exactly n
// backticks, or -1.
func closingRun(s string, n int) int {
	for i := 0; i < len(s); {
		if s[i] != '`' {
			i++
			continue
		}
		j := i
		for j < len(s) && s[j] == '`' {
			j++
		}
		if j-i == n {
			return i
		}
		i = j
	}
	return -1
}
