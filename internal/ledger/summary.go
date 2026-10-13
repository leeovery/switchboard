package ledger

import (
	"bytes"
	"cmp"
	"encoding/json"
	"fmt"
	"iter"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
)

const (
	// summaryVersion is the version of the summaries the ledger writes. A
	// release that summarises a day otherwise, or keeps more of it, writes
	// another, and so tells the summaries written before it: those whose lines
	// are still kept can be summarised again, and a reader knows what the
	// others never counted, rather than taking it for none. Version 2 added
	// each account's session ids, rises, resets and minutes at its cap and at
	// a limit. Version 3 gives a day of no requests the accounts the readings
	// history tells of, as any day's, and climbs each window as Climb does.
	// Version 4 adds each model's session ids.
	summaryVersion = 4
	// readingsBefore is how far before a day the readings that say the use
	// each window began it at are looked for: a week, the longest a window
	// runs, so a window read before then has reset since.
	readingsBefore = 7 * 24 * time.Hour
)

// Summary is a day of the ledger, summarised from its lines and the readings
// history: written once its day has ended, written again while lines come to
// be filed under the day, and kept for good once they're pruned. Fields may
// be added to it, never renamed; Version says which a summary was written
// with.
type Summary struct {
	Version int `json:"version"`
	// Day is the local day's date, as the ledger's files are named for it.
	Day string `json:"day"`
	// Lines counts the day's lines the summary was made from, those that
	// didn't read as a line among them: while they're kept, a day whose files
	// come to hold more of them is summarised again.
	Lines int `json:"lines"`
	// Bytes are the sizes of the day's files as the summary was made from
	// them, or last found to hold no more lines than it was made from: a day
	// whose files are still of them is never read to tell whether it stands.
	// A summary made as it's read, never written, has none.
	Bytes Bytes `json:"bytes,omitzero"`
	// Accounts are the accounts' days, in the order of their ids: those of
	// the accounts the day's requests were of, and of those the readings
	// history tells of, as tally.readings says, requests or not; one of no
	// account holds the requests the router answered without one, as when no
	// account had room, or a request's body couldn't be read. A day of no
	// requests, the history telling of no account, has none.
	Accounts []AccountDay `json:"accounts,omitempty"`
}

// Bytes are how many bytes a day's files hold: its plain file, which lines
// are appended to, and its compressed file, none of one that isn't there.
type Bytes struct {
	Plain      int64 `json:"plain"`
	Compressed int64 `json:"compressed"`
}

// bytesOf is the sizes of a day's files, as stat looks at them.
func bytesOf(stat dayfile.DayStat) Bytes {
	return Bytes{Plain: stat.PlainSize, Compressed: stat.CompressedSize}
}

// AccountDay is an account's day: its requests, by the model each asked for,
// and, of the account as a whole, its sessions, those moved onto it and off
// it, each window's highest use, how far it rose and when it reset, the
// limits it reached, and the minutes it spent at its cap and at a limit. Of
// an account the readings history alone tells of, which served no request,
// it has no models, and no sessions or moves.
//
// Those a summary of version 2 added, SessionIDs, Rise, Resets,
// MinutesAtCap and MinutesAtLimit, are nil where they were never read, as of
// a summary of version 1, or an account a summary made since kept as one of
// those gave it; a summary of version 2 gives none of them as empty, or 0.
// Rise, Resets and the minutes are never read, too, of an account the
// readings history held none of the windows of, in the day or the week
// before it, as when it pruned them before the day was summarised, as its
// keep is its own: what they'd have said is unknown, not none.
type AccountDay struct {
	// Account is the account's id, left out of the requests no account
	// answered.
	Account string `json:"account,omitempty"`
	// Models are the account's requests by the model each asked for, in the
	// order of their names.
	Models []ModelDay `json:"models,omitempty"`
	// Sessions counts the sessions its requests were of, as
	// Line.CountedSession says, and SessionIDs are their ids, in order, each
	// once, so a span of days, or every account, counts a session once.
	// MovedOn and MovedOff count those its requests moved onto it, and off
	// it.
	Sessions   int      `json:"sessions"`
	SessionIDs []string `json:"session_ids,omitzero"`
	MovedOn    int      `json:"moved_on"`
	MovedOff   int      `json:"moved_off"`
	// Highest is each window's highest use that day, by its key: the use it
	// began the day at among it, unless the window had reset by then.
	Highest map[string]float64 `json:"highest,omitempty"`
	// Rise is how far each window's use rose that day, by its key: the points
	// of it the day used, as windowDay says.
	Rise map[string]float64 `json:"rise,omitzero"`
	// Resets are the resets of its windows the readings history saw that day,
	// in the order they came.
	Resets []Reset `json:"resets,omitzero"`
	// Limits are the limits the account reached, in the order it reached them.
	Limits []Limit `json:"limits,omitempty"`
	// MinutesAtCap and MinutesAtLimit are the minutes of the day it spent at
	// its cap, and at a limit, as minutesAt says.
	MinutesAtCap   *int `json:"minutes_at_cap,omitzero"`
	MinutesAtLimit *int `json:"minutes_at_limit,omitzero"`
	// ReadBefore are the windows the readings history read in the week
	// before the day, by their keys, in their order, as the summary was made:
	// what tells a summary made again, as Summarise says, that a limit it
	// finds no reading before was read before once.
	ReadBefore []string `json:"read_before,omitempty"`
}

