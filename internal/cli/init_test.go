// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func setVersion(t *testing.T, v string) {
	t.Helper()
	old := buildVersion
	buildVersion = func() string { return v }
	t.Cleanup(func() { buildVersion = old })
}

func tree(t *testing.T, root string) string {
	t.Helper()
	var out []string
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			rel, _ := filepath.Rel(root, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	return strings.Join(out, "\n")
}

func TestInitPlanGolden(t *testing.T) {
	setVersion(t, "")
	root := t.TempDir()
	code, out, errOut := run("--root", root, "init")
	if code != 0 || errOut != "" {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	golden(t, "init-plan.stdout", strings.ReplaceAll(out, root, "<root>"))
	if got := tree(t, root); got != "" {
		t.Errorf("plan wrote files:\n%s", got)
	}
	code, out, _ = run("--root", root, "--json", "init")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	golden(t, "init-plan.json", strings.ReplaceAll(out, root, "<root>"))
	var doc struct {
		Version int
		Command string
		Files   []struct{ Path, Action string }
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil || doc.Version != 1 || doc.Command != "init" || len(doc.Files) != 13 {
		t.Errorf("document = %+v %v", doc, err)
	}
}

func TestInitApplyGoldenAndRerun(t *testing.T) {
	setVersion(t, "v1.2.3")
	root := t.TempDir()
	code, out, errOut := run("--root", root, "init", "--apply")
	if code != 0 || errOut != "" {
		t.Fatalf("exit %d stderr %q", code, errOut)
	}
	golden(t, "init-apply.stdout", strings.ReplaceAll(out, root, "<root>"))
	if n := len(strings.Split(tree(t, root), "\n")); n != 13 {
		t.Errorf("wrote %d files", n)
	}
	before := tree(t, root)
	for _, args := range [][]string{{"init"}, {"init", "--apply"}} {
		code, out, _ = run(append([]string{"--root", root}, args...)...)
		if code != 0 {
			t.Fatalf("%v: exit %d", args, code)
		}
		if strings.Contains(out, "\nwrite ") || strings.HasPrefix(out, "write ") || !strings.Contains(out, "nothing to write") {
			t.Errorf("%v rerun output:\n%s", args, out)
		}
	}
	golden(t, "init-rerun.stdout", strings.ReplaceAll(out, root, "<root>"))
	if tree(t, root) != before {
		t.Error("rerun changed the tree")
	}
	_, out, _ = run("--root", root, "--json", "init")
	if strings.Contains(out, `"write"`) || strings.Contains(out, "differs") {
		t.Errorf("rerun JSON plans a write: %s", out)
	}
}

func TestInitToolSelection(t *testing.T) {
	tests := []struct {
		args []string
		want string
	}{
		{[]string{"--tool", "zk"}, ".cscribe.toml\n.zk/.gitignore\n.zk/config.toml\n.zk/templates/note.md"},
		{[]string{"--tool", "quartz"}, ".cscribe.toml\n.github/workflows/pages.yml\n.quartz/index.md\n.quartz/settings.json"},
		{[]string{"--tool", "obsidian,zk"}, ".cscribe.toml\n.obsidian/.gitignore\n.obsidian/app.json\n.obsidian/core-plugins.json\n.obsidian/templates.json\n.obsidian/types.json\n.zk/.gitignore\n.zk/config.toml\n.zk/templates/note.md\ntemplates/Note.md"},
	}
	for _, tt := range tests {
		root := t.TempDir()
		args := append([]string{"--root", root, "init", "--apply"}, tt.args...)
		if code, _, errOut := run(args...); code != 0 {
			t.Fatalf("%v: %d %s", tt.args, code, errOut)
		}
		if got := tree(t, root); got != tt.want {
			t.Errorf("%v wrote:\n%s\nwant:\n%s", tt.args, got, tt.want)
		}
	}
}

func TestInitKeepsExistingAndOverwritesOnRequest(t *testing.T) {
	root := t.TempDir()
	mine := filepath.Join(root, ".zk", "config.toml")
	_ = os.MkdirAll(filepath.Dir(mine), 0o755)
	_ = os.WriteFile(mine, []byte("# mine\n"), 0o644)
	code, out, _ := run("--root", root, "init", "--tool", "zk", "--apply")
	if code != 0 || !strings.Contains(out, "differs (skipped)  .zk/config.toml") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	if b, _ := os.ReadFile(mine); string(b) != "# mine\n" {
		t.Errorf("kept file changed: %q", b)
	}
	// --overwrite without --apply only plans.
	_, out, _ = run("--root", root, "init", "--tool", "zk", "--overwrite", ".zk/config.toml")
	if !strings.Contains(out, "overwrite") {
		t.Errorf("plan lacks overwrite:\n%s", out)
	}
	if b, _ := os.ReadFile(mine); string(b) != "# mine\n" {
		t.Errorf("plan changed the file: %q", b)
	}
	code, _, _ = run("--root", root, "init", "--tool", "zk", "--apply", "--overwrite", ".zk/config.toml")
	if b, _ := os.ReadFile(mine); code != 0 || !strings.Contains(string(b), "[format.markdown]") {
		t.Errorf("exit %d, file %q", code, b)
	}
}

func TestInitErrors(t *testing.T) {
	root := t.TempDir()
	missing := filepath.Join(root, "absent")
	file := filepath.Join(root, "f")
	_ = os.WriteFile(file, nil, 0o644)
	tests := []struct {
		name string
		args []string
		code int
		kind string
	}{
		{"unknown tool", []string{"--root", root, "init", "--tool", "hugo"}, 2, "usage"},
		{"unknown overwrite", []string{"--root", root, "init", "--overwrite", "notes/x.md"}, 2, "usage"},
		{"unknown flag", []string{"--root", root, "init", "--nope"}, 2, "usage"},
		{"argument", []string{"--root", root, "init", "x"}, 2, "usage"},
		{"missing root", []string{"--root", missing, "init"}, 3, "no-collection"},
		{"root is a file", []string{"--root", file, "init"}, 3, "no-collection"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := run(tt.args...)
			if code != tt.code || out != "" || !strings.HasPrefix(errOut, "Error:") {
				t.Errorf("exit %d stdout %q stderr %q", code, out, errOut)
			}
			code, out, errOut = run(append([]string{"--json"}, tt.args...)...)
			var doc errorDocument
			if err := json.Unmarshal([]byte(out), &doc); err != nil || code != tt.code || errOut != "" ||
				doc.Command != "init" || doc.OK || doc.Error != tt.kind {
				t.Errorf("json: exit %d %q %q %v", code, out, errOut, err)
			}
		})
	}
	if _, err := os.Stat(missing); err == nil {
		t.Error("init created the missing root")
	}
}

func TestInitRefusesSymlinkAndReadOnlyRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}
	root, outside := t.TempDir(), t.TempDir()
	_ = os.Symlink(outside, filepath.Join(root, ".zk"))
	code, _, errOut := run("--root", root, "init", "--tool", "zk", "--apply")
	if code != 3 || !strings.Contains(errOut, "symlink") || tree(t, outside) != "" {
		t.Errorf("exit %d %q, outside: %q", code, errOut, tree(t, outside))
	}
	if tree(t, root) != ".zk" { // only the planted link
		t.Errorf("partial write before the refusal: %q", tree(t, root))
	}
	if os.Geteuid() == 0 {
		return
	}
	ro := t.TempDir()
	_ = os.Chmod(ro, 0o500)
	defer os.Chmod(ro, 0o700)
	if code, out, errOut := run("--root", ro, "init", "--apply"); code != 3 || out != "" || errOut == "" {
		t.Errorf("read-only: exit %d %q %q", code, out, errOut)
	}
}

