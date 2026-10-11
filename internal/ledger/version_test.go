package ledger_test

import (
	"encoding/json"
	"fmt"
	"iter"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
)

// tick is when the summaries' day's compressed file was last modified, to
// the second, as a file system that keeps whole seconds keeps it, as HFS+
// does.
var tick = on(1, 0, 30)

// compressedLines lays the summaries' day's two requests out in dir, in its
// compressed file, last modified at tick.
func compressedLines(t *testing.T, dir string) {
	t.Helper()
	compressed := filepath.Join(dir, "requests-"+date+".jsonl.gz")
	writeFile(t, dir, filepath.Base(compressed), gzipped(t, jsonOf(t, asked("1", on(0, 9, 0)))+"\n"+jsonOf(t, asked("2", on(0, 10, 0)))+"\n"))
	if err := os.Chtimes(compressed, time.Time{}, tick); err != nil {
		t.Fatal(err)
	}
}

// plainLines lays the summaries' day's two requests out in dir, in its plain
// file.
func plainLines(t *testing.T, dir string) {
	t.Helper()
	holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0)))
}

// heldVersionOne is the summary a release of version 1 made of the
// summaries' day, of its two requests, and its session's highest use.
var heldVersionOne = versionOne(date, 2, 2, `,"highest":{"5h":0.9}`)

// markVersionOne writes heldVersionOne as the ledger in dir holds it, marked
// with the day's files as they stand, and stamped, where the day has a
// compressed file, as that release stamped it: when the file was last
// modified.
func markVersionOne(t *testing.T, dir string) {
	t.Helper()
	sizes := dayBytes(t, dir, date)
	marked := strings.Replace(heldVersionOne, `"lines":2,`, fmt.Sprintf(`"lines":2,"bytes":{"plain":%d,"compressed":%d},`, sizes.Plain, sizes.Compressed), 1)
	writeFile(t, dir, "day-"+date+".json", []byte(marked+"\n"))
	if info, err := os.Stat(filepath.Join(dir, "requests-"+date+".jsonl.gz")); err == nil {
		if err := os.Chtimes(summaryFile(dir, date), time.Time{}, info.ModTime()); err != nil {
			t.Fatal(err)
		}
	}
}

// wholeSeconds has the file at path last modified at the second it was, as
// a file system that keeps whole seconds keeps it.
func wholeSeconds(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, time.Time{}, info.ModTime().Truncate(time.Second)); err != nil {
		t.Fatal(err)
	}
}

func TestADaySummarisedUnderVersionOneIsSummarisedAgainOnceWhileItsLinesAreKept(t *testing.T) {
	tests := []struct {
		name string
		// lay lays the day's lines out in dir.
		lay func(t *testing.T, dir string)
		// unread are the patterns of the names of the files a round that
		// finds the day's summary stands reads none of.
		unread []string
	}{
		{name: "its lines plain", lay: plainLines, unread: []string{"requests-*"}},
		{name: "its lines compressed, on a file system that keeps whole seconds", lay: compressedLines, unread: []string{"requests-*", "day-*"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			dir := t.TempDir()
			tt.lay(t, dir)
			markVersionOne(t, dir)
			l := ledger.Open(dir, 400*24*time.Hour, func() time.Time { return on(3, 10, 0) }, noReadings, caps, logs.For("router"))

			// The readings history holds none of the day's readings any longer,
			// pruned by a keep shorter than the ledger's.
			l.SummariseEnded(on(3, 10, 0))
			if got, want := heldSummary(t, dir, date), summaryOfTwo(windowsRead{highest: `{"5h":0.9}`}); got != want {
				t.Fatalf("the day's summary is\n%s\nwant it summarised again from its lines, as version 3, knowing what the one before did, "+
					"its windows' rises, resets and minutes never read, rather than none\n%s", got, want)
			}
			wholeSeconds(t, summaryFile(dir, date))
			for _, pattern := range tt.unread {
				unopenable(t, dir, pattern)
			}
			l.SummariseEnded(on(3, 11, 0))
			if n, summarised := opened(log), strings.Count(log.String(), `msg="summarised a day of the request ledger"`); n != 0 || summarised != 1 {
				t.Errorf("log reads\n%s\nthe round after opened %d of the day's files, and the day was summarised %d times; want none opened, "+
					"and the day summarised once: its summary is of version 3 now", log, n, summarised)
			}
		})
	}
}

