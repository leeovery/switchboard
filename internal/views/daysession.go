package views

import (
	"cmp"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
)

// DaySession is a session of a day, as its lines that day tell it, but
// Claude Code's quota checks: what a day's head keeps of it, for the Log's
// find, Projects and the List to read a day gone without its lines. Its
// whole id; the directory it ran in, as they last named it, "" where none
// did, and its project, as the last to name either named it, which
// status.ProjectOf reads with the directory; how many they are, and when the
// last arrived; when its first request and its last arrived, zero where it
// made none that day; and what its requests spent, by the account each went
// to and the model it asked for, as usedOn has them. What names it, as its
// requests that day tell, and the latest move of each of its models that
// day are the views' own, for a session ended on a day gone to be listed as
// the List lists one, as listedEnded has it.
type DaySession struct {
	ID, Dir, Project          string
	Lines                     int
	Last                      time.Time
	FirstRequest, LastRequest time.Time
	Uses                      []SessionUse
	naming                    naming
	moves                     []dayMove
}

// SessionUse is what a session's requests on a day that went to an account
// and asked for a model spent: how many they were; what they spent, kept as
// counts, to be priced at the prices of the day it's shown on, as Past and
// the List price theirs; and their tokens, as their usage counts them.
type SessionUse struct {
	Account, Model string
	Requests       int
	Spent          ledger.Spent
	Tokens         quota.Tokens
}

// dayMove is the latest move of a session's model on a day, as the story of
// its requests that day tells it: when it came; the model, the account it
// left and the one it went to, and why; the accounts it went out on that
// couldn't serve it; and rewrite, the writes to the cache of the request
// that wrote the session's context again on the account it went to, kept as
// counts, as Spent's AddCacheWrites keeps them, nil where none did, or its
// cache would have run out anyway, as the story's rewrites settles it.
type dayMove struct {
	at                      time.Time
	model, from, to, reason string
	tried                   []ledger.Tried
	rewrite                 *ledger.Spent
}

// DaySessions are the sessions of a day's lines, as Ledger's Day gives them,
// oldest first, each its record: those its summary names, each line counting
// toward the one Line.CountedSession says, the last to arrive first.
func DaySessions(day LogDay) []DaySession {
	return newDaySessions(true).with(day.records).sessions()
}

// DaySessionsOn are the sessions of the past day with the given date, as
// DaySessions gives them of its lines: read from its side, as l's DaySide
// gives it, where its head stands; else of its lines, as l's Day gives them,
// as of a day with no head standing, today's among them, and yesterday's
// until its head is built, or one whose side won't read.
func DaySessionsOn(l Ledger, date string) []DaySession {
	if side, ok := l.DaySide(date, RowsVersion); ok {
		if sessions, err := daySessionsIn(side); err == nil {
			return sessions
		}
	}
	return DaySessions(RecordsOf(l.Day(date)))
}

// daySessions are the sessions of a day's lines taken in so far, by their
// ids, each line its record: the one fold of a day's sessions, whether of
// the lines Day gives, as DaySessions folds them, as the day's side is
// built of them, or of today's as the tail gives them, as SessionDays folds
// them. Where told says, each session's requests are held, for its story to
// tell its moves once the day's lines are all taken in; and where hourly
// says, what each spent by the local hour its requests arrived in, by the
// account each went to, as today's hours are drawn of them, Projects' and
// Accounts' alike; and loose, what the requests of no session spent so,
// apart from the sessions, for Accounts' hours to count every request, as
// the day's summary does, nil where hourly doesn't say. Where the fold
// takes in more reads than one, as today's does, keys are the keys of the
// lines it took, as turnKeyOf has them, each counted, so a read given again
// counts none of them twice; nil of a fold of one read.
type daySessions struct {
	held   map[string]*foldedSession
	loose  accountHours
	keys   map[turnKey]int
	told   bool
	hourly bool
}

// foldedSession is a session of a day as daySessions folds it: its record
// so far; its requests, where its story is to be told of them; and what
// they spent by the local hour each arrived in, by account, where that's
// kept.
type foldedSession struct {
	DaySession
	requests []heldRequest
	hours    accountHours
}

// accountHours are what requests spent by the local hour each arrived in,
// by the account each went to, "" of those none answered: 24 sums an
// account, as few as a session's day ran on.
type accountHours map[string]*[24]ledger.Spent

