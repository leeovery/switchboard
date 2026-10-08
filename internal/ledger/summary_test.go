package ledger_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
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

// dayDone is when the summaries' day is summarised, as the router does,
// an hour after it ended.
var dayDone = on(1, 1, 0)

// caps are the caps the tests' days are summarised with, unless a test says
// otherwise: no account keeps a reserve, and the windows every model shares
// are the session and the week.
var caps = ledger.Caps{Shared: []string{"5h", "7d"}}

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
	s, err := ledger.Summarise(date, slices.Values(lines), readings, nil, caps, dayDone)
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
	none := windowsRead{}.json()
	want := `{"version":2,"day":"2026-10-05","lines":8,"accounts":[` +
		`{"models":[{"upstream":0,"no_usage":0,"unsent":1,"checks":0,"counts":0,"sessions":1},` +
		`{"model":"claude-opus-5-5","upstream":0,"no_usage":0,"unsent":1,"checks":0,"counts":0,"sessions":1}],"sessions":2,"session_ids":["four","three"],` +
		`"moved_on":0,"moved_off":0` + none + `},` +
		`{"account":"side","models":[{"model":"claude-opus-5-5","upstream":1,"no_usage":0,"unsent":0,"checks":0,"counts":0,"sessions":1,` +
		`"usage":{"input_tokens":5,"output_tokens":50}}],"sessions":1,"session_ids":["two"],"moved_on":0,"moved_off":0` + none + `},` +
		`{"account":"work","models":[{"model":"claude-haiku-4-5","upstream":0,"no_usage":0,"unsent":0,"checks":1,"counts":0,"sessions":0,` +
		`"usage":{"input_tokens":8,"output_tokens":1}},` +
		`{"model":"claude-opus-5-5","upstream":3,"no_usage":1,"unsent":0,"checks":0,"counts":1,"sessions":1,` +
		`"usage":{"cache_creation":{"ephemeral_1h_input_tokens":3120,"ephemeral_5m_input_tokens":0},"cache_creation_input_tokens":3120,` +
		`"cache_read_input_tokens":367800,"input_tokens":15,"output_tokens":965,"server_tool_use":{"web_search_requests":2}}}],` +
		`"sessions":1,"session_ids":["one"],"moved_on":0,"moved_off":0` + none + `}]}`
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
	want := `{"version":2,"day":"2026-10-05","lines":5,"accounts":[{"account":"work","models":[` +
		`{"model":"claude-haiku-4-5","upstream":1,"no_usage":1,"unsent":0,"checks":0,"counts":0,"sessions":1},` +
		`{"model":"claude-opus-5-5","upstream":1,"no_usage":0,"unsent":0,"checks":0,"counts":0,"sessions":1,"usage":{"input_tokens":10,"output_tokens":20}},` +
		`{"model":"claude-opus-5-5","inference_geo":"global","upstream":1,"no_usage":0,"unsent":0,"checks":0,"counts":0,"sessions":1,"usage":{"input_tokens":10,"output_tokens":20}},` +
		`{"model":"claude-opus-5-5","inference_geo":"us","upstream":2,"no_usage":0,"unsent":0,"checks":0,"counts":0,"sessions":2,"usage":{"input_tokens":20,"output_tokens":40}}],` +
		`"sessions":2,"session_ids":["one","two"],"moved_on":0,"moved_off":0` + windowsRead{}.json() + `}]}`
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
	want := `{"version":2,"day":"2026-10-05","lines":2,"accounts":[{"account":"work","models":[{"model":"claude-opus-5-5","upstream":2,"no_usage":0,"unsent":0,"checks":0,` +
		`"counts":0,"sessions":1,"usage":{"input_tokens":40,"iterations":{"advisor_message":{"input_tokens":823,"output_tokens":1612},` +
		`"message":{"input_tokens":40,"output_tokens":14}},"output_tokens":14}}],"sessions":1,"session_ids":["one"],"moved_on":0,"moved_off":0` +
		windowsRead{}.json() + `}]}`
	if got != want {
		t.Errorf("the summary is\n%s\nwant\n%s: the iterations' counts summed by their types, what gives none passed over", got, want)
	}
}

