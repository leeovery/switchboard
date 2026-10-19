package watch

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// The soak's world: requests sent at its pace's rate, two in five of them
// the long session's, which has run every day of the world's, the rest each
// of eleven seats, whose sessions run three or four days, each one's first
// of a day moving it to another account, as soakMoved says; every account's
// windows filed to the readings history every soakReadEvery; and the
// router's newest soakKept events in its document.
const (
	soakReadEvery = 10 * time.Minute
	soakKept      = 50
	soakMoved     = "rescored after 9h idle"
)

// soakPace is how busy the soak's world is: rate requests a day, each
// answer's progress told every step, each session listed by the router for
// listed after it was last seen.
type soakPace struct {
	rate   int
	step   time.Duration
	listed time.Duration
}

// ownersPace is the owner's: 25,000 requests a day, progress told every
// second, each session listed for an hour after it was last seen.
var ownersPace = soakPace{rate: 25000, step: time.Second, listed: time.Hour}

// gap is how long after each request the world sends the next.
func (p soakPace) gap() time.Duration {
	return 24 * time.Hour / time.Duration(p.rate)
}

// soakRun is when the router the soak's world reads started, as its health
// check and its events' files say.
var soakRun = start.Add(-30 * 24 * time.Hour).UTC()

// soakWorld is a state directory of the soak's own, as busy as the owner's,
// run from the start of its first day to the clock's time, at: each request
// sent in turn, told of as the router's stream tells, and filed on the
// ledger as it ends; the readings and the router's events filed; each day
// summarised once it has ended, and digested by the views' Digests, and its
// files compressed the day after, as the router has them.
type soakWorld struct {
	busyWorld
	tb         testing.TB
	pace       soakPace
	origin, at time.Time
	ledger     *ledger.Ledger
	// sent is the next request to send, flight those in flight, oldest first,
	// on says the account each session went out on last, and seen when each
	// was last seen, by its number.
	sent   int
	flight []soakRequest
	on     map[int]string
	seen   map[int]time.Time
	// events are the router's newest, next the id of the next; filed counts
	// the lines filed, and filedOn those filed on each day, by its date; and
	// pending are the lines yet to be written, by the path of the file they
	// go on.
	events  []status.Event
	next    int
	filed   int
	filedOn map[string]int
	pending map[string]*bytes.Buffer
}

// soakRequest is a request of the soak's: its number, its session's, the
// world's day it went out on, and when it went, its answer's first byte
// came and it ended; and the account it moved its session from, where it
// went out on another than the session's last.
type soakRequest struct {
	n, g, day         int
	sent, first, done time.Time
	firstTold         bool
	from              string
}

// arrived is when r arrived, moments before it went upstream, as its line
// says.
func (r soakRequest) arrived() time.Time {
	return r.sent.Add(-5 * time.Millisecond)
}

// newSoakWorld writes the soak's world, at pace, as it stands at now, run
// from the start of the day days before now's.
func newSoakWorld(tb testing.TB, now time.Time, days int, pace soakPace) *soakWorld {
	tb.Helper()
	w := &soakWorld{state: tb.TempDir(), now: now, tb: tb, pace: pace, on: make(map[int]string), seen: make(map[int]time.Time), next: 1,
		filedOn: make(map[string]int), pending: make(map[string]*bytes.Buffer)}
	w.origin = dayfile.DayStart(now, 1-days)
	w.at = w.origin
	for _, d := range []string{ledger.Dir(w.state), readings.Dir(w.state)} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			tb.Fatal(err)
		}
	}
	w.ledger = w.opened()
	for w.at.Before(now) {
		next := w.at.Add(time.Minute)
		if next.After(now) {
			next = now
		}
		w.advance(w.at, next)
	}
	w.flush()
	w.round(now)
	return w
}

// opened is the world's ledger as the router opens it, by the world's
// clock: digesting its days by the views' Digests, holding nothing of a
// pass made before.
func (w *soakWorld) opened() *ledger.Ledger {
	quiet := slog.New(slog.DiscardHandler)
	return ledger.Open(ledger.Dir(w.state), 400*24*time.Hour, func() time.Time { return w.at }, ledger.HistoryIn(readings.Files(readings.Dir(w.state), quiet)),
		w.caps(), quiet, ledger.Digesting(views.Digests{}))
}

// round is the router's round at at, its files written: the days that have
// ended summarised, and each finished day due digested by the views'
// Digests, as its digester does once the round kicks it, at once here.
func (w *soakWorld) round(at time.Time) {
	w.ledger.SummariseEnded(at)
	w.ledger.DigestEnded(w.tb.Context(), at)
}

