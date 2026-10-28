package setuptoken

import "fmt"

// ScreenError is what Make fails with where setup-token gave no token it
// could read: it exited without making one, it showed an error it would have
// retried at, it didn't hand its sign-in address over, or handed over
// something else, at each of which Make stops it, or it stopped before its
// screen showed whole the token it said it had made. Its message says which,
// and never holds what setup-token drew, which can hold the token; Screen
// gives that, for the person to see, and to copy a token from to paste
// instead.
type ScreenError struct {
	reason string
	// screen is a pointer, as tokens.Token's secret is, so that no way fmt
	// prints the error, or the error's value, prints the token it can hold.
	screen *string
	// cause is what stopped setup-token, where something did: its exit, or
	// the end of Make's context.
	cause error
}

// newScreenError is setup-token, which drew screen, giving no token for
// reason, cause having stopped it, where it's not nil.
func newScreenError(reason, screen string, cause error) *ScreenError {
	return &ScreenError{reason: reason, screen: &screen, cause: cause}
}

// exitedWithout is setup-token, which drew screen, exiting as exit says
// without making a token.
func exitedWithout(screen string, exit error) *ScreenError {
	return newScreenError("setup-token exited without making a token", screen, exit)
}

// failedAt is setup-token, which drew screen, showing an error it would have
// retried at. Its words stay on the screen: setup-token's own, they could
// hold what's no error's to say.
func failedAt(screen string) *ScreenError {
	return newScreenError("setup-token showed an error", screen, nil)
}

// notHandedOver is setup-token, which drew screen, not handing its sign-in
// address over.
func notHandedOver(screen string) *ScreenError {
	return newScreenError("setup-token didn't hand switchboard its sign-in address", screen, nil)
}

// notAnAddress is setup-token, which drew screen, handing over in place of its
// sign-in address something that isn't an https address.
func notAnAddress(screen string) *ScreenError {
	return newScreenError("setup-token handed switchboard something other than an https sign-in address", screen, nil)
}

// unreadable is setup-token, which drew screen, saying it made a token that
// screen doesn't show whole, cause having stopped it drawing it, where it's
// not nil.
func unreadable(screen string, cause error) *ScreenError {
	return newScreenError("setup-token said it made a token, but its screen didn't show one", screen, cause)
}

func (e *ScreenError) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("%s (%v)", e.reason, e.cause)
	}
	return e.reason
}

// Unwrap returns what stopped setup-token, where something did: its exit, as
// an *exec.ExitError, or the end of Make's context.
func (e *ScreenError) Unwrap() error {
	return e.cause
}

// Screen returns what setup-token last drew, as a terminal would have shown
// it, its escapes gone: a token it made included, for the person to copy. It's
// for the person alone, never a log.
func (e *ScreenError) Screen() string {
	if e.screen == nil {
		return ""
	}
	return *e.screen
}
