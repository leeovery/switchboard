package ledger

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestMergedDaysKeepEveryTotalOfBoth(t *testing.T) {
	at := func(hour int) time.Time { return time.Date(2026, 10, 5, hour, 0, 0, 0, time.UTC) }
	minutes := func(n int) *int { return &n }
	hours := func(at, n int) *[24]int {
		var h [24]int
		h[at] = n
		return &h
	}
	a := AccountDay{
		Account: "wk7q",
		Models: []ModelDay{
			{Model: "claude-opus-5-5", Upstream: 2, NoUsage: 1, Unsent: 1, Checks: 1, Counts: 1, Hours: hours(9, 2), Sessions: 1, SessionIDs: []string{"s1"},
				Usage: json.RawMessage(`{"input_tokens":10,"cache_creation":{"ephemeral_5m_input_tokens":4}}`)},
			{Model: "claude-opus-5-5", InferenceGeo: "us", Upstream: 1, Hours: hours(9, 1), Sessions: 1, SessionIDs: []string{"s1"}},
		},
		Sessions: 2, SessionIDs: []string{"s1"}, MovedOn: 1, MovedOff: 2,
		Highest:      map[string]float64{"5h": 0.4, "7d": 0.6},
		Rise:         map[string]float64{"5h": 0.1},
		Resets:       []Reset{{Window: "5h", At: at(14), Before: 0.4}},
		Limits:       []Limit{{Window: "5h", At: at(12)}},
		MinutesAtCap: minutes(5), MinutesAtLimit: minutes(3),
		ReadBefore: []string{"5h"},
	}
	b := AccountDay{
		Account: "wk7q",
		Models: []ModelDay{
			{Model: "claude-haiku-4-5", Upstream: 3, Hours: hours(10, 3), Sessions: 1, SessionIDs: []string{"s3"}},
			{Model: "claude-opus-5-5", Upstream: 1, Unsent: 2, Hours: hours(10, 1), Sessions: 2, SessionIDs: []string{"s1", "s2"},
				Usage: json.RawMessage(`{"input_tokens":5,"output_tokens":7,"cache_creation":{"ephemeral_5m_input_tokens":1}}`)},
		},
		Sessions: 3, SessionIDs: []string{"s1", "s2", "s3"}, MovedOn: 2,
		Highest:      map[string]float64{"5h": 0.7},
		Rise:         map[string]float64{"5h": 0.2, "7d": 0.05},
		Resets:       []Reset{{Window: "7d", At: at(8), Before: 0.9}},
		Limits:       []Limit{{Window: "7d", At: at(16)}},
		MinutesAtCap: minutes(10),
		ReadBefore:   []string{"7d", "5h"},
	}
	want := AccountDay{
		Account: "wk7q",
		Models: []ModelDay{
			{Model: "claude-haiku-4-5", Upstream: 3, Hours: hours(10, 3), Sessions: 1, SessionIDs: []string{"s3"}},
			{Model: "claude-opus-5-5", Upstream: 3, NoUsage: 1, Unsent: 3, Checks: 1, Counts: 1, Hours: func() *[24]int { h := hours(9, 2); h[10] = 1; return h }(),
				Sessions: 2, SessionIDs: []string{"s1", "s2"},
				Usage: json.RawMessage(`{"cache_creation":{"ephemeral_5m_input_tokens":5},"input_tokens":15,"output_tokens":7}`)},
			{Model: "claude-opus-5-5", InferenceGeo: "us", Upstream: 1, Hours: hours(9, 1), Sessions: 1, SessionIDs: []string{"s1"}},
		},
		Sessions: 4, SessionIDs: []string{"s1", "s2", "s3"}, MovedOn: 3, MovedOff: 2,
		Highest:      map[string]float64{"5h": 0.7, "7d": 0.6},
		Rise:         map[string]float64{"5h": 0.30000000000000004, "7d": 0.05},
		Resets:       []Reset{{Window: "7d", At: at(8), Before: 0.9}, {Window: "5h", At: at(14), Before: 0.4}},
		Limits:       []Limit{{Window: "5h", At: at(12)}, {Window: "7d", At: at(16)}},
		MinutesAtCap: minutes(15), MinutesAtLimit: minutes(3),
		ReadBefore: []string{"5h", "7d"},
	}
	if got := mergedDays(a, b); !reflect.DeepEqual(got, want) {
		t.Errorf("mergedDays() =\n%+v\nwant\n%+v", got, want)
	}
}

func TestMergedSessionsCountThoseUnnamedApart(t *testing.T) {
	tests := []struct {
		name           string
		countA, countB int
		idsA, idsB     []string
		wantCount      int
		wantIDs        []string
	}{
		{name: "both named", countA: 2, idsA: []string{"s1", "s2"}, countB: 2, idsB: []string{"s2", "s3"}, wantCount: 3, wantIDs: []string{"s1", "s2", "s3"}},
		{name: "some unnamed", countA: 3, idsA: []string{"s1"}, countB: 1, idsB: []string{"s1"}, wantCount: 3, wantIDs: []string{"s1"}},
		{name: "one never named", countA: 2, idsA: nil, countB: 1, idsB: []string{"s1"}, wantCount: 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			count, ids := mergedSessions(tt.countA, tt.idsA, tt.countB, tt.idsB)
			if count != tt.wantCount || !reflect.DeepEqual(ids, tt.wantIDs) {
				t.Errorf("mergedSessions() = %d, %q, want %d, %q", count, ids, tt.wantCount, tt.wantIDs)
			}
		})
	}
}

func TestMergedDaysOfFiguresOneNeverReadAreTheOthers(t *testing.T) {
	five := 5
	got := mergedDays(AccountDay{Account: "wk7q", MinutesAtCap: &five, Rise: map[string]float64{"5h": 0.1}}, AccountDay{Account: "wk7q"})
	if got.MinutesAtCap == nil || *got.MinutesAtCap != 5 || got.MinutesAtLimit != nil || !reflect.DeepEqual(got.Rise, map[string]float64{"5h": 0.1}) {
		t.Errorf("mergedDays() = %+v, want the minutes and rises the one that read them gave, and none never read of either", got)
	}
}
