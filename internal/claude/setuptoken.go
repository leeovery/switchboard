package claude

import (
	"regexp"
	"slices"
)

// What claude setup-token does and draws, as Claude Code 2.1.311 has it. It
// takes no options: it opens a sign-in address in the browser, whose page
// returns to a localhost callback it listens on, makes the token once the
// sign-in is approved there, shows it once, and saves it nowhere.
const (
	// SetupTokenCommand is the subcommand that makes a setup token: an OAuth
	// token that lasts a year.
	SetupTokenCommand = "setup-token"
	// BrowserEnv names the program setup-token opens its sign-in address
	// with, the address its one argument, run without a shell.
	BrowserEnv = "BROWSER"
	// SetupTokenMade is the line setup-token shows once it has made the
	// token, which it shows on a line of its own below.
	SetupTokenMade = "Long-lived authentication token created successfully!"
	// SetupTokenError begins the line setup-token shows a sign-in's error on.
	SetupTokenError = "OAuth error:"
	// SetupTokenBelow begins the line setup-token shows next below the token
	// it made, past a blank one.
	SetupTokenBelow = "Store this token securely."
	// SetupTokenRetry is what setup-token shows below an error it can retry,
	// and waits at.
	SetupTokenRetry = "Press Enter to retry"
)

var (
	// setupToken matches a setup token whole: an OAuth access token, as
	// setup-token makes, never the refresh token Claude Code's own scanner
	// matches too.
	setupToken = regexp.MustCompile(`^sk-ant-oat\d{2}-[A-Za-z0-9_-]{20,}$`)
	// setupTokenPart matches what a setup token is made of, and nothing
	// else.
	setupTokenPart = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
)

// IsSetupToken reports whether text is a setup token, whole, and nothing
// else. The start of one is one too, as its length isn't fixed: see
// IsSetupTokenPart.
func IsSetupToken(text string) bool {
	return setupToken.MatchString(text)
}

// IsSetupTokenPart reports whether text could be part of a setup token, as
// the rest of one broken across two rows is: what one is made of, and nothing
// else.
func IsSetupTokenPart(text string) bool {
	return setupTokenPart.MatchString(text)
}

// SetupTokenUnset returns the variables setup-token is started without: the
// base URL, which would send what it asks of the API elsewhere, and a token
// or a key, any of which only adds a warning to what it draws.
func SetupTokenUnset() []string {
	return slices.Concat([]string{BaseURLEnv, TokenEnv}, keyEnvs)
}
