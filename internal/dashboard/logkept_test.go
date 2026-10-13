package dashboard

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/theme"
)

// The columns shown are kept by name, in the table's order, at aside, and
// read back as they were: those shown at first as nothing, so a later
// build's stand; none, at alone, as none.
func TestTheLogsColumnsAreKeptByNameAndReadBack(t *testing.T) {
	tests := []struct {
		name   string
		ticked LogTicks
		kept   []string
	}{
		{name: "as the Log opens", kept: nil},
		{
			name: "worth shown", ticked: ticks(LogWorth),
			kept: []string{"session", "model", "on", "status", "first", "took", "read", "written", "out", "worth", "stopped", "routing"},
		},
		{
			name: "extras shown and routing hidden", ticked: ticks(LogRequestID, LogRouting, LogAgent),
			kept: []string{"session", "model", "on", "status", "first", "took", "read", "written", "out", "agent", "request id", "stopped"},
		},
		{
			name:   "at alone",
			ticked: ticks(LogSession, LogModel, LogOn, LogStatus, LogFirst, LogTook, LogRead, LogWritten, LogOut, LogStopped, LogRouting),
			kept:   []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kept := tt.ticked.Kept()
			if !slices.Equal(kept.Columns, tt.kept) || (kept.Columns == nil) != (tt.kept == nil) {
				t.Errorf("Kept() = %#v, want %#v", kept.Columns, tt.kept)
			}
			if back := LogKept(kept); back != tt.ticked {
				t.Errorf("LogKept(%#v) = %+v, want %+v", kept.Columns, back, tt.ticked)
			}
		})
	}
}

// A name the dashboard doesn't know, as a later build's column, shows
// nothing, and every column it doesn't name is hidden but at, named or not.
func TestALogColumnKeptByANameItDoesntKnowReadsAsHidden(t *testing.T) {
	got := LogKept(theme.LogPrefs{Columns: []string{"worth", "colour", "session", "at"}})
	var shown []LogColumn
	for _, c := range EveryLogColumn() {
		if got.Shows(c) {
			shown = append(shown, c)
		}
	}
	if want := []LogColumn{LogAt, LogSession, LogWorth}; !slices.Equal(shown, want) {
		t.Errorf("LogKept() shows %v, want %v", shown, want)
	}
	if kept := got.Kept().Columns; !slices.Equal(kept, []string{"session", "worth"}) {
		t.Errorf("Kept() = %q, want the names it knows, in the table's order, at aside", kept)
	}
}

// at always shows, and isn't counted among the columns shown, as the
// panel's "in 11 columns" counts them.
func TestAtAlwaysShowsAndIsntCounted(t *testing.T) {
	tests := []struct {
		name   string
		ticked LogTicks
		count  int
	}{
		{name: "as the Log opens", count: 11},
		{name: "worth shown", ticked: ticks(LogWorth), count: 12},
		{name: "at toggled", ticked: ticks(LogAt), count: 11},
		{name: "routing hidden", ticked: ticks(LogRouting), count: 10},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.ticked.Shows(LogAt) {
				t.Error("Shows(at) = false, want at always shown")
			}
			if got := tt.ticked.Count(); got != tt.count {
				t.Errorf("Count() = %d, want %d", got, tt.count)
			}
		})
	}
}