// Unnamed is how many of the sessions the account's day counts it doesn't
// name by their ids, those it might be of beyond those it names: every one,
// of a day summarised before version 2, which named none; none, of one that
// names them all, or that counts none, as an account the readings history
// alone tells of.
func (a AccountDay) Unnamed() int {
	return max(a.Sessions-len(a.SessionIDs), 0)
}

// Unnamed is how many of the sessions its accounts' days count they don't
// name, as AccountDay.Unnamed says: a day with any might be of any session.
func (s Summary) Unnamed() int {
	n := 0
	for _, a := range s.Accounts {
		n += a.Unnamed()
	}
	return n
}

// ModelDay is a model's day on an account: its requests, by how each went,
// the sessions they were of, and their usage.
//
// SessionIDs, which version 4 added, is nil where the model's sessions were
// never named: in a summary of an older version; of a model a summary made
// since kept as one of those gave it; and where a summary made since, of a
// day one of those summarised, found the ids of fewer than it counted, as
// knowing says. A summary of version 4 gives none as empty.
type ModelDay struct {
	// Model is the model the requests asked for, left out of those whose
	// bodies couldn't be read, and InferenceGeo where they asked for their
	// inference to run, as their shapes give it, left out where they didn't:
	// requests of a model that asked for another geo are a day of their own,
	// as US-only inference costs more.
	Model        string `json:"model,omitempty"`
	InferenceGeo string `json:"inference_geo,omitempty"`
	// Upstream counts the requests that spend quota that went upstream, and
	// NoUsage those of them whose answers gave no usage, as one cut short or
	// an error gives none, so what they spent is unknown; Unsent counts those
	// the router answered itself, never sending them; Checks counts Claude
	// Code's quota checks, and Counts the counts of tokens, however each went.
	Upstream int `json:"upstream"`
	NoUsage  int `json:"no_usage"`
	Unsent   int `json:"unsent"`
	Checks   int `json:"checks"`
	Counts   int `json:"counts"`
	// Sessions counts the sessions the requests were of, as
	// Line.CountedSession says, and SessionIDs are their ids, in order, each
	// once, so a span of days, or every account, counts a model's session
	// once.
	Sessions   int      `json:"sessions"`
	SessionIDs []string `json:"session_ids,omitzero"`
	// Usage is the requests' usage, summed field for field as the API gives
	// it, an object's fields within it, and a list's objects by their types,
	// as an answer's iterations give theirs: what isn't a count, as the
	// service tier, is left out.
	Usage json.RawMessage `json:"usage,omitempty"`
}

// Requests counts the requests, as every view and command counts them: the
// messages, those that went upstream and those the router answered itself.
// The checks and counts, which spend next to nothing, are counted apart,
// never among them.
func (m ModelDay) Requests() int {
	return m.Upstream + m.Unsent
}

// Tokens returns the tokens the requests' usage counts, summed.
func (m ModelDay) Tokens() quota.Tokens {
	tokens, _ := tokensIn(m.Usage)
	return tokens
}

// Limit is a limit an account reached, a window's status turning rejected:
// when it was read so, and when the window was to reset, as the reading gave
// it.
type Limit struct {
	Window   string    `json:"window"`
	At       time.Time `json:"at"`
	ResetsAt time.Time `json:"resets_at,omitzero"`
}

