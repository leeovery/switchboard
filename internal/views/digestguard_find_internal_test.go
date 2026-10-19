package views

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// The find's part of the reads over digests: the Log's find walked back
// through every past day at each moment, as the watch walks it while it's
// open, each day as DaySessionsOn reads it, through the Follower resting on
// digests, which reads a day's side where its head stands, held to the same
// walk through the one reading raw, given no side, which reads every day's
// lines: as it's shown, what it finds of what's typed. Where a day's side
// stands, the find never reads its lines; and today's never stands. Wherever
// the world makes a day's head stop standing, its side is planted with a
// session no line names, as plant has it, so a find that read that side
// would show otherwise than one reading the day raw.

// The hard moments the find read over sides meets, each met by some spread
// world, judged from the reads compared: a side read is met by a side read,
// never by the lines a fallback reads.
const (
	foundOnSide        = "the find reading a past day's side, its head standing, never its lines"
	foundDirChanged    = "the find reading the side of a day a session named two directories on"
	foundManyDays      = "the find reading the sides of several days a session ran on, judging it on its newest"
	foundYesterdayRaw  = "past midnight, the find reading the day before's lines, its head yet to be built"
	foundYesterdaySide = "at 01:30, the find reading the day before's side"
	foundLate          = "a late line filed onto a day after its head was built, the find reading the day's lines alone for it"
	foundNotAlone      = "a zone change's line filed beside the day it arrived on, the find reading that day's lines alone for it, its head built"
	foundOtherVersion  = "a day's head of another version of its side, the find reading the day's lines alone for it"
	foundOtherZone     = "a day's head built in another time zone, its bounds not the day's, the find reading the day's lines alone for it"
)

// findMoments are the hard moments the find read over sides meets. Not
// among them: a session a day's summary names but its lines don't, as where
// they're damaged, which the find meets only reading the day raw, as a
// side, built of the lines, never names it.
var findMoments = []string{
	foundOnSide, foundDirChanged, foundManyDays, foundYesterdayRaw, foundYesterdaySide, foundLate, foundNotAlone, foundOtherVersion, foundOtherZone,
}

// sameFind fails the test where the find walked back at the moment m through
// the Follower resting on digests, reading each day's side where its head
// stands, finds otherwise than walked through the one reading raw, given no
// side, as findWalked walks it, of each of findTyped; where it reads a day's
// lines whose side stands; or where today's side stands. It notes the hard
// moments the walk meets.
func (w *spread) sameFind(m moment) {
	w.t.Helper()
	sides := readingSides(w.based)
	src := findWalked(sides, m.now)
	w.alike(m.now, "the Log's find", foundTyped(src), foundTyped(findWalked(noSides{w.raw}, m.now)))
	if both := sidesRaw(sides); len(both) > 0 {
		w.t.Errorf("%s at %v: the find read the lines of %v, whose sides stand", w.name, m.now, both)
	}
	if today := Today(m.now).Date; w.headBuilt(today) {
		w.t.Errorf("%s at %v: a head of today, %s, is built, want none till it has ended", w.name, m.now, today)
	}
	w.noteFound(m, sides, src.Read)
}

// noteFound notes the hard moments the find walked at the moment m meets,
// as sides noted its reads, read the sessions of each day it read, by date:
// a side read, of a day a session named two directories on among them, the
// day before's at half past one, and several of a session's; and the days
// whose lines it read, the day before's past midnight, its head yet to be
// built; and those whose heads are built, as they don't stand, by why.
func (w *spread) noteFound(m moment, sides *sidesRead, read map[string][]DaySession) {
	yesterday := yesterdayOf(Today(m.now))
	days := make(map[string]int)
	for date := range sides.sides {
		w.met[foundOnSide] = true
		w.met[foundDirChanged] = w.met[foundDirChanged] || w.dirChanged(date)
		w.met[foundYesterdaySide] = w.met[foundYesterdaySide] || date == yesterday && m.now.Hour() == 1
		for _, s := range read[date] {
			days[s.ID]++
		}
	}
	w.met[foundManyDays] = w.met[foundManyDays] || slices.ContainsFunc(slices.Collect(maps.Values(days)), func(n int) bool { return n > 1 })
	for date := range sides.raw {
		if why := w.unstood(date); len(why) == 1 {
			w.met[why[0]] = true
		}
		w.met[foundYesterdayRaw] = w.met[foundYesterdayRaw] || date == yesterday && m.now.Hour() == 0 && !w.headBuilt(date)
	}
}

