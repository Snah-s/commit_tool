package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/cancelreader"

	"github.com/Snah-s/commit_tool/internal/catalog"
	"github.com/Snah-s/commit_tool/internal/commit"
)

var ErrCanceled = huh.ErrUserAborted

type model struct {
	draft     Draft
	catalog   catalog.Catalog
	form      *huh.Form
	emojis    *huh.Select[string]
	preview   viewport.Model
	confirmed bool
	width     int
	height    int
	fixedMode bool
}

type Options struct{ Accessible, FormatExplicit bool }

func newModel(c catalog.Catalog, draft Draft, options Options) *model {
	if draft.Mode == "" {
		draft.Mode = "standard"
	}
	if draft.Type == "" {
		draft.Type = "feat"
	}
	reconcile(&draft, c)
	m := &model{draft: draft, catalog: c, preview: viewport.New(), width: 80, height: 24, fixedMode: options.FormatExplicit}
	m.emojis = huh.NewSelect[string]().Title("Emoji (/ to search)").Value(&m.draft.Emoji).
		Options(m.emojiOptions()...).Height(5).Validate(func(code string) error {
		_, ok := c.Find(code)
		if !ok || (m.draft.Mode == "hybrid" && !c.Compatible(m.draft.Type, code)) {
			return errors.New("choose a compatible emoji; it is not selected automatically")
		}
		return nil
	})
	groups := []*huh.Group{
		huh.NewGroup(m.emojis).WithHideFunc(func() bool { return m.draft.Mode == "standard" }),
		huh.NewGroup(huh.NewInput().Title("Scope (optional)").Value(&m.draft.Scope).Validate(commit.ValidateScope)),
		huh.NewGroup(huh.NewInput().Title("Description").Value(&m.draft.Description).Validate(commit.ValidateDescription)),
		huh.NewGroup(huh.NewText().Title("Body (optional)").Value(&m.draft.Body).CharLimit(0).ExternalEditor(false).Validate(commit.ValidateBody)),
		huh.NewGroup(huh.NewConfirm().Title("Finish the message?").Description("Returns the message only; does not create a commit.").
			Affirmative("Finish").Negative("Cancel").Value(&m.confirmed)),
	}
	if !m.fixedMode || m.draft.Mode != "emoji" {
		kind := huh.NewGroup(huh.NewSelect[string]().Title("Type").Value(&m.draft.Type).
			Options(huh.NewOptions(commit.Types...)...)).WithHideFunc(func() bool { return m.draft.Mode == "emoji" })
		groups = append([]*huh.Group{kind}, groups...)
	}
	if !m.fixedMode {
		mode := huh.NewGroup(huh.NewSelect[string]().Title("Format").Value(&m.draft.Mode).
			Options(huh.NewOptions("standard", "emoji", "hybrid")...))
		groups = append([]*huh.Group{mode}, groups...)
	}
	m.form = huh.NewForm(groups...)
	m.form.SubmitCmd = tea.Quit
	m.form.CancelCmd = tea.Quit
	m.resize(80, 24)
	return m
}

func (m *model) emojiOptions() []huh.Option[string] {
	options := []huh.Option[string]{huh.NewOption("Choose an emoji…", "")}
	for _, entry := range m.catalog.Entries {
		if m.draft.Mode != "hybrid" || m.catalog.Compatible(m.draft.Type, entry.Code) {
			options = append(options, huh.NewOption(entry.Emoji+" "+entry.Code+" "+entry.Description, entry.Code))
		}
	}
	return options
}

func (m *model) Init() tea.Cmd { return m.form.Init() }

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := msg.(tea.WindowSizeMsg); ok {
		m.resize(size.Width, size.Height)
	}
	if press, ok := msg.(tea.KeyPressMsg); ok {
		switch press.String() {
		case "esc":
			if field, ok := m.form.GetFocusedField().(*huh.Select[string]); !ok || !field.GetFiltering() {
				m.form.State = huh.StateAborted
				return m, tea.Quit
			}
		case "pgup":
			m.preview.ScrollUp(m.preview.Height())
			return m, nil
		case "pgdown":
			m.preview.ScrollDown(m.preview.Height())
			return m, nil
		}
	}
	beforeMode, beforeType := m.draft.Mode, m.draft.Type
	_, cmd := m.form.Update(msg)
	if beforeMode != m.draft.Mode || beforeType != m.draft.Type {
		reconcile(&m.draft, m.catalog)
		m.emojis.Options(m.emojiOptions()...)
	}
	m.updatePreview()
	return m, cmd
}

func (m *model) resize(width, height int) {
	m.width, m.height = max(12, width), max(8, height)
	previewHeight := max(1, min(5, (m.height-4)/3))
	m.preview.SetWidth(m.width)
	m.preview.SetHeight(previewHeight)
	m.form.WithWidth(m.width).WithHeight(m.height - previewHeight - 3)
	m.updatePreview()
}

func (m *model) updatePreview() {
	m.preview.SetContent(ansi.Wrap(m.draft.Message(m.catalog), m.width, ""))
}

func (m *model) View() tea.View {
	if m.form.State != huh.StateNormal {
		return tea.NewView("")
	}
	content := m.form.View() + "\n" + ansi.Truncate(m.previewStatus(), m.width, "…") + "\n" + m.preview.View() +
		"\n" + ansi.Truncate("PgUp/PgDn preview · Esc cancels (or closes search)", m.width, "…")
	return tea.NewView(content)
}

