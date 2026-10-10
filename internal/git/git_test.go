package git

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func command(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSuffix(string(out), "\n")
}

func testRepo(t *testing.T) Repo {
	t.Helper()
	// Keep identity, signing and user hooks isolated from the test process.
	for _, key := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT"} {
		t.Setenv(key, "")
		if err := os.Unsetenv(key); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir := t.TempDir()
	command(t, dir, "init", "-q")
	command(t, dir, "config", "user.name", "Git backend test")
	command(t, dir, "config", "user.email", "git-test@example.invalid")
	r, err := Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func write(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func TestOpenAndCommentPrefix(t *testing.T) {
	ctx := context.Background()
	r := testRepo(t)
	if _, err := Open(ctx, t.TempDir()); err == nil {
		t.Fatal("accepted outside repository")
	}
	bare := t.TempDir()
	command(t, bare, "init", "--bare", "-q")
	if _, err := Open(ctx, bare); err == nil {
		t.Fatal("accepted bare repository")
	}
	if value, err := r.CommentPrefix(ctx); err != nil || value != "#" {
		t.Fatalf("default prefix: %q, %v", value, err)
	}
	command(t, r.Dir, "config", "core.commentString", "//")
	command(t, r.Dir, "config", "core.commentChar", "※")
	if value, err := r.CommentPrefix(ctx); err != nil || value != "※" {
		t.Fatalf("last alias precedence: %q, %v", value, err)
	}
	command(t, r.Dir, "config", "--add", "core.commentString", "auto")
	if value, err := r.CommentPrefix(ctx); err != nil || value != "auto" {
		t.Fatalf("auto prefix: %q, %v", value, err)
	}
	for _, test := range []struct{ name, prefix string }{{"empty", ""}, {"newline", "\n"}, {"tab", "\t"}} {
		t.Run(test.name, func(t *testing.T) {
			r := testRepo(t)
			command(t, r.Dir, "config", "core.commentString", test.prefix)
			if _, err := r.CommentPrefix(ctx); err == nil {
				t.Fatalf("invalid prefix accepted: %q", test.prefix)
			}
		})
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err := Open(canceled, r.Dir)
	if ExitCode(err) != 130 || ExitCode(errors.New("operation")) != 1 || ExitCode(nil) != 0 {
		t.Fatalf("exit codes: %v", err)
	}
}

func TestCommitPreservesIndexAndVerbatimMessage(t *testing.T) {
	ctx := context.Background()
	r := testRepo(t)
	path := filepath.Join(r.Dir, "file")
	write(t, path, "staged\n", 0600)
	command(t, r.Dir, "add", "file")
	write(t, path, "staged\nunstaged\n", 0600)
	message := "fix: preserve quotes ' and dollars $()\n\n# body content\n  indented\n\nSigned-off-by: Test <test@example.invalid>\n"
	var stdout, stderr bytes.Buffer
	if err := r.Commit(ctx, message, false, strings.NewReader("stdin\n"), &stdout, &stderr); err != nil {
		t.Fatalf("commit: %v: %s", err, &stderr)
	}
	if got := command(t, r.Dir, "show", "HEAD:file"); got != "staged" {
		t.Fatalf("committed unstaged content: %q", got)
	}
	if command(t, r.Dir, "diff", "--", "file") == "" {
		t.Fatal("unstaged change disappeared")
	}
	if got := command(t, r.Dir, "show", "-s", "--format=%B"); got != message {
		t.Fatalf("message changed: %q != %q", got, message)
	}
	if stdout.Len() == 0 {
		t.Fatal("Git stdout was not forwarded")
	}
	err := r.Commit(ctx, "chore: empty\n", false, nil, &stdout, &stderr)
	if ExitCode(err) != 1 {
		t.Fatalf("empty commit accepted: %v", err)
	}
}

func TestAutoAddOnlyInvocationDirectory(t *testing.T) {
	ctx := context.Background()
	r := testRepo(t)
	write(t, filepath.Join(r.Dir, "outside"), "original", 0600)
	write(t, filepath.Join(r.Dir, "sub", "inside"), "original", 0600)
	command(t, r.Dir, "add", ".")
	command(t, r.Dir, "commit", "-qm", "initial")
	write(t, filepath.Join(r.Dir, "outside"), "changed", 0600)
	write(t, filepath.Join(r.Dir, "sub", "inside"), "changed", 0600)
	sub, err := Open(ctx, filepath.Join(r.Dir, "sub"))
	if err != nil {
		t.Fatal(err)
	}
	if sub.Dir != filepath.Join(r.Dir, "sub") {
		t.Fatal("Open discarded invocation directory")
	}
	if err := sub.Commit(ctx, "fix: inside\n", true, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got := command(t, r.Dir, "show", "HEAD:outside"); got != "original" {
		t.Fatalf("autoAdd staged outside cwd: %q", got)
	}
	if got := command(t, r.Dir, "show", "HEAD:sub/inside"); got != "changed" {
		t.Fatalf("autoAdd missed cwd: %q", got)
	}
}

func TestHookOwnershipAndBlocking(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("managed hook installation and POSIX permissions require Unix")
	}
	ctx := context.Background()
	r := testRepo(t)
	path, err := r.hookPath(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.RemoveHook(ctx); err != nil {
		t.Fatal(err)
	}
	for _, content := range []string{
		"#!/bin/sh\n# gitmoji-native managed hook v1\necho foreign\n",
		legacyHook + "\n",
		hookPrefix + "'/tmp/valid'" + hookSuffix + "echo extra\n",
		hookPrefix + "'/tmp/path'; echo bad #" + hookSuffix,
	} {
		write(t, path, content, 0755)
		if active, err := r.HasManagedHook(ctx); err != nil || active {
			t.Fatalf("foreign recognized: %v %v", active, err)
		}
		if err := r.InstallHook(ctx, "/tmp/tool"); err == nil {
			t.Fatal("foreign hook overwritten")
		}
		if err := r.RemoveHook(ctx); err == nil {
			t.Fatal("foreign hook removed")
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != content {
			t.Fatal("foreign hook changed")
		}
	}
	write(t, path, legacyHook, 0755)
	if active, err := r.HasManagedHook(ctx); err != nil || !active {
		t.Fatalf("exact legacy not recognized: %v %v", active, err)
	}
	executable := filepath.Join(t.TempDir(), "tool with 'quotes' and $dollars")
	write(t, executable, "#!/bin/sh\nprintf '%s\\n' \"$@\"\n", 0755)
	if err := r.InstallHook(ctx, executable); err != nil {
		t.Fatal(err)
	}
	if err := r.InstallHook(ctx, executable); err != nil {
		t.Fatalf("repeat install: %v", err)
	}
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0755 {
		t.Fatalf("hook permissions: %v", info.Mode())
	}
	cmd := exec.Command(path, "file with space", "message", "$object'quoted")
	out, err := cmd.CombinedOutput()
	if err != nil || string(out) != "hook\nfile with space\nmessage\n$object'quoted\n" {
		t.Fatalf("wrapper forwarded arguments incorrectly: %q %v", out, err)
	}
	write(t, filepath.Join(r.Dir, "untracked"), "keep unstaged", 0600)
	if err := r.Commit(ctx, "fix: blocked\n", true, nil, nil, nil); err == nil {
		t.Fatal("client did not block managed hook")
	}
	if got := command(t, r.Dir, "diff", "--cached", "--name-only"); got != "" {
		t.Fatalf("blocked commit changed index: %s", got)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if active, err := r.HasManagedHook(ctx); err != nil || active {
		t.Fatalf("disabled hook treated active: %v %v", active, err)
	}
	if err := r.RemoveHook(ctx); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(executable, path); err != nil {
		t.Fatal(err)
	}
	if err := r.InstallHook(ctx, executable); err == nil {
		t.Fatal("overwrote symlink")
	}
	if err := r.RemoveHook(ctx); err == nil {
		t.Fatal("removed symlink")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	managedTarget := filepath.Join(t.TempDir(), "managed target")
	write(t, managedTarget, hookPrefix+shellQuote("/tmp/native")+hookSuffix, 0755)
	if err := os.Symlink(managedTarget, path); err != nil {
		t.Fatal(err)
	}
	if active, err := r.HasManagedHook(ctx); err != nil || !active {
		t.Fatalf("active managed symlink was not detected: %v %v", active, err)
	}
	if err := r.Commit(ctx, "fix: blocked symlink\n", true, nil, nil, nil); err == nil {
		t.Fatal("client bypassed an active managed symlink")
	}
	if err := r.RemoveHook(ctx); err == nil {
		t.Fatal("mutated a managed symlink")
	}
	if err := r.InstallHook(ctx, executable); err == nil {
		t.Fatal("updated a managed symlink target")
	}
	command(t, r.Dir, "config", "core.hooksPath", os.DevNull)
	if active, err := r.HasManagedHook(ctx); err != nil || active {
		t.Fatalf("disabled hooks path: %v %v", active, err)
	}
	if err := r.RemoveHook(ctx); err != nil {
		t.Fatalf("remove with disabled hooks path: %v", err)
	}
}

func TestWindowsManagedHookGuard(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows uses Git hook semantics rather than executable extensions")
	}
	ctx := context.Background()
	r := testRepo(t)
	path, err := r.hookPath(ctx)
	if err != nil {
		t.Fatal(err)
	}
	write(t, path, legacyHook, 0644)
	if active, err := r.HasManagedHook(ctx); err != nil || !active {
		t.Fatalf("legacy hook must block the client: %v %v", active, err)
	}
	write(t, filepath.Join(r.Dir, "unstaged"), "keep", 0600)
	if err := r.Commit(ctx, "docs: blocked\n", true, nil, nil, nil); err == nil {
		t.Fatal("client bypassed a managed hook on Windows")
	}
	if command(t, r.Dir, "diff", "--cached", "--name-only") != "" {
		t.Fatal("blocked client staged files")
	}
	if err := r.InstallHook(ctx, filepath.Join(r.Dir, "gitmoji.exe")); err == nil {
		t.Fatal("unvalidated Windows installation succeeded")
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != legacyHook {
		t.Fatal("rejected installation changed the legacy hook")
	}
}

func TestGitHookFailurePreservesStatusAndStreams(t *testing.T) {
	ctx := context.Background()
	r := testRepo(t)
	write(t, filepath.Join(r.Dir, "file"), "staged", 0600)
	command(t, r.Dir, "add", ".")
	write(t, filepath.Join(r.Dir, ".git", "hooks", "prepare-commit-msg"), "#!/bin/sh\necho hook-stdout\necho hook-stderr >&2\nexit 23\n", 0755)
	var stdout, stderr bytes.Buffer
	err := r.Commit(ctx, "fix: refused\n", false, nil, &stdout, &stderr)
	// Git maps a failing hook to its own status 1 rather than the hook's status.
	if ExitCode(err) != 1 || !strings.Contains(stderr.String(), "hook-stderr") || !strings.Contains(stdout.String()+stderr.String(), "hook-stdout") {
		t.Fatalf("Git failure: %v, stdout=%q stderr=%q", err, &stdout, &stderr)
	}
	if command(t, r.Dir, "diff", "--cached", "--name-only") != "file" {
		t.Fatal("failed commit changed staging")
	}
}

func TestHooksPathWorktreeAndRebase(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("managed hook installation awaits Windows terminal validation")
	}
	ctx := context.Background()
	r := testRepo(t)
	write(t, filepath.Join(r.Dir, "file"), "initial", 0600)
	command(t, r.Dir, "add", ".")
	command(t, r.Dir, "commit", "-qm", "initial")
	subDir := filepath.Join(r.Dir, "sub")
	if err := os.Mkdir(subDir, 0700); err != nil {
		t.Fatal(err)
	}
	sub, err := Open(ctx, subDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, hooks := range []string{"relative hooks", filepath.Join(t.TempDir(), "absolute hooks")} {
		command(t, r.Dir, "config", "core.hooksPath", hooks)
		if err := sub.InstallHook(ctx, "/tmp/gitmoji"); err != nil {
			t.Fatal(err)
		}
		expected := hooks
		if !filepath.IsAbs(expected) {
			expected = filepath.Join(r.Dir, expected)
		}
		if _, err := os.Stat(filepath.Join(expected, "prepare-commit-msg")); err != nil {
			t.Fatalf("hooksPath not resolved at worktree root: %v", err)
		}
		if err := sub.RemoveHook(ctx); err != nil {
			t.Fatal(err)
		}
	}
	command(t, r.Dir, "config", "--unset", "core.hooksPath")
	linked := filepath.Join(t.TempDir(), "linked tree")
	command(t, r.Dir, "worktree", "add", "-qb", "linked", linked)
	worktree, err := Open(ctx, linked)
	if err != nil {
		t.Fatal(err)
	}
	if err := worktree.InstallHook(ctx, "/tmp/gitmoji"); err != nil {
		t.Fatal(err)
	}
	if active, err := r.HasManagedHook(ctx); err != nil || !active {
		t.Fatalf("linked worktree common hook: %v %v", active, err)
	}
	if err := worktree.RemoveHook(ctx); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"rebase-merge", "rebase-apply"} {
		path := command(t, linked, "rev-parse", "--path-format=absolute", "--git-path", name)
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		if active, err := worktree.RebaseActive(ctx); err != nil || !active {
			t.Fatalf("linked rebase %s: %v %v", name, active, err)
		}
		if active, err := r.RebaseActive(ctx); err != nil || active {
			t.Fatalf("linked rebase leaked into main worktree: %v %v", active, err)
		}
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
	}
	command(t, linked, "config", "core.hooksPath", "worktree hooks")
	if err := worktree.InstallHook(ctx, "/tmp/gitmoji"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(linked, "worktree hooks", "prepare-commit-msg")); err != nil {
		t.Fatalf("linked relative hooksPath: %v", err)
	}
}
