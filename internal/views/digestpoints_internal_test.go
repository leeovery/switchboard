package views

import (
	"cmp"
	"encoding/json"
	"fmt"
	"hash/maphash"
	"maps"
	"math"
	"math/rand/v2"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"
	"unique"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
)

// pointsSeeds is how many seeds of pointedRun the points' round trip runs.
const pointsSeeds = 120

// pointedRun is a page's run as pagedRun makes it, and of one seed in three,
// another session's answer without an id, on work, in flight across the
// midnight after the session's first request, so its points hold it as
// they're sealed at that day's end.
func pointedRun(seed uint64) []ledger.Line {
	lines, _ := pagedRun(seed)
	if seed%3 != 0 {
		return lines
	}
	order := arrivalOrder(lines)
	first := lines[order[slices.IndexFunc(order, func(i int) bool { return lines[i].Session == "s" })]]
	midnight := dayfile.DayStart(first.At.Local(), 1)
	l := ledger.Line{
		At: midnight.Add(-3 * time.Minute).UTC(), Kind: ledger.KindMessage, Session: "o1", Model: foldOpus, Account: "work", Reason: "sticky",
		Status: 200, Attempts: 1, TotalMS: (6 * time.Minute).Milliseconds(), Class: ledger.ClassMain, Prompt: "q", Usage: json.RawMessage(foldUsages[2]),
	}
	limitedOf(&l, rand.New(rand.NewPCG(seed, 14)), time.Time{})
	lines = append(lines, l)
	slices.SortStableFunc(lines, func(a, b ledger.Line) int { return ended(&a).Compare(ended(&b)) })
	return lines
}

// The hard moments of the points' round trip, of points stored at the end
// of a day D and restored on a later day.
const (
	pointsLeftOut     = "answers of a day not the session's, left out of what's stored"
	pointsHeldAcross  = "a run's answers held across D's end, stored"
	pointsIdlessHeld  = "an answer without an id, stored"
	pointsIdlessAfter = "an answer without an id, taken in after D"
	pointsResets      = "a climb's resets, stored"
	pointsSomeModels  = "a window only some models share, stored"
	pointsDoneBefore  = "a run done by D's end, stored"
	pointsDoneAfter   = "a run done after D, a later one begun"
	pointsJoined      = "a day joining the run after D"
	pointsSplit       = "a model first reading a window only some models share after D, refused alike"
	pointsGap         = "a day of the session's own before the day before the read, after D, refused alike"
	pointsPlaced      = "D given again whole, placed"
	pointsMidnight    = "fed on across midnight"
)

// A page's points sealed at the end of a day, stored, read back and
// restored onto a later day, take that day's answers and those of each day
// between, oldest first, each whole, as pointing's took has them, then are
// sealed behind a moment on it, as sealedAt has them: they're the points of
// every answer read whole then, their points and their shares exactly, or
// refused where the points as sealed, never stored, refuse too. Fed on to a
// later moment, as the store feeds the days it reads again, D among them,
// they're the points read whole then too. What's stored holds no answer of
// a day that isn't the session's, and restored, stores again as it was. Of
// generated runs over several days, meeting the hard moments the constants
// above name: resets, a window only some models share, a run broken and
// resumed, and answers without ids among them.
func TestAPagesPointsRestoredFromTheirStoredFormAreThePointsFoldedWhole(t *testing.T) {
	met := make(map[string]bool)
	for seed := range uint64(pointsSeeds) {
		lines := pointedRun(seed)
		filed := answersFiled(lines)
		r := rand.New(rand.NewPCG(seed, 15))
		for _, cut := range daysOfLines(lines) {
			tr := &pointsTrip{t: t, name: fmt.Sprintf("run %d, stored at the end of %s", seed, cut.Date), filed: filed, cut: cut, met: met}
			for after := 1; after <= 3; after++ {
				on := Today(dayfile.DayStart(cut.Start, after))
				now := on.Start.Add(time.Duration(r.Int64N(int64(dayfile.DayStart(on.Start, 1).Sub(on.Start)))))
				if settled := tr.settled(); now.Before(settled) {
					now = settled
				}
				if on.Holds(now) {
					tr.check(on, now, now.Add(time.Duration(r.Int64N(int64(20*time.Hour)))))
				}
			}
		}
	}
	for _, moment := range []string{
		pointsLeftOut, pointsHeldAcross, pointsIdlessHeld, pointsIdlessAfter, pointsResets, pointsSomeModels, pointsDoneBefore, pointsDoneAfter,
		pointsJoined, pointsSplit, pointsGap, pointsPlaced, pointsMidnight,
	} {
		if !met[moment] {
			t.Errorf("no round trip meets %s", moment)
		}
	}
}