func TestASummaryHoldsTheSessionsMovedOntoAndOffEachAccount(t *testing.T) {
	answer := usage(`{"input_tokens":10,"output_tokens":20}`)
	moved := func(request, session, from, to string) ledger.Line {
		return ledger.Line{At: on(0, 12, 0), Request: request, Kind: ledger.KindMessage, Session: session, Model: opus, Account: to,
			Reason: "moved: " + from + " hit its limit", From: from, Status: 200, Attempts: 2, Usage: answer}
	}
	lines := []ledger.Line{
		moved("1", "one", "work", "side"),
		{At: on(0, 12, 1), Request: "2", Kind: ledger.KindMessage, Session: "one", Model: opus, Account: "side", Reason: "sticky", Status: 200, Attempts: 1,
			Usage: answer},
		// The session's other model's requests, which moved it again.
		moved("3", "one", "work", "side"),
		moved("4", "two", "side", "work"),
		// From an account no request was answered on that day.
		moved("5", "three", "gone", "personal"),
	}
	onOpus := func(upstream int) []ledger.ModelDay {
		return []ledger.ModelDay{{Model: opus, Upstream: upstream, Sessions: 1,
			Usage: usage(fmt.Sprintf(`{"input_tokens":%d,"output_tokens":%d}`, 10*upstream, 20*upstream))}}
	}

	got := summarised(t, noReadings, lines...)
	want := ledger.Summary{Version: 2, Day: date, Lines: 5, Accounts: []ledger.AccountDay{
		unread(ledger.AccountDay{Account: "gone", MovedOff: 1}),
		unread(ledger.AccountDay{Account: "personal", Models: onOpus(1), Sessions: 1, MovedOn: 1}, "three"),
		unread(ledger.AccountDay{Account: "side", Models: onOpus(3), Sessions: 1, MovedOn: 1, MovedOff: 1}, "one"),
		unread(ledger.AccountDay{Account: "work", Models: onOpus(1), Sessions: 1, MovedOn: 1, MovedOff: 1}, "two"),
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
			got := summarised(t, readingsOf(tt.readings...), asked("1", on(0, 9, 0)))
			if len(got.Accounts) != 1 || !reflect.DeepEqual(got.Accounts[0].Highest, tt.want) {
				t.Errorf("the summary is\n%+v\nwant work's windows' highest use %v", got, tt.want)
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
			got := summarised(t, readingsOf(tt.readings...), asked("1", on(0, 9, 0)))
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

	if _, err := ledger.Summarise(date, slices.Values([]ledger.Line{asked("1", on(0, 9, 0))}), history, nil, caps, dayDone); err != nil {
		t.Fatalf("Summarise() error = %v", err)
	}
	// A week, as long as the longest window runs, before the day's start.
	weekBefore := on(0, 0, 0).Add(-7 * 24 * time.Hour)
	if !from.Equal(weekBefore) || !to.Equal(on(1, 0, 0)) {
		t.Errorf("asked for the readings from %v to %v, want from a week before the day's start to its end, %v to %v", from, to, weekBefore, on(1, 0, 0))
	}
}

func TestADayOfNoRequestsIsASummaryOfNoAccounts(t *testing.T) {
	asked := false
	history := func(from, to time.Time) iter.Seq[readings.Reading] {
		asked = true
		return readingsOf(workRead(on(0, 10, 0), "5h", 0.3, on(0, 13, 0), quota.StatusRejected))(from, to)
	}

	got := string(summaryJSON(t, history))
	if want := `{"version":2,"day":"2026-10-05","lines":0}`; got != want || asked {
		t.Errorf("the summary of a day of no requests is %s, the readings asked for = %v; want %s, and none asked for", got, asked, want)
	}
}

func TestAnUpstreamRequestWhoseAnswerGaveNoUsageIsCounted(t *testing.T) {
	upstream := func(request string, edit func(l *ledger.Line)) ledger.Line {
		l := asked(request, on(0, 9, 0))
		l.Usage = nil
		edit(&l)
		return l
	}
	lines := []ledger.Line{
		asked("1", on(0, 9, 0)),
		upstream("2", func(l *ledger.Line) { l.Canceled = true }),
		upstream("3", func(l *ledger.Line) { l.CutOff = true }),
		upstream("4", func(l *ledger.Line) { l.Status, l.Answer.Error = 529, ledger.Error{Type: "overloaded_error"} }),
		// None of these is counted in upstream: one the router answered itself,
		// a count of tokens, and a quota check.
		upstream("5", func(l *ledger.Line) { l.Attempts, l.Status = 0, 429 }),
		upstream("6", func(l *ledger.Line) { l.Kind = ledger.KindCount }),
		upstream("7", func(l *ledger.Line) { l.Kind, l.Canceled = ledger.KindCheck, true }),
	}

	got := summarised(t, noReadings, lines...)
	if len(got.Accounts) != 1 || len(got.Accounts[0].Models) != 1 {
		t.Fatalf("the summary is %+v, want work's requests of one model", got)
	}
	if m := got.Accounts[0].Models[0]; m.Upstream != 4 || m.NoUsage != 3 || m.Tokens() != (quota.Tokens{Input: 10, Output: 20}) {
		t.Errorf("work's day of %s is %+v, want 4 requests upstream, the 3 whose answers gave no usage counted, and the usage of the one that gave it", opus, m)
	}
}

func TestADateThatIsntOneIsntSummarised(t *testing.T) {
	if _, err := ledger.Summarise("2026-10-32", slices.Values([]ledger.Line(nil)), noReadings, nil, caps, dayDone); err == nil {
		t.Error("Summarise() of 2026-10-32 succeeded, want it to fail: it isn't a date")
	}
}

// asked is a request of session one's answered on work at at, as the ledger
// holds it.
func asked(request string, at time.Time) ledger.Line {
	return ledger.Line{At: at, Request: request, Kind: ledger.KindMessage, Session: "one", Model: opus, Account: "work", Reason: "sticky",
		Status: 200, Attempts: 1, TotalMS: 1000, Usage: usage(`{"input_tokens":10,"output_tokens":20}`)}
}

// summaryOf is the summary of the day with the given date of as many of the
// requests asked makes as requests says, made from as many lines as lines
// says, and of work's windows as read gives them.
func summaryOf(date string, requests, lines int, read windowsRead) string {
	return fmt.Sprintf(`{"version":2,"day":"%s","lines":%d,"accounts":[%s]}`, date, lines, workDayJSON(requests, read))
}

// workDayJSON is work's day in a summary of version 2, as its JSON gives it,
// of as many of the requests asked makes as requests says, and of its
// windows as read gives them.
func workDayJSON(requests int, read windowsRead) string {
	return fmt.Sprintf(`{"account":"work","models":[{"model":"claude-opus-5-5","upstream":%d,"no_usage":0,"unsent":0,"checks":0,"counts":0,"sessions":1,`+
		`"usage":{"input_tokens":%d,"output_tokens":%d}}],"sessions":1,"session_ids":["one"],"moved_on":0,"moved_off":0%s}`,
		requests, 10*requests, 20*requests, read.json())
}

// summaryOfTwo is the summary the ledger writes of the summaries' day, of two
// of the requests asked makes, and of work's windows as read gives them.
func summaryOfTwo(read windowsRead) string {
	return summaryOf(date, 2, 2, read) + "\n"
}

// versionOne is the summary a release of version 1 wrote of the day with the
// given date, as summaryOf says, of the windows' highest use, highest, as its
// JSON gives it, where it's given.
func versionOne(date string, requests, lines int, highest string) string {
	return fmt.Sprintf(`{"version":1,"day":"%s","lines":%d,"accounts":[{"account":"work","models":[{"model":"claude-opus-5-5","upstream":%d,"no_usage":0,`+
		`"unsent":0,"checks":0,"counts":0,"sessions":1,"usage":{"input_tokens":%d,"output_tokens":%d}}],"sessions":1,"moved_on":0,"moved_off":0%s}]}`,
		date, lines, requests, 10*requests, 20*requests, highest)
}

// windowsRead is what a summary of an account's day holds of the readings
// history, each as its JSON gives it: its windows' highest use, and the
// limits it reached, left out where they're ""; their rises and resets, none
// where they're ""; the minutes it spent at its cap and at a limit; and the
// windows it read in the week before, left out where they're "".
type windowsRead struct {
	highest, rise, resets, limits string
	atCap, atLimit                int
	before                        string
}

// resetJSON is the reset of the window with the given key at at, its use
// before before, as a summary's JSON gives it.
func resetJSON(key string, at time.Time, before float64) string {
	return fmt.Sprintf(`{"window":"%s","at":"%s","before":%v}`, key, at.UTC().Format(time.RFC3339), before)
}

// unread is a, an account's day in a summary of version 2, as it's read
// back, its sessions those with the given ids, and nothing read of its
// windows.
func unread(a ledger.AccountDay, sessions ...string) ledger.AccountDay {
	none := 0
	a.SessionIDs, a.Rise, a.Resets = append([]string{}, sessions...), map[string]float64{}, []ledger.Reset{}
	a.MinutesAtCap, a.MinutesAtLimit = &none, &none
	return a
}

// json is what read gives, as a summary's JSON gives it, after its account's
// moves.
func (read windowsRead) json() string {
	field := func(name, value, none string) string {
		if value == "" {
			value = none
		}
		if value == "" {
			return ""
		}
		return `,"` + name + `":` + value
	}
	return field("highest", read.highest, "") + field("rise", read.rise, "{}") + field("resets", read.resets, "[]") +
		field("limits", read.limits, "") + fmt.Sprintf(`,"minutes_at_cap":%d,"minutes_at_limit":%d`, read.atCap, read.atLimit) +
		field("read_before", read.before, "")
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
// with the given date, as it's written, but for the sizes of the day's files
// it's marked with, its bytes, as unmarked gives it: "" where it holds none.
func heldSummary(t *testing.T, dir, date string) string {
	t.Helper()
	data, err := os.ReadFile(summaryFile(dir, date))
	if errors.Is(err, fs.ErrNotExist) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return unmarked(string(data))
}

// marking is the sizes of a day's files a summary is marked with, as it's
// written.
var marking = regexp.MustCompile(`,"bytes":\{"plain":\d+,"compressed":\d+\}`)

// unmarked is a summary, as it's written, but for the sizes of its day's
// files it's marked with.
func unmarked(summary string) string {
	return marking.ReplaceAllString(summary, "")
}

// heldBytes returns the sizes of the day's files the summary the ledger in
// dir holds of the local day with the given date is marked with.
func heldBytes(t *testing.T, dir, date string) ledger.Bytes {
	t.Helper()
	data, err := os.ReadFile(summaryFile(dir, date))
	if err != nil {
		t.Fatal(err)
	}
	var held ledger.Summary
	if err := json.Unmarshal(data, &held); err != nil {
		t.Fatal(err)
	}
	return held.Bytes
}

// dayBytes returns the sizes of the ledger's files in dir of the local day
// with the given date, none of one that isn't there.
func dayBytes(t *testing.T, dir, date string) ledger.Bytes {
	t.Helper()
	var sizes ledger.Bytes
	for name, size := range map[string]*int64{"requests-" + date + ".jsonl": &sizes.Plain, "requests-" + date + ".jsonl.gz": &sizes.Compressed} {
		info, err := os.Stat(filepath.Join(dir, name))
		switch {
		case errors.Is(err, fs.ErrNotExist):
		case err != nil:
			t.Fatal(err)
		default:
			*size = info.Size()
		}
	}
	return sizes
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
		l := ledger.Open(dir, 90*24*time.Hour, clockFrom(on(1, 0, 10)), readings, caps, logs.For("router"))
		stop := running(t, l)
		defer stop()

		synctest.Wait()
		if got := heldSummary(t, dir, date); got != "" {
			t.Fatalf("the day was summarised ten minutes after it ended, as\n%s\nwant it left an hour, for the requests still in flight as it ended to end", got)
		}
		time.Sleep(time.Hour)
		synctest.Wait()
		stop()
		// The window began within the day, and reset at 13:00.
		read := windowsRead{highest: `{"5h":0.25}`, rise: `{"5h":0.25}`, resets: `[` + resetJSON("5h", on(0, 13, 0), 0.25) + `]`}
		if got, want := heldSummary(t, dir, date), summaryOfTwo(read); got != want {
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

func TestADaysSummaryIsWrittenAgainAsLinesComeToBeFiledUnderIt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logstest.Capture(t)
		dir := t.TempDir()
		holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0)))
		l := ledger.Open(dir, 90*24*time.Hour, clockFrom(on(1, 10, 0)), noReadings, caps, logs.For("router"))
		stop := running(t, l)
		defer stop()

		synctest.Wait()
		if got, want := heldSummary(t, dir, date), summaryOfTwo(windowsRead{}); got != want {
			t.Fatalf("the day's summary is\n%s\nwant\n%s", got, want)
		}
		// A line filed under the day after it was summarised, as of a request
		// in flight past the hour the day is given.
		holdLines(t, dir, date, asked("3", on(0, 23, 59)))
		time.Sleep(time.Hour)
		synctest.Wait()
		if got, want := heldSummary(t, dir, date), summaryOf(date, 3, 3, windowsRead{})+"\n"; got != want {
			t.Errorf("at the round after the line came, the day's summary is\n%s\nwant it written again, of its three requests\n%s", got, want)
		}
		time.Sleep(3 * time.Hour)
		synctest.Wait()
		stop()
		if summarised := strings.Count(log.String(), `msg="summarised a day of the request ledger"`); summarised != 2 {
			t.Errorf("log reads\n%s\nwant the day summarised twice: as it ended, and as a line came to be filed under it, and not while its lines stayed as they were", log)
		}
	})
}

