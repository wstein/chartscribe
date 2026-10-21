// SPDX-License-Identifier: EUPL-1.2

package validate

import (
	"fmt"
	"strings"

	"github.com/wstein/chartscribe/internal/note"
)

// catKey is a map or note category; ok is false when there is not exactly one category tag.
type catKey struct {
	value string
	ok    bool
}

func (k catKey) String() string {
	if !k.ok {
		return "None"
	}
	return fmt.Sprintf("%q", k.value)
}

func (n parsedNote) categoryKey() catKey {
	v, ok := n.category()
	return catKey{v, ok}
}

func (c *checker) maps() {
	// canonical resolves a link name to a note title; later notes win, as in the reference.
	canonical := map[string]string{}
	for _, n := range c.parsed {
		canonical[strings.ToLower(n.title)] = n.title
		for _, a := range n.aliases {
			canonical[strings.ToLower(a)] = n.title
		}
	}
	type listing struct {
		mapName string
		cat     catKey
	}
	var catOrder []catKey
	mapFiles := map[catKey][]string{}
	listed := map[string][]listing{}
	for _, n := range c.parsed {
		if !n.isMap {
			continue
		}
		cat := n.categoryKey()
		if _, ok := mapFiles[cat]; !ok {
			catOrder = append(catOrder, cat)
		}
		mapFiles[cat] = append(mapFiles[cat], n.name)
		for _, target := range note.WikiTargets(n.body) {
			title, ok := canonical[note.LinkName(target)]
			if !ok {
				continue
			}
			// Several links from one map to the same note count once.
			dup := false
			for _, l := range listed[title] {
				if l.mapName == n.name {
					dup = true
				}
			}
			if !dup {
				listed[title] = append(listed[title], listing{n.name, cat})
			}
		}
	}
	for _, cat := range catOrder {
		if files := mapFiles[cat]; len(files) > 1 {
			c.add("map-duplicate", "", fmt.Sprintf("more than one map for category %s: %s", cat, strings.Join(files, ", ")))
		}
	}
	for _, n := range c.parsed {
		if n.isMap {
			continue
		}
		maps := listed[n.title]
		cat := n.categoryKey()
		switch {
		case len(maps) == 0:
			c.add("map-unlisted", n.name, "not listed in any map")
		case len(maps) > 1:
			var cats []string
			for _, m := range maps {
				cats = append(cats, m.cat.String())
			}
			c.add("map-multiple", n.name, fmt.Sprintf("listed in more than one map (%s)", strings.Join(cats, ", ")))
		case maps[0].cat != cat:
			c.add("map-category", n.name, fmt.Sprintf("category %s differs from its map's category %s", cat, maps[0].cat))
		case len(mapFiles[maps[0].cat]) == 0:
			c.add("category-no-map", "", fmt.Sprintf("category %s: no map", maps[0].cat))
		}
	}
	for _, n := range c.parsed {
		if cat := n.categoryKey(); !n.isMap && cat.ok && len(mapFiles[cat]) == 0 {
			c.add("category-no-map", "", fmt.Sprintf("category %s: no map", cat.value))
		}
	}
}
