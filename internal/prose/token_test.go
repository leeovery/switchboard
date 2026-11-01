package prose_test

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
)

// A token's year ending is told as news, whole, with what makes a new token,
// and short, without its date: its days left, its last day, or that it's up,
// its date in now's time zone.
func TestATokensYearEndingIsToldAsNewsWholeAndShort(t *testing.T) {
	until := time.Date(2026, 11, 17, 23, 30, 0, 0, time.UTC)
	east := time.FixedZone("UTC+1", 60*60)
	tests := []struct {
		name              string
		ending            prose.TokenEnding
		news, told, short string
	}{
		{
			name:   "days left",
			ending: prose.TokenEnding{Now: time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC), Until: until, DaysLeft: 21},
			news:   "token expires 17 Nov 2026 · 21 days left",
			told:   "token expires 17 Nov 2026 · 21 days left — switchboard setup makes a new one",
			short:  "token: 21 days left",
		},
		{
			name:   "its last day, east of UTC, the next day there",
			ending: prose.TokenEnding{Now: time.Date(2026, 11, 18, 0, 10, 0, 0, east), Until: until},
			news:   "token expires 18 Nov 2026 · last day",
			told:   "token expires 18 Nov 2026 · last day — switchboard setup makes a new one",
			short:  "token: last day",
		},
		{
			name:   "up",
			ending: prose.TokenEnding{Now: time.Date(2026, 12, 2, 9, 0, 0, 0, time.UTC), Until: until, Up: true},
			news:   "token expired 17 Nov 2026",
			told:   "token expired 17 Nov 2026 — switchboard setup makes a new one",
			short:  "token expired",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := prose.TokenSaid(tt.ending.News()); got != tt.news {
				t.Errorf("News() = %q, want %q", got, tt.news)
			}
			if got := prose.TokenSaid(tt.ending.Told()); got != tt.told {
				t.Errorf("Told() = %q, want %q", got, tt.told)
			}
			if got := prose.TokenSaid(tt.ending.Short()); got != tt.short {
				t.Errorf("Short() = %q, want %q", got, tt.short)
			}
		})
	}
}

// A token's year ending is told in its parts, for one who shows them apart:
// its verb and its date, in now's time zone; the two whole; and the days
// left, none once it's up.
func TestATokensYearEndingIsToldInItsParts(t *testing.T) {
	until := time.Date(2026, 11, 17, 23, 30, 0, 0, time.UTC)
	east := time.FixedZone("UTC+1", 60*60)
	tests := []struct {
		name                    string
		ending                  prose.TokenEnding
		verb, date, whole, left string
		counted                 bool
	}{
		{name: "days left", ending: prose.TokenEnding{Now: time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC), Until: until, DaysLeft: 21}, verb: "expires", date: "17 Nov 2026", whole: "expires 17 Nov 2026", left: "21 days left", counted: true},
		{name: "its last day, east of UTC, the next day there", ending: prose.TokenEnding{Now: time.Date(2026, 11, 18, 0, 10, 0, 0, east), Until: until}, verb: "expires", date: "18 Nov 2026", whole: "expires 18 Nov 2026", left: "last day", counted: true},
		{name: "up", ending: prose.TokenEnding{Now: time.Date(2026, 12, 2, 9, 0, 0, 0, time.UTC), Until: until, Up: true}, verb: "expired", date: "17 Nov 2026", whole: "expired 17 Nov 2026"},
		{name: "up, east of UTC, the next day there", ending: prose.TokenEnding{Now: time.Date(2026, 12, 2, 9, 0, 0, 0, east), Until: until, Up: true}, verb: "expired", date: "18 Nov 2026", whole: "expired 18 Nov 2026"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if verb, date := tt.ending.Dated(); verb != tt.verb || date != tt.date {
				t.Errorf("Dated() = %q, %q; want %q, %q", verb, date, tt.verb, tt.date)
			}
			if got := tt.ending.Expires(); got != tt.whole {
				t.Errorf("Expires() = %q, want %q", got, tt.whole)
			}
			if left, counted := tt.ending.Left(); left != tt.left || counted != tt.counted {
				t.Errorf("Left() = %q, %v; want %q, %v", left, counted, tt.left, tt.counted)
			}
		})
	}
}