// add adds what r, a request, spent to its account's hour it arrived in.
func (h accountHours) add(r *lineRecord) {
	hours := h[r.account]
	if hours == nil {
		hours = new([24]ledger.Spent)
		h[r.account] = hours
	}
	hours[r.arrived().Local().Hour()].Add(r.model, r.geo, r.spend)
}

// copied is a copy of the hours sharing nothing a fold changes with them.
func (h accountHours) copied() accountHours {
	c := make(accountHours, len(h))
	for account, hours := range h {
		kept := *hours
		c[account] = &kept
	}
	return c
}

// newDaySessions is a fold of a day's sessions of no line yet, holding each
// one's requests for its story where told says.
func newDaySessions(told bool) daySessions {
	return daySessions{held: make(map[string]*foldedSession), told: told}
}

// take takes in r, the record of the day's next line in Day's order, of the
// read read tells, toward the session it counts toward, as its
// countedSession has it, where it counts toward one, and it isn't given
// again, as read tells it by its key, as turnKeyOf has it, against those d
// keeps, so a line the ledger's files hold twice counts as the List and the
// Log count it: when it arrived in UTC, as the ledger writes it, and a day's
// side restores it, whatever zone a line edited by hand names; and, of a
// session's request, as isRequest has it, what it spent, and what names the
// session. A request of no session, where d keeps loose hours, it takes in
// there alone, given again or not as a session's is.
func (d daySessions) take(r *lineRecord, read *countedOnce) {
	id := r.countedSession
	if id == "" && (d.loose == nil || r.kind != RowRequest) {
		return
	}
	k := turnKeyOf(r)
	if read.again(k, d.keys[k]) {
		return
	}
	if d.keys != nil {
		d.keys[k]++
	}
	if id == "" {
		d.loose.add(r)
		return
	}
	s, ok := d.held[id]
	if !ok {
		s = &foldedSession{ID: id}
		if d.hourly {
			s.hours = accountHours{}
		}
		d.held[id] = s
	}
	s.Lines++
	s.Last = score.Later(s.Last, r.arrived())
	s.Dir, s.Project = placeAfter(s.Dir, s.Project, r.dir, r.project)
	if !r.isRequest() {
		return
	}
	s.requested(r)
	if s.hours != nil {
		s.hours.add(r)
	}
	if d.told {
		s.requests = append(s.requests, newHeldRequest(r.decodedRequest))
	}
}

// requested takes in r, a request of the session: when it arrived, what
// names the session, and what it spent, never writing where a copy of s's
// record may read.
func (s *DaySession) requested(r *lineRecord) {
	at := r.arrived()
	if s.FirstRequest.IsZero() || at.Before(s.FirstRequest) {
		s.FirstRequest = at
	}
	s.LastRequest = score.Later(s.LastRequest, at)
	s.naming.see(r.model, r.account, r.class, at)
	s.Uses = usedOn(s.Uses, r)
}

// usedOn is uses, as a session's record holds them, in the order of their
// accounts, then models, with r's request among them, its tokens those its
// usage counts: afresh, so a copy of the record holding uses never sees it.
func usedOn(uses []SessionUse, r *lineRecord) []SessionUse {
	i, found := slices.BinarySearchFunc(uses, r, func(u SessionUse, r *lineRecord) int {
		return cmp.Or(strings.Compare(u.Account, r.account), strings.Compare(u.Model, r.model))
	})
	uses = slices.Clone(uses)
	if !found {
		uses = slices.Insert(uses, i, SessionUse{Account: r.account, Model: r.model})
	}
	u := &uses[i]
	u.Requests++
	u.Spent.Add(r.model, r.geo, r.spend)
	if tokens, ok := r.spend.Tokens(); ok {
		u.Tokens = quota.Tokens{
			Input: u.Tokens.Input + tokens.Input, Output: u.Tokens.Output + tokens.Output,
			CacheRead: u.Tokens.CacheRead + tokens.CacheRead, CacheWrite: u.Tokens.CacheWrite + tokens.CacheWrite,
		}
	}
	return uses
}