// unopenable has the ledger's files in dir whose names match pattern, as
// "requests-*" its files of lines, open to no one until the test ends, so a
// round that opens one warns that it can't.
func unopenable(t *testing.T, dir, pattern string) {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(dir, pattern))
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if err := os.Chmod(file, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(file, 0o600) })
	}
}

// opened counts the ledger's files of lines the rounds logged to log have
// opened since unopenable had them open to no one, each warned of as one
// that can't be.
func opened(log *logstest.Log) int {
	return strings.Count(log.String(), "permission denied")
}

func TestARoundOpensNoFileOfADayWhoseLinesHaventChangedSinceItsSummary(t *testing.T) {
	log := logstest.Capture(t)
	dir := t.TempDir()
	// Three days of lines, the oldest compressed.
	writeFile(t, dir, "requests-2026-10-03.jsonl.gz", gzipped(t, jsonOf(t, asked("1", on(-2, 9, 0)))+"\n"))
	holdLines(t, dir, "2026-10-04", asked("2", on(-1, 9, 0)))
	holdLines(t, dir, date, asked("3", on(0, 9, 0)))
	l := ledger.Open(dir, 400*24*time.Hour, func() time.Time { return on(1, 10, 0) }, noReadings, caps, logs.For("router"))
	l.SummariseEnded(on(1, 10, 0))
	unopenable(t, dir, "requests-*")

	l.SummariseEnded(on(1, 11, 0))
	if n := opened(log); n != 0 {
		t.Errorf("log reads\n%s\nthe round opened %d of the days' files, want none: no day's lines changed since it was summarised", log, n)
	}
	// A line comes to be filed under the 5th, as of a request in flight past
	// the hour the day is given.
	plain := filepath.Join(dir, "requests-"+date+".jsonl")
	if err := os.Chmod(plain, 0o600); err != nil {
		t.Fatal(err)
	}
	holdLines(t, dir, date, asked("4", on(0, 23, 59)))
	l.SummariseEnded(on(1, 12, 0))
	if got, want := heldSummary(t, dir, date), summaryOf(date, 2, 2, windowsRead{})+"\n"; got != want || opened(log) != 0 {
		t.Errorf("log reads\n%s\nthe 5th's summary is\n%s\nwant it written again, of its two requests, from its file alone\n%s", log, got, want)
	}
	unopenable(t, dir, "requests-*")
	l.SummariseEnded(on(1, 13, 0))
	if n := opened(log); n != 0 {
		t.Errorf("log reads\n%s\nthe round after opened %d of the days' files, want none: the 5th was summarised as its lines last changed", log, n)
	}
}

