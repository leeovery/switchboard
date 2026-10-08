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
	// others never counted, rather than taking it for none.
	summaryVersion = 1
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
	// Accounts are the accounts' days, in the order of their ids: one of no
	// account holds the requests the router answered without one, as when no
	// account had room, or a request's body couldn't be read. A day of no
	// requests has none.
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
	// ReadBefore are the windows the readings history read in the week
	// before the day, by their keys, in their order, as the summary was made:
	// what tells a summary made again, as Summarise says, that a limit it
	// finds no reading before was read before once.
	ReadBefore []string `json:"read_before,omitempty"`
}

// ModelDay is a model's day on an account: its requests, by how each went,
// the sessions they were of, and their usage.
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
	// Sessions counts the sessions the requests were of, as counted says.
	Sessions int `json:"sessions"`
	// Usage is the requests' usage, summed field for field as the API gives
	// it, an object's fields within it, and a list's objects by their types,
	// as an answer's iterations give theirs: what isn't a count, as the
	// service tier, is left out.
	Usage json.RawMessage `json:"usage,omitempty"`
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

// Readings returns the readings the readings history holds of times from
// from up to to, in the order they were read.
type Readings func(from, to time.Time) iter.Seq[readings.Reading]

// readingAhead returns what each makes of every one of the days due, given
// oldest first, with the readings history read ahead for them: once for each
// run of them, as runOf says, so a read never takes in the readings between
// days far apart, and holds a week and a day of them at most, as ahead says.
// Each run's read holds the history's files from pruning until its days are
// made, as readings.Between does as it reads: the router's readings history
// prunes only once it has had the ledger summarise, which waits for any
// summarising under way, as SummariseEnded's lock has it, so it never waits
// on a read.
func readingAhead[T any](history Readings, due []dueDay, each func(dueDay, Readings) T) []T {
	made := make([]T, 0, len(due))
	for len(due) > 0 {
		run := due[:runOf(due)]
		made = append(made, readRun(history, run, each)...)
		due = due[len(run):]
	}
	return made
}

// readRun returns what each makes of every day of run, a run of days due, as
// runOf gives it, with the history read once for them all, from the week
// before the first to the end of the last.
func readRun[T any](history Readings, run []dueDay, each func(dueDay, Readings) T) []T {
	start, _, _ := dayfile.Day(run[0].date)
	_, end, _ := dayfile.Day(run[len(run)-1].date)
	next, stop := iter.Pull(history(start.Add(-readingsBefore), end))
	defer stop()
	read := &ahead{next: next}
	made := make([]T, len(run))
	for i, day := range run {
		made[i] = each(day, read.readings)
	}
	return made
}

// runOf returns how many of the days due, oldest first, from the first on,
// are a run: the week before each starts no later than the day before it
// ends, so a read of the one's readings goes on into the next's.
func runOf(due []dueDay) int {
	n := 1
	for ; n < len(due); n++ {
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
// before them, which no later ask wants. held are those read and kept.
type ahead struct {
	next func() (readings.Reading, bool)
	held []readings.Reading
}

// readings gives the readings of times from from up to to, as ahead says.
func (a *ahead) readings(from, to time.Time) iter.Seq[readings.Reading] {
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

// firstAt returns the index of the first of read, oldest first, read at or
// after t.
func firstAt(read []readings.Reading, t time.Time) int {
	i, _ := slices.BinarySearchFunc(read, t, func(r readings.Reading, t time.Time) int { return r.At.Compare(t) })
	return i
}

// Summarise returns the summary of the local day with the given date, as the
// ledger's files are named for it, made from the lines given, and the
// readings history gives of the day and the week before it, the last of each
// window's before the day saying the use it began the day at. A day of no
// lines is one of no accounts, the history asked for nothing: its readings
// tell of the accounts beside the day's requests alone. A summary to replace
// held, where it's given, the day's summary before, takes a limit whose
// window it finds no reading of before it, in the day or the week before,
// only where held read none of it in the week before either: the readings
// history may since have pruned the one held read, which would have said the
// limit held already. It fails for a date that isn't one.
func Summarise(date string, lines iter.Seq[Line], history Readings, held *Summary) (Summary, error) {
	start, end, ok := dayfile.Day(date)
	if !ok {
		return Summary{}, fmt.Errorf("summarise %q: not a date", date)
	}
	t, n := make(tally), 0
	for line := range lines {
		t.line(&line)
		n++
	}
	if n > 0 {
		t.readings(history(start.Add(-readingsBefore), end), start, held.readBefore)
	}
	return t.summary(date, n), nil
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
// windows read in the week before it.
type accountTally struct {
	models                      map[modelKey]*modelTally
	sessions, movedOn, movedOff sessions
	highest                     map[string]float64
	limits                      []Limit
	readBefore                  map[string]bool
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
		movedOff: make(sessions), highest: make(map[string]float64), readBefore: make(map[string]bool)}
	t[id] = a
	return a
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
		if l.unmetered() {
			m.day.NoUsage++
		}
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
// and the limits reached on it, from the readings history gives, those of the
// day and the week before it, in the order they came: the last of a window's
// before the day is the use it began the day at, unless it had reset by then.
// A limit whose window no reading came before, in the day or the week before,
// is tallied only where readBefore, of the summary it's to replace, reports
// that window wasn't read before either, as Summarise says. The windows read
// in the week before are noted on those of their accounts the day tallies.
func (t tally) readings(history iter.Seq[readings.Reading], start time.Time, readBefore func(accountWindow) bool) {
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
	for w, began := range last {
		t.account(w.account).peak(began)
	}
	for _, r := range day {
		a, w, read := t.account(r.Account), windowOf(r), r.Window()
		a.peak(read)
		if turned(last[w], read) && (seen[w] || !readBefore(w)) {
			a.limits = append(a.limits, Limit{Window: r.Key, At: r.At.UTC(), ResetsAt: r.ResetsAt.UTC()})
		}
		last[w], seen[w] = read, true
	}
	for w := range before {
		if a, ok := t[w.account]; ok {
			a.readBefore[w.key] = true
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

// peak takes w's use as its window's highest that day, where it's the
// highest yet.
func (a *accountTally) peak(w quota.Window) {
	if highest, ok := a.highest[w.Key]; !ok || w.Utilization > highest {
		a.highest[w.Key] = w.Utilization
	}
}

// summary returns the day with the given date as its summary, made from the
// number of lines given.
func (t tally) summary(date string, lines int) Summary {
	s := Summary{Version: summaryVersion, Day: date, Lines: lines}
	for _, id := range slices.Sorted(maps.Keys(t)) {
		s.Accounts = append(s.Accounts, t[id].summary(id))
	}
	return s
}

// summary returns the account's day, its id the one given.
func (a *accountTally) summary(id string) AccountDay {
	day := AccountDay{Account: id, Sessions: len(a.sessions), MovedOn: len(a.movedOn), MovedOff: len(a.movedOff),
		Highest: a.highest, Limits: a.limits, ReadBefore: slices.Sorted(maps.Keys(a.readBefore))}
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