// advance runs the world from was to at, returning what the router's stream
// tells meanwhile: each request due sent, a move told first where its
// session goes out on another account than it did last; each in flight
// told of, its line filed as it ends; the readings filed as each falls due;
// and the router telling of an account capped every three hours, with room
// again forty minutes on. The router's round comes as a day begins, the
// files of the day before the one that ended compressed, and on the hour.
func (w *soakWorld) advance(was, at time.Time) []router.StreamEvent {
	w.tb.Helper()
	w.at = at
	switch {
	case dayfile.DateOf(was) != dayfile.DateOf(at):
		w.flush()
		w.round(at)
		w.compress(dayfile.DateOf(dayfile.DayStart(at, -2)))
	case crossed(was, at, at.Truncate(time.Hour)):
		w.flush()
		w.round(at)
	}
	var told []router.StreamEvent
	for at.Sub(w.origin) >= time.Duration(w.sent)*w.pace.gap() {
		told = append(told, w.send()...)
	}
	told = append(told, w.progress(at)...)
	if was.Truncate(soakReadEvery) != at.Truncate(soakReadEvery) {
		w.fileReadings(at.Truncate(soakReadEvery))
	}
	if hour := at.Truncate(time.Hour); hour.Hour()%3 == 0 && crossed(was, at, hour) {
		w.fileEvent(status.Event{At: hour.UTC(), Kind: status.EventCap, Account: w.capped(hour), Windows: []string{"5h"}})
	}
	if hour := at.Truncate(time.Hour); hour.Hour()%3 == 0 && crossed(was, at, hour.Add(40*time.Minute)) {
		w.fileEvent(status.Event{At: hour.Add(40 * time.Minute).UTC(), Kind: status.EventRoom, Account: w.capped(hour)})
	}
	return told
}

// crossed reports whether the world, run from was to at, passed mark.
func crossed(was, at, mark time.Time) bool {
	return was.Before(mark) && !at.Before(mark)
}

// capped is the account the router tells of reaching its cap at the hour
// given, the next in turn every three hours.
func (w *soakWorld) capped(hour time.Time) string {
	return busyAccounts[hour.Hour()/3%len(busyAccounts)]
}

// seatOf is the session of the world's nth request, on the world's dth day:
// the long one, two in five; else the next of the day's eleven seats.
func seatOf(n, d int) int {
	if n%5 < 2 {
		return busyLong
	}
	return busySeat(d, 1+n%(busyRunning-1))
}

// send sends the world's next request: told of as moving its session first,
// where it goes out on another account than the session did last.
func (w *soakWorld) send() []router.StreamEvent {
	r := w.request(w.sent)
	w.sent++
	account := busyOn(r.g, r.day)
	var told []router.StreamEvent
	if was, ok := w.on[r.g]; ok && was != account {
		r.from = was
		moved := w.event(r, router.StreamMoved, r.sent)
		moved.From, moved.To, moved.Reason = was, account, soakMoved
		told = append(told, moved)
		w.fileEvent(status.Event{At: r.sent.UTC(), Kind: status.EventMoved, Session: busySession(r.g), From: was, To: account, Reason: soakMoved})
	}
	w.flight = append(w.flight, r)
	w.seen[r.g] = r.sent
	w.on[r.g] = account
	return append(told, w.event(r, router.StreamSent, r.sent))
}

// request is the world's nth request, as it's sent: when, by the world's
// pace, and of which session, its answer's first byte a second to four on
// and its end fifteen seconds to a minute on.
func (w *soakWorld) request(n int) soakRequest {
	sent := w.origin.Add(time.Duration(n) * w.pace.gap())
	d := w.dayOf(sent)
	return soakRequest{n: n, g: seatOf(n, d), day: d, sent: sent, first: sent.Add(time.Duration(1+n%4) * time.Second),
		done: sent.Add(time.Duration(15+n%7*6) * time.Second)}
}

// filedFrom are the requests whose lines the world has filed by its time
// that arrived from from on, in the order they were sent.
func (w *soakWorld) filedFrom(from time.Time) []soakRequest {
	var filed []soakRequest
	for n := max(int(from.Sub(w.origin)/w.pace.gap()), 0); n < w.sent; n++ {
		if r := w.request(n); !r.arrived().Before(from) && !r.done.After(w.at) {
			filed = append(filed, r)
		}
	}
	return filed
}