// Reset is a reset of an account's window the readings history saw: its
// window's key; when it reset, its reset time where it was read, or when the
// reading that showed it reset by hand came; and the highest use of the run
// it ended, Before, so a week's peak is the Before of the reset that ended it.
type Reset struct {
	Window string    `json:"window"`
	At     time.Time `json:"at"`
	Before float64   `json:"before"`
}

// Readings returns the readings the readings history holds of times from
// from up to to, in the order they were read.
type Readings func(from, to time.Time) iter.Seq[readings.Reading]

// History is the readings history, as the ledger summarises its days with
// it.
type History interface {
	// Between returns the readings it holds of times from from up to to, in
	// the order they were read, noting in faults, where they're given, the
	// days whose files it couldn't read for now, as dayfile.Faults says, as
	// it reads them.
	Between(from, to time.Time, faults *dayfile.Faults) iter.Seq[readings.Reading]
	// Dates returns the dates of the local days it holds readings of, oldest
	// first, as its files' names give them: none where it holds none. It
	// fails where its files can't be listed, which never tells of none.
	Dates() ([]string, error)
}

// readingAhead returns what each makes of every one of the days due, given
// oldest first, with the readings history read ahead for those of them it
// reaches, as their reach says: once for each run of them, as runOf says, so
// a read never takes in the readings between days far apart, and holds a
// week and a day of them at most, as ahead says. A day it doesn't reach is
// made with none read. Each run's read holds the history's files from
// pruning until its days are made, as readings.Read does as it reads, so
// each takes no lock of the history's files, as a listing of them does: a
// prune waiting on the read would hold the listing back, and the listing the
// read. The router's readings history prunes only once it has had the ledger
// summarise, which waits for any summarising under way, as SummariseEnded's
// lock has it. listed reports whether a listing of the history before the
// read gave a day's date, as one listing, shared by every run, gives them,
// so each run's faults note those whose files have gone since, as
// dayfile.Faults says: nil where there was none.
func readingAhead[T any](history History, due []dueDay, each func(dueDay, *ahead) T, listed func(date string) bool) []T {
	made := make([]T, 0, len(due))
	for len(due) > 0 {
		if !due[0].reach.reached {
			made = append(made, each(due[0], &ahead{}))
			due = due[1:]
			continue
		}
		run := due[:runOf(due)]
		made = append(made, readRun(history, run, each, listed)...)
		due = due[len(run):]
	}
	return made
}

// readRun returns what each makes of every day of run, a run of days due, as
// runOf gives it, with the history read once for them all, from the week
// before the first to the end of the last, its faults noted afresh, as
// listed says.
func readRun[T any](history History, run []dueDay, each func(dueDay, *ahead) T, listed func(date string) bool) []T {
	start, _, _ := dayfile.Day(run[0].date)
	_, end, _ := dayfile.Day(run[len(run)-1].date)
	read := &ahead{faults: dayfile.NewFaults(listed)}
	next, stop := iter.Pull(history.Between(start.Add(-readingsBefore), end, read.faults))
	defer stop()
	read.next = next
	made := make([]T, len(run))
	for i, day := range run {
		made[i] = each(day, read)
	}
	return made
}

// runOf returns how many of the days due, oldest first, from the first on,
// are a run: each reached, and the week before each starts no later than the
// day before it ends, so a read of the one's readings goes on into the
// next's.
func runOf(due []dueDay) int {
	n := 1
	for ; n < len(due) && due[n].reach.reached; n++ {
		_, before, _ := dayfile.Day(due[n-1].date)
		if start, _, _ := dayfile.Day(due[n].date); start.Add(-readingsBefore).After(before) {
			break
		}
	}
	return n
}

// ahead is the readings of a run of days, as next gives them, oldest first,
// read ahead as the days ask for theirs, oldest first: each ask gives those
// of the times asked for, reading on as far as it needs, and lets go of those
// before them, which no later ask wants; none where there's no next, as of a
// day the history doesn't reach. held are those read and kept, and faults the
// days whose files the read couldn't read for now, so far.
type ahead struct {
	next   func() (readings.Reading, bool)
	held   []readings.Reading
	faults *dayfile.Faults
}

