package termsafe

import (
	"strings"
	"testing"
)

func TestCleanKeepsReadableText(t *testing.T) {
	for _, s := range []string{
		"", "hello", "line one\nline two\n\ttabbed", "héllo wörld", "日本語のテキスト", "emoji 🚀 ✓", "العربية", "code: if (a < b) { return \"x\" }",
	} {
		if got := Clean(s); got != s {
			t.Errorf("%q was changed to %q", s, got)
		}
	}
}

func TestCleanRemovesWhatCanDriveATerminal(t *testing.T) {
	cases := map[string]string{
		"colour \x1b[31mred\x1b[0m text":               "colour red text",
		"cursor \x1b[2J\x1b[Hgone":                     "cursor gone",
		"title \x1b]0;pwned\x07after":                  "title after",
		"clipboard \x1b]52;c;ZXZpbA==\x1b\\after":      "clipboard after",
		"bell\x07 and backspace\x08 and null\x00 done": "bell and backspace and null done",
		"overwrite\rthe line":                          "overwrite\nthe line",
		"windows\r\nendings":                           "windows\nendings",
		"reset \x1bc now":                              "reset  now",
		"dcs \x1bPq#0;2;0;0;0\x1b\\after":              "dcs after",
		"c1 csi \u009b31mred":                          "c1 csi red",
		"c1 osc \u009d52;c;ZXZpbA==\u009cafter":        "c1 osc after",
		"delete\x7fchar":                               "deletechar",
		"bidi \u202eevil\u202c text":                   "bidi evil text",
		"unterminated \x1b]0;never ends":               "unterminated ",
	}
	for in, want := range cases {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCleanDropsInvalidBytes(t *testing.T) {
	if got := Clean("ok\x9b31m\xffend"); strings.ContainsAny(got, "\x9b\xff") {
		t.Fatalf("invalid bytes survived: %q", got)
	}
}

func TestNothingThatSurvivesCanStartAnEscape(t *testing.T) {
	nasty := "a\x1b[1;31mb\x1b]52;c;x\x07c\x1bPz\x1b\\d\u009b2Je\x1b(0f\x1b=g\x1b7h"
	got := Clean(nasty)
	for _, r := range got {
		if r == 0x1b || (r >= 0x80 && r <= 0x9f) {
			t.Fatalf("a control survived: %q", got)
		}
	}
	if !strings.HasPrefix(got, "a") {
		t.Fatalf("readable text was lost: %q", got)
	}
}

func TestLineAndClip(t *testing.T) {
	if got := Line("a\n b\t\tc\x1b[0m"); got != "a b c" {
		t.Fatalf("Line = %q", got)
	}
	if got := Clip(strings.Repeat("x", 50), 10); got != "xxxxxxxxxx…" {
		t.Fatalf("Clip = %q", got)
	}
}
