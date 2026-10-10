package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Run explicitly against the executable extracted from a candidate archive.
func TestReleaseArtifact(t *testing.T) {
	binary := os.Getenv("GITMOJI_ARTIFACT")
	if binary == "" {
		t.Skip("set GITMOJI_ARTIFACT to a packaged executable")
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	env := append(testEnvironment(t), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_EDITOR=:")
	run := func(command string, environment []string, code int, args ...string) string {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, command, args...)
		cmd.Dir, cmd.Env = dir, environment
		out, err := cmd.CombinedOutput()
		actual := 0
		if err != nil {
			exit, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatal(err)
			}
			actual = exit.ExitCode()
		}
		if actual != code {
			t.Fatalf("%s %v: exit %d, want %d: %s", command, args, actual, code, out)
		}
		return string(out)
	}
	// No external tools, even if Node is installed on the development runner.
	offline := append(append([]string{}, env...), "PATH="+t.TempDir())
	if text := run(binary, offline, 0, "--version"); !strings.Contains(text, "native, "+runtime.GOOS+"/"+runtime.GOARCH) || strings.Contains(text, "revision unknown") {
		t.Fatalf("missing artifact build information: %s", text)
	}
	run(binary, offline, 0, "--help")
	if text := run(binary, offline, 0, "list"); !strings.Contains(text, ":sparkles:") {
		t.Fatal("embedded catalog missing")
	}
	if text := run(binary, offline, 0, "prototype", "--format=hybrid", "--type=feat", "--emoji=:sparkles:", "--title=inspect package"); text != "feat: ✨ inspect package\n" {
		t.Fatalf("offline formatting: %q", text)
	}
	run(binary, offline, 0, "config", "--set", `commitFormat="standard"`)
	run(binary, offline, 0, "config", "--show")
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("offline commands created unexpected files in cwd: %v / %v", entries, err)
	}
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	// Unix Git can run through a symlink with its original libexec helpers.
	// Windows Git requires adjacent DLLs: native CI exercises its installed Git.
	if runtime.GOOS != "windows" {
		toolDir := t.TempDir()
		if err := os.Symlink(git, filepath.Join(toolDir, "git")); err != nil {
			t.Fatal(err)
		}
		env = append(env, "PATH="+toolDir)
	}
	run(git, env, 0, "init", "--quiet", "--initial-branch=main")
	run(git, env, 0, "config", "user.name", "Artifact Test")
	run(git, env, 0, "config", "user.email", "artifact@example.invalid")
	run(git, env, 0, "config", "commit.gpgsign", "false")
	path := filepath.Join(dir, "file")
	if err := os.WriteFile(path, []byte("staged\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run(git, env, 0, "add", "file")
	run(binary, env, 0, "commit", "--type=docs", "--title=verify package", "--message=  literal body")
	_, text, _ := strings.Cut(run(git, env, 0, "cat-file", "commit", "HEAD"), "\n\n")
	if text != "docs: verify package\n\n  literal body" {
		t.Fatalf("packaged client changed message: %q", text)
	}
	if runtime.GOOS == "windows" {
		run(binary, env, 1, "init")
		return
	}
	run(binary, env, 0, "init")
	run(binary, env, 1, "commit", "--type=docs", "--title=blocked by hook")
	if err := os.WriteFile(path, []byte("next\n"), 0600); err != nil {
		t.Fatal(err)
	}
	run(git, env, 0, "add", "file")
	run(git, env, 0, "commit", "--quiet", "-m", "manual title", "-m", "Signed-off-by: Artifact Test")
	_, text, _ = strings.Cut(run(git, env, 0, "cat-file", "commit", "HEAD"), "\n\n")
	if text != "manual title\n\nSigned-off-by: Artifact Test\n" {
		t.Fatalf("packaged hook changed non-TTY message: %q", text)
	}
	run(binary, env, 0, "remove")
}