// whole reports whether the read read the readings history's files that may
// hold the readings of the local day with the given date and of the week
// before it, as readingDates gives them, as whole as they'll ever read: none
// a fault, as dayfile.Faults says. Those files are read, under the read's own
// hold on them, by the time the day has its readings.
func (a *ahead) whole(date string) bool {
	return !a.faults.Any(readingDates(date))
}

// readingsReach is how many days before a day the readings history's files
// its summary reads begin: those of the week before it, and of the day
// before that.
const readingsReach = 8

// readingDates returns the dates of the readings history's files that may
// hold the readings of the local day with the given date, and of the week
// before it, oldest first: from the day before its week to the day after it,
// as a change of time zone files a reading under a date beside its own. The
// round's days, the history's reach and the read's faults all go by them.
func readingDates(date string) []string {
	start, _, ok := dayfile.Day(date)
	if !ok {
		return nil
	}
	return dayfile.Span(dayfile.DayStart(start, -readingsReach), dayfile.DayStart(start, 1))
}

// readingsHeld are the dates of the local days the readings history holds
// readings of, oldest first, as one listing of its files gives them.
type readingsHeld []string

// holds reports whether the history holds readings of the local day with the
// given date.
func (h readingsHeld) holds(date string) bool {
	_, found := slices.BinarySearch(h, date)
	return found
}

// reach returns how far the history holds the readings a summary of the
// local day with the given date reads, as reach says, by the dates
// readingDates gives their files.
func (h readingsHeld) reach(date string) reach {
	dates := readingDates(date)
	if len(dates) == 0 {
		return reach{}
	}
	i, _ := slices.BinarySearch(h, dates[0])
	reached := i < len(h) && h[i] <= dates[len(dates)-1]
	return reach{reached: reached, covered: reached && h[0] <= dates[0]}
}

// reach is how far the readings history holds the readings a local day's
// summary reads: reached, where it holds any of them; and covered, where it
// holds every one it ever held, its files from the first on, the day before
// the day's week, as the history began before it and has pruned none of
// them since.
type reach struct {
	reached, covered bool
}

// everyReach is the reach of a day where the readings history can't be
// listed to tell: every one, so its readings are read, as a listing that
// failed tells of none it doesn't hold.
var everyReach = reach{reached: true, covered: true}

// reads reports whether a summary of the day reads its readings, none
// reporting whether the day has no lines: one with lines reads them where
// they're reached; one of none, its summary of its readings alone, only
// where they're covered, so a window already at a limit as the history
// began, of which a day at its start holds no reading before, isn't taken
// for a limit reached that day.
func (r reach) reads(none bool) bool {
	return r.covered || r.reached && !none
}

// readings gives the readings of times from from up to to, as ahead says.
func (a *ahead) readings(from, to time.Time) iter.Seq[readings.Reading] {
	if a.next == nil {
		return readNone
	}
	a.held = a.held[firstAt(a.held, from):]
	for len(a.held) == 0 || a.held[len(a.held)-1].At.Before(to) {
		r, ok := a.next()
		if !ok {
			break
		}
		a.held = append(a.held, r)
	}
	return slices.Values(a.held[:firstAt(a.held, to)])
}

// readNone gives no reading.
func readNone(func(readings.Reading) bool) {}

// firstAt returns the index of the first of read, oldest first, read at or
// after t.
func firstAt(read []readings.Reading, t time.Time) int {
	i, _ := slices.BinarySearchFunc(read, t, func(r readings.Reading, t time.Time) int { return r.At.Compare(t) })
	return i
}

// Summarise returns the summary of the local day with the given date, as the
// ledger's files are named for it, made from the lines given, and the
// readings history gives of the day and the week before it, the last of each
// window's before the day saying the use it began the day at: of every
// account the lines or the readings tell of, as tally.readings says, so a day
// of no lines is one of the accounts the readings tell of alone. A summary
// to replace held, where it's given, the day's summary before, takes a limit
// whose window it finds no reading of before it, in the day or the week
// before, only where held read none of it in the week before either: the
// readings history may since have pruned the one held read, which would have
// said the limit held already. The day is summarised as of asOf, as far as
// it has gone where it hasn't ended, as today hasn't: its windows' resets,
// and the minutes its accounts spent at their caps, as caps gives them, and
// at a limit, go no further. It asks history for the readings once it has
// read every line. It fails for a date that isn't one.
func Summarise(date string, lines iter.Seq[Line], history Readings, held *Summary, caps Caps, asOf time.Time) (Summary, error) {
	start, end, ok := dayfile.Day(date)
	if !ok {
		return Summary{}, fmt.Errorf("summarise %q: not a date", date)
	}
	t, n := make(tally), 0
	for line := range lines {
		t.line(&line)
		n++
	}
	read := func() iter.Seq[readings.Reading] { return history(start.Add(-readingsBefore), end) }
	return t.day(date, n, read, held, caps, asOf), nil
}

