package ledger_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
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
	return ledger.NewFollower(state, clock.now, c, digestBody, logs.For("cli"))
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

// inArrival returns the lines the page holds, oldest first, each as it
// reads, as readAs has them: the tests' lines each arrived at a time of its
// own.
func (p *page) inArrival(t *testing.T) []string {
	t.Helper()
	return readAs(t, slices.SortedStableFunc(slices.Values(p.lines), func(a, b ledger.Held) int { return a.At.Compare(b.At) }))
}

// readAs is each of held as the line it reads as, written again: Today
// gives its lines without the JSON they were written as, which a Reader's
// hold.
func readAs(t *testing.T, held []ledger.Held) []string {
	t.Helper()
	lines := make([]string, len(held))
	for i, h := range held {
		lines[i] = jsonOf(t, h.Line)
	}
	return lines
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
	today := readAs(t, lines)
	if got := p.inArrival(t); !slices.Equal(got, today) {
		t.Errorf("%s: the page holds\n%s\nwant today's lines\n%s", step, strings.Join(got, "\n"), strings.Join(today, "\n"))
	}
	if got, _, _ := f.Today(ledger.Mark{}); !slices.Equal(readAs(t, got), today) {
		t.Errorf("%s: Today() from the zero Mark =\n%s\nwant\n%s, in the order a whole read gives", step, strings.Join(readAs(t, got), "\n"), strings.Join(today, "\n"))
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
func removeFile(t testing.TB, dir, name string) {
	t.Helper()
	if err := os.Remove(filepath.Join(dir, name)); err != nil {
		t.Fatal(err)
	}
}

// historyIn is the readings history in dir, as the router reads it.
func historyIn(dir string) ledger.History {
	return ledger.HistoryIn(readings.Files(dir, logs.For("router")))
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
	if want := []string{jsonOf(t, third), jsonOf(t, fourth)}; !slices.Equal(readAs(t, added), want) || afresh {
		t.Errorf("Today() gave\n%s\nafresh %v; want what the day's file gained, oldest first, not afresh\n%s", strings.Join(readAs(t, added), "\n"), afresh,
			strings.Join(want, "\n"))
	}
	if added, afresh := p.read(follower); len(added) != 0 || afresh {
		t.Errorf("Today() gave %d lines, afresh %v; want none, as nothing was written", len(added), afresh)
	}
}

// Today's lines, as the follower holds them, and a day's it reads again
// whole, hold none of the JSON they were written as, as
// the dashboard that follows them never reads it, while a Reader's lines,
// which a verb writes as the ledger holds them, hold theirs.
func TestTheFollowersLinesHoldNoneOfTheirJSON(t *testing.T) {
	state, dir, _ := stateDirs(t)
	holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 9, 30)))
	clock := &testClock{at: on(0, 10, 0)}
	follower := followerOf(state, clock, caps)
	today, _, _ := follower.Today(ledger.Mark{})
	for name, lines := range map[string][]ledger.Held{"Today()": today, "Day()": follower.Day(date)} {
		if len(lines) != 2 || slices.ContainsFunc(lines, func(h ledger.Held) bool { return h.JSON != nil }) {
			t.Errorf("%s gave %d lines, %q; want two, none holding its JSON", name, len(lines), heldJSON(lines))
		}
	}
	read := slices.Collect(readerAt(state, clock.at).Lines(startOfDay(clock.at)))
	if len(read) != 2 || slices.ContainsFunc(read, func(h ledger.Held) bool { return h.JSON == nil }) {
		t.Errorf("a Reader's Lines() gave %d lines, %q; want two, each holding its JSON", len(read), heldJSON(read))
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
		t.Errorf("Today() gave %q, want none until the line is whole", readAs(t, added))
	}
	appendTo(t, dir, "requests-"+date+".jsonl", "\n")
	if added, afresh := p.read(follower); !slices.Equal(readAs(t, added), []string{line}) || afresh {
		t.Errorf("Today() gave %q, afresh %v; want the line once it's whole, not afresh", readAs(t, added), afresh)
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
	if added, afresh := p.read(follower); !slices.Equal(readAs(t, added), []string{jsonOf(t, later)}) || afresh {
		t.Errorf("Today() gave %q, afresh %v; want the later line once the clock reaches it", readAs(t, added), afresh)
	}
}