// daysOfLines are the days from the one the first of lines arrived on
// through the one the last ended on.
func daysOfLines(lines []ledger.Line) []LedgerDay {
	first, last := lines[arrivalOrder(lines)[0]].At, ended(&lines[len(lines)-1])
	var days []LedgerDay
	for at := dayfile.DayStart(first.Local(), 0); !at.After(last); at = dayfile.DayStart(at, 1) {
		days = append(days, Today(at))
	}
	return days
}

// filedAnswer is an answer as a run's lines file it: the date of the day
// it's filed on, and the answer, as answeredOf has it of the page of s.
type filedAnswer struct {
	date string
	answered
}

// answersFiled are every account's answers among lines, as filedAnswer has
// each, in the order of lines.
func answersFiled(lines []ledger.Line) []filedAnswer {
	var filed []filedAnswer
	for i := range lines {
		if isAnswer(&lines[i]) {
			filed = append(filed, filedAnswer{date: dayfile.DateOf(lines[i].At), answered: answeredOf(&lines[i], "s")})
		}
	}
	return filed
}

// pointsTrip is a page's points over a generated run stored at the end of a
// day and restored on later days, as the round trip takes them: named by
// its run's seed and its day; the run's answers, as filed; the day they're
// stored at the end of; and the hard moments the trips met.
type pointsTrip struct {
	t     *testing.T
	name  string
	filed []filedAnswer
	cut   LedgerDay
	met   map[string]bool
}

// settled is when the last of the answers filed on the trip's day or before
// ended, as they're all filed by then: none after its day's start, where
// none is.
func (tr *pointsTrip) settled() time.Time {
	settled := instantOf(tr.cut.Start)
	for _, f := range tr.filed {
		if f.date <= tr.cut.Date {
			settled = later(settled, f.end)
		}
	}
	return settled.time()
}

// answersAt are the run's answers filed by now, as a ledger filing each as
// its request ends gives them, by the date of the day each is filed on.
func (tr *pointsTrip) answersAt(now time.Time) map[string][]answered {
	at := instantOf(now)
	byDate := make(map[string][]answered)
	for _, f := range tr.filed {
		if f.at.compare(at) <= 0 && f.end.compare(at) <= 0 {
			byDate[f.date] = append(byDate[f.date], f.answered)
		}
	}
	return byDate
}

