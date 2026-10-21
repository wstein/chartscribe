// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wstein/chartscribe/internal/collection"
	"github.com/wstein/chartscribe/internal/initcmd"
)

type initDocument struct {
	Version int             `json:"version"`
	Command string          `json:"command"`
	Files   []initcmd.Entry `json:"files"`
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
			files := initcmd.Files(sel, filepath.Base(root))
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
			if err := printInit(cmd, opts, entries, apply); err != nil {
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

func printInit(cmd *cobra.Command, opts *globalOptions, entries []initcmd.Entry, apply bool) error {
	if opts.json {
		return writeJSON(cmd.OutOrStdout(), initDocument{Version: 1, Command: "init", Files: entries})
	}
	var b strings.Builder
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
	_, err := cmd.OutOrStdout().Write([]byte(b.String()))
	return err
}
