package views

import (
	"slices"
	"sort"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
)

// foldColumns is how many columns a fold of a story takes.
const foldColumns = 5

// leastStretch is how many columns a stretch of a story takes at least to
// be drawn.
const leastStretch = 3

// The axis of a story has a time every half hour, or every hour once its
// columns stand for more than hourlyPast; but none within foldClear of a
// fold, in its first startClear or its last endClear, fewer than timesApart
// columns after the time before it, or starting more than rightClear
// columns from the width's end.
const (
	hourlyPast = 4 * time.Hour
	foldClear  = 20 * time.Minute
	startClear = 8 * time.Minute
	endClear   = 10 * time.Minute
	timesApart = 8
	rightClear = 10
)

// StoryColumns is a session's story laid across a width, as STORY draws it:
// its minutes, from its first to its newest, shared out evenly but for its
// folds, five columns each, so a column stands for exactly (minutes −
// folded) ÷ (width − 5 × folds) of them, never rounded, but never more than
// a minute, so a story younger than that is drawn from the left. Where its
// folds would take more than half the width, its oldest are merged into one.
type StoryColumns struct {
	// Rates are its columns' requests a minute, from its first, at column 0,
	// to its newest: each the mean over the minutes it stands for, a minute
	// cut by a column's edge shared by its overlap, a request counting in the
	// minute it was sent; none on a fold's columns. Its last stands for its
	// newest minute too.
	Rates []float64
	// Folds are the first column of each of its folds, oldest first.
	Folds []int
	// Moves are its moves, each at its marker's column, but those a fold
	// hides.
	Moves []StoryMove
	// Stretches are its stretches drawn, those leastStretch columns wide or
	// more, but those a fold hides.
	Stretches []StoryStretch
	// Axis are its axis's labels, by their columns.
	Axis  []AxisLabel
	scale storyScale
}

// StoryMove is a move on a story, at its marker's column: where the stretch
// it opens starts, a fold's last column where it came as the session came
// back after it.
type StoryMove struct {
	SessionMove
	Column int
}

// StoryStretch is a stretch on a story, from its first column to its last:
// from where it starts, a fold's last column where it starts as the session
// came back after it, to a column before the next starts, or its fold's first
// column where a fold comes first, or the story's last column.
type StoryStretch struct {
	Account     string
	First, Last int
}

// AxisLabel is a label on a story's axis: its column, and its text. End is
// set on the axis's last, "now", or "ended" once the session is over.
type AxisLabel struct {
	Column int
	Text   string
	End    bool
}

// Columns is s laid across width columns: none where it has no request, or
// the width hasn't room for it.
func (s Story) Columns(width int) StoryColumns {
	if s.Start.IsZero() {
		return StoryColumns{}
	}
	sc, ok := s.scaled(width)
	if !ok {
		return StoryColumns{}
	}
	c := StoryColumns{Rates: sc.rates(s.Sent), scale: sc}
	newest := len(c.Rates) - 1
	for j := range sc.folds {
		c.Folds = append(c.Folds, sc.foldFirst(j))
	}
	for _, m := range s.Moves {
		if !sc.hides(m.At) {
			c.Moves = append(c.Moves, StoryMove{SessionMove: m, Column: sc.opening(m.At)})
		}
	}
	c.Stretches = s.stretched(sc, newest)
	c.Axis = s.axis(sc, width, newest)
	return c
}

// At is the column the time t falls in: one a fold hides, the column of the
// last minute before it; one before its first, its first; and one after its
// newest, its newest.
func (c StoryColumns) At(t time.Time) int {
	if len(c.Rates) == 0 {
		return 0
	}
	return c.scale.at(t)
}

// Folded reports whether the column given is one of a fold's.
func (c StoryColumns) Folded(column int) bool {
	return slices.ContainsFunc(c.Folds, func(first int) bool { return column >= first && column < first+foldColumns })
}

// storyScale is where a story's minutes fall across its columns: its first
// minute; its folds, oldest first, the minutes each folds away, and the base
// column each comes before, the columns counted with the folds' left out;
// how many minutes its columns stand for, from its first to its newest, the
// folded left out; and the edges of its base columns, in those minutes from
// its first, as shares, each column's start, then the last one's end.
type storyScale struct {
	start  time.Time
	folds  []Fold
	folded []int
	before []int
	total  int
	edges  []float64
}

