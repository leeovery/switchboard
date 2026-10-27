package setuptoken

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/redact"
)

func TestScreenErrorKeepsTheScreenOutOfEveryWayItsShown(t *testing.T) {
	screen := madeScreen(" " + madeToken).text()
	// What's on the screen, as it is and as hex, as %x shows it.
	var secrets []string
	for _, secret := range []string{madeToken, "Store this token"} {
		secrets = append(secrets, secret, hex.EncodeToString([]byte(secret)))
	}
	tests := []struct {
		name string
		err  *ScreenError
	}{
		{name: "an exit without a token", err: exitedWithout(screen, errors.New("exit status 1"))},
		{name: "an error, its message holding a token", err: failedAt(screen, "Request failed for "+madeToken)},
		{name: "a token it can't read", err: unreadable(screen, nil)},
		{name: "a token it can't read, cancelled", err: unreadable(screen, context.Canceled)},
		{name: "no address handed over", err: notHandedOver(screen)},
		{name: "something else handed over", err: notAnAddress(screen)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logged strings.Builder
			slog.New(slog.NewTextHandler(&logged, nil)).Error("making a token", "error", tt.err, "value", *tt.err)
			shown := []string{tt.err.Error(), logged.String()}
			for _, verb := range []string{"%v", "%+v", "%s", "%q", "%x", "%#v", "%d"} {
				shown = append(shown, fmt.Sprintf(verb, tt.err), fmt.Sprintf(verb, *tt.err))
			}
			for _, s := range shown {
				if redact.HoldsToken(s) || slices.ContainsFunc(secrets, func(secret string) bool { return strings.Contains(s, secret) }) {
					t.Errorf("the error shows as %q, want nothing of the screen", s)
				}
			}
			if tt.err.Screen() != screen {
				t.Errorf("Screen() = %q, want the screen whole, token and all: %q", tt.err.Screen(), screen)
			}
		})
	}
}

func TestScreenErrorSaysWhatWentWrong(t *testing.T) {
	tests := []struct {
		name string
		err  *ScreenError
		want string
	}{
		{name: "an exit without a token", err: exitedWithout("", errors.New("exit status 1")), want: "setup-token exited without making a token (exit status 1)"},
		{name: "an exit without a token, of unknown status", err: exitedWithout("", nil), want: "setup-token exited without making a token"},
		{name: "an error", err: failedAt("", "Request failed with status code 400"), want: "setup-token showed an error: Request failed with status code 400"},
		{name: "an error without a message", err: failedAt("", ""), want: "setup-token showed an error"},
		{name: "a token it can't read", err: unreadable("", nil), want: "setup-token said it made a token, but its screen didn't show one"},
		{name: "a token it can't read, cancelled", err: unreadable("", context.Canceled), want: "setup-token said it made a token, but its screen didn't show one (context canceled)"},
		{name: "no address handed over", err: notHandedOver(""), want: "setup-token didn't hand switchboard its sign-in address"},
		{name: "something else handed over", err: notAnAddress(""), want: "setup-token handed switchboard something other than an https sign-in address"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestScreenErrorUnwrapsToWhatStoppedSetupToken(t *testing.T) {
	if err := unreadable("", fmt.Errorf("make a setup token: %w", context.Canceled)); !errors.Is(err, context.Canceled) {
		t.Errorf("unreadable(…, cancelled) = %v, want it to be the cancel", err)
	}
	if err := notHandedOver(""); errors.Unwrap(err) != nil {
		t.Errorf("notHandedOver() unwraps to %v, want nothing", errors.Unwrap(err))
	}
}
