// SPDX-License-Identifier: EUPL-1.2

package fix

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	mdLink = regexp.MustCompile(`\]\(([^)\s]+)\)`)
	scheme = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*:`)
)

// foldPath finds the one existing path below root that equals rel when case is ignored, one
// component at a time. It reports false when a component is missing or matches several entries.
func foldPath(root, rel string) (string, bool) {
	cur := root
	var parts []string
	for _, comp := range strings.Split(rel, "/") {
		if comp == "" || comp == ".." {
			return "", false
		}
		if comp == "." {
			parts = append(parts, comp)
			continue
		}
		if _, err := os.Lstat(filepath.Join(cur, comp)); err == nil {
			parts = append(parts, comp)
			cur = filepath.Join(cur, comp)
			continue
		}
		entries, err := os.ReadDir(cur)
		if err != nil {
			return "", false
		}
		var match []string
		for _, e := range entries {
			if strings.EqualFold(e.Name(), comp) {
				match = append(match, e.Name())
			}
		}
		if len(match) != 1 {
			return "", false
		}
		parts = append(parts, match[0])
		cur = filepath.Join(cur, match[0])
	}
	return strings.Join(parts, "/"), true
}

// readmeEdits rewrites, per README line, each broken relative link whose target exists after case-folding.
func readmeEdits(root, text string) []lineEdit {
	var edits []lineEdit
	for i, line := range strings.Split(text, "\n") {
		matches := mdLink.FindAllStringSubmatchIndex(line, -1)
		changed := line
		for j := len(matches) - 1; j >= 0; j-- {
			from, to := matches[j][2], matches[j][3]
			target := line[from:to]
			if scheme.MatchString(target) || strings.HasPrefix(target, "#") || filepath.IsAbs(target) {
				continue
			}
			rel, anchor, _ := strings.Cut(target, "#")
			if rel == "" {
				continue
			}
			if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
				continue
			}
			fixed, ok := foldPath(root, rel)
			if !ok || fixed == rel {
				continue
			}
			if strings.Contains(target, "#") {
				fixed += "#" + anchor
			}
			changed = changed[:from] + fixed + changed[to:]
		}
		if changed != line {
			edits = append(edits, lineEdit{file: "README.md", start: i, end: i + 1, repl: []string{changed}, rule: "readme-link"})
		}
	}
	return edits
}
