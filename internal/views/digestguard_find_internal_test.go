package views

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
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
// stands, the find never reads its lines; and today's never stands.

// The hard moments the find read over sides meets, each met by some spread
// world, judged from the reads compared: a side read is met by a side read,
// never by the lines a fallback reads.
const (
	foundOnSide        = "the find reading a past day's side, its head standing, never its lines"
	foundDirChanged    = "the find reading the side of a day a session named two directories on"
	foundYesterdayRaw  = "past midnight, the find reading the day before's lines, its head yet to be built"
	foundYesterdaySide = "at 01:30, the find reading the day before's side"
	foundLate          = "a late line filed onto a day after its head was built, the find reading the day's lines"
	foundNotAlone      = "a zone change's line filed beside the day it arrived on, the find reading that day's lines, its head built but not alone"
	foundOtherVersion  = "a day's head of another version of its side, the find reading the day's lines"
	foundOtherZone     = "a day's head built in another time zone, its bounds not the day's, the find reading the day's lines"
)

// findMoments are the hard moments the find read over sides meets.
var findMoments = []string{
	foundOnSide, foundDirChanged, foundYesterdayRaw, foundYesterdaySide, foundLate, foundNotAlone, foundOtherVersion, foundOtherZone,
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
	w.alike(m.now, "the Log's find", foundTyped(findWalked(sides, m.now)), foundTyped(findWalked(noSides{w.raw}, m.now)))
	if both := sidesRaw(sides); len(both) > 0 {
		w.t.Errorf("%s at %v: the find read the lines of %v, whose sides stand", w.name, m.now, both)
	}
	if today := Today(m.now).Date; w.headBuilt(today) {
		w.t.Errorf("%s at %v: a head of today, %s, is built, want none till it has ended", w.name, m.now, today)
	}
	w.noteFound(m, sides)
}

// noteFound notes the hard moments the find walked at the moment m meets,
// as sides noted its reads: a side read, of a day a session named two
// directories on among them, the day before's at half past one; and the
// days whose lines it read, the day before's past midnight, its head yet to
// be built; and those whose heads are built, as they don't stand, by why.
func (w *spread) noteFound(m moment, sides *sidesRead) {
	yesterday := yesterdayOf(Today(m.now))
	for date := range sides.sides {
		w.met[foundOnSide] = true
		w.met[foundDirChanged] = w.met[foundDirChanged] || w.dirChanged(date)
		w.met[foundYesterdaySide] = w.met[foundYesterdaySide] || date == yesterday && m.now.Hour() == 1
	}
	for date := range sides.raw {
		if why := w.unstood(date); why != "" {
			w.met[why] = true
		}
		w.met[foundYesterdayRaw] = w.met[foundYesterdayRaw] || date == yesterday && m.now.Hour() == 0 && !w.headBuilt(date)
	}
}

// unstood is the hard moment of the day with the given date, its head built
// but its lines read, by why its head doesn't stand: a late line filed onto
// it at the moment; its head upset then, as headUpset has it; or a zone
// change's line arriving on it, filed beside it. It's "" where the day's head
// isn't built, or none of those is why.
func (w *spread) unstood(date string) string {
	switch {
	case !w.headBuilt(date):
		return ""
	case slices.Contains(w.lateOn, date):
		return foundLate
	case w.headMade != nil && w.headMade.date == date:
		return w.headMade.moment
	case date == dayfile.DateOf(w.early.At) || date == dayfile.DateOf(w.late.At):
		return foundNotAlone
	}
	return ""
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
// moment.
func (w *spread) fileLate(l ledger.Line) {
	w.lateOn = append(w.lateOn, w.filedUnder(l))
	w.fileLine(l)
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

// upsetHead has the head of the day two before the one now falls on, where
// the ledger holds one, the latest built by then, rewritten as of another
// version of its side, or as built in another time zone, in turn from the
// one the world's seed gives, holding the upset made, as headMade has it.
// The router's round at the next moment builds it again.
func (w *spread) upsetHead(now time.Time) {
	w.t.Helper()
	date := dayfile.DateOf(dayfile.DayStart(now, -2))
	data, err := os.ReadFile(w.headPath(date))
	if errors.Is(err, fs.ErrNotExist) {
		return
	}
	if err != nil {
		w.t.Fatal(err)
	}
	plain := gunzipped(w.t, data)
	start, end, _ := dayfile.Day(date)
	field := func(key string, v int64) []byte { return fmt.Appendf(nil, `"%s":%d,`, key, v) }
	upsets := []struct {
		moment   string
		replaced [][2][]byte
	}{
		{moment: foundOtherVersion, replaced: [][2][]byte{{field("body", RowsVersion), field("body", RowsVersion+1)}}},
		{moment: foundOtherZone, replaced: [][2][]byte{
			{field("from", start.UnixNano()), field("from", start.Add(time.Hour).UnixNano())},
			{field("to", end.UnixNano()), field("to", end.Add(time.Hour).UnixNano())},
		}},
	}
	u := upsets[(w.headUpsets+int(w.seed))%len(upsets)]
	w.headUpsets++
	for _, r := range u.replaced {
		if !bytes.Contains(plain, r[0]) {
			w.t.Fatalf("%s: the head at %s names no %s:\n%s", w.name, w.headPath(date), r[0], plain)
		}
		plain = bytes.Replace(plain, r[0], r[1], 1)
	}
	if err := os.WriteFile(w.headPath(date), gzipped(w.t, plain), 0o600); err != nil {
		w.t.Fatal(err)
	}
	w.headMade = &headUpset{date: date, moment: u.moment}
}
