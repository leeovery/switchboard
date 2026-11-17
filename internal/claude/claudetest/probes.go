package claudetest

import "github.com/leeovery/switchboard/internal/claude"

// The failures of a probe the API refused, as the provider gives them: an
// ended subscription's, its organization's login refused, the API's message
// cut to the 60 characters a probe's error keeps; and a refused token's.
var (
	ErrSubscriptionEnded = ReadRefused("HTTP 403 · OAuth authentication is currently not allowed for this organ", claude.SubscriptionEnded)
	ErrTokenRefused      = ReadRefused("HTTP 401 · Invalid bearer token", claude.TokenRefused)
)

// ReadRefused is the failure of a probe the API refused, as the provider
// gives it, saying text: its RefusedRead says why the API refused it, as
// refused has it, as claude.RefusedReadOf reads it.
func ReadRefused(text string, refused claude.ReadRefusal) error {
	return readRefused{text: text, refused: refused}
}

// readRefused is a probe's failure the API refused, as ReadRefused gives it.
type readRefused struct {
	text    string
	refused claude.ReadRefusal
}

func (e readRefused) Error() string {
	return e.text
}

// RefusedRead is why the API refused the probe.
func (e readRefused) RefusedRead() claude.ReadRefusal {
	return e.refused
}