// unstood are the hard moments of the day with the given date, its head
// built but its lines read, by each reason the world gave its head not to
// stand: a late line filed onto it at the moment; its head upset then, as
// headUpset has it; and a zone change's line arriving on it, filed beside
// it. None where the day's head isn't built. A day read raw for more than
// one meets none of them, as which of them it was read raw for can't be
// told.
func (w *spread) unstood(date string) []string {
	if !w.headBuilt(date) {
		return nil
	}
	var why []string
	if slices.Contains(w.lateOn, date) {
		why = append(why, foundLate)
	}
	if w.headMade != nil && w.headMade.date == date {
		why = append(why, w.headMade.moment)
	}
	if slices.Contains(w.zoneDays(), date) {
		why = append(why, foundNotAlone)
	}
	slices.Sort(why)
	return slices.Compact(why)
}

// zoneDays are the dates of the local days the zone change's lines, early
// and late, arrived on, each filed beside its own.
func (w *spread) zoneDays() []string {
	return []string{dayfile.DateOf(w.early.At), dayfile.DateOf(w.late.At)}
}

// dirChanged reports whether a session's lines that arrived on the local
// day with the given date name two directories or more.
func (w *spread) dirChanged(date string) bool {
	dirs := make(map[string]map[string]bool)
	for _, l := range w.lines {
		if l.Dir == "" || dayfile.DateOf(l.At) != date {
			continue
		}
		if dirs[l.Session] == nil {
			dirs[l.Session] = make(map[string]bool)
		}
		dirs[l.Session][l.Dir] = true
	}
	return slices.ContainsFunc(sessionsIn(w.lines), func(id string) bool { return len(dirs[id]) > 1 })
}

// fileLate files l, a late line, as fileLine files it, noting the date of
// the day it's filed under among those late lines were filed onto at the
// moment, and planting that day's side, as plant has it.
func (w *spread) fileLate(l ledger.Line) {
	date := w.filedUnder(l)
	w.lateOn = append(w.lateOn, date)
	w.fileLine(l)
	w.plant(date)
}

// headPath is where the ledger keeps the head of the local day with the
// given date, as it names one.
func (c *digestChain) headPath(date string) string {
	return filepath.Join(ledger.Dir(c.state), "logday-"+date+".json.gz")
}

// headBuilt reports whether the ledger holds a head of the local day with
// the given date.
func (c *digestChain) headBuilt(date string) bool {
	_, err := os.Stat(c.headPath(date))
	return err == nil
}

// headIn is the JSON of the head the ledger holds of the local day with the
// given date, inflated, reporting false where it holds none.
func (c *digestChain) headIn(date string) ([]byte, bool) {
	c.t.Helper()
	data, err := os.ReadFile(c.headPath(date))
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false
	}
	if err != nil {
		c.t.Fatal(err)
	}
	return gunzipped(c.t, data), true
}

// writeHead writes plain, a head's JSON, as the head of the local day with
// the given date, gzipped, as the ledger writes one.
func (c *digestChain) writeHead(date string, plain []byte) {
	c.t.Helper()
	if err := os.WriteFile(c.headPath(date), gzipped(c.t, plain), 0o600); err != nil {
		c.t.Fatal(err)
	}
}

// plant has the side of the head the ledger holds of the local day with the
// given date hold a session no line of the day names, in a directory none
// does, its grounds left as they are: so a find that read that side, where
// the head doesn't stand, would find it. It plants none where the ledger
// holds no head of the day.
func (w *spread) plant(date string) {
	w.t.Helper()
	plain, ok := w.headIn(date)
	if !ok {
		return
	}
	side := []byte(`"side":[`)
	at := bytes.Index(plain, side)
	if at < 0 {
		w.t.Fatalf("%s: the head at %s holds no side:\n%s", w.name, w.headPath(date), plain)
	}
	at += len(side)
	w.planted++
	session := fmt.Appendf(nil, `{"id":"upset-head-%d","dir":"/upset"}`, w.planted)
	if plain[at] != ']' {
		session = append(session, ',')
	}
	w.writeHead(date, slices.Concat(plain[:at], session, plain[at:]))
}

// headUpset is the head of a day upset at a moment: the day's date, and the
// hard moment its upset makes.
type headUpset struct {
	date, moment string
}

// headsUpsetAt reports whether a day's head is upset at now, of the moments
// the reads are held at: a minute before one in the morning, when nothing
// else is.
func headsUpsetAt(now time.Time) bool {
	return now.Hour() == 0 && now.Minute() >= 30
}

// headUpsets are the upsets of a day's head at a moment, in turn: each
// upsets a day's head, the world letting it, so it doesn't stand, giving
// the date of the day and the moment it makes, or false where the world
// doesn't let it.
var headUpsets = []func(w *spread, now time.Time) (headUpset, bool){
	(*spread).headOtherVersion, (*spread).headOtherZone, (*spread).lateSession, (*spread).zonePlanted,
}

