package voice

import "strings"

// sentenceBytes is how much text one speech request should carry. Long enough to
// sound like a phrase, short enough to keep the first audio quick.
const sentenceBytes = 180

// Sentences splits a reply at sentence ends. The reply is prose, so the split
// is deliberately simple: punctuation followed by a space (or the end), then
// anything too long is cut at a word boundary so a request never runs away.
func Sentences(text string) []string {
	text = strings.TrimSpace(text)
	if text == "" {
		return nil
	}

	var chunks []string
	start := 0
	for i := 0; i < len(text); i++ {
		if !isSentenceEnd(text[i]) {
			continue
		}
		if i+1 != len(text) && text[i+1] != ' ' && text[i+1] != '\n' {
			continue
		}
		if chunk := strings.TrimSpace(text[start : i+1]); chunk != "" {
			chunks = append(chunks, chunk)
		}
		start = i + 1
	}
	if rest := strings.TrimSpace(text[start:]); rest != "" {
		chunks = append(chunks, rest)
	}

	var out []string
	for _, chunk := range chunks {
		out = append(out, splitLong(chunk)...)
	}
	return out
}

func isSentenceEnd(b byte) bool {
	return b == '.' || b == '!' || b == '?'
}

// splitLong cuts a chunk that is longer than sentenceBytes at its last space,
// so words survive.
func splitLong(chunk string) []string {
	if len(chunk) <= sentenceBytes {
		return []string{chunk}
	}

	cut := strings.LastIndex(chunk[:sentenceBytes], " ")
	if cut <= 0 {
		return []string{chunk}
	}

	var out []string
	out = append(out, chunk[:cut])
	out = append(out, splitLong(chunk[cut+1:])...)
	return out
}
