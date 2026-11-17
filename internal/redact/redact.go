// Package redact hides secrets in what switchboard shows and logs: a token it
// holds, and anything shaped like a Claude token, which a message from
// elsewhere can carry. The logs, the tokens package's tokens and the Claude
// provider all hide them with it, so a secret reads the same wherever it's
// hidden; and setup tells with it an answer that's a token pasted where it
// shows, which it never takes. It tells too whether a value may hold a
// secret before it's passed on, as the launcher asks of each value it keeps
// in a session's environment; and it hides the parts of an address that can
// carry a key, wherever the upstream's is shown.
package redact

import (
	"net/url"
	"regexp"
	"strings"
)

// Placeholder stands in for each secret hidden.
const Placeholder = "[redacted]"

// TokenPrefix begins every Claude API key and OAuth token.
const TokenPrefix = "sk-ant-"

// tokenShaped matches Claude API keys and OAuth tokens.
var tokenShaped = regexp.MustCompile(TokenPrefix + `[A-Za-z0-9_-]+`)

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
	if !strings.Contains(text, TokenPrefix) {
		return text
	}
	return tokenShaped.ReplaceAllString(text, Placeholder)
}

// URL returns address with its credentials, query and fragment, any of
// which can carry a key, each replaced by Placeholder: everything from the
// end of the scheme's :// to the last @, wherever it is, as a password can
// hold a ? or a #, then everything after the first ? or # that follows. It
// goes by the text alone, as the address may be no URL that parses, so an @
// in a path or a query hides the host too.
func URL(address string) string {
	from := 0
	if i := strings.Index(address, "://"); i >= 0 {
		from = i + len("://")
	}
	shown, rest := address[:from], address[from:]
	if at := strings.LastIndexByte(rest, '@'); at >= 0 {
		shown, rest = shown+Placeholder, rest[at:]
	}
	if i := strings.IndexAny(rest, "?#"); i >= 0 {
		rest = rest[:i+1] + Placeholder
	}
	return shown + rest
}

// URLError returns err, where it's a *url.Error, with its URL shown as URL
// shows it: a URL's parse, and an HTTP client's request, fail with one
// quoting the URL whole, its query and fragment included, the client's
// hiding the password alone. It's given the error as the call made it,
// as a wrap writes its text once, as it's made, the URL whole in it. It
// changes the error in place, as each is made afresh by the one call that
// fails with it, so nothing that unwraps one later finds the URL whole.
func URLError(err error) error {
	if failed, ok := err.(*url.Error); ok {
		failed.URL = URL(failed.URL)
	}
	return err
}