// scaled is s's scale across width columns, false where they haven't room
// for it.
func (s Story) scaled(width int) (storyScale, bool) {
	sc := storyScale{start: s.Start.Truncate(time.Minute), folds: mergedFolds(s.Folds, width)}
	for _, f := range sc.folds {
		n := sc.minute(f.To) - sc.minute(f.From) - 1
		sc.folded, sc.total = append(sc.folded, n), sc.total-n
	}
	sc.total += sc.minute(s.End)
	base := width - foldColumns*len(sc.folds)
	perColumn := columnSpanOf(sc.total, base)
	last := min(perColumn.column(sc.total), base-1)
	if last < len(sc.folds) {
		return storyScale{}, false
	}
	sc.edges = make([]float64, last+2)
	for b := range last + 1 {
		sc.edges[b] = perColumn.start(b)
	}
	sc.edges[last+1] = float64(sc.total + 1)
	// A fold comes before the column its come-back minute falls in, but after
	// the fold before it and with a column left for each after it, so every
	// stretch of minutes between folds keeps a column of its own.
	prev := 0
	for j, f := range sc.folds {
		back := sc.unfolded(f.To)
		b := min(max(perColumn.column(back), prev+1), last-(len(sc.folds)-1-j))
		sc.edges[b], sc.before, prev = float64(back), append(sc.before, b), b
	}
	return sc, true
}

// columnSpan is how many minutes a story's column stands for, as a
// fraction, its minutes over its columns: a minute where it has no more
// minutes than columns, so it's never more than a column a minute, or has no
// columns.
type columnSpan struct {
	minutes, columns int
}

// columnSpanOf is the span of a column of a story of total minutes, its
// folds left out, laid across base columns.
func columnSpanOf(total, base int) columnSpan {
	if total <= base || base < 1 {
		return columnSpan{minutes: 1, columns: 1}
	}
	return columnSpan{minutes: total, columns: base}
}

// column is the column the minute m, from the story's first, falls in. It's
// worked out in whole numbers, as start is, so a minute on a column's edge
// is never put in the column before it, which would stand for no minutes.
func (s columnSpan) column(m int) int {
	return m * s.columns / s.minutes
}

// start is where the column b starts, in minutes from the story's first.
func (s columnSpan) start(b int) float64 {
	return float64(b*s.minutes) / float64(s.columns)
}

// mergedFolds are folds, oldest first, as a story width columns wide draws
// them: the oldest merged into one, from the first's start to the last's
// end, where they'd take more than half the width.
func mergedFolds(folds []Fold, width int) []Fold {
	most := max(width/(2*foldColumns), 1)
	if len(folds) <= most {
		return folds
	}
	cut := len(folds) - most
	return slices.Concat([]Fold{{From: folds[0].From, To: folds[cut].To}}, folds[cut+1:])
}

// minute is how many minutes from the story's first the minute t falls in
// is.
func (sc storyScale) minute(t time.Time) int {
	return int(t.Truncate(time.Minute).Sub(sc.start) / time.Minute)
}

// unfolded is how many minutes from the story's first the minute t falls in
// is, those folded before it left out: of a time a fold hides, the minute
// before the fold.
func (sc storyScale) unfolded(t time.Time) int {
	gone := 0
	for j, f := range sc.folds {
		if t.Before(f.To) {
			if t.After(f.From) {
				t = f.From
			}
			break
		}
		gone += sc.folded[j]
	}
	return sc.minute(t) - gone
}

// hides reports whether a fold hides the time t: after the last request
// before it, and before the first after it.
func (sc storyScale) hides(t time.Time) bool {
	_, hidden := sc.hiding(t)
	return hidden
}

// base is the base column the unfolded minute u falls in.
func (sc storyScale) base(u int) int {
	n := len(sc.edges) - 1
	return min(sort.Search(n, func(b int) bool { return sc.edges[b+1] > float64(u) }), n-1)
}

// drawn is the column the base column b is drawn at, after the folds before
// it.
func (sc storyScale) drawn(b int) int {
	folds := 0
	for _, before := range sc.before {
		if before <= b {
			folds++
		}
	}
	return b + foldColumns*folds
}

// foldFirst is the first column of the fold numbered j, from 0 at the
// oldest.
func (sc storyScale) foldFirst(j int) int {
	return sc.before[j] + foldColumns*j
}

// at is the column the time t falls in.
func (sc storyScale) at(t time.Time) int {
	return sc.drawn(sc.base(sc.unfolded(t)))
}

// opening is the column what starts at t starts at: the last of a fold's
// columns where it's when the session came back after it, else the column t
// falls in.
func (sc storyScale) opening(t time.Time) int {
	for j, f := range sc.folds {
		if t.Equal(f.To) {
			return sc.foldFirst(j) + foldColumns - 1
		}
	}
	return sc.at(t)
}

// rates are the requests a minute of each column, of requests sent at the
// times given, as StoryColumns.Rates has them.
func (sc storyScale) rates(sent SentTimes) []float64 {
	perMinute := make([]int, sc.total+1)
	for t := range sent.All() {
		if u := sc.unfolded(t); !sc.hides(t) && u >= 0 && u <= sc.total {
			perMinute[u]++
		}
	}
	n := len(sc.edges) - 1
	rates := make([]float64, sc.drawn(n-1)+1)
	for b := range n {
		lo, hi := sc.edges[b], sc.edges[b+1]
		var sum float64
		for m := int(lo); float64(m) < hi; m++ {
			sum += float64(perMinute[m]) * (min(float64(m+1), hi) - max(float64(m), lo))
		}
		rates[sc.drawn(b)] = sum / (hi - lo)
	}
	return rates
}

