package views

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// shown is what a session's page built shows: the page; its story, but the
// times its requests went upstream, which are given apart, as a list, as
// SentTimes' All gives them, whatever of them it holds sealed; and until
// when it stands.
type shown struct {
	page  SessionPage
	story Story
	sent  []time.Time
	until time.Time
}

// shownBy is what the page b built shows.
func shownBy(b BuiltPage) shown {
	story := b.Story
	story.Sent = SentTimes{}
	return shown{page: b.Page, story: story, sent: slices.Collect(b.Story.Sent.All()), until: b.Until}
}

// pointed reports whether the page p shows the points of some account's
// window, as one whose read took its answers does.
func pointed(p SessionPage) bool {
	return slices.ContainsFunc(p.Accounts, func(a SessionAccount) bool { return len(a.Points) > 0 })
}

// opened is what the page of the session with the given id shows, opened
// as the watch opens it at the nine days' moment, read through l, the
// stream s telling, no router listing the sessions.
func (n nineDays) opened(l LedgerSessions, id string, s Stream) shown {
	built, _ := ReadPage(l, id, s, ledger.Pricing, 0, n.now).Build(PageLive{Unlisted: true, Stream: s, Now: n.now})
	return shownBy(built)
}

// printed is the page of the session given names as the sessions verb
// prints it at the nine days' moment, read through l, as NamedAmong names
// it, the router listing the sessions running, or, where it gives none,
// none listed; the stream s telling.
func (n nineDays) printed(l Ledger, given string, running []status.Session, s Stream) SessionPage {
	read := ReadSessions(l, running, n.now, given)
	id := NamedAmong(given, read.Named(running)).ID
	page, _ := SessionPageOf(id, PageSources{Routed: Routed(running, id), Unlisted: running == nil, Read: read, Stream: s, Prices: ledger.Pricing, Now: n.now})
	return page
}

// stored is what the page of the session with the given id shows, as the
// store holds it, first fed at the nine days' moment through l wanting the
// page, and where listing says, the List, pinned as the stream s tells, the
// page taken from it as the watch takes it.
func (n nineDays) stored(l LedgerSessions, id string, listing bool, s Stream) shown {
	want := SessionsWanted{Pins: pinsOf(s)}
	if listing {
		want = ListWanted(n.running, s)
	}
	want.Page = id
	store, _ := HoldSessions(ledger.Pricing).Fed(l, Taken{}, want, n.now)
	held, _ := HoldPage(id, SessionsRead{}, ledger.Pricing, 0, n.now).Held(store)
	built, _ := held.Build(PageLive{Unlisted: true, Stream: s, Now: n.now})
	return shownBy(built)
}

// chains are the dates of the parts of the chain each of the sessions with
// the given ids rests on, resting on its page, as the nine days' digests
// give them, by its id.
func (n nineDays) chains(ids ...string) map[string][]string {
	chains := make(map[string][]string, len(ids))
	for id, from := range n.follower(DigestVersion).SessionsOf(ids, onPage, nil) {
		chains[id] = partDates(from)
	}
	return chains
}