// markVersionTwo writes held, a summary of the summaries' day of version 2,
// as the ledger in dir holds it, marked with the day's files as they stand,
// and stamped, where the day has a compressed file, as that release stamped
// it: a second after the file was last modified.
func markVersionTwo(t *testing.T, dir, held string) {
	t.Helper()
	sizes := dayBytes(t, dir, date)
	marked := strings.Replace(held, `"lines":2,`, fmt.Sprintf(`"lines":2,"bytes":{"plain":%d,"compressed":%d},`, sizes.Plain, sizes.Compressed), 1)
	writeFile(t, dir, "day-"+date+".json", []byte(marked+"\n"))
	if info, err := os.Stat(filepath.Join(dir, "requests-"+date+".jsonl.gz")); err == nil {
		if err := os.Chtimes(summaryFile(dir, date), time.Time{}, info.ModTime().Add(time.Second)); err != nil {
			t.Fatal(err)
		}
	}
}

// ofVersion is summary, as its JSON gives it, of the version given.
func ofVersion(summary string, version int) string {
	return strings.Replace(summary, `{"version":3,`, fmt.Sprintf(`{"version":%d,`, version), 1)
}

func TestADaySummarisedUnderVersionTwoIsSummarisedAgainOnceFromWhatsKept(t *testing.T) {
	// The day's readings: work's session, read as its requests were answered,
	// and personal's week, read days before, reset in the day, personal
	// serving none of its requests.
	kept := []readings.Reading{historyBegan, workRead(on(0, 9, 0), "5h", 0.4, on(0, 13, 0), quota.StatusAllowed), personalRead(on(-3, 12, 0), "7d", 0.82, on(0, 10, 0))}
	personal := quietDayJSON("personal", windowsRead{read: true, highest: `{"7d":0.82}`, rise: `{"7d":0}`, resets: `[` + resetJSON("7d", on(0, 10, 0), 0.82) + `]`,
		before: `["7d"]`})
	work := workDayJSON(2, windowsRead{read: true, highest: `{"5h":0.4}`, rise: `{"5h":0.4}`, resets: `[` + resetJSON("5h", on(0, 13, 0), 0.4) + `]`})
	// whole is the day as version 3 summarises it from its lines and its
	// readings, and blind as version 2 did while the readings history couldn't
	// be read; read as it did with its readings.
	whole := fmt.Sprintf(`{"version":3,"day":"%s","lines":2,"accounts":[%s,%s]}`, date, personal, work)
	blind, read := ofVersion(summaryOf(date, 2, 2, windowsRead{}), 2), ofVersion(whole, 2)
	tests := []struct {
		name string
		held string
		// pruned is set where the day's lines are pruned since its summary,
		// and readings are those the readings history holds now.
		pruned   bool
		readings []readings.Reading
		want     string
	}{
		{name: "its lines and readings kept, summarised again from both", held: blind, readings: kept, want: whole},
		{name: "its lines pruned, its readings kept, summarised again from them, knowing its requests as it did", held: blind, pruned: true, readings: kept,
			want: whole},
		{name: "its lines kept, its readings pruned, summarised again from its lines, knowing what it did of the readings", held: read, want: whole},
		{name: "its lines and readings pruned, kept as it was", held: read, pruned: true, want: read},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			dir := t.TempDir()
			compressedLines(t, dir)
			markVersionTwo(t, dir, tt.held)
			if tt.pruned {
				removeFile(t, dir, "requests-"+date+".jsonl.gz")
			}
			l := ledger.Open(dir, 400*24*time.Hour, func() time.Time { return on(3, 10, 0) }, readingsOf(tt.readings...), caps, logs.For("router"))

			l.SummariseEnded(on(3, 10, 0))
			if got := strings.TrimSuffix(heldSummary(t, dir, date), "\n"); got != tt.want {
				t.Fatalf("the day's summary is\n%s\nwant\n%s", got, tt.want)
			}
			wholeSeconds(t, summaryFile(dir, date))
			unopenable(t, dir, "requests-*")
			unopenable(t, dir, "day-*")
			l.SummariseEnded(on(3, 11, 0))
			if n := opened(log); n != 0 {
				t.Errorf("log reads\n%s\nthe round after opened %d of the day's files, want none: its summary stands, as its stamp tells", log, n)
			}
		})
	}
}