// wholeToday returns today's lines, each as readAs has it, as a Reader gives
// them, reading the ledger in state whole at at.
func wholeToday(t *testing.T, state string, at time.Time) []string {
	t.Helper()
	return readAs(t, slices.Collect(readerAt(state, at).Lines(startOfDay(at))))
}

// requestsHeld returns the requests of lines, in their order.
func requestsHeld(lines []ledger.Held) []string {
	ids := make([]string, len(lines))
	for i, l := range lines {
		ids[i] = l.Request
	}
	return ids
}

func TestAFollowerHoldsNoneOfTodaysLinesBeforeTheMarkLastAskedWith(t *testing.T) {
	state, dir, _ := stateDirs(t)
	// The sixth, of a time to come, as when the clock was set back, is held
	// pending until its time comes.
	holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 9, 30)), asked("6", on(0, 11, 0)))
	clock := &testClock{at: on(0, 10, 0)}
	follower := followerOf(state, clock, caps)
	_, first, _ := follower.Today(ledger.Mark{})
	if held, want := follower.TodayHeld(), []string{"6"}; !slices.Equal(held, want) {
		t.Errorf("after the first read, the follower holds %q, want %q: every line it gave let go of, the line pending kept", held, want)
	}
	holdLines(t, dir, date, asked("3", on(0, 9, 40)))
	_, second, _ := follower.Today(first)
	if held, want := follower.TodayHeld(), []string{"3", "6"}; !slices.Equal(held, want) {
		t.Errorf("after a read from the first mark, the follower holds %q, want %q: those given since it, and the line pending", held, want)
	}
	holdLines(t, dir, date, asked("4", on(0, 9, 45)), asked("5", on(0, 9, 50)))
	gained, _, _ := follower.Today(second)
	if held, want := follower.TodayHeld(), []string{"4", "5", "6"}; !slices.Equal(held, want) {
		t.Errorf("after a read from the second mark, the follower holds %q, want %q: those given since it, and the line pending", held, want)
	}

	again, _, afresh := follower.Today(second)
	if got, want := requestsHeld(again), requestsHeld(gained); !slices.Equal(got, want) || afresh {
		t.Errorf("Today() from the same mark again gave %q, afresh %v; want %q again, not afresh", got, afresh, want)
	}
}

func TestAMarkBeforeWhatTheFollowerHoldsGetsTodayWholeReadAgain(t *testing.T) {
	for _, tt := range []struct {
		name string
		// mark is the mark asked with, given the first read to, before the
		// one last asked with.
		mark func(first ledger.Mark) ledger.Mark
	}{
		{name: "the zero mark", mark: func(ledger.Mark) ledger.Mark { return ledger.Mark{} }},
		{name: "a mark before the one last asked with", mark: func(first ledger.Mark) ledger.Mark { return first }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			state, dir, _ := stateDirs(t)
			// The day before's file holds one of its own and one of today's, as
			// after a change of time zone; the sixth is of a time to come, held
			// pending.
			holdLines(t, dir, "2026-10-04", asked("y", on(-1, 23, 0)), asked("z", on(0, 0, 30)))
			holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 9, 30)), asked("6", on(0, 11, 0)))
			clock := &testClock{at: on(0, 10, 0)}
			follower := followerOf(state, clock, caps)
			_, first, _ := follower.Today(ledger.Mark{})
			holdLines(t, dir, date, asked("3", on(0, 9, 40)))
			_, second, _ := follower.Today(first)
			holdLines(t, dir, date, asked("4", on(0, 9, 45)))
			follower.Today(second)

			got, next, afresh := follower.Today(tt.mark(first))
			wholeIs(t, "Today()", got, afresh, wholeToday(t, state, clock.at))
			if held, want := follower.TodayHeld(), []string{"6"}; !slices.Equal(held, want) {
				t.Errorf("after the whole read, the follower holds %q, want %q: every line it gave let go of, the line pending kept", held, want)
			}
			if gained, again, afresh := follower.Today(next); len(gained) != 0 || afresh || again != next {
				t.Errorf("Today() from the mark the whole read gave gave %q, afresh %v; want none, not afresh, as nothing was written", requestsHeld(gained), afresh)
			}
			holdLines(t, dir, date, asked("5", on(0, 9, 50)))
			if gained, _, afresh := follower.Today(next); !slices.Equal(requestsHeld(gained), []string{"5"}) || afresh {
				t.Errorf("Today() from the mark the whole read gave gave %q, afresh %v; want what the file gained since, not afresh", requestsHeld(gained), afresh)
			}
			for name, mark := range map[string]ledger.Mark{"the zero mark": {}, "the second mark": second} {
				got, _, afresh := follower.Today(mark)
				wholeIs(t, "Today() from "+name+" again", got, afresh, wholeToday(t, state, clock.at))
			}
		})
	}
}

