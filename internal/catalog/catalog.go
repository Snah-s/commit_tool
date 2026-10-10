package catalog

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Snah-s/commit_tool/internal/storage"
)

//go:embed assets/gitmojis.json assets/commit-emojis.json
var assets embed.FS

const maxCatalogBytes = 1 << 20

var identifier = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

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
	data, err := assets.ReadFile("assets/gitmojis.json")
	if err != nil {
		return Catalog{}, err
	}
	c, err := decode(data)
	if err != nil {
		return Catalog{}, fmt.Errorf("embedded catalog: %w", err)
	}
	data, err = assets.ReadFile("assets/commit-emojis.json")
	if err != nil {
		return c, err
	}
	if err = json.Unmarshal(data, &c.Types); err != nil {
		return c, fmt.Errorf("embedded classification: %w", err)
	}
	if err = c.validateTypes(); err != nil {
		return Catalog{}, fmt.Errorf("embedded classification: %w", err)
	}
	return c, nil
}

func decode(data []byte) (Catalog, error) {
	var c Catalog
	if len(data) > maxCatalogBytes {
		return c, fmt.Errorf("catalog exceeds %d bytes", maxCatalogBytes)
	}
	if !utf8.Valid(data) {
		return c, fmt.Errorf("catalog is not valid UTF-8")
	}
	data = bytes.TrimSpace(data)
	if len(data) > 0 && data[0] == '[' {
		if err := json.Unmarshal(data, &c.Entries); err != nil {
			return c, fmt.Errorf("invalid catalog JSON: %w", err)
		}
	} else {
		var document struct {
			Gitmojis []Emoji `json:"gitmojis"`
		}
		if err := json.Unmarshal(data, &document); err != nil {
			return c, fmt.Errorf("invalid catalog JSON: %w", err)
		}
		c.Entries = document.Gitmojis
	}
	if len(c.Entries) == 0 {
		return c, fmt.Errorf("catalog is empty")
	}
	codes, names := make(map[string]bool), make(map[string]bool)
	for i, entry := range c.Entries {
		for _, value := range []string{entry.Code, entry.Emoji, entry.Name, entry.Description} {
			if strings.TrimSpace(value) == "" || !utf8.ValidString(value) || strings.IndexFunc(value, unicode.IsControl) >= 0 {
				return Catalog{}, fmt.Errorf("catalog entry %d has an empty or unsafe field", i+1)
			}
		}
		if len(entry.Code) < 3 || entry.Code[0] != ':' || entry.Code[len(entry.Code)-1] != ':' || !identifier.MatchString(entry.Code[1:len(entry.Code)-1]) || !identifier.MatchString(entry.Name) {
			return Catalog{}, fmt.Errorf("catalog entry %d has an invalid code or name", i+1)
		}
		if codes[entry.Code] || names[entry.Name] {
			return Catalog{}, fmt.Errorf("catalog entry %d repeats a code or name", i+1)
		}
		codes[entry.Code], names[entry.Name] = true, true
	}
	return c, nil
}

func (c Catalog) validateTypes() error {
	types := []string{"feat", "fix", "docs", "refactor", "test", "chore"}
	if len(c.Types) != len(types) {
		return fmt.Errorf("classification must contain exactly six commit types")
	}
	for _, kind := range types {
		if len(c.Types[kind]) == 0 {
			return fmt.Errorf("classification for %s is empty", kind)
		}
		seen := make(map[string]bool)
		for _, code := range c.Types[kind] {
			if _, ok := c.Find(code); !ok || seen[code] {
				return fmt.Errorf("classification for %s has an unknown or repeated code: %s", kind, code)
			}
			seen[code] = true
		}
	}
	return nil
}

func read(r io.Reader) (Catalog, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxCatalogBytes+1))
	if err != nil {
		return Catalog{}, fmt.Errorf("read catalog: %w", err)
	}
	c, err := decode(data)
	if err != nil {
		return Catalog{}, err
	}
	embedded, err := Load()
	if err != nil {
		return Catalog{}, err
	}
	c.Types = make(map[string][]string, len(embedded.Types))
	for kind, codes := range embedded.Types {
		c.Types[kind] = nil
		for _, code := range codes {
			if _, ok := c.Find(code); ok {
				c.Types[kind] = append(c.Types[kind], code)
			}
		}
	}
	return c, nil
}

// ReadFile accepts the legacy array and the official gitmojis object.
func ReadFile(path string) (Catalog, error) {
	f, err := os.Open(path)
	if err != nil {
		return Catalog{}, err
	}
	defer f.Close()
	return read(f)
}

func CachePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(home) {
		return "", fmt.Errorf("home directory must be absolute")
	}
	return filepath.Join(home, ".gitmoji", "gitmojis.json"), nil
}

// Search ranks exact name/description, name prefix, description prefix, name
// substring, description substring, then all-query-token matches. Matching is
// case-insensitive; ties keep catalog order and an empty query returns all entries.
// ponytail: no Fuse.js fuzzy matching; add fuzzy scoring if typo tolerance is needed.
func (c Catalog) Search(query string) []Emoji {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return slices.Clone(c.Entries)
	}
	tokens := strings.Fields(query)
	var ranks [6][]Emoji
	for _, entry := range c.Entries {
		name, description := strings.ToLower(entry.Name), strings.ToLower(entry.Description)
		rank := 0
		switch {
		case name == query || description == query:
		case strings.HasPrefix(name, query):
			rank = 1
		case strings.HasPrefix(description, query):
			rank = 2
		case strings.Contains(name, query):
			rank = 3
		case strings.Contains(description, query):
			rank = 4
		default:
			rank = 5
			for _, token := range tokens {
				if !strings.Contains(name+" "+description, token) {
					rank = -1
					break
				}
			}
		}
		if rank >= 0 {
			ranks[rank] = append(ranks[rank], entry)
		}
	}
	var result []Emoji
	for _, entries := range ranks {
		result = append(result, entries...)
	}
	return result
}

// Update validates the complete response before replacing the legacy cache array.
// Missing or invalid caches are repaired even when content is unchanged.
// Reordering alone is unchanged. A supplied client keeps its transport and timeout;
// the default client uses net/http's environment proxy handling and a 10s timeout.
func Update(ctx context.Context, client *http.Client, url, cachePath string, current Catalog) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, fmt.Errorf("catalog request: %w", err)
	}
	if (req.URL.Scheme != "http" && req.URL.Scheme != "https") || req.URL.Host == "" {
		return false, fmt.Errorf("catalog URL must use HTTP or HTTPS with a host")
	}
	if client == nil {
		client = &http.Client{Timeout: 10 * time.Second}
	}
	response, err := client.Do(req)
	if err != nil {
		return false, fmt.Errorf("fetch catalog: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false, fmt.Errorf("fetch catalog: HTTP status %d", response.StatusCode)
	}
	latest, err := read(response.Body)
	if err != nil {
		return false, err
	}
	previous := make(map[string]Emoji, len(current.Entries))
	for _, entry := range current.Entries {
		previous[entry.Code] = entry
	}
	changed := len(latest.Entries) != len(current.Entries)
	for _, entry := range latest.Entries {
		if previous[entry.Code] != entry {
			changed = true
			break
		}
	}
	if !changed {
		if _, err := ReadFile(cachePath); err == nil {
			return false, nil
		}
	}
	data, err := json.Marshal(latest.Entries)
	if err != nil {
		return false, fmt.Errorf("encode catalog cache: %w", err)
	}
	if err = storage.WriteFile(cachePath, data, 0600); err != nil {
		return false, fmt.Errorf("write catalog cache: %w", err)
	}
	return changed, nil
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
