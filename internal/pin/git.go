// SPDX-License-Identifier: EUPL-1.2

package pin

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Git runs read-only git commands. Tests may replace it.
type Git interface {
	// Output runs `git <args>` in dir and returns its standard output.
	Output(dir string, args ...string) (string, error)
	// Stream runs `git <args>` in dir and calls fn with each record of the standard output, split at
	// sep, without reading it all into memory. A record longer than 256 KiB is passed truncated: the
	// callers read only hunk headers and paths.
	Stream(dir string, args []string, sep byte, fn func(rec string)) error
}

// ErrNoGit means the git program is not installed.
var ErrNoGit = errors.New("git not found")

// ExecGit is the real Git. git is started through exec with fixed arguments (no shell), read-only
// (GIT_OPTIONAL_LOCKS=0), with the options that decide diff output set on the command line and the
// environment variables that change it removed.
type ExecGit struct{}

// baseArgs precede every command: they do not depend on the user's git configuration.
var baseArgs = []string{"--no-pager", "-c", "core.quotepath=off", "-c", "core.autocrlf=false", "-c", "core.safecrlf=false",
	"-c", "diff.renames=true", "-c", "color.ui=false", "-c", "diff.external=", "-c", "diff.noprefix=false", "-c", "diff.interHunkContext=0"}

func command(ctx context.Context, dir string, args []string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", append(append([]string{}, baseArgs...), args...)...)
	cmd.Dir = dir
	var env []string
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "GIT_EXTERNAL_DIFF=") || strings.HasPrefix(kv, "GIT_DIFF_OPTS=") || strings.HasPrefix(kv, "GIT_PAGER=") {
			continue
		}
		env = append(env, kv)
	}
	cmd.Env = append(env, "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C", "GIT_TERMINAL_PROMPT=0")
	return cmd
}

const timeout = 60 * time.Second

// Output implements Git.
func (ExecGit) Output(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := command(ctx, dir, args)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	return string(out), wrap(err, stderr.String())
}

// Stream implements Git.
func (ExecGit) Stream(dir string, args []string, sep byte, fn func(rec string)) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	cmd := command(ctx, dir, args)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return wrap(err, "")
	}
	r := bufio.NewReaderSize(pipe, 256*1024)
	var readErr error
	for {
		chunk, err := r.ReadSlice(sep)
		if errors.Is(err, bufio.ErrBufferFull) {
			fn(string(chunk)) // truncated record: pass the start, drop the rest of it
			for errors.Is(err, bufio.ErrBufferFull) {
				_, err = r.ReadSlice(sep)
			}
			if err == nil {
				continue
			}
		}
		if err == nil {
			fn(string(chunk[:len(chunk)-1]))
			continue
		}
		if len(chunk) > 0 && errors.Is(err, io.EOF) {
			fn(string(chunk))
		}
		if !errors.Is(err, io.EOF) {
			readErr = err
		}
		break
	}
	_, _ = io.Copy(io.Discard, pipe)
	if err := cmd.Wait(); err != nil {
		return wrap(err, stderr.String())
	}
	return readErr
}

func wrap(err error, stderr string) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, exec.ErrNotFound) {
		return ErrNoGit
	}
	if s := strings.TrimSpace(stderr); s != "" {
		return fmt.Errorf("%v: %s", err, s)
	}
	return err
}
