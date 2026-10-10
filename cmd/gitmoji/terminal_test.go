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
	cmd.Env = append(append(testEnvironment(t), "TERM=xterm-256color", "ACCESSIBLE=", "GITMOJI_TEST_PROCESS=1", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GPG_TTY="+slave.Name()), cmd.Env...)
	if cmd.Dir == "" {
		cmd.Dir = t.TempDir()
		if err := os.WriteFile(filepath.Join(cmd.Dir, ".gitmojirc.json"), []byte(`{"scopePrompt":true}`), 0600); err != nil {
			t.Fatal(err)
		}
	}
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
			s.answer(t, "Choose a format", test.modeKeys)
			if test.mode != "emoji" {
				s.answer(t, "Choose a commit type", test.typeKeys)
			}
			if test.mode != "standard" {
				s.answer(t, "Choose a gitmoji", "/memo\r")
			}
			s.answer(t, "Scope", "users\r")
			s.answer(t, "Enter the commit title", "add résumé search\r")
			s.answer(t, "Body (optional)", "detail\r")
			s.answer(t, "Finish", "y\r")
			s.finish(t, 0)
			if got := s.stdout.String(); got != test.title+"\n\ndetail\n" {
				t.Fatalf("stdout contains UI or an incorrect message: %q", got)
			}
		})
	}
	for _, key := range []string{"\x03", "\x1b"} {
		t.Run(fmt.Sprintf("cancel_%q", key), func(t *testing.T) {
			s := startTerminal(t, helperCommand("prototype"))
			s.answer(t, "Choose a format", key)
			s.finish(t, 130)
			if s.stdout.Len() != 0 {
				t.Fatal("cancellation published a message")
			}
		})
	}
}

func TestInlineEmojiTerminal(t *testing.T) {
	s := startTerminal(t, helperCommand("prototype", "--format=emoji", "--emoji=:memo:", "--title=update README"))
	s.answer(t, "Choose a gitmoji", "/memo\r")
	s.answer(t, "Scope", "\r")
	s.answer(t, "Enter the commit title", "\r")
	s.answer(t, "Body (optional)", "\r")
	s.answer(t, "Finish", "y\r")
	s.finish(t, 0)
	if got := s.stdout.String(); got != "📝 Update README\n" {
		t.Fatalf("inline emoji message: %q", got)
	}
	for _, key := range []string{"\x03", "\x1b"} {
		s = startTerminal(t, helperCommand("prototype", "--format=emoji"))
		s.answer(t, "Choose a gitmoji", key)
		s.finish(t, 130)
	}
}

func TestAccessibleTerminal(t *testing.T) {
	s := startTerminal(t, helperCommand("prototype", "--accessible"))
	s.answer(t, "Choose a format", "\x1b[B\x1b[B\r")
	s.answer(t, "Choose a commit type", "\x1b[B\x1b[B\r")
	s.answer(t, "Choose a gitmoji", "/memo\r")
	s.answer(t, "Scope", "\n")
	s.answer(t, "Enter the commit title", "update guide\n")
	s.answer(t, "Body (optional)", "\n")
	s.answer(t, "Finish", "y\n")
	s.finish(t, 0)
	if got := s.stdout.String(); got != "docs: 📝 update guide\n" {
		t.Fatalf("accessible message: %q", got)
	}
	s = startTerminal(t, helperCommand("prototype", "--accessible"))
	s.answer(t, "Choose a format", "\x03")
	s.finish(t, 130)
	s = startTerminal(t, helperCommand("prototype", "--accessible"))
	s.answer(t, "Choose a format", "\r")
	s.answer(t, "Choose a commit type", "\r")
	s.answer(t, "Scope", "\x04")
	s.finish(t, 1)
	if s.stdout.Len() != 0 || !strings.Contains(ansi.Strip(s.text()), "EOF") {
		t.Fatal("EOF did not report a read error without publishing a message")
	}
}

func TestTerminalSignalCancellation(t *testing.T) {
	for _, test := range []struct {
		command, prompt string
		signal          os.Signal
	}{
		{"prototype", "Choose a format", os.Interrupt},
		{"config", "Commit format", os.Interrupt},
		{"prototype", "Choose a format", syscall.SIGTERM},
		{"config", "Commit format", syscall.SIGTERM},
	} {
		t.Run(test.command+"/"+test.signal.String(), func(t *testing.T) {
			s := startTerminal(t, helperCommand(test.command))
			s.answer(t, test.prompt, "")
			if err := s.cmd.Process.Signal(test.signal); err != nil {
				t.Fatal(err)
			}
			s.finish(t, 130)
			if s.stdout.Len() != 0 {
				t.Fatal("cancellation published output")
			}
		})
	}
}

