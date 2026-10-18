package ledger

import (
	"encoding/json"
	"testing"
	"time"
)

// A line's arrival is read as far as its at, wherever the line holds it, as
// a line read whole gives it; and a line that doesn't read as an object
// holding one gives none.
func TestALinesArrivalIsReadAsFarAsItsAt(t *testing.T) {
	at := time.Date(2026, 10, 5, 8, 30, 15, 123e6, time.UTC)
	written, err := json.Marshal((&Line{At: at, Request: "1", Kind: KindMessage, Session: "one"}).written())
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		line string
		at   time.Time
		ok   bool
	}{
		{name: "as the ledger writes it, at first", line: string(written), at: at, ok: true},
		{name: "at after the line's other fields", line: `{"request":"1","shape":{"at":"2026-01-01T00:00:00Z"},"at":"2026-10-05T08:30:15.123Z"}`, at: at, ok: true},
		{name: "cut short after its at", line: `{"at":"2026-10-05T08:30:15.123Z","request":"1`, at: at, ok: true},
		{name: "without an at", line: `{"request":"1","kind":"message"}`},
		{name: "an at that isn't a time", line: `{"at":"today"}`},
		{name: "not an object", line: `["at","2026-10-05T08:30:15.123Z"]`},
		{name: "not JSON", line: `at=2026-10-05T08:30:15.123Z`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := arrivalIn([]byte(tt.line))
			if !got.Equal(tt.at) || ok != tt.ok {
				t.Errorf("arrivalIn(%s) = %v, %t; want %v, %t", tt.line, got, ok, tt.at, tt.ok)
			}
		})
	}
}
