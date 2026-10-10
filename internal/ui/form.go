package ui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

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
	options   Options
}

type Options struct {
	Accessible, FormatExplicit bool
	InlineSelectors            bool
	HideScope, HideBody        bool
	Scopes                     []string
	MessageOptions             commit.Options
	ConfirmDescription         string
}

func newModel(c catalog.Catalog, draft Draft, options Options) *model {
	if draft.Mode == "" {
		draft.Mode = "standard"
	}
	if draft.Type == "" {
		draft.Type = "feat"
	}
	if options.MessageOptions.EmojiFormat == "" {
		options.MessageOptions = commit.DefaultOptions()
	}
	if options.ConfirmDescription == "" {
		options.ConfirmDescription = "Returns the message only; does not create a commit."
	}
	reconcile(&draft, c)
	m := &model{draft: draft, catalog: c, width: 80, height: 24, fixedMode: options.FormatExplicit, options: options}
	if options.Accessible && options.InlineSelectors {
		return m
	}
	m.preview = viewport.New()
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
	}
	if !options.HideScope {
		var scope huh.Field = huh.NewInput().Title("Scope (optional)").Value(&m.draft.Scope).Validate(commit.ValidateScope)
		if options.Scopes != nil {
			choices := []huh.Option[string]{huh.NewOption("Skip scope", "")}
			for _, value := range m.scopeChoices() {
				choices = append(choices, huh.NewOption(value, value))
			}
			scope = huh.NewSelect[string]().Title("Scope (optional)").Value(&m.draft.Scope).Options(choices...)
		}
		groups = append(groups, huh.NewGroup(scope))
	}
	groups = append(groups, huh.NewGroup(huh.NewInput().Title("Description").Value(&m.draft.Description).Validate(commit.ValidateDescription)))
	if !options.HideBody {
		groups = append(groups, huh.NewGroup(huh.NewText().Title("Body (optional)").Value(&m.draft.Body).CharLimit(0).ExternalEditor(false).Validate(commit.ValidateBody)))
	}
	groups = append(groups, huh.NewGroup(huh.NewConfirm().Title("Finish the message?").Description(options.ConfirmDescription).
		Affirmative("Finish").Negative("Cancel").Value(&m.confirmed)))
	if !m.fixedMode || m.draft.Mode != "emoji" {
		kind := huh.NewGroup(huh.NewSelect[string]().Title("Type").Value(&m.draft.Type).
			Options(huh.NewOptions(commit.Types...)...).Validate(m.validateType)).WithHideFunc(func() bool { return m.draft.Mode == "emoji" })
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
	var options []huh.Option[string]
	for _, entry := range m.catalog.Entries {
		if m.draft.Mode != "hybrid" || m.catalog.Compatible(m.draft.Type, entry.Code) {
			options = append(options, huh.NewOption(entry.Emoji+" "+entry.Code+" "+entry.Description, entry.Code))
		}
	}
	return options
}

func (m *model) validateType(kind string) error {
	if !slices.Contains(commit.Types, kind) {
		return errors.New("choose one of the listed types")
	}
	if m.draft.Mode == "hybrid" && len(m.catalog.Types[kind]) == 0 {
		return fmt.Errorf("%s has no classified gitmojis in this catalog; choose another type or use standard/emoji mode", kind)
	}
	return nil
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
		// Rebuilding Huh's options must not choose an emoji on a type change.
		code := m.draft.Emoji
		m.emojis.Options(m.emojiOptions()...)
		m.draft.Emoji = code
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
	m.preview.SetContent(ansi.Wrap(m.message().Text, m.width, ""))
}

func (m *model) message() commit.Message {
	return commit.Preview(m.draft, m.catalog, m.options.MessageOptions)
}