// check stores the points of the answers filed by now on the trip's day or
// before, sealed at its end, restores them on the day given and feeds them
// on to now, then to later, as fedOn has it, holding each to the points read
// whole then, and to the points as sealed, never stored, fed on alike.
func (tr *pointsTrip) check(on LedgerDay, now, later time.Time) {
	tr.t.Helper()
	answers := tr.answersAt(now)
	base := pointingOf(datesThrough(answers, tr.cut.Date), tr.cut).sealedAt(endOfDay(tr.cut), tr.cut)
	restored := tr.roundTrip(base)
	got, ok := tr.fedOn(restored, answers, on, now)
	sealed, sealedOK := tr.fedOn(base, answers, on, now)
	if !tr.refusedAlike(ok, sealedOK, restored, answers, on) {
		return
	}
	tr.sameAsWhole(sealed, answers, on, now, "as sealed, never stored")
	tr.sameAsWhole(got, answers, on, now, "restored")
	tr.noteFed(restored, got, answers)

	answers, on = tr.answersAt(later), Today(later)
	tr.met[pointsMidnight] = tr.met[pointsMidnight] || on.Date > Today(now).Date
	if got.isDay(tr.cut.Date) {
		again, placed := got.took(tr.cut.Date, answers[tr.cut.Date], true, on)
		if !placed || again != got {
			tr.t.Errorf("%s, fed on to %v, given %s again whole: placed %t, taking in %t; want it placed, nothing taken in", tr.name, later, tr.cut.Date, placed, again != got)
		}
		tr.met[pointsPlaced] = true
	}
	got, ok = tr.fedOn(got, answers, on, later)
	sealed, sealedOK = tr.fedOn(sealed, answers, on, later)
	if tr.refusedAlike(ok, sealedOK, restored, answers, on) {
		tr.sameAsWhole(sealed, answers, on, later, "as sealed, never stored, fed on")
		tr.sameAsWhole(got, answers, on, later, "restored and fed on")
	}
}

// datesThrough are those of byDate, answers or lines by date, of the day
// with the given date and before.
func datesThrough[T any](byDate map[string]T, date string) map[string]T {
	through := maps.Clone(byDate)
	maps.DeleteFunc(through, func(d string, _ T) bool { return d > date })
	return through
}

// roundTrip is p as a day's digest stores it, written, read back and
// restored, holding that what's stored holds none of the answers p holds
// of a day that isn't the session's, and that restored, it stores again as
// it was; noting the hard moments what's stored meets.
func (tr *pointsTrip) roundTrip(p *pointing) *pointing {
	tr.t.Helper()
	data, err := json.Marshal(p.stored())
	if err != nil {
		tr.t.Fatalf("%s: storing the points: %v", tr.name, err)
	}
	var st storedPointing
	if err := json.Unmarshal(data, &st); err != nil {
		tr.t.Fatalf("%s: reading back %s: %v", tr.name, data, err)
	}
	restored, err := st.restored()
	if err != nil {
		tr.t.Fatalf("%s: restoring %s: %v", tr.name, data, err)
	}
	if again, err := json.Marshal(restored.stored()); err != nil || string(again) != string(data) {
		tr.t.Fatalf("%s: stored as\n%s\nrestored, stores as\n%s, %v; want it as it was", tr.name, data, again, err)
	}
	for date := range restored.held {
		if !restored.isDay(date) {
			tr.t.Fatalf("%s: stored as %s, holding answers of %s, not a day of the session's; want none of them", tr.name, data, date)
		}
	}
	tr.noteStored(p, st)
	return restored
}

// fedOn is p, the points sealed at the end of the trip's day, fed the
// answers given of each day after it, oldest first, each whole, as took has
// it, on the day given, and sealed behind now, reporting false where took
// can't place them.
func (tr *pointsTrip) fedOn(p *pointing, answers map[string][]answered, on LedgerDay, now time.Time) (*pointing, bool) {
	for _, date := range slices.Sorted(maps.Keys(answers)) {
		if date <= tr.cut.Date {
			continue
		}
		var ok bool
		if p, ok = p.took(date, answers[date], true, on); !ok {
			return nil, false
		}
	}
	return p.sealedAt(instantOf(now), on), true
}

// refusedAlike reports whether the points restored and those as sealed both
// took the answers given in, holding that where one refused, the other did,
// and noting why.
func (tr *pointsTrip) refusedAlike(ok, sealedOK bool, restored *pointing, answers map[string][]answered, on LedgerDay) bool {
	tr.t.Helper()
	if ok != sealedOK {
		tr.t.Errorf("%s, fed on on %s: restored, it took the answers in %t; as sealed, never stored, %t; want them alike", tr.name, on.Date, ok, sealedOK)
		return false
	}
	if ok {
		return true
	}
	var after []answered
	for date, given := range answers {
		if date <= tr.cut.Date {
			continue
		}
		after = append(after, given...)
		if date < yesterdayOf(on) && slices.ContainsFunc(given, func(a answered) bool { return a.ours }) {
			tr.met[pointsGap] = true
		}
	}
	tr.met[pointsSplit] = tr.met[pointsSplit] || restored.run != nil && !restored.run.takes(after)
	return false
}

