package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	"github.com/Snah-s/commit_tool/internal/commit"
)

type Options struct {
	Command, HookFile, HookSource, HookObject string
	Queries                                   []string
	Draft                                     commit.Draft
	Help, Version, Accessible, Prototype      bool
	FormatSet, TypeSet, EmojiSet              bool
	TitleSet, ScopeSet, BodySet               bool
	ConfigSet                                 []string
	ImportFile                                string
	ImportSet, Show                           bool
}

var actions = map[string]string{
	"c": "commit", "commit": "commit", "g": "config", "config": "config",
	"i": "init", "init": "init", "r": "remove", "remove": "remove",
	"l": "list", "list": "list", "s": "search", "search": "search", "u": "update", "update": "update",
}

func knownCommand(value string) bool {
	return value == "commit" || value == "config" || value == "init" || value == "remove" || value == "list" || value == "search" || value == "update" || value == "hook" || value == "prototype"
}

func Parse(args []string) (Options, error) {
	o := Options{Draft: commit.Draft{Mode: "standard"}}
	for _, arg := range args {
		if !utf8.ValidString(arg) {
			return o, errors.New("argument contains invalid UTF-8")
		}
	}
	f := flag.NewFlagSet("gitmoji", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.BoolVar(&o.Help, "help", false, "")
	f.BoolVar(&o.Help, "h", false, "")
	f.BoolVar(&o.Version, "version", false, "")
	f.BoolVar(&o.Version, "v", false, "")
	f.BoolVar(&o.Accessible, "accessible", false, "")
	f.StringVar(&o.Draft.Mode, "format", "standard", "")
	f.StringVar(&o.Draft.Type, "type", "", "")
	f.StringVar(&o.Draft.Emoji, "emoji", "", "")
	f.StringVar(&o.Draft.Description, "title", "", "")
	f.StringVar(&o.Draft.Body, "message", "", "")
	f.StringVar(&o.Draft.Scope, "scope", "", "")
	f.StringVar(&o.HookFile, "hook", "", "")
	f.StringVar(&o.ImportFile, "import", "", "")
	f.BoolVar(&o.Show, "show", false, "")
	f.Func("set", "", func(value string) error { o.ConfigSet = append(o.ConfigSet, value); return nil })
	selected := make(map[string]*bool)
	for alias, command := range actions {
		if selected[command] == nil {
			selected[command] = new(bool)
		}
		f.BoolVar(selected[command], alias, false, "")
	}
	// flag does not interleave flags and positionals: split tokens without reinterpreting values.
	var flags, positional []string
	literalFirst := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			if len(positional) == 0 {
				literalFirst = true
			}
			positional = append(positional, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positional = append(positional, arg)
			continue
		}
		flags = append(flags, arg)
		name, _, inline := strings.Cut(strings.TrimLeft(arg, "-"), "=")
		if item := f.Lookup(name); item != nil && !inline {
			if boolean, ok := item.Value.(interface{ IsBoolFlag() bool }); !ok || !boolean.IsBoolFlag() {
				i++
				if i >= len(args) {
					return o, fmt.Errorf("missing value for %s", arg)
				}
				flags = append(flags, args[i])
			}
		}
	}
	if err := f.Parse(flags); err != nil {
		return o, err
	}
	hookSet := false
	f.Visit(func(item *flag.Flag) {
		switch item.Name {
		case "format":
			o.FormatSet = true
		case "type":
			o.TypeSet = true
		case "emoji":
			o.EmojiSet = true
		case "title":
			o.TitleSet = true
		case "message":
			o.BodySet = true
		case "scope":
			o.ScopeSet = true
		case "hook":
			hookSet = true
		case "import":
			o.ImportSet = true
		}
	})
	explicit := ""
	if len(positional) > 0 && !literalFirst && knownCommand(positional[0]) {
		explicit, positional = positional[0], positional[1:]
		if explicit == "prototype" {
			o.Prototype, explicit = true, "commit"
			if hookSet {
				explicit = "hook"
			}
		}
	}
	o.Command = explicit
	choose := func(command string) error {
		if o.Command != "" && o.Command != command {
			return fmt.Errorf("conflicting actions: %s and %s", o.Command, command)
		}
		o.Command = command
		return nil
	}
	for command, enabled := range selected {
		if *enabled {
			if err := choose(command); err != nil {
				return o, err
			}
		}
	}
	if hookSet {
		if err := choose("hook"); err != nil {
			return o, err
		}
	}
	if o.Command == "" {
		if len(positional) != 0 {
			return o, fmt.Errorf("unknown command: %q", positional[0])
		}
		if o.Help || o.Version || len(args) == 0 {
			o.Help = o.Help || !o.Version
			return o, nil
		}
		return o, errors.New("specify a command, for example commit or -c")
	}
	if o.Help || o.Version {
		return o, nil
	}
	switch o.Command {
	case "search":
		o.Queries = positional
	case "hook":
		if !hookSet && len(positional) > 0 {
			o.HookFile, positional = positional[0], positional[1:]
		}
		if o.HookFile == "" || strings.IndexByte(o.HookFile, 0) >= 0 || len(positional) > 2 {
			return o, errors.New("usage: hook file [source [object]]")
		}
		if len(positional) > 0 {
			o.HookSource = positional[0]
		}
		if len(positional) > 1 {
			o.HookObject = positional[1]
		}
	default:
		if len(positional) > 0 {
			return o, fmt.Errorf("unexpected arguments for %s: %q", o.Command, positional)
		}
	}
	if o.Command != "commit" && o.Command != "hook" && (o.FormatSet || o.TypeSet || o.EmojiSet || o.TitleSet || o.ScopeSet || o.BodySet || o.Accessible && o.Command != "config") {
		return o, fmt.Errorf("message flags are not supported with %s", o.Command)
	}
	if o.Command != "config" && (o.ImportSet || o.Show || len(o.ConfigSet) > 0) {
		return o, errors.New("--import, --show, and --set are only supported with config")
	}
	if o.ImportSet && (o.ImportFile == "" || len(o.ConfigSet) > 0 || o.Show) || o.Show && len(o.ConfigSet) > 0 {
		return o, errors.New("config: choose --show, --set KEY=JSON, or --import FILE separately")
	}
	return o, nil
}

