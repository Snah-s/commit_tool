package commit

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/Snah-s/commit_tool/internal/catalog"
)

var conventionalHeader = regexp.MustCompile(`^([a-zA-Z]+)(?:\(([^()]+)\))?: (.*)$`)
var emojiScope = regexp.MustCompile(`^\(([^()]+)\): (.*)$`)

// Parse recognizes known prefixes; a free-form title keeps Mode empty.
// The body stays literal, removing only the initial separator line.
func Parse(text string, c catalog.Catalog) (Draft, error) {
	if !utf8.ValidString(text) {
		return Draft{}, errors.New("message: invalid UTF-8")
	}
	title, body, _ := strings.Cut(text, "\n")
	title = strings.TrimSuffix(title, "\r")
	if strings.HasPrefix(body, "\r\n") {
		body = body[2:]
	} else {
		body = strings.TrimPrefix(body, "\n")
	}
	if strings.IndexFunc(title, unicode.IsControl) >= 0 {
		return Draft{}, errors.New("title contains control characters")
	}
	if err := ValidateBody(body); err != nil {
		return Draft{}, err
	}
	d := Draft{Description: title, Body: body}
	if match := conventionalHeader.FindStringSubmatch(title); match != nil {
		d.Mode, d.Type, d.Scope, d.Description = "standard", match[1], match[2], match[3]
		if code, rest := leadingEmoji(d.Description, c); code != "" {
			d.Mode, d.Emoji, d.Description = "hybrid", code, rest
		}
		return d, nil
	}
	if code, rest := leadingEmoji(title, c); code != "" {
		d.Mode, d.Emoji, d.Description = "emoji", code, rest
		if match := emojiScope.FindStringSubmatch(rest); match != nil {
			d.Scope, d.Description = match[1], match[2]
		}
	}
	return d, nil
}

func leadingEmoji(value string, c catalog.Catalog) (code, rest string) {
	longest := 0
	for _, entry := range c.Entries {
		for _, symbol := range []string{entry.Emoji, entry.Code} {
			if len(symbol) > longest && strings.HasPrefix(value, symbol+" ") {
				code, rest, longest = entry.Code, value[len(symbol)+1:], len(symbol)
			}
		}
	}
	return code, rest
}