// day returns the local day with the given date, a date that is one, as its
// summary, made from the n lines t tallies, and the readings read gives of
// the day and the week before it, as Summarise says.
func (t tally) day(date string, n int, read func() iter.Seq[readings.Reading], held *Summary, caps Caps, asOf time.Time) Summary {
	start, until, _ := dayfile.Day(date)
	if asOf.Before(until) {
		until = asOf
	}
	t.readings(read(), start, until, held.readBefore)
	return t.summary(date, n, caps)
}

// readBefore reports whether the summary, where there is one, was made with a
// reading of the window w names in the week before its day.
func (s *Summary) readBefore(w accountWindow) bool {
	if s == nil {
		return false
	}
	for _, a := range s.Accounts {
		if a.Account == w.account {
			return slices.Contains(a.ReadBefore, w.key)
		}
	}
	return false
}

// tally is a day as it's summarised: each account's, by its id, "" for the
// requests no account answered.
type tally map[string]*accountTally

// accountTally is an account's day as it's summarised: readBefore holds the
// windows read in the week before it, and windows how each window read went
// through it, by its key; read is set once the readings history gives a
// reading of any of its windows, in the day or the week before it.
type accountTally struct {
	models                      map[modelKey]*modelTally
	sessions, movedOn, movedOff sessions
	highest                     map[string]float64
	limits                      []Limit
	readBefore                  map[string]bool
	windows                     map[string]*windowDay
	read                        bool
}

// modelKey names a model's requests on an account as a summary keeps them
// apart: by the model they asked for, and the inference geo they asked for,
// as US-only inference prices them otherwise.
type modelKey struct {
	model, geo string
}

// keyOf names the model's requests on an account that l is one of.
func keyOf(l *Line) modelKey {
	return modelKey{model: l.Model, geo: l.Shape.InferenceGeo}
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
	a := &accountTally{models: make(map[modelKey]*modelTally), sessions: make(sessions), movedOn: make(sessions),
		movedOff: make(sessions), highest: make(map[string]float64), readBefore: make(map[string]bool),
		windows: make(map[string]*windowDay)}
	t[id] = a
	return a
}

// clone returns a copy of t, a tally of lines alone, with no readings
// tallied in it, to tally more in without changing t.
func (t tally) clone() tally {
	c := make(tally, len(t))
	for id, a := range t {
		ca := c.account(id)
		ca.sessions, ca.movedOn, ca.movedOff = maps.Clone(a.sessions), maps.Clone(a.movedOn), maps.Clone(a.movedOff)
		for key, m := range a.models {
			ca.models[key] = &modelTally{day: m.day, sessions: maps.Clone(m.sessions), usage: m.usage.clone()}
		}
	}
	return c
}

// model returns the tally of the model's requests key names, starting it
// where there's none yet.
func (a *accountTally) model(key modelKey) *modelTally {
	if m, ok := a.models[key]; ok {
		return m
	}
	m := &modelTally{day: ModelDay{Model: key.model, InferenceGeo: key.geo}, sessions: make(sessions), usage: make(counts)}
	a.models[key] = m
	return m
}

// line tallies one of the day's lines: on its account, and its model there,
// and as a move of its session from the account it was on before, where it
// moved it.
func (t tally) line(l *Line) {
	a := t.account(l.Account)
	a.model(keyOf(l)).line(l)
	a.sessions.add(l.CountedSession())
	if l.From != "" {
		a.movedOn.add(l.Session)
		t.account(l.From).movedOff.add(l.Session)
	}
}