func TestASummaryFoundToStandByCountingItsDaysLinesIsStampedSoTheNextRoundOpensNone(t *testing.T) {
	tests := []struct {
		name string
		// lay lays the day's files and its summary out in dir, as a round
		// finds them.
		lay func(t *testing.T, dir string)
	}{
		{
			name: "one written before summaries were stamped",
			lay: func(t *testing.T, dir string) {
				holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0)))
				writeFile(t, dir, "day-"+date+".json", []byte(summaryOfTwo(windowsRead{})))
			},
		},
		{
			name: "one whose day's lines were compressed since",
			lay: func(t *testing.T, dir string) {
				holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0)))
				writeAt(t, dir, on(3, 9, 0), noReadings)
			},
		},
		{
			name: "one made from more lines than its day's files hold now",
			lay: func(t *testing.T, dir string) {
				holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0)))
				writeFile(t, dir, "day-"+date+".json", []byte(summaryOf(date, 3, 3, windowsRead{})+"\n"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			dir := t.TempDir()
			tt.lay(t, dir)
			held := heldSummary(t, dir, date)
			l := ledger.Open(dir, 400*24*time.Hour, func() time.Time { return on(3, 10, 0) }, noReadings, caps, logs.For("router"))

			l.SummariseEnded(on(3, 10, 0))
			if got := heldSummary(t, dir, date); got != held {
				t.Errorf("the day's summary is\n%s\nwant it as it was, as it stands\n%s", got, held)
			}
			unopenable(t, dir, "requests-*")
			l.SummariseEnded(on(3, 11, 0))
			if n := opened(log); n != 0 {
				t.Errorf("log reads\n%s\nthe round after opened %d of the day's files, want none: the summary was stamped as its lines were counted", log, n)
			}
		})
	}
}

func TestASummaryMarkedAfreshAsItStandsKeepsEveryFieldItHolds(t *testing.T) {
	dir := t.TempDir()
	holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0)))
	// The day's summary, written by a later release, of a later version, of
	// a field this one doesn't know, before its day's files were marked.
	held := strings.Replace(strings.TrimSuffix(summaryOf(date, 2, 2, windowsRead{}), "}")+`,"later":{"kept":[1,2]}}`, `"version":2`, `"version":3`, 1)
	writeFile(t, dir, "day-"+date+".json", []byte(held+"\n"))

	writeAt(t, dir, on(1, 10, 0), noReadings)
	data, err := os.ReadFile(summaryFile(dir, date))
	if err != nil {
		t.Fatal(err)
	}
	sizes := dayBytes(t, dir, date)
	marked := fmt.Sprintf(`{"version":3,"day":"%s","lines":2,"bytes":{"plain":%d,"compressed":%d},`, date, sizes.Plain, sizes.Compressed) +
		strings.TrimPrefix(held, fmt.Sprintf(`{"version":3,"day":"%s","lines":2,`, date)) + "\n"
	if string(data) != marked {
		t.Errorf("the day's summary is\n%s\nwant it as it was, marked with its day's files' sizes alone, every field it held kept\n%s", data, marked)
	}
}