// A page at half past one, once the day before's digest stands, over nine
// days its session ran on every one of, and made a request on today, reads
// today's file alone, its session given resting on its page at the day
// before's end, with the chain of its parts to it, and no other session
// given any: where the page read raw reads every day's. It shows the page
// read raw, its points among it: opened as the watch opens it, and as the
// store holds it, for the page alone and for the List too. A day is told
// read by the lines its read hands on.
func TestAPageOverDigestsReadsTodaysFileAloneOnItsChainOfParts(t *testing.T) {
	n := newNineDays(t, []string{"a", "b", "c"})
	yesterday := yesterdayOf(n.today)
	chains := n.chains("a")
	if chain := chains["a"]; len(chain) == 0 || chain[len(chain)-1] != yesterday {
		t.Fatalf("a rests on its page with the chain of the parts of %v; want a chain to the day before's, %s", chain, yesterday)
	}
	tests := []struct {
		name string
		read func(l Ledger) shown
	}{
		{name: "opened as the watch opens it", read: func(l Ledger) shown { return n.opened(l, "a", Stream{}) }},
		{name: "held by the store for the page alone", read: func(l Ledger) shown { return n.stored(l, "a", false, Stream{}) }},
		{name: "held by the store for the List too", read: func(l Ledger) shown { return n.stored(l, "a", true, Stream{}) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			based, raw := takingDates(n.follower(DigestVersion)), takingDates(n.follower(DigestVersion+1))

			got, want := tt.read(based), tt.read(raw)

			if dates := based.datesRead(); !slices.Equal(dates, []string{n.today.Date}) {
				t.Errorf("over digests, the page read the lines of %v, want those of %s alone", dates, n.today.Date)
			}
			if dates, every := raw.datesRead(), dayfile.Span(n.first.Start, n.today.Start); !slices.Equal(dates, every) {
				t.Errorf("read raw, the page read the lines of %v, want those of every day, %v", dates, every)
			}
			if !reflect.DeepEqual(based.parts, chains) {
				t.Errorf("over digests, the read gave the chains of parts %v, want a's alone, %v", based.parts, chains)
			}
			if !reflect.DeepEqual(got, want) || !pointed(want.page) {
				t.Errorf("over digests, the page shows\n%+v\nwant, as read raw, its points among it\n%+v", got, want)
			}
		})
	}
}

// The sessions verb shows a session over digests as it shows it read raw,
// named by its whole id or as much of it as is unique: the session named
// resting on its page, every other the read gives on its state, so the read
// gives the named session's chain of parts alone. One left out of the day
// before's digest rests on the state of the day before that, so the read
// reads the day before's file too, handing on that day's answers, which the
// others' pages, resting on that day's state, have folded already: it reads
// that day's file and today's, no session read whole again. With the router
// listing the sessions, and with none.
func TestTheSessionsVerbOverDigestsShowsASessionAsReadRaw(t *testing.T) {
	ids := []string{"alpha", "bravo", "charlie"}
	n := newNineDays(t, ids, "bravo")
	yesterday := yesterdayOf(n.today)
	chains := n.chains(ids...)
	for _, id := range ids {
		want := yesterday
		if id == "bravo" {
			want = yesterdayOf(dayOf(yesterday))
		}
		if chain := chains[id]; len(chain) == 0 || chain[len(chain)-1] != want {
			t.Fatalf("%s rests on its page with the chain of the parts of %v, want a chain to %s's", id, chain, want)
		}
	}
	tests := []struct {
		name    string
		running []status.Session
	}{
		{name: "the router listing the sessions", running: n.running},
		{name: "none listing them"},
	}
	for _, tt := range tests {
		for _, id := range ids {
			for _, given := range []string{id, id[:2]} {
				t.Run(fmt.Sprintf("%s, %s named as %s", tt.name, id, given), func(t *testing.T) {
					l := takingDates(n.follower(DigestVersion))

					got := n.printed(l, given, tt.running, Stream{})

					if dates, want := l.datesRead(), []string{yesterday, n.today.Date}; !slices.Equal(dates, want) {
						t.Errorf("the verb read the lines of %v, want those of %v", dates, want)
					}
					if want := map[string][]string{id: chains[id]}; !reflect.DeepEqual(l.parts, want) {
						t.Errorf("the verb's read gave the chains of parts %v, want %s's alone, %v", l.parts, id, want)
					}
					if want := n.printed(n.follower(DigestVersion+1), given, tt.running, Stream{}); want.Session != id || !reflect.DeepEqual(got, want) || !pointed(want) {
						t.Errorf("over digests, the verb shows\n%+v\nwant, as read raw, %s's, its points among it\n%+v", got, id, want)
					}
				})
			}
		}
	}
}

