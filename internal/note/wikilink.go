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
	wikilink   = regexp.MustCompile(`\[\[([^\]\n]+)\]\]`)
	sourceLine = regexp.MustCompile(`(?m)^Source: .+, lines [0-9]+-[0-9]+$`)
)

// HasSource reports whether body has a line `Source: <text>, lines A-B`.
func HasSource(body string) bool { return sourceLine.MatchString(body) }

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

// trackingSpan returns the byte range of the tracking section of body.
func trackingSpan(body string) (start, end int, ok bool) {
	from := 0
	if loc := sourceLine.FindStringIndex(body); loc != nil {
		from = loc[1]
	}
	start = -1
	for pos := from; pos < len(body); {
		next := strings.IndexByte(body[pos:], '\n')
		lineEnd := len(body)
		if next >= 0 {
			lineEnd = pos + next
		}
		atLineStart := pos == 0 || body[pos-1] == '\n'
		if atLineStart && body[pos:lineEnd] == "## Tracking" {
			start = pos
			break
		}
		if next < 0 {
			break
		}
		pos = lineEnd + 1
	}
	if start < 0 {
		return 0, 0, false
	}
	end = len(body)
	for pos := start + len("## Tracking"); pos < len(body); {
		nl := strings.IndexByte(body[pos:], '\n')
		if nl < 0 {
			break
		}
		pos += nl + 1
		if strings.HasPrefix(body[pos:], "## ") {
			end = pos
			break
		}
	}
	return start, end, true
}