// wholeIs fails t where got, and whether it was given afresh, isn't today's
// lines whole, as want has them, afresh.
func wholeIs(t *testing.T, name string, got []ledger.Held, afresh bool, want []string) {
	t.Helper()
	if !slices.Equal(readAs(t, got), want) || !afresh {
		t.Errorf("%s gave\n%s\nafresh %v; want today's lines whole, afresh, as a whole read gives\n%s",
			name, strings.Join(readAs(t, got), "\n"), afresh, strings.Join(want, "\n"))
	}
}

func TestTodaysFilesGrowingAfterAWholeReadGiveEachLineOnce(t *testing.T) {
	state, dir, _ := stateDirs(t)
	holdLines(t, dir, date, asked("1", on(0, 9, 0)))
	clock := &testClock{at: on(0, 10, 0)}
	follower := followerOf(state, clock, caps)
	var p page
	for _, id := range []string{"2", "3"} {
		p.read(follower)
		holdLines(t, dir, date, asked(id, on(0, 9, 10)))
	}
	p.read(follower)
	// The page asks afresh, as the Log does as it opens, and the files grow
	// before it reads again.
	p.mark = ledger.Mark{}
	if _, afresh := p.read(follower); !afresh {
		t.Error("Today() from the zero Mark gave today's lines not afresh, want them afresh")
	}
	holdLines(t, dir, date, asked("4", on(0, 9, 20)))
	clock.at = on(0, 10, 5)

	if added, afresh := p.read(follower); !slices.Equal(requestsHeld(added), []string{"4"}) || afresh {
		t.Errorf("Today() gave %q, afresh %v; want only what the file gained since the whole read, not afresh", requestsHeld(added), afresh)
	}
	if got, want := p.inArrival(t), wholeToday(t, state, clock.at); !slices.Equal(got, want) {
		t.Errorf("the page holds\n%s\nwant each of today's lines once\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if held := follower.TodayHeld(); !slices.Equal(held, []string{"4"}) {
		t.Errorf("the follower holds %q, want only the line given since the mark last asked with", held)
	}
}

func TestTodaysLinesGivenAfreshAreReadAgainFromTheFiles(t *testing.T) {
	tests := []struct {
		name string
		at   time.Time
		do   func(t *testing.T, dir string)
		// pending are the lines the follower holds once it's given every one
		// of today's: those of a time to come.
		pending []string
	}{
		{
			name: "midnight passed, a request in flight as it did filed under the day before",
			at:   on(1, 0, 10),
			do: func(t *testing.T, dir string) {
				holdLines(t, dir, date, asked("late", on(0, 23, 59)), asked("in flight", on(1, 0, 1)))
				holdLines(t, dir, "2026-10-06", asked("next", on(1, 0, 5)))
			},
		},
		{
			name:    "the clock set back to the day before",
			at:      on(-1, 22, 0),
			do:      func(*testing.T, string) {},
			pending: []string{"1", "2"},
		},
		{
			name: "today's file replaced by one holding more",
			at:   on(0, 10, 10),
			do: func(t *testing.T, dir string) {
				data, err := os.ReadFile(filepath.Join(dir, "requests-"+date+".jsonl"))
				if err != nil {
					t.Fatal(err)
				}
				elsewhere := t.TempDir()
				writeFile(t, elsewhere, "requests", append(data, []byte(jsonOf(t, asked("more", on(0, 10, 5)))+"\n")...))
				if err := os.Rename(filepath.Join(elsewhere, "requests"), filepath.Join(dir, "requests-"+date+".jsonl")); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, dir, _ := stateDirs(t)
			holdLines(t, dir, "2026-10-04", asked("before", on(-1, 21, 0)))
			holdLines(t, dir, date, asked("1", on(0, 9, 0)))
			clock := &testClock{at: on(0, 10, 0)}
			follower := followerOf(state, clock, caps)
			var p page
			p.read(follower)
			holdLines(t, dir, date, asked("2", on(0, 9, 30)))
			p.read(follower)
			tt.do(t, dir)
			clock.at = tt.at

			if _, afresh := p.read(follower); !afresh {
				t.Error("today's lines given not afresh, want them afresh")
			}
			if got, want := p.inArrival(t), wholeToday(t, state, tt.at); !slices.Equal(got, want) {
				t.Errorf("the page holds\n%s\nwant today's lines\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
			if added, afresh := p.read(follower); len(added) != 0 || afresh {
				t.Errorf("Today() gave %q, afresh %v; want none, not afresh, as nothing was written", requestsHeld(added), afresh)
			}
			if held := follower.TodayHeld(); !slices.Equal(held, tt.pending) {
				t.Errorf("the follower holds %q, want %q: none given since the mark last asked with, but those pending", held, tt.pending)
			}
		})
	}
}

func TestTodaysFileAWholeReadCantReadIsReadAfreshNext(t *testing.T) {
	state, dir, _ := stateDirs(t)
	holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 9, 10)))
	clock := &testClock{at: on(0, 10, 0)}
	follower := followerOf(state, clock, caps)
	var p page
	p.read(follower)
	holdLines(t, dir, date, asked("3", on(0, 9, 20)))
	p.read(follower)
	p.read(follower)
	// A change of its mode moves neither the file's size nor when it was
	// last modified, so its tail alone would never tell.
	unopenable(t, dir, "requests-"+date+".jsonl")
	follower.Today(ledger.Mark{})
	if err := os.Chmod(filepath.Join(dir, "requests-"+date+".jsonl"), 0o600); err != nil {
		t.Fatal(err)
	}
	holdLines(t, dir, date, asked("4", on(0, 9, 30)))

	if _, afresh := p.read(follower); !afresh {
		t.Error("today's lines given not afresh after a whole read couldn't read today's file, want them afresh")
	}
	if got, want := p.inArrival(t), wholeToday(t, state, clock.at); !slices.Equal(got, want) {
		t.Errorf("the page holds\n%s\nwant today's lines\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestAWholeReadReadsAgainOnlyTheFilesThatGaveTodaysLines(t *testing.T) {
	log := logstest.Capture(t)
	state, dir, _ := stateDirs(t)
	holdLines(t, dir, "2026-10-04", asked("y", on(-1, 23, 0)))
	holdLines(t, dir, date, asked("1", on(0, 9, 0)))
	follower := followerOf(state, &testClock{at: on(0, 10, 0)}, caps)
	_, first, _ := follower.Today(ledger.Mark{})
	holdLines(t, dir, date, asked("2", on(0, 9, 10)))
	follower.Today(first)
	unopenable(t, dir, "requests-2026-10-04.jsonl")

	if got, _, _ := follower.Today(ledger.Mark{}); !slices.Equal(requestsHeld(got), []string{"1", "2"}) || log.Has("permission denied") {
		t.Errorf("log reads\n%s\nToday() from the zero Mark gave %q; want today's lines, the day before's file, which gave none of them, not opened",
			log, requestsHeld(got))
	}
}

func TestTodaysUnreadLinesAreWarnedOfOnceAsTheyreReadAfresh(t *testing.T) {
	log := logstest.Capture(t)
	state, dir, _ := stateDirs(t)
	holdLines(t, dir, date, asked("1", on(0, 9, 0)))
	appendTo(t, dir, "requests-"+date+".jsonl", "not a line\n")
	clock := &testClock{at: on(0, 10, 0)}
	follower := followerOf(state, clock, caps)
	follower.Today(ledger.Mark{})
	// Midnight passed: the day before's files, today's as was, are read again
	// from their start.
	clock.at = on(1, 0, 10)
	follower.Today(ledger.Mark{})

	if warned := strings.Count(log.String(), "request ledger lines unread"); warned != 1 {
		t.Errorf("log reads\n%s\nwant the unread line warned of once, not again as it's read afresh: %d times", log, warned)
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

// FirstDay tells the start of the first day the ledger holds, as Days reads
// from it, failing with fs.ErrNotExist while it holds none.
func TestTheFollowerTellsTheFirstDayTheLedgerHolds(t *testing.T) {
	state, dir, _ := stateDirs(t)
	at := on(0, 12, 0)
	follower := followerOf(state, &testClock{at: at}, caps)
	if _, err := follower.FirstDay(at); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("FirstDay() of a ledger holding nothing fails with %v, want fs.ErrNotExist", err)
	}
	holdLines(t, dir, "2026-10-03", asked("1", on(-2, 9, 0)))
	holdLines(t, dir, "2026-10-04", asked("2", on(-1, 9, 0)))
	if got, err := follower.FirstDay(at); err != nil || !got.Equal(on(-2, 0, 0)) {
		t.Errorf("FirstDay() = %v, %v; want the start of 3 October", got, err)
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

func TestTheReadingsHistorysListingIsHeldUntilItsDirectoryChanges(t *testing.T) {
	at := on(0, 12, 0)
	tests := []struct {
		name string
		// changed is when the history's directory last changed, and held
		// whether its listing is held.
		changed time.Time
		held    bool
	}{
		{name: "changed a second and more before it was listed", changed: at.Add(-time.Hour), held: true},
		{name: "changed in the second it was listed in, as a file system that keeps whole seconds can't tell from one changed after", changed: at},
	}
	const unlisted = `msg="can't read the readings history"`
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			state, dir, history := stateDirs(t)
			// A day not yet summarised, which asks whether the history reaches it.
			holdLines(t, dir, "2026-10-04", asked("1", on(-1, 9, 0)))
			holdReadings(t, history, "2026-10-04", workRead(on(-1, 10, 0), "5h", 0.3, on(-1, 13, 0), quota.StatusAllowed))
			if err := os.Chtimes(history, time.Time{}, tt.changed); err != nil {
				t.Fatal(err)
			}
			follower := followerOf(state, &testClock{at: at}, caps)
			follower.Days(on(-1, 0, 0))
			unlistable(t, history)

			follower.Days(on(-1, 0, 0))
			if listed := log.Has(unlisted); listed == tt.held {
				t.Errorf("log reads\n%s\nwant the listing held %v", log, tt.held)
			}

			// A file added changes the directory, which is listed again.
			holdReadings(t, history, date, workRead(on(0, 10, 0), "5h", 0.1, on(0, 15, 0), quota.StatusAllowed))
			follower.Days(on(-1, 0, 0))
			if !log.Has(unlisted) {
				t.Errorf("log reads\n%s\nwant the directory, changed, listed again", log)
			}
		})
	}
}

func TestAFollowerWarnsOfAReadingsHistoryItCantListOnceUntilItCan(t *testing.T) {
	log := logstest.Capture(t)
	state, dir, history := stateDirs(t)
	// A day not yet summarised, which asks whether the history reaches it.
	holdLines(t, dir, "2026-10-04", asked("1", on(-1, 9, 0)))
	unlistable(t, history)
	follower := followerOf(state, &testClock{at: on(0, 12, 0)}, caps)
	const unlisted = `msg="can't read the readings history"`

	follower.Days(on(-1, 0, 0))
	follower.Days(on(-1, 0, 0))
	if warned := strings.Count(log.String(), unlisted); warned != 1 {
		t.Errorf("log reads\n%s\nwant the history that can't be listed warned of once, not at each look: %d times", log, warned)
	}
	if err := os.Chmod(history, 0o700); err != nil {
		t.Fatal(err)
	}
	follower.Days(on(-1, 0, 0))
	unlistable(t, history)
	follower.Days(on(-1, 0, 0))
	if warned := strings.Count(log.String(), unlisted); warned != 2 {
		t.Errorf("log reads\n%s\nwant it warned of again once it can't be listed after it could: %d times", log, warned)
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

// eachWhole has a read of sessions read each whole, resting on no base.
func eachWhole(string) ledger.Rest { return ledger.RestWhole }

// wholeOf has a read of sessions read whole those with the given ids,
// resting on no base, and the others on their states.
func wholeOf(ids ...string) func(id string) ledger.Rest {
	return func(id string) ledger.Rest {
		if slices.Contains(ids, id) {
			return ledger.RestWhole
		}
		return ledger.RestOnState
	}
}

// requestsOf returns the ids of lines' requests, in their order.
func requestsOf(lines []ledger.Line) []string {
	ids := make([]string, len(lines))
	for i, l := range lines {
		ids[i] = l.Request
	}
	return ids
}

func TestSessionsReadsTodaysAndThoseAskedForEachAsSessionReadsOne(t *testing.T) {
	state, dir, history := stateDirs(t)
	holdLines(t, dir, "2026-10-03", askedBy("1", "one", on(-2, 9, 0)), askedBy("2", "old", on(-2, 10, 0)))
	holdLines(t, dir, "2026-10-04", askedBy("3", "two", on(-1, 9, 0)))
	// Request 6 arrived today, filed under yesterday's date, as after a
	// change of time zone.
	holdLines(t, dir, date, askedBy("4", "one", on(0, 9, 0)), askedBy("5", "two", on(0, 9, 30)), askedBy("6", "two", on(1, 0, 30)))
	summariseAt(dir, history, on(1, 1, 10))
	// Request 8 arrives after now, as after a clock set back; 9 is filed
	// under tomorrow's date, as a clock once set ahead files one.
	holdLines(t, dir, "2026-10-06", askedBy("7", "one", on(1, 9, 0)), askedBy("8", "one", on(1, 13, 0)))
	holdLines(t, dir, "2026-10-07", askedBy("9", "three", on(1, 9, 30)))
	clock := &testClock{at: on(1, 12, 0)}
	follower := followerOf(state, clock, caps)

	taken := make(map[string]string)
	got := follower.Sessions([]string{"old"}, eachWhole, func(date string, l ledger.Line) { taken[l.Request] += date })
	want := map[string][]string{"one": {"1", "4", "7"}, "two": {"3", "5", "6"}, "three": {"9"}, "old": {"2"}}
	if len(got) != len(want) {
		t.Errorf("Sessions() read %d sessions, want %d: today's, and old, asked for", len(got), len(want))
	}
	for id, requests := range want {
		if !slices.Equal(requestsOf(got[id].Lines), requests) || got[id].Base != nil {
			t.Errorf("Sessions() gave %s resting on %+v, its requests %q; want it on none, read whole, its requests %q, oldest first, each once",
				id, got[id].Base, requestsOf(got[id].Lines), requests)
		}
		var session []ledger.Line
		for h := range follower.Session(id) {
			session = append(session, h.Line)
		}
		slices.Reverse(session)
		if !slices.Equal(requestsOf(session), requests) {
			t.Errorf("Session(%s) gave requests %q, want %q, as Sessions gives them", id, requestsOf(session), requests)
		}
	}
	wantTaken := map[string]string{"1": "2026-10-03", "2": "2026-10-03", "3": "2026-10-04", "4": date, "5": date, "6": date, "7": "2026-10-06", "9": "2026-10-07"}
	if !maps.Equal(taken, wantTaken) {
		t.Errorf("Sessions() handed take %v, want each line read that arrived by now once, with its file's date %v", taken, wantTaken)
	}
}

// A read of sessions reads their days side by side, but takes in what each
// read as one read after another would: take is handed the lines of each
// day in the order its files hold them, the days oldest first, and a
// session's lines arriving at once come in that order, whichever day's read
// ends first.
func TestSessionsOfTakesInTheDaysItReadsSideBySideOldestFirst(t *testing.T) {
	state, dir, history := stateDirs(t)
	same := on(-2, 9, 0)
	var want []string
	for back := 6; back >= 0; back-- {
		day := on(-back, 0, 0).Format(time.DateOnly)
		lines := make([]ledger.Line, 5)
		for i := range lines {
			request := fmt.Sprintf("%d-%d", 6-back, i)
			lines[i], want = askedBy(request, "one", on(-back, 9, i)), append(want, request)
		}
		if back == 2 {
			lines[4].At = same
		}
		if back == 1 {
			lines[0].At = same
		}
		holdLines(t, dir, day, lines...)
	}
	summariseAt(dir, history, on(0, 1, 10))
	follower := followerOf(state, &testClock{at: on(0, 12, 0)}, caps)

	var taken []string
	got := follower.SessionsOf([]string{"one"}, eachWhole, func(_ string, l ledger.Line) { taken = append(taken, l.Request) })
	if !slices.Equal(taken, want) {
		t.Errorf("SessionsOf() handed take %q, want each day's lines in its files' order, the days oldest first, %q", taken, want)
	}
	held := requestsOf(got["one"].Lines)
	if i := slices.Index(held, "4-4"); i < 0 || i+1 == len(held) || held[i+1] != "5-0" {
		t.Errorf("SessionsOf() gave the session's requests %q, want 4-4 and 5-0, arriving at once, in their days' order", held)
	}
}

func TestSessionsReadsNoDayBeforeYesterdayWhereNoSessionIsRead(t *testing.T) {
	log := logstest.Capture(t)
	state, dir, history := stateDirs(t)
	holdLines(t, dir, "2026-10-03", askedBy("1", "one", on(-2, 9, 0)))
	holdLines(t, dir, "2026-10-04", askedBy("2", "one", on(-1, 9, 0)))
	summariseAt(dir, history, on(0, 1, 10))
	unopenable(t, dir, "*2026-10-03*")
	follower := followerOf(state, &testClock{at: on(1, 12, 0)}, caps)

	if got := follower.Sessions(nil, eachWhole, nil); len(got) != 0 || opened(log) != 0 {
		t.Errorf("log reads\n%s\nSessions() = %v, want no session read, nor a day before yesterday opened", log, got)
	}
}

// listCounted is a readings history that counts the times it's listed.
type listCounted struct {
	ledger.History
	listed int
}

func (l *listCounted) Dates() ([]string, error) {
	l.listed++
	return l.History.Dates()
}

func TestAReadOfTheDaysListsTheReadingsHistoryOnce(t *testing.T) {
	for _, by := range []string{"a reader", "a follower"} {
		t.Run("by "+by, func(t *testing.T) {
			state, dir, history := stateDirs(t)
			// Days not yet summarised, each of which asks the history whether it
			// reaches it, as its readings are read only where it does.
			for back := 3; back >= 1; back-- {
				holdLines(t, dir, on(-back, 0, 0).Format(time.DateOnly), asked("1", on(-back, 9, 0)))
			}
			h := &listCounted{History: historyIn(history)}
			reader, follower := readerAt(state, on(2, 12, 0)), followerOf(state, &testClock{at: on(2, 12, 0)}, caps)
			reader.SetHistory(h)
			follower.SetHistory(h)
			days := reader.Days
			if by == "a follower" {
				days = follower.Days
			}

			days(on(-3, 0, 0))
			if h.listed != 1 {
				t.Errorf("the read listed the readings history %d times, want once", h.listed)
			}
		})
	}
}

func TestASessionsDaysAreFoundWithoutTheReadingsHistory(t *testing.T) {
	tests := []struct {
		name string
		read func(f *ledger.Follower)
	}{
		{name: "by Session", read: func(f *ledger.Follower) {
			for range f.Session("one") {
			}
		}},
		{name: "by Sessions", read: func(f *ledger.Follower) { f.Sessions([]string{"one"}, eachWhole, nil) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			state, dir, history := stateDirs(t)
			holdLines(t, dir, "2026-10-03", askedBy("1", "one", on(-2, 9, 0)))
			holdLines(t, dir, "2026-10-07", askedBy("2", "one", on(2, 9, 0)))
			// Days of no requests between, not yet summarised, their readings
			// open to no one, so a read of them warns that it can't.
			holdReadings(t, history, "2026-10-04", workRead(on(-1, 10, 0), "5h", 0.3, on(-1, 13, 0), quota.StatusAllowed))
			holdReadings(t, history, date, workRead(on(0, 10, 0), "5h", 0.3, on(0, 13, 0), quota.StatusAllowed))
			unopenable(t, history, "readings-*")

			tt.read(followerOf(state, &testClock{at: on(2, 12, 0)}, caps))
			if n := opened(log); n != 0 {
				t.Errorf("log reads\n%s\nthe session's days were found opening %d of the readings history's files, want none: they name no session", log, n)
			}
		})
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

// A session's lines read within spans are those of the days whose files may
// hold lines of the spans alone, from the day before each one's start to the
// day after its end, newest first: every day between is passed over, never
// opened, its summary unread.
func TestASessionsLinesWithinSpansAreReadOfTheSpansDaysAlone(t *testing.T) {
	log := logstest.Capture(t)
	state, dir, history := stateDirs(t)
	dayOf := func(back int) string { return on(-back, 0, 0).Format(time.DateOnly) }
	for back := 30; back >= 0; back-- {
		holdLines(t, dir, dayOf(back), askedBy(dayOf(back), "one", on(-back, 9, 0)))
	}
	summariseAt(dir, history, on(-1, 1, 10))
	clock := &testClock{at: on(0, 12, 0)}
	turn, horizon := ledger.Span{From: on(-20, 8, 0), To: on(-20, 10, 0)}, ledger.Span{From: on(0, 11, 0), To: clock.at}
	unopenable(t, dir, "requests-2026-09-1[7-9]*")
	unopenable(t, dir, "requests-2026-09-2*")
	unopenable(t, dir, "requests-2026-09-30*")
	unopenable(t, dir, "requests-2026-10-0[1-3]*")

	var got []string
	for h := range followerOf(state, clock, caps).Session("one", turn, horizon) {
		got = append(got, h.Request)
	}
	if want := []string{dayOf(0), dayOf(1), dayOf(19), dayOf(20), dayOf(21)}; !slices.Equal(got, want) || opened(log) != 0 {
		t.Errorf("log reads\n%s\nSession() within %v and %v gave %q, want %q, newest first, no day between opened", log, turn, horizon, got, want)
	}
}

// Spanning holds a day to the spans given as dayfile.Dates has each: from
// the day before its start's to the day after its end's; every day where
// none is given.
func TestSpanningHoldsTheDaysEitherSideOfEachSpan(t *testing.T) {
	week := ledger.Span{From: on(-10, 23, 0), To: on(-8, 1, 0)}
	today := ledger.Span{From: on(0, 9, 0), To: on(0, 10, 0)}
	tests := []struct {
		name  string
		spans []ledger.Span
		date  string
		want  bool
	}{
		{"none given, a day long ago", nil, "2020-01-01", true},
		{"the day before a span's start", []ledger.Span{week}, "2026-09-24", true},
		{"two days before a span's start", []ledger.Span{week}, "2026-09-23", false},
		{"a day within a span", []ledger.Span{week}, "2026-09-26", true},
		{"the day after a span's end", []ledger.Span{week}, "2026-09-28", true},
		{"two days after a span's end", []ledger.Span{week}, "2026-09-29", false},
		{"between two spans", []ledger.Span{week, today}, "2026-10-01", false},
		{"the second of two spans", []ledger.Span{week, today}, date, true},
	}
	for _, tt := range tests {
		if got := ledger.Spanning(tt.spans)(tt.date); got != tt.want {
			t.Errorf("%s: Spanning(%v)(%s) = %v, want %v", tt.name, tt.spans, tt.date, got, tt.want)
		}
	}
}

// DaysOn gives the days asked for as Days gives them, each once, oldest
// first, but none before the first day the ledger holds or after today's,
// and looks at the files of those days alone, however far back they are.
func TestAFollowerGivesTheDaysAskedForAloneAsDaysGivesThem(t *testing.T) {
	state, dir, history := stateDirs(t)
	dayOf := func(back int) string { return on(-back, 0, 0).Format(time.DateOnly) }
	for back := 30; back >= 1; back-- {
		holdLines(t, dir, dayOf(back), asked(dayOf(back), on(-back, 9, 0)))
	}
	holdLines(t, dir, dayOf(0), asked("today", on(0, 9, 0)))
	summariseAt(dir, history, on(-10, 1, 10))
	clock := &testClock{at: on(0, 12, 0)}
	every := followerOf(state, clock, caps).Days(on(-30, 0, 0))
	follower := followerOf(state, clock, caps)

	got := follower.DaysOn([]string{dayOf(0), dayOf(20), dayOf(31), dayOf(20), dayOf(-1)})
	want := []ledger.Summary{every[10], every[30]}
	if !slices.Equal(fullJSON(t, got), fullJSON(t, want)) {
		t.Errorf("DaysOn() =\n%s\nwant\n%s, 20 days back and today, as Days gives them", strings.Join(fullJSON(t, got), "\n"), strings.Join(fullJSON(t, want), "\n"))
	}
	if looked := follower.Followed(); !slices.Equal(looked, []string{dayOf(20), dayOf(0)}) {
		t.Errorf("DaysOn() looked at the files of %q, want those of the days given alone", looked)
	}
	if got := follower.DaysOn(nil); got != nil {
		t.Errorf("DaysOn() of no day = %+v, want none", got)
	}
}