// A page whose restore is refused, as the stream tells of a request of its
// session sent before the end of the day its page rests on, which its state
// holds sealed, is read again whole, every day's answers taken afresh, not
// only those of the days after its base that the first read took: the page
// the store holds, the page as the watch opens it, the sessions verb's, and
// the page HoldPage holds, which holds none on a base, each shows the page
// read raw, its points of every day's answers. The request ended, its line
// yet to come, sent a minute before the last of the day before's, which a
// page resting on that day's state would take in after that one, out of its
// place.
func TestAPageReadWholeAfterARefusedRestoreTakesEveryDaysAnswers(t *testing.T) {
	n := newNineDays(t, []string{"a", "b", "c"})
	var last ledger.Line
	for _, l := range n.chain.lines {
		if l.Session == "a" && l.At.Before(n.today.Start) && l.At.After(last.At) {
			last = l
		}
	}
	sent := last.At.Add(-time.Minute)
	if n.now.Sub(sent) > pinnedFor {
		t.Fatalf("a's last request of the day before arrived at %v, too long before %v for one sent before it to pin it", last.At, n.now)
	}
	pinned := Told{
		Request: "pinned", Session: "a", Model: last.Model, Account: last.Account, Class: ledger.ClassMain, Prompt: "pinned",
		Sent: sent, Ended: sent.Add(time.Second), Heard: sent.Add(time.Second),
	}
	s := Stream{Requests: map[string]Told{pinned.Request: pinned}}
	tests := []struct {
		name string
		read func(l Ledger) shown
	}{
		{name: "the store's", read: func(l Ledger) shown { return n.stored(l, "a", false, s) }},
		{name: "as the watch opens it", read: func(l Ledger) shown { return n.opened(l, "a", s) }},
		{name: "the sessions verb's", read: func(l Ledger) shown { return shown{page: n.printed(l, "a", nil, s)} }},
		{name: "as HoldPage holds it", read: func(l Ledger) shown {
			page, _ := HoldPage("a", ReadSessions(l, nil, n.now, "a"), ledger.Pricing, 0, n.now).Page(PageLive{Unlisted: true, Stream: s, Now: n.now})
			return shown{page: page}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := takingDates(n.follower(DigestVersion))

			got := tt.read(l)

			if dates, every := l.datesRead(), dayfile.Span(n.first.Start, n.today.Start); !slices.Equal(dates, every) {
				t.Errorf("the page read the lines of %v, want those of every day, %v, as it's read again whole", dates, every)
			}
			if want := tt.read(n.follower(DigestVersion + 1)); !reflect.DeepEqual(got, want) || !pointed(want.page) {
				t.Errorf("over digests, the page shows\n%+v\nwant, as read raw, its points among it\n%+v", got, want)
			}
		})
	}
}

// A page's points resting on a state take in the answers of the days after
// its day alone, which they took in as they were sealed: given the answers
// of its own day again, or of a day before it, with one more of the
// session's own, which they'd refuse, as more than they folded that day,
// they take nothing in, and are as they were.
func TestAPagesPointsTakeInNoAnswerOfTheDaysThroughTheirBase(t *testing.T) {
	lines := daily(foldSonnet, foldSonnet, foldSonnet)
	base := dayOf(dateOn(1))
	end := endOfDay(base)
	folded := arrivedBefore(lines, end.time())
	p := readOf(folded, answersOf(linesByDate(folded), "s"), ledger.Pricing, base, end, true).page.points
	next := Today(end.time())
	for i, date := range []string{dateOn(0), dateOn(1)} {
		t.Run(date, func(t *testing.T) {
			again := lines[i]
			again.Request, again.At = "again", again.At.Add(time.Minute)
			given := []answered{answeredOf(&lines[i], "s"), answeredOf(&again, "s")}
			if _, ok := p.took(date, given, true, next); ok {
				t.Fatalf("the points take in %s's answers and one more of the session's own, want them refused", date)
			}

			got, ok := p.tookAfter(base.Date, map[string][]answered{date: given}, next)

			if !ok || got != p {
				t.Errorf("resting on the state of %s, given %s's answers, the points took %t, as %p; want them as they were, %p", base.Date, date, ok, got, p)
			}
		})
	}
}

// heldOver is a session's page held across feeds as the watch holds it: the
// page, and the store it joined, fed through l.
type heldOver struct {
	l     LedgerSessions
	page  HeldPage
	store HeldSessions
}

