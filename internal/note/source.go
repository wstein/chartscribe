// SPDX-License-Identifier: EUPL-1.2

package note

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Source kinds. A legacy `Source: <path>, lines A-B` line is KindFile.
const (
	KindFile        = "file"
	KindDecision    = "decision"
	KindURL         = "url"
	KindMeasurement = "measurement"
	KindUnknown     = "unknown"
)

// Kinds lists the typed kind prefixes (the file kind has none).
var Kinds = []string{KindDecision, KindMeasurement, KindURL}

var (
	anySourceLine = regexp.MustCompile(`(?m)^Source: .*$`)
	pinnedSource  = regexp.MustCompile(`^(.+), lines ([0-9]+)-([0-9]+) @(.*)$`)
	pinHex        = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
	legacySource  = regexp.MustCompile(`^(.+), lines ([0-9]+)-([0-9]+)$`)
	kindPrefix    = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9-]*):(?: (.*))?$`)
	decisionRest  = regexp.MustCompile(`^(.+), ([0-9]{4}-[0-9]{2}-[0-9]{2})$`)
	decisionAlias = regexp.MustCompile(`^decision ([0-9]{4}-[0-9]{2}-[0-9]{2}) (.+)$`)
	measureAlias  = regexp.MustCompile(`^measurement (.+) ([0-9]{4}-[0-9]{2}-[0-9]{2})$`)
	bareURL       = regexp.MustCompile(`^https?://`)
)

// Source is one parsed `Source:` line.
type Source struct {
	Kind      string // KindFile, KindDecision, KindURL, KindMeasurement or KindUnknown
	Raw       string // the text after "Source: "
	Path      string // file and measurement
	From, To  int    // file: the line range A-B
	Pin       string // file: the optional commit the range was cited at (7 to 40 lower-case hex digits)
	Key       string // measurement: the optional key
	Text      string // decision: the free text
	Date      string // decision: YYYY-MM-DD
	URL       string // url
	KindName  string // unknown: the prefix as written
	Malformed bool   // a known kind whose form does not parse
	Why       string // malformed: a specific reason, when there is one
}

// Valid reports whether the line is a well-formed source of a known kind.
func (s Source) Valid() bool { return !s.Malformed && s.Kind != KindUnknown }

// Verifiable reports whether the source cites a file below the source root.
// A file source always does; a measurement does only when its text looks like a
// relative path (see PathLike), otherwise it is free text such as a command.
func (s Source) Verifiable() bool {
	return s.Valid() && (s.Kind == KindFile || (s.Kind == KindMeasurement && PathLike(s.Path)))
}

// PathLike reports whether text is a single word that contains a "/" or a "."
// and does not start with "-": the shape of a relative path, not of a command.
func PathLike(text string) bool {
	return text != "" && !strings.ContainsAny(text, " \t") && !strings.HasPrefix(text, "-") && strings.ContainsAny(text, "/.")
}

// mainSourceLine is the line shape that earlier versions required: the legacy
// form anywhere in the body (code fences included).
var mainSourceLine = regexp.MustCompile(`(?m)^Source: .+, lines [0-9]+-[0-9]+$`)

// HasLegacySource reports whether body has a line `Source: <text>, lines A-B`
// anywhere (the rule of earlier versions, unchanged), not counting a bare URL
// with a range, which is a malformed url source.
func HasLegacySource(body string) bool {
	for _, line := range mainSourceLine.FindAllString(body, -1) {
		if !bareURL.MatchString(strings.TrimPrefix(line, "Source: ")) {
			return true
		}
	}
	return false
}

// ParseSources returns every line `Source: ...` of body that has a recognised
// shape: the legacy `<text>, lines A-B`, or `<kind>: ...` with a kind prefix.
// Other `Source:` lines are not sources (as before, they do not count).
// The legacy form wins over a kind prefix.
func ParseSources(body string) []Source {
	var out []Source
	for _, line := range anySourceLine.FindAllString(body, -1) {
		if s, ok := parseSource(strings.TrimPrefix(line, "Source: ")); ok {
			out = append(out, s)
		}
	}
	return out
}

