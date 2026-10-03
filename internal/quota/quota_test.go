package quota_test

import (
	"cmp"
	"encoding/json"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
)

func TestLength(t *testing.T) {
	tests := []struct {
		key    string
		want   time.Duration
		wantOK bool
	}{
		{key: "5h", want: 5 * time.Hour, wantOK: true},
		{key: "7d", want: 7 * 24 * time.Hour, wantOK: true},
		{key: "7d_oi", want: 7 * 24 * time.Hour, wantOK: true},
		{key: "12h_burst", want: 12 * time.Hour, wantOK: true},
		{key: "30d", want: 30 * 24 * time.Hour, wantOK: true},
		{key: "overage", wantOK: false},
		{key: "", wantOK: false},
		{key: "d", wantOK: false},
		{key: "0h", wantOK: false},
		{key: "5m", wantOK: false},
		{key: "7D", wantOK: false},
		{key: "7days", wantOK: false},
		{key: "5h30m", wantOK: false},
		{key: "_7d", wantOK: false},
		{key: "9999999999999999999999d", wantOK: false},
		{key: "3000000h", wantOK: false},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got, ok := quota.Length(tt.key)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("Length(%q) = %v, %v, want %v, %v", tt.key, got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestWindowSpan(t *testing.T) {
	resets := time.Date(2026, 9, 28, 18, 10, 0, 0, time.UTC)
	tests := []struct {
		name       string
		w          quota.Window
		wantStart  time.Time
		wantLength time.Duration
		wantOK     bool
	}{
		{name: "a whole length before its reset", w: quota.Window{Key: "5h", ResetsAt: resets}, wantStart: resets.Add(-5 * time.Hour), wantLength: 5 * time.Hour, wantOK: true},
		{name: "a week", w: quota.Window{Key: "7d_oi", ResetsAt: resets}, wantStart: resets.Add(-7 * 24 * time.Hour), wantLength: 7 * 24 * time.Hour, wantOK: true},
		{name: "from when it started again", w: quota.Window{Key: "5h", ResetsAt: resets, RestartedAt: resets.Add(-2 * time.Hour)}, wantStart: resets.Add(-2 * time.Hour), wantLength: 2 * time.Hour, wantOK: true},
		{name: "a whole length, started again before it began", w: quota.Window{Key: "5h", ResetsAt: resets, RestartedAt: resets.Add(-6 * time.Hour)}, wantStart: resets.Add(-5 * time.Hour), wantLength: 5 * time.Hour, wantOK: true},
		{name: "a whole length, started again at its reset", w: quota.Window{Key: "5h", ResetsAt: resets, RestartedAt: resets}, wantStart: resets.Add(-5 * time.Hour), wantLength: 5 * time.Hour, wantOK: true},
		{name: "none without a reset", w: quota.Window{Key: "5h"}},
		{name: "none without a length", w: quota.Window{Key: "overage", ResetsAt: resets}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			start, length, ok := tt.w.Span()
			if !start.Equal(tt.wantStart) || length != tt.wantLength || ok != tt.wantOK {
				t.Errorf("Span() = %v, %v, %v; want %v, %v, %v", start, length, ok, tt.wantStart, tt.wantLength, tt.wantOK)
			}
		})
	}
}

func TestSort(t *testing.T) {
	windows := windowsWithKeys("zeta", "7d_oi", "7d", "alpha", "1d", "5h", "12h", "7d_opus")

	quota.Sort(windows)
	want := []string{"5h", "12h", "1d", "7d", "7d_oi", "7d_opus", "alpha", "zeta"}
	if got := keys(windows); !slices.Equal(got, want) {
		t.Errorf("Sort() order = %v, want %v", got, want)
	}
}

func TestCompare(t *testing.T) {
	tests := []struct {
		a, b string
		// want is the sign of the result.
		want int
	}{
		{a: "5h", b: "7d", want: -1},
		{a: "7d", b: "5h", want: 1},
		{a: "7d", b: "7d_oi", want: -1},
		{a: "30d", b: "burst", want: -1},
		{a: "burst", b: "30d", want: 1},
		{a: "7d_oi", b: "7d_oi", want: 0},
	}
	for _, tt := range tests {
		t.Run(tt.a+" against "+tt.b, func(t *testing.T) {
			got := quota.Compare(quota.Window{Key: tt.a}, quota.Window{Key: tt.b})
			if cmp.Compare(got, 0) != tt.want {
				t.Errorf("Compare(%q, %q) = %d, want its sign to be %d", tt.a, tt.b, got, tt.want)
			}
			if got := quota.CompareKeys(tt.a, tt.b); cmp.Compare(got, 0) != tt.want {
				t.Errorf("CompareKeys(%q, %q) = %d, want its sign to be %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestMergeMax(t *testing.T) {
	session := quota.Window{Key: "5h", Label: "Session", Utilization: 0.20, ResetsAt: time.Unix(1790000000, 0), Status: quota.StatusAllowed}
	sessionLater := quota.Window{Key: "5h", Label: "Session", Utilization: 0.21, ResetsAt: time.Unix(1790000060, 0), Status: quota.StatusAllowedWarning}
	week := quota.Window{Key: "7d", Label: "Week", Utilization: 0.93}
	fableWeek := quota.Window{Key: "7d_oi", Label: "Fable week", Utilization: 0.05}
	tests := []struct {
		name string
		a, b []quota.Window
		want []quota.Window
	}{
		{
			name: "disjoint readings, in order",
			a:    []quota.Window{fableWeek},
			b:    []quota.Window{week, session},
			want: []quota.Window{session, week, fableWeek},
		},
		{
			name: "higher utilization kept whole, from the second reading",
			a:    []quota.Window{session, week},
			b:    []quota.Window{sessionLater, fableWeek},
			want: []quota.Window{sessionLater, week, fableWeek},
		},
		{
			name: "higher utilization kept whole, from the first reading",
			a:    []quota.Window{sessionLater, week},
			b:    []quota.Window{session},
			want: []quota.Window{sessionLater, week},
		},
		{
			name: "one side empty",
			a:    nil,
			b:    []quota.Window{week, session},
			want: []quota.Window{session, week},
		},
		{
			name: "both empty",
			want: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a, b := slices.Clone(tt.a), slices.Clone(tt.b)

			got := quota.MergeMax(a, b)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("MergeMax() =\n%+v\nwant\n%+v", got, tt.want)
			}
			if !reflect.DeepEqual(a, tt.a) || !reflect.DeepEqual(b, tt.b) {
				t.Error("MergeMax() modified its arguments")
			}
		})
	}
}

func TestJSON(t *testing.T) {
	usage := quota.Usage{
		Windows: []quota.Window{
			{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: time.Date(2026, 9, 28, 18, 10, 0, 0, time.UTC), Status: quota.StatusAllowed},
			{Key: "7d", Label: "Week", Utilization: 0},
		},
		Failures: []quota.Failure{{Label: "Fable", Window: "7d_oi", Error: "HTTP 529 · Overloaded"}},
	}
	want := `{"windows":[` +
		`{"key":"5h","label":"Session","utilization":0.23,"resets_at":"2026-09-28T18:10:00Z","status":"allowed"},` +
		`{"key":"7d","label":"Week","utilization":0}],` +
		`"failures":[{"label":"Fable","window":"7d_oi","error":"HTTP 529 · Overloaded"}]}`

	got, err := json.Marshal(usage)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if string(got) != want {
		t.Errorf("Marshal() =\n%s\nwant\n%s", got, want)
	}
	if got, _ := json.Marshal(quota.Usage{}); string(got) != "{}" {
		t.Errorf("Marshal(empty Usage) = %s, want {}", got)
	}
}

func windowsWithKeys(keys ...string) []quota.Window {
	windows := make([]quota.Window, 0, len(keys))
	for _, key := range keys {
		windows = append(windows, quota.Window{Key: key})
	}
	return windows
}

func keys(windows []quota.Window) []string {
	keys := make([]string, 0, len(windows))
	for _, w := range windows {
		keys = append(keys, w.Key)
	}
	return keys
}