// line tallies one of the day's lines of the model's requests on the
// account: a check and a count as such, sent on or not; one that went
// upstream, as sentOn says, as upstream, and where its answer gave no usage,
// as unmetered says, as that too; and the rest as unsent.
func (m *modelTally) line(l *Line) {
	switch {
	case l.Kind == KindCheck:
		m.day.Checks++
	case l.Kind == KindCount:
		m.day.Counts++
	case l.sentOn():
		m.day.Upstream++
		if l.unmetered() {
			m.day.NoUsage++
		}
	default:
		m.day.Unsent++
	}
	m.sessions.add(l.CountedSession())
	m.usage.add(l.Usage)
}

// readings tallies each window's highest use on the day that starts at start,
// and the limits reached on it, from the readings history gives, those of the
// day and the week before it, in the order they came: the last of a window's
// before the day is the use it began the day at, unless it had reset by then.
// The accounts they tell of are tallied, whether or not they served a
// request: each read in the day, and each a window of which began it at the
// use its last reading before gave, as one whose window reset in the day by
// that reading's reset. A limit whose window no reading came before, in the
// day or the week before, is tallied only where readBefore, of the summary
// it's to replace, reports that window wasn't read before either, as
// Summarise says. The windows read in the week before are noted on those of
// their accounts the day tallies, and how each window went through the day
// until until, as windowDay says: a reading taken out of turn, of a run of
// the window that has reset since, counts toward its highest use and its
// limits, as the true reading of that run it is, but not toward the climb of
// the run since.
func (t tally) readings(history iter.Seq[readings.Reading], start, until time.Time, readBefore func(accountWindow) bool) {
	last := make(map[accountWindow]quota.Window)
	var day []readings.Reading
	for r := range history {
		if r.At.Before(start) {
			last[windowOf(r)] = r.Window()
			continue
		}
		day = append(day, r)
	}
	before := make(map[accountWindow]bool, len(last))
	for w := range last {
		before[w] = true
	}
	seen := maps.Clone(before)
	maps.DeleteFunc(last, func(_ accountWindow, w quota.Window) bool { return w.ResetBy(start) })
	days := windowDays{start: start, until: until, days: make(map[accountWindow]*windowDay)}
	for w := range before {
		if began, ok := last[w]; ok {
			days.of(w).carry(began)
			t.account(w.account).peak(began)
		} else {
			days.of(w).afresh()
		}
	}
	for _, r := range day {
		a, w, read := t.account(r.Account), windowOf(r), r.Window()
		stands := days.of(w).take(read, r.At)
		a.peak(read)
		if reached(last[w], read, stands) && (seen[w] || !readBefore(w)) {
			a.limits = append(a.limits, Limit{Window: r.Key, At: r.At.UTC(), ResetsAt: r.ResetsAt.UTC()})
		}
		if stands {
			last[w] = read
		}
		seen[w], a.read = true, true
	}
	for w := range before {
		if a, ok := t[w.account]; ok {
			a.readBefore[w.key], a.read = true, true
		}
	}
	days.end(t)
}

// windowDays are how each window of the accounts went through the day that
// starts at start, until until, as windowDay says, by the account and key of
// each.
type windowDays struct {
	start, until time.Time
	days         map[accountWindow]*windowDay
}

// of returns the window w's day, starting it where there's none yet.
func (d windowDays) of(w accountWindow) *windowDay {
	day, ok := d.days[w]
	if !ok {
		day = newWindowDay(w.key, d.start, d.until)
		d.days[w] = day
	}
	return day
}

// end ends each window's day that took a reading, on those of their accounts
// the day tallies.
func (d windowDays) end(t tally) {
	for w, day := range d.days {
		if a, ok := t[w.account]; ok && day.read() {
			day.end()
			a.windows[w.key] = day
		}
	}
}

// accountWindow names a window of an account.
type accountWindow struct {
	account, key string
}

// windowOf names the window r is a reading of.
func windowOf(r readings.Reading) accountWindow {
	return accountWindow{account: r.Account, key: r.Key}
}

// turned reports whether a window read w, read was before, has turned
// rejected: it's rejected, and wasn't, or was read as another window, one
// with another reset, or not read at all.
func turned(was, w quota.Window) bool {
	return w.Status == quota.StatusRejected && (was.Status != quota.StatusRejected || !was.ResetsAt.Equal(w.ResetsAt))
}