func TestADaySummarisedUnderVersionTwoKeepsWhatItCountedOfALateReading(t *testing.T) {
	week := workRead(on(-3, 12, 0), "7d", 0.41, on(2, 10, 0), quota.StatusAllowed)
	// Work's session, at its limit from 10:02, a reading of the run before
	// it, which reset at 15:30, read late between.
	session := []readings.Reading{rejected(on(0, 10, 2), "5h", 1, on(0, 15, 40)), rejected(on(0, 10, 5), "5h", 1, on(0, 15, 30)),
		rejected(on(0, 10, 8), "5h", 1, on(0, 15, 40))}
	limits := `[` + limitJSON("5h", on(0, 10, 2), on(0, 15, 40)) + `,` + limitJSON("5h", on(0, 10, 5), on(0, 15, 30)) + `,` +
		limitJSON("5h", on(0, 10, 8), on(0, 15, 40)) + `]`
	// countedAgain is the day as version 2 summarised it: the late reading
	// taken for a run of its own, its limit counted again, and again as the
	// run it broke went on, a reset of that run at 10:08, and the rise from
	// nothing after it. Version 3 counts its rise 1, its one reset at 15:40,
	// and its one limit at 10:02, but knows no less than version 2 did.
	countedAgain := windowsRead{read: true, highest: `{"5h":1}`, rise: `{"5h":2}`,
		resets: `[` + resetJSON("5h", on(0, 10, 8), 1) + `,` + resetJSON("5h", on(0, 15, 40), 1) + `]`, limits: limits, atLimit: 338}
	withWeek := countedAgain
	withWeek.highest, withWeek.rise, withWeek.before = `{"5h":1,"7d":0.41}`, `{"5h":2,"7d":0}`, `["7d"]`
	whole := append([]readings.Reading{week}, session...)
	// limitsKept is the day as version 3 counts it, but for the limits version
	// 1 counted, which it keeps.
	limitsKept := windowsRead{read: true, highest: `{"5h":1,"7d":0.41}`, rise: `{"5h":1,"7d":0}`, resets: `[` + resetJSON("5h", on(0, 15, 40), 1) + `]`,
		limits: limits, atLimit: 338, before: `["7d"]`}
	tests := []struct {
		name string
		// held is the day's summary before, and readings those the readings
		// history holds now, damaged where damaged is set.
		held     string
		readings []readings.Reading
		damaged  bool
		want     windowsRead
	}{
		{name: "its readings kept", held: ofVersion(summaryOf(date, 2, 2, withWeek), 2), readings: whole, want: withWeek},
		{name: "the week before's readings pruned", held: ofVersion(summaryOf(date, 2, 2, withWeek), 2), readings: session, want: withWeek},
		{name: "the day's readings pruned, a later day's kept", held: ofVersion(summaryOf(date, 2, 2, countedAgain), 2),
			readings: []readings.Reading{workRead(on(2, 9, 0), "5h", 0.1, on(2, 13, 0), quota.StatusAllowed)}, want: countedAgain},
		{name: "the day's file of readings damaged", held: ofVersion(summaryOf(date, 2, 2, withWeek), 2), readings: whole, damaged: true, want: withWeek},
		{name: "of version 1", held: versionOne(date, 2, 2, `,"highest":{"5h":1},"limits":`+limits), readings: whole, want: limitsKept},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, dir, history := stateDirs(t)
			holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0)))
			markVersionTwo(t, dir, tt.held)
			for _, r := range tt.readings {
				holdReadings(t, history, r.At.Local().Format(time.DateOnly), r)
			}
			if tt.damaged {
				// A member of the day's file, after its readings, cut short.
				compressFile(t, history, "readings-"+date+".jsonl")
				compressed := filepath.Join(history, "readings-"+date+".jsonl.gz")
				data, err := os.ReadFile(compressed)
				if err != nil {
					t.Fatal(err)
				}
				writeFile(t, history, filepath.Base(compressed), append(data, gzipped(t, readingJSON(t, week)+"\n")[:5]...))
			}

			summariseAt(dir, history, on(1, 11, 0))
			if got, want := strings.TrimSuffix(heldSummary(t, dir, date), "\n"), summaryOf(date, 2, 2, tt.want); got != want {
				t.Errorf("the day's summary is\n%s\nwant it of version 3, knowing no less than the one before\n%s", got, want)
			}
		})
	}
}

