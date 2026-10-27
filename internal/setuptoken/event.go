package setuptoken

import (
	"log/slog"
	"net/url"

	"github.com/leeovery/switchboard/internal/redact"
)

// Step is a step of a token's making, as Make reports it.
type Step int

// The steps Make reports, in the order they come. The zero Step is none.
const (
	// StepAddress is the sign-in address ready: opened, as the event's
	// Opened says, or for the caller to show.
	StepAddress Step = iota + 1
	// StepWaiting is setup-token waiting on the sign-in's approval in the
	// browser.
	StepWaiting
	// StepMade is setup-token saying it has made the token.
	StepMade
)

// Opened is how the sign-in address was opened.
type Opened int

// The ways the sign-in address opens. The zero Opened is the caller's to
// show.
const (
	// NotOpened leaves the address for the caller to show, for the person to
	// open themselves, on this machine, where its page returns to.
	NotOpened Opened = iota
	// OpenedInBrowser is the address opened in the default browser as it is,
	// signed in to whichever account it's signed in to.
	OpenedInBrowser
	// OpenedPrivately is the address opened in a private window of the
	// default browser, which leaves the browser's own session alone.
	OpenedPrivately
)

// Event is a step of a token's making, as Make reports it.
type Event struct {
	Step Step
	// Address is the sign-in address, at StepAddress.
	Address Address
	// Opened is how Address was opened, at StepAddress. Where it's
	// NotOpened, Unopened says why the Opener couldn't open it, such as a
	// *NoPrivateWindowError.
	Opened   Opened
	Unopened error
}

// Address is a sign-in address. What it carries proves the sign-in is the one
// setup-token started, so it's never logged: printing or logging one shows a
// placeholder, and only Reveal gives it, to open or show.
type Address struct {
	// A pointer, as tokens.Token's secret is, for the same reason.
	url *string
}

// newAddress returns raw as a sign-in address, as setup-token hands one over:
// an https address. Anything else, which open would open as a file or an
// application, isn't one.
func newAddress(raw string) (Address, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" {
		return Address{}, false
	}
	return Address{url: &raw}, true
}

// Reveal returns the address itself, to open or show.
func (a Address) Reveal() string {
	if a.url == nil {
		return ""
	}
	return *a.url
}

// String returns a placeholder, never the address.
func (a Address) String() string {
	return redact.Placeholder
}

// LogValue returns the same placeholder as String.
func (a Address) LogValue() slog.Value {
	return slog.StringValue(a.String())
}