func (m *model) scopeChoices() []string {
	choices := slices.Clone(m.options.Scopes)
	if m.draft.Scope != "" && !slices.Contains(choices, m.draft.Scope) {
		choices = append(choices, m.draft.Scope)
	}
	return choices
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
	count := m.message().TitleLength
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
	err := runProgram(ctx, m, input, output)
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
	var err error
	if m.options.InlineSelectors {
		err = m.inlineFields(ctx, input, output)
	} else {
		err = runAccessible(ctx, input, func(reader io.Reader) error { return m.accessibleFields(reader, output) })
	}
	if err != nil {
		return Draft{}, err
	}
	return m.draft.Active(), nil
}

func runAccessible(ctx context.Context, input io.Reader, run func(io.Reader) error) error {
	reader, err := cancelreader.NewReader(input)
	if err != nil {
		return err
	}
	defer reader.Close()
	result := make(chan error, 1)
	go func() { result <- run(reader) }()
	select {
	case <-ctx.Done():
		reader.Cancel()
		<-result
		return ErrCanceled
	case err := <-result:
		return err
	}
}

func accessibleField(reader *lineReader, output io.Writer, field huh.Field) error {
	form := huh.NewForm(huh.NewGroup(field)).WithAccessible(true).WithInput(reader).WithOutput(output)
	if err := form.Run(); err != nil {
		return err
	}
	return reader.err
}

type inlineSelectorModel struct {
	form     *huh.Form
	owner    *model
	selector *huh.Select[string]
	step     string
	answers  []string
	done     bool
}

func (m *inlineSelectorModel) Init() tea.Cmd { return m.form.Init() }

func (m *inlineSelectorModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if press, ok := msg.(tea.KeyPressMsg); ok && press.String() == "esc" {
		if !m.selector.GetFiltering() {
			m.form.State = huh.StateAborted
			return m, tea.Quit
		}
	}
	if press, ok := msg.(tea.KeyPressMsg); ok && press.String() == "enter" {
		_, _ = m.selector.Update(msg)
		if m.selector.Error() == nil {
			return m, m.choose()
		}
		return m, nil
	}
	_, cmd := m.form.Update(msg)
	if _, ok := msg.(tea.KeyPressMsg); ok && m.step == "emoji" {
		// shortcut: repaint the inline list until incremental rendering handles differing emoji widths.
		return m, tea.Batch(cmd, tea.ClearScreen)
	}
	return m, cmd
}

func (m *inlineSelectorModel) View() tea.View {
	if m.done || m.form.State != huh.StateNormal {
		return tea.NewView("")
	}
	content := strings.Join(m.answers, "\n")
	if content != "" {
		content += "\n"
	}
	content += m.selector.View()
	if err := m.selector.Error(); err != nil {
		content += "\n" + err.Error()
	}
	view := tea.NewView(content)
	view.ReportFocus = true
	return view
}

func (m *inlineSelectorModel) answer() string {
	switch m.step {
	case "format":
		return "? Choose a format: " + m.owner.draft.Mode
	case "type":
		return "? Choose a commit type: " + m.owner.draft.Type
	case "emoji":
		if entry, ok := m.owner.catalog.Find(m.owner.draft.Emoji); ok {
			return "? Choose a gitmoji: " + entry.Emoji + " - " + entry.Description
		}
	case "scope":
		if m.owner.draft.Scope == "" {
			return "? Choose a scope (optional): Skip scope"
		}
		return "? Choose a scope (optional): " + m.owner.draft.Scope
	}
	return ""
}

func (m *model) inlineSelector(input io.Reader, output io.Writer) *inlineSelectorModel {
	selector := &inlineSelectorModel{owner: m}
	// Huh otherwise rebuilds the theme for each option on every render.
	light, dark := huh.ThemeCharm(false), huh.ThemeCharm(true)
	theme := huh.ThemeFunc(func(isDark bool) *huh.Styles {
		if isDark {
			return dark
		}
		return light
	})
	selector.selector = huh.NewSelect[string]()
	selector.selector.WithTheme(theme)
	selector.selector.Validate(selector.validate)
	selector.form = huh.NewForm(huh.NewGroup(selector.selector)).WithInput(input).WithOutput(output).
		WithShowHelp(false).WithTheme(theme)
	selector.form.SubmitCmd, selector.form.CancelCmd = tea.Quit, tea.Quit
	selector.configure(selector.nextStep(""))
	return selector
}

