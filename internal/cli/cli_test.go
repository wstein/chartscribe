// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

func run(args ...string) (int, string, string) {
	var out, errOut bytes.Buffer
	code := Run(context.Background(), "test", args, &out, &errOut)
	return code, out.String(), errOut.String()
}

func TestRun(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
		errHas   string
	}{
		{"version", []string{"version"}, 0, "cscribe test\n", ""},
		{"version with extra argument", []string{"version", "x"}, 2, "", "Error:"},
		{"unknown command", []string{"nope"}, 2, "", "unknown command"},
		{"unknown flag", []string{"--nope"}, 2, "", "unknown flag"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, out, errOut := run(tt.args...)
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d (stderr %q)", code, tt.wantCode, errOut)
			}
			if out != tt.wantOut {
				t.Errorf("stdout = %q, want %q", out, tt.wantOut)
			}
			if !strings.Contains(errOut, tt.errHas) {
				t.Errorf("stderr = %q, want it to contain %q", errOut, tt.errHas)
			}
		})
	}
}

func TestRunFreshTreePerCall(t *testing.T) {
	// Flag state from one call must not leak into the next.
	if code, _, _ := run("version", "--nope"); code != 2 {
		t.Fatalf("first call exit = %d, want 2", code)
	}
	if code, out, _ := run("version"); code != 0 || out != "cscribe test\n" {
		t.Fatalf("second call = %d %q", code, out)
	}
}

func TestRunCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var out, errOut bytes.Buffer
	if code := Run(ctx, "test", []string{"version"}, &out, &errOut); code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}
