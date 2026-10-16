package watch

import (
	"maps"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/views"
)

// byAccountWithGraphs has the Overview group its windows by account, GRAPHS
// shown, so the pool's charts sit beside Σ pool.
func byAccountWithGraphs(_ *runwayed, cfg *Config) {
	cfg.By, cfg.Sections = dashboard.ByAccount, dashboard.Sections{}.Toggled(dashboard.SectionGraphs)
}

// By account, GRAPHS shown, the Overview has the readings history's live
// tail read, as Runway does, and the pool's room in each window it charts
// beside Σ pool built of it, each column before now's known from work's one
// reading, two hours back, on, and now's Σ pool's; by window, it has nothing
// read, and charts no pool.
func TestThePoolsChartsAreJudgedByTheReadingsHistorysLiveTail(t *testing.T) {
	readAt := start.Add(-2 * time.Hour)
	r := runwayHarness(t, dashboard.Overview, byAccountWithGraphs, filedOn("work", "5h", readAt, 0.3))
	if want := []dashboard.PoolChart{{Key: "5h", Columns: 25}, {Key: "7d", Columns: 25}}; !slices.Equal(r.model.pool.charts, want) {
		t.Fatalf("the pool's charts are %v, want %v", r.model.pool.charts, want)
	}
	if !r.model.runwayTab.live.span.is(liveSpan(start)) || r.filed.tails == 0 {
		t.Fatalf("the live tail spans %+v, read %d times, want it over %+v, read", r.model.runwayTab.live.span, r.filed.tails, liveSpan(start))
	}
	room := r.model.pool.rooms["5h"]
	step := 5 * time.Hour / (25 - 1)
	end := start.Truncate(step)
	for c, col := range room.Columns[:24] {
		at := end.Add(-time.Duration(23-c) * step)
		if want := !at.Before(readAt); col.Known != want {
			t.Errorf("the pool's 5-hour column %d, at %s, is known %t, want %t", c, at.Format("15:04"), col.Known, want)
		}
	}
	if now, want := room.Columns[24], poolRoomAt(r, "5h"); !now.Known || now.Room != want {
		t.Errorf("the pool's 5-hour room at now is %v, known %t, want Σ pool's, %v", now.Room, now.Known, want)
	}

	byWindow := runwayHarness(t, dashboard.Overview, nil, filedOn("work", "5h", readAt, 0.3))
	if len(byWindow.model.pool.charts) != 0 || len(byWindow.filed.starts) != 0 {
		t.Errorf("by window, the pool's charts are %v, and %d tails were made, want none", byWindow.model.pool.charts, len(byWindow.filed.starts))
	}
}

// poolRoomAt is the pool's room in the window with the given key, as the
// moment of the document on screen has it now.
func poolRoomAt(r *runwayed, key string) float64 {
	for _, w := range r.model.moment(r.clock.now).Pool().Windows {
		if w.Key == key {
			return w.Room
		}
	}
	return -1
}

// The pool's room is built once a look, a chart at a time, as it takes in a
// read of the router's document, or of the tail; a frame builds none; and on
// another tab, none is built, the room built kept for when it shows again.
func TestThePoolsRoomIsBuiltAtALookAndKept(t *testing.T) {
	r := runwayHarness(t, dashboard.Overview, byAccountWithGraphs)
	var builds int
	r.model.pool.look = func(p views.PoolRoom, from views.PoolRoomFrom) views.PoolRoom {
		builds++
		return p.Look(from)
	}
	r.update(frameMsg{gen: r.model.frames})
	if builds != 0 {
		t.Errorf("a frame built the pool's room %d times, want none", builds)
	}
	r.lookAt(start.Add(interval))
	if builds < 2 {
		t.Errorf("a look built the pool's room %d times, want once a chart at the least", builds)
	}
	r.model.view = dashboard.History
	built := builds
	r.lookAt(start.Add(2 * interval))
	if builds != built || len(r.model.pool.charts) != 0 || len(r.model.pool.rooms) != 2 {
		t.Errorf("on another tab, a look built the pool's room %d times, charting %v, keeping %d rooms; want none built or charted, both kept",
			builds-built, r.model.pool.charts, len(r.model.pool.rooms))
	}
}

