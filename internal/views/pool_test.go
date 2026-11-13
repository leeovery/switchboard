package views_test

import (
	"errors"
	"io/fs"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// acmeRead reads Acme, q7mt, where work is now, Default, b2n8, the first
// pool, holding side and spare; the accounts record keeps lapsed, removed
// from Acme.
var acmeRead = views.PoolRead{
	ID: "q7mt", First: "b2n8",
	PoolOf: status.AccountPools(config.Accounts{{ID: "work", Pool: "q7mt"}, {ID: "side", Pool: "b2n8"}, {ID: "spare", Pool: "b2n8"}}, func() identity.Record {
		return identity.Record{Accounts: []identity.Entry{{ID: "lapsed", Pool: "q7mt"}}}
	}),
}

// A pool read shows an event as status.Event's InPool has it, whose own
// tests hold the rule whole: these hold that the read asks it, of its pool,
// its first and its accounts' pools.
func TestAPoolReadShowsTheEventsOfThePoolAsTheEventsRuleHasIt(t *testing.T) {
	tests := []struct {
		name  string
		event status.Event
	}{
		{name: "a start in the pool", event: status.Event{Kind: status.EventStarted, Account: "side", Pool: "q7mt"}},
		{name: "a start in another, on an account in the pool now", event: status.Event{Kind: status.EventStarted, Account: "work", Pool: "b2n8"}},
		{name: "a start in a pool since removed", event: status.Event{Kind: status.EventStarted, Account: "work", Pool: "x9z9"}},
		{name: "a pin naming no pool, from before pins named theirs", event: status.Event{Kind: status.EventPin, Account: "work", Accounts: []string{"work"}}},
		{name: "a move naming no pool, from before pools", event: status.Event{Kind: status.EventMoved, From: "side", To: "work"}},
		{name: "a limit of an account in the pool now", event: status.Event{Kind: status.EventLimit, Account: "work"}},
		{name: "a limit of an account in another pool now", event: status.Event{Kind: status.EventLimit, Account: "side"}},
		{name: "a refusal of an account removed from the pool", event: status.Event{Kind: status.EventRefused, Account: "lapsed"}},
		{name: "a limit of an account none places", event: status.Event{Kind: status.EventLimit, Account: "gone"}},
		{name: "the router's own", event: status.Event{Kind: status.EventRestart}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := tt.event.InPool(acmeRead.ID, acmeRead.First, acmeRead.PoolOf)
			if got := acmeRead.Shows(tt.event); got != want {
				t.Errorf("Shows(%+v) = %v, want %v, as InPool has it", tt.event, got, want)
			}
			if got := acmeRead.ShowsHeld(events.Held{Event: tt.event}); got != want {
				t.Errorf("ShowsHeld(%+v) = %v, want %v, as Shows has it", tt.event, got, want)
			}
			if got := (views.PoolRead{}).Shows(tt.event); !got {
				t.Errorf("a read of every pool's Shows(%+v) = false, want every event shown", tt.event)
			}
		})
	}
	// The rule's answers, as the read gives them, for its own pool.
	shown := map[string]bool{
		"a start in the pool": true, "a limit of an account in the pool now": true,
		"a refusal of an account removed from the pool": true, "a limit of an account none places": true, "the router's own": true,
	}
	for _, tt := range tests {
		if got := acmeRead.Shows(tt.event); got != shown[tt.name] {
			t.Errorf("%s: Acme's read shows it %v, want %v", tt.name, got, shown[tt.name])
		}
	}
}

