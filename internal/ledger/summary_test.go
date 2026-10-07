package ledger_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
)

// date is the date of the local day the summaries' tests summarise, a Monday.
const date = "2026-10-05"

// The models the tests' requests ask for.
const (
	opus  = "claude-opus-5-5"
	haiku = "claude-haiku-4-5"
)

// on returns the local time days after the summaries' day, at hour:minute.
func on(days, hour, minute int) time.Time {
	return time.Date(2026, 10, 5+days, hour, minute, 0, 0, time.Local)
}

// readingsOf gives read as the readings history does: those of the times
// asked for, in the order they came.
func readingsOf(read ...readings.Reading) ledger.Readings {
	return func(from, to time.Time) iter.Seq[readings.Reading] {
		return func(yield func(readings.Reading) bool) {
			for _, r := range read {
				if !r.At.Before(from) && r.At.Before(to) && !yield(r) {
					return
				}
			}
		}
	}
}

// noReadings gives no readings, as a readings history that holds none.
func noReadings(time.Time, time.Time) iter.Seq[readings.Reading] {
	return func(func(readings.Reading) bool) {}
}

// workRead is work's window with the given key read at at, at u of its use,
// resetting at resets, with the status given.
func workRead(at time.Time, key string, u float64, resets time.Time, status quota.Status) readings.Reading {
	return readings.Reading{At: at, Account: "work", Key: key, Utilization: u, ResetsAt: resets, Status: status, Source: readings.FromAnswer}
}

// usage is a usage as the API gives it.
func usage(s string) json.RawMessage {
	return json.RawMessage(s)
}

// summarised returns the summary of the day of lines, with readings, as it's
// written and read back: what the ledger keeps of it.
func summarised(t *testing.T, readings ledger.Readings, lines ...ledger.Line) ledger.Summary {
	t.Helper()
	data := summaryJSON(t, readings, lines...)
	var kept ledger.Summary
	if err := json.Unmarshal(data, &kept); err != nil {
		t.Fatal(err)
	}
	return kept
}

