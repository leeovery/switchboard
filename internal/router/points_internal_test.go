package router

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
)

// chartedAt is the time GET /history is asked at in tests: a Monday, 14:00
// UTC, 50 minutes into session's window, which started at 13:10.
var chartedAt = time.Date(2026, 9, 28, 14, 0, 0, 0, time.UTC)

// weekStarted is when week's window started: a whole week before its reset.
var weekStarted = time.Date(2026, 9, 25, 21, 0, 0, 0, time.UTC)

func TestGETHistoryTakesAWindowAndAStep(t *testing.T) {
	const tokenShaped = "sk-ant-oat01-fake_token-shaped"
	tests := []struct {
		name  string
		query string
		// wantErr is why the request is refused, "" when it's answered.
		wantErr string
	}{
		{name: "without a window", query: "step=5m", wantErr: "give the window, such as window=5h"},
		{name: "a window whose length can't be read", query: "window=week&step=5m", wantErr: `window "week" has no length to read: give one such as 5h or 7d`},
		{name: "a token given as the window", query: "window=" + tokenShaped + "&step=5m", wantErr: `window "[redacted]" has no length to read: give one such as 5h or 7d`},
		{name: "without a step", query: "window=5h", wantErr: "give the step between points, such as step=5m"},
		{name: "a step that isn't a duration", query: "window=5h&step=soon", wantErr: `step "soon" isn't a duration, such as 5m`},
		{name: "a token given as the step", query: "window=5h&step=" + tokenShaped, wantErr: `step "[redacted]" isn't a duration, such as 5m`},
		{name: "a step of nothing", query: "window=5h&step=0s", wantErr: "step 0s isn't more than 0"},
		{name: "a step less than nothing", query: "window=5h&step=-5m", wantErr: "step -5m isn't more than 0"},
		{
			name:    "a step the week would take more than 1,000 of",
			query:   "window=7d&step=10m",
			wantErr: "step 10m is too short for window 7d: its whole length would take more than 1000 steps, so give 10m4.8s or more",
		},
		{
			name:    "a step too short for a window whose key holds a token",
			query:   "window=7d_" + tokenShaped + "&step=10m",
			wantErr: "step 10m is too short for window 7d_[redacted]: its whole length would take more than 1000 steps, so give 10m4.8s or more",
		},
		{name: "a step the week takes 1,000 of", query: "window=7d&step=10m4.8s"},
		{name: "a step the session takes 1,000 of", query: "window=5h&step=18s"},
		{name: "a step past the session's whole length", query: "window=5h&step=6h"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newTestRouter(t, at(chartedAt), &stubProber{})

			code, body := askHistory(t, r, tt.query)
			if tt.wantErr == "" {
				if code != http.StatusOK {
					t.Errorf("GET /history?%s answered %d %s, want 200", tt.query, code, body)
				}
				return
			}
			want, _ := json.Marshal(map[string]string{"error": tt.wantErr})
			if code != http.StatusBadRequest || body != string(want)+"\n" {
				t.Errorf("GET /history?%s answered %d %s, want 400 %s", tt.query, code, body, want)
			}
		})
	}
}

func TestGETHistoryGivesEachStepTheLastReadingAtOrBeforeIt(t *testing.T) {
	clock := &testClock{}
	r, _ := newChartedRouter(t, clock)
	// Written out of the order they were read in, as after the clock was set
	// back.
	writeLines(t, r.history,
		lineOf("work", using(session, 0.2), at13(31)),
		lineOf("work", using(session, 0.1), at13(22)),
		lineOf("work", using(session, 0.25), at13(40)),
	)
	readAs(r, clock, "work", chartedAt, using(session, 0.3))

	got := historyAnswer(t, r, "window=5h&step=5m")
	// The window started at 13:10, and was first read at 13:22.
	want := AccountHistory{ID: "work", Start: at13(10), Points: []HistoryPoint{
		{At: at13(25), Utilization: 0.1},
		{At: at13(30), Utilization: 0.1},
		{At: at13(35), Utilization: 0.2},
		{At: at13(40), Utilization: 0.25},
		{At: at13(45), Utilization: 0.25},
		{At: at13(50), Utilization: 0.25},
		{At: at13(55), Utilization: 0.25},
		{At: chartedAt, Utilization: 0.3},
	}}
	if !reflect.DeepEqual(got.Accounts[0], want) {
		t.Errorf("work's history is\n%+v\nwant\n%+v", got.Accounts[0], want)
	}
}