// heldOn is the page of the session with the given id opened at now, read
// through l, as the watch opens it, joining a store holding it alone.
func heldOn(l LedgerSessions, id string, now time.Time) *heldOver {
	page := ReadPage(l, id, Stream{}, ledger.Pricing, 0, now)
	store := HoldSessions(ledger.Pricing).With(page.Found())
	return &heldOver{l: l, page: page.Joined(store), store: store}
}

// fed is what h's page shows once its store is fed t, a read of the
// ledger's tail at now, wanting the page, and the page holds its session as
// the store fed it, as the watch feeds them.
func (h *heldOver) fed(t Taken, now time.Time) shown {
	h.store, _ = h.store.Fed(h.l, t, SessionsWanted{}.Paging(h.page, Stream{}), now)
	h.page, _ = h.page.On(Today(now).Date)
	h.page, _ = h.page.Held(h.store)
	built, _ := h.page.Build(PageLive{Unlisted: true, Now: now})
	return shownBy(built)
}

// ledgerTail is a read of the ledger's tail as the dashboard reads it,
// through l: today's lines since mark, as Today gives them; and the lines of
// each day from day, the one it last read, until today's, read again whole,
// as the tail missed the end of each.
type ledgerTail struct {
	l    Ledger
	mark ledger.Mark
	day  string
}

// at is the tail read at now.
func (t *ledgerTail) at(now time.Time) Taken {
	gained, mark, _ := t.l.Today(t.mark)
	today := Today(now).Date
	days := make(map[string][]ledger.Held)
	for date := t.day; date < today; date = nextDate(date) {
		days[date] = t.l.Day(date)
	}
	t.mark, t.day = mark, today
	return Took(gained, days)
}

// A page opened over digests at half past one and held across the day's
// feeds, past midnight and to the next day's half past one, once that day's
// digest stands, as the watch holds it, its store fed a read of the
// ledger's tail at each moment, a request of its session filed before each,
// shows at each the page held raw: its page, story and points.
func TestAPageHeldAcrossFeedsOverDigestsIsThePageHeldRaw(t *testing.T) {
	n := newNineDays(t, []string{"a", "b", "c"})
	at := n.now
	clock := func() time.Time { return at }
	raw := ledger.NewFollower(n.chain.state, clock, ledger.CapsOf(nil, nil), DigestVersion+1, quiet)
	based := heldOn(ledger.NewFollower(n.chain.state, clock, ledger.CapsOf(nil, nil), DigestVersion, quiet), "a", at)
	held := heldOn(raw, "a", at)
	tail := &ledgerTail{l: raw, day: n.today.Date}
	asked := n.chain.lines[slices.IndexFunc(n.chain.lines, func(l ledger.Line) bool { return l.Session == "a" })]
	r := rand.New(rand.NewPCG(9, 10))
	tomorrow := dayfile.DayStart(n.today.Start, 1)
	moments := []time.Time{
		n.now.Add(30 * time.Minute), n.today.Start.Add(12 * time.Hour), tomorrow.Add(-time.Minute), tomorrow.Add(time.Minute),
		tomorrow.Add(59 * time.Minute), tomorrow.Add(90 * time.Minute),
	}
	for i, moment := range moments {
		l := asked
		l.Request, l.At, l.TotalMS = fmt.Sprintf("held-%d", i), moment.Add(-2*time.Minute), 3000
		limitedOf(&l, r, time.Time{})
		n.chain.file(dayfile.DateOf(l.At), l)
		at, n.chain.at = moment, moment
		n.chain.ledger.SummariseEnded(at)
		n.chain.ledger.DigestEnded(t.Context(), at)
		taken := tail.at(at)

		got, want := based.fed(taken, at), held.fed(taken, at)

		if !reflect.DeepEqual(got, want) || !pointed(want.page) {
			t.Errorf("at %v, over digests, the page held shows\n%+v\nwant, as held raw, its points among it\n%+v", at, got, want)
		}
	}
	if _, built := n.chain.digests.handed[n.today.Date]; !built {
		t.Errorf("%s isn't digested by %v", n.today.Date, at)
	}
}
