package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/Snah-s/commit_tool/internal/cli"
	"github.com/Snah-s/commit_tool/internal/config"
	"github.com/Snah-s/commit_tool/internal/ui"
	"github.com/charmbracelet/x/term"
)

func configure(ctx context.Context, o cli.Options, cwd string) int {
	path, err := config.NativePath()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	resolved, err := config.Resolve(cwd)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	p, err := config.Read(path, false)
	absent := errors.Is(err, os.ErrNotExist)
	if absent {
		p = config.Defaults()
	} else if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	interactive := term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stderr.Fd())
	if o.ImportSet {
		if !absent {
			fmt.Fprintln(os.Stderr, "Native profile already exists; import will not overwrite it.")
			return 2
		}
		candidate, err := config.Read(o.ImportFile, true)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
		if code := previewImport(o.ImportFile, candidate); code != 0 {
			return code
		}
		if interactive {
			confirmed, err := ui.ConfirmImport(ctx, os.Stdin, os.Stderr, true)
			if err != nil {
				return formError(err)
			}
			if !confirmed {
				return 130
			}
		}
		if err := config.Create(path, candidate); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return writeText("Imported preferences into " + path)
	}
	if o.Show || !interactive && len(o.ConfigSet) == 0 {
		globalJSON, err := p.JSON()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		effectiveJSON, err := resolved.Preferences.JSON()
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		data, err := json.MarshalIndent(struct {
			GlobalPath       string          `json:"globalPath"`
			Global           json.RawMessage `json:"global"`
			EffectiveSource  string          `json:"effectiveSource"`
			Effective        json.RawMessage `json:"effective"`
			ProjectOverrides bool            `json:"projectOverrides"`
		}{path, globalJSON, resolved.Source, effectiveJSON, resolved.Project}, "", "  ")
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return writeText(string(data))
	}
	if resolved.Project {
		fmt.Fprintf(os.Stderr, "Project preferences override global values: %s\n", resolved.Source)
	}
	if len(o.ConfigSet) > 0 {
		p, err = p.Set(o.ConfigSet)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 2
		}
	} else {
		if absent {
			source, err := config.LegacyPath()
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				return 2
			}
			candidate, err := config.Read(source, true)
			if err == nil {
				if code := previewImport(source, candidate); code != 0 {
					return code
				}
				confirmed, err := ui.ConfirmImport(ctx, os.Stdin, os.Stderr, true)
				if err != nil {
					return formError(err)
				}
				if confirmed {
					if err := config.Create(path, candidate); err != nil {
						fmt.Fprintln(os.Stderr, err)
						return 1
					}
					return writeText("Imported preferences into " + path)
				}
			} else if !errors.Is(err, os.ErrNotExist) {
				fmt.Fprintln(os.Stderr, err)
				return 2
			}
		}
		p, err = ui.Configure(ctx, os.Stdin, os.Stderr, p, true)
		if err != nil {
			return formError(err)
		}
	}
	if err := config.Save(path, p); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return writeText("Saved global preferences to " + path)
}

func previewImport(source string, p config.Preferences) int {
	data, err := p.JSON()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	_, err = fmt.Fprintf(os.Stderr, "Legacy source: %s\n%s\n", source, data)
	if err != nil {
		return 1
	}
	return 0
}

func formError(err error) int {
	if errors.Is(err, ui.ErrCanceled) {
		fmt.Fprintln(os.Stderr, "Canceled; preferences were not saved.")
		return 130
	}
	fmt.Fprintln(os.Stderr, err)
	return 1
}