func TestALineFiledUnderACompressedDayAfterTheClockWasSetBackIsSummarised(t *testing.T) {
	dir := t.TempDir()
	holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0)))
	// The router summarises the day, and compresses it two days on, and a
	// round after stamps its summary as the compressed file was written.
	writeAt(t, dir, on(3, 9, 0), noReadings)
	l := ledger.Open(dir, 400*24*time.Hour, func() time.Time { return on(3, 10, 0) }, noReadings, caps, logs.For("router"))
	l.SummariseEnded(on(3, 10, 0))
	compressed, err := os.Stat(filepath.Join(dir, "requests-"+date+".jsonl.gz"))
	if err != nil {
		t.Fatal(err)
	}
	// The clock set back to the day, a line is filed under it, in a plain
	// file of its own, modified, by that clock, before the compressed file.
	holdLines(t, dir, date, asked("3", on(0, 11, 0)))
	if err := os.Chtimes(filepath.Join(dir, "requests-"+date+".jsonl"), time.Time{}, compressed.ModTime().Add(-48*time.Hour)); err != nil {
		t.Fatal(err)
	}

	l.SummariseEnded(on(3, 11, 0))
	if got, want := heldSummary(t, dir, date), summaryOf(date, 3, 3, windowsRead{})+"\n"; got != want {
		t.Errorf("the day's summary is\n%s\nwant it written again, of its three requests\n%s", got, want)
	}
}

func TestADaysSummaryMadeFromMoreLinesThanItsFilesHoldNowStands(t *testing.T) {
	dir := t.TempDir()
	// The day's summary, made from three lines; its files hold two now, a
	// line lost since to damage, and the readings history its highest use
	// came from is pruned.
	held := summaryOf(date, 3, 3, windowsRead{highest: `{"5h":0.9}`, rise: `{"5h":0.4}`}) + "\n"
	writeFile(t, dir, "day-"+date+".json", []byte(held))
	holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0)))

	writeAt(t, dir, on(1, 10, 0), noReadings)
	if got := heldSummary(t, dir, date); got != held {
		t.Errorf("the day's summary is\n%s\nwant it as it was, made from more lines than its files hold now\n%s", got, held)
	}
}

func TestARoundReadsNoSummaryOfADayWhoseFilesHaventChangedSinceItWasMarked(t *testing.T) {
	log := logstest.Capture(t)
	dir := t.TempDir()
	// Days long done, compressed.
	for back := range 3 {
		date := on(-back, 0, 0).Format(time.DateOnly)
		writeFile(t, dir, "requests-"+date+".jsonl.gz", gzipped(t, jsonOf(t, asked("1", on(-back, 9, 0)))+"\n"))
	}
	l := ledger.Open(dir, 400*24*time.Hour, func() time.Time { return on(1, 10, 0) }, noReadings, caps, logs.For("router"))
	l.SummariseEnded(on(1, 10, 0))
	// The summaries can't be read, so a round that reads one warns that it
	// can't.
	unopenable(t, dir, "day-*")

	l.SummariseEnded(on(1, 11, 0))
	if n := opened(log); n != 0 {
		t.Errorf("log reads\n%s\nthe round read %d of the days' summaries, want none: a look at them, and their days' files, tells they stand", log, n)
	}
}

func TestALineAppendedWithinTheTickADaysFileWasLastModifiedInIsSummarised(t *testing.T) {
	dir := t.TempDir()
	plain := filepath.Join(dir, "requests-"+date+".jsonl")
	// tick is when the day's file was last modified, to the second, as a file
	// system that keeps whole seconds keeps it, as HFS+ does.
	tick := on(1, 0, 30)
	lastModifiedIn := func() {
		t.Helper()
		if err := os.Chtimes(plain, time.Time{}, tick); err != nil {
			t.Fatal(err)
		}
	}
	holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0)))
	lastModifiedIn()
	l := ledger.Open(dir, 400*24*time.Hour, func() time.Time { return on(1, 10, 0) }, noReadings, caps, logs.For("router"))
	l.SummariseEnded(on(1, 10, 0))
	// A line comes to be filed under the day, as of a request in flight past
	// the hour the day is given, within the second the file was last
	// modified in.
	holdLines(t, dir, date, asked("3", on(0, 23, 59)))
	lastModifiedIn()

	l.SummariseEnded(on(1, 11, 0))
	if got, want := heldSummary(t, dir, date), summaryOf(date, 3, 3, windowsRead{})+"\n"; got != want {
		t.Errorf("the day's summary is\n%s\nwant it written again, of its three requests\n%s", got, want)
	}
	if got, want := heldBytes(t, dir, date), dayBytes(t, dir, date); got != want {
		t.Errorf("the day's summary is marked with %+v, want its files' sizes, %+v", got, want)
	}
}

