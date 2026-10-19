package views

import (
	"bytes"
	"cmp"
	"fmt"
	"iter"
	"maps"
	"math/rand/v2"
	"os"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// turnWorld are the lines a turn is read from rows over, of the seed given: a
// page's run, as pagedRun makes it, each of its session's lines dressed as a
// router's says, one in eight without an id, and of one seed in three, the
// first of those given twice, as the ledger's files may hold one; one in
// five followed a second on by a quota check of the session, which counts
// toward none, and a count of its tokens, as checkAndCount has them; among
// the lines of a crowd of other sessions, as crowdOf has them. In the order
// the ledger files them.
func turnWorld(seed uint64) []ledger.Line {
	lines, _ := pagedRun(seed)
	r := rand.New(rand.NewPCG(seed, 15))
	twice := 0
	var asides []ledger.Line
	for i := range lines {
		if l := &lines[i]; l.Session == "s" {
			dressed(r, l)
			if r.IntN(8) == 0 {
				l.Request = ""
				twice = cmp.Or(twice, i+1)
			}
			if r.IntN(5) == 0 {
				asides = append(asides, checkAndCount(*l)...)
			}
		}
	}
	if seed%3 == 0 && twice > 0 {
		lines = append(lines, lines[twice-1])
	}
	lines = append(slices.Concat(lines, asides), crowdOf(lines)...)
	slices.SortStableFunc(lines, func(a, b ledger.Line) int { return ended(&a).Compare(ended(&b)) })
	return lines
}

// checkAndCount are a quota check of the session of l's request, and a count
// of its tokens, each its own, a second after l arrived: the check counts
// toward no session, the count toward its own.
func checkAndCount(l ledger.Line) []ledger.Line {
	asides := make([]ledger.Line, 2)
	for i, kind := range []string{ledger.KindCheck, ledger.KindCount} {
		asides[i] = ledger.Line{
			At: l.At.Add(time.Second), Request: fmt.Sprintf("%s-%s-%d", kind, l.Request, l.At.UnixMilli()), Kind: kind, Session: l.Session,
			Model: l.Model, Account: l.Account, Reason: l.Reason, Status: 200, Attempts: 1, TotalMS: 300, Class: l.Class, Prompt: l.Prompt,
		}
	}
	return asides
}

// crowdOf are the lines of a crowd of other sessions on each day from the
// one the first of lines arrived on to the one the last arrived on: a
// request every two minutes from the day's start, each of its own, of three
// sessions in turn. So a day holds rows of three chunks, and a chunk of them
// may hold none of lines'.
func crowdOf(lines []ledger.Line) []ledger.Line {
	arrival := func(a, b ledger.Line) int { return a.At.Compare(b.At) }
	var crowd []ledger.Line
	for _, date := range dayfile.Span(slices.MinFunc(lines, arrival).At, slices.MaxFunc(lines, arrival).At) {
		start, end, _ := dayfile.Day(date)
		for k, at := 0, start; at.Before(end); k, at = k+1, at.Add(2*time.Minute) {
			crowd = append(crowd, ledger.Line{
				At: at.UTC(), Request: fmt.Sprintf("c-%s-%d", date, k), Kind: ledger.KindMessage, Session: fmt.Sprintf("crowd%d", k%3), Model: foldHaiku,
				Account: "side", Status: 200, TotalMS: 1000,
			})
		}
	}
	return crowd
}

// turnLater are the lines a turn world's session's later looks find, as
// laterLines has them, the i'th at the moment given, after its run ended at
// last, the last of them with a turn pending: each filed but the request in
// flight for sixty days, which the stream tells of as it runs.
func turnLater(last time.Time, moments []time.Time) (filed, flying []ledger.Line) {
	for i, now := range moments {
		for _, l := range laterLines(i, now, last.Add(time.Minute), i == len(moments)-1) {
			if ended(&l).After(moments[len(moments)-1]) {
				flying = append(flying, l)
				continue
			}
			filed = append(filed, l)
		}
	}
	return filed, flying
}

// toldInFlight is a stream telling of each of the session's requests lines
// give that's in flight at now, sent moments after it arrived, as the
// router's stream tells of one: but the one it never tells of, and none it
// counts among no requests.
func toldInFlight(lines []ledger.Line, now time.Time) Stream {
	s := Stream{Live: true, Requests: make(map[string]Told)}
	for _, l := range lines {
		if l.Session != "s" || l.Request == "" || l.Request == unseen || !l.Counted() || l.At.After(now) || !ended(&l).After(now) {
			continue
		}
		s.Requests[l.Request] = Told{
			Request: l.Request, Session: l.Session, Model: l.Model, Account: l.Account, Prompt: l.Prompt, Class: l.Class, AgentID: l.AgentID,
			Sent: l.At.Add(100 * time.Millisecond), Heard: now,
		}
	}
	return s
}

// rawOnly reads through l as a Follower of a ledger holding no day's rows
// does: every day's lines raw, of the spans asked for.
type rawOnly struct {
	noRows
	l LedgerSession
}

func (r rawOnly) Session(id string, within ...ledger.Span) iter.Seq[ledger.Held] {
	return r.l.Session(id, within...)
}

// turnReads is a ledger as a turn reads it through the one it holds, noting
// the dates of the days DayRows was asked of, each with whether their rows
// stood; the chunks RowChunk read, by the date of their day, each by its
// member's offset; and the spans each read of a session's lines was given.
type turnReads struct {
	LedgerSession
	asked  map[string]bool
	chunks map[string][]int64
	spans  [][]ledger.Span
}

// readingTurn is l read as turnReads notes it, nothing read yet.
func readingTurn(l LedgerSession) *turnReads {
	return &turnReads{LedgerSession: l, asked: make(map[string]bool), chunks: make(map[string][]int64)}
}

func (r *turnReads) DayRows(date string, body int) (ledger.DayRows, bool) {
	rows, ok := r.LedgerSession.DayRows(date, body)
	r.asked[date] = ok
	return rows, ok
}

func (r *turnReads) RowChunk(date string, chunk ledger.RowsChunk) ([]byte, error) {
	r.chunks[date] = append(r.chunks[date], chunk.Offset)
	return r.LedgerSession.RowChunk(date, chunk)
}

func (r *turnReads) Session(id string, within ...ledger.Span) iter.Seq[ledger.Held] {
	r.spans = append(r.spans, within)
	return r.LedgerSession.Session(id, within...)
}

// rawMeets reports whether a read of a session's lines r noted was given a
// span meeting the local day with the given date, or none, as of every day.
func (r *turnReads) rawMeets(date string) bool {
	start, end, _ := dayfile.Day(date)
	return slices.ContainsFunc(r.spans, func(spans []ledger.Span) bool {
		return len(spans) == 0 || slices.ContainsFunc(spans, func(s ledger.Span) bool { return !s.From.After(end.Add(-time.Nanosecond)) && !s.To.Before(start) })
	})
}

// openedTurn is a turn of a page opened through a ledger, as the watch opens
// one: the read ReadTurn gave; and its requests, as TurnRequests gives them.
type openedTurn struct {
	read     HeldTurn
	requests []LogLine
}

// openTurn is the turn numbered n of page opened through l, as live builds
// the page, as openedTurn has it.
func openTurn(page HeldPage, n int, l LedgerSession, live PageLive) openedTurn {
	open := page.Opening(n, live)
	o := openedTurn{read: open.ReadTurn(l, live)}
	o.requests = open.Turned(o.read).TurnRequests(n, live)
	return o
}

// shown is what o shows: each of its requests' row and its details.
func (o openedTurn) shown() []any {
	var shown []any
	for _, l := range o.requests {
		d, _ := l.Details()
		shown = append(shown, l.Row, d)
	}
	return shown
}

// within are the records of o's read that arrived within any of spans, in
// the order read.
func (o openedTurn) within(spans []ledger.Span) []lineRecord {
	var records []lineRecord
	for _, r := range o.read.lines {
		at := r.arrived()
		if slices.ContainsFunc(spans, func(s ledger.Span) bool { return !at.Before(s.From) && !at.After(s.To) }) {
			records = append(records, r)
		}
	}
	return records
}

// turnChecked is a turn of the session s opened at a moment, read through the
// chain's Follower and raw, as sameTurns reads it: its number; whether it's
// pending; its span and the horizon's, as turnSpans has them; the reads the
// Follower noted; what each read gave and showed; and the records its read
// gives of the days it reads from their rows, as turnDaysOf restores them.
type turnChecked struct {
	n             int
	pending       bool
	turn, horizon ledger.Span
	reads         *turnReads
	rows, raw     openedTurn
	restored      []lineRecord
}

// sameTurns opens every turn of the session s at now, its page read seven
// minutes before through the chain's Follower, as ReadPage reads it, of the
// stream s tells, the ledger as the chain's last pass left it; then each
// turn read as the watch reads it: through the Follower, as read reads it
// where it's given, the days of its own whose rows stand read from them; and
// raw, every day's lines read. It fails the test where a turn shows
// otherwise read either way, or its read gives otherwise of the lines that
// arrived within its span and the horizon's, each once, in the order read
// raw; and gives each turn checked.
func (c *digestChain) sameTurns(now time.Time, s Stream, read func(LedgerSession) LedgerSession) []turnChecked {
	c.t.Helper()
	f := c.follower()
	var l LedgerSession = f
	if read != nil {
		l = read(f)
	}
	c.at = now.Add(-7 * time.Minute)
	page := ReadPage(f, "s", s, changingPrices(), 0, c.at)
	c.at = now
	live := PageLive{Stream: s, Now: now}
	w := page.worked(live)
	var checked []turnChecked
	for i, turn := range w.tally.turns {
		spans := w.turnSpans(turn)
		tc := turnChecked{n: i + 1, pending: turn.pending, turn: spans[0], horizon: spans[1], reads: readingTurn(l)}
		tc.rows, tc.raw = openTurn(page, tc.n, tc.reads, live), openTurn(page, tc.n, rawOnly{l: f}, live)
		tc.restored = turnDaysOf(l, "s", tc.turn, tc.horizon).restored
		if got, want := tc.rows.shown(), tc.raw.shown(); !reflect.DeepEqual(got, want) {
			c.t.Errorf("%s at %v: turn %d read from its days' rows shows otherwise than read raw\n%s", c.name, now, tc.n, unlike(got, want))
		}
		if got, want := tc.rows.within(spans), tc.raw.within(spans); !reflect.DeepEqual(got, want) {
			c.t.Errorf("%s at %v: turn %d read from its days' rows gives of its span and the horizon's %d lines\n%+v\nwant, as read raw, %d\n%+v",
				c.name, now, tc.n, len(got), got, len(want), want)
		}
		checked = append(checked, tc)
	}
	return checked
}

// fromRows are the dates of the days of tc's turn read from their rows:
// those its read found standing, of its own days before the horizon's.
func (tc turnChecked) fromRows() []string {
	var dates []string
	for date, stood := range tc.reads.asked {
		if stood {
			dates = append(dates, date)
		}
	}
	slices.Sort(dates)
	return dates
}

// chunkOf is a chunk of a day's rows as the test reads it: its member's
// offset, and the records of its rows.
type chunkOf struct {
	offset  int64
	records []lineRecord
}

// dayChunks are the chunks of the rows f gives of each day, by its date, as
// of reads them, each day's read once.
type dayChunks struct {
	t    *testing.T
	f    *ledger.Follower
	read map[string][]chunkOf
}

// chunksOf are the chunks of the rows f gives, none read yet.
func chunksOf(t *testing.T, f *ledger.Follower) dayChunks {
	return dayChunks{t: t, f: f, read: make(map[string][]chunkOf)}
}

// of are the chunks of the rows of the local day with the given date, as
// rowsRead reads them, in order, failing the test where they don't stand.
func (d dayChunks) of(date string) []chunkOf {
	d.t.Helper()
	if chunks, ok := d.read[date]; ok {
		return chunks
	}
	rows, ok := d.f.DayRows(date, RowsVersion)
	_, metas, read := rowsRead(d.t, d.f, date)
	if !ok || !read {
		d.t.Fatalf("the rows of %s don't stand", date)
	}
	chunks := make([]chunkOf, len(metas))
	for i, m := range metas {
		chunks[i] = chunkOf{offset: rows.Chunks[i].Offset, records: m.records}
	}
	d.read[date] = chunks
	return chunks
}

// names reports whether c holds a row its session's id counts toward.
func (c chunkOf) names(id string) bool {
	return slices.ContainsFunc(c.records, func(r lineRecord) bool { return r.countedSession == id })
}

// meets reports whether c's rows arrived from no later than span's end to no
// earlier than its start.
func (c chunkOf) meets(span ledger.Span) bool {
	arrival := func(a, b lineRecord) int { return a.at.compare(b.at) }
	first, last := slices.MinFunc(c.records, arrival).at, slices.MaxFunc(c.records, arrival).at
	return first.compare(instantOf(span.To)) <= 0 && last.compare(instantOf(span.From)) >= 0
}

// heldToItsRows fails the test where tc's turn, opened as sameTurns opens
// it, read the rows of a day otherwise than the rule says: rows asked of a
// day of its own, before the horizon's day, alone, every one asked; of each
// that stood, the chunks read as heldToItsChunks has them, and the day never
// read raw, each of its days read either way; and of those days, its read
// giving the records of the session's lines of the chunks read alone, its
// checks and counts among them, newest first, as Session gives a session's
// lines. It notes in met the hard moments it meets.
func (c *digestChain) heldToItsRows(tc turnChecked, chunks dayChunks, met map[string]bool) {
	c.t.Helper()
	before := dayfile.DateOf(tc.horizon.From)
	var own []string
	for _, date := range dayfile.Span(tc.turn.From, tc.turn.To) {
		if date < before {
			own = append(own, date)
		}
	}
	asked := slices.Sorted(maps.Keys(tc.reads.asked))
	if !slices.Equal(asked, own) {
		c.t.Errorf("%s at %v: turn %d asked the rows of %v, want of its own days before the horizon's, %v", c.name, c.at, tc.n, asked, own)
	}
	var restored []lineRecord
	for _, date := range slices.Backward(tc.fromRows()) {
		restored = append(restored, c.heldToItsChunks(tc, date, chunks, met)...)
		if tc.reads.rawMeets(date) {
			c.t.Errorf("%s at %v: turn %d read %s raw, its rows read, given %v", c.name, c.at, tc.n, date, tc.reads.spans)
		}
	}
	if !reflect.DeepEqual(tc.restored, restored) {
		c.t.Errorf("%s at %v: turn %d gives of the days it reads from their rows %d records\n%+v\nwant the session's of the chunks read, newest first, %d\n%+v",
			c.name, c.at, tc.n, len(tc.restored), tc.restored, len(restored), restored)
	}
	for _, date := range own {
		if !tc.reads.asked[date] && !tc.reads.rawMeets(date) {
			c.t.Errorf("%s at %v: turn %d read %s neither from its rows nor raw, given %v", c.name, c.at, tc.n, date, tc.reads.spans)
		}
	}
	noteTurn(tc, own, met)
}

// heldToItsChunks fails the test where tc's turn, opened as sameTurns opens
// it, read other chunks of the rows of the day with the given date, its rows
// standing, than those whose rows name the session and meet the turn's span,
// as chunks has them; and gives the records of the session's lines of those
// chunks, newest first, those of its checks and counts among them. It notes
// in met the hard moments it meets.
func (c *digestChain) heldToItsChunks(tc turnChecked, date string, chunks dayChunks, met map[string]bool) []lineRecord {
	c.t.Helper()
	var want []int64
	var session []lineRecord
	for _, chunk := range chunks.of(date) {
		switch {
		case chunk.names("s") && chunk.meets(tc.turn):
			want = append(want, chunk.offset)
			session = append(session, slices.DeleteFunc(slices.Clone(chunk.records), func(r lineRecord) bool { return r.session != "s" })...)
		case chunk.names("s"):
			met["a chunk of the session's passed over, of none of the turn's span"] = true
		default:
			met["a chunk naming none of the session's passed over"] = true
		}
	}
	if got := tc.reads.chunks[date]; !slices.Equal(got, want) {
		c.t.Errorf("%s at %v: turn %d read the chunks of %s at %v, want those naming its session and meeting its span, %v", c.name, c.at, tc.n, date, got, want)
	}
	if slices.ContainsFunc(session, func(r lineRecord) bool { return r.kind == RowCheck }) {
		met["a check of the session's, which counts toward none, restored of a chunk read"] = true
	}
	slices.Reverse(session)
	return session
}

// noteTurn notes in met the hard moments tc's turn meets, own the dates of
// its own days before the horizon's.
func noteTurn(tc turnChecked, own []string, met map[string]bool) {
	today, horizonDate := dayfile.DateOf(tc.horizon.To), dayfile.DateOf(tc.horizon.From)
	if slices.ContainsFunc(dayfile.Span(tc.turn.From, tc.turn.To), func(date string) bool { return horizonDate <= date && date < today }) {
		met["a day of a turn's own before today's read raw, the horizon pinned back over it"] = true
	}
	rows := tc.fromRows()
	if len(rows) == 0 {
		return
	}
	if len(rows) < len(own) {
		met["a day of a turn's own read raw beside one read from its rows"] = true
	}
	if tc.pending {
		met["a turn pending, a day of its own read from its rows"] = true
	}
	shown, agents := make(map[string]bool), make(map[string]map[string]bool)
	var idless []*lineRecord
	for _, l := range tc.rows.requests {
		r := l.record
		if r == nil {
			continue
		}
		date := dayfile.DateOf(r.arrived())
		if !slices.Contains(rows, date) {
			if !r.arrived().Before(tc.horizon.From) && rows[0] <= dayfile.DateOf(tc.horizon.From.AddDate(0, 0, -7)) {
				met["a turn a week back read from its rows, with a line since the horizon"] = true
			}
			continue
		}
		shown[date] = true
		if r.agentID != "" {
			if agents[r.agentID] == nil {
				agents[r.agentID] = make(map[string]bool)
			}
			agents[r.agentID][date] = true
		}
		if r.request == "" {
			met["a line without an id read from its rows"] = true
			if slices.ContainsFunc(idless, func(o *lineRecord) bool { return reflect.DeepEqual(o, r) }) {
				met["a line given twice read from its rows"] = true
			}
			idless = append(idless, r)
		}
	}
	if len(shown) > 1 {
		met["a turn across midnight, each day read from its rows"] = true
	}
	for _, days := range agents {
		if len(days) > 1 {
			met["a subagent's requests on two days, each read from its rows"] = true
		}
	}
}

// sessionSpan is when the first of the session s's requests lines give
// arrived, and when the last ended.
func sessionSpan(lines []ledger.Line) (first, last time.Time) {
	for _, l := range lines {
		if l.Session != "s" {
			continue
		}
		if first.IsZero() || l.At.Before(first) {
			first = l.At
		}
		if end := ended(&l); end.After(last) {
			last = end
		}
	}
	return first, last
}

// turnSeeds is how many seeds of the turn world turns are read over.
const turnSeeds = 3

// Every turn of a generated session's page, its days crowded, opened at a
// minute and at fifty-nine minutes past local midnight, the day before's
// head yet to be built, and at half past one, on each day from the one after
// its run began to the one after it ended, and three hours, a week and a
// month after, as laterLines has its requests then, shows what it shows read
// raw, read with the days of its own before the horizon's whose rows stand
// read from them: every row and its details, as sameTurns has it. Of the
// lines that arrived within its span and the horizon's, the read gives each
// once, in the order read raw; and it reads the rows of its own days before
// the horizon's alone, of each the chunks naming its session and meeting
// its span alone, giving of them the session's lines alone, and never a day
// raw whose rows it read, as heldToItsRows has it. Among the turns: one
// across midnight, a subagent's requests on two days, a turn pending, one a
// week back with a line since the horizon, and lines without an id, one
// given twice, each read from rows; a day of a turn's own read raw, as
// yesterday's is till its head is built, beside one read from rows; one the
// horizon is pinned back over; chunks passed over, of the session's and of
// none of its; and a check of the session's given of a chunk read, so the
// session its line is of is told apart from the one it counts toward.
func TestATurnShowsFromItsDaysRowsWhatItShowsReadRaw(t *testing.T) {
	met := make(map[string]bool)
	for seed := range uint64(turnSeeds) {
		world := turnWorld(seed)
		first, last := sessionSpan(world)
		laters := []time.Time{last.Add(3 * time.Hour), last.Add(7 * 24 * time.Hour), last.Add(30 * 24 * time.Hour)}
		filed, flying := turnLater(last, laters)
		c := newDigestChain(t, fmt.Sprintf("turn world %d", seed), seed, slices.Concat(world, filed))
		every := slices.Concat(c.lines, flying)
		moments := slices.Clone(laters)
		for day := dayfile.DayStart(first.Local(), 1); !day.After(dayfile.DayStart(last.Local(), 1)); day = dayfile.DayStart(day, 1) {
			for _, past := range []time.Duration{time.Minute, 59 * time.Minute, 90 * time.Minute} {
				moments = append(moments, day.Add(past))
			}
		}
		slices.SortFunc(moments, time.Time.Compare)
		for _, now := range moments {
			c.by(now.Add(-7 * time.Minute))
			chunks := chunksOf(t, c.follower())
			for _, tc := range c.sameTurns(now, toldInFlight(every, now), nil) {
				c.heldToItsRows(tc, chunks, met)
			}
		}
	}
	for _, moment := range []string{
		"a turn across midnight, each day read from its rows", "a subagent's requests on two days, each read from its rows",
		"a turn pending, a day of its own read from its rows", "a turn a week back read from its rows, with a line since the horizon",
		"a line without an id read from its rows", "a line given twice read from its rows",
		"a day of a turn's own read raw beside one read from its rows", "a day of a turn's own before today's read raw, the horizon pinned back over it",
		"a chunk of the session's passed over, of none of the turn's span", "a chunk naming none of the session's passed over",
		"a check of the session's, which counts toward none, restored of a chunk read",
	} {
		if !met[moment] {
			t.Errorf("no turn world meets %s", moment)
		}
	}
}

// upsetSeed is the seed of the turn world a day of whose turn is upset.
const upsetSeed = 1

// A day of a turn's own whose rows don't stand, or a chunk of which it needs
// won't restore, or whose meta won't read, is read raw, never from its rows,
// and the turn shows what it shows read raw, as sameTurns has it: a chunk it
// needs damaged, its rows file as long; a chunk it needs restoring a row
// fewer than its head counts; its head holding a meta of a chunk it needs
// that isn't one, its JSON reading still; its rows file gone, its head
// standing; its head of another version; and a late line filed onto it
// after its head was built. Each is upset a week after the world's run, the
// router's next pass yet to build it again.
func TestATurnReadsADayRawWhoseRowsItNeedsDontStand(t *testing.T) {
	tests := []struct {
		name  string
		upset func(c *digestChain, date string, chunk ledger.RowsChunk)
		short bool
		stood bool
	}{
		{name: "a chunk it needs damaged, its rows file as long", upset: (*digestChain).damageChunk, stood: true},
		{name: "a chunk it needs restoring a row fewer than its head counts", short: true, stood: true},
		{name: "its head holding a meta of a chunk it needs that isn't one", upset: (*digestChain).spoilMeta, stood: true},
		{name: "its rows file gone, its head standing", upset: func(c *digestChain, date string, _ ledger.RowsChunk) { c.removeRows(date) }},
		{name: "its head of another version", upset: func(c *digestChain, date string, _ ledger.RowsChunk) { c.headOfOtherVersion(date) }},
		{name: "a late line filed onto it after its head was built", upset: func(c *digestChain, date string, _ ledger.RowsChunk) { c.fileLateOnto(date) }},
	}
	world := turnWorld(upsetSeed)
	_, last := sessionSpan(world)
	now := last.Add(7 * 24 * time.Hour)
	built := newDigestChain(t, "an upset turn world", upsetSeed, world)
	built.by(now.Add(-7 * time.Minute))
	n, date, chunk := built.turnReadingRows(now)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := built.cloned(t)
			if tt.upset != nil {
				tt.upset(c, date, chunk)
			}
			var read func(LedgerSession) LedgerSession
			if tt.short {
				read = func(l LedgerSession) LedgerSession { return rowShort{LedgerSession: l, offset: chunk.Offset} }
			}
			tc := c.sameTurns(now, Stream{}, read)[n-1]
			if tc.reads.asked[date] != tt.stood || !tc.reads.rawMeets(date) {
				t.Errorf("turn %d, upset, found the rows of %s standing %t, reading it raw %t, given %v; want them standing %t, the day read raw",
					n, date, tc.reads.asked[date], tc.reads.rawMeets(date), tc.reads.spans, tt.stood)
			}
		})
	}
}

