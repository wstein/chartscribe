// SPDX-License-Identifier: EUPL-1.2

package fix

import (
	"sort"
	"strings"

	"github.com/wstein/chartscribe/internal/note"
	"github.com/wstein/chartscribe/internal/validate"
)

// categoryOf returns the category value of a note when it has exactly one category/ tag.
func categoryOf(tags []string) (string, bool) {
	var found []string
	for _, t := range tags {
		if rest, ok := strings.CutPrefix(t, "category/"); ok {
			found = append(found, rest)
		}
	}
	if len(found) == 1 {
		return found[0], true
	}
	return "", false
}

// mapEdits plans one appended link line per map-unlisted note whose category has exactly one map.
// texts holds the current note texts keyed by file name; positions refer to orig.
func mapEdits(findings []validate.Finding, texts, orig map[string]string) []lineEdit {
	type noteInfo struct {
		isMap bool
		cat   string
		hasC  bool
	}
	infos := map[string]noteInfo{}
	for name, text := range texts {
		fm, _, err := note.Parse(text)
		if err != nil {
			continue
		}
		tv, _ := fm.Get("tags")
		var info noteInfo
		if tv.IsList && !tv.NonString {
			info.cat, info.hasC = categoryOf(tv.List)
			info.isMap = contains(tv.List, "type/map")
		}
		infos[name] = info
	}
	var unlisted []string
	badTitle := map[string]bool{}
	for _, f := range findings {
		if f.Rule == "map-unlisted" {
			unlisted = append(unlisted, f.Note)
		}
		if f.Rule == "title-char" {
			badTitle[f.Note] = true
		}
	}
	sort.Strings(unlisted)
	var edits []lineEdit
	for _, name := range unlisted {
		info, ok := infos[name]
		if !ok || info.isMap || !info.hasC || badTitle[name] {
			continue
		}
		var maps []string
		for other, oi := range infos {
			if oi.isMap && oi.hasC && oi.cat == info.cat {
				maps = append(maps, other)
			}
		}
		if len(maps) != 1 {
			continue
		}
		file := "notes/" + maps[0]
		lines := strings.Split(orig[maps[0]], "\n")
		at := len(lines)
		for at > 0 && lines[at-1] == "" {
			at--
		}
		edits = append(edits, lineEdit{file: file, start: at, end: at,
			repl: []string{"- [[" + strings.TrimSuffix(name, ".md") + "]]"}, rule: "map-unlisted"})
	}
	return edits
}