func TestInitDefaultsToCwdOrNearestCollection(t *testing.T) {
	root := t.TempDir()
	_ = os.Mkdir(filepath.Join(root, "notes"), 0o755)
	sub := filepath.Join(root, "notes")
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	_ = os.Chdir(sub)
	if code, _, _ := run("init", "--tool", "zk", "--apply"); code != 0 {
		t.Fatal(code)
	}
	if _, err := os.Stat(filepath.Join(root, ".zk", "config.toml")); err != nil {
		t.Errorf("not written at the nearest collection root: %v", err)
	}
	empty := t.TempDir()
	_ = os.Chdir(empty)
	if code, _, _ := run("init", "--tool", "zk", "--apply"); code != 0 {
		t.Fatal(code)
	}
	if _, err := os.Stat(filepath.Join(empty, ".zk", "config.toml")); err != nil {
		t.Errorf("not written in the working directory: %v", err)
	}
}

func TestInitPinsTheBinaryVersion(t *testing.T) {
	read := func(root string) string {
		b, _ := os.ReadFile(filepath.Join(root, ".github", "workflows", "pages.yml"))
		return string(b)
	}
	setVersion(t, "v1.2.3")
	root := t.TempDir()
	_, out, _ := run("--root", root, "init", "--tool", "quartz", "--apply")
	if !strings.Contains(read(root), "CSCRIBE_VERSION: v1.2.3\n") || !strings.Contains(out, "the workflow installs cscribe v1.2.3") {
		t.Errorf("known version:\n%s\n%s", read(root), out)
	}
	setVersion(t, "")
	root = t.TempDir()
	_, out, _ = run("--root", root, "init", "--tool", "quartz", "--apply")
	if !strings.Contains(read(root), "CSCRIBE_VERSION: REPLACE_WITH_CSCRIBE_VERSION\n") || !strings.Contains(out, "carries REPLACE_WITH_CSCRIBE_VERSION") {
		t.Errorf("unknown version:\n%s\n%s", read(root), out)
	}
	_, out, _ = run("--root", root, "--json", "init", "--tool", "quartz")
	if !strings.Contains(out, `"workflowVersion": "REPLACE_WITH_CSCRIBE_VERSION"`) {
		t.Errorf("json: %s", out)
	}
	_, out, _ = run("--root", root, "init", "--tool", "zk")
	if strings.Contains(out, "workflow") {
		t.Errorf("zk-only output mentions the workflow:\n%s", out)
	}
}

