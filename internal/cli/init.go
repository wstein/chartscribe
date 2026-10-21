// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wstein/chartscribe/internal/collection"
	"github.com/wstein/chartscribe/internal/initcmd"
)

type initDocument struct {
	Version         int             `json:"version"`
	Command         string          `json:"command"`
	Root            string          `json:"root"`
	WorkflowVersion string          `json:"workflowVersion,omitempty"`
	Files           []initcmd.Entry `json:"files"`
}

const workflowPath = ".github/workflows/pages.yml"

// buildVersion is the version of the running binary that the Quartz workflow
// installs; tests replace it.
var buildVersion = func() string {
	info, _ := debug.ReadBuildInfo()
	if info == nil {
		return pickVersion(Version, "", "")
	}
	rev := ""
	for _, s := range info.Settings {
		if s.Key == "vcs.revision" {
			rev = s.Value
		}
	}
	return pickVersion(Version, info.Main.Version, rev)
}

// pickVersion prefers a release version set by ldflags, then the module
// version, then the VCS revision; "" means this build knows none.
func pickVersion(ldflags, module, revision string) string {
	for _, v := range []string{ldflags, module} {
		if strings.HasSuffix(v, "+dirty") {
			return "" // a dirty tree has no version go install can fetch
		}
		if v != "" && v != "dev" && v != "(devel)" {
			return v
		}
	}
	return revision
}

func initCommand(opts *globalOptions) *cobra.Command {
	var tools, overwrite []string
	var apply bool
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Write the configuration for zk, Obsidian and Quartz",
		Long: `Print the plan for the tool configuration files, one line per file: write,
exists (skipped), differs (skipped) or overwrite. Nothing is written without --apply.

--apply writes the files that do not exist. An existing file is never touched
unless --overwrite names it (a path relative to the root, or "all"). --tool
selects zk, obsidian or quartz (repeatable or comma-separated; default all);
.cscribe.toml is written under every tool. init writes files only: it starts no
tool and installs no plugin. The root is --root, else the nearest directory with
a notes/ folder, else the working directory.
Exit status: 0 done, 2 usage error, 3 environment error.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			var sel []initcmd.Tool
			for _, name := range tools {
				t := initcmd.Tool(name)
				if t != initcmd.ZK && t != initcmd.Obsidian && t != initcmd.Quartz {
					return failure(cmd, opts, exitUsage, "usage", fmt.Errorf("unknown tool %q (known: zk, obsidian, quartz)", name))
				}
				sel = append(sel, t)
			}
			root, err := initRoot(opts.root)
			if err != nil {
				kind := "io"
				if errors.Is(err, collection.ErrNoCollection) {
					kind = "no-collection"
				}
				return failure(cmd, opts, exitEnv, kind, err)
			}
			files := initcmd.Files(sel, filepath.Base(root), buildVersion())
			entries, err := initcmd.Plan(root, files, overwrite)
			if errors.Is(err, initcmd.ErrUnknownOverwrite) {
				return failure(cmd, opts, exitUsage, "usage", err)
			}
			if err != nil {
				return failure(cmd, opts, exitEnv, "io", err)
			}
			if apply {
				if err := initcmd.Apply(root, files, entries); err != nil {
					return failure(cmd, opts, exitEnv, "io", err)
				}
			}
			if err := printInit(cmd, opts, root, files, entries, apply); err != nil {
				return runtimeFailure{err}
			}
			return nil
		},
	}
	cmd.Flags().StringSliceVar(&tools, "tool", nil, "Tool to configure: zk, obsidian or quartz (repeatable; default all)")
	cmd.Flags().BoolVar(&apply, "apply", false, "Write the files (default: print the plan only)")
	cmd.Flags().StringArrayVar(&overwrite, "overwrite", nil, "Replace this existing file (path relative to the root) or all")
	return cmd
}

// initRoot is --root (which must be a directory), else the nearest collection
// root, else the working directory: init may start in an empty directory.
func initRoot(explicit string) (string, error) {
	if explicit != "" {
		abs, err := filepath.Abs(explicit)
		if err != nil {
			return "", err
		}
		info, err := os.Stat(abs)
		if err != nil || !info.IsDir() {
			return "", fmt.Errorf("%w: %s is not a directory", collection.ErrNoCollection, explicit)
		}
		return abs, nil
	}
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	root, err := collection.FindRoot("", cwd)
	if errors.Is(err, collection.ErrNoCollection) {
		return cwd, nil
	}
	return root, err
}

func printInit(cmd *cobra.Command, opts *globalOptions, root string, files []initcmd.File, entries []initcmd.Entry, apply bool) error {
	pinned := ""
	for _, f := range files {
		if f.Path == workflowPath {
			pinned = workflowPin(f.Content)
		}
	}
	if opts.json {
		return writeJSON(cmd.OutOrStdout(), initDocument{Version: 1, Command: "init", Root: root, WorkflowVersion: pinned, Files: entries})
	}
	var b strings.Builder
	fmt.Fprintf(&b, "init: root %s\n", root)
	todo, skipped := 0, 0
	for _, e := range entries {
		fmt.Fprintf(&b, "%-18s %s\n", e.Action, e.Path)
		if e.Action == initcmd.Write || e.Action == initcmd.Overwrite {
			todo++
		} else {
			skipped++
		}
	}
	switch {
	case todo == 0:
		fmt.Fprintf(&b, "init: nothing to write (%d skipped)\n", skipped)
	case apply:
		fmt.Fprintf(&b, "init: wrote %d files (%d skipped)\n", todo, skipped)
	default:
		fmt.Fprintf(&b, "init: plan only, %d files to write (%d skipped); rerun with --apply\n", todo, skipped)
	}
	switch pinned {
	case "":
	case initcmd.VersionPlaceholder:
		fmt.Fprintf(&b, "init: the workflow carries %s (this build has no version); set it before enabling Pages\n", pinned)
	default:
		fmt.Fprintf(&b, "init: the workflow installs cscribe %s\n", pinned)
	}
	_, err := cmd.OutOrStdout().Write([]byte(b.String()))
	return err
}

// workflowPin reads the version out of the generated workflow.
func workflowPin(workflow string) string {
	_, rest, _ := strings.Cut(workflow, "CSCRIBE_VERSION: ")
	v, _, _ := strings.Cut(rest, "\n")
	return v
}