func (m *inlineSelectorModel) validate(value string) error {
	switch m.step {
	case "format":
		if slices.Contains([]string{"standard", "emoji", "hybrid"}, value) {
			return nil
		}
	case "type":
		return m.owner.validateType(value)
	case "emoji":
		if _, ok := m.owner.catalog.Find(value); ok &&
			(m.owner.draft.Mode != "hybrid" || m.owner.catalog.Compatible(m.owner.draft.Type, value)) {
			return nil
		}
		return errors.New("choose a compatible gitmoji")
	case "scope":
		if slices.Contains(m.owner.scopeChoices(), value) || value == "" {
			return commit.ValidateScope(value)
		}
	}
	return errors.New("choose one of the listed options")
}

func (m *inlineSelectorModel) nextStep(after string) string {
	switch after {
	case "":
		if !m.owner.fixedMode {
			return "format"
		}
		return m.modeStep()
	case "format":
		return m.modeStep()
	case "type":
		if m.owner.draft.Mode == "hybrid" {
			return "emoji"
		}
		return m.scopeStep()
	case "emoji":
		return m.scopeStep()
	case "scope":
		return ""
	default:
		return ""
	}
}

func (m *inlineSelectorModel) modeStep() string {
	if m.owner.draft.Mode != "emoji" {
		return "type"
	}
	return "emoji"
}

func (m *inlineSelectorModel) scopeStep() string {
	if !m.owner.options.HideScope && m.owner.options.Scopes != nil {
		return "scope"
	}
	return ""
}

func (m *inlineSelectorModel) configure(step string) {
	if step == "" {
		m.done = true
		return
	}
	m.step = step
	var title string
	var options []huh.Option[string]
	var value *string
	switch step {
	case "format":
		title, options, value = "Choose a format", huh.NewOptions("standard", "emoji", "hybrid"), &m.owner.draft.Mode
	case "type":
		title, options, value = "Choose a commit type", huh.NewOptions(commit.Types...), &m.owner.draft.Type
	case "emoji":
		title, options, value = "Choose a gitmoji", m.owner.emojiOptions(), &m.owner.draft.Emoji
	case "scope":
		title, value = "Choose a scope (optional)", &m.owner.draft.Scope
		options = append(options, huh.NewOption("Skip scope", ""))
		for _, scope := range m.owner.scopeChoices() {
			options = append(options, huh.NewOption(scope, scope))
		}
	}
	if len(options) > 0 {
		options[0] = options[0].Selected(true)
	}
	var temporary string
	m.selector.Value(&temporary).Title(title).Options(options...).Height(1 + min(5, len(options)))
	m.selector.Value(value).Validate(m.validate).Focus()
}

func (m *inlineSelectorModel) choose() tea.Cmd {
	if answer := m.answer(); answer != "" {
		m.answers = append(m.answers, answer)
	}
	m.selector.Blur()
	if m.step == "format" || m.step == "type" {
		reconcile(&m.owner.draft, m.owner.catalog)
	}
	next := m.nextStep(m.step)
	if next == "" {
		m.done = true
		return tea.Quit
	}
	m.configure(next)
	return nil
}

func (m *model) accessibleFields(input io.Reader, output io.Writer) error {
	r := &lineReader{reader: input}
	if err := m.selectionFields(r, output); err != nil {
		return err
	}
	return m.textFields(r, output)
}

func (m *model) inlineFields(ctx context.Context, input io.Reader, output io.Writer) error {
	selector := m.inlineSelector(input, output)
	err := runProgram(ctx, selector, input, output)
	if ctx.Err() != nil || errors.Is(err, tea.ErrInterrupted) || selector.form.State == huh.StateAborted {
		return ErrCanceled
	}
	if err != nil {
		return err
	}
	if len(selector.answers) > 0 {
		if _, err := fmt.Fprintln(output, strings.Join(selector.answers, "\n")); err != nil {
			return err
		}
	}
	return runAccessible(ctx, input, func(reader io.Reader) error {
		return m.textFields(&lineReader{reader: reader}, output)
	})
}

