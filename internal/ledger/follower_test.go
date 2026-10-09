package ledger_test

import (
	"encoding/json"
	"errors"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"slices"
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

// testClock is a clock the test sets.
type testClock struct {
	at time.Time
}

func (c *testClock) now() time.Time {
	return c.at
}

// followerOf returns a follower of the ledger in state, by clock, its days
// summarised with caps.
func followerOf(state string, clock *testClock, c ledger.Caps) *ledger.Follower {
	return ledger.NewFollower(state, clock.now, c, logs.For("cli"))
}

// page is what one who reads today's lines with a Follower holds of them:
// those read, and the mark they're read to.
type page struct {
	lines []ledger.Held
	mark  ledger.Mark
}

// read reads today's lines f gives since the page last read them, letting go
// of those it held where they're given afresh, and returns them.
func (p *page) read(f *ledger.Follower) (added []ledger.Held, afresh bool) {
	added, p.mark, afresh = f.Today(p.mark)
	if afresh {
		p.lines = nil
	}
	p.lines = append(p.lines, added...)
	return added, afresh
}

// inArrival returns the JSON of the lines the page holds, oldest first: the
// tests' lines each arrived at a time of its own.
func (p *page) inArrival() []string {
	return heldJSON(slices.SortedStableFunc(slices.Values(p.lines), func(a, b ledger.Held) int { return a.At.Compare(b.At) }))
}

// fullJSON returns each of summaries as JSON, the sizes of its day's files
// it's marked with among it.
func fullJSON(t *testing.T, summaries []ledger.Summary) []string {
	t.Helper()
	var all []string
	for _, s := range summaries {
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, string(data))
	}
	return all
}

// startOfDay returns the start of the local day at falls on.
func startOfDay(at time.Time) time.Time {
	return dayfile.DayStart(at.Local(), 0)
}

// wholeSession returns the JSON of the lines of the session with the given
// id the ledger in state holds, newest first, as a Reader reads them whole
// at at.
func wholeSession(state, id string, at time.Time) []string {
	var lines []ledger.Held
	for h := range readerAt(state, at).Lines(time.Time{}) {
		if h.Session == id {
			lines = append(lines, h)
		}
	}
	slices.Reverse(lines)
	return heldJSON(lines)
}

