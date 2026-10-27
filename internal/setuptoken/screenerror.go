package setuptoken

import (
	"fmt"

	"github.com/leeovery/switchboard/internal/redact"
)

// ScreenError is what Make fails with where setup-token gave no token it
// could read: it exited without making one, it showed an error it would have
// retried at, it didn't hand its sign-in address over, at each of which Make
// stops it, or its screen didn't show the token it said it had made. Its
// message says which, and never holds what setup-token drew, which can hold
// the token; Screen gives that, for the person to see, and to copy a token
// from to paste instead.
type ScreenError struct {
	reason string
	screen string
	// exit is how setup-token exited, where it exited by itself.
	exit error
}

// exitedWithout is setup-token, which drew screen, exiting as exit says
// without making a token.
func exitedWithout(screen string, exit error) *ScreenError {
	return &ScreenError{reason: "setup-token exited without making a token", screen: screen, exit: exit}
}

// failedAt is setup-token, which drew screen, showing the error message,
// which it would have retried at.
func failedAt(screen, message string) *ScreenError {
	reason := "setup-token showed an error"
	if message != "" {
		reason += ": " + redact.Text(message)
	}
	return &ScreenError{reason: reason, screen: screen}
}

// notHandedOver is setup-token, which drew screen, not handing its sign-in
// address over: not running the shim, or running it with something else.
func notHandedOver(screen string) *ScreenError {
	return &ScreenError{reason: "setup-token didn't hand switchboard its sign-in address", screen: screen}
}

// unreadable is setup-token, which drew screen, saying it made a token that
// screen doesn't show as one.
func unreadable(screen string) *ScreenError {
	return &ScreenError{reason: "setup-token said it made a token, but its screen didn't show one", screen: screen}
}

func (e *ScreenError) Error() string {
	if e.exit != nil {
		return fmt.Sprintf("%s (%v)", e.reason, e.exit)
	}
	return e.reason
}

// Unwrap returns how setup-token exited, where it exited by itself.
func (e *ScreenError) Unwrap() error {
	return e.exit
}

// Screen returns what setup-token last drew, as a terminal would have shown
// it, its escapes gone: a token it made included, for the person to copy. It's
// for the person alone, never a log.
func (e *ScreenError) Screen() string {
	return e.screen
}