// summaryJSON returns the summary of the day of lines, with readings, as
// it's written.
func summaryJSON(t *testing.T, readings ledger.Readings, lines ...ledger.Line) []byte {
	t.Helper()
	s, err := ledger.Summarise(date, slices.Values(lines), readings)
	if err != nil {
		t.Fatalf("Summarise() error = %v", err)
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestASummaryHoldsEachAccountsRequestsByModel(t *testing.T) {
	lines := []ledger.Line{
		{At: on(0, 9, 0), Request: "1", Kind: ledger.KindMessage, Session: "one", Model: opus, Account: "work", Reason: "sticky", Status: 200, Attempts: 1,
			Usage: usage(`{"input_tokens":12,"cache_creation_input_tokens":3120,"cache_read_input_tokens":182340,` +
				`"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":3120},"output_tokens":845,"service_tier":"standard"}`)},
		{At: on(0, 9, 5), Request: "2", Kind: ledger.KindMessage, Session: "one", Model: opus, Account: "work", Reason: "sticky", Status: 200, Attempts: 2,
			Usage: usage(`{"input_tokens":3,"cache_read_input_tokens":185460,"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":0},` +
				`"output_tokens":120,"server_tool_use":{"web_search_requests":2},"service_tier":"standard"}`)},
		{At: on(0, 9, 6), Request: "3", Kind: ledger.KindCount, Session: "one", Model: opus, Account: "work", Reason: "sticky", Status: 200, Attempts: 1},
		// Claude Code's quota check as a session resumes, under an id never
		// used again.
		{At: on(0, 9, 7), Request: "4", Kind: ledger.KindCheck, Session: "resuming", Model: haiku, Account: "work", Reason: "new", Status: 200, Attempts: 1,
			Usage: usage(`{"input_tokens":8,"output_tokens":1,"service_tier":"standard"}`)},
		{At: on(0, 9, 8), Request: "5", Kind: ledger.KindMessage, Model: opus, Account: "work", Reason: "sticky", Status: 529, Attempts: 1},
		{At: on(0, 10, 0), Request: "6", Kind: ledger.KindMessage, Session: "two", Model: opus, Account: "side", Reason: "sticky", Status: 200, Attempts: 1,
			Usage: usage(`{"input_tokens":5,"output_tokens":50}`)},
		{At: on(0, 11, 0), Request: "7", Kind: ledger.KindMessage, Session: "three", Model: opus, Reason: "no account has room", Status: 429},
		{At: on(0, 11, 1), Request: "8", Kind: ledger.KindMessage, Session: "four", Status: 413},
	}

	got := string(summaryJSON(t, noReadings, lines...))
	want := `{"version":1,"day":"2026-10-05","accounts":[` +
		`{"models":[{"upstream":0,"unsent":1,"checks":0,"counts":0,"sessions":1},` +
		`{"model":"claude-opus-5-5","upstream":0,"unsent":1,"checks":0,"counts":0,"sessions":1}],"sessions":2,"moved_on":0,"moved_off":0},` +
		`{"account":"side","models":[{"model":"claude-opus-5-5","upstream":1,"unsent":0,"checks":0,"counts":0,"sessions":1,` +
		`"usage":{"input_tokens":5,"output_tokens":50}}],"sessions":1,"moved_on":0,"moved_off":0},` +
		`{"account":"work","models":[{"model":"claude-haiku-4-5","upstream":0,"unsent":0,"checks":1,"counts":0,"sessions":0,` +
		`"usage":{"input_tokens":8,"output_tokens":1}},` +
		`{"model":"claude-opus-5-5","upstream":3,"unsent":0,"checks":0,"counts":1,"sessions":1,` +
		`"usage":{"cache_creation":{"ephemeral_1h_input_tokens":3120,"ephemeral_5m_input_tokens":0},"cache_creation_input_tokens":3120,` +
		`"cache_read_input_tokens":367800,"input_tokens":15,"output_tokens":965,"server_tool_use":{"web_search_requests":2}}}],` +
		`"sessions":1,"moved_on":0,"moved_off":0}]}`
	if got != want {
		t.Errorf("the summary is\n%s\nwant\n%s", got, want)
	}
}

func TestASummaryKeepsApartAModelsRequestsThatAskedForAnotherInferenceGeo(t *testing.T) {
	asking := func(request, session, geo string) ledger.Line {
		return ledger.Line{At: on(0, 9, 0), Request: request, Kind: ledger.KindMessage, Session: session, Model: opus, Account: "work", Reason: "sticky",
			Status: 200, Attempts: 1, Shape: ledger.Shape{InferenceGeo: geo}, Usage: usage(`{"input_tokens":10,"output_tokens":20}`)}
	}
	lines := []ledger.Line{asking("1", "one", "us"), asking("2", "one", ""), asking("3", "two", "us"), asking("4", "two", "global"),
		{At: on(0, 9, 1), Request: "5", Kind: ledger.KindMessage, Session: "one", Model: haiku, Account: "work", Reason: "sticky", Status: 200, Attempts: 1}}

	got := string(summaryJSON(t, noReadings, lines...))
	want := `{"version":1,"day":"2026-10-05","accounts":[{"account":"work","models":[` +
		`{"model":"claude-haiku-4-5","upstream":1,"unsent":0,"checks":0,"counts":0,"sessions":1},` +
		`{"model":"claude-opus-5-5","upstream":1,"unsent":0,"checks":0,"counts":0,"sessions":1,"usage":{"input_tokens":10,"output_tokens":20}},` +
		`{"model":"claude-opus-5-5","inference_geo":"global","upstream":1,"unsent":0,"checks":0,"counts":0,"sessions":1,"usage":{"input_tokens":10,"output_tokens":20}},` +
		`{"model":"claude-opus-5-5","inference_geo":"us","upstream":2,"unsent":0,"checks":0,"counts":0,"sessions":2,"usage":{"input_tokens":20,"output_tokens":40}}],` +
		`"sessions":2,"moved_on":0,"moved_off":0}]}`
	if got != want {
		t.Errorf("the summary is\n%s\nwant\n%s: a model's requests kept apart by the inference geo they asked for, none first", got, want)
	}
}

func TestASummarySumsAnAnswersIterationsByType(t *testing.T) {
	answered := func(request, iterations string) ledger.Line {
		return ledger.Line{At: on(0, 9, 0), Request: request, Kind: ledger.KindMessage, Session: "one", Model: opus, Account: "work", Reason: "sticky",
			Status: 200, Attempts: 1, Usage: usage(`{"input_tokens":20,"output_tokens":7,"iterations":` + iterations + `,"service_tier":"standard"}`)}
	}
	lines := []ledger.Line{
		answered("1", `[{"type":"message","input_tokens":8,"output_tokens":3},{"type":"message","input_tokens":12,"output_tokens":4}]`),
		answered("2", `[{"type":"message","input_tokens":20,"output_tokens":7},`+
			`{"type":"advisor_message","model":"claude-opus-5","input_tokens":823,"output_tokens":1612},{"untyped":1},"text"]`),
	}

	got := string(summaryJSON(t, noReadings, lines...))
	want := `{"version":1,"day":"2026-10-05","accounts":[{"account":"work","models":[{"model":"claude-opus-5-5","upstream":2,"unsent":0,"checks":0,` +
		`"counts":0,"sessions":1,"usage":{"input_tokens":40,"iterations":{"advisor_message":{"input_tokens":823,"output_tokens":1612},` +
		`"message":{"input_tokens":40,"output_tokens":14}},"output_tokens":14}}],"sessions":1,"moved_on":0,"moved_off":0}]}`
	if got != want {
		t.Errorf("the summary is\n%s\nwant\n%s: the iterations' counts summed by their types, what gives none passed over", got, want)
	}
}

func TestASummaryHoldsTheSessionsMovedOntoAndOffEachAccount(t *testing.T) {
	moved := func(request, session, from, to string) ledger.Line {
		return ledger.Line{At: on(0, 12, 0), Request: request, Kind: ledger.KindMessage, Session: session, Model: opus, Account: to,
			Reason: "moved: " + from + " hit its limit", From: from, Status: 200, Attempts: 2}
	}
	lines := []ledger.Line{
		moved("1", "one", "work", "side"),
		{At: on(0, 12, 1), Request: "2", Kind: ledger.KindMessage, Session: "one", Model: opus, Account: "side", Reason: "sticky", Status: 200, Attempts: 1},
		// The session's other model's requests, which moved it again.
		moved("3", "one", "work", "side"),
		moved("4", "two", "side", "work"),
		// From an account no request was answered on that day.
		moved("5", "three", "gone", "personal"),
	}
	onOpus := func(upstream int) []ledger.ModelDay {
		return []ledger.ModelDay{{Model: opus, Upstream: upstream, Sessions: 1}}
	}

	got := summarised(t, noReadings, lines...)
	want := ledger.Summary{Version: 1, Day: date, Accounts: []ledger.AccountDay{
		{Account: "gone", MovedOff: 1},
		{Account: "personal", Models: onOpus(1), Sessions: 1, MovedOn: 1},
		{Account: "side", Models: onOpus(3), Sessions: 1, MovedOn: 1, MovedOff: 1},
		{Account: "work", Models: onOpus(1), Sessions: 1, MovedOn: 1, MovedOff: 1},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("the summary is\n%+v\nwant\n%+v: each session moved counted once on the account it moved onto, and once on the one it moved off", got, want)
	}
}

func TestASummaryHoldsEachWindowsHighestUse(t *testing.T) {
	tests := []struct {
		name     string
		readings []readings.Reading
		want     map[string]float64
	}{
		{
			name: "the highest the day read",
			readings: []readings.Reading{
				workRead(on(0, 9, 0), "5h", 0.2, on(0, 13, 0), quota.StatusAllowed),
				workRead(on(0, 10, 0), "5h", 0.5, on(0, 13, 0), quota.StatusAllowed),
				workRead(on(0, 14, 0), "5h", 0.3, on(0, 18, 0), quota.StatusAllowed),
			},
			want: map[string]float64{"5h": 0.5},
		},
		{
			name: "the use the window began the day at, higher than it read after it reset that day",
			readings: []readings.Reading{
				workRead(on(-1, 22, 0), "5h", 0.8, on(0, 2, 0), quota.StatusAllowed),
				workRead(on(0, 3, 0), "5h", 0.1, on(0, 8, 0), quota.StatusAllowed),
				workRead(on(0, 6, 0), "5h", 0.5, on(0, 8, 0), quota.StatusAllowed),
			},
			want: map[string]float64{"5h": 0.8},
		},
		{
			name: "not the use read before the day, as the window had reset by the time it began",
			readings: []readings.Reading{
				workRead(on(-1, 18, 0), "5h", 0.9, on(-1, 23, 0), quota.StatusAllowed),
				workRead(on(0, 10, 0), "5h", 0.2, on(0, 15, 0), quota.StatusAllowed),
			},
			want: map[string]float64{"5h": 0.2},
		},
		{
			name: "the use the window began the day at, of one read days before and not since",
			readings: []readings.Reading{
				workRead(on(-3, 12, 0), "7d", 0.41, on(2, 10, 0), quota.StatusAllowed),
				workRead(on(0, 10, 0), "5h", 0.2, on(0, 15, 0), quota.StatusAllowed),
			},
			want: map[string]float64{"5h": 0.2, "7d": 0.41},
		},
		{
			name: "the use the window last read before the day, after it was reset by hand",
			readings: []readings.Reading{
				workRead(on(-2, 12, 0), "7d", 0.5, on(2, 10, 0), quota.StatusAllowed),
				workRead(on(-1, 16, 0), "7d", 0, on(2, 10, 0), quota.StatusAllowed),
			},
			want: map[string]float64{"7d": 0},
		},
		{
			name: "none of a window that began the day empty and wasn't read that day",
			readings: []readings.Reading{
				workRead(on(-1, 9, 0), "5h", 0.6, on(-1, 13, 0), quota.StatusAllowed),
				workRead(on(0, 10, 0), "7d", 0.3, on(2, 10, 0), quota.StatusAllowed),
			},
			want: map[string]float64{"7d": 0.3},
		},
		{
			name: "none read after the day",
			readings: []readings.Reading{
				workRead(on(0, 23, 0), "5h", 0.2, on(1, 3, 0), quota.StatusAllowed),
				workRead(on(1, 0, 0), "5h", 0.9, on(1, 3, 0), quota.StatusAllowed),
			},
			want: map[string]float64{"5h": 0.2},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summarised(t, readingsOf(tt.readings...))
			want := ledger.Summary{Version: 1, Day: date, Accounts: []ledger.AccountDay{{Account: "work", Highest: tt.want}}}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("the summary is\n%+v\nwant\n%+v", got, want)
			}
		})
	}
}

func TestASummaryHoldsTheLimitsEachAccountReached(t *testing.T) {
	rejected := func(at time.Time, resets time.Time) readings.Reading {
		return workRead(at, "5h", 1, resets, quota.StatusRejected)
	}
	allowed := func(at time.Time, u float64, resets time.Time) readings.Reading {
		return workRead(at, "5h", u, resets, quota.StatusAllowed)
	}
	reached := func(at, resets time.Time) ledger.Limit {
		return ledger.Limit{Window: "5h", At: at.UTC(), ResetsAt: resets.UTC()}
	}
	tests := []struct {
		name     string
		readings []readings.Reading
		want     []ledger.Limit
	}{
		{
			name:     "a window's status turning rejected",
			readings: []readings.Reading{allowed(on(0, 15, 0), 0.9, on(0, 18, 10)), rejected(on(0, 15, 37), on(0, 18, 10))},
			want:     []ledger.Limit{reached(on(0, 15, 37), on(0, 18, 10))},
		},
		{
			name: "once, rejected again while the limit holds",
			readings: []readings.Reading{allowed(on(0, 15, 0), 0.9, on(0, 18, 10)), rejected(on(0, 15, 37), on(0, 18, 10)),
				rejected(on(0, 16, 0), on(0, 18, 10))},
			want: []ledger.Limit{reached(on(0, 15, 37), on(0, 18, 10))},
		},
		{
			name: "again, in the window after its reset",
			readings: []readings.Reading{rejected(on(0, 15, 37), on(0, 18, 10)), allowed(on(0, 18, 20), 0.05, on(0, 23, 20)),
				rejected(on(0, 22, 0), on(0, 23, 20))},
			want: []ledger.Limit{reached(on(0, 15, 37), on(0, 18, 10)), reached(on(0, 22, 0), on(0, 23, 20))},
		},
		{
			name:     "again, in the window after its reset, nothing read between",
			readings: []readings.Reading{rejected(on(0, 15, 37), on(0, 18, 10)), rejected(on(0, 22, 0), on(0, 23, 20))},
			want:     []ledger.Limit{reached(on(0, 15, 37), on(0, 18, 10)), reached(on(0, 22, 0), on(0, 23, 20))},
		},
		{
			name: "again, once the window was reset by hand",
			readings: []readings.Reading{rejected(on(0, 15, 37), on(0, 18, 10)), allowed(on(0, 16, 0), 0, on(0, 18, 10)),
				rejected(on(0, 17, 30), on(0, 18, 10))},
			want: []ledger.Limit{reached(on(0, 15, 37), on(0, 18, 10)), reached(on(0, 17, 30), on(0, 18, 10))},
		},
		{
			name:     "none the day began with",
			readings: []readings.Reading{rejected(on(-1, 22, 0), on(0, 2, 0)), rejected(on(0, 1, 0), on(0, 2, 0))},
		},
		{
			name:     "one reached in the day, once a limit before it had reset by the day's start",
			readings: []readings.Reading{rejected(on(-1, 18, 0), on(-1, 23, 0)), rejected(on(0, 10, 0), on(0, 12, 0))},
			want:     []ledger.Limit{reached(on(0, 10, 0), on(0, 12, 0))},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := summarised(t, readingsOf(tt.readings...))
			if len(got.Accounts) != 1 || !slices.EqualFunc(got.Accounts[0].Limits, tt.want, sameLimit) {
				t.Errorf("the summary is\n%+v\nwant work's limits %+v", got, tt.want)
			}
		})
	}
}