// with is d, the records of a read's lines given taken in too, in their
// order, as take takes each: afresh, each session they change copied, and
// its loose hours and keys, so d, and any copy of it, never sees them.
func (d daySessions) with(records []lineRecord) daySessions {
	if len(records) == 0 {
		return d
	}
	if d.keys != nil {
		d.keys = maps.Clone(d.keys)
	}
	if d.loose != nil {
		d.loose = d.loose.copied()
	}
	held := maps.Clone(d.held)
	changed := make(map[string]bool)
	for i := range records {
		id := records[i].countedSession
		if s, ok := held[id]; ok && !changed[id] {
			held[id], changed[id] = s.copied(), true
		}
	}
	d.held = held
	read := countedOnceOf(len(records))
	for i := range records {
		d.take(&records[i], read)
	}
	return d
}

// copied is a copy of s for a fold to take more in, sharing nothing it
// changes with s: its hours its own. Its uses are taken in afresh, as
// usedOn has them, and its requests only appended to.
func (s *foldedSession) copied() *foldedSession {
	c := *s
	c.requests = slices.Clip(s.requests)
	if s.hours != nil {
		c.hours = s.hours.copied()
	}
	return &c
}

// sessions are the sessions taken in, the last to arrive first, as
// DaySessions orders them, each with its moves, where its story's told.
func (d daySessions) sessions() []DaySession {
	sessions := make([]DaySession, 0, len(d.held))
	for _, s := range d.held {
		day := s.DaySession
		if d.told {
			day.moves = latestMoves(s.requests)
		}
		sessions = append(sessions, day)
	}
	slices.SortFunc(sessions, func(a, b DaySession) int { return cmp.Or(b.Last.Compare(a.Last), strings.Compare(a.ID, b.ID)) })
	return sessions
}

// latestMoves are the latest move of each model the requests given moved, a
// session's of a day, in the order those moves came, as its story of them
// tells them, unpriced: none where none moved it.
func latestMoves(requests []heldRequest) []dayMove {
	if !slices.ContainsFunc(requests, func(r heldRequest) bool { return r.from.Value() != "" }) {
		return nil
	}
	requests = slices.Clone(requests)
	slices.SortStableFunc(requests, inOrder)
	s := storyOf(requests, ledger.Table{}, LedgerDay{})
	var moves []dayMove
	for i := range s.moves {
		if m := &s.moves[i]; s.latestMove(m.model) == m {
			moves = append(moves, dayMoveOf(m))
		}
	}
	return moves
}

// dayMoveOf is the move m as a day's record of its session holds it.
func dayMoveOf(m *move) dayMove {
	d := dayMove{at: m.at, model: m.model, from: m.from, to: m.to, reason: m.reason, tried: m.tried}
	if r := m.rewrote; r != nil {
		var writes ledger.Spent
		writes.AddCacheWrites(m.model, r.geo.Value(), r.spend)
		d.rewrite = &writes
	}
	return d
}

// storedDaySession is a DaySession as a day's side stores it, the side a
// list of them in DaySessions' order: each time as nanoseconds since the
// Unix epoch, restored in UTC, as the ledger writes a line's time; what
// names it, as a stored story holds its sightings; and its moves, as
// storedDayMove has them; zeros left out. A day's head holds its bounds so
// too, and stands only where they're the day's, so no time a side holds is
// beyond the years nanoseconds since the epoch hold.
type storedDaySession struct {
	ID           string          `json:"id"`
	Dir          string          `json:"dir,omitzero"`
	Project      string          `json:"project,omitzero"`
	Lines        int             `json:"lines,omitzero"`
	Last         int64           `json:"last,omitzero"`
	FirstRequest int64           `json:"first_request,omitzero"`
	LastRequest  int64           `json:"last_request,omitzero"`
	Uses         []storedUse     `json:"uses,omitempty"`
	Main         storedSighting  `json:"main,omitzero"`
	Latest       storedSighting  `json:"latest,omitzero"`
	Moves        []storedDayMove `json:"moves,omitempty"`
}

// storedUse is a SessionUse as a day's side stores it, its spent in its own
// stored form; zeros left out.
type storedUse struct {
	Account  string       `json:"account,omitzero"`
	Model    string       `json:"model,omitzero"`
	Requests int          `json:"requests,omitzero"`
	Spent    ledger.Spent `json:"spent"`
	Tokens   storedTokens `json:"tokens,omitzero"`
}

// storedTokens are tokens as a day's side stores them, by short keys, zeros
// left out.
type storedTokens struct {
	Input      int `json:"i,omitzero"`
	Output     int `json:"o,omitzero"`
	CacheRead  int `json:"r,omitzero"`
	CacheWrite int `json:"w,omitzero"`
}

