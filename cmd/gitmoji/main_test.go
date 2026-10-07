package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"testing"
	"time"
)

func TestPrototypeProcess(t *testing.T) {
	if os.Getenv("GITMOJI_TEST_PROCESS") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			os.Exit(run(ctx, os.Args[i+1:]))
		}
	}
	t.Fatal("missing test process arguments")
}

func TestCLIWithoutTTY(t *testing.T) {
	for _, test := range []struct {
		args    []string
		code    int
		message string
	}{
		{[]string{"commit", "--type=feat", "--title=add README and ID", "--message=  `literal` $(literal)\n\nSigned-off-by: Ana\n"}, 0, "feat: add README and ID\n\n  `literal` $(literal)\n\nSigned-off-by: Ana\n"},
		{[]string{"--commit", "--format=hybrid", "--type=docs", "--emoji=:memo:", "--title=update guide"}, 0, "docs: 📝 update guide\n"},
		{[]string{"-c", "--format=emoji", "--emoji=:beers:", "--title=celebrate"}, 0, "🍻 Celebrate\n"},
		{[]string{"commit", "--type=feat", "--title=" + strings.Repeat("á", 67)}, 0, "feat: " + strings.Repeat("á", 67) + "\n"},
		{[]string{"commit"}, 2, ""},
		{[]string{"commit", "--format=hybrid", "--type=docs", "--emoji=:sparkles:", "--title=update"}, 2, ""},
		{[]string{"commit", "--title=text.", "--type=feat"}, 2, ""},
		{[]string{"commit", "--format=emoji", "--type=", "--emoji=:memo:", "--title=update"}, 2, ""},
		{[]string{"-s", "bug", "linter"}, 1, ""},
		{[]string{"config"}, 1, ""},
		{[]string{"init"}, 1, ""},
		{[]string{"hook", "/file/that/does/not/exist"}, 1, ""},
	} {
		t.Run(strings.Join(test.args, " "), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestPrototypeProcess$", "--"}, test.args...)...)
			cmd.Dir = t.TempDir()
			cmd.Env = append(os.Environ(), "GITMOJI_TEST_PROCESS=1", "PATH=/no/git/node")
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