// The pool's room in a window its charts no longer chart, as a plan change
// leaves them, is let go at the look that charts them without it, the rooms
// in the windows they still chart kept.
func TestThePoolsRoomInAWindowNoLongerChartedIsLetGo(t *testing.T) {
	withFable := func(r *runwayed, cfg *Config) {
		byAccountWithGraphs(r, cfg)
		cfg.Size = Size{Width: 220, Height: 50}
		accounts := three()
		for i := range accounts {
			accounts[i].Windows = append(accounts[i].Windows, fableWeek(0.5))
		}
		r.startRouter(routerDocument(accounts...))
	}
	r := runwayHarness(t, dashboard.Overview, withFable)
	if keys := slices.Sorted(maps.Keys(r.model.pool.rooms)); !slices.Equal(keys, []string{"5h", "7d", "7d_oi"}) {
		t.Fatalf("the pool's rooms are of %v, want the 5-hour, the week and the Fable week", keys)
	}
	r.startRouter(routerDocument(three()...))
	r.lookAt(start.Add(interval))
	if keys := slices.Sorted(maps.Keys(r.model.pool.rooms)); !slices.Equal(keys, []string{"5h", "7d"}) || len(r.model.pool.charts) != 2 {
		t.Errorf("with no account reading the Fable week, the pool's charts are %v, its rooms of %v, want the 5-hour's and the week's alone", r.model.pool.charts, keys)
	}
}

// The pool's charts alone have the live tail read again only once their past
// is judged through a moment after the read of what it holds was asked: a
// look before the 5-hour chart's columns move a step on reads nothing, as
// what the tail gained since is judged at none of them; the look as they
// move reads it, the column the step brings left not known till that read
// lands, though what the tail held before has work read by then, and judged
// of what the read gained once it has.
func TestThePoolsChartsReadTheTailAgainOnlyAsTheirPastMovesOn(t *testing.T) {
	r := runwayHarness(t, dashboard.Overview, byAccountWithGraphs, filedOn("work", "5h", start.Add(-2*time.Hour), 0.3))
	next := views.PoolRoomThrough("5h", 25, start).Add(5 * time.Hour / (25 - 1))
	if views.PoolRoomThrough("7d", 25, next).After(start) {
		t.Fatalf("the week's chart moves on by %s, before the 5-hour's", next.Format("15:04"))
	}
	read := filedOn("work", "5h", next.Add(-time.Minute), 0.6)
	r.filed.held = append(r.filed.held, read)
	tails := r.filed.tails
	r.clock.now = next.Add(-time.Second)
	r.deliver(r.press("R")...)
	if r.filed.tails != tails {
		t.Errorf("a look before the pool's charts moved on read the live tail %d times, want none", r.filed.tails-tails)
	}
	r.clock.now = next
	msgs := r.press("R")
	i := slices.IndexFunc(msgs, isFetched)
	if i < 0 {
		t.Fatalf("R read nothing of the router: %#v", msgs)
	}
	landing := r.update(msgs[i])
	if col := r.model.pool.rooms["5h"].Columns[23]; col.Known {
		t.Errorf("before the tail's read landed, the 5-hour chart's last column before now's, at %s, holds %v; want it not known till the read lands", next.Format("15:04"), col.Room)
	}
	r.deliver(slices.Concat(msgs[:i], msgs[i+1:], landing)...)
	if r.filed.tails != tails+1 {
		t.Errorf("the look as the 5-hour chart moved on read the live tail %d times, want once", r.filed.tails-tails)
	}
	columns := r.model.pool.rooms["5h"].Columns
	if col, before := columns[23], columns[22]; !col.Known || math.Abs(before.Room-col.Room-0.3) > 1e-9 {
		t.Errorf("once the read landed, the 5-hour chart's column at %s holds %v, known %t, the one before %v; want work's reading at %s judged, 0.3 less",
			next.Format("15:04"), col.Room, col.Known, before.Room, read.At.Format("15:04"))
	}
}
