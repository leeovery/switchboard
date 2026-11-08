package forecast

import (
	"encoding/json"
	"math"
	"slices"
	"testing"
	"time"
)

// holdFrom is when the hold curve's tests start.
var holdFrom = time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)

// holdHours are the hours the hold curve has its points at.
var holdHours = []time.Duration{time.Hour, 3 * time.Hour, 6 * time.Hour, 12 * time.Hour, 24 * time.Hour, 48 * time.Hour}

// holdPoints are the hold curve's points with the shares given, at holdHours.
func holdPoints(shares ...float64) []Point {
	points := make([]Point, len(shares))
	for i, share := range shares {
		points[i] = Point{After: holdHours[i], Share: share}
	}
	return points
}

// defaultPoints are the default curve's points, e^(−h/6h).
func defaultPoints() []Point {
	shares := make([]float64, len(holdHours))
	for i, after := range holdHours {
		shares[i] = math.Exp(-after.Hours() / 6)
	}
	return holdPoints(shares...)
}

// nearPoints reports whether two curves' points lie at the same hours, with
// shares within a rounding of each other.
func nearPoints(a, b []Point) bool {
	return slices.EqualFunc(a, b, func(p, q Point) bool { return p.After == q.After && math.Abs(p.Share-q.Share) < 1e-12 })
}

func TestHoldPoints(t *testing.T) {
	// Answers weighing 30 in all, from sessions on their account for times at
	// a bin's edge or within it, and none from 6 to 12 hours.
	binned := Hold{}
	for _, a := range []struct {
		weight    float64
		onAccount time.Duration
	}{
		{1, 0}, {2, time.Hour}, {3, 2 * time.Hour}, {4, 3 * time.Hour},
		{5, 23 * time.Hour}, {6, 47 * time.Hour}, {9, 72 * time.Hour},
	} {
		binned = binned.Answered(a.weight, a.onAccount, holdFrom)
	}
	// One answer from a session new to its account, then a week on one from
	// a session on its account for two days, weighing e^(−1) as much.
	aged := Hold{}.Answered(1, 0, holdFrom).Answered(1, 48*time.Hour, holdFrom.Add(7*24*time.Hour))
	tests := []struct {
		name string
		hold Hold
		at   time.Time
		want []Point
	}{
		{"no answers, the default curve", Hold{}, holdFrom, defaultPoints()},
		{"under a day of answers, the default curve", binned, holdFrom.Add(24*time.Hour - time.Minute), defaultPoints()},
		{
			"a day of answers, each point the weight that long on or longer", binned, holdFrom.Add(24 * time.Hour),
			holdPoints(29.0/30, 24.0/30, 20.0/30, 20.0/30, 15.0/30, 9.0/30),
		},
		{
			"newer answers weigh more", aged, holdFrom.Add(7 * 24 * time.Hour),
			holdPoints(slices.Repeat([]float64{1 / (1 + 1/math.E)}, 6)...),
		},
		{
			"time on an account under none counts as none", Hold{}.Answered(1, -time.Hour, holdFrom), holdFrom.Add(24 * time.Hour),
			holdPoints(0, 0, 0, 0, 0, 0),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.hold.Points(tt.at); !nearPoints(got, tt.want) {
				t.Errorf("Points(%v) = %v, want %v", tt.at.Sub(holdFrom), got, tt.want)
			}
		})
	}
}

