package ledger

import (
	"fmt"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"
	"time"
)

// Reading days ahead reads those of its window side by side, and however
// long a day before them takes, never more than its window ahead of the day
// it hands on, each handed on with its lines in the order given.
func TestReadingDaysAheadReadsNoMoreThanItsWindowAheadOfTheDayHandedOn(t *testing.T) {
	const ahead = 3
	var dates []string
	for day := range 12 {
		dates = append(dates, fmt.Sprintf("2026-10-%02d", day+1))
	}
	var begun atomic.Int32
	// beside holds every day's, so no read waits on the first.
	beside := make(chan string, len(dates))
	read := func(date string) []Line {
		begun.Add(1)
		if date != dates[0] {
			beside <- date
			return []Line{{Request: date}}
		}
		deadline := time.After(10 * time.Second)
		for range ahead - 1 {
			select {
			case <-beside:
			case <-deadline:
				t.Error("the first day's read waits on the rest of its window's, want them read beside it")
				return []Line{{Request: date}}
			}
		}
		// The first day takes a while yet, for any read begun past the window
		// to show.
		time.Sleep(20 * time.Millisecond)
		return []Line{{Request: date}}
	}
	var taken []string
	readAhead(dates, ahead, read, func(date string, lines []Line) {
		if n, most := int(begun.Load()), len(taken)+1+ahead; n > most {
			t.Errorf("as %s is handed on, %d days' reads are begun, want %d at most: those handed on, it, and %d ahead", date, n, most, ahead)
		}
		if len(lines) != 1 || lines[0].Request != date {
			t.Errorf("%s is handed on with %+v, want its own lines", date, lines)
		}
		taken = append(taken, date)
	})
	if !slices.Equal(taken, dates) {
		t.Errorf("the days are handed on as %q, want them in the order given, %q", taken, dates)
	}
}

// A read of a session's lines holds its own, read whole, with their JSON,
// and reads any other for its session alone, giving none of it, as a read
// of a few sessions does.
func TestASessionsLinesAloneAreHeldWithTheirJSON(t *testing.T) {
	tests := []struct {
		name, line string
		// read is whether the line reads as one, and held whether it's held
		// with its JSON, read whole as lineIn reads it.
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
			want, _ := lineIn([]byte(tt.line))
			if !tt.held {
				want = Line{}
			}
			if !reflect.DeepEqual(h.Line, want) {
				t.Errorf("gave %+v, want %+v", h.Line, want)
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