// storedDayMove is a dayMove as a day's side stores it: when it came as
// nanoseconds since the Unix epoch, as storedDaySession's times are; the
// accounts it went out on, as a stored request holds them; and the rewrite's
// writes in a spent's stored form; zeros left out.
type storedDayMove struct {
	At      int64         `json:"at"`
	Model   string        `json:"model,omitzero"`
	From    string        `json:"from,omitzero"`
	To      string        `json:"to,omitzero"`
	Reason  string        `json:"reason,omitzero"`
	Tried   []storedTried `json:"tried,omitempty"`
	Rewrite *ledger.Spent `json:"rewrite,omitempty"`
}

// stored is s as a day's side stores it.
func (s DaySession) stored() storedDaySession {
	stored := storedDaySession{
		ID: s.ID, Dir: s.Dir, Project: s.Project, Lines: s.Lines, Last: sideNanos(s.Last),
		FirstRequest: sideNanos(s.FirstRequest), LastRequest: sideNanos(s.LastRequest),
		Main: s.naming.main.stored(), Latest: s.naming.latest.stored(),
	}
	for _, u := range s.Uses {
		t := u.Tokens
		stored.Uses = append(stored.Uses, storedUse{
			Account: u.Account, Model: u.Model, Requests: u.Requests, Spent: u.Spent,
			Tokens: storedTokens{Input: t.Input, Output: t.Output, CacheRead: t.CacheRead, CacheWrite: t.CacheWrite},
		})
	}
	for _, m := range s.moves {
		stored.Moves = append(stored.Moves, storedDayMove{
			At: sideNanos(m.at), Model: m.model, From: m.from, To: m.to, Reason: m.reason, Tried: storedTriedOf(m.tried), Rewrite: m.rewrite,
		})
	}
	return stored
}

// daySession is the DaySession s stores.
func (s storedDaySession) daySession() DaySession {
	d := DaySession{
		ID: s.ID, Dir: s.Dir, Project: s.Project, Lines: s.Lines, Last: sideTime(s.Last),
		FirstRequest: sideTime(s.FirstRequest), LastRequest: sideTime(s.LastRequest),
		naming: naming{main: s.Main.sighting(), latest: s.Latest.sighting()},
	}
	for _, u := range s.Uses {
		t := u.Tokens
		d.Uses = append(d.Uses, SessionUse{
			Account: u.Account, Model: u.Model, Requests: u.Requests, Spent: u.Spent,
			Tokens: quota.Tokens{Input: t.Input, Output: t.Output, CacheRead: t.CacheRead, CacheWrite: t.CacheWrite},
		})
	}
	for _, m := range s.Moves {
		d.moves = append(d.moves, dayMove{at: sideTime(m.At), model: m.Model, from: m.From, to: m.To, reason: m.Reason, tried: triedOf(m.Tried), rewrite: m.Rewrite})
	}
	return d
}

// sideNanos is t as nanoseconds since the Unix epoch, as a day's side stores a
// time: 0 of the zero time.
func sideNanos(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixNano()
}

// sideTime is the time a day's side stores as nanos, as sideNanos has it,
// in UTC: the zero time of 0.
func sideTime(nanos int64) time.Time {
	if nanos == 0 {
		return time.Time{}
	}
	return time.Unix(0, nanos).UTC()
}

// daySideOf is the side of a day of the given sessions, as DaySessions gives
// them, as the day's head holds it. Its error is the ledger's to wrap, with
// the day's date.
func daySideOf(sessions []DaySession) (json.RawMessage, error) {
	stored := make([]storedDaySession, len(sessions))
	for i, s := range sessions {
		stored[i] = s.stored()
	}
	return json.Marshal(stored)
}

// daySessionsIn are the sessions side holds, as daySideOf gave it, failing
// where it doesn't read as a day's side.
func daySessionsIn(side json.RawMessage) ([]DaySession, error) {
	var stored []storedDaySession
	if err := json.Unmarshal(side, &stored); err != nil {
		return nil, fmt.Errorf("a day's side: %w", err)
	}
	sessions := make([]DaySession, len(stored))
	for i, s := range stored {
		sessions[i] = s.daySession()
	}
	return sessions, nil
}
