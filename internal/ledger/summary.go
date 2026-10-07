package ledger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"iter"
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/quota"
)

const (
	// summaryVersion is the version of the summaries the ledger writes. A
	// release that summarises a day otherwise, or keeps more of it, writes
	// another, and so tells the summaries written before it: those whose lines
	// are still kept can be summarised again, and a reader knows what the
	// others never counted, rather than taking it for none.
	summaryVersion = 1
	// readingsBefore is how far before a day the readings that say the use
	// each window began it at are looked for: a week, the longest a window
	// runs, so a window read before then has reset since.
	readingsBefore = 7 * 24 * time.Hour
)

// Summary is a day of the ledger, summarised from its lines and the readings
// history: written once its day has ended, and kept for good. Fields may be
// added to it, never renamed; Version says which a summary was written with.
type Summary struct {
	Version int `json:"version"`
	// Day is the local day's date, as the ledger's files are named for it.
	Day string `json:"day"`
	// Accounts are the accounts' days, in the order of their ids: one of no
	// account holds the requests the router answered without one, as when no
	// account had room, or a request's body couldn't be read.
	Accounts []AccountDay `json:"accounts,omitempty"`
}

// AccountDay is an account's day: its requests, by the model each asked for,
// and, of the account as a whole, its sessions, those moved onto it and off
// it, each window's highest use, and the limits it reached.
type AccountDay struct {
	// Account is the account's id, left out of the requests no account
	// answered.
	Account string `json:"account,omitempty"`
	// Models are the account's requests by the model each asked for, in the
	// order of their names.
	Models []ModelDay `json:"models,omitempty"`
	// Sessions counts the sessions its requests were of, as counted says,
	// and MovedOn and MovedOff those its requests moved onto it, and off it.
	Sessions int `json:"sessions"`
	MovedOn  int `json:"moved_on"`
	MovedOff int `json:"moved_off"`
	// Highest is each window's highest use that day, by its key: the use it
	// began the day at among it, unless the window had reset by then.
	Highest map[string]float64 `json:"highest,omitempty"`
	// Limits are the limits the account reached, in the order it reached them.
	Limits []Limit `json:"limits,omitempty"`
}

// ModelDay is a model's day on an account: its requests, by how each went,
// the sessions they were of, and their usage.
type ModelDay struct {
	// Model is the model the requests asked for, left out of those whose
	// bodies couldn't be read.
	Model string `json:"model,omitempty"`
	// Upstream counts the requests that spend quota that went upstream, and
	// Unsent those the router answered itself, never sending them; Checks
	// counts Claude Code's quota checks, and Counts the counts of tokens,
	// however each went.
	Upstream int `json:"upstream"`
	Unsent   int `json:"unsent"`
	Checks   int `json:"checks"`
	Counts   int `json:"counts"`
	// Sessions counts the sessions the requests were of, as counted says.
	Sessions int `json:"sessions"`
	// Usage is the requests' usage, summed field for field as the API gives
	// it, an object's fields within it: what isn't a count, as the service
	// tier, is left out.
	Usage json.RawMessage `json:"usage,omitempty"`
}

// Limit is a limit an account reached, a window's status turning rejected:
// when it was read so, and when the window was to reset, as the reading gave
// it.
type Limit struct {
	Window   string    `json:"window"`
	At       time.Time `json:"at"`
	ResetsAt time.Time `json:"resets_at,omitzero"`
}

// Reading is a window of an account as the readings history holds it, read
// at At, when its reading changed.
type Reading struct {
	At      time.Time
	Account string
	Window  quota.Window
}

// Readings returns the readings the readings history holds of times from
// from up to to, in the order they came.
type Readings func(from, to time.Time) iter.Seq[Reading]

// Summarise returns the summary of the local day with the given date, as the
// ledger's files are named for it, from the day's lines, and the readings
// readings gives of the day and the week before it, the last of each
// window's before the day saying the use it began the day at. It fails for a
// date that isn't one.
func Summarise(date string, lines iter.Seq[Line], readings Readings) (Summary, error) {
	start, end, ok := dayfile.Day(date)
	if !ok {
		return Summary{}, fmt.Errorf("summarise %q: not a date", date)
	}
	t := make(tally)
	for line := range lines {
		t.line(&line)
	}
	t.readings(readings(start.Add(-readingsBefore), end), start)
	return t.summary(date), nil
}

// tally is a day as it's summarised: each account's, by its id, "" for the
// requests no account answered.
type tally map[string]*accountTally

// accountTally is an account's day as it's summarised.
type accountTally struct {
	models                      map[string]*modelTally
	sessions, movedOn, movedOff sessions
	highest                     map[string]float64
	limits                      []Limit
}

// modelTally is a model's day on an account as it's summarised: its requests
// counted, by how each went, in day, the rest as it's gathered.
type modelTally struct {
	day      ModelDay
	sessions sessions
	usage    counts
}

// account returns the tally of the account with the given id, starting it
// where there's none yet.
func (t tally) account(id string) *accountTally {
	if a, ok := t[id]; ok {
		return a
	}
	a := &accountTally{models: make(map[string]*modelTally), sessions: make(sessions), movedOn: make(sessions),
		movedOff: make(sessions), highest: make(map[string]float64)}
	t[id] = a
	return a
}

// model returns the tally of the model with the given name, starting it
// where there's none yet.
func (a *accountTally) model(name string) *modelTally {
	if m, ok := a.models[name]; ok {
		return m
	}
	m := &modelTally{day: ModelDay{Model: name}, sessions: make(sessions), usage: make(counts)}
	a.models[name] = m
	return m
}

