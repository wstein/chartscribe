// SPDX-License-Identifier: EUPL-1.2

package note

import (
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
	legacySource  = regexp.MustCompile(`^(.+), lines ([0-9]+)-([0-9]+)$`)
	kindPrefix    = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9-]*):(?: (.*))?$`)
	decisionRest  = regexp.MustCompile(`^(.+), ([0-9]{4}-[0-9]{2}-[0-9]{2})$`)
)

// Source is one parsed `Source:` line.
type Source struct {
	Kind      string // KindFile, KindDecision, KindURL, KindMeasurement or KindUnknown
	Raw       string // the text after "Source: "
	Path      string // file and measurement
	From, To  int    // file: the line range A-B
	Key       string // measurement: the optional key
	Text      string // decision: the free text
	Date      string // decision: YYYY-MM-DD
	URL       string // url
	KindName  string // unknown: the prefix as written
	Malformed bool   // a known kind whose form does not parse
}

// Valid reports whether the line is a well-formed source of a known kind.
func (s Source) Valid() bool { return !s.Malformed && s.Kind != KindUnknown }

// Verifiable reports whether the source cites a file below the source root.
func (s Source) Verifiable() bool {
	return s.Valid() && (s.Kind == KindFile || s.Kind == KindMeasurement)
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

func parseSource(raw string) (Source, bool) {
	if m := legacySource.FindStringSubmatch(raw); m != nil {
		return Source{Kind: KindFile, Raw: raw, Path: strings.TrimSpace(m[1]), From: atoi(m[2]), To: atoi(m[3])}, true
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
