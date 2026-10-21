// SPDX-License-Identifier: EUPL-1.2

package initcmd

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func actions(es []Entry) string {
	var out []string
	for _, e := range es {
		out = append(out, e.Path+"="+string(e.Action))
	}
	return strings.Join(out, " ")
}

func snapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	m := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		if d.IsDir() {
			m[rel+"/"] = ""
			return nil
		}
		b, err := os.ReadFile(p)
		m[rel] = string(b)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func TestPlanWritesNothing(t *testing.T) {
	root := t.TempDir()
	files := Files(nil, "demo", "")
	es, err := Plan(root, files, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(es) != len(files) {
		t.Fatalf("entries = %d, want %d", len(es), len(files))
	}
	for _, e := range es {
		if e.Action != Write {
			t.Errorf("%s: %s, want write", e.Path, e.Action)
		}
	}
	if got := snapshot(t, root); len(got) != 1 {
		t.Errorf("plan wrote %v", got)
	}
}

func TestApplyThenRerunPlansNothing(t *testing.T) {
	root := t.TempDir()
	files := Files(nil, "demo", "")
	es, _ := Plan(root, files, nil)
	if err := Apply(root, files, es); err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		got, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(f.Path)))
		if err != nil || string(got) != f.Content {
			t.Errorf("%s: %v %q", f.Path, err, got)
		}
	}
	before := snapshot(t, root)
	es, err := Plan(root, files, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range es {
		if e.Action != Exists {
			t.Errorf("%s: %s, want exists", e.Path, e.Action)
		}
	}
	if err := Apply(root, files, es); err != nil {
		t.Fatal(err)
	}
	if after := snapshot(t, root); len(after) != len(before) {
		t.Error("rerun changed the tree")
	}
}

