package catalog

import (
	"embed"
	"encoding/json"
	"fmt"
	"slices"
)

//go:embed assets/gitmojis.json assets/commit-emojis.json
var assets embed.FS

type Emoji struct {
	Code        string `json:"code"`
	Emoji       string `json:"emoji"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type Catalog struct {
	Entries []Emoji
	Types   map[string][]string
}

func Load() (Catalog, error) {
	var c Catalog
	var document struct {
		Gitmojis []Emoji `json:"gitmojis"`
	}
	data, err := assets.ReadFile("assets/gitmojis.json")
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(data, &document); err != nil {
		return c, fmt.Errorf("embedded catalog: %w", err)
	}
	c.Entries = document.Gitmojis
	data, err = assets.ReadFile("assets/commit-emojis.json")
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(data, &c.Types); err != nil {
		return c, fmt.Errorf("embedded classification: %w", err)
	}
	return c, nil
}

func (c Catalog) Find(code string) (Emoji, bool) {
	for _, entry := range c.Entries {
		if entry.Code == code {
			return entry, true
		}
	}
	return Emoji{}, false
}

func (c Catalog) Compatible(commitType, code string) bool {
	return slices.Contains(c.Types[commitType], code)
}
