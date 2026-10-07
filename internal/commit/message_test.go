package commit

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Snah-s/commit_tool/internal/catalog"
)

func TestMessageFormatsAndParse(t *testing.T) {
	c, err := catalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	body := "  paragraph with `quotes` and $(literal)  \n\n\tBREAKING CHANGE: preserve ID\nSigned-off-by: Ana\n"
	for _, test := range []struct{ mode, kind, code, scope, format, title string }{
		{"standard", "feat", "", "users", "emoji", "feat(users): add README and ID"},
		{"standard", "docs", "", "", "code", "docs: add README and ID"},
		{"emoji", "", ":sparkles:", "users", "emoji", "✨ (users): add README and ID"},
		{"emoji", "", ":memo:", "", "code", ":memo: add README and ID"},
		{"hybrid", "feat", ":sparkles:", "users", "emoji", "feat(users): ✨ add README and ID"},
		{"hybrid", "docs", ":memo:", "", "code", "docs: :memo: add README and ID"},
	} {
		t.Run(test.title, func(t *testing.T) {
			d := Draft{Mode: test.mode, Type: test.kind, Emoji: test.code, Scope: test.scope, Description: "add README and ID", Body: body}
			m, err := Build(d, c, Options{EmojiFormat: test.format})
			if err != nil {
				t.Fatal(err)
			}
			if m.Title != test.title || m.Text != test.title+"\n\n"+body || m.TitleLength != utf8.RuneCountInString(test.title) {
				t.Fatalf("message: %+v", m)
			}
			parsed, err := Parse(m.Text, c)
			if err != nil || parsed != d {
				t.Fatalf("roundtrip: %+v, %v", parsed, err)
			}
		})
	}
	for _, kind := range Types {
		if _, err := Build(Draft{Mode: "standard", Type: kind, Description: "fix search"}, c, DefaultOptions()); err != nil {
			t.Fatal(err)
		}
	}
	for _, entry := range c.Entries {
		for _, format := range []string{"emoji", "code"} {
			d := Draft{Mode: "emoji", Emoji: entry.Code, Description: "Preserve catalog", Body: body}
			m, err := Build(d, c, Options{EmojiFormat: format})
			if err != nil {
				t.Fatal(err)
			}
			parsed, err := Parse(m.Text, c)
			if err != nil || parsed != d {
				t.Fatalf("emoji %s/%s: %+v %v", entry.Code, format, parsed, err)
			}
		}
	}
	for _, mode := range []string{"emoji", "hybrid"} {
		d := Draft{Mode: mode, Emoji: ":sparkles:", Description: "éclair with README"}
		if mode == "hybrid" {
			d.Type = "feat"
		}
		m, err := Build(d, c, DefaultOptions())
		if err != nil {
			t.Fatal(err)
		}
		if (mode == "emoji" && m.Title != "✨ Éclair with README") || (mode == "hybrid" && m.Title != "feat: ✨ éclair with README") {
			t.Fatalf("capitalization: %q", m.Title)
		}
	}
	for _, n := range []int{72, 73} {
		m, err := Build(Draft{Mode: "standard", Type: "feat", Description: strings.Repeat("á", n-6)}, c, DefaultOptions())
		if err != nil || m.TitleLength != n || m.LongTitle != (n > 72) {
			t.Fatalf("counter: %+v, %v", m, err)
		}
	}
}

func TestMessageValidationAndRecovery(t *testing.T) {
	c, err := catalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	base := Draft{Mode: "hybrid", Type: "feat", Emoji: ":sparkles:", Description: "add résumé search"}
	for name, change := range map[string]func(*Draft){
		"mode":               func(d *Draft) { d.Mode = "other" },
		"type":               func(d *Draft) { d.Type = "ci" },
		"type missing":       func(d *Draft) { d.Type = "" },
		"emoji missing":      func(d *Draft) { d.Emoji = "" },
		"emoji unknown":      func(d *Draft) { d.Emoji = ":unknown:" },
		"emoji incompatible": func(d *Draft) { d.Emoji = ":memo:" },
		"emoji unclassified": func(d *Draft) { d.Emoji = ":beers:" },
		"standard emoji":     func(d *Draft) { d.Mode = "standard" },
		"emoji type":         func(d *Draft) { d.Mode = "emoji" },
		"empty":              func(d *Draft) { d.Description = "  " },
		"period":             func(d *Draft) { d.Description = "text.  " },
		"newline":            func(d *Draft) { d.Description = "a\nb" },
		"control":            func(d *Draft) { d.Description = "a\x1bb" },
		"UTF8":               func(d *Draft) { d.Description = "\xff" },
		"scope case":         func(d *Draft) { d.Scope = "API" },
		"scope whitespace":   func(d *Draft) { d.Scope = "two modules" },
		"scope delimiter":    func(d *Draft) { d.Scope = "a):b" },
		"body NUL":           func(d *Draft) { d.Body = "a\x00b" },
		"body UTF8":          func(d *Draft) { d.Body = "\xff" },
	} {
		t.Run(name, func(t *testing.T) {
			d := base
			change(&d)
			if _, err := Build(d, c, DefaultOptions()); err == nil {
				t.Fatalf("accepted %+v", d)
			}
		})
	}
	if _, err := Build(base, c, Options{EmojiFormat: "other"}); err == nil {
		t.Fatal("accepted invalid emojiFormat")
	}
	for _, text := range []string{"free-form description", "change ✨ in the middle", ":unknown: keep literal", "feat!: preserve title", "feat(): preserve"} {
		d, err := Parse(text+"\n\n  body\n\nSigned-off-by: Ana\n", c)
		if err != nil || d.Mode != "" || d.Description != text || d.Body != "  body\n\nSigned-off-by: Ana\n" {
			t.Fatalf("recovery: %+v, %v", d, err)
		}
	}
	d, err := Parse("fix(api): 🐛 preserve ID\r\n\r\n  body\r\n\r\nBREAKING CHANGE: literal\r\n", c)
	if err != nil || d.Description != "preserve ID" || d.Emoji != ":bug:" || d.Body != "  body\r\n\r\nBREAKING CHANGE: literal\r\n" {
		t.Fatalf("CRLF: %+v, %v", d, err)
	}
	for _, text := range []string{"a\x00b", "\xff", "feat: a\n\nb\x1b"} {
		if _, err := Parse(text, c); err == nil {
			t.Fatalf("accepted %q", text)
		}
	}
}
