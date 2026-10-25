package ledger_test

import (
	"encoding/json"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
)

// inZone has the local time zone be the one with the given name until the
// test ends, as a process started there has it.
func inZone(t *testing.T, name string) {
	t.Helper()
	loc, err := time.LoadLocation(name)
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	was := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = was })
}

// sum is the sum of hours.
func sum(hours [24]int) int {
	n := 0
	for _, in := range hours {
		n += in
	}
	return n
}

func TestEachRequestUpstreamIsCountedInTheLocalHourItArrivedIn(t *testing.T) {
	tests := []struct {
		name, zone, date string
		// odd is the hour the clocks skip, or repeat, and in the requests
		// counted in it, of one that arrived every half hour of the day: every
		// other hour counts two.
		odd, in int
	}{
		{name: "a day of 24 hours", zone: "Europe/London", date: "2026-10-05", odd: 1, in: 2},
		{name: "a day of 23 hours, the clocks gone forward over 01:00", zone: "Europe/London", date: "2026-03-29", odd: 1, in: 0},
		{name: "a day of 25 hours, the clocks gone back over 01:00", zone: "Europe/London", date: "2026-10-25", odd: 1, in: 4},
		{name: "a day of 23 hours, the clocks gone forward over midnight", zone: "America/Santiago", date: "2026-09-06", odd: 0, in: 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inZone(t, tt.zone)
			start, end, _ := dayfile.Day(tt.date)
			var lines []ledger.Line
			for at := start; at.Before(end); at = at.Add(30 * time.Minute) {
				lines = append(lines, asked(strconv.Itoa(len(lines)), at))
			}

			s, err := ledger.Summarise(tt.date, slices.Values(lines), noReadings.of, nil, caps, end.Add(time.Hour))
			if err != nil || len(s.Accounts) != 1 || len(s.Accounts[0].Models) != 1 {
				t.Fatalf("the summary is %+v (%v), want work's requests of one model", s, err)
			}
			var want [24]int
			for hour := range want {
				want[hour] = 2
			}
			want[tt.odd] = tt.in
			m := s.Accounts[0].Models[0]
			if m.Hours == nil || *m.Hours != want {
				t.Fatalf("the day's requests are counted by the hour as %v, want %v", m.Hours, want)
			}
			if n := sum(*m.Hours); n != m.Upstream || m.Upstream != len(lines) {
				t.Errorf("the hours sum to %d, the requests upstream %d, want both the day's %d", n, m.Upstream, len(lines))
			}
		})
	}
}

// modelsOf returns work's models in a summary, as its JSON gives it, by
// their names.
func modelsOf(t *testing.T, summary string) map[string]ledger.ModelDay {
	t.Helper()
	var s ledger.Summary
	if err := json.Unmarshal([]byte(summary), &s); err != nil {
		t.Fatalf("the summary %s: %v", summary, err)
	}
	models := make(map[string]ledger.ModelDay)
	for _, a := range s.Accounts {
		if a.Account == "work" {
			for _, m := range a.Models {
				models[m.Model] = m
			}
		}
	}
	return models
}