// progress is what the stream tells by at of the requests in flight: each
// one's first byte, as it comes; its answer's progress; and its end, its
// line filed as it ends.
func (w *soakWorld) progress(at time.Time) []router.StreamEvent {
	var told []router.StreamEvent
	for i := range w.flight {
		r := &w.flight[i]
		switch {
		case !r.done.After(at):
			done := w.event(*r, router.StreamDone, r.done)
			done.Status, done.Chars = 200, chars(*r, r.done)
			told = append(told, done)
			w.file(*r)
		case !r.first.After(at) && !r.firstTold:
			r.firstTold = true
			told = append(told, w.event(*r, router.StreamFirst, r.first))
		case r.firstTold:
			progress := w.event(*r, router.StreamProgress, at)
			progress.Chars = chars(*r, at)
			told = append(told, progress)
		}
	}
	w.flight = slices.DeleteFunc(w.flight, func(r soakRequest) bool { return !r.done.After(at) })
	return told
}

// chars is how many characters the answer to r has streamed by at.
func chars(r soakRequest, at time.Time) int {
	return max(int(at.Sub(r.first)/time.Millisecond), 0)
}

// event is what the stream tells of r at at, as kind says.
func (w *soakWorld) event(r soakRequest, kind string, at time.Time) router.StreamEvent {
	return router.StreamEvent{At: at.UTC(), Kind: kind, Request: soakID(r.n), Attempt: 1, Session: busySession(r.g), Dir: fmt.Sprintf("~/Code/project-%d", r.g),
		Model: "claude-opus-5-5", Account: busyOn(r.g, r.day), Prompt: soakPrompt(r), Class: "main"}
}

// inFlight is what the stream tells of the requests in flight at now, as it
// opens.
func (w *soakWorld) inFlight(now time.Time) []router.StreamEvent {
	var told []router.StreamEvent
	for _, r := range w.flight {
		e := w.event(r, router.StreamInFlight, now)
		e.SentAt = r.sent.UTC()
		if r.firstTold {
			e.FirstAt, e.Chars = r.first.UTC(), chars(r, now)
		}
		told = append(told, e)
	}
	return told
}

// soakID is the id of the world's nth request.
func soakID(n int) string {
	return fmt.Sprintf("%016x", soakNumber(n))
}

func soakNumber(n int) uint64 {
	return uint64(0xa000000000000000) + uint64(n)
}

// soakPrompt is the prompt r answers, thirty requests a prompt.
func soakPrompt(r soakRequest) string {
	return fmt.Sprintf("%s-%d-%d", busySession(r.g), r.day, r.n/busyRunning/30)
}

// file files r's line on the ledger, as the router does as it ends: on the
// day it arrived, moments before it went upstream, telling of the move it
// made, where it moved its session.
func (w *soakWorld) file(r soakRequest) {
	at := r.arrived()
	reason, from := status.ReasonSticky, ""
	if r.from != "" {
		reason, from = soakMoved, fmt.Sprintf(`,"from":%q`, r.from)
	}
	fifth := float64(at.Hour()%5*60+at.Minute()) / 300
	var line bytes.Buffer
	fmt.Fprintf(&line, busyLine, at.UTC().Format("2006-01-02T15:04:05.000Z"), soakNumber(r.n), busySession(r.g), r.g, busyOn(r.g, r.day),
		reason, from, r.first.Sub(at).Milliseconds(), r.done.Sub(at).Milliseconds(), soakPrompt(r),
		400000+r.n%500000, 20+r.n%300, uint64(r.n), r.n%100, r.n%20000, r.n%400000, r.n%20000, r.n%8000,
		fifth, at.Truncate(5*time.Hour).Add(5*time.Hour).Unix(), float64(r.day%7)/7+fifth/7, w.day(r.day).AddDate(0, 0, 7-r.day%7).Unix())
	w.write(filepath.Join(ledger.Dir(w.state), "requests-"+dayfile.DateOf(at)+".jsonl"), line.Bytes())
	w.filed++
	w.filedOn[dayfile.DateOf(at)]++
}

// fileReadings files every account's windows, as they stand at at, to the
// readings history, as the router files those that changed.
func (w *soakWorld) fileReadings(at time.Time) {
	w.tb.Helper()
	var lines bytes.Buffer
	for _, a := range w.accounts(at) {
		for _, win := range a.Windows {
			data, err := json.Marshal(readings.Reading{At: at.UTC(), Account: a.ID, Key: win.Key, Utilization: win.Utilization, ResetsAt: win.ResetsAt,
				Status: quota.StatusAllowed, Source: readings.FromAnswer})
			if err != nil {
				w.tb.Fatal(err)
			}
			lines.Write(append(data, '\n'))
		}
	}
	w.write(filepath.Join(readings.Dir(w.state), "readings-"+dayfile.DateOf(at)+".jsonl"), lines.Bytes())
}

