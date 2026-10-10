package ui

import (
	"context"
	"encoding/json"
	"io"

	tea "charm.land/bubbletea/v2"
	"charm.land/huh/v2"
	"github.com/Snah-s/commit_tool/internal/config"
)

func preferenceForm(ctx context.Context, input io.Reader, output io.Writer, accessible bool, fields ...huh.Field) error {
	if accessible {
		return runAccessible(ctx, input, func(reader io.Reader) error {
			r := &lineReader{reader: reader}
			for _, field := range fields {
				if err := accessibleField(r, output, field); err != nil {
					return err
				}
			}
			return nil
		})
	}
	groups := make([]*huh.Group, 0, len(fields))
	for _, field := range fields {
		groups = append(groups, huh.NewGroup(field))
	}
	keys := huh.NewDefaultKeyMap()
	keys.Quit.SetKeys("esc", "ctrl+c")
	form := huh.NewForm(groups...).WithKeyMap(keys)
	form.SubmitCmd, form.CancelCmd = tea.Quit, tea.Quit
	err := runProgram(ctx, preferenceModel{form}, input, output)
	if form.State == huh.StateAborted {
		return ErrCanceled
	}
	return err
}

type preferenceModel struct{ *huh.Form }

func (m preferenceModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	_, cmd := m.Form.Update(msg)
	return m, cmd
}

func (m preferenceModel) View() tea.View {
	v := tea.NewView(m.Form.View())
	v.ReportFocus = true
	return v
}

func ConfirmImport(ctx context.Context, input io.Reader, output io.Writer, accessible bool) (bool, error) {
	confirmed := false
	err := preferenceForm(ctx, input, output, accessible, huh.NewConfirm().Title("Import these legacy preferences?").Description("Writes a new native profile; preserves the original.").Value(&confirmed))
	return confirmed, err
}

func Configure(ctx context.Context, input io.Reader, output io.Writer, p config.Preferences, accessible bool) (config.Preferences, error) {
	scope := any(p.ScopePrompt)
	if p.Scopes != nil {
		scope = p.Scopes
	}
	encoded, err := json.Marshal(scope)
	if err != nil {
		return p, err
	}
	scopeText := string(encoded)
	confirmed := false
	fields := []huh.Field{
		huh.NewInput().Title("Commit format (standard/emoji/hybrid)").Value(&p.CommitFormat).Validate(func(value string) error {
			_, err := p.Set([]string{"commitFormat=" + quoteDefault(value, p.CommitFormat)})
			return err
		}),
		huh.NewInput().Title("Emoji format (emoji/code)").Value(&p.EmojiFormat).Validate(func(value string) error {
			_, err := p.Set([]string{"emojiFormat=" + quoteDefault(value, p.EmojiFormat)})
			return err
		}),
		huh.NewConfirm().Title("Capitalize descriptions in emoji mode?").Value(&p.CapitalizeTitle),
		huh.NewConfirm().Title("Automatically stage changes when creating a commit?").Value(&p.AutoAdd),
		huh.NewInput().Title("Scope prompt (JSON boolean or array of scopes)").Value(&scopeText).Validate(func(value string) error {
			if value == "" {
				value = scopeText
			}
			_, err := p.Set([]string{"scopePrompt=" + value})
			return err
		}),
		huh.NewConfirm().Title("Prompt for a message body?").Value(&p.MessagePrompt),
		huh.NewInput().Title("Catalog URL").Value(&p.GitmojisURL).Validate(func(value string) error {
			_, err := p.Set([]string{"gitmojisUrl=" + quoteDefault(value, p.GitmojisURL)})
			return err
		}),
		huh.NewConfirm().Title("Save global preferences?").Description("Project preferences may override these values.").Value(&confirmed),
	}
	if err := preferenceForm(ctx, input, output, accessible, fields...); err != nil {
		return p, err
	}
	if !confirmed {
		return p, ErrCanceled
	}
	return p.Set([]string{"scopePrompt=" + scopeText})
}

func quoteDefault(value, fallback string) string {
	if value == "" {
		value = fallback
	}
	data, _ := json.Marshal(value)
	return string(data)
}
