// SPDX-License-Identifier: EUPL-1.2

// Package gittest creates throw-away git repositories for tests. Commits take their identity from the
// environment of the command, and the user's git configuration is not read.
package gittest

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Repo is a git repository in a temporary directory.
type Repo struct {
	T   *testing.T
	Dir string // the real path
}

// New runs `git init` in a fresh temporary directory.
func New(t *testing.T) *Repo {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	r := &Repo{T: t, Dir: dir}
	r.Git("init", "-q")
	return r
}

// Git runs git in the repository and returns its trimmed output; a failure ends the test.
func (r *Repo) Git(args ...string) string {
	r.T.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = r.Dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.org", "GIT_COMMITTER_NAME=t",
		"GIT_COMMITTER_EMAIL=t@example.org", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_SYSTEM=/dev/null")
	out, err := cmd.CombinedOutput()
	if err != nil {
		r.T.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// Write writes a file below the repository, creating directories.
func (r *Repo) Write(name, text string) {
	r.T.Helper()
	path := filepath.Join(r.Dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		r.T.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		r.T.Fatal(err)
	}
}

// Commit stages everything and returns the full id of the new commit.
func (r *Repo) Commit(msg string) string {
	r.T.Helper()
	r.Git("add", "-A")
	r.Git("-c", "commit.gpgsign=false", "commit", "-q", "--allow-empty", "-m", msg)
	return r.Git("rev-parse", "HEAD")
}
