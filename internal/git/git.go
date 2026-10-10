package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"unicode"

	"github.com/Snah-s/commit_tool/internal/storage"
)

type Repo struct{ Dir string }

func Open(ctx context.Context, cwd string) (Repo, error) {
	dir, err := filepath.Abs(cwd)
	if err != nil {
		return Repo{}, err
	}
	r := Repo{Dir: dir}
	inside, err := r.capture(ctx, "rev-parse", "--is-inside-work-tree")
	if err != nil {
		return Repo{}, err
	}
	if inside != "true" {
		return Repo{}, errors.New("Git requires a non-bare working tree")
	}
	return r, nil
}

func (r Repo) capture(ctx context.Context, args ...string) (string, error) {
	var out, stderr bytes.Buffer
	if err := r.run(ctx, nil, &out, &stderr, args...); err != nil {
		return "", fmt.Errorf("git %s: %s: %w", args[0], strings.TrimSpace(stderr.String()), err)
	}
	return strings.TrimSuffix(out.String(), "\n"), nil
}

func (r Repo) run(ctx context.Context, input io.Reader, output, errorOutput io.Writer, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir, cmd.Stdin, cmd.Stdout, cmd.Stderr = r.Dir, input, output, errorOutput
	err := cmd.Run()
	if err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func (r Repo) hookPath(ctx context.Context) (string, error) {
	// Resolve the directory only: Git can dereference a hook-file symlink.
	dir, err := r.capture(ctx, "rev-parse", "--path-format=absolute", "--git-path", "hooks")
	return filepath.Join(dir, "prepare-commit-msg"), err
}

func (r Repo) CommentPrefix(ctx context.Context) (string, error) {
	values, err := r.capture(ctx, "config", "--null", "--get-regexp", `^core\.comment(char|string)$`)
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return "#", nil
	}
	if err != nil {
		return "", err
	}
	prefix := "#"
	for _, setting := range strings.Split(strings.TrimSuffix(values, "\x00"), "\x00") {
		_, value, ok := strings.Cut(setting, "\n")
		if !ok || value == "" || strings.ContainsFunc(value, unicode.IsControl) {
			return "", errors.New("Git comment prefix must be nonempty and contain no control characters")
		}
		prefix = value
	}
	return prefix, nil
}

const hookPrefix = "#!/bin/sh\n# gitmoji-native managed hook v1\nexec "
const hookSuffix = " hook \"$@\"\n"

// Exact contents from gitmoji-cli c4b0e56adea61c9e279b18d16cd46a837dd4bd55,
// src/commands/hook/hook.js (the legacy template has no trailing newline).
const legacyHook = "#!/usr/bin/env bash\n# gitmoji as a commit hook\n" +
	"if npx -v >&/dev/null\nthen\n" +
	"  exec < /dev/tty\n  npx -c \"gitmoji --hook $1 $2\"\n" +
	"else\n  exec < /dev/tty\n  gitmoji --hook $1 $2\nfi"

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func managed(data []byte) bool {
	content := string(data)
	if content == legacyHook {
		return true
	}
	if !strings.HasPrefix(content, hookPrefix) || !strings.HasSuffix(content, hookSuffix) {
		return false
	}
	quoted := strings.TrimSuffix(strings.TrimPrefix(content, hookPrefix), hookSuffix)
	if len(quoted) < 2 || quoted[0] != '\'' || quoted[len(quoted)-1] != '\'' {
		return false
	}
	path := strings.ReplaceAll(quoted[1:len(quoted)-1], "'\\''", "'")
	return filepath.IsAbs(path) && !strings.ContainsRune(path, 0) && shellQuote(path) == quoted
}

func readHook(path string) ([]byte, os.FileInfo, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, info, errors.New("prepare-commit-msg is not a regular file; refusing to alter it")
	}
	data, err := os.ReadFile(path)
	return data, info, err
}

func (r Repo) HasManagedHook(ctx context.Context) (bool, error) {
	path, err := r.hookPath(ctx)
	if err != nil {
		return false, err
	}
	dir, err := os.Stat(filepath.Dir(path))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !dir.IsDir() {
		return false, nil
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, nil
	}
	if runtime.GOOS != "windows" {
		if _, err := exec.LookPath(path); err != nil {
			return false, nil
		}
	}
	data, err := os.ReadFile(path)
	return managed(data), err
}

func (r Repo) InstallHook(ctx context.Context, executable string) error {
	if runtime.GOOS == "windows" {
		return errors.New("managed hooks on Windows await terminal validation; use the commit client")
	}
	if !filepath.IsAbs(executable) || strings.ContainsRune(executable, 0) {
		return errors.New("hook executable must be an absolute path")
	}
	path, err := r.hookPath(ctx)
	if err != nil {
		return err
	}
	data, _, err := readHook(path)
	wrapper := []byte(hookPrefix + shellQuote(executable) + hookSuffix)
	if errors.Is(err, os.ErrNotExist) {
		return storage.CreateFile(path, wrapper, 0755)
	}
	if err != nil {
		return err
	}
	if !managed(data) {
		return errors.New("prepare-commit-msg belongs to another tool; integrate it manually")
	}
	return storage.WriteFile(path, wrapper, 0755)
}

func (r Repo) RemoveHook(ctx context.Context) error {
	path, err := r.hookPath(ctx)
	if err != nil {
		return err
	}
	if dir, err := os.Stat(filepath.Dir(path)); err == nil && !dir.IsDir() {
		return nil
	}
	data, _, err := readHook(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !managed(data) {
		return errors.New("prepare-commit-msg belongs to another tool; refusing to remove it")
	}
	return os.Remove(path)
}

func (r Repo) RebaseActive(ctx context.Context) (bool, error) {
	for _, name := range []string{"rebase-merge", "rebase-apply"} {
		path, err := r.capture(ctx, "rev-parse", "--path-format=absolute", "--git-path", name)
		if err != nil {
			return false, err
		}
		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			return true, nil
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return false, err
		}
	}
	return false, nil
}

func (r Repo) Commit(ctx context.Context, message string, autoAdd bool, input io.Reader, output, errorOutput io.Writer) error {
	active, err := r.HasManagedHook(ctx)
	if err != nil {
		return err
	}
	if active {
		return errors.New("the managed prepare-commit-msg hook is active; use git commit or remove the hook first")
	}
	if autoAdd {
		if err := r.run(ctx, input, output, errorOutput, "add", "."); err != nil {
			return err
		}
	}
	tempDir, err := filepath.Abs(os.TempDir())
	if err != nil {
		return err
	}
	file, err := os.CreateTemp(tempDir, "gitmoji-message-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := io.WriteString(file, message); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return r.run(ctx, input, output, errorOutput, "commit", "-F", file.Name(), "--cleanup=verbatim")
}

func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return 130
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() > 0 {
		return exit.ExitCode()
	}
	return 1
}
