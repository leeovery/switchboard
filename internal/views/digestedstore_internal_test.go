package views

import (
	"errors"
	"io/fs"
	"maps"
	"math/rand/v2"
	"os"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// datesTaken is a ledger as the one it holds reads it that notes the date of
// each day whose lines a read of its sessions hands on, as a Follower hands
// on each line it reads, of whichever session; and the dates of the parts of
// the chain each session it gives rests on, oldest first, by its id, of those
// given any.
type datesTaken struct {
	Ledger
	dates map[string]bool
	parts map[string][]string
}

// takingDates is l, noting what datesTaken notes, none yet.
func takingDates(l Ledger) datesTaken {
	return datesTaken{Ledger: l, dates: make(map[string]bool), parts: make(map[string][]string)}
}

func (c datesTaken) Sessions(ids []string, rest func(string) ledger.Rest, take func(string, ledger.Line)) map[string]ledger.SessionFrom {
	return c.gave(c.Ledger.Sessions(ids, rest, c.taking(take)))
}

func (c datesTaken) SessionsOf(ids []string, rest func(string) ledger.Rest, take func(string, ledger.Line)) map[string]ledger.SessionFrom {
	return c.gave(c.Ledger.SessionsOf(ids, rest, c.taking(take)))
}

// gave is sessions, as a read gave them, the dates of each one's chain of
// parts noted.
func (c datesTaken) gave(sessions map[string]ledger.SessionFrom) map[string]ledger.SessionFrom {
	for id, from := range sessions {
		if dates := partDates(from); len(dates) > 0 {
			c.parts[id] = append(c.parts[id], dates...)
		}
	}
	return sessions
}

// partDates are the dates of the parts of the chain from rests on, oldest
// first: none where it's given none.
func partDates(from ledger.SessionFrom) []string {
	var dates []string
	for _, p := range from.Parts {
		dates = append(dates, p.Date)
	}
	return dates
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

// datesRead are the dates of the days whose lines c's reads of its sessions
// handed on, oldest first.
func (c datesTaken) datesRead() []string {
	return slices.Sorted(maps.Keys(c.dates))
}

// nineDays is a ledger of nine days whose sessions each ran on every one,
// and made a request on the day after them, digested at half past one that
// day, once the day before's digest stands: its first day and that day,
// that moment, and the sessions as the router runs them then.
type nineDays struct {
	chain        *digestChain
	first, today LedgerDay
	now          time.Time
	running      []status.Session
}

// newNineDays returns the nine days, their sessions those with the given
// ids, each a generated session of its own, each answer reading its
// account's windows, as limitedOf has them. Those left out are given no
// state in the day before's digest, as of one the Digests couldn't hold, so
// each rests on its state of the day before that.
func newNineDays(t *testing.T, ids []string, leftOut ...string) nineDays {
	t.Helper()
	first, today := Today(foldDays[0]), Today(dayfile.DayStart(foldDays[0], 9))
	var lines []ledger.Line
	for i, id := range ids {
		session := renamed(generatedSession(uint64(i)), id)
		early := session[0]
		early.Request, early.At = "today-"+id, today.Start.Add(time.Duration(30+i)*time.Minute)
		lines = append(lines, dayAfterDay(session, 9)...)
		lines = append(lines, early)
	}
	r := rand.New(rand.NewPCG(9, 9))
	for i := range lines {
		limitedOf(&lines[i], r, time.Time{})
	}
	slices.SortStableFunc(lines, func(a, b ledger.Line) int { return ended(&a).Compare(ended(&b)) })
	c := newDigestChain(t, "nine days", 0, lines)
	yesterday := yesterdayOf(today)
	c.digests.alter = func(date string, folded map[string]ledger.Folded) {
		if date == yesterday {
			for _, id := range leftOut {
				delete(folded, id)
			}
		}
	}
	now := today.Start.Add(90 * time.Minute)
	c.at = now
	c.ledger.SummariseEnded(now)
	c.ledger.DigestEnded(t.Context(), now)
	running := make([]status.Session, len(ids))
	for i, id := range ids {
		running[i] = status.Session{ID: id, Assignments: []status.Assignment{{Model: foldOpus, Account: "work", LastSeen: now}}}
	}
	return nineDays{chain: c, first: first, today: today, now: now, running: running}
}

// follower is a follower of the nine days' ledger at their moment, of the
// given version of the sessions' states.
func (n nineDays) follower(version int) *ledger.Follower {
	return ledger.NewFollower(n.chain.state, func() time.Time { return n.now }, ledger.CapsOf(nil, nil), version, quiet)
}

// list is the List of the nine days' sessions at their moment, read through
// l.
func (n nineDays) list(l Ledger) SessionList {
	return ListSessions(SessionSources{Running: n.running, Ledger: l, Prices: ledger.Pricing, Now: n.now})
}

// The List at half past one, once the day before's digest stands, over nine
// days whose sessions each ran on every one, and made a request today,
// reads today's file alone, every session resting on its state at the day
// before's end: where the same List read raw reads every day's. A day is
// told read by the lines its read hands on, and the day after today's holds
// none.
func TestAListOverDigestsReadsTheNearDaysFilesAlone(t *testing.T) {
	ids := []string{"a", "b", "c"}
	n := newNineDays(t, ids)
	yesterday := yesterdayOf(n.today)
	for id, from := range n.follower(DigestVersion).SessionsOf(ids, nil, nil) {
		if from.Base == nil || from.Base.Date != yesterday {
			t.Fatalf("session %s rests on %+v, want its state at the end of %s", id, from.Base, yesterday)
		}
	}
	tests := []struct {
		name    string
		version int
		read    []string
	}{
		{name: "over digests", version: DigestVersion, read: []string{n.today.Date}},
		{name: "read raw", version: DigestVersion + 1, read: dayfile.Span(n.first.Start, n.today.Start)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := takingDates(n.follower(tt.version))

			list := n.list(l)

			if len(list.Sessions) != len(ids) {
				t.Errorf("the List lists %d sessions, want %d", len(list.Sessions), len(ids))
			}
			if dates := l.datesRead(); !slices.Equal(dates, tt.read) {
				t.Errorf("the List read the lines of %v, want those of %v", dates, tt.read)
			}
		})
	}
}

// The List read by a reader of a newer version of the sessions' states than
// nine days' digests hold, as the first after a release raising
// DigestVersion, is the List read with no digest at all: every digest
// outdated, each session read raw.
func TestAListOverDigestsOfAnotherVersionIsTheListReadRaw(t *testing.T) {
	n := newNineDays(t, []string{"a", "b", "c"})
	newer := DigestVersion + 1
	outdated := n.list(n.follower(newer))
	for _, date := range dayfile.Span(n.first.Start, n.today.Start) {
		if err := os.Remove(n.chain.digestPath(date)); err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatal(err)
		}
	}

	raw := n.list(n.follower(newer))

	if len(raw.Sessions) == 0 || !reflect.DeepEqual(outdated, raw) {
		t.Errorf("over digests of another version, the List is\n%+v\nwant it as read with no digest, of sessions\n%+v", outdated, raw)
	}
}