func TestGETHistoryPlacesEachWindowAsItRunsNow(t *testing.T) {
	noReset := session
	noReset.ResetsAt = time.Time{}
	lapsed := session
	lapsed.ResetsAt = at13(0)
	passed := week
	passed.ResetsAt = at13(30)
	previous := session
	previous.ResetsAt = at13(10)
	tests := []struct {
		name  string
		query string
		// taken are the readings the router takes in of work, in order, and
		// lines those its history's files hold.
		taken []reading
		lines []reading
		want  AccountHistory
	}{
		{
			name:  "reset by hand, from when it started again",
			query: "window=5h&step=10m",
			taken: []reading{lineOf("work", using(session, 0.6), at13(30)), lineOf("work", using(session, 0.05), at13(40))},
			lines: []reading{
				lineOf("work", using(session, 0.5), at13(20)),
				lineOf("work", using(session, 0.6), at13(30)),
				lineOf("work", using(session, 0.05), at13(40)),
			},
			want: AccountHistory{ID: "work", Start: at13(40), Points: []HistoryPoint{
				{At: at13(40), Utilization: 0.05},
				{At: at13(50), Utilization: 0.05},
				{At: chartedAt, Utilization: 0.05},
			}},
		},
		{
			name:  "none of a session that has lapsed",
			query: "window=5h&step=10m",
			taken: []reading{lineOf("work", using(lapsed, 0.4), at13(0).Add(-time.Hour))},
			lines: []reading{lineOf("work", using(lapsed, 0.4), at13(0).Add(-time.Hour))},
			want:  AccountHistory{ID: "work"},
		},
		{
			name:  "none of a window read without a reset",
			query: "window=5h&step=10m",
			taken: []reading{lineOf("work", using(noReset, 0.4), at13(30))},
			lines: []reading{lineOf("work", using(noReset, 0.4), at13(30))},
			want:  AccountHistory{ID: "work"},
		},
		{
			name:  "none of a week past its reset",
			query: "window=7d&step=1h",
			taken: []reading{lineOf("work", using(passed, 0.4), at13(0))},
			lines: []reading{lineOf("work", using(passed, 0.4), at13(0))},
			want:  AccountHistory{ID: "work"},
		},
		{
			name:  "none of a window never read",
			query: "window=5h&step=10m",
			lines: []reading{lineOf("work", using(session, 0.4), at13(30))},
			want:  AccountHistory{ID: "work"},
		},
		{
			name:  "nothing of an earlier window, nor from before the window started",
			query: "window=5h&step=10m",
			taken: []reading{lineOf("work", using(session, 0.2), at13(50))},
			lines: []reading{
				lineOf("work", using(previous, 0.7), at13(0).Add(-30*time.Minute)),
				lineOf("work", using(noReset, 0.8), at13(5)),
				lineOf("work", using(previous, 0.9), at13(15)),
				lineOf("work", using(session, 0.1), at13(25)),
				lineOf("work", using(noReset, 0.15), at13(35)),
			},
			want: AccountHistory{ID: "work", Start: at13(10), Points: []HistoryPoint{
				{At: at13(30), Utilization: 0.1},
				{At: at13(40), Utilization: 0.15},
				{At: at13(50), Utilization: 0.2},
				{At: chartedAt, Utilization: 0.2},
			}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clock := &testClock{}
			r, _ := newChartedRouter(t, clock)
			writeLines(t, r.history, tt.lines...)
			for _, taken := range tt.taken {
				readAs(r, clock, taken.Account, taken.At, taken.window())
			}
			clock.now = chartedAt

			if got := historyAnswer(t, r, tt.query).Accounts[0]; !reflect.DeepEqual(got, tt.want) {
				t.Errorf("work's history is\n%+v\nwant\n%+v", got, tt.want)
			}
		})
	}
}