// sameAsWhole holds p, points fed on to now as fedOn has them, to the points
// of every answer given read whole on the day given, sealed behind now:
// their points and their shares, exactly.
func (tr *pointsTrip) sameAsWhole(p *pointing, answers map[string][]answered, on LedgerDay, now time.Time, how string) {
	tr.t.Helper()
	whole := pointingOf(answers, on).sealedAt(instantOf(now), on)
	if !reflect.DeepEqual(p.worked(), whole.worked()) || !reflect.DeepEqual(p.taken(), whole.taken()) {
		tr.t.Errorf("%s, %s, read on %s at %v, takes the shares %v, its points %v; want, as read whole, %v, %v",
			tr.name, how, on.Date, now, p.taken(), p.worked(), whole.taken(), whole.worked())
	}
}

// noteStored notes the hard moments st, p as stored, meets.
func (tr *pointsTrip) noteStored(p *pointing, st storedPointing) {
	for date := range p.held {
		tr.met[pointsLeftOut] = tr.met[pointsLeftOut] || !p.isDay(date)
	}
	tr.met[pointsHeldAcross] = tr.met[pointsHeldAcross] || len(st.Held) > 0
	tr.met[pointsDoneBefore] = tr.met[pointsDoneBefore] || len(st.Done) > 0
	for _, held := range st.Held {
		tr.met[pointsIdlessHeld] = tr.met[pointsIdlessHeld] || slices.ContainsFunc(held, func(a storedAnswer) bool { return a.Request == "" })
	}
	if st.Run == nil {
		return
	}
	for _, a := range st.Run.Accounts {
		for _, w := range a.Windows {
			tr.met[pointsResets] = tr.met[pointsResets] || len(w.Climb.Resets()) > 0
			tr.met[pointsSomeModels] = tr.met[pointsSomeModels] || !w.Shared
		}
	}
}

// noteFed notes the hard moments the points restored meet, fed the answers
// given on as fed.
func (tr *pointsTrip) noteFed(restored, fed *pointing, answers map[string][]answered) {
	for date, given := range answers {
		tr.met[pointsIdlessAfter] = tr.met[pointsIdlessAfter] || date > tr.cut.Date && slices.ContainsFunc(given, func(a answered) bool { return a.request == "" })
	}
	switch was, is := restored.run, fed.run; {
	case was == nil || is == nil:
	case is.first != was.first:
		tr.met[pointsDoneAfter] = true
	case is.last > was.last:
		tr.met[pointsJoined] = true
	}
}

