package setuptoken

import (
	"fmt"
	"log/slog"
	"strings"
	"testing"
)

const signIn = "https://claude.example/oauth/authorize?code_challenge=test-challenge&state=test-state-work"

func TestAddressNeverShowsItselfButRevealed(t *testing.T) {
	address, ok := ParseAddress(signIn)
	if !ok {
		t.Fatalf("ParseAddress(%q) refused it, want an address", signIn)
	}
	var logged strings.Builder
	slog.New(slog.NewTextHandler(&logged, nil)).Info("opened", "address", address, "event", Event{Step: StepAddress, Address: address})
	for _, shown := range []string{fmt.Sprintf("%v %+v %s %#v %v", address, address, address, address, Event{Address: address}), logged.String()} {
		if strings.Contains(shown, "test-state-work") {
			t.Errorf("the address shows as %q, want a placeholder", shown)
		}
	}
	if address.Reveal() != signIn {
		t.Errorf("Reveal() = %q, want %q", address.Reveal(), signIn)
	}
}

func TestNewAddressTakesAnHTTPSAddressAlone(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "an https address", raw: signIn, want: true},
		{name: "an http address", raw: "http://claude.example/oauth/authorize"},
		{name: "a file", raw: "/System/Applications/Calculator.app"},
		{name: "a file's address", raw: "file:///System/Applications/Calculator.app"},
		{name: "an https address without a host", raw: "https:///oauth/authorize"},
		{name: "an option", raw: "-a Calculator"},
		{name: "nothing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, got := ParseAddress(tt.raw); got != tt.want {
				t.Errorf("ParseAddress(%q) took it %v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}
