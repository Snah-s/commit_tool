package commit

import "strings"

// HookParts isolates template annotations without deleting or rewriting them.
// Comments inside the body stay literal; Git applies its own cleanup after the hook.
type HookParts struct {
	Text, Leading, Trailing string
}

func SplitHook(text, prefix, source string) HookParts {
	lines := strings.SplitAfter(text, "\n")
	if prefix == "auto" {
		// ponytail: infer auto from trailing comments; use an explicit prefix for ambiguous templates.
		prefix = ""
		for i := len(lines) - 1; i >= 0; i-- {
			line := strings.TrimSpace(lines[i])
			if line == "" {
				continue
			}
			if strings.ContainsRune("#;@!$%^&|:", rune(line[0])) {
				prefix = line[:1]
			}
			break
		}
	}
	comment := func(line string) bool { return prefix != "" && strings.HasPrefix(line, prefix) }
	blank := func(line string) bool { return strings.TrimSpace(line) == "" }
	start, end := 0, len(lines)
	if source != "message" {
		for start < end && (blank(lines[start]) || comment(lines[start])) {
			start++
		}
	}
	for i := start; i < end; i++ {
		if prefix != "" && strings.TrimRight(lines[i], "\r\n") == prefix+" ------------------------ >8 ------------------------" {
			end = i
			break
		}
	}
	footer := end
	hasComment := false
	for footer > start+1 && (blank(lines[footer-1]) || comment(lines[footer-1])) {
		hasComment = hasComment || comment(lines[footer-1])
		footer--
	}
	if hasComment {
		end = footer
	}
	return HookParts{strings.Join(lines[start:end], ""), strings.Join(lines[:start], ""), strings.Join(lines[end:], "")}
}

func (p HookParts) WithMessage(message string) string {
	leading := p.Leading
	if leading != "" && !strings.HasSuffix(leading, "\n") {
		leading += "\n"
	}
	if !strings.HasSuffix(message, "\n") {
		message += "\n"
	}
	return leading + message + p.Trailing
}