// ParseSourceLine parses the text after "Source: " of one line.
func ParseSourceLine(raw string) (Source, bool) { return parseSource(raw) }

// FormatFile renders a file source line (with "Source: ") and, when pin is not empty, its pin.
func FormatFile(path string, from, to int, pin string) string {
	line := fmt.Sprintf("Source: %s, lines %d-%d", path, from, to)
	if pin != "" {
		line += " @" + pin
	}
	return line
}

func parseSource(raw string) (Source, bool) {
	// A bare URL is a url source; it comes first so that "<url>, lines A-B" is
	// reported as a URL with a range, not accepted as a file path.
	if bareURL.MatchString(raw) {
		s := Source{Kind: KindURL, Raw: raw}
		switch {
		case legacySource.MatchString(raw):
			s.Malformed, s.Why = true, "url sources take no line range"
		case validURL(raw):
			s.URL = raw
		default:
			s.Malformed = true
		}
		return s, true
	}
	if m := legacySource.FindStringSubmatch(raw); m != nil {
		return Source{Kind: KindFile, Raw: raw, Path: strings.TrimSpace(m[1]), From: atoi(m[2]), To: atoi(m[3])}, true
	}
	if m := pinnedSource.FindStringSubmatch(raw); m != nil {
		s := Source{Kind: KindFile, Raw: raw, Path: strings.TrimSpace(m[1]), From: atoi(m[2]), To: atoi(m[3]), Pin: m[4]}
		switch {
		case !pinHex.MatchString(m[4]):
			s.Pin, s.Malformed, s.Why = "", true, "the pin must be 7 to 40 lower-case hex digits"
		case s.From < 1 || s.From > s.To:
			s.Malformed, s.Why = true, "a pinned range must satisfy 1 <= A <= B"
		}
		return s, true
	}
	// Aliases without a colon: "decision <date> <who>" and "measurement <what> <date>".
	if m := decisionAlias.FindStringSubmatch(raw); m != nil {
		s := Source{Kind: KindDecision, Raw: raw, Date: m[1], Text: strings.TrimSpace(m[2])}
		if !validDate(m[1]) {
			s.Malformed, s.Date, s.Text = true, "", ""
		}
		return s, true
	}
	if m := measureAlias.FindStringSubmatch(raw); m != nil {
		s := Source{Kind: KindMeasurement, Raw: raw, Path: strings.TrimSpace(m[1]), Date: m[2]}
		if !validDate(m[2]) {
			s.Malformed, s.Path, s.Date = true, "", ""
		}
		return s, true
	}
	m := kindPrefix.FindStringSubmatch(raw)
	if m == nil {
		return Source{}, false
	}
	rest := strings.TrimSpace(m[2])
	s := Source{Kind: m[1], Raw: raw}
	switch m[1] {
	case KindDecision:
		if d := decisionRest.FindStringSubmatch(rest); d != nil && strings.TrimSpace(d[1]) != "" && validDate(d[2]) {
			s.Text, s.Date = strings.TrimSpace(d[1]), d[2]
		} else {
			s.Malformed = true
		}
	case KindURL:
		if validURL(rest) {
			s.URL = rest
		} else {
			s.Malformed = true
		}
	case KindMeasurement:
		path, key, _ := strings.Cut(rest, ", ")
		s.Path, s.Key = strings.TrimSpace(path), strings.TrimSpace(key)
		s.Malformed = s.Path == ""
	default:
		s.Kind, s.KindName = KindUnknown, m[1]
	}
	return s, true
}

func atoi(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return math.MaxInt
	}
	return n
}

func validDate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// validURL accepts http and https URLs with a host and no space or control
// character. (net/url is not used: the validator keeps no net dependency.)
func validURL(s string) bool {
	rest, ok := strings.CutPrefix(s, "https://")
	if !ok {
		rest, ok = strings.CutPrefix(s, "http://")
	}
	if !ok {
		return false
	}
	for _, r := range s {
		if r <= ' ' || r == 0x7f {
			return false
		}
	}
	host := rest
	if i := strings.IndexAny(host, "/?#"); i >= 0 {
		host = host[:i]
	}
	return host != ""
}
