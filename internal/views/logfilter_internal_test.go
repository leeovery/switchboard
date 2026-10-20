package views

import (
	"fmt"
	"maps"
	"math/rand/v2"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
)

// A past day is read back through the filter the Requests show with every
// kind, as kinds choose little; whole, the zero LogFilter, where the filter
// keeps every line whatever its kind, or the stream tells of a request whose
// line may be filed on the day: sent on it, or sent within a minute after
// the midnight it ends at, as FiledOn files one, or after the one it begins
// at, as one sent so most likely arrived on it. A request sent on another
// day, or never sent, leaves the filter's choices.
func TestAPastDayIsReadThroughItsFilterWithEveryKindOrWhole(t *testing.T) {
	const date = "2026-10-07"
	start, end, _ := dayfile.Day(date)
	told := func(sent time.Time) Stream {
		return Stream{Requests: map[string]Told{"r1": {Request: "r1", Session: "s", Sent: sent}}}
	}
	session := LogFilter{Sessions: []string{"s"}, Kinds: []RowKind{RowRequest}}
	ofSession := LogFilter{Sessions: []string{"s"}}
	tests := []struct {
		name string
		f    LogFilter
		s    Stream
		want LogFilter
	}{
		{"a session's, its kinds let go", session, Stream{}, ofSession},
		{"the failed of an account's, its kinds let go", LogFilter{Accounts: []string{"work"}, Kinds: []RowKind{RowCheck}, Failed: true}, Stream{},
			LogFilter{Accounts: []string{"work"}, Failed: true}},
		{"the Log's opening filter, which keeps every line of its kinds", LogOpening(), Stream{}, LogFilter{}},
		{"one ticking none", LogFilter{}, Stream{}, LogFilter{}},
		{"a session's, the stream telling of a request sent on the day", session, told(start.Add(12 * time.Hour)), LogFilter{}},
		{"a session's, the stream telling of one sent just after the midnight the day ends at", session, told(end.Add(30 * time.Second)), LogFilter{}},
		{"a session's, the stream telling of one sent just after the midnight the day begins at", session, told(start.Add(30 * time.Second)), LogFilter{}},
		{"a session's, the stream telling of one sent the day before", session, told(start.Add(-time.Hour)), ofSession},
		{"a session's, the stream telling of one sent two minutes after the day ends", session, told(end.Add(2 * time.Minute)), ofSession},
		{"a session's, the stream telling of one never sent", session, told(time.Time{}), ofSession},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.f.ThroughOn(tt.s, date); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("%+v.ThroughOn(%+v, %s) = %+v, want %+v", tt.f, tt.s.Requests, date, got, tt.want)
			}
		})
	}
}

// A day held through a filter serves another, so the day tables through it
// as held whole, where each of the other's choices is within the filter's,
// every one where the filter ticks none, and the other keeps only those that
// failed where the filter does; whatever kinds either ticks.
func TestADayHeldThroughAFilterServesThoseWithinIt(t *testing.T) {
	s, both, work, opus := []string{"s"}, []string{"s", "t"}, []string{"work"}, []string{foldOpus}
	tests := []struct {
		name       string
		through, f LogFilter
		want       bool
	}{
		{"whole serves one ticking none", LogFilter{}, LogFilter{}, true},
		{"whole serves the failed of a session's", LogFilter{}, LogFilter{Sessions: s, Failed: true}, true},
		{"a session's serves the session's", LogFilter{Sessions: s}, LogFilter{Sessions: s}, true},
		{"a session's serves the session's failed on an account of a model, of a kind",
			LogFilter{Sessions: s}, LogFilter{Sessions: s, Accounts: work, Models: opus, Kinds: []RowKind{RowCheck}, Failed: true}, true},
		{"two sessions' serve one of them", LogFilter{Sessions: both}, LogFilter{Sessions: []string{"t"}}, true},
		{"a session's serves not the session's and another's", LogFilter{Sessions: s}, LogFilter{Sessions: both}, false},
		{"a session's serves not an account's of every session", LogFilter{Sessions: s}, LogFilter{Accounts: work}, false},
		{"an account's of a model serves not the account's of every model", LogFilter{Accounts: work, Models: opus}, LogFilter{Accounts: work}, false},
		{"the failed serve the failed of a session's", LogFilter{Failed: true}, LogFilter{Sessions: s, Failed: true}, true},
		{"the failed serve not every line", LogFilter{Failed: true}, LogFilter{}, false},
		{"a session's failed serve not the session's", LogFilter{Sessions: s, Failed: true}, LogFilter{Sessions: s}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.through.serves(tt.f); got != tt.want {
				t.Errorf("%+v.serves(%+v) = %t, want %t", tt.through, tt.f, got, tt.want)
			}
		})
	}
}