// A session named on a day it made no request of its own on, as by a count
// of tokens alone, has its points stored at that day's end hold none of the
// day's answers, every other session's, though the points as sealed hold
// them; and the points restored, read on the day after, are those read
// whole, which hold them still, as the day before.
func TestAPagesPointsStoredOnADayItMadeNoRequestOnHoldNoneOfItsAnswers(t *testing.T) {
	at := func(day, hour, minute int) time.Time {
		return time.Date(2026, 10, day, hour, minute, 0, 0, time.Local).UTC()
	}
	answer := func(session string, arrived time.Time, used float64) ledger.Line {
		return ledger.Line{
			Request: fmt.Sprintf("%s-%d", session, arrived.Unix()), At: arrived, Kind: ledger.KindMessage, Session: session, Model: foldOpus, Account: "work",
			Reason: "sticky", Status: 200, Attempts: 1, TotalMS: 4000, Class: ledger.ClassMain, Prompt: "p1", Usage: json.RawMessage(foldUsages[2]),
			Limits: map[string]string{"7d-utilization": strconv.FormatFloat(used, 'f', 4, 64), "7d-reset": strconv.FormatInt(at(12, 9, 0).Unix(), 10)},
		}
	}
	counted := answer("s", at(7, 12, 0), 0)
	counted.Kind, counted.Limits = ledger.KindCount, nil
	lines := []ledger.Line{
		answer("s", at(6, 10, 0), 0.10), answer("o", at(6, 10, 30), 0.12), answer("s", at(6, 11, 0), 0.15),
		answer("o", at(7, 9, 0), 0.20), counted, answer("o", at(7, 15, 0), 0.30), answer("o", at(7, 23, 0), 0.35),
		answer("s", at(8, 9, 0), 0.40), answer("o", at(8, 9, 30), 0.44), answer("s", at(8, 10, 0), 0.50),
	}
	named, read := Today(at(7, 12, 0)), at(8, 11, 0)
	tr := &pointsTrip{t: t, name: "a session named on 7 October by a count alone", filed: answersFiled(lines), cut: named, met: make(map[string]bool)}
	answers := tr.answersAt(read)
	base := pointingOf(datesThrough(answers, named.Date), named).sealedAt(endOfDay(named), named)
	if held := len(base.held[named.Date].all()); held != 3 {
		t.Fatalf("the points sealed at the end of %s hold %d of its answers, want its 3, every other session's", named.Date, held)
	}
	if st := base.stored(); len(st.Held) > 0 {
		t.Errorf("the points stored at the end of %s hold answers of %v, want none", named.Date, slices.Collect(maps.Keys(st.Held)))
	}
	on := Today(read)
	got, ok := tr.fedOn(tr.roundTrip(base), answers, on, read)
	whole := pointingOf(answers, on).sealedAt(instantOf(read), on)
	if wholeHeld, gotHeld := len(whole.held[named.Date].all()), len(got.held[named.Date].all()); wholeHeld == 0 || gotHeld > 0 {
		t.Fatalf("read whole, the points hold %d answers of %s, restored %d; want them held read whole alone", wholeHeld, named.Date, gotHeld)
	}
	if !ok || !reflect.DeepEqual(got.worked(), whole.worked()) || !reflect.DeepEqual(got.taken(), whole.taken()) {
		t.Errorf("restored, read on %s, the points are %v, %t; want %v, as read whole", on.Date, got.worked(), ok, whole.worked())
	}
	if want := map[string]int{"7d": 9}; !reflect.DeepEqual(got.of("work"), want) {
		t.Errorf("s took %v points of work's windows, want %v", got.of("work"), want)
	}
}

// A page's points seal a day's requests each once, as a request is counted:
// one with an id once, however often its line is given; one without as
// often as given. So given the day again whole once they sealed it, they
// place it on what they sealed, taking nothing in: of the second day of
// givenTwiceRun's, its four requests of five lines. Folded whole, as a page
// read raw folds them; and on the first day's, as a day's digest folds
// them.
func TestAPagesPointsSealADaysRequestsEachOnceAndPlaceItGivenAgain(t *testing.T) {
	answers := answersOf(linesByDate(givenTwiceRun()), "s")
	first, second, third := dayOf(dateOn(0)), dayOf(dateOn(1)), dayOf(dateOn(2))
	sealed := pointingOf(datesThrough(answers, first.Date), first).sealedAt(endOfDay(first), first)
	foldedOn, ok := sealed.took(second.Date, answers[second.Date], true, second)
	if !ok {
		t.Fatalf("the points sealed at the end of %s can't take %s's answers in, want them taken", first.Date, second.Date)
	}
	tests := []struct {
		name   string
		points *pointing
	}{
		{name: "folded whole", points: pointingOf(answers, second).sealedAt(endOfDay(second), second)},
		{name: "folded on the first day's", points: foldedOn.sealedAt(endOfDay(second), second)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, placed := tt.points.took(second.Date, answers[second.Date], true, third)

			if folded := tt.points.folded[second.Date]; folded != 4 || !placed || got != tt.points {
				t.Errorf("sealing %d of %s's %d answers, the points given them again placed them %t, as they were %t; want 4 sealed, placed as they were",
					folded, second.Date, len(answers[second.Date]), placed, got == tt.points)
			}
		})
	}
}