// sameAsWhole fails t, naming the step, where what f gives at at, and the
// page holds, isn't what a Reader gives, reading the ledger in state whole at
// at: the days' summaries from from, today's lines, but that of the request
// writing names, still being written, which Today gives once it's whole, and
// the lines of session one, newest first.
func sameAsWhole(t *testing.T, step string, f *ledger.Follower, p *page, state string, at, from time.Time, writing string) {
	t.Helper()
	whole := readerAt(state, at)
	if got, want := fullJSON(t, f.Days(from)), fullJSON(t, whole.Days(from)); !slices.Equal(got, want) {
		t.Errorf("%s: Days() =\n%s\nwant\n%s, as a whole read gives", step, strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	var lines []ledger.Held
	for h := range whole.Lines(startOfDay(at)) {
		if h.Request != writing {
			lines = append(lines, h)
		}
	}
	today := heldJSON(lines)
	if got := p.inArrival(); !slices.Equal(got, today) {
		t.Errorf("%s: the page holds\n%s\nwant today's lines\n%s", step, strings.Join(got, "\n"), strings.Join(today, "\n"))
	}
	if got, _, _ := f.Today(ledger.Mark{}); !slices.Equal(heldJSON(got), today) {
		t.Errorf("%s: Today() from the zero Mark =\n%s\nwant\n%s, in the order a whole read gives", step, strings.Join(heldJSON(got), "\n"), strings.Join(today, "\n"))
	}
	if got, want := heldJSON(slices.Collect(f.Session("one"))), wholeSession(state, "one", at); !slices.Equal(got, want) {
		t.Errorf("%s: Session() =\n%s\nwant\n%s", step, strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// askedBy is a request of the session with the given id answered on work at
// at, as the ledger holds it.
func askedBy(request, session string, at time.Time) ledger.Line {
	line := asked(request, at)
	line.Session = session
	return line
}

// appendTo appends text to the file with the given name in dir.
func appendTo(t testing.TB, dir, name, text string) {
	t.Helper()
	out, err := os.OpenFile(filepath.Join(dir, name), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := out.WriteString(text); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

// holdReadings adds read, as the readings history writes them, to its plain
// file in dir of the local day with the given date.
func holdReadings(t testing.TB, dir, date string, read ...readings.Reading) {
	t.Helper()
	for _, r := range read {
		appendTo(t, dir, "readings-"+date+".jsonl", readingJSON(t, r)+"\n")
	}
}

// compressFile compresses the plain file with the given name in dir, as a
// prune does: its lines added to its compressed file, a gzip member of their
// own, and the plain file gone.
func compressFile(t *testing.T, dir, name string) {
	t.Helper()
	plain, compressed := filepath.Join(dir, name), filepath.Join(dir, name+".gz")
	lines, err := os.ReadFile(plain)
	if err != nil {
		t.Fatal(err)
	}
	held, err := os.ReadFile(compressed)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	if err := os.WriteFile(compressed, append(held, gzipped(t, string(lines))...), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(plain); err != nil {
		t.Fatal(err)
	}
}

// removeFile removes the file with the given name in dir, as a prune does.
func removeFile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.Remove(filepath.Join(dir, name)); err != nil {
		t.Fatal(err)
	}
}

// historyIn is the readings history in dir, as the router reads it.
func historyIn(dir string) ledger.Readings {
	files := readings.Files(dir, logs.For("router"))
	return func(from, to time.Time) iter.Seq[readings.Reading] { return readings.Between(files, from, to) }
}

// summariseAt has the router's ledger in dir summarise the days that have
// ended at at, with the readings history in historyDir, as its round does.
func summariseAt(dir, historyDir string, at time.Time) {
	ledger.Open(dir, 400*24*time.Hour, func() time.Time { return at }, historyIn(historyDir), caps, logs.For("router")).SummariseEnded(at)
}

// personalRead is personal's window with the given key read at at, at u of
// its use, resetting at resets, allowed.
func personalRead(at time.Time, key string, u float64, resets time.Time) readings.Reading {
	r := workRead(at, key, u, resets, quota.StatusAllowed)
	r.Account = "personal"
	return r
}

func TestAFollowerGivesWhatAWholeReadGivesAsTheLedgerGrows(t *testing.T) {
	state, dir, history := stateDirs(t)
	// The week before, its readings compressed as they're kept, the oldest
	// day's from before it, and the two days before today's start.
	for back := 8; back >= 3; back-- {
		day := on(-back, 12, 0).Format(time.DateOnly)
		holdReadings(t, history, day, workRead(on(-back, 12, 0), "7d", 0.1*float64(9-back), on(4, 9, 0), quota.StatusAllowed),
			personalRead(on(-back, 13, 0), "7d", 0.05*float64(9-back), on(5, 9, 0)))
		compressFile(t, history, "readings-"+day+".jsonl")
	}
	holdReadings(t, history, "2026-10-03", workRead(on(-2, 20, 0), "5h", 0.3, on(-1, 1, 0), quota.StatusAllowed))
	holdReadings(t, history, "2026-10-04", workRead(on(-1, 20, 0), "5h", 0.5, on(0, 1, 0), quota.StatusAllowed),
		workRead(on(-1, 21, 0), "7d", 0.7, on(4, 9, 0), quota.StatusAllowed))
	holdReadings(t, history, date, workRead(on(0, 9, 0), "5h", 0.1, on(0, 14, 0), quota.StatusAllowed))
	// Three days before today, summarised as the router does, then compressed,
	// and summarised again, as its rounds do; two days before, summarised;
	// yesterday, not yet, as when the router stopped as it ended.
	holdLines(t, dir, "2026-10-02", askedBy("a", "one", on(-3, 9, 0)), askedBy("b", "two", on(-3, 10, 0)))
	holdLines(t, dir, "2026-10-03", askedBy("c", "two", on(-2, 9, 0)), askedBy("d", "one", on(-2, 11, 0)))
	summariseAt(dir, history, on(-1, 2, 0))
	compressFile(t, dir, "requests-2026-10-02.jsonl")
	summariseAt(dir, history, on(-1, 3, 0))
	holdLines(t, dir, "2026-10-04", askedBy("e", "one", on(-1, 9, 0)), askedBy("f", "two", on(-1, 22, 0)))
	holdLines(t, dir, date, askedBy("g", "one", on(0, 9, 0)), askedBy("h", "two", on(0, 9, 20)))

	from := on(-10, 0, 0)
	steps := []struct {
		name string
		at   time.Time
		do   func(t *testing.T)
		// afresh is set where today's lines are given afresh, and writing names
		// the request whose line is still being written, if one is.
		afresh  bool
		writing string
	}{
		{name: "the first read", at: on(0, 10, 0), do: func(*testing.T) {}, afresh: true},
		{
			name: "today's lines and readings appended to, its session read at its limit",
			at:   on(0, 10, 30),
			do: func(t *testing.T) {
				holdLines(t, dir, date, askedBy("i", "one", on(0, 10, 10)), askedBy("j", "three", on(0, 10, 20)))
				holdReadings(t, history, date, workRead(on(0, 10, 10), "5h", 0.6, on(0, 14, 0), quota.StatusAllowed),
					workRead(on(0, 10, 20), "5h", 1, on(0, 14, 0), quota.StatusRejected))
			},
		},
		{
			name: "a line of a long request that arrived before others filed after them",
			at:   on(0, 10, 40),
			do: func(t *testing.T) {
				holdLines(t, dir, date, askedBy("k", "one", on(0, 10, 5)), askedBy("l", "one", on(0, 10, 35)))
			},
		},
		{
			name: "a line half written",
			at:   on(0, 10, 41),
			do: func(t *testing.T) {
				appendTo(t, dir, "requests-"+date+".jsonl", strings.TrimSuffix(jsonOf(t, askedBy("m", "one", on(0, 10, 41))), "}"))
			},
		},
		{name: "its end written", at: on(0, 10, 42), do: func(t *testing.T) { appendTo(t, dir, "requests-"+date+".jsonl", "}\n") }},
		{
			name: "a line written all but its line ending, which reads as a line all the same",
			at:   on(0, 10, 44),
			do: func(t *testing.T) {
				appendTo(t, dir, "requests-"+date+".jsonl", jsonOf(t, askedBy("u", "three", on(0, 10, 43))))
			},
			writing: "u",
		},
		{name: "its line ending written", at: on(0, 10, 45), do: func(t *testing.T) { appendTo(t, dir, "requests-"+date+".jsonl", "\n") }},
		{name: "the clock moved on, nothing written", at: on(0, 12, 0), do: func(*testing.T) {}},
		{
			name: "a line and a reading filed late into the day before's files, and one of today's there, as after a change of time zone",
			at:   on(0, 12, 5),
			do: func(t *testing.T) {
				holdLines(t, dir, "2026-10-04", askedBy("n", "one", on(-1, 23, 59)), askedBy("o", "one", on(0, 0, 30)))
				holdReadings(t, history, "2026-10-04", workRead(on(-1, 23, 59), "5h", 0.55, on(0, 1, 0), quota.StatusAllowed))
			},
		},
		{name: "yesterday summarised", at: on(0, 12, 10), do: func(*testing.T) { summariseAt(dir, history, on(0, 12, 10)) }},
		{
			name: "a line filed under yesterday after its summary",
			at:   on(0, 12, 15),
			do:   func(t *testing.T) { holdLines(t, dir, "2026-10-04", askedBy("p", "one", on(-1, 23, 58))) },
		},
		{name: "yesterday's summary written again", at: on(0, 13, 10), do: func(*testing.T) { summariseAt(dir, history, on(0, 13, 10)) }},
		{
			name: "a line of today's filed under the day after, as after a change of time zone",
			at:   on(0, 13, 20),
			do:   func(t *testing.T) { holdLines(t, dir, "2026-10-06", askedBy("v", "one", on(0, 9, 30))) },
		},
		{
			name: "midnight passed, a request in flight as it did filed under the day before",
			at:   on(1, 0, 10),
			do: func(t *testing.T) {
				holdLines(t, dir, date, askedBy("q", "one", on(0, 23, 59)))
				holdLines(t, dir, "2026-10-06", askedBy("r", "one", on(1, 0, 5)))
				holdReadings(t, history, "2026-10-06", workRead(on(1, 0, 5), "5h", 0.05, on(1, 5, 0), quota.StatusAllowed))
			},
			afresh: true,
		},
		{
			name: "the day before yesterday's files compressed",
			at:   on(1, 0, 20),
			do: func(t *testing.T) {
				compressFile(t, dir, "requests-2026-10-03.jsonl")
				compressFile(t, history, "readings-2026-10-03.jsonl")
			},
		},
		{name: "yesterday summarised", at: on(1, 1, 10), do: func(*testing.T) { summariseAt(dir, history, on(1, 1, 10)) }},
		{
			name: "the oldest days pruned",
			at:   on(1, 1, 20),
			do: func(t *testing.T) {
				removeFile(t, dir, "requests-2026-10-02.jsonl.gz")
				removeFile(t, history, "readings-2026-09-27.jsonl.gz")
			},
		},
		{
			name: "today's file replaced by one holding more",
			at:   on(1, 1, 30),
			do: func(t *testing.T) {
				replaced := filepath.Join(t.TempDir(), "requests")
				data, err := os.ReadFile(filepath.Join(dir, "requests-2026-10-06.jsonl"))
				if err != nil {
					t.Fatal(err)
				}
				data = append(data, []byte(jsonOf(t, askedBy("s", "one", on(1, 1, 25)))+"\n")...)
				if err := os.WriteFile(replaced, data, 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Rename(replaced, filepath.Join(dir, "requests-2026-10-06.jsonl")); err != nil {
					t.Fatal(err)
				}
			},
			afresh: true,
		},
		{
			name: "today's file shrunk",
			at:   on(1, 1, 40),
			do: func(t *testing.T) {
				if err := os.Truncate(filepath.Join(dir, "requests-2026-10-06.jsonl"), int64(len(jsonOf(t, askedBy("r", "one", on(1, 0, 5))))+1)); err != nil {
					t.Fatal(err)
				}
			},
			afresh: true,
		},
		{
			name: "today appended to again",
			at:   on(1, 2, 0),
			do:   func(t *testing.T) { holdLines(t, dir, "2026-10-06", askedBy("t", "one", on(1, 1, 50))) },
		},
	}
	clock := &testClock{}
	follower := followerOf(state, clock, caps)
	var p page
	for _, step := range steps {
		step.do(t)
		clock.at = step.at

		if _, afresh := p.read(follower); afresh != step.afresh {
			t.Errorf("%s: today's lines given afresh %v, want %v", step.name, afresh, step.afresh)
		}
		sameAsWhole(t, step.name, follower, &p, state, step.at, from, step.writing)
	}
}

func TestEachReadOfTodaysLinesAfterTheFirstGivesWhatTheDaysFilesGained(t *testing.T) {
	state, dir, _ := stateDirs(t)
	holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 9, 30)))
	clock := &testClock{at: on(0, 10, 0)}
	follower := followerOf(state, clock, caps)
	var p page
	p.read(follower)
	// Lines are written as their requests end: the third arrived before the
	// fourth, and ended after it.
	third, fourth := asked("3", on(0, 9, 40)), asked("4", on(0, 9, 45))
	holdLines(t, dir, date, fourth, third)

	added, afresh := p.read(follower)
	if want := []string{jsonOf(t, third), jsonOf(t, fourth)}; !slices.Equal(heldJSON(added), want) || afresh {
		t.Errorf("Today() gave\n%s\nafresh %v; want what the day's file gained, oldest first, not afresh\n%s", strings.Join(heldJSON(added), "\n"), afresh,
			strings.Join(want, "\n"))
	}
	if added, afresh := p.read(follower); len(added) != 0 || afresh {
		t.Errorf("Today() gave %d lines, afresh %v; want none, as nothing was written", len(added), afresh)
	}
}

func TestALineOfTodaysStillBeingWrittenIsGivenOnceItsWhole(t *testing.T) {
	state, dir, _ := stateDirs(t)
	clock := &testClock{at: on(0, 10, 0)}
	follower := followerOf(state, clock, caps)
	line := jsonOf(t, asked("1", on(0, 9, 0)))
	// Written all but its line ending, it reads as a line all the same.
	appendTo(t, dir, "requests-"+date+".jsonl", line)
	var p page

	if added, _ := p.read(follower); len(added) != 0 {
		t.Errorf("Today() gave %q, want none until the line is whole", heldJSON(added))
	}
	appendTo(t, dir, "requests-"+date+".jsonl", "\n")
	if added, afresh := p.read(follower); !slices.Equal(heldJSON(added), []string{line}) || afresh {
		t.Errorf("Today() gave %q, afresh %v; want the line once it's whole, not afresh", heldJSON(added), afresh)
	}
}

func TestALineOfTodaysThatArrivesLaterByTheClockIsGivenOnceItsTimeComes(t *testing.T) {
	state, dir, _ := stateDirs(t)
	// The clock set back, a line of a time to come is in today's file.
	later := asked("2", on(0, 11, 0))
	holdLines(t, dir, date, asked("1", on(0, 9, 0)), later)
	clock := &testClock{at: on(0, 10, 0)}
	follower := followerOf(state, clock, caps)
	var p page
	p.read(follower)

	clock.at = on(0, 11, 0)
	if added, afresh := p.read(follower); !slices.Equal(heldJSON(added), []string{jsonOf(t, later)}) || afresh {
		t.Errorf("Today() gave %q, afresh %v; want the later line once the clock reaches it", heldJSON(added), afresh)
	}
}

func TestTodaysMinutesAtItsCapAndAtALimitMoveOnWithTheClockWithNothingWritten(t *testing.T) {
	state, dir, history := stateDirs(t)
	holdLines(t, dir, date, asked("1", on(0, 9, 0)))
	// Work's session at its cap from 09:00, and at its limit from 10:00 until
	// it resets at 14:00.
	holdReadings(t, history, date, workRead(on(0, 9, 0), "5h", 0.95, on(0, 14, 0), quota.StatusAllowed),
		workRead(on(0, 10, 0), "5h", 1, on(0, 14, 0), quota.StatusRejected))
	clock := &testClock{at: on(0, 10, 30)}
	follower := followerOf(state, clock, reserving)
	follower.Days(on(0, 0, 0))

	for _, tt := range []struct {
		at             time.Time
		atCap, atLimit int
	}{
		{at: on(0, 11, 0), atCap: 60, atLimit: 60},
		{at: on(0, 15, 0), atCap: 60, atLimit: 240},
	} {
		clock.at = tt.at
		days := follower.Days(on(0, 0, 0))
		work := days[len(days)-1].Accounts[0]
		if *work.MinutesAtCap != tt.atCap || *work.MinutesAtLimit != tt.atLimit {
			t.Errorf("at %s, work spent %d minutes at its cap and %d at its limit; want %d and %d", tt.at.Format(time.Kitchen), *work.MinutesAtCap,
				*work.MinutesAtLimit, tt.atCap, tt.atLimit)
		}
	}
}

func TestAPastDaysSummaryIsHeldUntilItsStampOrItsDaysFilesChange(t *testing.T) {
	log := logstest.Capture(t)
	state, dir, history := stateDirs(t)
	holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0)))
	summariseAt(dir, history, on(1, 1, 10))
	clock := &testClock{at: on(1, 12, 0)}
	follower := followerOf(state, clock, caps)
	first := fullJSON(t, follower.Days(on(0, 0, 0)))
	// Unchanged, no file of the day is opened again.
	unopenable(t, dir, "*"+date+"*")

	if got := fullJSON(t, follower.Days(on(0, 0, 0))); !slices.Equal(got, first) || log.Has("level=WARN") {
		t.Errorf("log reads\n%s\nDays() =\n%s\nwant it as first read\n%s, no file of the day opened", log, strings.Join(got, "\n"), strings.Join(first, "\n"))
	}

	tests := []struct {
		name string
		// change changes the day's summary, or its files.
		change func(t *testing.T)
	}{
		{
			name: "its summary rewritten, as one damaged in place, and stamped as it was",
			change: func(t *testing.T) {
				summary := summaryFile(dir, date)
				info, err := os.Stat(summary)
				if err != nil {
					t.Fatal(err)
				}
				writeFile(t, dir, "day-"+date+".json", []byte(summaryOf(date, 3, 3, windowsRead{})+"\n"))
				if err := os.Chtimes(summary, time.Time{}, info.ModTime().Add(time.Second)); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name:   "its bytes changed, as a line filed under the day after its summary changes them",
			change: func(t *testing.T) { holdLines(t, dir, date, asked("3", on(0, 23, 59))) },
		},
	}
	for _, tt := range tests {
		for _, file := range []string{summaryFile(dir, date), filepath.Join(dir, "requests-"+date+".jsonl")} {
			if err := os.Chmod(file, 0o600); err != nil {
				t.Fatal(err)
			}
		}
		tt.change(t)

		if got, want := fullJSON(t, follower.Days(on(0, 0, 0))), fullJSON(t, readerAt(state, clock.at).Days(on(0, 0, 0))); !slices.Equal(got, want) {
			t.Errorf("%s: Days() =\n%s\nwant it read again\n%s", tt.name, strings.Join(got, "\n"), strings.Join(want, "\n"))
		}
	}
}

func TestASessionsLinesFiledUnderTheDayAfterTodaysAreRead(t *testing.T) {
	tests := []struct {
		name string
		// lay lays the ledger out in dir.
		lay func(t *testing.T, dir string)
	}{
		{
			name: "one of today's filed under the day after, as after a change of time zone",
			lay: func(t *testing.T, dir string) {
				holdLines(t, dir, date, askedBy("1", "one", on(0, 9, 0)))
				holdLines(t, dir, "2026-10-06", askedBy("2", "one", on(0, 9, 30)))
			},
		},
		{
			name: "the ledger's only file the day after's, as a clock once set ahead names one, holding one of today's",
			lay:  func(t *testing.T, dir string) { holdLines(t, dir, "2026-10-06", askedBy("1", "one", on(0, 9, 30))) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, dir, _ := stateDirs(t)
			tt.lay(t, dir)
			at := on(0, 12, 0)

			got, want := heldJSON(slices.Collect(followerOf(state, &testClock{at: at}, caps).Session("one"))), wholeSession(state, "one", at)
			if !slices.Equal(got, want) || len(want) == 0 {
				t.Errorf("Session() =\n%s\nwant\n%s, as a whole read gives", strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
		})
	}
}

func TestTheFirstDayTheLedgerHoldsIsHeldUntilItsDirectoryChanges(t *testing.T) {
	at := on(0, 12, 0)
	tests := []struct {
		name string
		// changed is when the ledger's directory last changed; held, whether
		// its listing is held; and days, how many days the next read gives,
		// from the first it holds where it's held, or every one asked for, as
		// it can't be listed again.
		changed time.Time
		held    bool
		days    int
	}{
		{name: "changed a second and more before it was listed", changed: at.Add(-time.Hour), held: true, days: 2},
		{
			name:    "changed in the second it was listed in, as a file system that keeps whole seconds can't tell from one changed after",
			changed: at,
			days:    6,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			state, dir, _ := stateDirs(t)
			holdLines(t, dir, "2026-10-04", asked("1", on(-1, 9, 0)))
			if err := os.Chtimes(dir, time.Time{}, tt.changed); err != nil {
				t.Fatal(err)
			}
			follower := followerOf(state, &testClock{at: at}, caps)
			follower.Days(on(-5, 0, 0))
			// Its files can be opened by name, but it can't be listed again.
			if err := os.Chmod(dir, 0o300); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

			days := follower.Days(on(-5, 0, 0))
			if listed := log.Has(`msg="can't read the request ledger"`, "dir="+dir); listed == tt.held || len(days) != tt.days {
				t.Errorf("log reads\n%s\nDays() gave %d days; want %d, the listing held %v", log, len(days), tt.days, tt.held)
			}

			// A file named for a day added changes the directory, which is
			// listed again.
			holdLines(t, dir, "2026-10-03", asked("0", on(-2, 9, 0)))
			follower.Days(on(-5, 0, 0))
			if !log.Has(`msg="can't read the request ledger"`, "dir="+dir) {
				t.Errorf("log reads\n%s\nwant the directory, changed, listed again", log)
			}
		})
	}
}

func TestAFollowerIsSafeForConcurrentUse(t *testing.T) {
	state, dir, _ := stateDirs(t)
	holdLines(t, dir, date, asked("1", on(0, 9, 0)))
	follower := followerOf(state, &testClock{at: on(0, 12, 0)}, caps)
	reads := []func(){
		func() { follower.Days(on(-1, 0, 0)) },
		func() { follower.Today(ledger.Mark{}) },
		func() {
			for range follower.Session("one") {
			}
		},
	}

	var wg sync.WaitGroup
	for i := range 30 {
		wg.Go(reads[i%len(reads)])
	}
	holdLines(t, dir, date, asked("2", on(0, 10, 0)))
	wg.Wait()
	if got, _, _ := follower.Today(ledger.Mark{}); len(got) != 2 {
		t.Errorf("Today() gave %d lines, want both", len(got))
	}
}

func TestTheDaysBeforeOneAreItsDaysSummariesWithoutSummarisingIt(t *testing.T) {
	log := logstest.Capture(t)
	state, dir, _ := stateDirs(t)
	holdLines(t, dir, "2026-10-03", asked("1", on(-2, 9, 0)))
	holdLines(t, dir, "2026-10-04", asked("2", on(-1, 9, 0)))
	holdLines(t, dir, date, asked("3", on(0, 9, 0)))
	at := on(0, 12, 0)
	follower := followerOf(state, &testClock{at: at}, caps)
	want := fullJSON(t, readerAt(state, at).Days(on(-2, 0, 0)))[:2]
	// Today's can't be read, and isn't.
	unopenable(t, dir, "requests-"+date+"*")

	if got := fullJSON(t, follower.DaysBefore(at)); !slices.Equal(got, want) || opened(log) != 0 {
		t.Errorf("log reads\n%s\nDaysBefore() =\n%s\nwant\n%s, as a whole read gives them, today's file never opened", log, strings.Join(got, "\n"),
			strings.Join(want, "\n"))
	}
	if got := follower.DaysBefore(on(-2, 12, 0)); len(got) != 0 {
		t.Errorf("DaysBefore() the first day the ledger holds = %+v, want none", got)
	}
}

func TestADaysLinesAreEverySessionsFiledUnderItInTheOrderTheyCame(t *testing.T) {
	log := logstest.Capture(t)
	state, dir, _ := stateDirs(t)
	holdLines(t, dir, "2026-10-04", askedBy("1", "one", on(-1, 9, 0)), askedBy("2", "two", on(-1, 9, 30)))
	compressFile(t, dir, "requests-2026-10-04.jsonl")
	holdLines(t, dir, date, askedBy("3", "two", on(0, 9, 0)))
	appendTo(t, dir, "requests-"+date+".jsonl", "not a line\n")
	holdLines(t, dir, date, askedBy("4", "one", on(0, 8, 0)))
	follower := followerOf(state, &testClock{at: on(0, 12, 0)}, caps)

	for day, want := range map[string][]string{"2026-10-04": {"1", "2"}, date: {"3", "4"}, "2026-10-03": nil} {
		var got []string
		for l := range follower.DayLines(day) {
			got = append(got, l.Request)
		}
		if !slices.Equal(got, want) {
			t.Errorf("DayLines(%s) gave %q, want %q", day, got, want)
		}
	}
	if !log.Has("level=WARN", `msg="request ledger lines unread"`, "day="+date, "lines=1") {
		t.Errorf("log reads\n%s\nwant the line of %s that doesn't read warned of", log, date)
	}
}

func TestASessionsLinesAreReadOfItsOwnDaysAloneNewestFirst(t *testing.T) {
	log := logstest.Capture(t)
	state, dir, history := stateDirs(t)
	holdLines(t, dir, "2026-10-03", askedBy("1", "one", on(-2, 9, 0)))
	holdLines(t, dir, "2026-10-04", askedBy("2", "two", on(-1, 9, 0)))
	holdLines(t, dir, date, askedBy("3", "one", on(0, 9, 0)), askedBy("4", "two", on(0, 9, 30)), askedBy("5", "one", on(0, 10, 0)))
	summariseAt(dir, history, on(1, 1, 10))
	holdLines(t, dir, "2026-10-06", askedBy("6", "two", on(1, 9, 0)))
	clock := &testClock{at: on(1, 12, 0)}
	follower := followerOf(state, clock, caps)
	follower.Days(on(-2, 0, 0))
	want := wholeSession(state, "one", clock.at)
	// The days whose summaries don't name session one can't be read, and
	// aren't.
	unopenable(t, dir, "requests-2026-10-04*")
	unopenable(t, dir, "requests-2026-10-06*")

	got := slices.Collect(follower.Session("one"))
	if !slices.Equal(heldJSON(got), want) || opened(log) != 0 {
		t.Errorf("log reads\n%s\nSession() =\n%s\nwant\n%s, newest first, no day that doesn't name it opened", log, strings.Join(heldJSON(got), "\n"),
			strings.Join(want, "\n"))
	}

	var first []string
	for h := range follower.Session("one") {
		first = append(first, h.Request)
		break
	}
	if !slices.Equal(first, []string{"5"}) {
		t.Errorf("Session() gave %q before it was stopped, want the newest alone", first)
	}
}
