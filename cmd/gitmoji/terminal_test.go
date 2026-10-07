//go:build linux || darwin

package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"
	"github.com/creack/pty"
)

type terminalSession struct {
	master, slave *os.File
	cmd           *exec.Cmd
	initial       *term.State
	stdout        bytes.Buffer
	mu            sync.Mutex
	transcript    strings.Builder
	done          chan error
}

func startTerminal(t *testing.T, cmd *exec.Cmd) *terminalSession {
	t.Helper()
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	s := &terminalSession{master: master, slave: slave, cmd: cmd, done: make(chan error, 1)}
	if err := pty.Setsize(master, &pty.Winsize{Rows: 15, Cols: 40}); err != nil {
		t.Fatal(err)
	}
	s.initial, err = term.GetState(slave.Fd())
	if err != nil {
		t.Fatal(err)
	}
	cmd.Stdin, cmd.Stderr, cmd.Stdout = slave, slave, &s.stdout
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "ACCESSIBLE=", "GITMOJI_TEST_PROCESS=1", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true, Setctty: true, Ctty: 0}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		master.Close()
		slave.Close()
	})
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := master.Read(buf)
			if n > 0 {
				chunk := string(buf[:n])
				s.mu.Lock()
				s.transcript.WriteString(chunk)
				s.mu.Unlock()
				if strings.Contains(chunk, "\x1b[6n") {
					_, _ = io.WriteString(master, "\x1b[1;1R")
				}
			}
			if err != nil {
				return
			}
		}
	}()
	go func() { s.done <- cmd.Wait() }()
	return s
}

func helperCommand(args ...string) *exec.Cmd {
	return exec.Command(os.Args[0], append([]string{"-test.run=^TestPrototypeProcess$", "--"}, args...)...)
}

func (s *terminalSession) text() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.transcript.String()
}