// A page's points' stored form is their days; their shares done; their last
// run, its accounts' windows, each climb in its own stored form; the
// answers they hold of the session's days, each as its fields, its windows
// in claude.KeptWindow's own stored form; and how many of each day's they
// let go of: each by a short key, every zero left out. Never the answers of
// a day that isn't the session's, an answer's sum, nor their points.
func TestAPagesPointsStoredFormIsWhatTheyHoldOfTheSessionsDays(t *testing.T) {
	read := time.Date(2026, 10, 7, 23, 59, 0, 0, time.UTC)
	climb := ledger.NewClimb("5h", nil)
	climb.Take(quota.Window{Key: "5h", Label: "Session", Utilization: 0.25, ResetsAt: read.Add(time.Hour)}, read.Add(-time.Hour))
	opus := unique.Make(foldOpus)
	limits := map[string]string{"5h-utilization": "0.3", "5h-reset": strconv.FormatInt(read.Add(time.Hour).Unix(), 10)}
	answer := answered{
		request: "r1", at: instantOf(read.Add(-time.Minute)), end: instantOf(read.Add(2 * time.Minute)), read: instantOf(read),
		account: unique.Make("work"), model: opus, tokens: 1610, ours: true, windows: claude.KeptWindows(limits),
	}
	answer.sum = answer.summed()
	other := answer
	other.request, other.ours = "", false
	p := &pointing{
		days: []string{"2026-10-05", "2026-10-07"},
		done: map[string]map[string]float64{"side": {"7d": 0.125}},
		run: &runShares{first: "2026-10-07", last: "2026-10-07", through: instantOf(read.Add(time.Minute)), accounts: map[string]*accountShares{
			"work": {ours: map[unique.Handle[string]]bool{opus: true}, windows: map[string]*windowShare{
				"5h": {climb: climb, shared: true, models: map[unique.Handle[string]]bool{opus: true}, pending: tokensEnded{all: 3000, ours: 1000}, share: 0.0625},
			}},
		}},
		held:   map[string]answersHeld{"2026-10-07": heldOf([]answered{answer}), "2026-10-08": heldOf([]answered{other})},
		folded: map[string]int{"2026-10-05": 4, "2026-10-07": 2},
	}
	got, err := json.Marshal(p.stored())
	want := `{"d":["2026-10-05","2026-10-07"],"dn":{"side":{"7d":0.125}},"r":{"f":"2026-10-07","l":"2026-10-07","t":{"s":1791417600},` +
		`"a":{"work":{"w":{"5h":{"c":{"k":"5h","l":{"k":"5h","lb":"Session","u":0.25,"r":{"s":63927017940}},"t":{"s":63927010740},"r":true,` +
		`"tp":0.25,"b":true,"lt":{"s":63927017940}},"s":true,"m":["claude-opus-5-5"],"p":{"a":3000,"o":1000},"sh":0.0625}},"o":["claude-opus-5-5"]}}},` +
		`"h":{"2026-10-07":[{"r":"r1","a":{"s":1791417480},"e":{"s":1791417660},"rd":{"s":1791417540},"ac":"work","m":"claude-opus-5-5","tk":1610,"o":true,` +
		`"w":[{"k":"5h","u":0.3,"r":1791421140}]}]},"f":{"2026-10-05":4,"2026-10-07":2}}`
	if err != nil || string(got) != want {
		t.Errorf("stored as\n%s, %v, want\n%s", got, err, want)
	}
}

