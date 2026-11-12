package idmove

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// The router writes its lines with json.Marshal, which escapes <, > and &:
// a reason renamed is written so too, so every byte but the ids is as the
// router would write the line renamed.
func TestALineRenamedIsWrittenAsTheRouterWritesIt(t *testing.T) {
	line := func(account, reason string) []byte {
		data, err := json.Marshal(ledger.Line{At: time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC), Request: "r1", Kind: ledger.KindMessage,
			Account: account, Reason: reason, Status: 200, Attempts: 1})
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	rename := func(id string) string {
		if id == "work" {
			return "wk7q"
		}
		return id
	}
	was := line("work", status.ReasonPinYields+"work <has> no room & more")
	got, changed := lineNames.renamed(was, rename)
	if want := line("wk7q", status.ReasonPinYields+"wk7q <has> no room & more"); !changed || string(got) != string(want) {
		t.Errorf("renamed, the line reads\n%s\nwant\n%s", got, want)
	}
}

func TestEachPoolsPinInTheStateFileIsRenamed(t *testing.T) {
	ids := map[string]string{"work": "wk7q", "side": "sd3m"}
	rename := func(id string) string {
		if now, ok := ids[id]; ok {
			return now
		}
		return id
	}
	was := `{"version": 1, "pins": {"work": {"accounts": ["work", "side"], "since": "2026-11-08T09:00:00Z", "move": true}}}`
	got, changed := stateNames.renamed([]byte(was), rename)
	if want := `{"version": 1, "pins": {"work": {"accounts": ["wk7q", "sd3m"], "since": "2026-11-08T09:00:00Z", "move": true}}}`; !changed || string(got) != want {
		t.Errorf("renamed, the state file reads\n%s\nwant\n%s: each pool's pin's accounts renamed, the pool's id as it was", got, want)
	}
}

// marshalled is v as the router writes it.
func marshalled(t *testing.T, v any) []byte {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// A line given a pool is written as the router writes one routed in it: the
// pool where its type marshals it, every other byte as it was.
func TestALineGivenAPoolIsWrittenAsTheRouterWritesOneRoutedInIt(t *testing.T) {
	at := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name      string
		n         names
		was, want any
	}{
		{
			name: "a request's, of a session and a model",
			n:    lineNames,
			was:  ledger.Line{At: at, Request: "r1", Kind: ledger.KindMessage, Session: "s1", Model: "claude-opus-5-5", Account: "b6s7", Reason: "sticky", Status: 200},
			want: ledger.Line{At: at, Request: "r1", Kind: ledger.KindMessage, Session: "s1", Model: "claude-opus-5-5", Pool: "b2n8", Account: "b6s7", Reason: "sticky", Status: 200},
		},
		{
			name: "a request's the router answered itself, of no session",
			n:    lineNames,
			was:  ledger.Line{At: at, Request: "r2", Kind: ledger.KindMessage, Reason: "no account has room", Status: 429},
			want: ledger.Line{At: at, Request: "r2", Kind: ledger.KindMessage, Pool: "b2n8", Reason: "no account has room", Status: 429},
		},
		{
			name: "a pin's",
			n:    eventNames,
			was:  status.Event{ID: 1, At: at, Kind: status.EventPin, Account: "b6s7", Accounts: []string{"b6s7"}, By: "cli"},
			want: status.Event{ID: 1, At: at, Kind: status.EventPin, Account: "b6s7", Accounts: []string{"b6s7"}, Pool: "b2n8", By: "cli"},
		},
		{
			name: "an auto's, of a session",
			n:    eventNames,
			was:  status.Event{ID: 2, At: at, Kind: status.EventAuto, Session: "s1"},
			want: status.Event{ID: 2, At: at, Kind: status.EventAuto, Pool: "b2n8", Session: "s1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := marshalled(t, tt.want)
			if got, given := tt.n.pool.pooled(marshalled(t, tt.was), "b2n8"); !given || string(got) != string(want) {
				t.Errorf("given a pool, the line reads\n%s\nwant\n%s", got, want)
			}
		})
	}
}

// A line that names a pool already, a line of a kind given none, as an
// event but a pin's or an auto's, and one that isn't one JSON object, are
// left as they are.
func TestALineThatNamesAPoolOrIsGivenNoneIsLeftAsItIs(t *testing.T) {
	at := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	for _, tt := range []struct {
		name string
		n    names
		line []byte
	}{
		{name: "a request's naming a pool", n: lineNames, line: marshalled(t, ledger.Line{At: at, Request: "r1", Kind: ledger.KindMessage, Pool: "q7mt", Reason: "sticky"})},
		{name: "a limit's event", n: eventNames, line: marshalled(t, status.Event{ID: 1, At: at, Kind: status.EventLimit, Account: "b6s7"})},
		{name: "a reading's", n: readingNames, line: []byte(`{"account":"b6s7","key":"five_hour"}`)},
		{name: "a line cut short", n: lineNames, line: []byte(`{"at":"2026-11-08T12:00:00Z","requ`)},
		{name: "two objects", n: lineNames, line: []byte(`{"at":"x"}{"at":"y"}`)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got, given := tt.n.pool.pooled(tt.line, "b2n8"); given || string(got) != string(tt.line) {
				t.Errorf("the line reads\n%s\nwant it as it was\n%s", got, tt.line)
			}
		})
	}
}
