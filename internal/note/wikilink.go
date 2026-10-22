// SPDX-License-Identifier: EUPL-1.2

package note

import (
	"regexp"
	"strings"
)

// ForbiddenTitle matches a character that a note title may not contain: the
// characters a file name cannot hold and those that cut a wikilink target
// ([[a#b]] names the heading b of a), which would make the note unlinkable.
// validate (title-char) and new share this one definition.
var ForbiddenTitle = regexp.MustCompile(`[\\/:*?"<>|#\[\]^]`)

// ForbiddenTitleChars lists the characters of ForbiddenTitle for messages.
const ForbiddenTitleChars = `\ / : * ? " < > | # [ ] ^`

var (
	wikilink = regexp.MustCompile(`\[\[([^\]\n]+)\]\]`)
)

// HasSource reports whether body has a well-formed Source line: the legacy
// `Source: <text>, lines A-B` or a typed one (see ParseSources).
func HasSource(body string) bool {
	for _, s := range ParseSources(body) {
		if s.Valid() {
			return true
		}
	}
	return false
}

// sourceEnd returns the end offset of the first well-formed Source line.
func sourceEnd(body string) (int, bool) {
	for _, loc := range anySourceLine.FindAllStringIndex(body, -1) {
		if s, ok := parseSource(strings.TrimPrefix(body[loc[0]:loc[1]], "Source: ")); ok && s.Valid() {
			return loc[1], true
		}
	}
	return 0, false
}

// WikiTargets returns the raw text between [[ and ]] of every wikilink in body.
func WikiTargets(body string) []string {
	var out []string
	for _, m := range wikilink.FindAllStringSubmatch(body, -1) {
		out = append(out, m[1])
	}
	return out
}

// LinkName reduces a wikilink target to the lower-case title or alias it
// names: the label (after |) and the heading (after #) are dropped.
func LinkName(target string) string {
	if i := strings.IndexAny(target, "|#"); i >= 0 {
		target = target[:i]
	}
	return strings.ToLower(strings.TrimSpace(target))
}

// StripTracking removes the generated `## Tracking` section: it starts at the
// line `## Tracking` (after the Source line when the body has one) and ends
// before the next `## ` heading or at the end of the body.
func StripTracking(body string) string {
	start, end, ok := trackingSpan(body)
	if !ok {
		return body
	}
	return body[:start] + body[end:]
}

// trackingSpan returns the byte range of the first tracking section of body.
// Headings inside fenced code blocks are not headings.
func trackingSpan(body string) (start, end int, ok bool) {
	from := 0
	if end, ok := sourceEnd(body); ok {
		from = end
	}
	start = -1
	end = len(body)
	eachLine(body, func(ls, le int, fenced bool) bool {
		if fenced {
			return true
		}
		line := body[ls:le]
		if start < 0 {
			if ls >= from && line == "## Tracking" {
				start = ls
			}
			return true
		}
		if ls > start && strings.HasPrefix(line, "## ") {
			end = ls
			return false
		}
		return true
	})
	if start < 0 {
		return 0, 0, false
	}
	return start, end, true
}

// StaleTracking reports a second `## Tracking` heading after the first
// section (outside code fences); ReplaceTracking leaves it in place.
func StaleTracking(body string) bool {
	_, end, ok := trackingSpan(body)
	if !ok {
		return false
	}
	found := false
	eachLine(body, func(ls, le int, fenced bool) bool {
		if ls >= end && !fenced && body[ls:le] == "## Tracking" {
			found = true
			return false
		}
		return true
	})
	return found
}

// StripCode removes the lines of fenced code blocks (fences included), so a
// marker inside a code block is not read as a marker.
func StripCode(text string) string {
	var sb strings.Builder
	eachLine(text, func(ls, le int, fenced bool) bool {
		if !fenced {
			sb.WriteString(text[ls:min(le+1, len(text))])
		}
		return true
	})
	return sb.String()
}
