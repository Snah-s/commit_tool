package ui

import (
	"bytes"
	"context"
	"errors"
	"image/color"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/Snah-s/commit_tool/internal/catalog"
	"github.com/Snah-s/commit_tool/internal/commit"
	"github.com/Snah-s/commit_tool/internal/config"
)

func testCatalog(t *testing.T) catalog.Catalog {
	t.Helper()
	c, err := catalog.Load()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestPreferencePromptsAndPreview(t *testing.T) {
	c := testCatalog(t)
	original := Draft{Mode: "hybrid", Type: "docs", Emoji: ":memo:", Scope: "api", Description: "update README", Body: "  original\n\nSigned-off-by: Ana\n"}
	opts := Options{Accessible: true, FormatExplicit: true, HideScope: true, HideBody: true, MessageOptions: commit.Options{EmojiFormat: "code"}}
	var output bytes.Buffer
	d, err := Run(context.Background(), strings.NewReader("\n\n\ny\n"), &output, c, original, opts)
	if err != nil || d != original || !strings.Contains(output.String(), "docs(api): :memo: update README") || strings.Contains(output.String(), "Scope (") || strings.Contains(output.String(), "Body (optional)") {
		t.Fatalf("hidden prompts/preview: %+v %v / %q", d, err, output.String())
	}
	opts.HideScope = false
	opts.Scopes = []string{"api", "cli"}
	d, err = Run(context.Background(), strings.NewReader("\n\n1\n\ny\n"), io.Discard, c, original, opts)
	if err != nil || d.Scope != "" || d.Body != original.Body {
		t.Fatalf("optional configured scope: %+v %v", d, err)
	}
	opts.Scopes = []string{}
	output.Reset()
	d, err = Run(context.Background(), strings.NewReader("\n\n1\n\ny\n"), &output, c, original, opts)
	if err != nil || d.Scope != "" || !strings.Contains(output.String(), "Skip scope") {
		t.Fatalf("empty configured scope list accepted a new scope: %+v %v / %q", d, err, output.String())
	}
	opts.MessageOptions = commit.Options{EmojiFormat: "code", CapitalizeTitle: false}
	m := newModel(c, Draft{Mode: "emoji", Emoji: ":memo:", Description: "update README"}, opts)
	if m.message().Title != ":memo: update README" || !strings.Contains(ansi.Strip(m.View().Content), ":memo: update README") {
		t.Fatal("preview differs from preferences")
	}
}

func TestAccessibleConfiguration(t *testing.T) {
	p, err := Configure(context.Background(), strings.NewReader("hybrid\ncode\nn\nn\n[\"api\",\"cli\"]\nn\n\ny\n"), io.Discard, config.Defaults(), true)
	if err != nil || p.CommitFormat != "hybrid" || p.EmojiFormat != "code" || p.CapitalizeTitle || p.MessagePrompt || !p.ScopePrompt || len(p.Scopes) != 2 {
		t.Fatalf("configuration: %+v %v", p, err)
	}
	if _, err := Configure(context.Background(), strings.NewReader(""), io.Discard, config.Defaults(), true); !errors.Is(err, io.EOF) {
		t.Fatalf("config EOF: %v", err)
	}
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
	for _, option := range m.emojiOptions() {
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
	for i, mode := range []string{"standard", "emoji", "hybrid"} {
		t.Run(mode, func(t *testing.T) {
			answers := strconv.Itoa(i+1) + "\n"
			if mode != "emoji" {
				answers += "3\n"
			}
			if mode != "standard" {
				options := (&model{catalog: c, draft: Draft{Mode: mode, Type: "docs"}}).emojiOptions()
				for index, option := range options {
					if option.Value == ":memo:" {
						answers += strconv.Itoa(index+1) + "\n"
						break
					}
				}
			}
			answers += "\nupdate guide\nfirst paragraph\ny\n"
			var output bytes.Buffer
			d, err := Run(context.Background(), strings.NewReader(answers), &output, c, Draft{}, Options{Accessible: true})
			if err != nil {
				t.Fatal(err)
			}
			if d.Body != "first paragraph" || !strings.Contains(output.String(), d.Message(c)) {
				t.Fatalf("body/preview: %+v / %q", d, output.String())
			}
		})
	}
	for _, input := range []string{"", "3\n1\n", "1\n1\n\n"} {
		_, err := Run(context.Background(), strings.NewReader(input), io.Discard, c, Draft{}, Options{Accessible: true})
		if !errors.Is(err, io.EOF) {
			t.Fatalf("EOF (%q): %v", input, err)
		}
	}
	_, err := Run(context.Background(), strings.NewReader("1\n1\n\nupdate guide\n\nn\n"), io.Discard, c, Draft{}, Options{Accessible: true})
	if !errors.Is(err, ErrCanceled) {
		t.Fatalf("accessible cancellation: %v", err)
	}
	original := Draft{Description: "update guide", Body: "  original paragraph\n\nSigned-off-by: Ana\n"}
	d, err := Run(context.Background(), strings.NewReader("\n\n\n\ny\n"), io.Discard, c, original, Options{Accessible: true, HideBody: true})
	if err != nil || d.Body != original.Body || d.Description != original.Description {
		t.Fatalf("defaults and existing body: %+v, %v", d, err)
	}
}

func TestInlineEmojiInitialSelection(t *testing.T) {
	c := testCatalog(t)
	for _, code := range []string{"", ":memo:"} {
		t.Run(code, func(t *testing.T) {
			owner := newModel(c, Draft{Mode: "emoji", Emoji: code}, Options{Accessible: true, InlineSelectors: true})
			selector := owner.inlineSelector(strings.NewReader(""), io.Discard)
			selector.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
			want := code
			if want == "" {
				want = c.Entries[0].Code
			}
			if got, ok := selector.selector.Hovered(); !ok || got != want || selector.step != "emoji" {
				t.Fatalf("initial emoji: got %q, step %q, want %q", got, selector.step, want)
			}
		})
	}
}

func TestInlineSelectorStagesReuseOneCompactList(t *testing.T) {
	owner := newModel(testCatalog(t), Draft{}, Options{Accessible: true, InlineSelectors: true})
	if owner.form != nil || owner.emojis != nil {
		t.Fatal("inline flow constructed an unused full form")
	}
	selector := owner.inlineSelector(strings.NewReader(""), io.Discard)
	selector.form.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if view := ansi.Strip(selector.View().Content); len(strings.Split(view, "\n")) != 4 {
		t.Fatalf("format selector reserved empty rows: %q", view)
	}
	selector.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	selector.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	selector.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if owner.draft.Mode != "hybrid" || selector.step != "type" {
		t.Fatalf("format selection did not advance: mode=%q step=%q", owner.draft.Mode, selector.step)
	}
	selector.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if selector.step != "emoji" {
		t.Fatalf("type selection did not advance to gitmoji: %q", selector.step)
	}
	view := ansi.Strip(selector.View().Content)
	if strings.Contains(view, "Choose an emoji") || !strings.Contains(view, "> ✨ :sparkles:") {
		t.Fatalf("selector did not start with a real compatible emoji: %q", view)
	}
	if got := strings.Count(view, "> "); got != 1 {
		t.Fatalf("gitmoji selector rendered %d cursors: %q", got, view)
	}
	if lines := strings.Split(view, "\n"); len(lines) > 8 {
		t.Fatalf("gitmoji selector used too many rows: %d / %q", len(lines), view)
	}
	for _, background := range []color.Color{color.Black, color.White, color.Black} {
		selector.Update(tea.BackgroundColorMsg{Color: background})
		if got := ansi.Strip(selector.View().Content); got != view {
			t.Fatalf("background change altered the selection: %q", got)
		}
	}
	selector.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !selector.done || owner.draft.Emoji != ":sparkles:" || len(selector.answers) != 3 {
		t.Fatalf("gitmoji selection did not finish: draft=%+v answers=%q", owner.draft, selector.answers)
	}
}

func TestOptionalSingleLineBody(t *testing.T) {
	for _, body := range []string{"", "one paragraph", "  indented body  "} {
		t.Run(body, func(t *testing.T) {
			m := newModel(testCatalog(t), Draft{Mode: "standard", Type: "docs", Body: "old body\n\nSigned-off-by: Ana"}, Options{Accessible: true, InlineSelectors: true, HideScope: true})
			var output bytes.Buffer
			err := m.textFields(&lineReader{reader: strings.NewReader("update guide\n" + body + "\ny\n")}, &output)
			view := output.String()
			if err != nil || m.draft.Body != body || !m.confirmed {
				t.Fatalf("body: %q, confirmed=%v, err=%v", m.draft.Body, m.confirmed, err)
			}
			if strings.Count(view, "Body (optional)") != 1 || strings.Contains(view, "Write/replace") || strings.Contains(view, "/done") {
				t.Fatalf("body asked for extra input: %q", view)
			}
			if body == "" && m.message().Text != "docs: update guide" {
				t.Fatalf("empty input kept an existing body: %q", m.message().Text)
			}
		})
	}
}

func TestConfirmationDescription(t *testing.T) {
	const description = "Stages changes under the current directory and creates a Git commit."
	m := newModel(testCatalog(t), Draft{Mode: "standard", Type: "docs", Description: "update guide"}, Options{Accessible: true, InlineSelectors: true, HideScope: true, HideBody: true, ConfirmDescription: description})
	var output bytes.Buffer
	if err := m.textFields(&lineReader{reader: strings.NewReader("\ny\n")}, &output); err != nil {
		t.Fatal(err)
	}
	text := output.String()
	if strings.Count(text, description) != 1 || strings.Index(text, description) > strings.Index(text, "Finish the message?") {
		t.Fatalf("confirmation omitted its effect: %q", text)
	}
}

func TestEmptyHybridCategory(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalog.json")
	if err := os.WriteFile(path, []byte(`[{"emoji":"🆕","code":":new:","name":"new","description":"New entry."}]`), 0600); err != nil {
		t.Fatal(err)
	}
	c, err := catalog.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	owner := newModel(c, Draft{Mode: "hybrid"}, Options{Accessible: true, InlineSelectors: true, FormatExplicit: true})
	selector := owner.inlineSelector(strings.NewReader(""), io.Discard)
	selector.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if selector.step != "type" || owner.draft.Emoji != "" || !strings.Contains(ansi.Strip(selector.View().Content), "no classified gitmojis") {
		t.Fatalf("empty category reused type options: step=%q draft=%+v view=%q", selector.step, owner.draft, selector.View().Content)
	}
	// A different category with entries can still be chosen.
	owner.catalog.Types["fix"] = []string{":new:"}
	selector.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	selector.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	selector.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if selector.step != "emoji" || owner.draft.Type != "fix" || owner.draft.Emoji != ":new:" {
		t.Fatalf("compatible category could not be chosen: step=%q draft=%+v", selector.step, owner.draft)
	}
	var output bytes.Buffer
	draft, err := Run(context.Background(), strings.NewReader("1\n2\n1\nupdate guide\ny\n"), &output, owner.catalog, Draft{Mode: "hybrid"}, Options{Accessible: true, FormatExplicit: true, HideScope: true, HideBody: true})
	if err != nil || draft.Type != "fix" || draft.Emoji != ":new:" || !strings.Contains(output.String(), "no classified gitmojis") {
		t.Fatalf("numeric empty category: draft=%+v err=%v output=%q", draft, err, output.String())
	}
}

func TestAccessibleInvalidSelectionEOF(t *testing.T) {
	for _, stage := range []struct{ name, prefix string }{{"format", ""}, {"type", "1\n"}, {"emoji", "2\n"}, {"scope", "1\n1\n"}} {
		for _, invalid := range []string{"invalid", "0", "99", "-1"} {
			t.Run(stage.name+"/"+invalid, func(t *testing.T) {
				defer func() {
					if value := recover(); value != nil {
						t.Fatalf("invalid selection followed by EOF panicked: %v", value)
					}
				}()
				m := newModel(testCatalog(t), Draft{}, Options{Accessible: true, Scopes: []string{"api"}})
				err := m.accessibleFields(strings.NewReader(stage.prefix+invalid+"\n"), io.Discard)
				if !errors.Is(err, io.EOF) || m.confirmed {
					t.Fatalf("invalid selection EOF: err=%v confirmed=%v", err, m.confirmed)
				}
			})
		}
	}
}

func TestAccessibleSelectionRecovery(t *testing.T) {
	input := "invalid\n0\n99\n1\ninvalid\n3\n99\n2\nupdate guide\ny\n"
	draft, err := Run(context.Background(), strings.NewReader(input), io.Discard, testCatalog(t), Draft{}, Options{Accessible: true, HideBody: true, Scopes: []string{"api"}})
	if err != nil || draft.Mode != "standard" || draft.Type != "docs" || draft.Scope != "api" || draft.Description != "update guide" {
		t.Fatalf("valid answer after invalid selections: draft=%+v err=%v", draft, err)
	}
}
