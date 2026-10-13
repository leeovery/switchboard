package views_test

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/views"
)

func TestARequestToldOfIsInFlightTillItEndsOrIsLost(t *testing.T) {
	tests := []struct {
		name string
		told views.Told
		want bool
	}{
		{name: "sent", told: views.Told{Sent: now}, want: true},
		{name: "refused, before it goes out again", told: views.Told{Sent: now, Verdict: views.VerdictLimited, Status: 429}, want: true},
		{name: "ended", told: views.Told{Sent: now, Ended: now.Add(time.Second)}},
		{name: "lost as the stream was forgotten", told: views.Told{Sent: now, Lost: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.told.InFlight(); got != tt.want {
				t.Errorf("InFlight = %t, want %t", got, tt.want)
			}
		})
	}
}

func TestARequestToldOfSpendsQuotaButAQuotaCheckOrACountOfTokens(t *testing.T) {
	tests := []struct {
		name string
		told views.Told
		want bool
	}{
		{name: "a message", told: views.Told{}, want: true},
		{name: "Claude Code's quota check", told: views.Told{Check: true}},
		{name: "a count of tokens", told: views.Told{Count: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.told.Spends(); got != tt.want {
				t.Errorf("Spends = %t, want %t", got, tt.want)
			}
		})
	}
}
