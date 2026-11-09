package forecast

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/score"
)

const (
	// holdTau is the time constant answers' weight in the hold curve decays
	// with.
	holdTau = 7 * 24 * time.Hour
	// holdKnown is how long after its first answer the hold curve goes by its
	// answers: before, it's the default curve.
	holdKnown = 24 * time.Hour
	// holdBins is how many bins of their sessions' time on their account the
	// hold curve keeps answers' weight in.
	holdBins = 7
)

// holdEdges are where the hold curve's bins begin, the last running on
// without end. Each but the first is a point of the curve.
var holdEdges = [holdBins]time.Duration{
	0, time.Hour, 3 * time.Hour, 6 * time.Hour, 12 * time.Hour, 24 * time.Hour, 48 * time.Hour,
}

// Hold is the hold curve: of the use the router has sent, the share that
// came from sessions on their account for a while or more, so the share of
// an account's use still on it that long on, as its sessions stay. Each
// answer weighs as its tokens do, as Weight has it, and counts less as it
// ages, with a time constant of a week. Its zero value has taken in no
// answer, and is the default curve.
type Hold struct {
	// weights are the answers' weight in each bin of holdEdges, decayed to at.
	weights [holdBins]float64
	at      time.Time
	// first is when the first answer came.
	first time.Time
}

// Point is a point of the hold curve: the share of an account's use still on
// it After on.
type Point struct {
	After time.Duration
	Share float64
}

// Weight is how much an answer's tokens weigh, by the price ratios every
// model shares: input 1, output 5, cache writes 1.25 and cache reads 0.1.
func Weight(input, output, cacheWrite, cacheRead int64) float64 {
	return float64(input) + 5*float64(output) + 1.25*float64(cacheWrite) + 0.1*float64(cacheRead)
}

// Answered returns h having taken in an answer of weight, as Weight weighs
// it, at at, from a session on its account for onAccount. An answer before
// the last h took in counts as at the last, and one whose weight isn't one to
// take in, as countable says, leaves h as it was.
func (h Hold) Answered(weight float64, onAccount time.Duration, at time.Time) Hold {
	if !countable(weight) {
		return h
	}
	if h.first.IsZero() {
		h.first = at
	}
	at = score.Later(at, h.at)
	factor := decay(at.Sub(h.at), holdTau)
	for i := range h.weights {
		h.weights[i] *= factor
	}
	h.weights[binOf(onAccount)] += weight
	h.at = at
	return h
}

// binOf is the bin of holdEdges an answer from a session on its account for
// onAccount weighs in.
func binOf(onAccount time.Duration) int {
	for i := holdBins - 1; i > 0; i-- {
		if onAccount >= holdEdges[i] {
			return i
		}
	}
	return 0
}

// Learning reports whether the hold curve at at is still the default, as
// Points gives it: with less than a day between its first answer and at, or
// no weight taken in.
func (h Hold) Learning(at time.Time) bool {
	return h.first.IsZero() || at.Sub(h.first) < holdKnown || h.total() <= 0
}

// total is the weight of every answer h has taken in.
func (h Hold) total() float64 {
	var total float64
	for _, w := range h.weights {
		total += w
	}
	return total
}

// Points returns the hold curve at at, at 1, 3, 6, 12, 24 and 48 hours: each
// the weight of answers from sessions on their account that long or longer,
// over all the weight. While it's Learning, it's the default curve's points.
func (h Hold) Points(at time.Time) []Point {
	if h.Learning(at) {
		return defaultPoints()
	}
	total := h.total()
	points := make([]Point, holdBins-1)
	var held float64
	for i := holdBins - 1; i > 0; i-- {
		held += h.weights[i]
		points[i-1] = Point{After: holdEdges[i], Share: held / total}
	}
	return points
}

// Curve returns the hold curve points draw, in the order of their hours: 1
// at none, straight between each point and the next, and past the last
// falling straight to 0 at twice its hours. A point at none or before is left
// out, and with no points after none it's the default curve.
func Curve(points []Point) func(after time.Duration) float64 {
	knots := slices.DeleteFunc(slices.Clone(points), func(p Point) bool { return p.After <= 0 })
	if len(knots) == 0 {
		knots = defaultPoints()
	}
	slices.SortStableFunc(knots, func(a, b Point) int { return cmp.Compare(a.After, b.After) })
	knots = append(knots, Point{After: 2 * knots[len(knots)-1].After})
	return func(after time.Duration) float64 {
		from := Point{Share: 1}
		for _, to := range knots {
			if after <= to.After {
				return between(from, to, after)
			}
			from = to
		}
		return 0
	}
}

// between is the share at after, on the straight line from one point to the
// next, after lying no later than the next: the next's own where they lie
// together, and the first's where after lies before it.
func between(from, to Point, after time.Duration) float64 {
	if to.After <= from.After {
		return to.Share
	}
	if after <= from.After {
		return from.Share
	}
	return from.Share + (to.Share-from.Share)*float64(after-from.After)/float64(to.After-from.After)
}

// defaultPoints are the hold curve's points with less than a day of answers:
// all of an account's use taken to stay on it at each, so where it leans
// wrong, it leans to an early run-out rather than room that isn't there.
func defaultPoints() []Point {
	points := make([]Point, holdBins-1)
	for i, after := range holdEdges[1:] {
		points[i] = Point{After: after, Share: 1}
	}
	return points
}

// storedHold is a Hold as the router's state file stores it, as MarshalJSON
// writes it and UnmarshalJSON reads it back: each bin's weight, in the order
// of holdEdges, when they're decayed to, and when the first answer came.
type storedHold struct {
	Weights     []float64 `json:"weights"`
	WeighedAt   time.Time `json:"weighed_at,omitzero"`
	FirstAnswer time.Time `json:"first_answer,omitzero"`
}

// MarshalJSON writes h in its stored form.
func (h Hold) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(storedHold{Weights: h.weights[:], WeighedAt: h.at, FirstAnswer: h.first})
	if err != nil {
		return nil, fmt.Errorf("encode a hold curve: %w", err)
	}
	return b, nil
}

// UnmarshalJSON reads h back from its stored form, as MarshalJSON wrote it.
// null leaves h as it was. A form of another shape, as of another count of
// bins, or with a field missing or of another type, reads as the zero Hold,
// the default curve. Only bytes that aren't JSON are refused.
func (h *Hold) UnmarshalJSON(b []byte) error {
	if bytes.Equal(b, []byte("null")) {
		return nil
	}
	var stored storedHold
	err := json.Unmarshal(b, &stored)
	if _, malformed := errors.AsType[*json.SyntaxError](err); malformed {
		return fmt.Errorf("decode a hold curve: %w", err)
	}
	// Refusing a curve it can't read would set aside the whole state file it's
	// in; read as none, it's rebuilt from the answers that come next.
	if err != nil || len(stored.Weights) != holdBins || stored.WeighedAt.IsZero() || stored.FirstAnswer.IsZero() {
		*h = Hold{}
		return nil
	}
	*h = Hold{weights: [holdBins]float64(stored.Weights), at: stored.WeighedAt, first: stored.FirstAnswer}
	return nil
}
