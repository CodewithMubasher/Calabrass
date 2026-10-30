package permission

import (
	"strings"
	"testing"
)

func TestRedactRemovesCredentialsAndKeepsTheShape(t *testing.T) {
	cases := []struct{ in, mustKeep string }{
		{`curl -H "Authorization: Bearer abc123def456" https://api.example.com`, "https://api.example.com"},
		{`npm publish --token npm_abcdefghijklmnop`, "npm publish"},
		{`mytool --api-key=SUPERSECRETVALUE run`, "run"},
		{`DB_PASSWORD=hunter2 npm test`, "npm test"},
		{`git clone https://me:s3cr3tpass@github.com/me/repo.git`, "github.com/me/repo.git"},
		{`echo sk-abcdefghijklmnopqrstuvwxyz`, "echo"},
		{`echo ghp_abcdefghijklmnopqrstuvwxyz123456`, "echo"},
	}
	for _, c := range cases {
		got := Redact(c.in)
		for _, leak := range []string{"abc123def456", "npm_abcdefghijklmnop", "SUPERSECRETVALUE", "hunter2", "s3cr3tpass", "sk-abcdefghijklmnopqrstuvwxyz", "ghp_abcdefghijklmnopqrstuvwxyz123456"} {
			if strings.Contains(got, leak) {
				t.Errorf("%q still contains %q: %q", c.in, leak, got)
			}
		}
		if !strings.Contains(got, c.mustKeep) {
			t.Errorf("%q lost %q: %q", c.in, c.mustKeep, got)
		}
	}
}

func TestRedactLeavesOrdinaryCommandsAlone(t *testing.T) {
	for _, s := range []string{"npm test", "go build ./...", "git status", "ls -la", "echo hello world", ""} {
		if got := Redact(s); got != s {
			t.Errorf("%q was changed to %q", s, got)
		}
	}
}
