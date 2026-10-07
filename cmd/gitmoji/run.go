package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"

	"github.com/Snah-s/commit_tool/internal/catalog"
	"github.com/Snah-s/commit_tool/internal/cli"
	"github.com/Snah-s/commit_tool/internal/commit"
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
		return writeText("gitmoji " + version + " (native, phase 2)")
	}
	switch o.Command {
	case "config", "list", "search", "update":
		fmt.Fprintf(os.Stderr, "%s: implementation pending phase 3\n", o.Command)
		return 1
	case "init", "remove":
		fmt.Fprintf(os.Stderr, "%s: implementation pending phase 4\n", o.Command)
		return 1
	}
	c, err := catalog.Load()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if (o.TypeSet && o.Draft.Mode == "emoji") || (o.EmojiSet && o.Draft.Mode == "standard") {
		fmt.Fprintln(os.Stderr, "--type is not supported in emoji mode; --emoji is not supported in standard mode")
		return 2
	}
	if err := o.Draft.ValidateFields(c, false); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 2
	}
	draft := o.Draft
	input, output := os.Stdin, os.Stderr
	if o.Command == "hook" {
		data, err := os.ReadFile(o.HookFile)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		if o.HookSource == "commit" || o.HookSource == "merge" {
			return 0
		}
		tty, err := openControllingTerminal()
		if err != nil {
			return 0
		}
		defer tty.Close()
		input, output = tty, tty
		parsed, err := commit.Parse(string(data), c)
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
	if interactive {
		draft, err = ui.Run(ctx, input, output, c, draft, ui.Options{Accessible: o.Accessible || os.Getenv("ACCESSIBLE") != "", FormatExplicit: o.FormatSet})
		if errors.Is(err, ui.ErrCanceled) {
			fmt.Fprintln(os.Stderr, "Canceled; the message was not modified and no commit was created.")
			if o.Command == "hook" {
				return 0
			}
			return 130
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	message, err := commit.Build(draft, c, commit.DefaultOptions())
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
	if !o.Prototype {
		fmt.Fprintln(os.Stderr, "Phase 2: message prepared; Git execution and hook writing are pending phase 4.")
	}
	return writeText(message.Text)
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
