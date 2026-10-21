// SPDX-License-Identifier: EUPL-1.2

package validate

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	mdLink = regexp.MustCompile(`\]\(([^)\s]+)\)`)
	scheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)
)

// readme checks the Markdown links of README.md at the collection root: a
// link must point to an existing path inside the collection. External schemes
// and #anchors are ignored.
func (c *checker) readme(root string) error {
	path := filepath.Join(root, "README.md")
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) || (err == nil && !info.Mode().IsRegular()) {
		return nil
	}
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	rootReal := resolve(rootAbs)
	for _, m := range mdLink.FindAllStringSubmatch(string(data), -1) {
		target := m[1]
		if scheme.MatchString(target) || strings.HasPrefix(target, "#") {
			continue
		}
		rel, _, _ := strings.Cut(target, "#")
		full := rel
		if !filepath.IsAbs(rel) {
			full = filepath.Join(rootAbs, rel)
		}
		resolved := resolve(full)
		inside := resolved == rootReal || strings.HasPrefix(resolved, rootReal+string(filepath.Separator))
		switch {
		case !inside:
			c.add("readme-link", "README.md", fmt.Sprintf("link %q points outside the collection", target))
		default:
			if _, err := os.Stat(resolved); err != nil {
				c.add("readme-link", "README.md", fmt.Sprintf("broken link %q", target))
			}
		}
	}
	return nil
}

// resolve makes path absolute and clean and follows symlinks as far as the path exists.
func resolve(path string) string {
	path = filepath.Clean(path)
	if real, err := filepath.EvalSymlinks(path); err == nil {
		return real
	}
	parent := filepath.Dir(path)
	if parent == path {
		return path
	}
	return filepath.Join(resolve(parent), filepath.Base(path))
}