// Two filters joined tick together each choice either ticks, ticking none
// where either ticks none, keep only those that failed where both do, and
// keep every kind: so the join serves both, and keeps no line neither keeps
// but where a choice of one is joined with a choice of the other.
func TestTwoFiltersJoinedServeBoth(t *testing.T) {
	s, work := []string{"s"}, []string{"work"}
	tests := []struct {
		name string
		f, o LogFilter
		want LogFilter
	}{
		{"two sessions'", LogFilter{Sessions: s}, LogFilter{Sessions: []string{"t", "s"}}, LogFilter{Sessions: []string{"s", "t"}}},
		{"a session's and an account's, crossing, are whole", LogFilter{Sessions: s}, LogFilter{Accounts: work}, LogFilter{}},
		{"a session's on two accounts",
			LogFilter{Sessions: s, Accounts: work}, LogFilter{Sessions: s, Accounts: []string{"side"}, Kinds: []RowKind{RowCheck}},
			LogFilter{Sessions: s, Accounts: []string{"work", "side"}}},
		{"the failed of a session's and the failed", LogFilter{Sessions: s, Failed: true}, LogFilter{Failed: true}, LogFilter{Failed: true}},
		{"the failed of a session's and the session's", LogFilter{Sessions: s, Failed: true}, LogFilter{Sessions: s}, LogFilter{Sessions: s}},
		{"whole and a session's", LogFilter{}, LogFilter{Sessions: s}, LogFilter{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.f.joined(tt.o)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("%+v.joined(%+v) = %+v, want %+v", tt.f, tt.o, got, tt.want)
			}
			if !got.serves(tt.f) || !got.serves(tt.o) {
				t.Errorf("%+v.joined(%+v) = %+v serves the one %t, the other %t; want both", tt.f, tt.o, got, got.serves(tt.f), got.serves(tt.o))
			}
		})
	}
}

// summariesOn are the summaries of days by their dates, read as
// LedgerDates reads them, noting the dates each read asked for.
type summariesOn struct {
	on    map[string]ledger.Summary
	asked [][]string
}

func (s *summariesOn) DaysOn(dates []string) []ledger.Summary {
	s.asked = append(s.asked, dates)
	var summaries []ledger.Summary
	for _, date := range slices.Sorted(slices.Values(dates)) {
		if summary, ok := s.on[date]; ok {
			summaries = append(summaries, summary)
		}
	}
	return summaries
}

// A day may hold a line a filter keeps where its own summary, or that of a
// day either side of it, may, as a change of time zone files a line under a
// date beside its own, of the filter with every kind; and never where none
// of the three may, a day two before aside. No summary counts a day's
// failures, so of those that failed they say what they say of every line.
// A filter keeping every line may keep one on any day, read of no summary.
func TestADaysSummariesAndThoseBesideItSayWhetherItMayKeepALine(t *testing.T) {
	const date = "2026-10-08"
	day := func(date, account, model string, requests, checks int, sessions ...string) ledger.Summary {
		m := ledger.ModelDay{Model: model, Upstream: requests, Checks: checks, Sessions: len(sessions), SessionIDs: sessions}
		return ledger.Summary{Day: date, Accounts: []ledger.AccountDay{{Account: account, Models: []ledger.ModelDay{m}}}}
	}
	summaries := map[string]ledger.Summary{"2026-10-06": day("2026-10-06", "work", foldOpus, 3, 0, "s"), date: day(date, "work", foldOpus, 3, 0, "t")}
	tests := []struct {
		name string
		f    LogFilter
		on   map[string]ledger.Summary
		want bool
	}{
		{"of a session its own summary names", LogFilter{Sessions: []string{"t"}}, summaries, true},
		{"of a session the day before's names", LogFilter{Sessions: []string{"s"}}, map[string]ledger.Summary{"2026-10-07": day("2026-10-07", "work", foldOpus, 1, 0, "s")}, true},
		{"of a session the day after's names", LogFilter{Sessions: []string{"s"}}, map[string]ledger.Summary{"2026-10-09": day("2026-10-09", "work", foldOpus, 1, 0, "s")}, true},
		{"of a session only a day two before names", LogFilter{Sessions: []string{"s"}}, summaries, false},
		{"of an account none of the three tells of", LogFilter{Accounts: []string{"side"}}, summaries, false},
		{"of the failed of an account none of the three tells of", LogFilter{Accounts: []string{"side"}, Failed: true}, summaries, false},
		{"of the failed alone, of a day of lines none counts a failure of", LogFilter{Failed: true}, summaries, true},
		{"of an account's requests, of a day of its checks alone, as every kind is read",
			LogFilter{Accounts: []string{"work"}, Kinds: []RowKind{RowRequest}}, map[string]ledger.Summary{date: day(date, "work", foldOpus, 0, 2)}, true},
		{"of a model, of days of none", LogFilter{Models: []string{foldOpus}}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			days := &summariesOn{on: tt.on}
			if got := tt.f.mayKeepOn(days, date); got != tt.want {
				t.Errorf("%+v.mayKeepOn(%s), of summaries of %v, = %t, want %t", tt.f, date, slices.Sorted(maps.Keys(tt.on)), got, tt.want)
			}
			if want := [][]string{{"2026-10-07", date, "2026-10-09"}}; !reflect.DeepEqual(days.asked, want) {
				t.Errorf("%+v.mayKeepOn(%s) asked the summaries of %v, want %v", tt.f, date, days.asked, want)
			}
		})
	}
}