// Defaults applies explicit values, including empty ones, and preserves the existing body.
func (o Options) Defaults(initial commit.Draft) commit.Draft {
	initial.Mode = o.Draft.Mode
	if o.TypeSet {
		initial.Type = o.Draft.Type
	}
	if o.EmojiSet {
		initial.Emoji = o.Draft.Emoji
	}
	if o.TitleSet {
		initial.Description = o.Draft.Description
	}
	if o.ScopeSet {
		initial.Scope = o.Draft.Scope
	}
	if o.BodySet {
		initial.Body = o.Draft.Body
	}
	return initial.Active()
}

const Help = `Usage: gitmoji <command> [flags]

  commit (-c, --commit)        Create a Git commit using the current index
  prototype                   Prepare a message without changing Git or files
  config (-g, --config)        Edit global preferences; without TTY, show preferences
  list (-l, --list)            List the offline catalog
  search (-s, --search)        Search multiple queries independently
  update (-u, --update)        Update the catalog; retain the cache on failure
  init (-i, --init)            Install/update the recognized managed hook
  remove (-r, --remove)        Remove only a recognized managed hook
  hook, --hook FILE            Review and save file [source [object]] with TTY

  --format emoji|standard|hybrid   Default: standard; overrides preferences
  --type feat|fix|docs|refactor|test|chore
  --emoji :code:                  Exact catalog code
  --title DESCRIPTION            Description, without title prefixes
  --scope SCOPE                  Optional, lowercase
  --message BODY                 Literal paragraphs/trailers
  --accessible                   Compatibility alias; prompts are inline by default
  config --show                  Show global/effective preferences and their source
  config --set KEY=JSON           Save a global preference; repeat for multiple values
  config --import FILE            Import a legacy profile when no native profile exists
  -h, --help                     Help without side effects
  -v, --version                  Native version without side effects

No TTY: commit requires --title and a complete selection; prototype returns the message.
Flags can be interleaved; -- ends flags and allows literal queries.
Values starting with a dash: --title=-value. Repeated flags: last value wins.
Commit honors autoAdd (false by default); an active managed hook blocks this client.
Hooks preserve messages on cancel, without TTY, during rebase, and for commit/merge sources.
`