func TestADaySummarisedAgainKnowsNoLessThanTheSummaryItReplaces(t *testing.T) {
	limitAt, resets := on(0, 9, 30), on(0, 13, 0)
	reached := `[{"window":"5h","at":"` + limitAt.UTC().Format(time.RFC3339) + `","resets_at":"` + resets.UTC().Format(time.RFC3339) + `"}]`
	rejected := workRead(limitAt, "5h", 1, resets, quota.StatusRejected)
	// The window's run to its reset at 13:00, from its start within the day,
	// at its limit from 09:30.
	limited := windowsRead{highest: `{"5h":1}`, rise: `{"5h":1}`, resets: `[` + resetJSON("5h", resets, 1) + `]`, limits: reached, atLimit: 210}
	withBefore := func(read windowsRead) windowsRead {
		read.before = `["5h"]`
		return read
	}
	// twoLines are the day's two requests' lines, and aTornLine the line of
	// a day whose only line was torn as it was written.
	twoLines := func(t *testing.T, dir string) {
		holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0)))
	}
	aTornLine := func(t *testing.T, dir string) {
		writeFile(t, dir, "requests-"+date+".jsonl", []byte(`{"at":"2026-10-05T09:00:00Z","requ`+"\n"))
	}
	tests := []struct {
		name string
		// lay lays the day's lines out in dir as it's first summarised.
		lay func(t *testing.T, dir string)
		// then and now are the readings the readings history holds as the day
		// is summarised, and as it's summarised again.
		then, now []readings.Reading
		// requests and lines are the summary's requests and lines, and
		// readings what it holds of the readings, as its JSON gives them.
		requests, lines int
		readings        windowsRead
	}{
		{
			name: "its windows' highest use, rise, resets, limits and minutes, the readings they came from pruned since", lay: twoLines,
			then:     []readings.Reading{workRead(on(0, 9, 0), "5h", 0.9, resets, quota.StatusAllowed), rejected},
			requests: 3, lines: 3, readings: limited,
		},
		{
			name: "no limit read first, the reading before it pruned since", lay: twoLines,
			then: []readings.Reading{workRead(on(-1, 23, 0), "5h", 1, on(0, 2, 0), quota.StatusRejected), workRead(on(0, 1, 0), "5h", 1, on(0, 2, 0), quota.StatusRejected)},
			now:  []readings.Reading{workRead(on(0, 1, 0), "5h", 1, on(0, 2, 0), quota.StatusRejected)}, requests: 3, lines: 3,
			// At its limit from the day's start, as the reading carried into it
			// read, to its reset at 02:00.
			readings: windowsRead{highest: `{"5h":1}`, rise: `{"5h":0}`, resets: `[` + resetJSON("5h", on(0, 2, 0), 1) + `]`, atLimit: 120, before: `["5h"]`},
		},
		{
			name: "a limit read first it held, the reading before it pruned since", lay: twoLines,
			then:     []readings.Reading{workRead(on(-3, 9, 0), "5h", 0.5, on(-3, 13, 0), quota.StatusAllowed), rejected},
			now:      []readings.Reading{rejected},
			requests: 3, lines: 3, readings: withBefore(limited),
		},
		{
			name: "a limit read first, the summary it replaces read no readings, its day's one line torn", lay: aTornLine,
			then: []readings.Reading{rejected}, now: []readings.Reading{rejected}, requests: 1, lines: 2, readings: limited,
		},
		{
			name: "a limit read first, the summary it replaces read no readings, the history's files unreadable then", lay: twoLines,
			now: []readings.Reading{rejected}, requests: 3, lines: 3, readings: limited,
		},
	}
	for _, tt := range tests {
		for _, by := range []string{"the router", "a reader"} {
			t.Run(tt.name+", by "+by, func(t *testing.T) {
				state, dir, _ := stateDirs(t)
				tt.lay(t, dir)
				writeAt(t, dir, on(1, 10, 0), readingsOf(tt.then...))
				// A line comes to be filed under the day, as of a request in
				// flight past the hour the day is given.
				holdLines(t, dir, date, asked("3", on(0, 23, 59)))

				got := summaryAgain(t, by, state, dir, on(1, 11, 0), tt.now)
				if want := summaryOf(date, tt.requests, tt.lines, tt.readings); got != want {
					t.Errorf("the day's summary is\n%s\nwant it of its requests, knowing what the one it replaces did of the readings\n%s", got, want)
				}
			})
		}
	}
}

func TestADaySummarisedAgainCountsNoLessThanTheSummaryItReplacesWhereLinesWereLostSince(t *testing.T) {
	inHaiku := func(request string, at time.Time) ledger.Line {
		l := asked(request, at)
		l.Model, l.Session = haiku, "two"
		return l
	}
	for _, by := range []string{"the router", "a reader"} {
		t.Run("by "+by, func(t *testing.T) {
			state, dir, _ := stateDirs(t)
			// The day's lines, compressed, a member for each of two goes, and
			// its summary, of its three requests.
			writeFile(t, dir, "requests-"+date+".jsonl.gz", gzipped(t, jsonOf(t, asked("1", on(0, 9, 0)))+"\n",
				jsonOf(t, asked("2", on(0, 10, 0)))+"\n"+jsonOf(t, asked("3", on(0, 11, 0)))+"\n"))
			writeAt(t, dir, on(3, 10, 0), noReadings)
			// The compressed file damaged since, its second member lost, and
			// three lines filed under the day after, as after the clock was set
			// back to it.
			compressed := filepath.Join(dir, "requests-"+date+".jsonl.gz")
			if err := os.WriteFile(compressed, append(gzipped(t, jsonOf(t, asked("1", on(0, 9, 0)))+"\n"), gzipped(t, "lost\n")[:5]...), 0o600); err != nil {
				t.Fatal(err)
			}
			holdLines(t, dir, date, inHaiku("4", on(0, 12, 0)), inHaiku("5", on(0, 12, 1)), inHaiku("6", on(0, 12, 2)))

			got := summaryAgain(t, by, state, dir, on(3, 11, 0), nil)
			model := func(name string) string {
				return `{"model":"` + name + `","upstream":3,"no_usage":0,"unsent":0,"checks":0,"counts":0,"sessions":1,"usage":{"input_tokens":30,"output_tokens":60}}`
			}
			// Its files hold four lines now, of its six requests.
			want := `{"version":2,"day":"2026-10-05","lines":6,"accounts":[{"account":"work","models":[` + model(haiku) + `,` + model(opus) + `],` +
				`"sessions":2,"session_ids":["one","two"],"moved_on":0,"moved_off":0` + windowsRead{}.json() + `}]}`
			if got != want {
				t.Errorf("the day's summary is\n%s\nwant its three requests of %s it held still, and the three of %s filed since, made from no fewer lines\n%s",
					got, opus, haiku, want)
			}
		})
	}
}

