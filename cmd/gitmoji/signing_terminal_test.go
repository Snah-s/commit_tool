//go:build linux

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestSignedTerminalCommit(t *testing.T) {
	gpg, err := exec.LookPath("gpg")
	if err != nil {
		t.Skip("GnuPG unavailable")
	}
	pinentry, err := exec.LookPath("pinentry-tty")
	if err != nil {
		t.Skip("pinentry-tty unavailable")
	}
	conf, err := exec.LookPath("gpgconf")
	if err != nil {
		t.Skip("gpgconf unavailable")
	}
	dir, env := phase4Repo(t)
	keyHome := t.TempDir()
	if err := os.Chmod(keyHome, 0700); err != nil {
		t.Fatal(err)
	}
	env = append(env, "GNUPGHOME="+keyHome)
	if err := os.WriteFile(filepath.Join(keyHome, "gpg-agent.conf"), []byte("pinentry-program "+pinentry+"\ndefault-cache-ttl 0\nmax-cache-ttl 0\n"), 0600); err != nil {
		t.Fatal(err)
	}
	stopAgent := func() {
		cmd := exec.Command(conf, "--homedir", keyHome, "--kill", "gpg-agent")
		cmd.Env = env
		_ = cmd.Run()
	}
	t.Cleanup(stopAgent)
	const passphrase = "phase-5-terminal-test"
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, gpg, "--batch", "--pinentry-mode", "loopback", "--passphrase", passphrase, "--quick-generate-key", "Phase 5 Test <phase5@example.invalid>", "ed25519", "sign", "0")
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("temporary protected key: %v / %s", err, out)
	}
	stopAgent()
	gitCommand(t, dir, env, "config", "gpg.program", gpg)
	gitCommand(t, dir, env, "config", "commit.gpgsign", "true")
	gitCommand(t, dir, env, "config", "user.signingkey", "phase5@example.invalid")
	if err := os.WriteFile(filepath.Join(dir, ".gitmojirc.json"), []byte(`{"scopePrompt":false,"messagePrompt":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "signed"), []byte("content"), 0600); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, dir, env, "add", "signed")
	cmd = exec.Command(os.Args[0], "commit", "--format=standard", "--type=docs", "--title=sign from terminal")
	cmd.Dir, cmd.Env = dir, env
	s := startTerminal(t, cmd)
	s.answer(t, "Choose a commit type", "\r")
	s.answer(t, "Enter the commit title", "\r")
	s.answer(t, "Finish", "y\r")
	s.answer(t, "Passphrase:", "")
	state, err := unix.IoctlGetTermios(int(s.slave.Fd()), unix.TCGETS)
	if err != nil || state.Lflag&unix.ICANON == 0 {
		t.Fatalf("UI left raw mode active for pinentry: %v", err)
	}
	if _, err := s.master.Write([]byte(passphrase + "\n")); err != nil {
		t.Fatal(err)
	}
	s.finish(t, 0)
	gitCommand(t, dir, env, "verify-commit", "HEAD")
}
