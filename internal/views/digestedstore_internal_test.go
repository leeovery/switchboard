package views

import (
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// datesTaken is a ledger as the one it holds reads it that notes the date of
// each day whose lines a read of its sessions hands on, as a Follower hands
// on each line it reads, of whichever session.
type datesTaken struct {
	Ledger
	dates map[string]bool
}

func (c datesTaken) Sessions(ids []string, whole func(string) bool, take func(string, ledger.Line)) map[string]ledger.SessionFrom {
	return c.Ledger.Sessions(ids, whole, c.taking(take))
}

func (c datesTaken) SessionsOf(ids []string, whole func(string) bool, take func(string, ledger.Line)) map[string]ledger.SessionFrom {
	return c.Ledger.SessionsOf(ids, whole, c.taking(take))
}

// taking is take, where it's given, each line handed it noted by its day's
// date first.
func (c datesTaken) taking(take func(string, ledger.Line)) func(string, ledger.Line) {
	return func(date string, l ledger.Line) {
		c.dates[date] = true
		if take != nil {
			take(date, l)
		}
	}
}

// The List at half past one, once the day before's digest stands, over nine
// days whose sessions each ran on every one, and made a request today,
// reads today's file alone, every session resting on its state at the day
// before's end: where the same List read raw reads every day's. A day is
// told read by the lines its read hands on, and the day after today's holds
// none.
func TestAListOverDigestsReadsTheNearDaysFilesAlone(t *testing.T) {
	ids := []string{"a", "b", "c"}
	first, today := Today(foldDays[0]), Today(dayfile.DayStart(foldDays[0], 9))
	var lines []ledger.Line
	for i, id := range ids {
		session := renamed(generatedSession(uint64(i)), id)
		early := session[0]
		early.Request, early.At = "today-"+id, today.Start.Add(time.Duration(30+i)*time.Minute)
		lines = append(lines, dayAfterDay(session, 9)...)
		lines = append(lines, early)
	}
	slices.SortStableFunc(lines, func(a, b ledger.Line) int { return ended(&a).Compare(ended(&b)) })
	c := newDigestChain(t, "nine days", 0, lines)
	now := today.Start.Add(90 * time.Minute)
	c.at = now
	c.ledger.SummariseEnded(now)
	c.ledger.DigestEnded(t.Context(), now)
	clock := func() time.Time { return now }
	based := ledger.NewFollower(c.state, clock, ledger.CapsOf(nil, nil), DigestVersion, quiet)
	yesterday := yesterdayOf(today)
	for id, from := range based.SessionsOf(ids, func(string) bool { return false }, nil) {
		if from.Base == nil || from.Base.Date != yesterday {
			t.Fatalf("session %s rests on %+v, want its state at the end of %s", id, from.Base, yesterday)
		}
	}
	running := make([]status.Session, len(ids))
	for i, id := range ids {
		running[i] = status.Session{ID: id, Assignments: []status.Assignment{{Model: foldOpus, Account: "work", LastSeen: now}}}
	}
	tests := []struct {
		name    string
		version int
		read    []string
	}{
		{name: "over digests", version: DigestVersion, read: []string{today.Date}},
		{name: "read raw", version: DigestVersion + 1, read: dayfile.Span(first.Start, today.Start)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := datesTaken{Ledger: ledger.NewFollower(c.state, clock, ledger.CapsOf(nil, nil), tt.version, quiet), dates: make(map[string]bool)}

			list := ListSessions(SessionSources{Running: running, Ledger: l, Prices: ledger.Pricing, Now: now})

			if len(list.Sessions) != len(ids) {
				t.Errorf("the List lists %d sessions, want %d", len(list.Sessions), len(ids))
			}
			if dates := slices.Sorted(maps.Keys(l.dates)); !slices.Equal(dates, tt.read) {
				t.Errorf("the List read the lines of %v, want those of %v", dates, tt.read)
			}
		})
	}
}