// summaryAgain returns the summary of the summaries' day in the ledger in
// dir, in the state directory state, summarised again at at, with the
// readings held, by the router's round, as it writes it, or by a reader of
// the ledger, as Days gives it, the readings in the readings history's
// files: but for the sizes of the day's files it's marked with.
func summaryAgain(t *testing.T, by, state, dir string, at time.Time, held []readings.Reading) string {
	t.Helper()
	if by == "the router" {
		writeAt(t, dir, at, readingsOf(held...))
		return strings.TrimSuffix(heldSummary(t, dir, date), "\n")
	}
	for _, r := range held {
		writeFile(t, readings.Dir(state), "readings-"+r.At.Local().Format(time.DateOnly)+".jsonl", []byte(readingJSON(t, r)+"\n"))
	}
	return summariesJSON(t, readerAt(state, at).Days(on(0, 0, 0))[:1])[0]
}

func TestARoundReadsTheReadingsOfEachDayItSummarisesAndTheWeekBeforeAlone(t *testing.T) {
	dir := t.TempDir()
	// A day stuck unsummarised since, as one of its files couldn't be opened
	// until now, a month before yesterday, and yesterday.
	old, yesterday := on(-30, 0, 0), on(0, 0, 0)
	holdLines(t, dir, old.Format(time.DateOnly), asked("1", on(-30, 9, 0)))
	holdLines(t, dir, date, asked("2", on(0, 9, 0)))
	var held []readings.Reading
	for at := old.AddDate(0, 0, -7); at.Before(on(1, 0, 0)); at = at.Add(time.Hour) {
		held = append(held, workRead(at, "5h", 0.1, at.Add(5*time.Hour), quota.StatusAllowed))
	}
	given := 0
	history := func(from, to time.Time) iter.Seq[readings.Reading] {
		return func(yield func(readings.Reading) bool) {
			for r := range readingsOf(held...)(from, to) {
				given++
				if !yield(r) {
					return
				}
			}
		}
	}

	writeAt(t, dir, on(1, 10, 0), history)
	for _, day := range []time.Time{old, yesterday} {
		if heldSummary(t, dir, day.Format(time.DateOnly)) == "" {
			t.Fatalf("no summary of %s", day.Format(time.DateOnly))
		}
	}
	// Each day's and its week before's, up to the first after the day's end.
	if most := 2 * (8*24 + 1); given > most {
		t.Errorf("the readings history gave %d readings, want %d at most: those of each day and the week before it, none of the month between", given, most)
	}
}

func TestADayOneOfWhoseFilesCantBeOpenedIsLeftForALaterRound(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logstest.Capture(t)
		dir := t.TempDir()
		// The day's compressed file reads, but not the plain file beside it,
		// which holds a line added since, as after the clock was set back to
		// the day, as the first round comes.
		compressed, err := json.Marshal(asked("1", on(0, 9, 0)))
		if err != nil {
			t.Fatal(err)
		}
		writeFile(t, dir, "requests-"+date+".jsonl.gz", gzipped(t, string(compressed)+"\n"))
		holdLines(t, dir, date, asked("2", on(0, 10, 0)))
		plain := filepath.Join(dir, "requests-"+date+".jsonl")
		if err := os.Chmod(plain, 0o000); err != nil {
			t.Fatal(err)
		}
		l := ledger.Open(dir, 90*24*time.Hour, clockFrom(on(3, 9, 0)), noReadings, caps, logs.For("router"))
		stop := running(t, l)
		defer stop()

		synctest.Wait()
		if got := heldSummary(t, dir, date); got != "" {
			t.Fatalf("the day was summarised as one of its files couldn't be opened, as\n%s\nwant it left for a round that reads it whole", got)
		}
		if !log.Has("level=WARN", `msg="can't summarise the request ledger"`, "day="+date, "requests-"+date+".jsonl") {
			t.Errorf("log reads\n%s\nwant the day that can't be summarised warned of, naming the file that can't be opened", log)
		}
		if err := os.Chmod(plain, 0o600); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Hour)
		synctest.Wait()
		stop()
		if got, want := heldSummary(t, dir, date), summaryOfTwo(windowsRead{}); got != want {
			t.Errorf("at the round after, the day's summary is\n%s\nwant\n%s", got, want)
		}
	})
}

func TestADayOfNoLineThatReadsIsSummarisedOnceUntilALineComesThatDoes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logstest.Capture(t)
		dir := t.TempDir()
		// A day whose only line was torn as it was written.
		const torn = "2026-10-04"
		writeFile(t, dir, "requests-"+torn+".jsonl", []byte(`{"at":"2026-10-04T09:00:00Z","requ`))
		l := ledger.Open(dir, 90*24*time.Hour, clockFrom(on(1, 9, 0)), noReadings, caps, logs.For("router"))
		stop := running(t, l)
		defer stop()

		synctest.Wait()
		if got, want := heldSummary(t, dir, torn), `{"version":2,"day":"2026-10-04","lines":1}`+"\n"; got != want {
			t.Errorf("the day's summary is\n%s\nwant one of no requests, made from its one line\n%s", got, want)
		}
		time.Sleep(3 * time.Hour)
		synctest.Wait()
		if warned := strings.Count(log.String(), `msg="request ledger lines unread"`); warned != 1 || !log.Has("level=WARN", "day="+torn, "lines=1") {
			t.Errorf("log reads\n%s\nwant the line that doesn't read warned of once, as the day was summarised, not at each round", log)
		}
		// A line of the day, filed under it after its summary, as of a request
		// in flight past the hour the day is given.
		late := asked("1", on(-1, 23, 30))
		l.Note(&late)
		time.Sleep(time.Hour)
		synctest.Wait()
		stop()
		if got, want := heldSummary(t, dir, torn), summaryOf(torn, 1, 2, windowsRead{})+"\n"; got != want {
			t.Errorf("once a line that reads came, the day's summary is\n%s\nwant it written again, of its request\n%s", got, want)
		}
	})
}