func TestPickVersion(t *testing.T) {
	for _, tt := range []struct{ ld, mod, rev, want string }{
		{"v1.0.0", "v0.9.0", "abc", "v1.0.0"},
		{"dev", "v0.9.0", "abc", "v0.9.0"},
		{"dev", "(devel)", "abc123", "abc123"},
		{"", "", "", ""},
		{"dev", "v0.0.0-20261021120000-0123456789ab+dirty", "abc", ""},
		{"dev", "(devel)", "", ""},
	} {
		if got := pickVersion(tt.ld, tt.mod, tt.rev); got != tt.want {
			t.Errorf("%+v = %q", tt, got)
		}
	}
}

func TestInitPrintsTheResolvedRoot(t *testing.T) {
	root := t.TempDir()
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	_ = os.MkdirAll(filepath.Join(root, "notes", "sub"), 0o755)
	cwd, _ := os.Getwd()
	defer os.Chdir(cwd)
	_ = os.Chdir(filepath.Join(root, "notes", "sub"))
	_, out, _ := run("init", "--tool", "zk")
	if !strings.HasPrefix(out, "init: root "+root+"\n") {
		t.Errorf("human output:\n%s", out)
	}
	_, out, _ = run("--json", "init", "--tool", "zk")
	var doc initDocument
	if err := json.Unmarshal([]byte(out), &doc); err != nil || doc.Root != root {
		t.Errorf("json root %q (%v), want %q", doc.Root, err, root)
	}
}
