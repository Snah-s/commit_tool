package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"runtime"
	"strings"

	"github.com/charmbracelet/x/term"

	"github.com/Snah-s/commit_tool/internal/catalog"
	"github.com/Snah-s/commit_tool/internal/cli"
	"github.com/Snah-s/commit_tool/internal/commit"
	"github.com/Snah-s/commit_tool/internal/config"
	gitclient "github.com/Snah-s/commit_tool/internal/git"
	"github.com/Snah-s/commit_tool/internal/ui"
)

func run(ctx context.Context, args []string) int {
	o, err := cli.Parse(args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	if o.Help {
		return writeText(cli.Help)
	}
	if o.Version {
		return writeText(fmt.Sprintf("gitmoji %s (native, %s/%s, %s, revision %s, commit date %s)", version, runtime.GOOS, runtime.GOARCH, runtime.Version(), revision, buildDate))
	}
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	var repo gitclient.Repo
	if o.Command == "init" || o.Command == "remove" {
		repo, err = gitclient.Open(ctx, cwd)
		if err != nil {
			return gitFailure(err)
		}
		if o.Command == "init" {
			var executable string
			executable, err = os.Executable()
			if err == nil {
				err = repo.InstallHook(ctx, executable)
			}
		} else {
			err = repo.RemoveHook(ctx)
		}
		if err != nil {
			return gitFailure(err)
		}
		return writeText("Managed hook " + map[string]string{"init": "installed.", "remove": "removed (or already absent)."}[o.Command])
	}
	input, output := os.Stdin, os.Stderr
	var hookData []byte
	var parts commit.HookParts
	if o.Command == "hook" {
		hookData, err = readHook(o.HookFile)
		if err != nil {
			return gitFailure(err)
		}
		if o.HookSource == "commit" || o.HookSource == "merge" {
			return 0
		}
		tty, err := openControllingTerminal()
		if err != nil {
			return 0
		}
		defer tty.Close()
		if !term.IsTerminal(tty.Fd()) {
			return 0
		}
		input, output = tty, tty
		parts.Text = string(hookData)
		if !o.Prototype {
			repo, err = gitclient.Open(ctx, cwd)
			if err != nil {
				return gitFailure(err)
			}
			rebasing, err := repo.RebaseActive(ctx)
			if err != nil {
				return gitFailure(err)
			}
			if rebasing {
				return 0
			}
			prefix, err := repo.CommentPrefix(ctx)
			if err != nil {
				return gitFailure(err)
			}
			if err := commit.ValidateBody(string(hookData)); err != nil {
				return gitFailure(err)
			}
			parts = commit.SplitHook(string(hookData), prefix, o.HookSource)
		}
	}
	if o.Command == "config" {
		return configure(ctx, o, cwd)
	}
	resolved, err := config.Resolve(cwd)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	p := resolved.Preferences
	if !o.FormatSet {
		o.Draft.Mode = p.CommitFormat
	}
	cachePath, err := catalog.CachePath()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	c, err := catalog.ReadFile(cachePath)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "warning: %s: %v; using embedded catalog\n", cachePath, err)
		}
		c, err = catalog.Load()
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	switch o.Command {
	case "list":
		return printCatalog(c.Entries)
	case "search":
		if len(o.Queries) == 0 {
			return printCatalog(c.Entries)
		}
		for _, query := range o.Queries {
			if _, err := fmt.Fprintf(os.Stderr, "Search: %q\n", query); err != nil {
				return 1
			}
			if code := printCatalog(c.Search(query)); code != 0 {
				return code
			}
		}
		return 0
	case "update":
		changed, err := catalog.Update(ctx, nil, p.GitmojisURL, cachePath, c)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if changed {
			return writeText("Catalog updated.")
		}
		return writeText("Catalog is already up to date.")
	}
	messageOptions := commit.Options{EmojiFormat: p.EmojiFormat, CapitalizeTitle: p.CapitalizeTitle}
	if (o.TypeSet && o.Draft.Mode == "emoji") || (o.EmojiSet && o.Draft.Mode == "standard") {
		fmt.Fprintln(os.Stderr, "--type is not supported in emoji mode; --emoji is not supported in standard mode")
		return 2
	}
	if err := o.Draft.ValidateFields(c, false); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	draft := o.Draft
	if o.Command == "hook" {
		parsed, err := commit.Parse(parts.Text, c)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if !o.TitleSet && !o.TypeSet && !o.EmojiSet && !o.ScopeSet && !o.BodySet && parsed.Mode == o.Draft.Mode && parsed.ValidateFields(c, true) == nil {
			return 0
		}
		draft = o.Defaults(parsed)
	}
	interactive := term.IsTerminal(input.Fd()) && term.IsTerminal(output.Fd())
	confirmation := "Returns the message only; does not create a commit."
	if !o.Prototype && o.Command == "commit" {
		if !interactive {
			if _, err := commit.Build(draft, c, messageOptions); err != nil {
				fmt.Fprintln(os.Stderr, err)
				fmt.Fprintln(os.Stderr, "No TTY: provide --title and --type/--emoji as required by --format.")
				return 2
			}
		}
		repo, err = gitclient.Open(ctx, cwd)
		if err != nil {
			return gitFailure(err)
		}
		owned, err := repo.HasManagedHook(ctx)
		if err != nil {
			return gitFailure(err)
		}
		if owned {
			return gitFailure(errors.New("managed prepare-commit-msg hook is active; use git commit or remove the managed hook before using this client"))
		}
		confirmation = "Creates a Git commit from the index."
		if p.AutoAdd {
			confirmation = "Stages changes under the current directory and creates a Git commit."
		}
	} else if !o.Prototype && o.Command == "hook" {
		confirmation = "Saves the message file; Git continues the commit."
	}
	if interactive {
		draft, err = ui.Run(ctx, input, output, c, draft, ui.Options{Accessible: true, InlineSelectors: true, FormatExplicit: o.FormatSet, HideScope: !p.ScopePrompt, HideBody: !p.MessagePrompt, Scopes: p.Scopes, MessageOptions: messageOptions, ConfirmDescription: confirmation})
		if errors.Is(err, ui.ErrCanceled) {
			if o.Command == "hook" {
				fmt.Fprintln(os.Stderr, "Canceled; the original message was preserved and Git may continue.")
				return 0
			}
			fmt.Fprintln(os.Stderr, "Canceled; no commit was created.")
			return 130
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	message, err := commit.Build(draft, c, messageOptions)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		if !interactive {
			fmt.Fprintln(os.Stderr, "No TTY: provide --title and --type/--emoji as required by --format.")
		}
		return 2
	}
	if message.LongTitle {
		fmt.Fprintf(os.Stderr, "warning: title has %d Unicode code points; recommended length is 72\n", message.TitleLength)
	}
	if o.Prototype {
		return writeText(message.Text)
	}
	if o.Command == "hook" {
		if err := replaceHook(o.HookFile, hookData, []byte(parts.WithMessage(message.Text))); err != nil {
			return gitFailure(err)
		}
		return 0
	}
	if err := repo.Commit(ctx, message.Text, p.AutoAdd, os.Stdin, os.Stdout, os.Stderr); err != nil {
		return gitFailure(err)
	}
	return 0
}

func gitFailure(err error) int {
	fmt.Fprintln(os.Stderr, err)
	return gitclient.ExitCode(err)
}

func printCatalog(entries []catalog.Emoji) int {
	for _, entry := range entries {
		if _, err := fmt.Fprintf(os.Stdout, "%s %s %s\n", entry.Emoji, entry.Code, entry.Description); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	return 0
}

func writeText(text string) int {
	if !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if _, err := fmt.Fprint(os.Stdout, text); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