// line tallies one of the day's lines: on its account, and its model there,
// and as a move of its session from the account it was on before, where it
// moved it.
func (t tally) line(l *Line) {
	a := t.account(l.Account)
	a.model(l.Model).line(l)
	a.sessions.add(l.counted())
	if l.From != "" {
		a.movedOn.add(l.Session)
		t.account(l.From).movedOff.add(l.Session)
	}
}

// line tallies one of the day's lines of the model's requests on the
// account.
func (m *modelTally) line(l *Line) {
	switch {
	case l.Kind == KindCheck:
		m.day.Checks++
	case l.Kind == KindCount:
		m.day.Counts++
	case l.Attempts > 0:
		m.day.Upstream++
	default:
		m.day.Unsent++
	}
	m.sessions.add(l.counted())
	m.usage.add(l.Usage)
}

// counted is the session the line's request counts toward: none for a quota
// check, which Claude Code sends as it resumes a session under an id it
// never uses again.
func (l *Line) counted() string {
	if l.Kind == KindCheck {
		return ""
	}
	return l.Session
}

// readings tallies each window's highest use on the day that starts at start,
// and the limits reached on it, from readings, those of the day and the week
// before it, in the order they came: the last of a window's before the day
// is the use it began the day at, unless it had reset by then.
func (t tally) readings(readings iter.Seq[Reading], start time.Time) {
	last := make(map[accountWindow]quota.Window)
	var day []Reading
	for r := range readings {
		if r.At.Before(start) {
			last[windowOf(r)] = r.Window
			continue
		}
		day = append(day, r)
	}
	maps.DeleteFunc(last, func(_ accountWindow, w quota.Window) bool { return resetBy(w, start) })
	for w, began := range last {
		t.account(w.account).peak(began)
	}
	for _, r := range day {
		a, w := t.account(r.Account), windowOf(r)
		a.peak(r.Window)
		if turned(last[w], r.Window) {
			a.limits = append(a.limits, Limit{Window: r.Window.Key, At: r.At.UTC(), ResetsAt: r.Window.ResetsAt.UTC()})
		}
		last[w] = r.Window
	}
}

// accountWindow names a window of an account.
type accountWindow struct {
	account, key string
}

// windowOf names the window r is a reading of.
func windowOf(r Reading) accountWindow {
	return accountWindow{account: r.Account, key: r.Window.Key}
}

// resetBy reports whether the window read w had reset by t, as its reading
// said it would.
func resetBy(w quota.Window, t time.Time) bool {
	return !w.ResetsAt.IsZero() && !w.ResetsAt.After(t)
}

// turned reports whether a window read w, read was before, has turned
// rejected: it's rejected, and wasn't, or was read as another window, one
// with another reset, or not read at all.
func turned(was, w quota.Window) bool {
	return w.Status == quota.StatusRejected && (was.Status != quota.StatusRejected || !was.ResetsAt.Equal(w.ResetsAt))
}

// peak takes w's use as its window's highest that day, where it's the
// highest yet.
func (a *accountTally) peak(w quota.Window) {
	if highest, ok := a.highest[w.Key]; !ok || w.Utilization > highest {
		a.highest[w.Key] = w.Utilization
	}
}

// summary returns the day with the given date as its summary.
func (t tally) summary(date string) Summary {
	s := Summary{Version: summaryVersion, Day: date}
	for _, id := range slices.Sorted(maps.Keys(t)) {
		s.Accounts = append(s.Accounts, t[id].summary(id))
	}
	return s
}

// summary returns the account's day, its id the one given.
func (a *accountTally) summary(id string) AccountDay {
	day := AccountDay{Account: id, Sessions: len(a.sessions), MovedOn: len(a.movedOn), MovedOff: len(a.movedOff),
		Highest: a.highest, Limits: a.limits}
	for _, name := range slices.Sorted(maps.Keys(a.models)) {
		day.Models = append(day.Models, a.models[name].summary())
	}
	return day
}

// summary returns the model's day on the account.
func (m *modelTally) summary() ModelDay {
	day := m.day
	day.Sessions = len(m.sessions)
	if len(m.usage) > 0 {
		// A map of counts and maps of them always marshals.
		day.Usage, _ = json.Marshal(m.usage)
	}
	return day
}

// requests counts the requests the summary is of.
func (s Summary) requests() int {
	n := 0
	for _, a := range s.Accounts {
		for _, m := range a.Models {
			n += m.Upstream + m.Unsent + m.Checks + m.Counts
		}
	}
	return n
}

// sessions are sessions, by their ids.
type sessions map[string]bool

// add adds the session with the given id: none for "", which names none.
func (s sessions) add(id string) {
	if id != "" {
		s[id] = true
	}
}

// counts are the counts usage, as the API gives it, holds, summed by their
// names, an object's within it, as the API nests them.
type counts map[string]any

// add adds the counts usage holds to c: each whole number by its name, and
// each object's within it. What else it holds, as the service tier, is left
// out, as is usage that isn't an object.
func (c counts) add(usage json.RawMessage) {
	decoder := json.NewDecoder(bytes.NewReader(usage))
	decoder.UseNumber()
	var fields map[string]any
	if decoder.Decode(&fields) == nil {
		c.addFields(fields)
	}
}

// addFields adds the counts fields hold to c, as add does.
func (c counts) addFields(fields map[string]any) {
	for name, v := range fields {
		switch v := v.(type) {
		case json.Number:
			if n, err := v.Int64(); err == nil {
				sum, _ := c[name].(int64)
				c[name] = sum + n
			}
		case map[string]any:
			within, ok := c[name].(counts)
			if !ok {
				within = make(counts)
				c[name] = within
			}
			within.addFields(v)
		}
	}
}