func TestSessionsASummaryOfVersionOneNeverNamedAreNamedOnlyWhereTheIDsAccountForItsCount(t *testing.T) {
	// counted is version 1's day of work's two requests, of as many sessions
	// as it gives, which it never named.
	counted := func(sessions int) string {
		return strings.Replace(heldVersionOne, `"sessions":1,"moved_on"`, fmt.Sprintf(`"sessions":%d,"moved_on"`, sessions), 1)
	}
	tests := []struct {
		name string
		held string
		// lines lays the day's lines out in dir, where they're kept.
		lines    func(t *testing.T, dir string)
		ids      []string
		sessions int
	}{
		{name: "its lines pruned, its readings alone named none", held: heldVersionOne, lines: func(*testing.T, string) {}, sessions: 1},
		{name: "its lines kept, naming fewer than it counted", held: counted(2), lines: plainLines, sessions: 2},
		{name: "its lines kept, naming as many as it counted", held: counted(1), lines: plainLines, ids: []string{"one"}, sessions: 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			tt.lines(t, dir)
			writeFile(t, dir, "day-"+date+".json", []byte(tt.held+"\n"))

			writeAt(t, dir, on(3, 10, 0), readingsOf(historyBegan, workRead(on(0, 10, 0), "5h", 0.3, on(0, 13, 0), quota.StatusAllowed)))
			data, err := os.ReadFile(summaryFile(dir, date))
			if err != nil {
				t.Fatal(err)
			}
			var got ledger.Summary
			if err := json.Unmarshal(data, &got); err != nil || got.Version != 3 || len(got.Accounts) != 1 {
				t.Fatalf("the day's summary is %s (%v), want work's day, of version 3", data, err)
			}
			if a := got.Accounts[0]; !reflect.DeepEqual(a.SessionIDs, tt.ids) || a.Sessions != tt.sessions {
				t.Errorf("work's sessions are %d, named %#v; want %d, named %#v: never named but where their ids account for their count",
					a.Sessions, a.SessionIDs, tt.sessions, tt.ids)
			}
		})
	}
}

// lockedHistory is the readings history in its files, held by a lock of its
// own as dayfile.Files holds them: to read, as a read of it hands on its
// readings, and as it's listed; and to write as it's pruned, which a read
// starts as it hands on its first reading, as the history's writer prunes
// on a goroutine of its own. A prune waiting on the lock holds back any read
// or listing asked for after it, as a sync.RWMutex does; pruned is closed
// once it's done.
type lockedHistory struct {
	ledger.History
	mu     sync.RWMutex
	once   sync.Once
	pruned chan struct{}
}

func (h *lockedHistory) Between(from, to time.Time, faults *dayfile.Faults) iter.Seq[readings.Reading] {
	return func(yield func(readings.Reading) bool) {
		h.mu.RLock()
		defer h.mu.RUnlock()
		for r := range h.History.Between(from, to, faults) {
			h.once.Do(h.prune)
			if !yield(r) {
				return
			}
		}
	}
}

func (h *lockedHistory) Dates() ([]string, error) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.History.Dates()
}

// prune starts a prune, returning once it waits on the read that holds the
// history.
func (h *lockedHistory) prune() {
	go func() {
		h.mu.Lock()
		defer h.mu.Unlock()
		close(h.pruned)
	}()
	for h.mu.TryRLock() {
		h.mu.RUnlock()
		runtime.Gosched()
	}
}

// waitFor waits for done to be closed, failing with failure where it isn't
// within ten seconds.
func waitFor(t *testing.T, done <-chan struct{}, failure string) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal(failure)
	}
}

