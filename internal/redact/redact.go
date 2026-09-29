// Package redact hides secrets in what switchboard shows and logs: a token it
// holds, and anything shaped like a Claude token, which a message from
// elsewhere can carry. The logs, the config's tokens and the Claude provider
// all hide them with it, so a secret reads the same wherever it's hidden.
package redact

import (
	"regexp"
	"strings"
)

// Placeholder stands in for each secret hidden.
const Placeholder = "[redacted]"

// tokenShaped matches Claude API keys and OAuth tokens, which all begin
// sk-ant-.
var tokenShaped = regexp.MustCompile(`sk-ant-[A-Za-z0-9_-]+`)

// Text returns text with each of secrets, and anything shaped like a Claude
// token, replaced by Placeholder.
func Text(text string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, Placeholder)
		}
	}
	return tokenShaped.ReplaceAllString(text, Placeholder)
}