func TestGETHistoryReadsTheDaysTheWindowSpansAndTheReadingsHeldSince(t *testing.T) {
	clock := &testClock{}
	r, dir := newChartedRouter(t, clock)
	day := func(offset int, hour int) time.Time {
		return time.Date(2026, 9, 26+offset, hour, 0, 0, 0, time.UTC)
	}
	// Compressed days, the second left in both forms as the router stopped
	// compressing it, then a plain day.
	compressLines(t, dir,
		lineOf("work", using(week, 0.35), day(0, 12)),
		lineOf("work", using(week, 0.4), day(0, 14)),
	)
	stopped := lineOf("work", using(week, 0.5), day(1, 13))
	compressLines(t, dir, lineOf("work", using(week, 0.45), day(1, 12)), stopped)
	writeLines(t, r.history, stopped, lineOf("work", using(week, 0.55), day(2, 9)))
	// The reading taken in now is still queued, so its line isn't written.
	readAs(r, clock, "work", chartedAt, using(week, 0.6))

	got := historyAnswer(t, r, "window=7d&step=5h")
	want := AccountHistory{ID: "work", Start: weekStarted, Points: []HistoryPoint{
		{At: day(0, 12), Utilization: 0.35},
		{At: day(0, 17), Utilization: 0.4},
		{At: day(0, 22), Utilization: 0.4},
		{At: day(1, 3), Utilization: 0.4},
		{At: day(1, 8), Utilization: 0.4},
		{At: day(1, 13), Utilization: 0.5},
		{At: day(1, 18), Utilization: 0.5},
		{At: day(1, 23), Utilization: 0.5},
		{At: day(2, 4), Utilization: 0.5},
		{At: day(2, 9), Utilization: 0.55},
		{At: chartedAt, Utilization: 0.6},
	}}
	if !reflect.DeepEqual(got.Accounts[0], want) {
		t.Errorf("work's history is\n%+v\nwant\n%+v", got.Accounts[0], want)
	}
}

func TestGETHistoryGivesEveryAccountConfiguredInOrderInUTC(t *testing.T) {
	clock := &testClock{}
	r, _ := newChartedRouter(t, clock)
	// Work's session as an answer gave its reset, in a time zone of its own.
	elsewhere := session
	elsewhere.ResetsAt = session.ResetsAt.In(time.FixedZone("UTC+9", 9*60*60))
	writeLines(t, r.history,
		lineOf("gone", using(session, 0.9), at13(20)),
		lineOf("work", using(session, 0.1), at13(20)),
		lineOf("work", using(week, 0.95), at13(35)),
	)
	readAs(r, clock, "work", at13(50), using(elsewhere, 0.2))
	clock.now = chartedAt

	code, body := askHistory(t, r, "window=5h&step=10m")
	want := `{"window":"5h","step":"10m","accounts":[` +
		`{"id":"work","start":"2026-09-28T13:10:00Z","points":[` +
		`{"at":"2026-09-28T13:20:00Z","utilization":0.1},{"at":"2026-09-28T13:30:00Z","utilization":0.1},` +
		`{"at":"2026-09-28T13:40:00Z","utilization":0.1},{"at":"2026-09-28T13:50:00Z","utilization":0.2},` +
		`{"at":"2026-09-28T14:00:00Z","utilization":0.2}]},` +
		`{"id":"personal"},{"id":"side"}]}` + "\n"
	if code != http.StatusOK || body != want {
		t.Errorf("GET /history answered %d\n%s\nwant 200\n%s", code, body, want)
	}
}

