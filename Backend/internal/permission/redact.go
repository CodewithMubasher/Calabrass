package permission

import "regexp"

// The ledger and the log record what the agent asked to run. A command line
// often carries a credential (`curl -H "Authorization: Bearer ..."`, `--token
// abc`, `PASSWORD=...`), and a record that keeps it would turn a log into a
// place secrets collect. Redact removes the values and keeps the shape, so the
// record still says what ran.
var secretPatterns = []*regexp.Regexp{
	// authorization: bearer xyz   /   Authorization: Basic xyz
	regexp.MustCompile(`(?i)(authorization\s*[:=]\s*)(bearer|basic|token)?\s*[^\s"']+`),
	// --token abc, --api-key=abc, --password abc, -p abc is too ambiguous to guess at
	regexp.MustCompile(`(?i)(--?(?:access[-_]?)?(?:token|api[-_]?key|apikey|secret|password|passwd|pwd|auth)[=\s]+)[^\s"']+`),
	// KEY=value for names that look like credentials
	regexp.MustCompile(`(?i)\b([A-Z0-9_]*(?:TOKEN|SECRET|PASSWORD|PASSWD|API_?KEY|ACCESS_?KEY|PRIVATE_?KEY)[A-Z0-9_]*\s*=\s*)[^\s"']+`),
	// user:password@host in a URL
	regexp.MustCompile(`(?i)(://[^/\s:@]+:)[^/\s@]+(@)`),
	// well-known token shapes
	regexp.MustCompile(`\b(sk-[A-Za-z0-9_-]{16,}|gh[pousr]_[A-Za-z0-9]{20,}|AKIA[0-9A-Z]{16}|xox[baprs]-[A-Za-z0-9-]{10,}|AIza[0-9A-Za-z_-]{30,})\b`),
}

const redacted = "[redacted]"

// Redact returns s with credential values replaced.
func Redact(s string) string {
	if s == "" {
		return s
	}
	s = secretPatterns[0].ReplaceAllString(s, "${1}"+redacted)
	s = secretPatterns[1].ReplaceAllString(s, "${1}"+redacted)
	s = secretPatterns[2].ReplaceAllString(s, "${1}"+redacted)
	s = secretPatterns[3].ReplaceAllString(s, "${1}"+redacted+"${2}")
	s = secretPatterns[4].ReplaceAllString(s, redacted)
	return s
}