// reached reports whether w, a reading of a window that stood at was, reached
// a limit: it turned rejected, as turned says, where the window stands at it;
// else, of a run that has reset since, read late, whose reset tells nothing
// against was's, it's rejected where the window as it stood wasn't, so a
// limit the window stood at isn't counted again.
func reached(was, w quota.Window, stands bool) bool {
	if stands {
		return turned(was, w)
	}
	return w.Status == quota.StatusRejected && was.Status != quota.StatusRejected
}

// peak takes w's use as its window's highest that day, where it's the
// highest yet.
func (a *accountTally) peak(w quota.Window) {
	if highest, ok := a.highest[w.Key]; !ok || w.Utilization > highest {
		a.highest[w.Key] = w.Utilization
	}
}

// summary returns the day with the given date as its summary, made from the
// number of lines given, its accounts' caps as caps gives them.
func (t tally) summary(date string, lines int, caps Caps) Summary {
	s := Summary{Version: summaryVersion, Day: date, Lines: lines}
	for _, id := range slices.Sorted(maps.Keys(t)) {
		s.Accounts = append(s.Accounts, t[id].summary(id, caps))
	}
	return s
}

// summary returns the account's day, its id the one given, and its cap as
// caps gives it: its windows' rises, resets and minutes never read where the
// readings history held none of them, as AccountDay says.
func (a *accountTally) summary(id string, caps Caps) AccountDay {
	day := AccountDay{Account: id, Sessions: len(a.sessions), SessionIDs: a.sessions.ids(), MovedOn: len(a.movedOn),
		MovedOff: len(a.movedOff), Highest: a.highest, Limits: a.limits, ReadBefore: slices.Sorted(maps.Keys(a.readBefore))}
	if a.read {
		atCap, atLimit := minutesAt(a.windows, caps.Reserves[id], caps.Shared)
		day.Rise, day.Resets, day.MinutesAtCap, day.MinutesAtLimit = risesOf(a.windows), resetsOf(a.windows), &atCap, &atLimit
	}
	for _, key := range slices.SortedFunc(maps.Keys(a.models), byModel) {
		day.Models = append(day.Models, a.models[key].summary())
	}
	return day
}

// byModel orders the keys of a model's requests by the model's name, then by
// the inference geo they asked for, none first.
func byModel(a, b modelKey) int {
	return cmp.Or(strings.Compare(a.model, b.model), strings.Compare(a.geo, b.geo))
}

// summary returns the model's day on the account.
func (m *modelTally) summary() ModelDay {
	day := m.day
	day.Sessions, day.SessionIDs = len(m.sessions), m.sessions.ids()
	if len(m.usage) > 0 {
		// A map of counts and maps of them always marshals.
		day.Usage, _ = json.Marshal(m.usage)
	}
	return day
}

// linesCounted counts the lines the summary's counts stand for: its requests,
// quota checks and counts of tokens, a line each.
func (s Summary) linesCounted() int {
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

// ids returns the sessions' ids, in order: empty, never nil, where there are
// none, as a summary tells none from never read.
func (s sessions) ids() []string {
	ids := slices.AppendSeq(make([]string, 0, len(s)), maps.Keys(s))
	slices.Sort(ids)
	return ids
}

// counts are the counts usage, as the API gives it, holds, summed by their
// names, an object's within it, as the API nests them, and a list's objects'
// within their types.
type counts map[string]any

// add adds the counts usage holds to c: each whole number by its name, each
// object's within it, and the objects of a list that give their types, as an
// answer's iterations do, within it by their types. What else it holds, as
// the service tier, is left out, as is usage that isn't an object.
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
			c.within(name).addFields(v)
		case []any:
			c.within(name).addTyped(v)
		}
	}
}

// addTyped adds the counts of each object of list that gives its type to c,
// within the type, as addFields adds an object's.
func (c counts) addTyped(list []any) {
	for _, item := range list {
		fields, _ := item.(map[string]any)
		if kind, ok := fields["type"].(string); ok {
			c.within(kind).addFields(fields)
		}
	}
}

// clone returns a copy of c, the counts within it copied too.
func (c counts) clone() counts {
	copied := make(counts, len(c))
	for name, v := range c {
		if within, ok := v.(counts); ok {
			v = within.clone()
		}
		copied[name] = v
	}
	return copied
}

// within returns the counts c holds within name, starting them where there
// are none yet.
func (c counts) within(name string) counts {
	within, ok := c[name].(counts)
	if !ok {
		within = make(counts)
		c[name] = within
	}
	return within
}
