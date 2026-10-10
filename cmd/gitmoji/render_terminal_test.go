//go:build linux || darwin

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Snah-s/commit_tool/internal/catalog"
)

// A PTY transcript cannot detect artifacts left on screen by incremental redraws.
func TestEmojiSelectorScreen(t *testing.T) {
	// Older tmux captures emoji filler cells as extra spaces.
	normalize := func(text string) string { return strings.Join(strings.Fields(text), " ") }
	for _, mode := range []string{"wcwidth", "grapheme", "legacy-emoji"} {
		t.Run(mode, func(t *testing.T) {
			tmux, err := exec.LookPath("tmux")
			if err != nil {
				t.Skip("tmux unavailable for terminal screen verification")
			}
			c, err := catalog.Load()
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			socket := filepath.Join(dir, "tmux.sock")
			env := append(testEnvironment(t), "GITMOJI_TEST_PROCESS=1", "TERM=xterm-256color", "ACCESSIBLE=")
			run := func(args ...string) string {
				t.Helper()
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				cmd := exec.CommandContext(ctx, tmux, append([]string{"-S", socket}, args...)...)
				cmd.Dir, cmd.Env = dir, env
				out, err := cmd.CombinedOutput()
				if err != nil {
					t.Fatalf("tmux %v: %v: %s", args, err, out)
				}
				return string(out)
			}
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				_ = exec.CommandContext(ctx, tmux, "-S", socket, "kill-server").Run()
			})
			run("-f", "/dev/null", "new-session", "-d", "-s", "selector", "-x", "180", "-y", "24",
				"sh", "-c", "printf 'Previous terminal output\\n'; exec \"$@\"", "--",
				os.Args[0], "-test.run=^TestPrototypeProcess$", "--", "prototype")
			waitScreen := func(selected string) string {
				t.Helper()
				deadline := time.Now().Add(3 * time.Second)
				for {
					screen := run("capture-pane", "-p", "-t", "selector")
					if strings.Contains(normalize(screen), normalize(selected)) {
						return screen
					}
					if time.Now().After(deadline) {
						t.Fatalf("selection %q did not appear: %s", selected, screen)
					}
					time.Sleep(10 * time.Millisecond)
				}
			}
			waitScreen("> standard")
			run("send-keys", "-t", "selector", "Down", "Enter")
			first := c.Entries[0]
			waitScreen("> " + first.Emoji + " " + first.Code + " " + first.Description)
			if run("show-options", "-s", "-qv", "variation-selector-always-wide") != "" {
				width := "on"
				if mode == "wcwidth" {
					width = "off"
				}
				run("set-option", "-s", "variation-selector-always-wide", width)
			} else if mode == "wcwidth" {
				// Before tmux 3.6, VS16 is always wide and cannot emulate wcwidth.
				t.Skip("tmux lacks variation-selector-always-wide for wcwidth verification")
			}
			if mode == "grapheme" {
				// Emulate Foot's mode 2027 reply; tmux does not advertise this mode.
				run("send-keys", "-l", "-t", "selector", "\x1b[?2027;1$y")
			}
			labels := make(map[string]bool, len(c.Entries))
			for _, entry := range c.Entries {
				labels[normalize(entry.Emoji+" "+entry.Code+" "+entry.Description)] = true
			}
			for i, entry := range c.Entries {
				if i > 0 {
					run("send-keys", "-t", "selector", "Down")
				}
				screen := waitScreen("> " + entry.Emoji + " " + entry.Code + " " + entry.Description)
				if !strings.Contains(screen, "Previous terminal output") || !strings.Contains(screen, "? Choose a format: emoji") {
					t.Fatalf("redraw erased previous terminal output: %s", screen)
				}
				if strings.Count(screen, "> ") != 1 {
					t.Fatalf("multiple selection cursors: %s", screen)
				}
				for _, line := range strings.Split(screen, "\n") {
					if !strings.HasPrefix(line, "┃ ") {
						continue
					}
					label := strings.TrimSpace(strings.TrimPrefix(line, "┃ "))
					label = strings.TrimSpace(strings.TrimPrefix(label, "> "))
					if label != "" && label != "Choose a gitmoji" && !labels[normalize(label)] {
						t.Fatalf("corrupted option while selecting %s: %s", entry.Code, screen)
					}
				}
			}
		})
	}
}