func TestPreferenceTerminalCancellation(t *testing.T) {
	for _, prompt := range []string{"Commit format", "Emoji format"} {
		t.Run(prompt, func(t *testing.T) {
			s := startTerminal(t, helperCommand("config"))
			if prompt == "Emoji format" {
				s.answer(t, "Commit format", "\n")
			}
			s.answer(t, prompt, "\x03")
			s.finish(t, 130)
			if s.stdout.Len() != 0 || !strings.Contains(ansi.Strip(s.text()), "preferences were not saved") {
				t.Fatal("cancellation saved preferences or did not report it")
			}
		})
	}
}

func TestExplicitFormatAndHookConversion(t *testing.T) {
	s := startTerminal(t, helperCommand("prototype", "--format=standard", "--type=docs", "--scope=api", "--title=update README", "--message=  body\n\nSigned-off-by: Ana\n"))
	s.answer(t, "Choose a commit type", "\r")
	s.answer(t, "Scope", "\r")
	s.answer(t, "Enter the commit title", "\r")
	s.answer(t, "Body (optional)", "\r")
	s.answer(t, "Finish", "y\r")
	s.finish(t, 0)
	if strings.Contains(ansi.Strip(s.text()), "Choose a format") || s.stdout.String() != "docs(api): update README\n" {
		t.Fatalf("explicit format/defaults: %q / %q", ansi.Strip(s.text()), s.stdout.String())
	}
	s = startTerminal(t, helperCommand("prototype", "--format=emoji", "--emoji=:memo:", "--title=update README"))
	s.answer(t, "Choose a gitmoji", "\r")
	s.answer(t, "Scope", "\r")
	s.answer(t, "Enter the commit title", "\r")
	s.answer(t, "Body (optional)", "\r")
	s.answer(t, "Finish", "y\r")
	s.finish(t, 0)
	if strings.Contains(ansi.Strip(s.text()), "Choose a format") || strings.Contains(ansi.Strip(s.text()), "Choose a commit type") || s.stdout.String() != "📝 Update README\n" {
		t.Fatalf("omitted emoji fields: %q / %q", ansi.Strip(s.text()), s.stdout.String())
	}
	path := filepath.Join(t.TempDir(), "message with spaces")
	original := "📝 (api): update README\n\n  body\n\nBREAKING CHANGE: preserve\nSigned-off-by: Ana\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	s = startTerminal(t, helperCommand("prototype", "--hook", path, "message", "--format=hybrid", "--type=docs", "--accessible"))
	s.answer(t, "Choose a commit type", "\r")
	s.answer(t, "Choose a gitmoji", "\r")
	s.answer(t, "Scope", "\r")
	s.answer(t, "Enter the commit title", "\r")
	s.answer(t, "Body (optional)", "converted details\r")
	s.answer(t, "Finish", "y\r")
	s.finish(t, 0)
	if s.stdout.String() != "docs(api): 📝 update README\n\nconverted details\n" {
		t.Fatalf("conversion: %q", s.stdout.String())
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != original {
		t.Fatal("prototype modified the message file")
	}
	if err := os.WriteFile(path, []byte("docs: update README\n\npreserve\n"), 0600); err != nil {
		t.Fatal(err)
	}
	s = startTerminal(t, helperCommand("prototype", "--hook", path, "--format=standard"))
	s.finish(t, 0)
	if s.stdout.Len() != 0 {
		t.Fatalf("reprocessed a valid title: %q", s.stdout.String())
	}
}

func TestPreferenceTerminal(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".gitmojirc.json"), []byte(`{"commitFormat":"emoji","emojiFormat":"code","capitalizeTitle":false,"scopePrompt":false,"messagePrompt":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := helperCommand("prototype", "--format=emoji", "--emoji=:memo:", "--title=update README", "--scope=api", "--message=  original\n\nSigned-off-by: Ana\n")
	cmd.Dir = dir
	s := startTerminal(t, cmd)
	s.answer(t, "Choose a gitmoji", "\r")
	s.answer(t, "Enter the commit title", "\r")
	s.answer(t, "Finish", "y\r")
	s.finish(t, 0)
	if s.stdout.String() != ":memo: (api): update README\n\n  original\n\nSigned-off-by: Ana\n" || strings.Contains(ansi.Strip(s.text()), "Scope (") || strings.Contains(ansi.Strip(s.text()), "Body (") {
		t.Fatalf("preferences/defaults: %q / %q", s.stdout.String(), ansi.Strip(s.text()))
	}
	cmd = helperCommand("config", "--accessible")
	cmd.Dir = t.TempDir()
	s = startTerminal(t, cmd)
	s.answer(t, "Commit format", "hybrid\n")
	s.answer(t, "Emoji format", "code\n")
	s.answer(t, "Capitalize", "n\n")
	s.answer(t, "Automatically stage", "n\n")
	s.answer(t, "Scope prompt", "false\n")
	s.answer(t, "Prompt for a message body", "n\n")
	s.answer(t, "Catalog URL", "\n")
	s.answer(t, "Save global", "y\n")
	s.finish(t, 0)
	if !strings.Contains(s.stdout.String(), "Saved global preferences") {
		t.Fatalf("configuration save: %q", s.stdout.String())
	}
	cmd = helperCommand("config")
	cmd.Dir = t.TempDir()
	s = startTerminal(t, cmd)
	s.answer(t, "Commit format", "\x03")
	s.finish(t, 130)
	if s.stdout.Len() != 0 || !strings.Contains(ansi.Strip(s.text()), "preferences were not saved") {
		t.Fatalf("configuration cancellation: %q / %q", s.stdout.String(), ansi.Strip(s.text()))
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
	s.answer(t, "Choose a format", "\x03")
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
		cmd.Env = append(testEnvironment(t), "GITMOJI_TEST_PROCESS=1")
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

func TestNativeCommitTerminal(t *testing.T) {
	dir, env := phase4Repo(t)
	if err := os.WriteFile(filepath.Join(dir, ".gitmojirc.json"), []byte(`{"autoAdd":true,"scopePrompt":false,"messagePrompt":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "feature"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "commit", "--format=standard", "--type=feat", "--title=add feature", "--message=  body\n\nSigned-off-by: Ana\n")
	cmd.Dir, cmd.Env = dir, env
	s := startTerminal(t, cmd)
	s.answer(t, "Choose a commit type", "\r")
	s.answer(t, "Enter the commit title", "\r")
	s.answer(t, "Finish", "y\r")
	s.finish(t, 0)
	if !strings.Contains(gitCommand(t, dir, env, "log", "-1", "--format=%B"), "feat: add feature\n\n  body\n\nSigned-off-by: Ana") {
		t.Fatal("commit did not match the preview")
	}
	t.Run("autoAdd_disclosure", func(t *testing.T) {
		if !strings.Contains(ansi.Strip(s.text()), "Stages changes") {
			t.Fatal("confirmation did not disclose staging changes")
		}
	})
	if err := os.WriteFile(filepath.Join(dir, "pending"), []byte("keep unstaged"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		args      []string
		name, key string
		code      int
	}{
		{[]string{"commit", "--format=standard"}, "cancel", "\x03", 130},
		{[]string{"commit", "--format=standard", "--accessible"}, "EOF", "\x04", 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			head := gitCommand(t, dir, env, "rev-parse", "HEAD")
			cmd := exec.Command(os.Args[0], test.args...)
			cmd.Dir, cmd.Env = dir, env
			s := startTerminal(t, cmd)
			if test.name == "EOF" {
				s.answer(t, "Choose a commit type", "\r")
				s.answer(t, "Enter the commit title", test.key)
			} else {
				s.answer(t, "Choose a commit type", test.key)
			}
			s.finish(t, test.code)
			if gitCommand(t, dir, env, "diff", "--cached", "--name-only") != "" || gitCommand(t, dir, env, "rev-parse", "HEAD") != head {
				t.Fatal("cancel/EOF staged changes")
			}
			if test.name == "EOF" && !strings.Contains(ansi.Strip(s.text()), "EOF") {
				t.Fatal("EOF did not report a read error")
			}
		})
	}
}

func TestNativeHookConversionAndWriteFailure(t *testing.T) {
	dir, env := phase4Repo(t)
	if err := os.WriteFile(filepath.Join(dir, ".gitmojirc.json"), []byte(`{"commitFormat":"hybrid","emojiFormat":"code","scopePrompt":false,"messagePrompt":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "message with spaces")
	original := "📝 (api): update README\n\n  body\n\nSigned-off-by: Ana\n"
	if err := os.WriteFile(path, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "hook", path, "message", "--format=hybrid", "--type=docs", "--accessible")
	cmd.Dir, cmd.Env = dir, env
	s := startTerminal(t, cmd)
	s.answer(t, "Choose a commit type", "\r")
	s.answer(t, "Choose a gitmoji", "\r")
	s.answer(t, "Enter the commit title", "\n")
	s.answer(t, "Finish", "y\n")
	s.finish(t, 0)
	want := "docs(api): :memo: update README\n\n  body\n\nSigned-off-by: Ana\n"
	if data, err := os.ReadFile(path); err != nil || string(data) != want || s.stdout.Len() != 0 {
		t.Fatalf("hook did not save cleanly: %q / %v / %q", data, err, s.stdout.String())
	}
	for _, test := range []struct {
		key  string
		mode os.FileMode
		code int
	}{
		{"\x03", 0600, 0},
		{"\r", 0400, 1},
	} {
		if err := os.Chmod(path, 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(original), test.mode); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(path, test.mode); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(os.Args[0], "hook", path, "message", "--format=hybrid", "--type=docs", "--accessible")
		cmd.Dir, cmd.Env = dir, env
		s := startTerminal(t, cmd)
		s.answer(t, "Choose a commit type", test.key)
		if test.code == 1 {
			s.answer(t, "Choose a gitmoji", "\r")
			s.answer(t, "Enter the commit title", "\n")
			s.answer(t, "Finish", "y\n")
		}
		s.finish(t, test.code)
		if data, err := os.ReadFile(path); err != nil || string(data) != original {
			t.Fatal("cancel/failed write lost original")
		}
	}
}

func TestInstalledHookTemplateAndGitOperations(t *testing.T) {
	dir, env := phase4Repo(t)
	env = append(env, "ACCESSIBLE=1")
	if err := os.WriteFile(filepath.Join(dir, ".gitmojirc.json"), []byte(`{"scopePrompt":false,"messagePrompt":false}`), 0600); err != nil {
		t.Fatal(err)
	}
	template := filepath.Join(t.TempDir(), "template with spaces")
	if err := os.WriteFile(template, []byte("; template help\n\nraw title\n\n  paragraph\n# literal body\n\nSigned-off-by: Ana\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, dir, env, "config", "core.commentChar", ";")
	gitCommand(t, dir, env, "config", "commit.template", template)
	if err := os.WriteFile(filepath.Join(dir, "file"), []byte("base"), 0600); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, dir, env, "add", "file")
	nativeCommand(t, dir, env, 0, "init")
	cmd := exec.Command("git", "commit")
	cmd.Dir, cmd.Env = dir, env
	s := startTerminal(t, cmd)
	s.answer(t, "Choose a format", "\r")
	s.answer(t, "Choose a commit type", "\x1b[B\x1b[B\r")
	s.answer(t, "Enter the commit title", "update guide\n")
	s.answer(t, "Finish", "y\n")
	s.finish(t, 0)
	message := gitCommand(t, dir, env, "log", "-1", "--format=%B")
	if !strings.HasPrefix(message, "docs: update guide\n") || !strings.Contains(message, "  paragraph\n# literal body\n\nSigned-off-by: Ana") || strings.Contains(message, "; template help") {
		t.Fatalf("template/comments/trailers: %q", message)
	}
	gitCommand(t, dir, env, "config", "--unset", "commit.template")
	// Amend and merge sources must not prompt, even with a controlling terminal.
	cmd = exec.Command("git", "commit", "--amend", "--no-edit")
	cmd.Dir, cmd.Env = dir, env
	s = startTerminal(t, cmd)
	s.finish(t, 0)
	if gitCommand(t, dir, env, "log", "-1", "--format=%B") != message || strings.Contains(ansi.Strip(s.text()), "Finish the message?") {
		t.Fatal("amend reprocessed the message")
	}
	gitCommand(t, dir, env, "checkout", "-qb", "side")
	if err := os.WriteFile(filepath.Join(dir, "side"), []byte("side"), 0600); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, dir, env, "add", "side")
	gitCommand(t, dir, env, "commit", "-qm", "side message")
	gitCommand(t, dir, env, "checkout", "-q", "main")
	cmd = exec.Command("git", "merge", "--no-ff", "--no-edit", "side")
	cmd.Dir, cmd.Env = dir, env
	s = startTerminal(t, cmd)
	s.finish(t, 0)
	if strings.Contains(ansi.Strip(s.text()), "Finish the message?") {
		t.Fatal("merge opened prompts")
	}
	gitCommand(t, dir, env, "checkout", "-qb", "squash")
	if err := os.WriteFile(filepath.Join(dir, "squashed"), []byte("squash"), 0600); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, dir, env, "add", "squashed")
	gitCommand(t, dir, env, "commit", "-qm", "raw squash details")
	gitCommand(t, dir, env, "checkout", "-q", "main")
	gitCommand(t, dir, env, "merge", "--squash", "squash")
	cmd = exec.Command("git", "commit")
	cmd.Dir, cmd.Env = dir, env
	s = startTerminal(t, cmd)
	s.answer(t, "Choose a format", "\r")
	s.answer(t, "Choose a commit type", "\r")
	s.answer(t, "Enter the commit title", "combine squash changes\n")
	s.answer(t, "Finish", "y\n")
	s.finish(t, 0)
	message = gitCommand(t, dir, env, "log", "-1", "--format=%B")
	if !strings.HasPrefix(message, "feat: combine squash changes\n") || !strings.Contains(message, "raw squash details") {
		t.Fatalf("squash body was lost: %q", message)
	}
	head := gitCommand(t, dir, env, "rev-parse", "HEAD")
	linked := filepath.Join(t.TempDir(), "linked worktree")
	gitCommand(t, dir, env, "worktree", "add", "-qb", "linked", linked)
	if err := os.WriteFile(filepath.Join(linked, "linked"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, linked, env, "add", "linked")
	gitCommand(t, linked, env, "commit", "-qm", "raw linked message", "-m", "preserve body")
	if gitCommand(t, dir, env, "rev-parse", "HEAD") != head || !strings.Contains(gitCommand(t, linked, env, "log", "-1", "--format=%B"), "raw linked message\n\npreserve body") {
		t.Fatal("linked worktree hook changed another worktree or lost the message")
	}
	nativeCommand(t, linked, env, 1, "commit", "--type=docs", "--title=blocked in linked tree")
}

func TestHookSkipsActualRebase(t *testing.T) {
	dir, env := phase4Repo(t)
	path := filepath.Join(dir, "file")
	if err := os.WriteFile(path, []byte("base\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, dir, env, "add", "file")
	gitCommand(t, dir, env, "commit", "-qm", "base")
	gitCommand(t, dir, env, "checkout", "-qb", "topic")
	if err := os.WriteFile(path, []byte("topic\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, dir, env, "commit", "-qam", "raw topic title", "-m", "preserve topic body")
	gitCommand(t, dir, env, "checkout", "-q", "main")
	if err := os.WriteFile(path, []byte("main\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, dir, env, "commit", "-qam", "main change")
	nativeCommand(t, dir, env, 0, "init")
	gitCommand(t, dir, env, "checkout", "-q", "topic")
	cmd := exec.Command("git", "rebase", "main")
	cmd.Dir, cmd.Env = dir, env
	if _, err := cmd.CombinedOutput(); err == nil {
		t.Fatal("expected an actual rebase conflict")
	}
	message := filepath.Join(dir, "pending message")
	original := "raw pending title\n\nbody\n"
	if err := os.WriteFile(message, []byte(original), 0600); err != nil {
		t.Fatal(err)
	}
	cmd = exec.Command(os.Args[0], "hook", message)
	cmd.Dir, cmd.Env = dir, env
	s := startTerminal(t, cmd)
	s.finish(t, 0)
	if data, err := os.ReadFile(message); err != nil || string(data) != original || strings.Contains(ansi.Strip(s.text()), "Finish the message?") {
		t.Fatal("active rebase was reprocessed")
	}
	if err := os.WriteFile(path, []byte("resolved\n"), 0600); err != nil {
		t.Fatal(err)
	}
	gitCommand(t, dir, env, "add", "file")
	cmd = exec.Command("git", "rebase", "--continue")
	cmd.Dir, cmd.Env = dir, env
	s = startTerminal(t, cmd)
	s.finish(t, 0)
	if !strings.Contains(gitCommand(t, dir, env, "log", "-1", "--format=%B"), "raw topic title\n\npreserve topic body") || strings.Contains(ansi.Strip(s.text()), "Finish the message?") {
		t.Fatal("rebase lost/reprocessed the message")
	}
}
