package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Snah-s/commit_tool/internal/commit"
	"github.com/Snah-s/commit_tool/internal/storage"
)

type Preferences struct {
	AutoAdd         bool
	ScopePrompt     bool
	Scopes          []string
	MessagePrompt   bool
	EmojiFormat     string
	CapitalizeTitle bool
	GitmojisURL     string
	CommitFormat    string
	Unknown         map[string]json.RawMessage
}

type Resolved struct {
	Preferences Preferences
	Source      string
	Project     bool
}

func Defaults() Preferences {
	return Preferences{MessagePrompt: true, EmojiFormat: "emoji", CapitalizeTitle: true,
		GitmojisURL: "https://gitmoji.dev/api/gitmojis", CommitFormat: "standard"}
}

func Parse(data []byte, legacy bool) (Preferences, error) {
	p := Defaults()
	fields, err := object(data)
	if err != nil {
		return p, err
	}
	if legacy {
		p.CommitFormat = "emoji"
	}
	p.Unknown = make(map[string]json.RawMessage)
	for key, value := range fields {
		var target any
		switch key {
		case "autoAdd":
			target = &p.AutoAdd
		case "messagePrompt":
			target = &p.MessagePrompt
		case "capitalizeTitle":
			target = &p.CapitalizeTitle
		case "emojiFormat":
			target = &p.EmojiFormat
		case "gitmojisUrl":
			target = &p.GitmojisURL
		case "commitFormat":
			target = &p.CommitFormat
		case "scopePrompt":
			if bytes.HasPrefix(value, []byte("[")) {
				if err := json.Unmarshal(value, &p.Scopes); err != nil {
					return p, fmt.Errorf("scopePrompt: expected a boolean or array of strings: %w", err)
				}
				// Unmarshal accepts null string elements; check their JSON types as well.
				var entries []json.RawMessage
				_ = json.Unmarshal(value, &entries)
				for i, scope := range p.Scopes {
					if bytes.Equal(entries[i], []byte("null")) || scope == "" {
						return p, errors.New("scopePrompt: scopes must be nonempty strings")
					}
					if err := commit.ValidateScope(scope); err != nil {
						return p, fmt.Errorf("scopePrompt: %w", err)
					}
				}
				p.ScopePrompt = true
				continue
			}
			target = &p.ScopePrompt
		default:
			p.Unknown[key] = value
			continue
		}
		if bytes.Equal(value, []byte("null")) {
			return p, fmt.Errorf("%s: null is not allowed", key)
		}
		if err := json.Unmarshal(value, target); err != nil {
			return p, fmt.Errorf("%s: invalid type: %w", key, err)
		}
	}
	if !slices.Contains([]string{"standard", "emoji", "hybrid"}, p.CommitFormat) {
		return p, errors.New("commitFormat: choose standard, emoji, or hybrid")
	}
	if !slices.Contains([]string{"emoji", "code"}, p.EmojiFormat) {
		return p, errors.New("emojiFormat: choose emoji or code")
	}
	u, err := url.Parse(p.GitmojisURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.Opaque != "" {
		return p, errors.New("gitmojisUrl: expected an absolute HTTP(S) URL")
	}
	return p, nil
}

func object(data []byte) (map[string]json.RawMessage, error) {
	if len(data) > 1<<20 || !utf8.Valid(data) {
		return nil, errors.New("JSON must be valid UTF-8 and at most 1 MiB")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return nil, fmt.Errorf("invalid JSON object: %w", err)
	}
	if fields == nil {
		return nil, errors.New("expected a JSON object")
	}
	return fields, nil
}

func (p Preferences) JSON() ([]byte, error) {
	fields := make(map[string]any, len(p.Unknown)+7)
	for key, value := range p.Unknown {
		fields[key] = value
	}
	fields["autoAdd"], fields["messagePrompt"] = p.AutoAdd, p.MessagePrompt
	fields["emojiFormat"], fields["capitalizeTitle"] = p.EmojiFormat, p.CapitalizeTitle
	fields["gitmojisUrl"], fields["commitFormat"] = p.GitmojisURL, p.CommitFormat
	fields["scopePrompt"] = p.ScopePrompt
	if p.Scopes != nil {
		fields["scopePrompt"] = p.Scopes
	}
	return json.MarshalIndent(fields, "", "  ")
}

func Read(path string, legacy bool) (Preferences, error) {
	data, err := readFile(path)
	if err != nil {
		return Preferences{}, err
	}
	p, err := Parse(data, legacy)
	if err != nil {
		return p, fmt.Errorf("%s: %w", path, err)
	}
	return p, nil
}

func readFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(io.LimitReader(f, (1<<20)+1))
}