// A pool's days hold the accounts that worked in it that day, each with its
// work there alone, and of those idle in it, the ones status.AccountIn has
// as the pool's, PoolOf placing them there or nowhere; the days the reader
// read stay as they were.
func TestAPoolsDaysHoldItsWorkAndItsOwnIdleAccounts(t *testing.T) {
	day := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	whole := ledger.Summary{Day: "2026-11-03", Accounts: []ledger.AccountDay{
		{Account: "work"},
		{Account: "side", Sessions: 3, Pools: []ledger.PoolDay{
			{Pool: "b2n8", Sessions: 1}, {Pool: "q7mt", Sessions: 2},
		}},
		{Account: "lapsed"}, {Account: "gone"}, {Account: "spare", Pool: "b2n8", Sessions: 4}, {},
	}}
	read := givenDays{whole}
	days := acmeRead.Days(read).Days(day)
	if len(days) != 1 {
		t.Fatalf("Acme's days are %d, want 1", len(days))
	}
	sessions := map[string]int{}
	var kept []string
	for _, a := range days[0].Accounts {
		kept = append(kept, a.Account)
		sessions[a.Account] = a.Sessions
	}
	if want := []string{"work", "side", "lapsed", "gone"}; !slices.Equal(kept, want) {
		t.Errorf("Acme's day holds %q, want %q: side, which worked in it, and of those idle, the one it holds, the removed one the record keeps in it and the one none places", kept, want)
	}
	if sessions["side"] != 2 {
		t.Errorf("side's day in Acme counts %d sessions, want its 2 there alone", sessions["side"])
	}
	if len(read[0].Accounts) != 6 || read[0].Accounts[1].Sessions != 3 {
		t.Errorf("the reader's day holds %d accounts, side's %d sessions, once Acme's was read; want its 6 as they were, side's 3",
			len(read[0].Accounts), read[0].Accounts[1].Sessions)
	}
	if every := (views.PoolRead{}).Days(read).Days(day); len(every[0].Accounts) != 6 {
		t.Errorf("every pool's day holds %d accounts, want all 6", len(every[0].Accounts))
	}
}

// A pool's windows are those of the accounts status.AccountIn has as the
// pool's, the one removed from it and the one none places among them; every
// pool's are every one.
func TestAPoolsWindowsAreThoseOfItsAccounts(t *testing.T) {
	all := views.Windows{Windows: []views.WindowReadings{
		{Account: "work", Window: "5h"}, {Account: "side", Window: "5h"}, {Account: "lapsed", Window: "7d"}, {Account: "gone", Window: "5h"},
	}}
	var accounts []string
	for _, w := range acmeRead.Windows(all).Windows {
		accounts = append(accounts, w.Account)
	}
	if want := []string{"work", "lapsed", "gone"}; !slices.Equal(accounts, want) {
		t.Errorf("Acme's windows are of %q, want %q", accounts, want)
	}
	if every := (views.PoolRead{}).Windows(all); len(every.Windows) != 4 {
		t.Errorf("every pool's windows are %d, want all 4", len(every.Windows))
	}
}

// givenDays is a reader of the days given, whatever is asked.
type givenDays []ledger.Summary

func (d givenDays) Days(time.Time) []ledger.Summary { return d }

// A pool's read of the router's files reads its events alone, as Shows has
// them, whichever read reads them: the first, one back to an earlier day, or
// one again as the router restarts; every pool's read reads every one, and
// one of no files none.
func TestAPoolsReadOfTheRoutersFilesReadsItsEventsAlone(t *testing.T) {
	day := time.Date(2026, 11, 3, 0, 0, 0, 0, time.Local)
	at := func(hour int) time.Time { return day.Add(time.Duration(hour) * time.Hour) }
	filed := filedLines{
		{ID: 1, At: at(1), Kind: status.EventCap, Account: "side"},
		{ID: 2, At: at(2), Kind: status.EventStarted, Account: "side", Pool: "q7mt"},
		{ID: 3, At: at(13), Kind: status.EventLimit, Account: "side"},
		{ID: 4, At: at(14), Kind: status.EventLimit, Account: "work"},
		{ID: 5, At: at(15), Kind: status.EventRestart},
	}
	now := at(16)
	acme := acmeRead.Filed(filed)
	read := views.ReadEventsFrom(acme, at(12), now)
	none := views.ReadEventsFrom(nil, at(0), now)
	tests := []struct {
		name string
		read views.EventsRead
		want []int
	}{
		{name: "the first read", read: read, want: []int{4, 5}},
		{name: "a read back to an earlier day", read: read.Took(read.BackTo(acme, at(0))), want: []int{2, 4, 5}},
		{name: "a read again", read: none.Took(none.Refiled(acme, at(0), now)), want: []int{2, 4, 5}},
		{name: "every pool's read", read: views.ReadEventsFrom(views.PoolRead{}.Filed(filed), at(0), now), want: []int{1, 2, 3, 4, 5}},
	}
	for _, tt := range tests {
		var ids []int
		for _, e := range tt.read.Events() {
			ids = append(ids, e.Event.ID)
		}
		if !slices.Equal(ids, tt.want) {
			t.Errorf("%s holds the events %v, want %v", tt.name, ids, tt.want)
		}
	}
	if acmeRead.Filed(nil) != nil {
		t.Error("a pool's read of no files reads some, want none")
	}
}

