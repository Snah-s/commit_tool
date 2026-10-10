package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func gitCommand(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir, cmd.Env = dir, env
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v / %s", args, err, out)
	}
	return string(out)
}

func phase4Repo(t *testing.T) (string, []string) {
	t.Helper()
	dir := t.TempDir()
	env := append(testEnvironment(t), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_EDITOR=:", "GITMOJI_TEST_NATIVE=1")
	gitCommand(t, dir, env, "init", "--quiet", "--initial-branch=main")
	gitCommand(t, dir, env, "config", "user.name", "Phase 4 Test")
	gitCommand(t, dir, env, "config", "user.email", "phase4@example.invalid")
	return dir, env
}

func nativeCommand(t *testing.T, dir string, env []string, code int, args ...string) (string, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], args...)
	cmd.Dir, cmd.Env = dir, env
	var out, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &stderr
	err := cmd.Run()
	actual := 0
	if err != nil {
		exit, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatal(err)
		}
		actual = exit.ExitCode()
	}
	if actual != code {
		t.Fatalf("%v: exit %d, expected %d / %s / %s", args, actual, code, &out, &stderr)
	}
	return out.String(), stderr.String()
}

func TestNativeGitClientAndHookLifecycle(t *testing.T) {
	dir, env := phase4Repo(t)
	path := filepath.Join(dir, "file")
	if err := os.WriteFile(path, []byte("staged\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, dir, env, "add", "file")
	if err := os.WriteFile(path, []byte("staged\nunstaged\n"), 0600); err != nil {
		t.Fatal(err)
	}
	body := "# literal body\n  `$()` quotes '\n\nSigned-off-by: Ana\n"
	nativeCommand(t, dir, env, 0, "commit", "--type=fix", "--title=preserve staged data", "--message="+body)
	if gitCommand(t, dir, env, "show", "HEAD:file") != "staged\n" || gitCommand(t, dir, env, "diff", "--", "file") == "" {
		t.Fatal("client committed unstaged data")
	}
	_, message, _ := strings.Cut(gitCommand(t, dir, env, "cat-file", "commit", "HEAD"), "\n\n")
	if message != "fix: preserve staged data\n\n"+body {
		t.Fatalf("message changed: %q", message)
	}
	head := gitCommand(t, dir, env, "rev-parse", "HEAD")
	nativeCommand(t, dir, env, 1, "commit", "--type=chore", "--title=no staged changes")
	if gitCommand(t, dir, env, "rev-parse", "HEAD") != head {
		t.Fatal("created an empty commit")
	}
	if runtime.GOOS == "windows" {
		_, stderr := nativeCommand(t, dir, env, 1, "init")
		if !strings.Contains(stderr, "Windows await terminal validation") {
			t.Fatal("Windows hook installation must explicitly report its limitation")
		}
		return
	}
	if err := os.WriteFile(filepath.Join(dir, ".gitmojirc.json"), []byte(`{"autoAdd":true}`), 0600); err != nil {
		t.Fatal(err)
	}
	nativeCommand(t, dir, env, 0, "init")
	nativeCommand(t, dir, env, 0, "init")
	nativeCommand(t, dir, env, 1, "commit", "--type=fix", "--title=blocked before staging")
	if gitCommand(t, dir, env, "diff", "--cached", "--name-only") != "" {
		t.Fatal("blocked client changed index")
	}
	nativeCommand(t, dir, env, 0, "remove")
	nativeCommand(t, dir, env, 0, "remove")
	hook := filepath.Join(dir, ".git", "hooks", "prepare-commit-msg")
	foreign := []byte("#!/bin/sh\n# gitmoji-native managed hook v1\necho foreign\n")
	if err := os.WriteFile(hook, foreign, 0755); err != nil {
		t.Fatal(err)
	}
	nativeCommand(t, dir, env, 1, "init")
	nativeCommand(t, dir, env, 1, "remove")
	if data, err := os.ReadFile(hook); err != nil || !bytes.Equal(data, foreign) {
		t.Fatal("foreign hook changed")
	}
}

func TestHookReplacementProtectsOriginal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "message")
	original := []byte("raw title\n\nbody\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := replaceHook(path, []byte("different"), []byte("new")); err == nil {
		t.Fatal("overwrote a concurrent edit")
	}
	if err := os.Chmod(path, 0400); err != nil {
		t.Fatal(err)
	}
	if err := replaceHook(path, original, []byte("new")); err == nil {
		t.Fatal("overwrote a read-only file")
	}
	if data, err := os.ReadFile(path); err != nil || !bytes.Equal(data, original) {
		t.Fatal("failed replacement changed original")
	}
	if err := os.Chmod(path, 0640); err != nil {
		t.Fatal(err)
	}
	if err := replaceHook(path, original, []byte("docs: update message\n")); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(path)
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0640 {
		t.Fatal("message permissions changed")
	}
}

func TestSignedNativeCommit(t *testing.T) {
	gpg, err := exec.LookPath("gpg")
	if err != nil {
		t.Skip("GnuPG unavailable")
	}
	dir, env := phase4Repo(t)
	// Short paths keep GnuPG agent sockets within Unix path-length limits.
	keyHome, err := os.MkdirTemp("", "gm-gpg-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(keyHome) })
	env = append(env, "GNUPGHOME="+keyHome)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, gpg, "--batch", "--pinentry-mode", "loopback", "--passphrase", "", "--quick-generate-key", "Phase 4 Test <phase4@example.invalid>", "ed25519", "sign", "0")
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("temporary signing key: %v / %s", err, out)
	}
	t.Cleanup(func() {
		if conf, err := exec.LookPath("gpgconf"); err == nil {
			cmd := exec.Command(conf, "--homedir", keyHome, "--kill", "gpg-agent")
			cmd.Env = env
			_ = cmd.Run()
		}
	})
	gitCommand(t, dir, env, "config", "gpg.program", gpg)
	gitCommand(t, dir, env, "config", "commit.gpgsign", "true")
	gitCommand(t, dir, env, "config", "user.signingkey", "phase4@example.invalid")
	if err := os.WriteFile(filepath.Join(dir, "signed"), []byte("content"), 0600); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, dir, env, "add", "signed")
	nativeCommand(t, dir, env, 0, "commit", "--type=docs", "--title=sign commit")
	gitCommand(t, dir, env, "verify-commit", "HEAD")
	head := gitCommand(t, dir, env, "rev-parse", "HEAD")
	gitCommand(t, dir, env, "config", "gpg.program", filepath.Join(t.TempDir(), "missing signer"))
	if err := os.WriteFile(filepath.Join(dir, "signed"), []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, dir, env, "add", "signed")
	nativeCommand(t, dir, env, 128, "commit", "--type=docs", "--title=refuse failed signature")
	if gitCommand(t, dir, env, "rev-parse", "HEAD") != head || !strings.Contains(gitCommand(t, dir, env, "diff", "--cached", "--name-only"), "signed") {
		t.Fatal("failed signing created a commit or changed staging")
	}
}