// A filter keeping every line, whatever its kinds, may keep a line on any
// day, as the Log reads such a day whole, reading no summary.
func TestAFilterKeepingEveryLineMayKeepOneOnAnyDayReadingNoSummary(t *testing.T) {
	for _, f := range []LogFilter{{}, LogOpening(), {Kinds: []RowKind{RowCheck}}} {
		days := &summariesOn{}
		if got := f.mayKeepOn(days, "2026-10-08"); !got || len(days.asked) > 0 {
			t.Errorf("%+v.mayKeepOn(2026-10-08) = %t, asking the summaries of %v; want true, asking none", f, got, days.asked)
		}
	}
}

// The rule a filter may keep a line by holds from the line's row to its
// chunk to the summaries of the day it's filed under, of the lines of
// rowsWorlds, made odd now and then, as oddly has them, and filed as
// filedAround files them, for drawnFilters filters a seed, as drawnFilter
// draws them: a row the filter keeps, as chose keeps it, its chunk's meta
// may keep, as mayKeep has it, the records of the day it arrived on cut
// into chunks of rowsAChunk in the order they arrived; the summary of the
// day its line is filed under may keep, as mayKeep has it of the filter
// with every kind; and so the day it arrived on may, as mayKeepOn has it of
// its summary and those of the days beside it.
func TestAFiltersMayKeepHoldsFromARowToItsChunkToItsDaysSummaries(t *testing.T) {
	var kept, beside, cut int
	for _, w := range rowsWorlds {
		for seed := range uint64(chainSeeds) {
			lines := w.lines(seed)
			r := rand.New(rand.NewPCG(seed, 0xc4a1))
			for i := range lines {
				oddly(r, &lines[i])
			}
			filed := filedAround(r, lines)
			metas, chunks := chunkMetasOf(t, lines)
			days := summariesFiled(t, lines, filed)
			rows := make([]RequestRow, len(lines))
			for i := range lines {
				rec := lineRecordOf(&lines[i])
				rows[i] = lineFacts(&rec.rowRecord, nil)
			}
			ids := idsOf(lines)
			for range drawnFilters {
				f := drawnFilter(r, ids)
				for i := range lines {
					if !f.chose(&rows[i]) {
						continue
					}
					kept++
					arrived := dayfile.DateOf(lines[i].At)
					if filed[i] != arrived {
						beside++
					}
					if problem := keptOtherwise(f, &metas[i], days, filed[i], arrived); problem != "" {
						t.Errorf("%s %d, %+v keeps line %d, %s, arrived on %s, filed under %s: %s", w.name, seed, f, i, lines[i].Request, arrived, filed[i], problem)
					}
				}
			}
			cut += chunks
		}
	}
	if kept == 0 || beside == 0 || cut == 0 {
		t.Errorf("filters kept %d lines, %d of them filed beside the day they arrived on, of days cut into another chunk %d times; want some of each", kept, beside, cut)
	}
}

// drawnFilters is how many filters a seed's lines are judged through.
const drawnFilters = 40

// keptOtherwise says how the rule a filter may keep a line by breaks of a
// line f keeps, whose chunk's meta is meta, filed under the day with the
// date filed, of days, arrived on the day with the date arrived: "" where
// its meta may keep it, the summary of the day it's filed under may, and
// those of the day it arrived on and the days beside it.
func keptOtherwise(f LogFilter, meta *chunkMeta, days *summariesOn, filed, arrived string) string {
	switch {
	case !meta.mayKeep(f):
		return fmt.Sprintf("its chunk's meta, of %+v, may keep none", meta.lines)
	case !f.through().mayKeep(days.on[filed]):
		return fmt.Sprintf("the summary of the day it's filed under, %+v, may keep none", days.on[filed])
	case !f.mayKeepOn(days, arrived):
		return "the summaries of the day it arrived on and those beside it may keep none"
	}
	return ""
}