func TestHoldAnswered(t *testing.T) {
	tests := []struct {
		name string
		hold Hold
		want []Point
	}{
		{"an answer weighing nothing leaves the curve as it was", Hold{}.Answered(0, 2*time.Hour, holdFrom), defaultPoints()},
		{
			"an answer before the last counts as at the last",
			Hold{}.Answered(1, 0, holdFrom.Add(7*24*time.Hour)).Answered(1, 48*time.Hour, holdFrom),
			holdPoints(slices.Repeat([]float64{0.5}, 6)...),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			at := holdFrom.Add(8 * 24 * time.Hour)
			if got := tt.hold.Points(at); !nearPoints(got, tt.want) {
				t.Errorf("Points(8 days) = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHoldStoredForm(t *testing.T) {
	hold := Hold{}.Answered(2, 2*time.Hour, holdFrom).Answered(1, 30*time.Hour, holdFrom.Add(7*24*time.Hour))
	b, err := json.Marshal(hold)
	if err != nil {
		t.Fatalf("Marshal(%v) error = %v", hold, err)
	}
	want := `{"weights":[0,0.7357588823428847,0,0,0,1,0],"weighed_at":"2026-11-09T09:00:00Z","first_answer":"2026-11-02T09:00:00Z"}`
	if string(b) != want {
		t.Errorf("Marshal(hold) = %s, want %s", b, want)
	}

	var read Hold
	if err := json.Unmarshal(b, &read); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", b, err)
	}
	at := holdFrom.Add(8 * 24 * time.Hour)
	next := holdFrom.Add(7*24*time.Hour + time.Hour)
	if got, want := read.Answered(3, 5*time.Hour, next).Points(at), hold.Answered(3, 5*time.Hour, next).Points(at); !slices.Equal(got, want) {
		t.Errorf("read back and answered: Points(8 days) = %v, want %v", got, want)
	}
}

func TestHoldStoredFormRefusesAnotherCountOfBins(t *testing.T) {
	for _, stored := range []string{
		`{"weights":[0,1,0,0,0,0]}`,
		`{"weights":[0,1,0,0,0,0,0,0]}`,
		`{}`,
	} {
		t.Run(stored, func(t *testing.T) {
			var hold Hold
			if err := json.Unmarshal([]byte(stored), &hold); err == nil {
				t.Errorf("Unmarshal(%s) = %v, want an error", stored, hold)
			}
		})
	}
}

func TestStoredZeroHoldReadsBackAsTheDefault(t *testing.T) {
	b, err := json.Marshal(Hold{})
	if err != nil {
		t.Fatalf("Marshal(Hold{}) error = %v", err)
	}
	var read Hold
	if err := json.Unmarshal(b, &read); err != nil {
		t.Fatalf("Unmarshal(%s) error = %v", b, err)
	}
	if read != (Hold{}) {
		t.Errorf("Unmarshal(%s) = %v, want the zero Hold", b, read)
	}
}

func TestCurve(t *testing.T) {
	points := holdPoints(0.8, 0.5, 0.4, 0.3, 0.25, 0.2)
	reversed := slices.Clone(points)
	slices.Reverse(reversed)
	tests := []struct {
		name   string
		points []Point
		after  time.Duration
		want   float64
	}{
		{"1 at none", points, 0, 1},
		{"1 before none", points, -time.Hour, 1},
		{"straight from 1 to the first point", points, 30 * time.Minute, 0.9},
		{"at a point, its share", points, 3 * time.Hour, 0.5},
		{"straight between two points", points, 2 * time.Hour, 0.65},
		{"at the last point, its share", points, 48 * time.Hour, 0.2},
		{"past the last, falling straight to 0", points, 72 * time.Hour, 0.1},
		{"0 at twice the last point's hours", points, 96 * time.Hour, 0},
		{"0 beyond", points, 200 * time.Hour, 0},
		{"points in any order, in the order of their hours", reversed, 2 * time.Hour, 0.65},
		{"no points, the default curve", nil, 6 * time.Hour, 1 / math.E},
		{"no points, the default curve's 1 at none", nil, 0, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Curve(tt.points)(tt.after); math.Abs(got-tt.want) > 1e-12 {
				t.Errorf("Curve(%v)(%v) = %v, want %v", tt.points, tt.after, got, tt.want)
			}
		})
	}
}

func TestCurveLeavesItsPointsAsTheyWere(t *testing.T) {
	points := holdPoints(0.2, 0.5)
	slices.Reverse(points)
	want := slices.Clone(points)

	_ = Curve(points)(time.Hour)

	if !slices.Equal(points, want) {
		t.Errorf("Curve's points after = %v, want %v", points, want)
	}
}

func TestWeight(t *testing.T) {
	tests := []struct {
		name                                 string
		input, output, cacheWrite, cacheRead int64
		want                                 float64
	}{
		{"input weighs 1", 1, 0, 0, 0, 1},
		{"output weighs 5", 0, 1, 0, 0, 5},
		{"a cache write weighs 1.25", 0, 0, 1, 0, 1.25},
		{"a cache read weighs 0.1", 0, 0, 0, 1, 0.1},
		{"each adds to the whole", 100, 10, 8, 1000, 260},
		{"no tokens weigh nothing", 0, 0, 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Weight(tt.input, tt.output, tt.cacheWrite, tt.cacheRead); math.Abs(got-tt.want) > 1e-12 {
				t.Errorf("Weight(%d, %d, %d, %d) = %v, want %v", tt.input, tt.output, tt.cacheWrite, tt.cacheRead, got, tt.want)
			}
		})
	}
}