// sameLimit reports whether a and b are the same limit, reached at the same
// time.
func sameLimit(a, b ledger.Limit) bool {
	return a.Window == b.Window && a.At.Equal(b.At) && a.ResetsAt.Equal(b.ResetsAt)
}

func TestASummaryAsksForTheReadingsOfItsDayAndTheWeekBefore(t *testing.T) {
	var from, to time.Time
	history := func(f, t time.Time) iter.Seq[readings.Reading] {
		from, to = f, t
		return noReadings(f, t)
	}

	if _, err := ledger.Summarise(date, slices.Values([]ledger.Line(nil)), history); err != nil {
		t.Fatalf("Summarise() error = %v", err)
	}
	// A week, as long as the longest window runs, before the day's start.
	weekBefore := on(0, 0, 0).Add(-7 * 24 * time.Hour)
	if !from.Equal(weekBefore) || !to.Equal(on(1, 0, 0)) {
		t.Errorf("asked for the readings from %v to %v, want from a week before the day's start to its end, %v to %v", from, to, weekBefore, on(1, 0, 0))
	}
}

func TestADateThatIsntOneIsntSummarised(t *testing.T) {
	if _, err := ledger.Summarise("2026-10-32", slices.Values([]ledger.Line(nil)), noReadings); err == nil {
		t.Error("Summarise() of 2026-10-32 succeeded, want it to fail: it isn't a date")
	}
}

