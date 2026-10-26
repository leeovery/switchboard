// Package redact hides secrets in what switchboard shows and logs: a token it
// holds, and anything shaped like a Claude token, which a message from
// elsewhere can carry. The logs, the tokens package's tokens and the Claude
// provider all hide them with it, so a secret reads the same wherever it's
// hidden; and setup tells with it an answer that's a token pasted where it
// shows, which it never takes. It tells too whether a value may hold a
// secret before it's passed on, as the launcher asks of each value it keeps
// in a session's environment.
package redact

import (
	"regexp"
	"strings"
)

// Placeholder stands in for each secret hidden.
const Placeholder = "[redacted]"

// tokenPrefix begins every Claude API key and OAuth token.
const tokenPrefix = "sk-ant-"

// tokenShaped matches Claude API keys and OAuth tokens.
var tokenShaped = regexp.MustCompile(tokenPrefix + `[A-Za-z0-9_-]+`)

// HoldsToken reports whether text holds anything shaped like a Claude token.
func HoldsToken(text string) bool {
	return tokenShaped.MatchString(text)
}

// HoldsSecret reports whether value, such as a variable's, may hold a secret:
// anything shaped like a Claude token, or an @ or a ?, which credentials in a
// URL's userinfo, as https://user:secret@gateway.example.com, and a query,
// which can carry a key, as ?key=…, each need. It parses no URL, as the
// program the value is passed to may read one otherwise: Node's URL takes
// https:user:pass@host as userinfo, where Go's reads it as opaque. An @ in a
// URL's path, or a bare ?, reads as one too.
func HoldsSecret(value string) bool {
	return HoldsToken(value) || strings.ContainsAny(value, "@?")
}

// Text returns text with each of secrets, and anything shaped like a Claude
// token, replaced by Placeholder.
func Text(text string, secrets ...string) string {
	for _, secret := range secrets {
		if secret != "" {
			text = strings.ReplaceAll(text, secret, Placeholder)
		}
	}
	// Searching by the pattern allocates, however long the text, and almost
	// none holds a token.
	if !strings.Contains(text, tokenPrefix) {
		return text
	}
	return tokenShaped.ReplaceAllString(text, Placeholder)
}