func (d givenDays) DaysOn([]string) []ledger.Summary { return d }

// pooledLines are lines of Acme, q7mt, of the first pool, b2n8, of none, as
// from before pools, the first pool's, and of Acme named in capitals.
var pooledLines = []ledger.Held{
	{Request: "acme", Pool: "q7mt"},
	{Request: "first", Pool: "b2n8"},
	{Request: "none"},
	{Request: "acme in capitals", Pool: "Q7MT"},
}

// requestsOf are the ids of the requests of lines, in order.
func requestsOf(lines []ledger.Held) []string {
	var ids []string
	for _, h := range lines {
		ids = append(ids, h.Request)
	}
	return ids
}

// A pool read's lines, as Held gives them of a slice and Lines of a
// sequence, are those that recorded its pool, ids compared ignoring case,
// those naming none the first pool's, as ledger.RecordedIn has it: of every
// pool's read every one, and of none's none.
func TestAPoolReadsLinesAreThoseThatRecordedItsPool(t *testing.T) {
	tests := []struct {
		name string
		read views.PoolRead
		want []string
	}{
		{name: "Acme's", read: acmeRead, want: []string{"acme", "acme in capitals"}},
		{name: "the first pool's", read: views.PoolRead{ID: "b2n8", First: "b2n8"}, want: []string{"first", "none"}},
		{name: "every pool's", read: views.PoolRead{}, want: []string{"acme", "first", "none", "acme in capitals"}},
		{name: "no pool's", read: views.NoPool()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := requestsOf(tt.read.Held(pooledLines)); !slices.Equal(got, tt.want) {
				t.Errorf("Held gives the lines %q, want %q", got, tt.want)
			}
			if got := requestsOf(slices.Collect(tt.read.Lines(slices.Values(pooledLines)))); !slices.Equal(got, tt.want) {
				t.Errorf("Lines gives the lines %q, want %q", got, tt.want)
			}
		})
	}
}

// A pool's days by their dates, as Dates reads them, are its days as Days
// reads them: one rule of a pool's day, whichever way the days are asked
// for.
func TestAPoolsDaysByTheirDatesAreItsDays(t *testing.T) {
	day := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	read := givenDays{{Day: "2026-11-03", Accounts: []ledger.AccountDay{
		{Account: "work"}, {Account: "side", Sessions: 3, Pools: []ledger.PoolDay{{Pool: "b2n8", Sessions: 1}, {Pool: "q7mt", Sessions: 2}}}, {Account: "spare"},
	}}}
	for _, r := range []views.PoolRead{acmeRead, {}, views.NoPool()} {
		if got, want := r.Dates(read).DaysOn([]string{"2026-11-03"}), r.Days(read).Days(day); !reflect.DeepEqual(got, want) {
			t.Errorf("the days of %+v by their dates are\n%+v\nwant, as Days reads them\n%+v", r, got, want)
		}
	}
}

// sessionsHeld is a ledger holding the sessions given, whatever is asked.
type sessionsHeld struct {
	views.Ledger
	sessions map[string]ledger.SessionFrom
}

func (l sessionsHeld) Sessions(ledger.RoutedIn, []string, func(string) ledger.Rest, func(string, ledger.Line)) map[string]ledger.SessionFrom {
	return l.sessions
}

