package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Snah-s/commit_tool/internal/config"
)

func TestConfigurationAndOfflineCatalog(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "project")
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(cwd, 0700); err != nil {
		t.Fatal(err)
	}
	env := append(os.Environ(), "GITMOJI_TEST_PROCESS=1", "HOME="+home, "USERPROFILE="+home, "XDG_CONFIG_HOME="+filepath.Join(home, "config"), "APPDATA="+filepath.Join(home, "config"), "PATH=/no/git/node")
	execute := func(code int, args ...string) (string, string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], append([]string{"-test.run=^TestPrototypeProcess$", "--"}, args...)...)
		cmd.Dir = cwd
		cmd.Env = env
		var out, stderr bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &stderr
		err := cmd.Run()
		got := 0
		if err != nil {
			exit, ok := err.(*exec.ExitError)
			if !ok {
				t.Fatal(err)
			}
			got = exit.ExitCode()
		}
		if got != code {
			t.Fatalf("%v: exit %d, expected %d; %s", args, got, code, stderr.String())
		}
		return out.String(), stderr.String()
	}
	show, _ := execute(0, "config", "--show")
	var initial struct {
		GlobalPath string `json:"globalPath"`
	}
	if err := json.Unmarshal([]byte(show), &initial); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(initial.GlobalPath); !os.IsNotExist(err) {
		t.Fatal("show created preferences")
	}
	execute(0, "config", "--set", `commitFormat="hybrid"`, "--set", `emojiFormat="code"`, "--set", `capitalizeTitle=false`)
	out, _ := execute(0, "prototype", "--type=feat", "--emoji=:sparkles:", "--title=add résumé")
	if out != "feat: :sparkles: add résumé\n" {
		t.Fatalf("saved preferences: %q", out)
	}
	out, _ = execute(0, "prototype", "--format=standard", "--type=docs", "--title=update README", "--scope=cli")
	if out != "docs(cli): update README\n" {
		t.Fatalf("explicit precedence: %q", out)
	}
	execute(2, "config", "--set", `commitFormat="bad"`)
	p, err := config.Read(initial.GlobalPath, false)
	if err != nil || p.CommitFormat != "hybrid" {
		t.Fatalf("invalid update changed profile: %+v %v", p, err)
	}
	project := filepath.Join(cwd, ".gitmojirc.json")
	if err := os.WriteFile(project, []byte(`{}`), 0600); err != nil {
		t.Fatal(err)
	}
	out, _ = execute(0, "prototype", "--type=docs", "--title=use project defaults")
	if out != "docs: use project defaults\n" {
		t.Fatalf("project/global merge: %q", out)
	}
	if err := os.WriteFile(project, []byte(`{"messagePrompt":null}`), 0600); err != nil {
		t.Fatal(err)
	}
	execute(2, "prototype", "--type=docs", "--title=do not commit")
	execute(0, "--help")
	execute(0, "--version")
	if err := os.Remove(project); err != nil {
		t.Fatal(err)
	}
	out, _ = execute(0, "list")
	if !strings.Contains(out, ":sparkles:") || !strings.Contains(out, ":bug:") {
		t.Fatalf("offline list: %q", out)
	}
	search, headers := execute(0, "bug", "linter", "-s")
	if !strings.Contains(search, ":bug:") || !strings.Contains(search, ":rotating_light:") || !strings.Contains(headers, `Search: "bug"`) || !strings.Contains(headers, `Search: "linter"`) {
		t.Fatalf("multiple search: %q / %q", search, headers)
	}
	cache := filepath.Join(home, ".gitmoji", "gitmojis.json")
	if err := os.MkdirAll(filepath.Dir(cache), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cache, []byte(`invalid`), 0600); err != nil {
		t.Fatal(err)
	}
	out, warnings := execute(0, "list")
	if !strings.Contains(out, ":bug:") || !strings.Contains(warnings, "using embedded catalog") {
		t.Fatalf("corrupt cache: %q / %q", out, warnings)
	}
	if err := os.WriteFile(cache, []byte(`[{"code":":new:","emoji":"🆕","name":"new","description":"Add a new entry"}]`), 0600); err != nil {
		t.Fatal(err)
	}
	out, _ = execute(0, "prototype", "--format=emoji", "--emoji=:new:", "--title=add new emoji")
	if out != ":new: add new emoji\n" {
		t.Fatalf("new unclassified emoji: %q", out)
	}
	execute(2, "prototype", "--type=feat", "--emoji=:new:", "--title=reject unclassified hybrid")
	response := `[{"code":":new:","emoji":"🆕","name":"new","description":"Changed catalog content"}]`
	var status atomic.Int32
	status.Store(http.StatusOK)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(int(status.Load()))
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()
	urlJSON, err := json.Marshal(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	execute(0, "config", "--set", "gitmojisUrl="+string(urlJSON))
	execute(0, "update")
	updated, err := os.ReadFile(cache)
	if err != nil || !strings.Contains(string(updated), "Changed catalog content") {
		t.Fatalf("update: %q %v", updated, err)
	}
	status.Store(http.StatusBadGateway)
	execute(1, "update")
	retained, err := os.ReadFile(cache)
	if err != nil || !bytes.Equal(updated, retained) {
		t.Fatal("failed CLI update lost cache")
	}
	legacy := filepath.Join(root, "legacy.json")
	original := []byte(`{"emojiFormat":"code","custom":{"keep":true}}`)
	if err := os.WriteFile(legacy, original, 0600); err != nil {
		t.Fatal(err)
	}
	execute(2, "config", "--import", legacy)
	if err := os.Remove(initial.GlobalPath); err != nil {
		t.Fatal(err)
	}
	execute(0, "config", "--import", legacy)
	p, err = config.Read(initial.GlobalPath, false)
	if err != nil || p.CommitFormat != "emoji" || len(p.Unknown) != 1 {
		t.Fatalf("legacy import: %+v %v", p, err)
	}
	data, err := os.ReadFile(legacy)
	if err != nil || !bytes.Equal(data, original) {
		t.Fatal("import changed source")
	}
}