// asked is a request of session one's answered on work at at, as the ledger
// holds it.
func asked(request string, at time.Time) ledger.Line {
	return ledger.Line{At: at, Request: request, Kind: ledger.KindMessage, Session: "one", Model: opus, Account: "work", Reason: "sticky",
		Status: 200, Attempts: 1, TotalMS: 1000, Usage: usage(`{"input_tokens":10,"output_tokens":20}`)}
}

// summaryOfTwo is the summary the ledger writes of the summaries' day, of two
// of the requests asked makes, and of the windows' highest use, highest, as
// its JSON gives it, where it's given.
func summaryOfTwo(highest string) string {
	return `{"version":1,"day":"2026-10-05","accounts":[{"account":"work","models":[{"model":"claude-opus-5-5","upstream":2,"unsent":0,"checks":0,` +
		`"counts":0,"sessions":1,"usage":{"input_tokens":20,"output_tokens":40}}],"sessions":1,"moved_on":0,"moved_off":0` + highest + `}]}` + "\n"
}

// holdLines adds lines, as the ledger writes them, to its plain file in dir
// of the local day with the given date.
func holdLines(t *testing.T, dir, date string, lines ...ledger.Line) {
	t.Helper()
	var data []byte
	for _, line := range lines {
		written, err := json.Marshal(line)
		if err != nil {
			t.Fatal(err)
		}
		data = append(append(data, written...), '\n')
	}
	file, err := os.OpenFile(filepath.Join(dir, "requests-"+date+".jsonl"), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

// summaryFile is where the ledger in dir keeps the summary of the local day
// with the given date.
func summaryFile(dir, date string) string {
	return filepath.Join(dir, "day-"+date+".json")
}

// heldSummary returns the summary the ledger in dir holds of the local day
// with the given date, as it's written: "" where it holds none.
func heldSummary(t *testing.T, dir, date string) string {
	t.Helper()
	data, err := os.ReadFile(summaryFile(dir, date))
	if errors.Is(err, fs.ErrNotExist) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// clockFrom returns a clock that reads from as a synctest bubble's time
// starts, and goes on as its time does.
func clockFrom(from time.Time) func() time.Time {
	began := time.Now()
	return func() time.Time { return from.Add(time.Since(began)) }
}

func TestADayIsSummarisedOnTheFirstRoundAnHourAfterItEndsAndTodayNever(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logstest.Capture(t)
		dir := t.TempDir()
		// The day's last request arrived as it ended.
		holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 23, 59)))
		holdLines(t, dir, "2026-10-06", asked("3", on(1, 0, 5)))
		readings := readingsOf(workRead(on(0, 9, 0), "5h", 0.25, on(0, 13, 0), quota.StatusAllowed))
		// Ten minutes after the day ended.
		l := ledger.Open(dir, 90*24*time.Hour, clockFrom(on(1, 0, 10)), readings, logs.For("router"))
		stop := running(t, l)
		defer stop()

		synctest.Wait()
		if got := heldSummary(t, dir, date); got != "" {
			t.Fatalf("the day was summarised ten minutes after it ended, as\n%s\nwant it left an hour, for the requests still in flight as it ended to end", got)
		}
		time.Sleep(time.Hour)
		synctest.Wait()
		stop()
		if got, want := heldSummary(t, dir, date), summaryOfTwo(`,"highest":{"5h":0.25}`); got != want {
			t.Errorf("an hour on, the day's summary is\n%s\nwant\n%s", got, want)
		}
		if info, err := os.Stat(summaryFile(dir, date)); err != nil || info.Mode() != 0o600 {
			t.Errorf("the day's summary is %v (%v), want it the user's alone, %v", info.Mode(), err, fs.FileMode(0o600))
		}
		if got := heldSummary(t, dir, "2026-10-06"); got != "" {
			t.Errorf("today was summarised, as\n%s\nwant it never, as it hasn't ended", got)
		}
		if !log.Has("level=INFO", `msg="summarised a day of the request ledger"`, "day="+date, "requests=2") {
			t.Errorf("log reads\n%s\nwant the day summarised noted, with its requests", log)
		}
	})
}

