package commit

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Snah-s/commit_tool/internal/catalog"
)

var Types = []string{"feat", "fix", "docs", "refactor", "test", "chore"}

type Draft struct {
	Mode, Type, Emoji, Scope, Description, Body string
}

type Options struct {
	EmojiFormat     string
	CapitalizeTitle bool
}

func DefaultOptions() Options { return Options{EmojiFormat: "emoji", CapitalizeTitle: true} }

type Message struct {
	Title, Text string
	TitleLength int
	LongTitle   bool
}

func ValidateDescription(value string) error {
	if !utf8.ValidString(value) {
		return errors.New("description: invalid UTF-8")
	}
	if strings.TrimSpace(value) == "" {
		return errors.New("enter a description")
	}
	if strings.HasSuffix(strings.TrimSpace(value), ".") || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return errors.New("description must not contain control characters, line breaks, or a trailing period")
	}
	return nil
}

func ValidateScope(value string) error {
	if !utf8.ValidString(value) || value != strings.ToLower(value) || strings.ContainsAny(value, "():") || strings.IndexFunc(value, unicode.IsSpace) >= 0 || strings.IndexFunc(value, unicode.IsControl) >= 0 {
		return errors.New("scope must be lowercase, valid UTF-8, without whitespace, control characters, or ():")
	}
	return nil
}

func ValidateBody(value string) error {
	if !utf8.ValidString(value) || strings.IndexFunc(value, func(r rune) bool {
		return unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t'
	}) >= 0 {
		return errors.New("body must be valid UTF-8, without control characters except tabs and line breaks")
	}
	return nil
}

// ValidateFields allows incomplete answers only while editing the form.
func (d Draft) ValidateFields(c catalog.Catalog, complete bool) error {
	switch d.Mode {
	case "standard", "hybrid":
		if (complete || d.Type != "") && !slices.Contains(Types, d.Type) {
			return errors.New("--type: choose feat, fix, docs, refactor, test, or chore")
		}
	case "emoji":
		if d.Type != "" {
			return errors.New("--type is not supported in emoji mode")
		}
	default:
		return errors.New("--format: choose emoji, standard, or hybrid")
	}
	if d.Mode == "standard" && d.Emoji != "" {
		return errors.New("--emoji is not supported in standard mode")
	}
	if d.Mode != "standard" && (complete || d.Emoji != "") {
		if _, ok := c.Find(d.Emoji); !ok {
			return fmt.Errorf("--emoji: unknown code %q; use an exact shortcode", d.Emoji)
		}
		if d.Mode == "hybrid" && d.Type != "" && !c.Compatible(d.Type, d.Emoji) {
			return fmt.Errorf("emoji %q has no classification compatible with %q", d.Emoji, d.Type)
		}
	}
	if err := ValidateScope(d.Scope); err != nil {
		return err
	}
	if complete || d.Description != "" {
		if err := ValidateDescription(d.Description); err != nil {
			return err
		}
	}
	return ValidateBody(d.Body)
}

func Build(d Draft, c catalog.Catalog, options Options) (Message, error) {
	if err := d.ValidateFields(c, true); err != nil {
		return Message{}, err
	}
	if options.EmojiFormat != "emoji" && options.EmojiFormat != "code" {
		return Message{}, errors.New("emojiFormat must be emoji or code")
	}
	return Preview(d, c, options), nil
}

// Preview uses the same format as Build while allowing incomplete fields.
func Preview(d Draft, c catalog.Catalog, options Options) Message {
	description := d.Description
	emoji, _ := c.Find(d.Emoji)
	symbol := emoji.Emoji
	if options.EmojiFormat == "code" {
		symbol = emoji.Code
	}
	var title string
	if d.Mode == "emoji" {
		if options.CapitalizeTitle && description != "" {
			r, size := utf8.DecodeRuneInString(description)
			description = string(unicode.ToUpper(r)) + description[size:]
		}
		title = symbol + " "
		if d.Scope != "" {
			title += "(" + d.Scope + "): "
		}
		title += description
	} else {
		title = d.Type
		if d.Scope != "" {
			title += "(" + d.Scope + ")"
		}
		title += ": "
		if d.Mode == "hybrid" {
			title += symbol + " "
		}
		title += description
	}
	text := title
	if d.Body != "" {
		text += "\n\n" + d.Body
	}
	count := utf8.RuneCountInString(title)
	return Message{Title: title, Text: text, TitleLength: count, LongTitle: count > 72}
}

func (d Draft) Title(c catalog.Catalog) string   { return Preview(d, c, DefaultOptions()).Title }
func (d Draft) Message(c catalog.Catalog) string { return Preview(d, c, DefaultOptions()).Text }

// Active removes hidden selections when leaving the form, preserving them while editing.
func (d Draft) Active() Draft {
	if d.Mode == "standard" {
		d.Emoji = ""
	}
	if d.Mode == "emoji" {
		d.Type = ""
	}
	return d
}