// stretched are s's stretches as its columns on sc draw them, its newest
// column given.
func (s Story) stretched(sc storyScale, newest int) []StoryStretch {
	shown := slices.DeleteFunc(slices.Clone(s.Stretches), func(st Stretch) bool { return sc.hides(st.From) })
	var drawn []StoryStretch
	for i, st := range shown {
		first, last := sc.opening(st.From), newest
		if j := slices.IndexFunc(sc.folds, func(f Fold) bool { return !f.From.Before(st.From) }); j >= 0 {
			last = sc.foldFirst(j)
		}
		if i+1 < len(shown) {
			last = min(last, sc.opening(shown[i+1].From)-1)
		}
		if last-first+1 >= leastStretch {
			drawn = append(drawn, StoryStretch{Account: st.Account, First: first, Last: last})
		}
	}
	return drawn
}

// axisLabels are the labels on a story's axis, as they're placed.
type axisLabels []AxisLabel

// place places the label l, where it fits in width columns, a cell clear of
// each placed, reporting whether it does.
func (a *axisLabels) place(l AxisLabel, width int) bool {
	end := l.Column + len(l.Text)
	if l.Column < 0 || end > width {
		return false
	}
	for _, p := range *a {
		if l.Column <= p.Column+len(p.Text) && p.Column <= end {
			return false
		}
	}
	*a = append(*a, l)
	return true
}

// axis are s's axis's labels as its columns on sc draw them, width columns
// wide, its newest column given, by their columns: its first time at column
// 0; "now", or "ended", at the width's end, or a cell after its newest
// column where it's younger, but after its first time; at each fold its
// hours, at its first column, and when it came back, from the column after
// it; and its times, as times places them. None overlaps another, a cell
// apart at least, those placed first kept.
func (s Story) axis(sc storyScale, width, newest int) []AxisLabel {
	var a axisLabels
	start := AxisLabel{Text: prose.TimeOfDay(s.End, s.Start)}
	a.place(start, width)
	end := AxisLabel{Text: "now", End: true}
	if s.Ended {
		end.Text = "ended"
	}
	end.Column = width - len(end.Text)
	if newest < width-1 {
		end.Column = min(max(newest+1, len(start.Text)+1), end.Column)
	}
	a.place(end, width)
	for j, f := range sc.folds {
		a.place(AxisLabel{Column: sc.foldFirst(j), Text: prose.Hours(f.To.Sub(f.From))}, width)
		a.place(AxisLabel{Column: sc.foldFirst(j) + foldColumns, Text: prose.TimeOfDay(s.End, f.To)}, width)
	}
	s.times(sc, width, &a)
	slices.SortFunc(a, func(x, y AxisLabel) int { return x.Column - y.Column })
	return a
}

// times places the times on s's axis, as its columns on sc draw them, width
// columns wide: each half hour by the clock, or each hour once its columns
// stand for more than hourlyPast, at its own column, but those the axis's
// rules leave out.
func (s Story) times(sc storyScale, width int, a *axisLabels) {
	step := 30 * time.Minute
	if time.Duration(sc.total)*time.Minute > hourlyPast {
		step = time.Hour
	}
	end := s.End.Truncate(time.Minute)
	from := s.Start.In(s.End.Location())
	t := time.Date(from.Year(), from.Month(), from.Day(), from.Hour(), 0, 0, 0, from.Location())
	prev := -timesApart
	for ; !t.After(end); t = t.Add(step) {
		if f, ok := sc.hiding(t); ok {
			t = t.Add(f.To.Sub(t).Truncate(step))
			continue
		}
		column := sc.at(t)
		if t.Sub(sc.start) < startClear || end.Sub(t) < endClear || sc.nearFold(t) || column-prev < timesApart || column > width-rightClear {
			continue
		}
		if a.place(AxisLabel{Column: column, Text: prose.TimeOfDay(s.End, t)}, width) {
			prev = column
		}
	}
}

// hiding is the fold that hides the time t, false where none does.
func (sc storyScale) hiding(t time.Time) (Fold, bool) {
	i := slices.IndexFunc(sc.folds, func(f Fold) bool { return t.After(f.From) && t.Before(f.To) })
	if i < 0 {
		return Fold{}, false
	}
	return sc.folds[i], true
}

// nearFold reports whether the time t is within foldClear of a fold's
// either end.
func (sc storyScale) nearFold(t time.Time) bool {
	return slices.ContainsFunc(sc.folds, func(f Fold) bool {
		return t.Sub(f.From).Abs() < foldClear || t.Sub(f.To).Abs() < foldClear
	})
}