func (m *model) previewStatus() string {
	count := utf8.RuneCountInString(m.draft.Title(m.catalog))
	if count > 72 {
		return fmt.Sprintf("Preview · %d/72 · warning: long title", count)
	}
	return fmt.Sprintf("Preview · %d/72 Unicode code points", count)
}

func Run(ctx context.Context, input io.Reader, output io.Writer, c catalog.Catalog, draft Draft, options Options) (Draft, error) {
	m := newModel(c, draft, options)
	if options.Accessible {
		return m.runAccessible(ctx, input, output)
	}
	_, err := tea.NewProgram(m, tea.WithInput(input), tea.WithOutput(output), tea.WithContext(ctx)).Run()
	if ctx.Err() != nil || errors.Is(err, tea.ErrInterrupted) || m.form.State == huh.StateAborted {
		return Draft{}, ErrCanceled
	}
	if err != nil {
		return Draft{}, err
	}
	if !m.confirmed {
		return Draft{}, ErrCanceled
	}
	return m.draft.Active(), nil
}

// Huh creates a scanner per field: limiting reads avoids consuming the next
// answer and detects EOF that its accessible form does not propagate.
type lineReader struct {
	reader   io.Reader
	err      error
	line     strings.Builder
	lastLine string
}

func (r *lineReader) Read(p []byte) (int, error) {
	if len(p) > 1 {
		p = p[:1]
	}
	n, err := r.reader.Read(p)
	if n > 0 {
		if p[0] == '\n' {
			r.lastLine = strings.TrimSuffix(r.line.String(), "\r")
			r.line.Reset()
		} else {
			r.line.WriteByte(p[0])
		}
	}
	if err != nil {
		r.err = err
	}
	return n, err
}

func (m *model) runAccessible(ctx context.Context, input io.Reader, output io.Writer) (Draft, error) {
	reader, err := cancelreader.NewReader(input)
	if err != nil {
		return Draft{}, err
	}
	defer reader.Close()
	result := make(chan error, 1)
	go func() { result <- m.accessibleFields(reader, output) }()
	select {
	case <-ctx.Done():
		reader.Cancel()
		<-result
		return Draft{}, ErrCanceled
	case err := <-result:
		if err != nil {
			return Draft{}, err
		}
		return m.draft.Active(), nil
	}
}

func (m *model) accessibleFields(input io.Reader, output io.Writer) error {
	r := &lineReader{reader: input}
	run := func(field huh.Field) error {
		form := huh.NewForm(huh.NewGroup(field)).WithAccessible(true).WithInput(r).WithOutput(output)
		if err := form.Run(); err != nil {
			return err
		}
		return r.err
	}
	// Validated inputs avoid an invalid index in Select.RunAccessible on EOF.
	if !m.fixedMode {
		if err := run(huh.NewInput().Title("Format (standard/emoji/hybrid)").Value(&m.draft.Mode).Validate(func(v string) error {
			if v == "" {
				v = m.draft.Mode
			}
			if v != "standard" && v != "emoji" && v != "hybrid" {
				return errors.New("choose standard, emoji, or hybrid")
			}
			return nil
		})); err != nil {
			return err
		}
	}
	if m.draft.Mode != "emoji" {
		if err := run(huh.NewInput().Title("Type (feat/fix/docs/refactor/test/chore)").Value(&m.draft.Type).Validate(func(v string) error {
			if v == "" {
				v = m.draft.Type
			}
			if !slices.Contains(commit.Types, v) {
				return errors.New("choose one of the six types")
			}
			return nil
		})); err != nil {
			return err
		}
	}
	reconcile(&m.draft, m.catalog)
	if m.draft.Mode != "standard" {
		for _, option := range m.emojiOptions()[1:] {
			fmt.Fprintln(output, option.Key)
		}
		if err := run(huh.NewInput().Title("Emoji (code :name:)").Value(&m.draft.Emoji).Validate(func(v string) error {
			if v == "" {
				v = m.draft.Emoji
			}
			_, ok := m.catalog.Find(v)
			if !ok || (m.draft.Mode == "hybrid" && !m.catalog.Compatible(m.draft.Type, v)) {
				return errors.New("choose a code from the list")
			}
			return nil
		})); err != nil {
			return err
		}
	}
	for _, field := range []huh.Field{
		huh.NewInput().Title("Scope (optional)").Value(&m.draft.Scope).Validate(commit.ValidateScope),
		huh.NewInput().Title("Description").Value(&m.draft.Description).Validate(func(v string) error {
			if v == "" {
				v = m.draft.Description
			}
			return commit.ValidateDescription(v)
		}),
	} {
		if err := run(field); err != nil {
			return err
		}
	}
	editBody := false
	if err := run(huh.NewConfirm().Title("Write/replace the body?").Value(&editBody)); err != nil {
		return err
	}
	if editBody {
		var lines []string
		for {
			var line string
			if err := run(huh.NewInput().Title("Body (line; /done ends input)").Value(&line)); err != nil {
				return err
			}
			// PromptString trims whitespace; the body needs the literal line.
			line = r.lastLine
			if line == "/done" {
				break
			}
			lines = append(lines, line)
		}
		m.draft.Body = strings.Join(lines, "\n")
	}
	if err := commit.ValidateBody(m.draft.Body); err != nil {
		return err
	}
	fmt.Fprintf(output, "%s\n%s\n", m.previewStatus(), m.draft.Message(m.catalog))
	if err := run(huh.NewConfirm().Title("Finish the message? (returns message only)").Value(&m.confirmed)); err != nil {
		return err
	}
	if !m.confirmed {
		return ErrCanceled
	}
	return nil
}
