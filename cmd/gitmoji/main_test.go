package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestPrototypeProcess(t *testing.T) {
	if os.Getenv("GITMOJI_TEST_PROCESS") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
			defer stop()
			os.Exit(run(ctx, os.Args[i+1:]))
		}
	}
	t.Fatal("missing test process arguments")
}

// The installed wrapper invokes this test binary exactly like the native CLI.
func TestMain(m *testing.M) {
	if os.Getenv("GITMOJI_TEST_NATIVE") == "1" {
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		code := run(ctx, os.Args[1:])
		stop()
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func TestCLIWithoutTTY(t *testing.T) {
	for _, test := range []struct {
		args    []string
		code    int
		message string
	}{
		{[]string{"prototype", "--type=feat", "--title=add README and ID", "--message=  `literal` $(literal)\n\nSigned-off-by: Ana\n"}, 0, "feat: add README and ID\n\n  `literal` $(literal)\n\nSigned-off-by: Ana\n"},
		{[]string{"prototype", "--commit", "--format=hybrid", "--type=docs", "--emoji=:memo:", "--title=update guide"}, 0, "docs: 📝 update guide\n"},
		{[]string{"prototype", "-c", "--format=emoji", "--emoji=:beers:", "--title=celebrate"}, 0, "🍻 Celebrate\n"},
		{[]string{"prototype", "--type=feat", "--title=" + strings.Repeat("á", 67)}, 0, "feat: " + strings.Repeat("á", 67) + "\n"},
		{[]string{"commit", "--type=docs", "--title=requires Git"}, 1, ""},
		{[]string{"commit"}, 2, ""},
		{[]string{"commit", "--format=hybrid", "--type=docs", "--emoji=:sparkles:", "--title=update"}, 2, ""},
		{[]string{"commit", "--title=text.", "--type=feat"}, 2, ""},
		{[]string{"commit", "--format=emoji", "--type=", "--emoji=:memo:", "--title=update"}, 2, ""},
		{[]string{"init"}, 1, ""},
		{[]string{"hook", "/file/that/does/not/exist"}, 1, ""},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestPrototypeProcess$", "--"}, test.args...)...)
			cmd.Dir = t.TempDir()
			cmd.Env = append(testEnvironment(t), "GITMOJI_TEST_PROCESS=1", "PATH=/no/git/node")
			var out, stderr bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &stderr
			err := cmd.Run()
			code := 0
			if err != nil {
				exit, ok := err.(*exec.ExitError)
				if !ok {
					t.Fatal(err)
				}
				code = exit.ExitCode()
			}
			if code != test.code || out.String() != test.message {
				t.Fatalf("exit %d, stdout %q, stderr %q", code, out.String(), stderr.String())
			}
			if len(test.args) > 2 && strings.Contains(test.args[len(test.args)-1], strings.Repeat("á", 67)) && !strings.Contains(stderr.String(), "73 Unicode code points") {
				t.Fatalf("missing warning: %q", stderr.String())
			}
		})
	}
}

func testEnvironment(t *testing.T) []string {
	t.Helper()
	home := t.TempDir()
	var env []string
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		switch key {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT", "GIT_AUTHOR_NAME", "GIT_AUTHOR_EMAIL", "GIT_COMMITTER_NAME", "GIT_COMMITTER_EMAIL":
			continue
		}
		env = append(env, entry)
	}
	return append(env, "HOME="+home, "USERPROFILE="+home, "XDG_CONFIG_HOME="+filepath.Join(home, "config"), "APPDATA="+filepath.Join(home, "config"))
}
