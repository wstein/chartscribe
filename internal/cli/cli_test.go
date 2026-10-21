// SPDX-License-Identifier: EUPL-1.2

package cli

import (
	"bytes"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		wantCode int
		wantOut  string
		wantErr  string
	}{
		{"version", []string{"version"}, 0, "cscribe dev\n", ""},
		{"no arguments", nil, 2, "", "usage: cscribe version\n"},
		{"unknown command", []string{"nope"}, 2, "", "cscribe: unknown command \"nope\"\nusage: cscribe version\n"},
		{"version with extra argument", []string{"version", "x"}, 2, "", "cscribe: version takes no arguments\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			code := Run(tt.args, &out, &errOut)
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
			if out.String() != tt.wantOut {
				t.Errorf("stdout = %q, want %q", out.String(), tt.wantOut)
			}
			if errOut.String() != tt.wantErr {
				t.Errorf("stderr = %q, want %q", errOut.String(), tt.wantErr)
			}
		})
	}
}