// A page's points read back are refused, for the session's page to be read
// whole, where their days aren't dates, oldest first, each once, as a day
// is known by them; or where a window of their run has no climb to take
// readings on. Points whose shares aren't finite numbers aren't stored, as
// JSON can't carry them.
func TestAPagesPointsThatCantBeFollowedOnArentStoredOrRestored(t *testing.T) {
	climb := ledger.NewClimb("5h", nil)
	run := func(c *ledger.Climb) *storedRun {
		return &storedRun{First: "2026-10-07", Last: "2026-10-07", Accounts: map[string]storedAccount{"work": {Windows: map[string]storedShare{"5h": {Climb: c}}}}}
	}
	restores := []struct {
		name    string
		stored  storedPointing
		refused bool
	}{
		{name: "days in order, a window with its climb", stored: storedPointing{Days: []string{"2026-10-06", "2026-10-07"}, Run: run(climb)}},
		{name: "none at all", stored: storedPointing{}},
		{name: "a day that isn't a date", stored: storedPointing{Days: []string{"7 October"}}, refused: true},
		{name: "days out of order", stored: storedPointing{Days: []string{"2026-10-07", "2026-10-06"}}, refused: true},
		{name: "a day twice", stored: storedPointing{Days: []string{"2026-10-07", "2026-10-07"}}, refused: true},
		{name: "a window without its climb", stored: storedPointing{Days: []string{"2026-10-07"}, Run: run(nil)}, refused: true},
	}
	for _, tt := range restores {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := tt.stored.restored(); (err != nil) != tt.refused {
				t.Errorf("restored with error %v, want it refused %t", err, tt.refused)
			}
		})
	}
	stores := []struct {
		name    string
		points  pointing
		refused bool
	}{
		{name: "every share a finite number", points: pointing{done: map[string]map[string]float64{"work": {"5h": 0.5}}}},
		{name: "a share done past what a float holds", points: pointing{done: map[string]map[string]float64{"work": {"5h": math.Inf(1)}}}, refused: true},
		{name: "a share of the run that isn't a number", refused: true, points: pointing{run: &runShares{accounts: map[string]*accountShares{
			"work": {windows: map[string]*windowShare{"5h": {climb: climb, share: math.NaN()}}},
		}}}},
	}
	for _, tt := range stores {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := json.Marshal(tt.points.stored()); (err != nil) != tt.refused {
				t.Errorf("stored with error %v, want it refused %t", err, tt.refused)
			}
		})
	}
}

// Readings an account's answers read at once, ended at once, without ids,
// are taken in the one order in whichever run, however its sums are seeded,
// as points stored by one run are fed on by another: by the windows they
// read, the lower use first. So of two others' answers reading work's
// five-hour at 0.3 and 0.5 together, the session's took a third of the rise
// to 0.3 alone, its tokens a third of those ended since the reading before.
func TestReadingsReadAtOnceAreTakenInTheOneOrderInWhicheverRun(t *testing.T) {
	seeded := sumsSeed
	t.Cleanup(func() { sumsSeed = seeded })
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	reset := strconv.FormatInt(at.Add(3*time.Hour).Unix(), 10)
	first := int64(100)
	answer := func(request, session string, minutes int, used string) ledger.Line {
		return ledger.Line{
			Request: request, At: at.Add(time.Duration(minutes) * time.Minute), Kind: ledger.KindMessage, Session: session, Model: foldOpus, Account: "work",
			Status: 200, Attempts: 1, TotalMS: 4000, Usage: json.RawMessage(foldUsages[2]), Limits: map[string]string{"5h-utilization": used, "5h-reset": reset},
		}
	}
	read := answer("s2", "s", 1, "0.2")
	read.FirstMS = &first
	lines := map[string][]ledger.Line{"2026-10-07": {
		answer("s1", "s", 0, "0.2"), read, answer("", "o", 5, "0.5"), answer("", "o", 5, "0.3"),
	}}
	day := Today(at)
	var want map[string]map[string]float64
	for run := range 20 {
		sumsSeed = maphash.MakeSeed()
		points := pointingOf(answersOf(lines, "s"), day).sealedAt(instantOf(at.Add(time.Hour)), day)
		if run == 0 {
			want = points.taken()
		}
		if got := points.taken(); !reflect.DeepEqual(got, want) || points.of("work")["5h"] != 3 {
			t.Fatalf("in run %d, s took %v of work's windows, %d points of its five-hour; want %v, as the first run took, 3 points", run, got, points.of("work")["5h"], want)
		}
	}
}