// filedAround are the dates of the days lines are filed under, as r draws
// them: each the day it arrived on, but one in ten a day beside it, as a
// change of time zone files a line.
func filedAround(r *rand.Rand, lines []ledger.Line) []string {
	filed := make([]string, len(lines))
	for i := range lines {
		day := 0
		if r.IntN(10) == 0 {
			day = []int{-1, 1}[r.IntN(2)]
		}
		filed[i] = dayfile.DateOf(dayfile.DayStart(lines[i].At, day))
	}
	return filed
}

// chunkMetasOf are the metas of the chunks the records of lines are cut
// into, each line's its chunk's, as the rows of the day it arrived on are
// built of its records in the order they arrived, rowsAChunk a chunk, each
// read as its stored form holds it; and how many times a day's records were
// cut into another chunk.
func chunkMetasOf(t *testing.T, lines []ledger.Line) ([]chunkMeta, int) {
	t.Helper()
	byDay := make(map[string][]int)
	for _, i := range arrivalOrder(lines) {
		date := dayfile.DateOf(lines[i].At)
		byDay[date] = append(byDay[date], i)
	}
	metas, cut := make([]chunkMeta, len(lines)), 0
	for _, day := range byDay {
		var build rowsBuild
		for _, i := range day {
			rec := lineRecordOf(&lines[i])
			build.take(&rec)
		}
		built, err := build.built()
		if err != nil {
			t.Fatal(err)
		}
		for c, chunk := range built {
			meta, err := chunkMetaOf(chunk.Meta)
			if err != nil {
				t.Fatal(err)
			}
			for _, i := range day[c*rowsAChunk : c*rowsAChunk+chunk.Count] {
				metas[i] = meta
			}
		}
		cut += len(built) - 1
	}
	return metas, cut
}

// summariesFiled are the summaries of the days lines are filed under, each
// the dated day of filed's given, made as the ledger makes one of its lines,
// with no readings, as of the day's end.
func summariesFiled(t *testing.T, lines []ledger.Line, filed []string) *summariesOn {
	t.Helper()
	byDay := make(map[string][]ledger.Line)
	for i, date := range filed {
		byDay[date] = append(byDay[date], lines[i])
	}
	days := &summariesOn{on: make(map[string]ledger.Summary)}
	for date, lines := range byDay {
		_, end, _ := dayfile.Day(date)
		s, err := ledger.Summarise(date, slices.Values(lines), readings.Empty{}.Between, nil, ledger.CapsOf(nil, nil), end)
		if err != nil {
			t.Fatal(err)
		}
		days.on[date] = s
	}
	return days
}

// filterIDs are the ids a world's lines give a filter's choices to tick:
// their accounts, their models, and the sessions they count toward, each
// once, in order.
type filterIDs struct {
	accounts, models, sessions []string
}

// idsOf are the ids lines give, as filterIDs has them.
func idsOf(lines []ledger.Line) filterIDs {
	var ids filterIDs
	for i := range lines {
		l := &lines[i]
		ids.accounts, ids.models, ids.sessions = append(ids.accounts, l.Account), append(ids.models, l.Model), append(ids.sessions, l.CountedSession())
	}
	once := func(ids []string) []string {
		return slices.DeleteFunc(slices.Compact(slices.Sorted(slices.Values(ids))), func(id string) bool { return id == "" })
	}
	return filterIDs{accounts: once(ids.accounts), models: once(ids.models), sessions: once(ids.sessions)}
}

// drawnFilter is a filter of the Log's Requests as r draws it, of the ids
// given: each choice ticking none, or one or two of them, now and then with
// one no line gives; keeping those that failed alone now and then; and of
// any kinds.
func drawnFilter(r *rand.Rand, ids filterIDs) LogFilter {
	return LogFilter{
		Accounts: drawnChoice(r, ids.accounts), Models: drawnChoice(r, ids.models), Sessions: drawnChoice(r, ids.sessions),
		Kinds: slices.DeleteFunc(RowKinds(), func(RowKind) bool { return r.IntN(2) == 0 }), Failed: r.IntN(3) == 0,
	}
}

// drawnChoice is a filter's choice as r draws it, of the ids given, as
// drawnFilter has it: none ticked half the time.
func drawnChoice(r *rand.Rand, ids []string) []string {
	if len(ids) == 0 || r.IntN(2) == 0 {
		return nil
	}
	var choice []string
	for range 1 + r.IntN(2) {
		choice = append(choice, ids[r.IntN(len(ids))])
	}
	if r.IntN(5) == 0 {
		choice = append(choice, "none-such")
	}
	return slices.Compact(slices.Sorted(slices.Values(choice)))
}