// heldWith is the summaries' day as a release of the version given summarised
// it, made from as many lines as lines says: work's requests of the models
// given, of session one.
func heldWith(t *testing.T, version, lines int, models ...ledger.ModelDay) string {
	t.Helper()
	for i := range models {
		models[i].Sessions, models[i].SessionIDs = 1, []string{"one"}
	}
	data, err := json.Marshal(ledger.Summary{Version: version, Day: date, Lines: lines, Accounts: []ledger.AccountDay{
		{Account: "work", Models: models, Sessions: 1, SessionIDs: []string{"one"}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// ofModel is l, asking for the model given.
func ofModel(l ledger.Line, model string) ledger.Line {
	l.Model = model
	return l
}

// A model's hours, written again, go whole with the count upstream they sum
// to, the more of the summary made afresh's and the one it replaces': never
// the more of each hour, which would sum past it, as the requests lost since
// can't be told from those filed since. Of two counting as many, those
// counted afresh, unless only the summary before's were counted; and none
// where the count is of a summary that never counted them.
func TestAModelsHoursGoWithTheCountUpstreamTheySumTo(t *testing.T) {
	check := asked("c", on(0, 11, 0))
	check.Kind = ledger.KindCheck
	// upstream is a model's requests upstream, as a summary counts them, and
	// hours its hours, nil where they're never counted.
	type upstream struct {
		count int
		hours *[24]int
	}
	tests := []struct {
		name string
		// held is the day's summary before, and lines its files' lines now.
		held  string
		lines []ledger.Line
		want  map[string]upstream
	}{
		{
			name: "fewer upstream than the summary before, some lost since and others filed since: its hours whole",
			held: heldWith(t, 6, 3, ledger.ModelDay{Model: opus, Upstream: 3, Hours: hoursAt(on(0, 9, 0), on(0, 10, 0), on(0, 11, 0))}),
			lines: []ledger.Line{asked("1", on(0, 9, 0)), asked("4", on(0, 23, 0)),
				ofModel(asked("5", on(0, 12, 0)), haiku), ofModel(asked("6", on(0, 12, 5)), haiku)},
			want: map[string]upstream{
				opus:  {3, hoursAt(on(0, 9, 0), on(0, 10, 0), on(0, 11, 0))},
				haiku: {2, hoursAt(on(0, 12, 0), on(0, 12, 5))},
			},
		},
		{
			name:  "more upstream than the summary before, counted in other hours, as in another time zone: the hours afresh, whole",
			held:  heldWith(t, 6, 2, ledger.ModelDay{Model: opus, Upstream: 2, Hours: hoursAt(on(0, 8, 0), on(0, 9, 0))}),
			lines: []ledger.Line{asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0)), asked("3", on(0, 23, 0))},
			want:  map[string]upstream{opus: {3, hoursAt(on(0, 9, 0), on(0, 10, 0), on(0, 23, 0))}},
		},
		{
			name:  "as many upstream as the summary before, counted in other hours: the hours afresh",
			held:  heldWith(t, 6, 2, ledger.ModelDay{Model: opus, Upstream: 2, Hours: hoursAt(on(0, 8, 0), on(0, 9, 0))}),
			lines: []ledger.Line{asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0)), check},
			want:  map[string]upstream{opus: {2, hoursAt(on(0, 9, 0), on(0, 10, 0))}},
		},
		{
			name:  "as many upstream as a summary of version 5, which never counted its hours: the hours afresh",
			held:  heldWith(t, 5, 2, ledger.ModelDay{Model: opus, Upstream: 2}),
			lines: []ledger.Line{asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0))},
			want:  map[string]upstream{opus: {2, hoursAt(on(0, 9, 0), on(0, 10, 0))}},
		},
		{
			name: "fewer upstream than a summary of version 5 counted: none, as its were never counted",
			held: heldWith(t, 5, 3, ledger.ModelDay{Model: opus, Upstream: 3}),
			lines: []ledger.Line{asked("1", on(0, 9, 0)),
				ofModel(asked("5", on(0, 12, 0)), haiku), ofModel(asked("6", on(0, 12, 5)), haiku)},
			want: map[string]upstream{opus: {3, nil}, haiku: {2, hoursAt(on(0, 12, 0), on(0, 12, 5))}},
		},
	}
	for _, tt := range tests {
		for _, by := range []string{"the router", "a reader"} {
			t.Run(tt.name+", by "+by, func(t *testing.T) {
				state, dir, _ := stateDirs(t)
				writeFile(t, dir, "day-"+date+".json", []byte(tt.held+"\n"))
				holdLines(t, dir, date, tt.lines...)

				got := modelsOf(t, summaryAgain(t, by, state, dir, on(3, 10, 0), nil))
				if len(got) != len(tt.want) {
					t.Fatalf("work's models are %+v, want %d of them", got, len(tt.want))
				}
				for model, want := range tt.want {
					m := got[model]
					if m.Upstream != want.count || !sameHours(m.Hours, want.hours) {
						t.Errorf("%s's requests upstream are %d, by the hour %v; want %d, %v", model, m.Upstream, m.Hours, want.count, want.hours)
					}
					if m.Hours != nil && sum(*m.Hours) != m.Upstream {
						t.Errorf("%s's hours sum to %d, want its %d requests upstream", model, sum(*m.Hours), m.Upstream)
					}
				}
			})
		}
	}
}

// sameHours reports whether a and b are the same hours, or both never
// counted.
func sameHours(a, b *[24]int) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

// A day whose hours were never counted, as one summarised before version 6
// whose lines are gone, has none, never as many of none: kept as it is, by
// the router and a reader alike, or summarised again from its readings, its
// models kept as it gave them.
func TestHoursNeverCountedAreLeftOutNeverGivenAsNone(t *testing.T) {
	held := ofVersion(summaryOf(date, 2, windowsRead{}, theTwo...), 5)
	tests := []struct {
		name string
		// kept are the readings the readings history keeps of the day.
		kept []readings.Reading
	}{
		{name: "its readings gone too, kept as it is"},
		{name: "its readings kept, summarised again from them", kept: []readings.Reading{historyBegan,
			workRead(on(0, 10, 0), "5h", 0.3, on(0, 13, 0), quota.StatusAllowed)}},
	}
	for _, tt := range tests {
		for _, by := range []string{"the router", "a reader"} {
			t.Run(tt.name+", by "+by, func(t *testing.T) {
				state, dir, _ := stateDirs(t)
				writeFile(t, dir, "day-"+date+".json", []byte(held+"\n"))

				got := summaryAgain(t, by, state, dir, on(3, 10, 0), tt.kept)
				m, ok := modelsOf(t, got)[opus]
				if !ok || m.Upstream != 2 {
					t.Fatalf("the day's summary is\n%s\nwant work's two requests of %s in it", got, opus)
				}
				if m.Hours != nil || strings.Contains(got, `"hours"`) {
					t.Errorf("the day's summary is\n%s\nwant its hours left out, never counted", got)
				}
			})
		}
	}
}