func TestAWindowsReadingsComeOnceEachFromTheDaysAroundItsSpan(t *testing.T) {
	clock := &testClock{}
	r, dir := newChartedRouter(t, clock)
	date := func(t time.Time, offset int) string { return t.Local().AddDate(0, 0, offset).Format(time.DateOnly) }
	read := func(u float64) reading { return lineOf("work", using(week, u), weekStarted.Add(time.Hour)) }
	// Files named for the days around the week's span, as a change of time
	// zone can name them, hold readings of other days.
	writeDay(t, dir, plainFile(date(weekStarted, -2)), linesOf(t, read(0.05)))
	writeDay(t, dir, plainFile(date(weekStarted, -1)), linesOf(t, read(0.1)))
	writeDay(t, dir, plainFile(date(chartedAt, 1)), linesOf(t, read(0.6)))
	writeDay(t, dir, plainFile(date(chartedAt, 2)), linesOf(t, read(0.65)))
	noon := func(day int, minute int) time.Time { return time.Date(2026, 9, day, 12, minute, 0, 0, time.UTC) }
	compressLines(t, dir, lineOf("work", using(week, 0.35), noon(26, 0)), lineOf("work", using(week, 0.4), noon(26, 5)))
	// The router stopped compressing the day after, before its plain file
	// went.
	stopped := lineOf("work", using(week, 0.5), noon(27, 5))
	compressLines(t, dir, lineOf("work", using(week, 0.45), noon(27, 0)), stopped)
	writeLines(t, r.history,
		stopped,
		lineOf("work", using(week, 0.55), noon(28, 0)),
		lineOf("work", using(session, 0.9), noon(28, 0)),
		lineOf("gone", using(week, 0.9), noon(28, 0)),
	)

	got := r.history.windowReadings("7d", []string{"work"}, weekStarted, chartedAt)
	var uses []float64
	for _, line := range got["work"] {
		uses = append(uses, line.Utilization)
	}
	if want := []float64{0.1, 0.35, 0.4, 0.45, 0.5, 0.55, 0.6}; !slices.Equal(uses, want) || len(got) != 1 {
		t.Errorf("read the uses %v of %d accounts, want work's alone, %v: those of the days from the one before the week started to the one after now, each once, in the order they came", uses, len(got), want)
	}
}

// askHistory asks the router's control API for GET /history with the given
// query, and returns the status it answers with, and what it answers.
func askHistory(t *testing.T, r *Router, query string) (int, string) {
	t.Helper()
	rec := httptest.NewRecorder()
	r.Control().ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/history?"+query, nil))
	return rec.Code, rec.Body.String()
}

// historyAnswer asks the router's control API for GET /history with the
// given query, which it must answer, and returns what it answers.
func historyAnswer(t *testing.T, r *Router, query string) History {
	t.Helper()
	code, body := askHistory(t, r, query)
	if code != http.StatusOK {
		t.Fatalf("GET /history?%s answered %d %s, want 200", query, code, body)
	}
	var h History
	if err := json.Unmarshal([]byte(body), &h); err != nil {
		t.Fatal(err)
	}
	return h
}

// newChartedRouter builds a router of testConfigured on clock's time, its
// readings history kept in a directory of the test's, which it returns, with
// nothing writing it: what the router takes in is held in memory alone.
func newChartedRouter(t *testing.T, clock *testClock) (*Router, string) {
	t.Helper()
	r := newTestRouter(t, clock.read, &stubProber{})
	dir := t.TempDir()
	r.history.open(dir)
	return r, dir
}

// readAs has the router take in w, a window of the account with the given id,
// as an answer read it at at, its clock then at at.
func readAs(r *Router, clock *testClock, id string, at time.Time, w quota.Window) {
	clock.now = at
	r.state.record(id, []quota.Window{w}, r.state.mark())
}

// using is w, read using u of it.
func using(w quota.Window, u float64) quota.Window {
	w.Utilization = u
	return w
}

// lineOf is the history's line of w, a window of the account with the given
// id, as an answer read it at at.
func lineOf(id string, w quota.Window, at time.Time) reading {
	return readingsOf(id, []quota.Window{w}, at.UTC(), fromAnswer)[0]
}

// at13 is the given minute past 13:00 on chartedAt's day.
func at13(minute int) time.Time {
	return time.Date(2026, 9, 28, 13, minute, 0, 0, time.UTC)
}

// writeLines appends lines to the plain files of the history h, each to its
// local day's, as the router does.
func writeLines(t *testing.T, h *history, lines ...reading) {
	t.Helper()
	if err := h.files.Append(h.lines(lines)); err != nil {
		t.Fatal(err)
	}
}

// compressLines adds lines to the compressed files of the history in dir,
// each local day's to its own, as a member of their own after any the file
// holds, as compressing their plain files would: where the time zone puts the
// lines of two calls on one day, it holds both.
func compressLines(t *testing.T, dir string, lines ...reading) {
	t.Helper()
	byDay := make(map[string][]reading)
	for _, l := range lines {
		date := l.At.Local().Format(time.DateOnly)
		byDay[date] = append(byDay[date], l)
	}
	for date, day := range byDay {
		path := filepath.Join(dir, compressedFile(date).name())
		held, err := os.ReadFile(path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(held, gzipOf(t, linesOf(t, day...))...), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