func (s *terminalSession) answer(t *testing.T, prompt, value string) {
	t.Helper()
	deadline := time.Now().Add(8 * time.Second)
	for !strings.Contains(ansi.Strip(s.text()), prompt) {
		if time.Now().After(deadline) {
			t.Fatalf("prompt did not appear %q: %q", prompt, ansi.Strip(s.text()))
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := io.WriteString(s.master, value); err != nil {
		t.Fatal(err)
	}
}

func (s *terminalSession) finish(t *testing.T, code int) {
	t.Helper()
	select {
	case err := <-s.done:
		got := 0
		if err != nil {
			if exit, ok := err.(*exec.ExitError); ok {
				got = exit.ExitCode()
			} else {
				t.Fatal(err)
			}
		}
		if got != code {
			t.Fatalf("exit %d, expected %d; %q", got, code, ansi.Strip(s.text()))
		}
	case <-time.After(8 * time.Second):
		t.Fatalf("process does not terminate: %q", ansi.Strip(s.text()))
	}
	state, err := term.GetState(s.slave.Fd())
	if err != nil || !reflect.DeepEqual(state, s.initial) {
		t.Fatalf("terminal was not restored: %v", err)
	}
	if strings.Contains(s.text(), "\x1b[?1049h") {
		t.Fatal("entered the alternate screen")
	}
	if strings.Contains(s.text(), "\x1b[3J") {
		t.Fatal("erased scrollback")
	}
	if hidden := strings.LastIndex(s.text(), "\x1b[?25l"); hidden >= 0 && strings.LastIndex(s.text(), "\x1b[?25h") < hidden {
		t.Fatal("cursor visibility was not restored")
	}
}

func TestTerminalFlow(t *testing.T) {
	for _, test := range []struct {
		mode, modeKeys, typeKeys, title string
	}{
		{"standard", "\r", "\r", "feat(users): add résumé search"},
		{"emoji", "\x1b[B\r", "", "📝 (users): Add résumé search"},
		{"hybrid", "\x1b[B\x1b[B\r", "\x1b[B\x1b[B\r", "docs(users): 📝 add résumé search"},
	} {
		t.Run(test.mode, func(t *testing.T) {
			s := startTerminal(t, helperCommand("prototype"))
			s.answer(t, "Format", test.modeKeys)
			if test.mode != "emoji" {
				s.answer(t, "Type", test.typeKeys)
			}
			if test.mode != "standard" {
				s.answer(t, "Emoji", "/memo\r")
			}
			s.answer(t, "Scope", "users\r")
			s.answer(t, "Description", "add résumé search\r")
			s.answer(t, "Body", "detail\nSigned-off-by: Ana\r")
			s.answer(t, "Finish", "y\r")
			s.finish(t, 0)
			if got := s.stdout.String(); got != test.title+"\n\ndetail\nSigned-off-by: Ana\n" {
				t.Fatalf("stdout contains UI or an incorrect message: %q", got)
			}
		})
	}
	for _, key := range []string{"\x03", "\x1b"} {
		t.Run(fmt.Sprintf("cancel_%q", key), func(t *testing.T) {
			s := startTerminal(t, helperCommand("prototype"))
			s.answer(t, "Format", key)
			s.finish(t, 130)
			if s.stdout.Len() != 0 {
				t.Fatal("cancellation published a message")
			}
		})
	}
}

func TestAccessibleTerminal(t *testing.T) {
	s := startTerminal(t, helperCommand("prototype", "--accessible"))
	s.answer(t, "Format", "hybrid\n")
	s.answer(t, "Type", "docs\n")
	s.answer(t, "Emoji (", ":memo:\n")
	s.answer(t, "Scope", "\n")
	s.answer(t, "Description", "update guide\n")
	s.answer(t, "Write/replace", "n\n")
	s.answer(t, "Finish", "y\n")
	s.finish(t, 0)
	if got := s.stdout.String(); got != "docs: 📝 update guide\n" {
		t.Fatalf("accessible message: %q", got)
	}
	s = startTerminal(t, helperCommand("prototype", "--accessible"))
	s.answer(t, "Format", "\x03")
	s.finish(t, 130)
	s = startTerminal(t, helperCommand("prototype", "--accessible"))
	s.answer(t, "Format", "\x04")
	s.finish(t, 1)
}

func TestExplicitFormatAndHookConversion(t *testing.T) {
	s := startTerminal(t, helperCommand("commit", "--format=standard", "--type=docs", "--scope=api", "--title=update README", "--message=  body\n\nSigned-off-by: Ana\n"))
	s.answer(t, "Type", "\r")
	s.answer(t, "Scope", "\r")
	s.answer(t, "Description", "\r")
	s.answer(t, "Body", "\r")
	s.answer(t, "Finish", "y\r")
	s.finish(t, 0)
	if strings.Contains(ansi.Strip(s.text()), "Format") || s.stdout.String() != "docs(api): update README\n\n  body\n\nSigned-off-by: Ana\n" {
		t.Fatalf("explicit format/defaults: %q / %q", ansi.Strip(s.text()), s.stdout.String())
	}
	s = startTerminal(t, helperCommand("commit", "--format=emoji", "--emoji=:memo:", "--title=update README"))
	s.answer(t, "Emoji", "\r")
	s.answer(t, "Scope", "\r")
	s.answer(t, "Description", "\r")
	s.answer(t, "Body", "\r")
	s.answer(t, "Finish", "y\r")
	s.finish(t, 0)
	if strings.Contains(ansi.Strip(s.text()), "Format") || strings.Contains(ansi.Strip(s.text()), "Type") || s.stdout.String() != "📝 Update README\n" {
		t.Fatalf("omitted emoji fields: %q / %q", ansi.Strip(s.text()), s.stdout.String())
	}
	path := filepath.Join(t.TempDir(), "message with spaces")
	original := "📝 (api): update README\n\n  body\n\nBREAKING CHANGE: preserve\nSigned-off-by: Ana\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	s = startTerminal(t, helperCommand("hook", path, "message", "--format=hybrid", "--type=docs", "--accessible"))
	s.answer(t, "Type", "\n")
	s.answer(t, "Emoji (", "\n")
	s.answer(t, "Scope", "\n")
	s.answer(t, "Description", "\n")
	s.answer(t, "Write/replace", "n\n")
	s.answer(t, "Finish", "y\n")
	s.finish(t, 0)
	if s.stdout.String() != "docs(api): 📝 update README\n\n  body\n\nBREAKING CHANGE: preserve\nSigned-off-by: Ana\n" {
		t.Fatalf("conversion: %q", s.stdout.String())
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != original {
		t.Fatal("hook modified the file in phase 2")
	}
	if err := os.WriteFile(path, []byte("docs: update README\n\npreserve\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s = startTerminal(t, helperCommand("hook", path, "--format=standard"))
	s.finish(t, 0)
	if s.stdout.Len() != 0 {
		t.Fatalf("reprocessed a valid title: %q", s.stdout.String())
	}
}

func TestGitHookTerminalAndNoTTY(t *testing.T) {
	repo := t.TempDir()
	git := func(args ...string) []byte {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return out
	}
	git("init", "--quiet")
	git("config", "user.name", "Phase 1 Test")
	git("config", "user.email", "phase1@example.invalid")
	git("config", "commit.gpgsign", "false")
	quote := func(value string) string { return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'" }
	wrapper := "#!/bin/sh\nGITMOJI_TEST_PROCESS=1 " + quote(os.Args[0]) + " -test.run='^TestPrototypeProcess$' -- prototype --hook \"$1\" \"$2\" \"$3\"\n"
	if err := os.WriteFile(filepath.Join(repo, ".git", "hooks", "prepare-commit-msg"), []byte(wrapper), 0700); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "commit", "--allow-empty", "-m", "add hook", "-m", "paragraph\n\nSigned-off-by: Ana")
	cmd.Dir = repo
	s := startTerminal(t, cmd)
	s.answer(t, "Format", "\x03")
	s.finish(t, 0)
	message := string(git("log", "-1", "--format=%B"))
	if !strings.Contains(message, "add hook\n\nparagraph\n\nSigned-off-by: Ana") {
		t.Fatalf("hook lost the message: %q", message)
	}
	// Without a session/control terminal: the client fails and the hook preserves the file.
	path := filepath.Join(t.TempDir(), "message")
	original := []byte("add hook\n\npreserve body\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		args []string
		code int
	}{{[]string{"prototype"}, 2}, {[]string{"prototype", "--hook", path}, 0}} {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		cmd := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestPrototypeProcess$", "--"}, test.args...)...)
		cmd.Env = append(os.Environ(), "GITMOJI_TEST_PROCESS=1")
		cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		err := cmd.Run()
		cancel()
		got := 0
		if err != nil {
			if exit, ok := err.(*exec.ExitError); ok {
				got = exit.ExitCode()
			} else {
				t.Fatal(err)
			}
		}
		if got != test.code {
			t.Fatalf("without TTY %v: %d", test.args, got)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(data, original) {
		t.Fatal("hook without TTY modified the message")
	}
}