// turnReadingRows is a turn of the session s, opened at now as sameTurns
// opens it, that reads a chunk of the rows of a day of its own: its number,
// the day's date, and the first chunk of it read, failing the test where
// none does.
func (c *digestChain) turnReadingRows(now time.Time) (n int, date string, chunk ledger.RowsChunk) {
	c.t.Helper()
	f := c.follower()
	for _, tc := range c.sameTurns(now, Stream{}, nil) {
		for _, date := range tc.fromRows() {
			if read := tc.reads.chunks[date]; len(read) > 0 {
				rows, _ := f.DayRows(date, RowsVersion)
				i := slices.IndexFunc(rows.Chunks, func(c ledger.RowsChunk) bool { return c.Offset == read[0] })
				return tc.n, date, rows.Chunks[i]
			}
		}
	}
	c.t.Fatalf("%s: no turn opened at %v reads a chunk of a day's rows", c.name, now)
	return 0, "", ledger.RowsChunk{}
}

// damageChunk flips a byte in the middle of chunk, a chunk of the rows of the
// local day with the given date, the rows file as long as it was.
func (c *digestChain) damageChunk(date string, chunk ledger.RowsChunk) {
	c.t.Helper()
	data, err := os.ReadFile(c.rowsPath(date))
	if err != nil {
		c.t.Fatal(err)
	}
	data[chunk.Offset+chunk.Length/2] ^= 0xff
	if err := os.WriteFile(c.rowsPath(date), data, 0o600); err != nil {
		c.t.Fatal(err)
	}
}

