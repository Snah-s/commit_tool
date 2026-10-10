package cli

import (
	"reflect"
	"testing"

	"github.com/Snah-s/commit_tool/internal/commit"
)

func TestArguments(t *testing.T) {
	for alias, command := range actions {
		for _, prefix := range []string{"-", "--"} {
			o, err := Parse([]string{prefix + alias})
			if err != nil || o.Command != command {
				t.Fatalf("%s%s: %+v %v", prefix, alias, o, err)
			}
		}
	}
	for _, command := range []string{"commit", "config", "init", "remove", "list", "search", "update"} {
		o, err := Parse([]string{command, "--" + command})
		if err != nil || o.Command != command {
			t.Fatalf("redundant command %s: %v", command, err)
		}
	}
	for _, args := range [][]string{{"search", "bug", "linter"}, {"bug", "-s", "linter"}, {"-s", "--", "commit", "--bug"}} {
		o, err := Parse(args)
		want := []string{"bug", "linter"}
		if args[0] == "-s" {
			want = []string{"commit", "--bug"}
		}
		if err != nil || o.Command != "search" || !reflect.DeepEqual(o.Queries, want) {
			t.Fatalf("queries %v: %+v %v", args, o, err)
		}
	}
	o, err := Parse([]string{"--title", "add `README` $(literal) and ID", "commit", "--type=feat", "--scope", "api", "--message=  body\n\nSigned-off-by: Ana\n", "--format=hybrid", "--emoji=:sparkles:"})
	if err != nil || !o.TitleSet || !o.BodySet || !o.FormatSet || o.Draft.Mode != "hybrid" || o.Draft.Description != "add `README` $(literal) and ID" {
		t.Fatalf("values: %+v %v", o, err)
	}
	o, err = Parse([]string{"commit", "--title=-value", "--title", "-résumé", "--type=feat"})
	if err != nil || o.Draft.Description != "-résumé" {
		t.Fatalf("leading dash/repeated value: %+v %v", o, err)
	}
	o, err = Parse([]string{"hook", "/path with spaces/message", "commit", "abc"})
	if err != nil || o.HookFile != "/path with spaces/message" || o.HookSource != "commit" || o.HookObject != "abc" {
		t.Fatalf("hook: %+v %v", o, err)
	}
	o, err = Parse([]string{"prototype", "--hook", "/path/message", "message"})
	if err != nil || o.Command != "hook" || !o.Prototype {
		t.Fatalf("prototype: %+v %v", o, err)
	}
	for _, args := range [][]string{nil, {"-h"}, {"--help"}, {"commit", "--help"}, {"-v"}, {"--version"}} {
		o, err := Parse(args)
		if err != nil || (!o.Help && !o.Version) {
			t.Fatalf("help/version %v: %+v %v", args, o, err)
		}
	}
	for _, args := range [][]string{{"commit", "--list"}, {"-c", "-g"}, {"-s", "--hook=x"}, {"unknown"}, {"--unknown"}, {"commit", "--title"}, {"commit", "extra"}, {"list", "--type=feat"}, {"hook"}, {"hook", "a", "b", "c", "d"}, {"commit", "--title", "\xff"}, {"-s", "--", "\xff"}} {
		if _, err := Parse(args); err == nil {
			t.Fatalf("accepted %v", args)
		}
	}
	o, err = Parse([]string{"config", "--set", `commitFormat="hybrid"`, "--set", `scopePrompt=["api"]`, "--accessible"})
	if err != nil || len(o.ConfigSet) != 2 || !o.Accessible {
		t.Fatalf("config args: %+v %v", o, err)
	}
	for _, args := range [][]string{{"commit", "--show"}, {"list", "--import=profile"}, {"config", "--import="}, {"config", "--show", "--set=autoAdd=true"}, {"config", "--set=autoAdd=true", "--import=profile"}} {
		if _, err := Parse(args); err == nil {
			t.Fatalf("accepted config action conflict %v", args)
		}
	}
}

func TestExplicitDefaults(t *testing.T) {
	initial := commit.Draft{Mode: "emoji", Emoji: ":memo:", Scope: "api", Description: "original title", Body: "  paragraph\n\nSigned-off-by: Ana\n"}
	o, err := Parse([]string{"commit", "--format=hybrid", "--type=docs"})
	if err != nil {
		t.Fatal(err)
	}
	d := o.Defaults(initial)
	if d.Mode != "hybrid" || d.Type != "docs" || d.Emoji != initial.Emoji || d.Body != initial.Body || d.Description != initial.Description {
		t.Fatalf("defaults: %+v", d)
	}
	o, err = Parse([]string{"commit", "--title=", "--scope=", "--message="})
	if err != nil {
		t.Fatal(err)
	}
	d = o.Defaults(initial)
	if d.Mode != "standard" || d.Emoji != "" || d.Scope != "" || d.Description != "" || d.Body != "" {
		t.Fatalf("explicit empty values: %+v", d)
	}
}
