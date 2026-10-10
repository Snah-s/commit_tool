package commit

import (
	"strings"
	"testing"
)

func TestHookAnnotations(t *testing.T) {
	for _, prefix := range []string{"#", ";", "//", "⁑⁕⁑"} {
		leading := prefix + " template help\n\n"
		body := "  original\n# literal body\n\nSigned-off-by: Ana\n"
		trailing := "\n" + prefix + " status\n" + prefix + " file\n"
		original := leading + "raw title\n\n" + body + trailing
		p := SplitHook(original, prefix, "template")
		if p.Leading != leading || p.Trailing != trailing || p.Text != "raw title\n\n"+body {
			t.Fatalf("%q: %+v", prefix, p)
		}
		if got := p.WithMessage(strings.Replace(p.Text, "raw title", "docs: update README", 1)); got != strings.Replace(original, "raw title", "docs: update README", 1) {
			t.Fatalf("annotations changed: %q", got)
		}
	}
	p := SplitHook("# literal title\n\n# literal body\n", "#", "message")
	if !strings.HasPrefix(p.Text, "# literal title") || p.WithMessage(p.Text) != "# literal title\n\n# literal body\n" {
		t.Fatalf("message literal comments changed: %+v", p)
	}
	text := "raw title\n\nbody\n\n; ------------------------ >8 ------------------------\n+ diff content\n"
	p = SplitHook(text, ";", "")
	if !strings.Contains(p.Trailing, "+ diff content") || strings.Contains(p.Text, ">8") || p.WithMessage(p.Text) != text {
		t.Fatalf("scissors lost: %+v", p)
	}
	p = SplitHook("raw title\n\nbody\n\n; status\n", "auto", "template")
	if p.Trailing != "\n; status\n" || p.Text != "raw title\n\nbody\n" {
		t.Fatalf("auto suffix: %+v", p)
	}
}
