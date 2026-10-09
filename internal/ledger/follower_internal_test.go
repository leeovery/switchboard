package ledger

import "testing"

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