// Huh's numeric select indexes its options before checking read errors.
func accessibleSelect(r *lineReader, output io.Writer, title string, value *string, options []huh.Option[string], validate func(string) error) error {
	if len(options) == 0 {
		return errors.New("no options available for " + title)
	}
	if _, err := fmt.Fprintln(output, title); err != nil {
		return err
	}
	choice := "1"
	for i, option := range options {
		if _, err := fmt.Fprintf(output, "%d. %s\n", i+1, option.Key); err != nil {
			return err
		}
		if option.Value == *value {
			choice = strconv.Itoa(i + 1)
		}
	}
	field := huh.NewInput().Title(fmt.Sprintf("Enter a number between 1 and %d:", len(options))).Value(&choice).Validate(func(input string) error {
		if input == "" {
			input = choice
		}
		index, err := strconv.Atoi(input)
		if err != nil || index < 1 || index > len(options) {
			return errors.New("choose one of the listed numbers")
		}
		if validate != nil {
			return validate(options[index-1].Value)
		}
		return nil
	})
	if err := accessibleField(r, output, field); err != nil {
		return err
	}
	index, err := strconv.Atoi(choice)
	if err != nil || index < 1 || index > len(options) {
		return errors.New("choose one of the listed numbers")
	}
	*value = options[index-1].Value
	return nil
}

func (m *model) selectionFields(r *lineReader, output io.Writer) error {
	if !m.fixedMode {
		if err := accessibleSelect(r, output, "Choose a format", &m.draft.Mode, huh.NewOptions("standard", "emoji", "hybrid"), nil); err != nil {
			return err
		}
	}
	if m.draft.Mode != "emoji" {
		if err := accessibleSelect(r, output, "Choose a commit type", &m.draft.Type, huh.NewOptions(commit.Types...), m.validateType); err != nil {
			return err
		}
	}
	reconcile(&m.draft, m.catalog)
	if m.draft.Mode != "standard" {
		if err := accessibleSelect(r, output, "Choose a gitmoji", &m.draft.Emoji, m.emojiOptions(), nil); err != nil {
			return err
		}
	}
	if !m.options.HideScope && m.options.Scopes != nil {
		scopeOptions := []huh.Option[string]{huh.NewOption("Skip scope", "")}
		for _, choice := range m.scopeChoices() {
			scopeOptions = append(scopeOptions, huh.NewOption(choice, choice))
		}
		if err := accessibleSelect(r, output, "Choose a scope (optional)", &m.draft.Scope, scopeOptions, commit.ValidateScope); err != nil {
			return err
		}
	}
	return nil
}

func (m *model) textFields(r *lineReader, output io.Writer) error {
	run := func(field huh.Field) error { return accessibleField(r, output, field) }
	if !m.options.HideScope && m.options.Scopes == nil {
		if err := run(huh.NewInput().Title("Scope (optional)").Value(&m.draft.Scope).Validate(commit.ValidateScope)); err != nil {
			return err
		}
	}
	if err := run(huh.NewInput().Title("Enter the commit title").Value(&m.draft.Description).Validate(func(v string) error {
		if v == "" {
			v = m.draft.Description
		}
		return commit.ValidateDescription(v)
	})); err != nil {
		return err
	}
	if !m.options.HideBody {
		var body string
		if err := run(huh.NewInput().Title("Body (optional)").Value(&body).Validate(commit.ValidateBody)); err != nil {
			return err
		}
		// PromptString trims whitespace; preserve the literal body entered.
		m.draft.Body = r.lastLine
	}
	if err := commit.ValidateBody(m.draft.Body); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(output, "%s\n%s\n%s\n", m.previewStatus(), m.message().Text, m.options.ConfirmDescription); err != nil {
		return err
	}
	if err := run(huh.NewConfirm().Title("Finish the message?").Value(&m.confirmed)); err != nil {
		return err
	}
	if !m.confirmed {
		return ErrCanceled
	}
	return nil
}
