// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"io"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

const hintMarker = "Are you an AI?"

func TestAgentHintOnRootHelpOnly(t *testing.T) {
	for _, args := range [][]string{{"--help"}, {"help"}, {}} {
		_, out, _ := run(args...)
		if !strings.Contains(out, hintMarker) {
			t.Errorf("root help %v lacks the agent hint:\n%s", args, out)
		}
		for _, url := range []string{
			"https://raw.githubusercontent.com/wstein/chartscribe/main/docs/agent-guide.md",
			"https://raw.githubusercontent.com/wstein/chartscribe/main/llms.txt",
		} {
			if !strings.Contains(out, url) {
				t.Errorf("root help %v lacks %s", args, url)
			}
		}
	}
	var walk func(c *cobra.Command, path []string)
	walk = func(c *cobra.Command, path []string) {
		for _, sub := range c.Commands() {
			p := append(append([]string{}, path...), sub.Name())
			_, out, _ := run(append(append([]string{}, p...), "--help")...)
			if out == "" {
				t.Errorf("%v --help printed nothing", p)
			}
			if strings.Contains(out, hintMarker) {
				t.Errorf("subcommand help %v carries the agent hint", p)
			}
			walk(sub, p)
		}
	}
	walk(newRoot("test", io.Discard, io.Discard), nil)
}