// An answer sums otherwise than another where any of what it holds
// differs, and alike where their lines say otherwise only of what neither
// holds; and restored from its stored form, it's the answer it was, its sum
// among it.
func TestAnAnswerSumsAllItHolds(t *testing.T) {
	first := int64(1000)
	line := func() ledger.Line {
		l := sumLine()
		l.FirstMS, l.Limits = &first, map[string]string{"5h-utilization": "0.3", "5h-reset": "1791421140", "5h-status": "allowed"}
		return l
	}
	tests := []struct {
		name  string
		page  string
		edit  func(l *ledger.Line)
		alike bool
	}{
		{name: "its id", edit: func(l *ledger.Line) { l.Request = "r2" }},
		{name: "when it arrived", edit: func(l *ledger.Line) { l.At = l.At.Add(time.Nanosecond) }},
		{name: "when it ended", edit: func(l *ledger.Line) { l.TotalMS++ }},
		{name: "when its headers came", edit: func(l *ledger.Line) { l.FirstMS = new(int64(1001)) }},
		{name: "its account", edit: func(l *ledger.Line) { l.Account = "personal" }},
		{name: "its model", edit: func(l *ledger.Line) { l.Model = foldSonnet }},
		{name: "its tokens", edit: func(l *ledger.Line) { l.Usage = json.RawMessage(foldUsages[3]) }},
		{name: "its session, the page's", edit: func(l *ledger.Line) { l.Session = "other" }},
		{name: "a window's use", edit: func(l *ledger.Line) { l.Limits["5h-utilization"] = "0.31" }},
		{name: "a window's reset", edit: func(l *ledger.Line) { l.Limits["5h-reset"] = "1791421141" }},
		{name: "a window's status", edit: func(l *ledger.Line) { l.Limits["5h-status"] = "rejected" }},
		{name: "another window read", edit: func(l *ledger.Line) { l.Limits["7d-utilization"] = "0.5" }},
		{name: "no window read", edit: func(l *ledger.Line) { l.Limits = nil }},
		{name: "its session, neither the page's", page: "page", alike: true, edit: func(l *ledger.Line) { l.Session = "other" }},
		{name: "its prompt", alike: true, edit: func(l *ledger.Line) { l.Prompt = "p2" }},
		{name: "its directory", alike: true, edit: func(l *ledger.Line) { l.Dir = "/code/other" }},
		{name: "its tokens written otherwise, as many", alike: true, edit: func(l *ledger.Line) {
			l.Usage = json.RawMessage(`{"output_tokens":500,"input_tokens":10,"cache_read_input_tokens":100000,"cache_creation_input_tokens":1000}`)
		}},
		{name: "the answer's status beside its windows", alike: true, edit: func(l *ledger.Line) { l.Limits["status"] = "allowed" }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page, base := cmp.Or(tt.page, "s"), line()
			l := line()
			tt.edit(&l)
			a := answeredOf(&l, page)
			if alike := a.sum == answeredOf(&base, page).sum; alike != tt.alike {
				t.Errorf("a line differing in %s sums alike: %t, want %t", tt.name, alike, tt.alike)
			}
			stored := a.stored()
			if restored := stored.restored(); !reflect.DeepEqual(restored, a) {
				t.Errorf("a line differing in %s, restored from its stored form, is %+v, want %+v", tt.name, restored, a)
			}
		})
	}
}