// upsetHead has the next of headUpsets, in turn from the one the world's
// seed gives, upset a day's head at now, holding the upset made, as headMade
// has it, and planting the day's side, as plant has it. The router's round
// at the next moment builds the head again.
func (w *spread) upsetHead(now time.Time) {
	upset := headUpsets[(w.headUpsets+int(w.seed))%len(headUpsets)]
	w.headUpsets++
	if made, ok := upset(w, now); ok {
		w.headMade = &made
		w.plant(made.date)
	}
}

// twoBack is the date of the day two before the one now falls on, the latest
// whose head is built by a minute before one.
func twoBack(now time.Time) string {
	return dayfile.DateOf(dayfile.DayStart(now, -2))
}

// headOtherVersion has the head of the day two before now's hold its side as
// of a version after the views', as a later release writes one.
func (w *spread) headOtherVersion(now time.Time) (headUpset, bool) {
	date := twoBack(now)
	return headUpset{date: date, moment: foundOtherVersion}, w.headOfOtherVersion(date)
}

// headOtherZone has the head of the day two before now's hold its day's
// bounds an hour later, as a head built in another time zone holds them.
func (w *spread) headOtherZone(now time.Time) (headUpset, bool) {
	date := twoBack(now)
	return headUpset{date: date, moment: foundOtherZone}, w.headOfOtherZone(date)
}

// headOfOtherVersion has the head of the local day with the given date hold
// its side and rows as of a version after the views', as a later release
// writes one, reporting false where the ledger holds no head of the day.
func (c *digestChain) headOfOtherVersion(date string) bool {
	field := func(v int) []byte { return fmt.Appendf(nil, `"body":%d,`, v) }
	return c.rewriteHead(date, [2][]byte{field(RowsVersion), field(RowsVersion + 1)})
}

// headOfOtherZone has the head of the local day with the given date hold its
// day's bounds an hour later, as a head built in another time zone holds
// them, reporting false where the ledger holds no head of the day.
func (c *digestChain) headOfOtherZone(date string) bool {
	start, end, _ := dayfile.Day(date)
	field := func(key string, at time.Time) []byte { return fmt.Appendf(nil, `"%s":%d,`, key, at.UnixNano()) }
	return c.rewriteHead(date,
		[2][]byte{field("from", start), field("from", start.Add(time.Hour))},
		[2][]byte{field("to", end), field("to", end.Add(time.Hour))})
}

// rewriteHead rewrites the head of the local day with the given date, each
// of replaced's firsts replaced by its second, reporting false where the
// ledger holds no head of the day.
func (c *digestChain) rewriteHead(date string, replaced ...[2][]byte) bool {
	c.t.Helper()
	plain, ok := c.headIn(date)
	if !ok {
		return false
	}
	for _, r := range replaced {
		if !bytes.Contains(plain, r[0]) {
			c.t.Fatalf("%s: the head at %s names no %s:\n%s", c.name, c.headPath(date), r[0], plain)
		}
		plain = bytes.Replace(plain, r[0], r[1], 1)
	}
	c.writeHead(date, plain)
	return true
}

// lateSession files a late line onto the day two before now's, its head
// built, as fileLate files it: of a session of its own, in a directory of
// its own, a copy of the day's first request otherwise, so a side that
// stood without it would find one session fewer. Its moment, foundLate, is
// noted of the day as fileLate notes it, so it holds no head upset.
func (w *spread) lateSession(now time.Time) (headUpset, bool) {
	date := twoBack(now)
	i := slices.IndexFunc(w.lines, func(l ledger.Line) bool { return w.filedUnder(l) == date && isRequest(&l) })
	if !w.headBuilt(date) || i < 0 {
		return headUpset{}, false
	}
	l := w.lines[i]
	l.Request, l.Session, l.Dir = "upset-late-"+date, "upset-late-"+date, "/upset-late"
	w.fileLate(l)
	return headUpset{}, false
}

// zonePlanted has the head of a day a zone change's line arrived on, filed
// beside it, not stand: the first, early's then late's, whose head is built
// and the digest of the day the line is filed under too, which tells of the
// line, so the day isn't alone.
func (w *spread) zonePlanted(time.Time) (headUpset, bool) {
	for _, l := range []ledger.Line{w.early, w.late} {
		date := dayfile.DateOf(l.At)
		filed, _, _ := dayfile.Day(w.filedUnder(l))
		if w.headBuilt(date) && w.digestsHeld(filed) {
			return headUpset{date: date, moment: foundNotAlone}, true
		}
	}
	return headUpset{}, false
}
