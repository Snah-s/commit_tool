package ui

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Snah-s/commit_tool/internal/catalog"
)

func testCatalog(t *testing.T) catalog.Catalog {
	t.Helper()
	c, err := catalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPreviewAndTypeChange(t *testing.T) {
	c := testCatalog(t)
	d := Draft{Mode: "hybrid", Type: "feat", Emoji: ":sparkles:", Scope: "users", Description: "add résumé search", Body: "first paragraph\n\nSigned-off-by: Ana"}
	if got := d.Message(c); got != "feat(users): ✨ add résumé search\n\nfirst paragraph\n\nSigned-off-by: Ana" {
		t.Fatalf("preview: %q", got)
	}
	m := newModel(c, d, Options{})
	m.form.NextGroup()
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if m.draft.Type != "fix" {
		t.Fatalf("keyboard did not change the type: %q", m.draft.Type)
	}
	if m.draft.Emoji != "" || m.draft.Body != d.Body || m.draft.Description != d.Description || m.draft.Scope != d.Scope {
		t.Fatalf("type change discarded data or selected an emoji: %+v", m.draft)
	}
	for _, option := range m.emojiOptions()[1:] {
		if !c.Compatible("fix", option.Value) {
			t.Fatalf("incompatible option: %q", option.Value)
		}
	}
	for _, mode := range []string{"standard", "emoji", "hybrid"} {
		d.Mode = mode
		title := d.Title(c)
		if mode == "standard" && strings.Contains(title, "✨") {
			t.Fatal("standard contains an emoji")
		}
		if mode == "emoji" && title != "✨ (users): Add résumé search" {
			t.Fatalf("emoji: %q", title)
		}
	}
}

func TestEmojiSearchAndEscape(t *testing.T) {
	m := newModel(testCatalog(t), Draft{Mode: "hybrid", Type: "feat"}, Options{})
	m.form.NextGroup()
	m.form.NextGroup()
	m.Update(tea.KeyPressMsg{Code: '/', Text: "/"})
	m.Update(tea.KeyPressMsg{Code: 's', Text: "sparkles"})
	if !m.emojis.GetFiltering() || !strings.Contains(ansi.Strip(m.form.View()), ":sparkles:") {
		t.Fatal("search does not show the emoji")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEscape})
	if m.form.State != huh.StateNormal || m.emojis.GetFiltering() {
		t.Fatal("Escape from search canceled the form")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if m.draft.Emoji != ":sparkles:" {
		t.Fatalf("search selection: %q", m.draft.Emoji)
	}
}

func TestSmallTerminalAndUnicodeCounter(t *testing.T) {
	m := newModel(testCatalog(t), Draft{Description: strings.Repeat("á", 80)}, Options{})
	m.Update(tea.WindowSizeMsg{Width: 40, Height: 15})
	view := ansi.Strip(m.View().Content)
	if got := utf8.RuneCountInString(m.draft.Title(m.catalog)); got != 86 {
		t.Fatalf("counter: %d", got)
	}
	if m.View().AltScreen || !strings.Contains(view, "86/72") {
		t.Fatalf("alternate screen or incorrect counter: %q", view)
	}
	if got := len(strings.Split(view, "\n")); got > 15 {
		t.Fatalf("view exceeds terminal: %d lines", got)
	}
	for _, line := range strings.Split(view, "\n") {
		if ansi.StringWidth(line) > 40 {
			t.Fatalf("line overflows: %q", line)
		}
	}
}

func TestAccessibleModesAndEOF(t *testing.T) {
	c := testCatalog(t)
	for _, mode := range []string{"standard", "emoji", "hybrid"} {
		t.Run(mode, func(t *testing.T) {
			answers := mode + "\n"
			if mode != "emoji" {
				answers += "docs\n"
			}
			if mode != "standard" {
				answers += ":memo:\n"
			}
			answers += "\nupdate guide\ny\nfirst paragraph\n\n  indented block  \nSigned-off-by: Ana\n/done\ny\n"
			var output bytes.Buffer
			d, err := Run(context.Background(), strings.NewReader(answers), &output, c, Draft{}, Options{Accessible: true})
			if err != nil {
				t.Fatal(err)
			}
			if d.Body != "first paragraph\n\n  indented block  \nSigned-off-by: Ana" || !strings.Contains(output.String(), d.Message(c)) {
				t.Fatalf("body/preview: %+v / %q", d, output.String())
			}
		})
	}
	for _, input := range []string{"", "hybrid\nfeat\n", "standard\nfeat\n\n"} {
		_, err := Run(context.Background(), strings.NewReader(input), io.Discard, c, Draft{}, Options{Accessible: true})
		if !errors.Is(err, io.EOF) {
			t.Fatalf("EOF (%q): %v", input, err)
		}
	}
	_, err := Run(context.Background(), strings.NewReader("standard\nfeat\n\nupdate guide\nn\nn\n"), io.Discard, c, Draft{}, Options{Accessible: true})
	if !errors.Is(err, ErrCanceled) {
		t.Fatalf("accessible cancellation: %v", err)
	}
	original := Draft{Description: "update guide", Body: "  original paragraph\n\nSigned-off-by: Ana\n"}
	d, err := Run(context.Background(), strings.NewReader("\n\n\n\nn\ny\n"), io.Discard, c, original, Options{Accessible: true})
	if err != nil || d.Body != original.Body || d.Description != original.Description {
		t.Fatalf("defaults and existing body: %+v, %v", d, err)
	}
}
