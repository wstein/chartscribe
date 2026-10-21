// SPDX-License-Identifier: EUPL-1.2

package note

import "strings"

// fence is an open fenced code block: its character and length, and the
// container it sits in (blockquote levels, then the list content indent).
type fence struct {
	char   byte
	n      int
	quotes int
	indent int
}

// CodeLines reports, for every line of text split at "\n", whether it belongs
// to a fenced code block (the fence lines included), and whether text ends
// inside an unclosed fence. It follows the CommonMark rules for fenced code:
// a container prefix (blockquote `>` markers, list markers with their content
// indent) may precede the fence; the fence has 0-3 spaces of indent relative to
// the container and three or more backticks or tildes (a backtick fence has no
// backtick in its info string); it closes with the same character, at least as
// many, and only spaces after; a line that leaves the container ends the fence;
// an unclosed fence runs to the end.
func CodeLines(text string) (fenced []bool, unclosed bool) {
	var open *fence
	for _, line := range strings.Split(text, "\n") {
		if open != nil {
			rest, in := stripContainer(line, open)
			switch {
			case !in:
				open = nil // the container ended, and with it the fence
			case closes(rest, open):
				fenced = append(fenced, true)
				open = nil
				continue
			default:
				fenced = append(fenced, true)
				continue
			}
		}
		if f, ok := opens(line); ok {
			open = f
			fenced = append(fenced, true)
			continue
		}
		fenced = append(fenced, false)
	}
	return fenced, open != nil
}

// UnclosedFence reports whether text ends inside a fenced code block.
func UnclosedFence(text string) bool {
	_, u := CodeLines(text)
	return u
}

func leadingSpaces(s string) int { return len(s) - len(strings.TrimLeft(s, " ")) }

// listMarker returns the width of a list marker (with the spaces after it) at
// the start of s, or 0.
func listMarker(s string) int {
	i := 0
	switch {
	case s != "" && strings.IndexByte("-+*", s[0]) >= 0:
		i = 1
	default:
		for i < len(s) && i < 9 && s[i] >= '0' && s[i] <= '9' {
			i++
		}
		if i == 0 || i >= len(s) || (s[i] != '.' && s[i] != ')') {
			return 0
		}
		i++
	}
	if i >= len(s) || s[i] != ' ' {
		return 0
	}
	sp := leadingSpaces(s[i:])
	if sp > 4 || i+sp == len(s) { // five spaces: indented code after a one-space marker
		sp = 1
	}
	return i + sp
}

// opens parses the container prefix of line and a fence opening after it.
func opens(line string) (*fence, bool) {
	f := &fence{}
	rest := line
	for {
		sp := leadingSpaces(rest)
		if sp > 3 {
			break
		}
		t := rest[sp:]
		if f.indent == 0 && strings.HasPrefix(t, ">") {
			f.quotes++
			rest = strings.TrimPrefix(t[1:], " ")
			continue
		}
		if w := listMarker(t); w > 0 {
			f.indent += sp + w
			rest = t[w:]
			continue
		}
		break
	}
	sp := leadingSpaces(rest)
	if sp > 3 {
		return nil, false
	}
	t := rest[sp:]
	if t == "" || (t[0] != '`' && t[0] != '~') {
		return nil, false
	}
	n := len(t) - len(strings.TrimLeft(t, string(t[0])))
	if n < 3 || (t[0] == '`' && strings.Contains(t[n:], "`")) {
		return nil, false
	}
	f.char, f.n = t[0], n
	return f, true
}

// stripContainer removes the container prefix of the fence from a line inside
// it; in is false when the line is not part of the container any more.
func stripContainer(line string, f *fence) (rest string, in bool) {
	rest = line
	for i := 0; i < f.quotes; i++ {
		sp := leadingSpaces(rest)
		if sp > 3 || !strings.HasPrefix(rest[sp:], ">") {
			return "", false
		}
		rest = strings.TrimPrefix(rest[sp+1:], " ")
	}
	if f.indent > 0 && strings.TrimSpace(rest) != "" {
		if leadingSpaces(rest) < f.indent {
			return "", false
		}
		rest = rest[f.indent:]
	}
	return rest, true
}

func closes(rest string, f *fence) bool {
	sp := leadingSpaces(rest)
	if sp > 3 {
		return false
	}
	t := strings.TrimRight(rest[sp:], " \t")
	return len(t) >= f.n && strings.Trim(t, string(f.char)) == ""
}

// eachLine calls fn for every line of text (ls..le excludes the newline) with
// whether the line belongs to a fenced code block, until fn returns false.
func eachLine(text string, fn func(ls, le int, fenced bool) bool) {
	fenced, _ := CodeLines(text)
	ls := 0
	for _, f := range fenced {
		le := strings.IndexByte(text[ls:], '\n')
		next := len(text) + 1
		if le < 0 {
			le = len(text)
		} else {
			le += ls
			next = le + 1
		}
		if ls < len(text) || le > ls { // a final empty line after "\n" is not a line
			if !fn(ls, le, f) {
				return
			}
		}
		ls = next
	}
}