func TestADayIsSummarisedOnceNeverAgain(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logstest.Capture(t)
		dir := t.TempDir()
		holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0)))
		l := ledger.Open(dir, 90*24*time.Hour, clockFrom(on(1, 10, 0)), noReadings, logs.For("router"))
		stop := running(t, l)
		defer stop()

		synctest.Wait()
		// A line filed under the day after it was summarised, as of a request
		// in flight past its summary.
		holdLines(t, dir, date, asked("3", on(0, 23, 59)))
		time.Sleep(3 * time.Hour)
		synctest.Wait()
		stop()
		if got, want := heldSummary(t, dir, date), summaryOfTwo(""); got != want {
			t.Errorf("the day's summary is\n%s\nwant it as first written, of its two requests\n%s", got, want)
		}
		if summarised := strings.Count(log.String(), `msg="summarised a day of the request ledger"`); summarised != 1 {
			t.Errorf("log reads\n%s\nwant the day summarised once", log)
		}
	})
}

func TestADayTheRouterWasStoppedAsItEndedIsSummarisedByItsNextRound(t *testing.T) {
	dir := t.TempDir()
	first, second := asked("1", on(0, 23, 40)), asked("2", on(0, 23, 45))
	// The router stops ten minutes before the day ends, and starts again two
	// days on.
	writeAt(t, dir, on(0, 23, 50), noReadings, &first, &second)
	if got := heldSummary(t, dir, date); got != "" {
		t.Fatalf("the day was summarised before it ended, as\n%s\nwant it left until it had", got)
	}

	writeAt(t, dir, on(2, 9, 0), noReadings)
	if got, want := heldSummary(t, dir, date), summaryOfTwo(""); got != want {
		t.Errorf("the day's summary is\n%s\nwant it written as the router started again\n%s", got, want)
	}
	if got := heldSummary(t, dir, "2026-10-06"); got != "" {
		t.Errorf("the day after was summarised, as\n%s\nwant none: the ledger holds no lines of it", got)
	}
}