func (l sessionsHeld) SessionsOf(ledger.RoutedIn, []string, func(string) ledger.Rest, func(string, ledger.Line)) map[string]ledger.SessionFrom {
	return l.sessions
}

// poolsAsked is a ledger holding no session, which notes the pool each read
// of its sessions asked for, as each was handed it.
type poolsAsked struct {
	views.Ledger
	asked *[]ledger.RoutedIn
}

func (l poolsAsked) Sessions(in ledger.RoutedIn, _ []string, _ func(string) ledger.Rest, _ func(string, ledger.Line)) map[string]ledger.SessionFrom {
	*l.asked = append(*l.asked, in)
	return nil
}

func (l poolsAsked) SessionsOf(in ledger.RoutedIn, _ []string, _ func(string) ledger.Rest, _ func(string, ledger.Line)) map[string]ledger.SessionFrom {
	*l.asked = append(*l.asked, in)
	return nil
}

// A pool's read asks the ledger it reads through for the pool's sessions
// alone, by each read: Acme's, of the requests routed in Acme's, none in
// Default's, the first's, nor naming none. Read through another pool's read
// again, it asks for those routed in both, so a read through a second pool
// reads no wider than the first: Default's within Acme's, none at all. Every
// pool's read asks for every pool's.
func TestAPoolsReadAsksTheLedgerForThePoolsSessionsAlone(t *testing.T) {
	var asked []ledger.RoutedIn
	held := poolsAsked{asked: &asked}
	defaultRead := views.PoolRead{ID: "b2n8", First: "b2n8"}
	acmeRead.Ledger(held).Sessions(nil, nil, nil, nil)
	acmeRead.Ledger(held).SessionsOf(nil, []string{"one"}, nil, nil)
	acmeRead.Ledger(defaultRead.Ledger(held)).Sessions(nil, nil, nil, nil)
	views.PoolRead{}.Ledger(held).SessionsOf(nil, []string{"one"}, nil, nil)

	recorded := []string{"q7mt", "b2n8", ""}
	want := [][]bool{{true, false, false}, {true, false, false}, {false, false, false}}
	if len(asked) != 4 {
		t.Fatalf("the ledger was asked %d reads, want 4", len(asked))
	}
	for i, w := range want {
		got := make([]bool, len(recorded))
		for j, pool := range recorded {
			got[j] = asked[i] != nil && asked[i](pool)
		}
		if !slices.Equal(got, w) {
			t.Errorf("read %d asked for the requests of %q as %v, want %v", i, recorded, got, w)
		}
	}
	if asked[3] != nil {
		t.Error("every pool's read asked the ledger for one pool's, want every pool's")
	}
}

// No pool's read, as a dashboard showing no pool reads, reads no pool's
// work: no line, no day's account, no window, no event, the router's own
// among them, and no session, one resting on a digest's state among them.
func TestNoPoolsReadReadsNoPoolsWork(t *testing.T) {
	none := views.NoPool()
	if got := none.Held(pooledLines); len(got) != 0 {
		t.Errorf("no pool's read keeps the lines %q, want none", requestsOf(got))
	}
	day := time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)
	read := givenDays{{Day: "2026-11-03", Accounts: []ledger.AccountDay{{Account: "work", Sessions: 1}, {Account: "spare"}}}}
	if days := none.Days(read).Days(day); len(days) != 1 || len(days[0].Accounts) != 0 {
		t.Errorf("no pool's read of the days is %+v, want the day, holding no account", days)
	}
	if w := none.Windows(views.Windows{Windows: []views.WindowReadings{{Account: "work", Window: "5h"}}}); len(w.Windows) != 0 {
		t.Errorf("no pool's read keeps the windows %+v, want none", w.Windows)
	}
	if none.Shows(status.Event{Kind: status.EventRestart}) || none.Shows(status.Event{Kind: status.EventStarted}) {
		t.Error("no pool's read shows the router's own event, or a start naming no pool, want neither")
	}
	at := time.Date(2026, 11, 3, 9, 0, 0, 0, time.Local)
	filed := filedLines{{ID: 1, At: at, Kind: status.EventRestart}, {ID: 2, At: at, Kind: status.EventLimit, Account: "work"}}
	if read := views.ReadEventsFrom(none.Filed(filed), at.Add(-time.Hour), at.Add(time.Hour)); len(read.Events()) != 0 {
		t.Errorf("no pool's read of the router's files reads %d events, want none", len(read.Events()))
	}
	held := sessionsHeld{sessions: map[string]ledger.SessionFrom{
		"resting": {Base: &ledger.SessionBase{}}, "lined": {Lines: []ledger.Line{{Request: "r1"}}},
	}}
	if got := none.Ledger(held).Sessions(nil, nil, nil, nil); len(got) != 0 {
		t.Errorf("no pool's read of the sessions gives %d, want none", len(got))
	}
}

