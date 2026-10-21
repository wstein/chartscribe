// SPDX-License-Identifier: EUPL-1.2

package validate

import (
	"fmt"
	"strings"

	"github.com/wstein/chartscribe/internal/note"
)

func (c *checker) known() map[string]bool {
	known := map[string]bool{}
	for _, n := range c.parsed {
		known[strings.ToLower(n.title)] = true
		for _, a := range n.aliases {
			known[strings.ToLower(a)] = true
		}
	}
	return known
}

func (c *checker) wikilinks() {
	known := c.known()
	for _, n := range c.parsed {
		for _, target := range note.WikiTargets(n.body) {
			if !known[note.LinkName(target)] {
				c.add("wikilink-unresolved", n.name, fmt.Sprintf("wikilink [[%s]] does not resolve to a note title or alias", target))
			}
		}
	}
}