func TestADayWhoseLinesAreCompressedIsSummarised(t *testing.T) {
	dir := t.TempDir()
	var lines bytes.Buffer
	for _, line := range []ledger.Line{asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0))} {
		if err := json.NewEncoder(&lines).Encode(line); err != nil {
			t.Fatal(err)
		}
	}
	// Compressed by a router from before the ledger was summarised.
	var compressed bytes.Buffer
	w := gzip.NewWriter(&compressed)
	if _, err := w.Write(lines.Bytes()); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "requests-"+date+".jsonl.gz"), compressed.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}

	writeAt(t, dir, on(4, 9, 0), noReadings)
	if got, want := heldSummary(t, dir, date), summaryOfTwo(""); got != want {
		t.Errorf("the day's summary is\n%s\nwant\n%s", got, want)
	}
}

func TestLinesThatDontReadArePassedOverAsADayIsSummarised(t *testing.T) {
	log := logstest.Capture(t)
	dir := t.TempDir()
	holdLines(t, dir, date, asked("1", on(0, 9, 0)))
	damaged := `{"at":"2026-10-05T09:0` + "\n" + "not a line\n"
	held := filepath.Join(dir, "requests-"+date+".jsonl")
	file, err := os.OpenFile(held, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(damaged); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	holdLines(t, dir, date, asked("2", on(0, 10, 0)))
	// The last line torn, as a crash partway through its write leaves it.
	file, err = os.OpenFile(held, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(`{"at":"2026-10-05T23:59:59Z","request":"3","ki`); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	writeAt(t, dir, on(1, 9, 0), noReadings)
	if got, want := heldSummary(t, dir, date), summaryOfTwo(""); got != want {
		t.Errorf("the day's summary is\n%s\nwant it of the two lines that read\n%s", got, want)
	}
	if !log.Has("level=WARN", `msg="request ledger lines unread"`, "day="+date, "lines=3") {
		t.Errorf("log reads\n%s\nwant the three lines that don't read noted", log)
	}
}

func TestADayNoneOfWhoseLinesReadIsLeftForARoundThatFindsThem(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logstest.Capture(t)
		dir := t.TempDir()
		holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0)))
		// A day whose only line was torn as it was written.
		torn := "2026-10-04"
		if err := os.WriteFile(filepath.Join(dir, "requests-"+torn+".jsonl"), []byte(`{"at":"2026-10-04T09:00:00Z","requ`), 0o600); err != nil {
			t.Fatal(err)
		}
		held := filepath.Join(dir, "requests-"+date+".jsonl")
		// The day's file can't be read as its first round comes.
		if err := os.Chmod(held, 0o000); err != nil {
			t.Fatal(err)
		}
		l := ledger.Open(dir, 90*24*time.Hour, clockFrom(on(1, 9, 0)), noReadings, logs.For("router"))
		stop := running(t, l)
		defer stop()

		synctest.Wait()
		if got := heldSummary(t, dir, date); got != "" {
			t.Fatalf("the day was summarised as its file couldn't be read, as\n%s\nwant it left for a round that reads it", got)
		}
		if !log.Has("level=WARN", `msg="can't read the request ledger"`, "file=requests-"+date+".jsonl") {
			t.Errorf("log reads\n%s\nwant the file that can't be read noted", log)
		}
		if err := os.Chmod(held, 0o600); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Hour)
		synctest.Wait()
		stop()
		if got, want := heldSummary(t, dir, date), summaryOfTwo(""); got != want {
			t.Errorf("at the round after, the day's summary is\n%s\nwant\n%s", got, want)
		}
		if got := heldSummary(t, dir, torn); got != "" {
			t.Errorf("the day of a torn line alone was summarised, as\n%s\nwant it left, as none of its lines read", got)
		}
	})
}

func TestASummaryThatCantBeWrittenIsLoggedAndWrittenOnALaterRound(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logstest.Capture(t)
		dir := t.TempDir()
		holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0)))
		l := ledger.Open(dir, 90*24*time.Hour, clockFrom(on(1, 0, 30)), noReadings, logs.For("router"))
		stop := running(t, l)
		defer stop()
		synctest.Wait()
		// Nothing can be written in the ledger's directory by the time the day
		// is to be summarised.
		if err := os.Chmod(dir, 0o500); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

		time.Sleep(time.Hour)
		synctest.Wait()
		if !log.Has("level=WARN", `msg="can't summarise the request ledger"`, "day="+date) {
			t.Errorf("log reads\n%s\nwant the summary that couldn't be written noted", log)
		}
		if got := heldSummary(t, dir, date); got != "" {
			t.Fatalf("the day's summary is\n%s\nwant none written", got)
		}
		if err := os.Chmod(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Hour)
		synctest.Wait()
		stop()
		if got, want := heldSummary(t, dir, date), summaryOfTwo(""); got != want {
			t.Errorf("at the round after, the day's summary is\n%s\nwant\n%s", got, want)
		}
	})
}
