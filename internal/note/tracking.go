// SPDX-License-Identifier: EUPL-1.2

package note

import "strings"

// ReplaceTracking returns body with the generated `## Tracking` section
// replaced by lines, appended when missing, or removed when lines is empty.
// Bytes outside the section are unchanged, except that a missing final
// newline is added before an appended section.
func ReplaceTracking(body string, lines []string) string {
	section := ""
	if len(lines) > 0 {
		section = "## Tracking\n\n" + strings.Join(lines, "\n") + "\n"
	}
	start, end, ok := trackingSpan(body)
	if !ok {
		if section == "" {
			return body
		}
		switch {
		case body == "" || strings.HasSuffix(body, "\n\n"):
		case strings.HasSuffix(body, "\n"):
			body += "\n"
		default:
			body += "\n\n"
		}
		return body + section
	}
	if section != "" && end < len(body) {
		section += "\n" // a heading follows
	}
	return body[:start] + section + body[end:]
}