func TestASummaryThatCantBeReadIsWarnedOfAtEachRoundUntilItCanBe(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logstest.Capture(t)
		dir := t.TempDir()
		holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0)))
		// The day's summary leads nowhere it can be read: a link to itself.
		summary := summaryFile(dir, date)
		if err := os.Symlink(filepath.Base(summary), summary); err != nil {
			t.Fatal(err)
		}
		l := ledger.Open(dir, 90*24*time.Hour, clockFrom(on(1, 10, 0)), noReadings, caps, logs.For("router"))
		stop := running(t, l)
		defer stop()

		synctest.Wait()
		time.Sleep(time.Hour)
		synctest.Wait()
		if warned := strings.Count(log.String(), `msg="can't summarise the request ledger"`); warned != 2 || !log.Has("level=WARN", "day="+date) {
			t.Errorf("log reads\n%s\nwant the summary that can't be read warned of at each of the two rounds", log)
		}
		if info, err := os.Lstat(summary); err != nil || info.Mode().Type() != fs.ModeSymlink {
			t.Fatalf("the day's summary is %v (%v), want it left as it was for the round after", info, err)
		}
		if err := os.Remove(summary); err != nil {
			t.Fatal(err)
		}
		time.Sleep(time.Hour)
		synctest.Wait()
		stop()
		if got, want := heldSummary(t, dir, date), summaryOfTwo(windowsRead{}); got != want {
			t.Errorf("once it could be read, the day's summary is\n%s\nwant\n%s", got, want)
		}
	})
}

func TestASummaryThatDoesntReadAsItsDaysIsSummarisedAgain(t *testing.T) {
	tests := []struct {
		name string
		held string
	}{
		{name: "of another day", held: summaryOf("2026-10-04", 2, 2, windowsRead{})},
		{name: "of no day", held: `{"version":1,"lines":2}`},
		{name: "cut short", held: `{"version":1,"day":`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			dir := t.TempDir()
			holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0)))
			writeFile(t, dir, "day-"+date+".json", []byte(tt.held+"\n"))

			writeAt(t, dir, on(1, 10, 0), noReadings)
			if got, want := heldSummary(t, dir, date), summaryOfTwo(windowsRead{}); got != want {
				t.Errorf("the day's summary is\n%s\nwant it written again from its lines\n%s", got, want)
			}
			if !log.Has("level=WARN", `msg="can't read the request ledger's summary of a day; summarising it from its lines"`, "day="+date) {
				t.Errorf("log reads\n%s\nwant the summary that doesn't read as the day's warned of", log)
			}
		})
	}
}

func TestARoundReadsTheReadingsHistoryOnceForTheDaysItSummarises(t *testing.T) {
	dir := t.TempDir()
	for day := range 3 {
		holdLines(t, dir, on(day, 0, 0).Format(time.DateOnly), asked("1", on(day, 9, 0)))
	}
	asked := 0
	history := func(from, to time.Time) iter.Seq[readings.Reading] {
		asked++
		return readingsOf(workRead(on(0, 9, 0), "5h", 0.2, on(0, 13, 0), quota.StatusAllowed),
			workRead(on(2, 9, 0), "5h", 0.4, on(2, 13, 0), quota.StatusAllowed))(from, to)
	}

	writeAt(t, dir, on(4, 9, 0), history)
	read := []windowsRead{
		{highest: `{"5h":0.2}`, rise: `{"5h":0.2}`, resets: `[` + resetJSON("5h", on(0, 13, 0), 0.2) + `]`},
		{before: `["5h"]`},
		{highest: `{"5h":0.4}`, rise: `{"5h":0.4}`, resets: `[` + resetJSON("5h", on(2, 13, 0), 0.4) + `]`, before: `["5h"]`},
	}
	for day, read := range read {
		date := on(day, 0, 0).Format(time.DateOnly)
		if got, want := heldSummary(t, dir, date), summaryOf(date, 1, 1, read)+"\n"; got != want {
			t.Errorf("the summary of %s is\n%s\nwant\n%s", date, got, want)
		}
	}
	if asked != 1 {
		t.Errorf("the readings history was asked for readings %d times, want once for the three days", asked)
	}
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
	if got, want := heldSummary(t, dir, date), summaryOfTwo(windowsRead{}); got != want {
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
	if got, want := heldSummary(t, dir, date), summaryOfTwo(windowsRead{}); got != want {
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
	if got, want := heldSummary(t, dir, date), summaryOf(date, 2, 5, windowsRead{})+"\n"; got != want {
		t.Errorf("the day's summary is\n%s\nwant it of the two lines that read, made from all five\n%s", got, want)
	}
	if !log.Has("level=WARN", `msg="request ledger lines unread"`, "day="+date, "lines=3") {
		t.Errorf("log reads\n%s\nwant the three lines that don't read noted", log)
	}
}

func TestASummaryThatCantBeWrittenIsLoggedAndWrittenOnALaterRound(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		log := logstest.Capture(t)
		dir := t.TempDir()
		holdLines(t, dir, date, asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0)))
		l := ledger.Open(dir, 90*24*time.Hour, clockFrom(on(1, 0, 30)), noReadings, caps, logs.For("router"))
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
		if got, want := heldSummary(t, dir, date), summaryOfTwo(windowsRead{}); got != want {
			t.Errorf("at the round after, the day's summary is\n%s\nwant\n%s", got, want)
		}
	})
}