func Resolve(cwd string) (Resolved, error) {
	dir, err := filepath.Abs(cwd)
	if err != nil {
		return Resolved{}, err
	}
	for {
		manifest := filepath.Join(dir, "package.json")
		data, err := readFile(manifest)
		if err == nil {
			fields, err := object(data)
			if err != nil {
				return Resolved{}, fmt.Errorf("%s: %w", manifest, err)
			}
			if value, ok := fields["gitmoji"]; ok {
				p, err := Parse(value, false)
				if err != nil {
					return Resolved{}, fmt.Errorf("%s: %w", manifest, err)
				}
				return Resolved{p, manifest, true}, nil
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return Resolved{}, err
		}
		rc := filepath.Join(dir, ".gitmojirc.json")
		p, err := Read(rc, false)
		if err == nil {
			return Resolved{p, rc, true}, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return Resolved{}, err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	path, err := NativePath()
	if err != nil {
		return Resolved{}, err
	}
	p, err := Read(path, false)
	if errors.Is(err, os.ErrNotExist) {
		return Resolved{Preferences: Defaults(), Source: "defaults"}, nil
	}
	return Resolved{Preferences: p, Source: path}, err
}

func NativePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(dir) {
		return "", errors.New("user configuration directory must be absolute")
	}
	return filepath.Join(dir, "gitmoji", "config.json"), nil
}

func LegacyPath() (string, error) {
	var dir string
	switch runtime.GOOS {
	case "darwin":
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, "Library", "Preferences")
	case "windows":
		dir = os.Getenv("APPDATA")
		if dir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			dir = filepath.Join(home, "AppData", "Roaming")
		}
	default:
		dir = os.Getenv("XDG_CONFIG_HOME")
		if dir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return "", err
			}
			dir = filepath.Join(home, ".config")
		}
	}
	if !filepath.IsAbs(dir) {
		return "", errors.New("legacy configuration directory must be absolute")
	}
	if runtime.GOOS == "windows" {
		return filepath.Join(dir, "gitmoji-nodejs", "Config", "config.json"), nil
	}
	return filepath.Join(dir, "gitmoji-nodejs", "config.json"), nil
}

func (p Preferences) Set(assignments []string) (Preferences, error) {
	data, err := p.JSON()
	if err != nil {
		return p, err
	}
	fields, err := object(data)
	if err != nil {
		return p, err
	}
	for _, assignment := range assignments {
		key, value, ok := strings.Cut(assignment, "=")
		if !ok || key == "" || !json.Valid([]byte(value)) {
			return p, fmt.Errorf("--set: expected KEY=JSON, got %q", assignment)
		}
		switch key {
		case "autoAdd", "scopePrompt", "messagePrompt", "emojiFormat", "capitalizeTitle", "gitmojisUrl", "commitFormat":
			fields[key] = json.RawMessage(value)
		default:
			return p, fmt.Errorf("--set: unknown preference %q", key)
		}
	}
	data, err = json.Marshal(fields)
	if err != nil {
		return p, err
	}
	return Parse(data, false)
}

func Save(path string, p Preferences) error {
	data, err := p.JSON()
	if err != nil {
		return err
	}
	if _, err := Parse(data, false); err != nil {
		return err
	}
	return storage.WriteFile(path, append(data, '\n'), 0600)
}

func Create(path string, p Preferences) error {
	data, err := p.JSON()
	if err != nil {
		return err
	}
	if _, err := Parse(data, false); err != nil {
		return err
	}
	return storage.CreateFile(path, append(data, '\n'), 0600)
}

// Import writes a new native profile only after validating the legacy file.
func Import(source, destination string) (Preferences, error) {
	if _, err := os.Lstat(destination); err == nil {
		return Preferences{}, fmt.Errorf("%s: native profile already exists; import will not overwrite it", destination)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Preferences{}, err
	}
	p, err := Read(source, true)
	if err != nil {
		return p, err
	}
	return p, Create(destination, p)
}