// fileEvent has the router keep e, its newest soakKept in its document, and
// file it on the day it fell on.
func (w *soakWorld) fileEvent(e status.Event) {
	e.ID, w.next = w.next, w.next+1
	w.events = append(w.events, e)
	if len(w.events) > soakKept {
		w.events = slices.Clone(w.events[len(w.events)-soakKept:])
	}
	data, err := events.Marshal(events.Line{Event: e, Run: soakRun})
	if err != nil {
		w.tb.Fatal(err)
	}
	w.write(filepath.Join(ledger.Dir(w.state), "events-"+dayfile.DateOf(e.At)+".jsonl"), append(data, '\n'))
}

// write holds data to append to the file at path, as flush writes it.
func (w *soakWorld) write(path string, data []byte) {
	b, ok := w.pending[path]
	if !ok {
		b = new(bytes.Buffer)
		w.pending[path] = b
	}
	b.Write(data)
}

// flush appends what's held to write to each file, made where it isn't.
func (w *soakWorld) flush() {
	w.tb.Helper()
	for _, path := range slices.Sorted(maps.Keys(w.pending)) {
		file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY|os.O_CREATE, 0o600)
		if err != nil {
			w.tb.Fatal(err)
		}
		_, err = file.Write(w.pending[path].Bytes())
		if closed := file.Close(); err == nil {
			err = closed
		}
		if err != nil {
			w.tb.Fatal(err)
		}
	}
	clear(w.pending)
}

// compress compresses the files of the day with the given date, as the
// router does once the day after it has ended.
func (w *soakWorld) compress(date string) {
	w.tb.Helper()
	for _, path := range []string{
		filepath.Join(ledger.Dir(w.state), "requests-"+date+".jsonl"),
		filepath.Join(ledger.Dir(w.state), "events-"+date+".jsonl"),
		filepath.Join(readings.Dir(w.state), "readings-"+date+".jsonl"),
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		writeDay(w.tb, filepath.Dir(path), filepath.Base(path), data, true)
		if err := os.Remove(path); err != nil {
			w.tb.Fatal(err)
		}
	}
}

// accounts are the world's accounts as they stand at at: each 5-hour window
// as far through as its five hours are, and each week as far as its seven
// days, each account further than the one before.
func (w *soakWorld) accounts(at time.Time) []status.Account {
	accounts := make([]status.Account, len(busyAccounts))
	fifth := at.Truncate(5 * time.Hour)
	week := dayfile.DayStart(at, -int(at.Weekday()+6)%7)
	hours := float64(at.Sub(fifth)) / float64(5*time.Hour)
	days := float64(at.Sub(week)) / float64(7*24*time.Hour)
	for i, id := range busyAccounts {
		accounts[i] = status.Account{ID: id, Label: strings.ToUpper(id[:1]) + id[1:], TokenSet: true, FetchedAt: at.UTC(), Windows: []quota.Window{
			{Key: "5h", Label: "Session", Utilization: hours * float64(i+1) / 5, ResetsAt: fifth.Add(5 * time.Hour).UTC()},
			{Key: "7d", Label: "Week", Utilization: days * float64(i+1) / 5, ResetsAt: week.AddDate(0, 0, 7).UTC()},
		}}
	}
	return accounts
}

// document is the router's document at now.
func (w *soakWorld) document(now time.Time) status.Document {
	doc := routerDocument(w.accounts(now)...)
	doc.GeneratedAt, doc.Events, doc.Router.Requests = now.UTC(), slices.Clone(w.events), w.sent
	return doc
}

// listed are the sessions the router lists at now: those seen within its
// pace's listed, the one seen last first, each on the account it went out
// on last.
func (w *soakWorld) listed(now time.Time) []status.Session {
	var sessions []status.Session
	for g, seen := range w.seen {
		if now.Sub(seen) > w.pace.listed {
			delete(w.seen, g)
			continue
		}
		sessions = append(sessions, status.Session{ID: busySession(g), Assignments: []status.Assignment{{Dir: fmt.Sprintf("~/Code/project-%d", g),
			Model: "claude-opus-5-5", Family: "opus", Account: w.on[g], Reason: status.ReasonSticky, AssignedAt: dayfile.DayStart(seen, 0).UTC(),
			LastSeen: seen.UTC()}}})
	}
	slices.SortFunc(sessions, func(a, b status.Session) int {
		return cmp.Or(b.Assignments[0].LastSeen.Compare(a.Assignments[0].LastSeen), strings.Compare(a.ID, b.ID))
	})
	return sessions
}

// dayOf is the world's day at falls on, numbered as busyWorld.day numbers
// them, the day the soak starts on busyDays-1.
func (w *soakWorld) dayOf(at time.Time) int {
	d := busyDays - 1
	for at.Before(w.day(d)) {
		d--
	}
	for !at.Before(w.day(d + 1)) {
		d++
	}
	return d
}

// longSent is how many of the requests sent are the long session's.
func (w *soakWorld) longSent() int {
	return w.sent/5*2 + min(w.sent%5, 2)
}
