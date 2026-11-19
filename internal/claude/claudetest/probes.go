package claudetest

import (
	"net/http"

	"github.com/leeovery/switchboard/internal/quota"
)

// The failures of a probe the API refused, as the provider gives them: an
// ended subscription's, its organization's login refused, the API's message
// cut to the 60 characters a probe's error keeps; and a refused token's.
var (
	ErrSubscriptionEnded = ReadRefused(http.StatusForbidden, "HTTP 403 · OAuth authentication is currently not allowed for this organ", quota.SubscriptionEnded)
	ErrTokenRefused      = ReadRefused(http.StatusUnauthorized, "HTTP 401 · Invalid bearer token", quota.TokenRefused)
)

// ReadRefused is the failure of a probe the API refused, answering status,
// as the provider gives it, saying text: its RefusedRead says why the API
// refused it, as refused has it, and its APIStatus the status.
func ReadRefused(status int, text string, refused quota.ReadRefusal) error {
	return readRefused{status: status, text: text, refused: refused}
}

// readRefused is a probe's failure the API refused, as ReadRefused gives it.
type readRefused struct {
	status  int
	text    string
	refused quota.ReadRefusal
}

func (e readRefused) Error() string {
	return e.text
}

// RefusedRead is why the API refused the probe.
func (e readRefused) RefusedRead() quota.ReadRefusal {
	return e.refused
}

// APIStatus is the status the API answered the probe with.
func (e readRefused) APIStatus() int {
	return e.status
}
