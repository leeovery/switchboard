package setuptoken

import (
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/redact"
)

func TestScreenErrorKeepsTheScreenOutOfItsMessage(t *testing.T) {
	screen := madeScreen(" " + madeToken).text()
	tests := []struct {
		name string
		err  *ScreenError
	}{
		{name: "an exit without a token", err: exitedWithout(screen, errors.New("exit status 1"))},
		{name: "an error, its message holding a token", err: failedAt(screen, "Request failed for "+madeToken)},
		{name: "a token it can't read", err: unreadable(screen)},
		{name: "no address handed over", err: notHandedOver(screen)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var logged strings.Builder
			slog.New(slog.NewTextHandler(&logged, nil)).Error("making a token", "error", tt.err)
			for _, shown := range []string{tt.err.Error(), fmt.Sprintf("%v %+v %s", tt.err, tt.err, tt.err), logged.String()} {
				if redact.HoldsToken(shown) || strings.Contains(shown, "Store this token") {
					t.Errorf("the error shows as %q, want nothing of the screen", shown)
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
		{name: "a token it can't read", err: unreadable(""), want: "setup-token said it made a token, but its screen didn't show one"},
		{name: "no address handed over", err: notHandedOver(""), want: "setup-token didn't hand switchboard its sign-in address"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}