// startedDays is a ledger that began at first, holding the days given by
// their dates, as DaysOn asks for them, or failing to say where it began.
type startedDays struct {
	views.Ledger
	first  time.Time
	failed error
	days   map[string]ledger.Summary
}

func (l startedDays) FirstDay(time.Time) (time.Time, error) {
	return l.first, l.failed
}

func (l startedDays) DaysOn(dates []string) []ledger.Summary {
	var held []ledger.Summary
	for _, date := range dates {
		if s, ok := l.days[date]; ok {
			held = append(held, s)
		}
	}
	return held
}

// A pool's work begins on the first day the ledger holds whose summary in
// the pool holds work, however far into it: Acme's, on the 40th of 60 days,
// past a month's read of them, on that day; Default's, working on the first,
// where the ledger began; a pool that worked on no day before today, and no
// pool's read, today. Every pool's read begins where the ledger does, as the
// ledger tells it, and a ledger that fails to say fails the pool's read.
func TestAPoolsWorkBeginsOnTheFirstDayItsSummaryHoldsWork(t *testing.T) {
	now := time.Date(2026, 11, 3, 15, 0, 0, 0, time.Local)
	dayOf := func(back int) time.Time { return dayfile.DayStart(now, -back) }
	l := &startedDays{first: dayOf(60), days: make(map[string]ledger.Summary)}
	for back := 60; back > 0; back-- {
		date := dayfile.DateOf(dayOf(back))
		day := ledger.Summary{Day: date, Accounts: []ledger.AccountDay{{Account: "side", Pool: "b2n8", Sessions: 1}, {Account: "work", Pool: "q7mt"}}}
		if back <= 20 {
			day.Accounts[1].Sessions = 1
		}
		l.days[date] = day
	}
	idle := views.PoolRead{ID: "x9z9", First: "b2n8", PoolOf: acmeRead.PoolOf}
	for _, tt := range []struct {
		name string
		read views.PoolRead
		want time.Time
	}{
		{name: "Acme's", read: acmeRead, want: dayOf(20)},
		{name: "Default's", read: views.PoolRead{ID: "b2n8", First: "b2n8", PoolOf: acmeRead.PoolOf}, want: dayOf(60)},
		{name: "a pool yet to work", read: idle, want: dayOf(0)},
		{name: "no pool's", read: views.NoPool(), want: dayOf(0)},
	} {
		if got, err := tt.read.Start(l).FirstDay(now); err != nil || !got.Equal(tt.want) {
			t.Errorf("%s work begins %v, %v, want %v", tt.name, got, err, tt.want)
		}
	}
	if start := (views.PoolRead{}).Start(l); start != views.LedgerStart(l) {
		t.Errorf("every pool's read begins as %v tells it, want the ledger itself", start)
	}
	failing := &startedDays{failed: fs.ErrPermission}
	if _, err := acmeRead.Start(failing).FirstDay(now); !errors.Is(err, fs.ErrPermission) {
		t.Errorf("a pool's read of a ledger that fails to say where it began fails with %v, want its failure", err)
	}
	if start := acmeRead.Start(nil); start != nil {
		t.Errorf("a pool's read of no ledger begins as %v tells it, want nothing to tell", start)
	}
}