// spoilMeta has the head of the local day with the given date hold the meta
// of chunk, a chunk of its rows as DayRows gives it, as a word, which is
// JSON but no chunk's meta, the rest of the head as it was, failing the test
// where the head holds no such meta.
func (c *digestChain) spoilMeta(date string, chunk ledger.RowsChunk) {
	c.t.Helper()
	if !c.rewriteHead(date, [2][]byte{chunk.Meta, []byte(`"spoiled"`)}) {
		c.t.Fatalf("%s: the ledger holds no head of %s", c.name, date)
	}
}

// A chunk's meta meets a turn of a session where it counts a row toward the
// session, and its rows arrived from no later than the turn's span's end to
// no earlier than its start, both held: a chunk whose last row arrived as
// the span begins meets it, and whose first arrived as it ends, but not a
// nanosecond outside either; nor one holding only a quota check of the
// session, which counts toward none, or another session's rows alone,
// within the span. Its meta is read as the chunk's builder stores it.
func TestAChunksMetaMeetsATurnsSpanItsEndsHeld(t *testing.T) {
	from := time.Date(2026, 10, 7, 12, 0, 0, 0, time.UTC)
	span := ledger.Span{From: from, To: from.Add(time.Hour)}
	line := func(session, kind string, at time.Time) ledger.Line {
		return ledger.Line{At: at, Request: at.Format(time.RFC3339Nano), Kind: kind, Session: session, Model: foldOpus, Account: "work", Status: 200}
	}
	request := func(at time.Time) ledger.Line { return line("s", ledger.KindMessage, at) }
	tests := []struct {
		name  string
		lines []ledger.Line
		meets bool
	}{
		{"its last row arrived as the span begins", []ledger.Line{request(from.Add(-time.Hour)), request(from)}, true},
		{"its last row arrived a nanosecond before the span begins", []ledger.Line{request(from.Add(-time.Hour)), request(from.Add(-time.Nanosecond))}, false},
		{"its first row arrived as the span ends", []ledger.Line{request(span.To), request(span.To.Add(time.Hour))}, true},
		{"its first row arrived a nanosecond after the span ends", []ledger.Line{request(span.To.Add(time.Nanosecond)), request(span.To.Add(time.Hour))}, false},
		{"its rows within the span", []ledger.Line{request(from.Add(time.Minute))}, true},
		{"its one row within the span a quota check of the session", []ledger.Line{line("s", ledger.KindCheck, from.Add(time.Minute))}, false},
		{"its rows within the span another session's", []ledger.Line{line("other", ledger.KindMessage, from.Add(time.Minute))}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var c rowsChunk
			for _, rec := range recordsOfLines(tt.lines) {
				if err := c.take(&rec); err != nil {
					t.Fatal(err)
				}
			}
			built, err := c.built()
			if err != nil {
				t.Fatal(err)
			}
			meta, err := chunkMetaOf(built.meta)
			if err != nil {
				t.Fatal(err)
			}
			if got := meta.meets("s", span); got != tt.meets {
				t.Errorf("the chunk meets the session s's span from %v to %v %t, want %t", span.From, span.To, got, tt.meets)
			}
		})
	}
}

// rowShort is a ledger whose chunk at offset reads a row fewer, its last,
// than its head counts.
type rowShort struct {
	LedgerSession
	offset int64
}

func (r rowShort) RowChunk(date string, chunk ledger.RowsChunk) ([]byte, error) {
	rows, err := r.LedgerSession.RowChunk(date, chunk)
	if err != nil || chunk.Offset != r.offset {
		return rows, err
	}
	return rows[:bytes.LastIndexByte(rows[:len(rows)-1], '\n')+1], nil
}
