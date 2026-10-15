package ledger

import (
	"reflect"
	"testing"
)

func TestASessionsLinesAloneAreHeldWithTheirJSON(t *testing.T) {
	tests := []struct {
		name, line string
		// read is whether the line reads as one, and held whether it's held
		// with its JSON.
		read, held bool
	}{
		{name: "the session's", line: `{"at":"2026-10-05T09:00:00Z","request":"1","kind":"message","session":"one","reason":"sticky"}`, read: true, held: true},
		{name: "another session's", line: `{"at":"2026-10-05T09:00:00Z","request":"2","kind":"message","session":"two","reason":"sticky"}`, read: true},
		{name: "one of no session", line: `{"at":"2026-10-05T09:00:00Z","request":"3","kind":"count","reason":"sticky"}`, read: true},
		{name: "one cut short", line: `{"at":"2026-10-05T09:00:00Z","requ`},
	}
	decode := sessionHeldIn("one")
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, read := decode([]byte(tt.line))
			if read != tt.read || (h.JSON != nil) != tt.held || tt.held && string(h.JSON) != tt.line {
				t.Errorf("read %v, its JSON %q; want read %v, its JSON held %v", read, h.JSON, tt.read, tt.held)
			}
		})
	}
}

// A read of a few sessions reads the lines of those it keeps whole, and of
// any other its session alone, giving none of it, but tells a line that
// doesn't read as one as lineIn does, whosever it is.
func TestALineOfASessionNotKeptIsReadForItsSessionAlone(t *testing.T) {
	tests := []struct {
		name, line string
		// read is whether the line reads as one, and whole whether it's given
		// as lineIn gives it.
		read, whole bool
	}{
		{name: "a kept session's", line: `{"at":"2026-10-05T09:00:00Z","request":"1","kind":"message","session":"one","reason":"sticky"}`, read: true, whole: true},
		{name: "another session's", line: `{"at":"2026-10-05T09:00:00Z","request":"2","kind":"message","session":"two","reason":"sticky"}`, read: true},
		{name: "one of no session", line: `{"at":"2026-10-05T09:00:00Z","request":"3","kind":"count","reason":"sticky"}`, read: true},
		{name: "another session's, cut short", line: `{"at":"2026-10-05T09:00:00Z","request":"4","kind":"message","session":"two","rea`},
		{name: "one whose session isn't one", line: `{"at":"2026-10-05T09:00:00Z","request":"5","kind":"message","session":2}`},
	}
	decode := keptLineIn(map[string]bool{"one": true})
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, read := decode([]byte(tt.line))
			want, _ := lineIn([]byte(tt.line))
			if !tt.whole {
				want = Line{}
			}
			if read != tt.read || !reflect.DeepEqual(got, want) {
				t.Errorf("read %v, giving %+v; want read %v, giving %+v", read, got, tt.read, want)
			}
		})
	}
}
