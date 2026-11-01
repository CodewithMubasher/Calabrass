// Package termsafe makes text from outside safe to draw in a terminal.
//
// Everything a model, a tool, a web page or a server sends ends up printed on
// the person's screen. A terminal obeys escape sequences, so text that carries
// them is not just text: it can move the cursor and overwrite what was drawn
// (hiding a command from the person while showing another), set the window
// title, and on many terminals write to the clipboard (OSC 52) or ask the
// terminal to answer with data. None of that is something a reply should be
// able to do.
//
// Clean removes control characters and escape sequences and keeps everything
// meant to be read: letters in every language, emoji, newlines and tabs.
package termsafe

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// Clean returns s with escape sequences and control characters removed.
func Clean(s string) string {
	if s == "" {
		return s
	}
	// The common case is clean text; do not allocate for it.
	if !needsCleaning(s) {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		switch {
		case r == utf8.RuneError && size == 1:
			// A byte that is not valid UTF-8 could be a raw C1 control on a
			// terminal that reads it as one. Drop it.
			i++
		case r == 0x1b:
			i += skipEscape(s[i:])
		case r == 0x9b, r == 0x9d, r == 0x90, r == 0x98, r == 0x9e, r == 0x9f:
			// 8-bit CSI / OSC / DCS / SOS / PM / APC introducers: skip what they carry.
			i += size + skipSequenceBody(s[i+size:], r == 0x9b)
		case r == '\n', r == '\t':
			b.WriteRune(r)
			i += size
		case r == '\r':
			// A carriage return lets a line be overwritten after it is drawn.
			// A CRLF pair becomes a plain newline.
			if i+1 < len(s) && s[i+1] == '\n' {
				i += size
				continue
			}
			b.WriteByte('\n')
			i += size
		case unicode.IsControl(r), isBidiOverride(r):
			i += size
		default:
			b.WriteRune(r)
			i += size
		}
	}
	return b.String()
}

func needsCleaning(s string) bool {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < 0x20 && c != '\n' && c != '\t' {
			return true
		}
		if c == 0x7f || c >= 0x80 {
			// Possibly a C1 control, a bidi override or an invalid byte; the slow
			// path decides.
			if c >= 0x80 {
				r, size := utf8.DecodeRuneInString(s[i:])
				if r == utf8.RuneError && size == 1 || unicode.IsControl(r) || isBidiOverride(r) {
					return true
				}
				i += size - 1
				continue
			}
			return true
		}
	}
	return false
}

// isBidiOverride reports the characters that reorder how text is displayed, which
// can make a harmless-looking line read as something else ("Trojan Source").
func isBidiOverride(r rune) bool {
	return (r >= 0x202a && r <= 0x202e) || (r >= 0x2066 && r <= 0x2069)
}

// skipEscape returns how many bytes of s (which starts with ESC) make up one
// escape sequence.
func skipEscape(s string) int {
	if len(s) < 2 {
		return 1
	}
	switch s[1] {
	case '[': // CSI: parameters, then a final byte in @ to ~
		return 2 + skipSequenceBody(s[2:], true)
	case ']', 'P', 'X', '^', '_': // OSC, DCS, SOS, PM, APC: end at BEL or ST
		return 2 + skipSequenceBody(s[2:], false)
	default:
		// ESC followed by one or two intermediate bytes and a final byte, or a
		// single-character escape such as ESC c (reset).
		n := 2
		for n < len(s) && s[n-1] >= 0x20 && s[n-1] <= 0x2f && s[n] >= 0x20 && s[n] <= 0x2f {
			n++
		}
		if n < len(s) && s[n-1] >= 0x20 && s[n-1] <= 0x2f {
			n++
		}
		return n
	}
}

// skipSequenceBody returns the length of a CSI body (csi true) or of a string
// sequence body, including its terminator. An unterminated sequence consumes the
// rest of the input: a reply must not be able to leave the terminal inside one.
func skipSequenceBody(s string, csi bool) int {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if csi {
			if c >= 0x40 && c <= 0x7e {
				return i + 1
			}
			continue
		}
		if c == 0x07 { // BEL
			return i + 1
		}
		if c == 0x1b && i+1 < len(s) && s[i+1] == '\\' { // ST
			return i + 2
		}
		if c == 0x9c { // 8-bit ST (as the lead byte of its UTF-8 form: U+009C)
			return i + 1
		}
		if c == 0xc2 && i+1 < len(s) && s[i+1] == 0x9c {
			return i + 2
		}
	}
	return len(s)
}

// Line is Clean for text that must stay on one line: newlines and tabs become
// spaces.
func Line(s string) string {
	s = Clean(s)
	return strings.Join(strings.Fields(s), " ")
}

// Clip cleans s and cuts it to at most n runes, for text that came from outside
// and is about to be put in an error message.
func Clip(s string, n int) string {
	s = Line(s)
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