func TestARoundFinishesAsAPruneOfTheReadingsWaitsOnItsRead(t *testing.T) {
	_, dir, history := stateDirs(t)
	holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0)))
	markVersionTwo(t, dir, ofVersion(summaryOf(date, 2, 2, windowsRead{}), 2))
	// The day after, its lines not yet summarised, so the read of the two
	// goes on past the first's end as the first is summarised.
	holdLines(t, dir, "2026-10-06", asked("3", on(1, 9, 0)))
	holdReadings(t, history, date, workRead(on(0, 9, 0), "5h", 0.2, on(0, 13, 0), quota.StatusAllowed),
		workRead(on(0, 10, 0), "5h", 0.4, on(0, 13, 0), quota.StatusAllowed))
	holdReadings(t, history, "2026-10-06", workRead(on(1, 9, 0), "5h", 0.1, on(1, 13, 0), quota.StatusAllowed))
	h := &lockedHistory{History: historyIn(history), pruned: make(chan struct{})}
	l := ledger.Open(dir, 400*24*time.Hour, func() time.Time { return on(2, 11, 0) }, h, caps, logs.For("router"))

	summarised := make(chan struct{})
	go func() {
		l.SummariseEnded(on(2, 11, 0))
		close(summarised)
	}()
	waitFor(t, summarised, "the round hung as the prune waited on its read: it took a lock of the readings history as its read held one")
	waitFor(t, h.pruned, "the prune never came: the round never read the readings history")
	if got := heldSummary(t, dir, date); !strings.Contains(got, `"version":3`) {
		t.Errorf("the day's summary is\n%s\nwant it summarised again, as version 3", got)
	}
}

func TestAReaderReadsASummaryOfVersionOneAsItIsOnceItsLinesAreGone(t *testing.T) {
	tests := []struct {
		name string
		// lay lays the day's lines out in dir, where they're kept.
		lay  func(t *testing.T, dir string)
		want string
	}{
		{name: "its lines kept, summarised from them", lay: compressedLines, want: summaryOf(date, 2, 2, windowsRead{highest: `{"5h":0.9}`})},
		{name: "its lines gone, as it is", lay: func(*testing.T, string) {}, want: heldVersionOne},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, dir, _ := stateDirs(t)
			tt.lay(t, dir)
			markVersionOne(t, dir)
			held := heldSummary(t, dir, date)

			days := readerAt(state, on(3, 10, 0)).Days(on(0, 0, 0))
			if got := summariesJSON(t, days[:1]); got[0] != tt.want {
				t.Errorf("the day is\n%s\nwant\n%s", got[0], tt.want)
			}
			if got := heldSummary(t, dir, date); got != held {
				t.Errorf("the ledger holds the day's summary as\n%s\nwant it as it was\n%s", got, held)
			}
		})
	}
}

func TestAReaderTakesASummaryOfADayWithNoLinesAsItStandsWhateverItsVersion(t *testing.T) {
	for _, by := range []string{"a reader", "a follower"} {
		t.Run("by "+by, func(t *testing.T) {
			log := logstest.Capture(t)
			state, dir, history := stateDirs(t)
			// The day's lines are pruned, its summary of version 2, and readings
			// in its reach kept, those of the day before its week, which reach no
			// day after it, open to no one, so a read of them warns that it
			// can't.
			held := ofVersion(summaryOf(date, 2, 2, windowsRead{}), 2)
			writeFile(t, dir, "day-"+date+".json", []byte(held+"\n"))
			holdReadings(t, history, "2026-09-27", workRead(on(-8, 10, 0), "7d", 0.3, on(-2, 10, 0), quota.StatusAllowed))
			unopenable(t, history, "readings-*")
			days := readerAt(state, on(3, 10, 0)).Days
			if by == "a follower" {
				days = followerOf(state, &testClock{at: on(3, 10, 0)}, caps).Days
			}

			if got := summariesJSON(t, days(on(0, 0, 0))[:1])[0]; got != held || opened(log) != 0 {
				t.Errorf("log reads\n%s\nthe day is\n%s\nwant it as it stands, its readings unread, as the router alone summarises it again\n%s", log, got, held)
			}
		})
	}
}

func TestASummaryOfVersionOneWhoseLinesAreGoneHoldsNoneOfWhatItNeverRead(t *testing.T) {
	state, dir, _ := stateDirs(t)
	markVersionOne(t, dir)

	got := readerAt(state, on(3, 10, 0)).Days(on(0, 0, 0))[0]
	if len(got.Accounts) != 1 {
		t.Fatalf("the day is %+v, want work's day in it", got)
	}
	if a := got.Accounts[0]; a.SessionIDs != nil || a.Rise != nil || a.Resets != nil || a.MinutesAtCap != nil || a.MinutesAtLimit != nil {
		t.Errorf("work's day is %+v, want its session ids, rises, resets and minutes never read, not none", a)
	}
	if got.Version != 1 {
		t.Errorf("the day's summary is of version %d, want 1, as it was written", got.Version)
	}
}
