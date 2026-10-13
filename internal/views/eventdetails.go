package views

import (
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// EventDetails is an event's details, as the Log's card of it holds them:
// the row it's of, which the card's title takes its time from, and its
// sections, WHAT HAPPENED, `<ACCOUNT>` THEN and WHAT FOLLOWED, each worked
// out from the event, the ledger's lines read and the rows of the moves it
// counted.
type EventDetails struct {
	// Row is the event's row.
	Row LogEvent
	// Headline is what happened, as the card's title and its `event` item
	// say it: "work reached its cap".
	Headline status.Words
	// Happened are WHAT HAPPENED's items: `event`, then those of what the
	// kind carries, worded as the Events table words them.
	Happened []DetailItem
	// KnownFrom is the request a cap, a limit or a refusal was known from,
	// as Happened's `known from` names it: nil for any other event, and
	// where no line read tells of one.
	KnownFrom *KnownRequest
	// Then is `<ACCOUNT>` THEN, of an event naming one account: nil for any
	// other.
	Then *AccountThen
	// Followed is WHAT FOLLOWED, of a limit, a cap or a refusal, and of a
	// move a line tells of: nil for any other.
	Followed *Followed
}

// DetailItem is an item of a details card's section: its name, as `known
// from`, and its value, in the parts it's drawn in.
type DetailItem struct {
	Name  string
	Value DetailWords
}

// DetailWords are a value of a details card's item, in the parts it's drawn
// in.
type DetailWords []DetailPart

// DetailPart is a run of DetailWords, of the kind it is, which says how it's
// drawn; Use is a DetailUse's share used, which its level is of.
type DetailPart struct {
	Text string
	Kind DetailPartKind
	Use  float64
}

// DetailPartKind is what a DetailPart is.
type DetailPartKind int

const (
	// DetailSaid is words around a value, as " of its 5-hour window".
	DetailSaid DetailPartKind = iota
	// DetailValue is a value: a time, a count or a session's id.
	DetailValue
	// DetailUse is a window's use, as "95%", drawn in its level's colour.
	DetailUse
	// DetailAccount is an account, by its id, a value the card names as the
	// document names it.
	DetailAccount
)

// String runs the words together, as a card drawn without colour shows them.
func (w DetailWords) String() string {
	var b strings.Builder
	for _, p := range w {
		b.WriteString(p.Text)
	}
	return b.String()
}

// KnownRequest is a request an event was known from: the router's id for
// it, its session's whole id, "" for none, and when it arrived.
type KnownRequest struct {
	Request, Session string
	At               time.Time
}

// AccountThen is how the account an event befell stood as it happened.
type AccountThen struct {
	Account string
	// Windows are its windows as the last answer on it at or before the
	// event read them, in quota's order: none where no line read tells.
	Windows []WindowThen
	// Resets is when what the event tells of resets: a cap's last window, a
	// limit lifting, a window a prime started; else the first window the
	// event names, or else the first of Windows, as they read it. It's zero
	// where that isn't known.
	Resets time.Time
	// Sessions counts the sessions whose requests went out on it in the hour
	// before.
	Sessions int
}

// Followed is what followed an event: of a limit, a cap or a refusal, the
// moves it counted, till when new sessions go elsewhere, and what the moves
// cost together; of a move, what its move came to.
type Followed struct {
	// Moves are the moves the event counted, in the order they came.
	Moves []FollowedMove
	// Till is when new sessions go elsewhere till, as the event says: zero
	// where it doesn't. Family is the model family whose requests alone go
	// elsewhere, of a refusal of its alone: "" for every request.
	Till   time.Time
	Family string
	// InAll is what the moves cost together, as their rows say each: nil
	// where no move's cost is known.
	InAll *ledger.Picodollars
	// Move is a move's own move, as its row says it: nil for any other
	// event.
	Move *EventMove
}

// FollowedMove is a move an event counted, as its row says it: the move
// event's key, the session moved and the account it went to, and what its
// move came to, nil where no line read tells of it.
type FollowedMove struct {
	Key         EventKey
	Session, To string
	Move        *EventMove
}

// sessionsBefore is how long before an event the sessions on its account
// are counted from.
const sessionsBefore = time.Hour

// Details are the details of the event with the given key, one of every
// event the table read, those the filter hides among them, the lines read
// telling what's known of it: false where it read none with the key.
func (t EventsTable) Details(key EventKey, read LogRead) (EventDetails, bool) {
	i := slices.IndexFunc(t.told, func(e toldEvent) bool { return e.row.Key.same() == key.same() })
	if i < 0 {
		return EventDetails{}, false
	}
	d := detailing{toldEvent: t.told[i], table: t, lines: read.lines}
	return d.details(), true
}

// detailing works out an event's details: the event, told; the table it's
// of; and the ledger's lines read, as the Log's read holds them.
type detailing struct {
	toldEvent
	table EventsTable
	lines []*loggedLine
}

func (d detailing) details() EventDetails {
	details := EventDetails{Row: d.row, Headline: d.headline()}
	details.Happened = append([]DetailItem{{Name: "event", Value: detailWords(details.Headline)}}, d.happened()...)
	if holdsBack(d.event.Kind) {
		details.KnownFrom = d.knownFrom()
		if details.KnownFrom != nil {
			details.Happened = append(details.Happened, DetailItem{Name: "known from", Value: d.knownWords(*details.KnownFrom)})
		}
	}
	if len(d.row.Accounts) == 1 {
		details.Then = d.then(d.row.Accounts[0])
	}
	details.Followed = d.followed()
	return details
}

// holdsBack reports whether the kind given holds an account back, moving
// its sessions: a limit, a cap or a refusal.
func holdsBack(kind string) bool {
	return kind == status.EventLimit || kind == status.EventCap || kind == status.EventRefused
}

// headline is what happened, as the card's title says it.
func (d detailing) headline() status.Words {
	e := d.event
	who, id := account(e.Account), session(e.Session)
	switch e.Kind {
	case status.EventStarted:
		return slices.Concat(id, clause(" started on "), who)
	case status.EventMoved:
		if why := status.EventWords(e.Reason, status.Facts{From: e.From, To: e.To, Until: d.fact.runsOut, Now: d.row.At}); why.String() != "" {
			return why
		}
		return slices.Concat(id, clause(" moved"))
	case status.EventPressure:
		return slices.Concat(who, clause(" came under pressure"))
	case status.EventCap:
		return slices.Concat(who, clause(" reached its cap"))
	case status.EventLimit:
		return slices.Concat(who, clause(" reached its limit"))
	case status.EventRefused:
		if e.Family != "" {
			return slices.Concat(who, clause("'s "+prose.Clean(e.Family)+" requests refused"))
		}
		return slices.Concat(who, clause("'s token refused"))
	case status.EventPrimed:
		return slices.Concat(who, clause(" primed"))
	case status.EventRoom:
		return slices.Concat(who, clause(" open again"))
	case status.EventPin:
		if e.Session != "" {
			return slices.Concat(id, clause(" pinned to "), who)
		}
		return clause("pin set by hand")
	case status.EventAuto:
		if e.Session != "" {
			return slices.Concat(id, clause(" back to the router's choice"))
		}
		return clause("new sessions back to the router's choice")
	case status.EventRestart:
		return clause("restart due")
	case status.EventHealth:
		if d.row.Well {
			return clause("the router is healthy again")
		}
		return clause("the router turned unhealthy")
	}
	return slices.Concat(who, clause(" "+KindWord(e.Kind)))
}

// happened are WHAT HAPPENED's items after `event`, as the kind carries
// them: known from aside, which details adds.
func (d detailing) happened() []DetailItem {
	e := d.event
	var items itemList
	switch e.Kind {
	case status.EventStarted:
		items.add("session", d.sessionWords())
		items.add("model", valued(ledger.ShortModelID(prose.Clean(e.Model))))
		if !isNewReason(e.Reason) {
			items.add("why", detailWords(status.EventWords(e.Reason, status.Facts{To: e.Account, Until: d.fact.runsOut, Now: d.row.At})))
		}
	case status.EventMoved:
		items.add("session", d.sessionWords())
		items.add("model", valued(ledger.ShortModelID(prose.Clean(e.Model))))
		items.add("from", accountWords(e.From))
		items.add("to", accountWords(e.To))
	case status.EventPressure:
		items.add("window", d.windowsWords())
		items.add("runs out", d.timeWords(e.Until))
		items.add("rate since", d.timeWords(e.Since))
	case status.EventCap:
		items.add("its cap", d.capWords())
	case status.EventLimit:
		items.add("its limit", d.windowsWords())
		items.add("till", d.timeWords(e.Until))
	case status.EventRefused:
		if e.Status != 0 {
			items.add("answer", valued(strconv.Itoa(e.Status)))
		}
		items.add("family", valued(prose.Clean(e.Family)))
		items.add("till", d.timeWords(e.Until))
	case status.EventPrimed, status.EventRoom:
		items.add("window", d.windowsWords())
	case status.EventPin, status.EventAuto:
		items = d.pinned()
	case status.EventRestart:
		items.add("why", saidWords(restartWhy(e.Reason)))
	case status.EventHealth:
		items.add("why", saidWords(prose.Clean(e.Reason)))
	}
	return items
}

// itemList is a section's items, as they're added.
type itemList []DetailItem

// add adds the item with the given name and value, where it has one.
func (l *itemList) add(name string, value DetailWords) {
	if value.String() != "" {
		*l = append(*l, DetailItem{Name: name, Value: value})
	}
}

// pinned are the items of routing set or given back by hand: of a
// session's own pin, the session and its account; of the global pin, its
// accounts, which sessions it sends there, as set, and whether it cleared
// every session's own; and who did it.
func (d detailing) pinned() itemList {
	e := d.event
	var items itemList
	name := "accounts"
	if e.Kind == status.EventAuto {
		name = "from"
	}
	if e.Session != "" {
		items.add("session", d.sessionWords())
		items.add(name, accountWords(e.Account))
	} else {
		items.add(name, accountWords(e.Accounts...))
	}
	if e.Kind == status.EventPin && e.Session == "" {
		goes := "new sessions"
		if e.Move {
			goes = "new and running sessions"
		}
		items.add("goes", saidWords(goes))
	}
	if e.Force {
		items.add("own pins", saidWords("every session's cleared"))
	}
	items.add("set from", saidWords(setFrom[e.By]))
	return items
}

// isNewReason reports whether a session started for no reason but being
// new.
func isNewReason(reason string) bool {
	reason = prose.Clean(reason)
	return reason == "" || reason == status.ReasonNew
}

// sessionWords name the event's session as the table names one: its id,
// cut, and its directory's name.
func (d detailing) sessionWords() DetailWords {
	id := status.ShownID(d.event.Session)
	if id == "" {
		return nil
	}
	w := valued(id)
	if dir := status.DirName(d.row.Dir, dirCells); dir != "" {
		w = append(w, DetailPart{Text: " " + dir})
	}
	return w
}

// dirCells is how many cells the Log names a session's directory in.
const dirCells = 12

// windowsWords name the windows the event names, as its line does: "its
// 5-hour window", or "its 5-hour and week windows"; none where it names
// none.
func (d detailing) windowsWords() DetailWords {
	return valued(its(d.fact.windows, ""))
}

// capWords say a cap as its line does: "95% of its 5-hour window", its use
// drawn as a window's is; its windows alone without a reserve.
func (d detailing) capWords() DetailWords {
	if d.event.Reserve <= 0 {
		return d.windowsWords()
	}
	used := 1 - d.event.Reserve
	return DetailWords{{Text: prose.Percent(used), Kind: DetailUse, Use: used}, {Text: " of " + its(d.fact.windows, "its window")}}
}

// timeWords say a time, as prose.Dated says it beside the event: none for
// zero.
func (d detailing) timeWords(t time.Time) DetailWords {
	if t.IsZero() {
		return nil
	}
	return valued(prose.Dated(d.row.At, t))
}

// knownWords name the request an event was known from: "7f3a's request at
// 14:30:51", or "a request at 14:30:51" of none.
func (d detailing) knownWords(k KnownRequest) DetailWords {
	at := valued(prose.ToTheSecond(d.row.At, k.At.In(d.row.At.Location())))
	if id := status.ShownID(k.Session); id != "" {
		return slices.Concat(valued(id), saidWords("'s request at "), at)
	}
	return slices.Concat(saidWords("a request at "), at)
}

// valued is a value alone: none for "".
func valued(text string) DetailWords {
	if text == "" {
		return nil
	}
	return DetailWords{{Text: text, Kind: DetailValue}}
}

// accountWords name the accounts with the given ids, a comma between: none
// for none.
func accountWords(ids ...string) DetailWords {
	var w DetailWords
	for _, id := range cleaned(ids) {
		if len(w) > 0 {
			w = append(w, DetailPart{Text: ", "})
		}
		w = append(w, DetailPart{Text: id, Kind: DetailAccount})
	}
	return w
}

// saidWords are words around no value: none for "".
func saidWords(text string) DetailWords {
	if text == "" {
		return nil
	}
	return DetailWords{{Text: text}}
}

// detailWords are status's words as a card's value draws them: each account
// an account, and each figure and span a value.
func detailWords(w status.Words) DetailWords {
	said := make(DetailWords, len(w))
	for i, p := range w {
		said[i] = DetailPart{Text: p.Text}
		switch p.Kind {
		case status.PartPlain:
		case status.PartAccount:
			said[i].Kind = DetailAccount
		default:
			said[i].Kind = DetailValue
		}
	}
	return said
}

// onAccount are the lines read of requests that went out on the account
// with the given id, there or tried there, that arrived at or before at, in
// the order they arrived.
func (d detailing) onAccount(id string, at time.Time) []*ledger.Line {
	var on []*ledger.Line
	for _, h := range d.lines {
		l := &h.line
		if !l.At.After(at) && (l.Account == id || triedOn(l, id, "")) {
			on = append(on, l)
		}
	}
	slices.SortStableFunc(on, func(a, b *ledger.Line) int { return a.At.Compare(b.At) })
	return on
}

// triedOn reports whether the request l went out on the account with the
// given id and was left there for why, any why for "".
func triedOn(l *ledger.Line, id, why string) bool {
	return slices.ContainsFunc(l.Tried, func(t ledger.Tried) bool { return t.Account == id && (why == "" || t.Why == why) })
}

// knownFrom is the request the event, a limit, a cap or a refusal, was
// known from: of the requests on its account up to it, the first that told
// of what it tells after the last that told otherwise, as knowing judges
// each. It's nil where none did.
func (d detailing) knownFrom() *KnownRequest {
	var first *ledger.Line
	for _, l := range d.onAccount(d.event.Account, d.event.At) {
		switch tells, otherwise := d.knowing(l); {
		case tells && first == nil:
			first = l
		case otherwise:
			first = nil
		}
	}
	if first == nil {
		return nil
	}
	return &KnownRequest{Request: first.Request, Session: first.Session, At: first.At}
}

// knowing judges whether the request l, on or tried on the event's account,
// told of what the event tells, or told otherwise: of a cap, its answer read
// one of the cap's windows holding the account back, or each it read
// holding it back at nothing; of a limit, it met the limit there, answered
// 429, or its answer read one of the limit's windows holding it back at its
// limit, or it was answered there with room; of a refusal, it was refused
// there, or answered there, a request of the family refused alone, where
// that's all that's refused.
func (d detailing) knowing(l *ledger.Line) (tells, otherwise bool) {
	e := d.event
	on := l.Account == e.Account
	switch e.Kind {
	case status.EventCap:
		atCap, read := d.reads(l, score.HeldAtCap)
		return on && atCap, on && read && !atCap
	case status.EventLimit:
		spent, _ := d.reads(l, score.HeldAtLimit)
		spent = on && spent
		return triedOn(l, e.Account, status.WhyLimit) || on && l.Status == http.StatusTooManyRequests || spent, on && !l.Failed() && !spent
	case status.EventRefused:
		if e.Family != "" && (claude.Provider{}).Family(l.Model) != prose.Clean(e.Family) {
			return false, false
		}
		refused := on && (l.Status == http.StatusUnauthorized || l.Status == http.StatusForbidden)
		return triedOn(l, e.Account, status.WhyRefused) || refused, on && !l.Failed()
	}
	return false, false
}

// reads judges the windows the answer to l read that the event names, every
// one it read where it names none, as claude.WindowsRead judges them by the
// event's reserve: whether any held the account back as far as least, or
// further, and whether it read any.
func (d detailing) reads(l *ledger.Line, least score.Hold) (held, read bool) {
	for _, w := range claude.WindowsRead(l.Limits, d.event.Reserve) {
		if len(d.event.Windows) > 0 && !slices.Contains(d.event.Windows, w.Key) {
			continue
		}
		read, held = true, held || w.Held >= least
	}
	return held, read
}

// then is how the account with the given id stood as the event happened:
// its windows as the last answer on it at or before then read them; when
// what the event tells of resets; and how many sessions' requests went out
// on it in the hour before, there or tried there, as a day's summary counts
// sessions, a quota check counting toward none.
func (d detailing) then(id string) *AccountThen {
	then := &AccountThen{Account: id}
	sessions := make(map[string]bool)
	for _, l := range d.onAccount(id, d.event.At) {
		if session := l.CountedSession(); session != "" && !l.At.Before(d.event.At.Add(-sessionsBefore)) {
			sessions[session] = true
		}
		if windows := claude.WindowsRead(l.Limits, d.event.Reserve); l.Account == id && len(windows) > 0 {
			then.Windows = windowsThen(windows, d.table.names)
		}
	}
	then.Sessions, then.Resets = len(sessions), d.resets(then.Windows)
	return then
}

// resets is when what the event tells of resets, as AccountThen's Resets
// says, of the windows as they read then.
func (d detailing) resets(windows []WindowThen) time.Time {
	e := d.event
	switch {
	case !e.Until.IsZero() && (e.Kind == status.EventCap || e.Kind == status.EventLimit || e.Kind == status.EventPrimed):
		return e.Until
	case len(windows) == 0:
		return time.Time{}
	}
	if len(e.Windows) > 0 {
		if i := slices.IndexFunc(windows, func(w WindowThen) bool { return w.Key == e.Windows[0] }); i >= 0 {
			return windows[i].Resets
		}
	}
	return windows[0].Resets
}

// followed is what followed the event: of a limit, a cap or a refusal, the
// moves it counted, as their rows say them, in the order they came, till
// when new sessions go elsewhere, and what the moves cost together; of a
// move a line tells of, what its move came to; nil for any other.
func (d detailing) followed() *Followed {
	switch {
	case d.event.Kind == status.EventMoved && d.row.Move != nil:
		return &Followed{Move: d.row.Move}
	case !holdsBack(d.event.Kind):
		return nil
	}
	f := &Followed{Till: d.event.Until, Family: prose.Clean(d.event.Family)}
	for _, e := range d.table.told {
		if e.event.Kind != status.EventMoved || foldedInto(e.event) != d.event.ID || !e.row.Key.Run.Equal(d.row.Key.Run) {
			continue
		}
		f.Moves = append(f.Moves, FollowedMove{Key: e.row.Key, Session: e.event.Session, To: e.event.To, Move: e.row.Move})
		if e.row.Move != nil && e.row.Move.Cost != nil {
			f.InAll = summedCost(f.InAll, *e.row.Move.Cost)
		}
	}
	return f
}

// summedCost is sum with c added: c where sum is nil.
func summedCost(sum *ledger.Picodollars, c ledger.Picodollars) *ledger.Picodollars {
	if sum != nil {
		c += *sum
	}
	return &c
}