func TestExistingFileIsKeptWithoutOverwrite(t *testing.T) {
	root := t.TempDir()
	files := Files([]Tool{ZK}, "demo", "")
	mine := filepath.Join(root, ".zk", "config.toml")
	if err := os.MkdirAll(filepath.Dir(mine), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mine, []byte("# mine\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	es, err := Plan(root, files, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := actions(es); !strings.Contains(got, ".zk/config.toml=differs (skipped)") {
		t.Errorf("plan = %s", got)
	}
	if err := Apply(root, files, es); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(mine); string(b) != "# mine\n" {
		t.Errorf("existing file changed: %q", b)
	}
	if _, err := os.Stat(filepath.Join(root, ".cscribe.toml")); err != nil {
		t.Errorf("missing files were not written: %v", err)
	}
}

func TestOverwriteReplacesOnlyTheNamedFile(t *testing.T) {
	root := t.TempDir()
	files := Files([]Tool{ZK}, "demo", "")
	for _, p := range []string{".zk/config.toml", ".zk/.gitignore"} {
		full := filepath.Join(root, filepath.FromSlash(p))
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte("mine\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	es, err := Plan(root, files, []string{"./.zk/config.toml"})
	if err != nil {
		t.Fatal(err)
	}
	if got := actions(es); !strings.Contains(got, ".zk/config.toml=overwrite") || !strings.Contains(got, ".zk/.gitignore=differs (skipped)") {
		t.Errorf("plan = %s", got)
	}
	if err := Apply(root, files, es); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(filepath.Join(root, ".zk", "config.toml")); string(b) != zkConfig {
		t.Errorf("config not replaced: %q", b)
	}
	if b, _ := os.ReadFile(filepath.Join(root, ".zk", ".gitignore")); string(b) != "mine\n" {
		t.Errorf("other file changed: %q", b)
	}
}

func TestOverwriteAll(t *testing.T) {
	root := t.TempDir()
	files := Files([]Tool{ZK}, "demo", "")
	es, _ := Plan(root, files, nil)
	_ = Apply(root, files, es)
	_ = os.WriteFile(filepath.Join(root, ".zk", ".gitignore"), []byte("mine\n"), 0o644)
	es, err := Plan(root, files, []string{"all"})
	if err != nil {
		t.Fatal(err)
	}
	got := actions(es)
	if !strings.Contains(got, ".zk/.gitignore=overwrite") || !strings.Contains(got, ".zk/config.toml=exists (skipped)") {
		t.Errorf("plan = %s (identical files must stay untouched)", got)
	}
}

func TestOverwriteMustNameAFileOfTheSet(t *testing.T) {
	files := Files([]Tool{ZK}, "demo", "")
	for _, p := range []string{"notes/Idea.md", ".obsidian/app.json", "../x", "/etc/passwd", ""} {
		_, err := Plan(t.TempDir(), files, []string{p})
		if !errors.Is(err, ErrUnknownOverwrite) {
			t.Errorf("overwrite %q: err = %v", p, err)
		}
	}
}

func TestRefusesSymlinkAndDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlinks")
	}
	files := Files([]Tool{ZK}, "demo", "")
	t.Run("target symlink", func(t *testing.T) {
		root, outside := t.TempDir(), filepath.Join(t.TempDir(), "victim")
		_ = os.WriteFile(outside, []byte("v"), 0o644)
		if err := os.Symlink(outside, filepath.Join(root, ".cscribe.toml")); err != nil {
			t.Fatal(err)
		}
		if _, err := Plan(root, files, []string{"all"}); !errors.Is(err, ErrUnsafe) {
			t.Errorf("err = %v", err)
		}
		if b, _ := os.ReadFile(outside); string(b) != "v" {
			t.Error("victim changed")
		}
	})
	t.Run("directory symlink", func(t *testing.T) {
		root, outside := t.TempDir(), t.TempDir()
		if err := os.Symlink(outside, filepath.Join(root, ".zk")); err != nil {
			t.Fatal(err)
		}
		if _, err := Plan(root, files, nil); !errors.Is(err, ErrUnsafe) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("target is a directory", func(t *testing.T) {
		root := t.TempDir()
		_ = os.Mkdir(filepath.Join(root, ".cscribe.toml"), 0o755)
		if _, err := Plan(root, files, []string{"all"}); !errors.Is(err, ErrUnsafe) {
			t.Errorf("err = %v", err)
		}
	})
	t.Run("apply refuses a link planted after the plan", func(t *testing.T) {
		root, outside := t.TempDir(), filepath.Join(t.TempDir(), "victim")
		es, _ := Plan(root, files, nil)
		_ = os.Symlink(outside, filepath.Join(root, ".cscribe.toml"))
		if err := Apply(root, files, es); err == nil {
			t.Error("Apply wrote through a symlink")
		}
		if _, err := os.Stat(outside); err == nil {
			t.Error("victim created")
		}
	})
}

func TestApplyReportsWriteFailure(t *testing.T) {
	if os.Geteuid() == 0 || runtime.GOOS == "windows" {
		t.Skip("needs a non-root user to make a directory read-only")
	}
	root := t.TempDir()
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(root, 0o700)
	files := Files(nil, "demo", "")
	es, err := Plan(root, files, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(root, files, es); err == nil {
		t.Error("want an error from a read-only root")
	}
}

func TestApplyNeverTruncatesAFileThatAppearedAfterThePlan(t *testing.T) {
	root := t.TempDir()
	files := Files([]Tool{ZK}, "demo", "")
	es, _ := Plan(root, files, nil)
	mine := filepath.Join(root, ".cscribe.toml")
	_ = os.WriteFile(mine, []byte("mine\n"), 0o644)
	if err := Apply(root, files, es); err == nil {
		t.Error("Apply overwrote a file created after the plan")
	}
	if b, _ := os.ReadFile(mine); string(b) != "mine\n" {
		t.Errorf("file changed: %q", b)
	}
}

func TestOverwriteKeepsTheFileMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("modes")
	}
	root := t.TempDir()
	files := Files([]Tool{ZK}, "demo", "")
	full := filepath.Join(root, ".cscribe.toml")
	_ = os.WriteFile(full, []byte("mine\n"), 0o600)
	_ = os.Chmod(full, 0o600)
	es, _ := Plan(root, files, []string{".cscribe.toml"})
	if err := Apply(root, files, es); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(full)
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600", info.Mode().Perm())
	}
	if b, _ := os.ReadFile(full); string(b) != cscribeToml {
		t.Errorf("not replaced: %q", b)
	}
}
