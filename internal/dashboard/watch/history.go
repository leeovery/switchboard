package watch

import (
	"context"
	"errors"
	"maps"
	"slices"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// The steps GET /history is asked for, as the charts draw them: the 5-hour
// window's at five minutes, and a window longer than shortUpTo, as a week,
// at half an hour; but never shorter than lets the router give a window's
// whole length in mostPoints, as it refuses to.
const (
	shortStep  = 5 * time.Minute
	longStep   = 30 * time.Minute
	shortUpTo  = 24 * time.Hour
	mostPoints = 1000
)

// mostSeen is how many of a window's readings the watch keeps of those it has
// seen itself: the oldest go first, as the router's history, where there is
// one, holds them anyway.
const mostSeen = 1000

// history is how the accounts' windows have been used, as the watch keeps it
// for the charts: each window's as the router gave it at the last full read,
// or without the router, as the readings history's files held it at the
// last; and the readings the watch has seen itself, which carry those on
// since.
type history struct {
	// asked counts the times the router, or the readings history's files,
	// have been asked: an answer to one a later asking overtook is dropped.
	asked int
	// router is each window's history, by its key, as the router last gave
	// it.
	router map[string]router.History
	// filed are the readings of each account's windows the readings
	// history's files held when last read, the oldest first.
	filed map[dashboard.Ref][]score.Reading
	// filedSince is set once the files have been read since probing began:
	// the readings the watch sees itself carry them on from then, so they're
	// read again only once probing begins again.
	filedSince bool
	// outdated is set while the router is from before GET /history.
	outdated bool
	// seen are the readings of each account's windows the watch has seen
	// itself, the oldest first.
	seen map[dashboard.Ref][]score.Reading
}

// historyMsg is what asking the router for its history found: each window's,
// by its key, and why one couldn't be had, the last that couldn't.
type historyMsg struct {
	asked int
	got   map[string]router.History
	err   error
}

// filedMsg is what reading the readings history's files found: each
// account's windows' readings, the oldest first.
type filedMsg struct {
	asked int
	got   map[dashboard.Ref][]score.Reading
}

// stepFor is the step the history of the window with the given key is asked
// for at, as history says; reporting false for a window whose key doesn't
// give its length.
func stepFor(key string) (time.Duration, bool) {
	length, ok := quota.Length(key)
	if !ok {
		return 0, false
	}
	step := longStep
	if length <= shortUpTo {
		step = shortStep
	}
	least := (length + mostPoints - 1) / mostPoints
	return max(step, least.Truncate(time.Minute)+time.Minute), true
}

// keysOf are the keys of doc's windows that have a step to ask the history
// at, each once, in the order the accounts first have them.
func keysOf(doc status.Document) []string {
	var keys []string
	for _, a := range doc.Accounts {
		for _, w := range a.Windows {
			if _, ok := stepFor(w.Key); ok && !slices.Contains(keys, w.Key) {
				keys = append(keys, w.Key)
			}
		}
	}
	return keys
}

// askHistory asks the router for the history of doc's windows, as the next
// asking, as histories asks it.
func (m Model) askHistory(doc status.Document) (Model, tea.Cmd) {
	m.history.asked++
	asked, ctx, source := m.history.asked, m.ctx, m.cfg.Source
	return m, func() tea.Msg {
		return histories(ctx, source, doc, asked)
	}
}

// histories asks the source for the history of doc's windows, each at its
// step, as the asking given: a router from before GET /history stops it,
// there being nothing more to ask of it.
func histories(ctx context.Context, source Source, doc status.Document, asked int) historyMsg {
	msg := historyMsg{asked: asked, got: make(map[string]router.History)}
	for _, key := range keysOf(doc) {
		step, _ := stepFor(key)
		got, err := source.History(ctx, key, step)
		if err != nil {
			msg.err = err
			if errors.Is(err, router.ErrNoHistory) {
				return msg
			}
			continue
		}
		msg.got[key] = got
	}
	return msg
}

// readFiled reads the readings history's files for the readings of doc's
// windows as they run at now, as the next asking, where there's no router to
// ask.
func (m Model) readFiled(doc status.Document, now time.Time) (Model, tea.Cmd) {
	m.history.asked++
	m.history.filedSince = true
	asked, files := m.history.asked, m.cfg.Readings
	return m, func() tea.Msg {
		return filedMsg{asked: asked, got: filedOf(files, doc, now)}
	}
}

// filedOf are the readings the readings history's files hold of doc's
// windows, by account and window, read as the history verb reads them, from
// the day the earliest of those running at now started.
func filedOf(files views.Readings, doc status.Document, now time.Time) map[dashboard.Ref][]score.Reading {
	from := now
	for _, a := range doc.Accounts {
		for _, w := range a.Windows {
			if start, _, ok := w.Span(); ok && !a.HasLapsed(w) {
				from = score.Earlier(from, start)
			}
		}
	}
	got := make(map[dashboard.Ref][]score.Reading)
	for _, w := range views.NewWindows(files, nil, from, now).Windows {
		ref := dashboard.Ref{Account: w.Account, Window: w.Window}
		for _, r := range w.Readings {
			got[ref] = append(got[ref], score.Reading{At: r.At, Utilization: r.Utilization})
		}
	}
	return got
}

// filedAs takes in what reading the readings history's files found, unless a
// later asking overtook it.
func (h history) filedAs(msg filedMsg) history {
	if msg.asked == h.asked {
		h.filed = msg.got
	}
	return h
}

// answered takes in what asking the router found, unless a later asking
// overtook it: the windows' histories it gave, beside those it couldn't
// give again; and whether the router is from before GET /history.
func (h history) answered(msg historyMsg) history {
	if msg.asked != h.asked {
		return h
	}
	outdated := errors.Is(msg.err, router.ErrNoHistory)
	switch {
	case outdated && !h.outdated:
		logger.Info("the router is from before GET /history: the charts draw from what the watch reads itself, and it tells of no events")
	case msg.err != nil && !outdated:
		logger.Warn("couldn't read the router's history", "error", msg.err)
	}
	h.outdated = outdated
	h.router = maps.Clone(h.router)
	if h.router == nil {
		h.router = make(map[string]router.History)
	}
	maps.Copy(h.router, msg.got)
	return h
}

// saw takes in the readings of doc's windows: each window's use as read,
// when its account was read, one reading the same use again carrying the
// last on to then. A window's readings from before it last started go.
func (h history) saw(doc status.Document) history {
	seen := maps.Clone(h.seen)
	if seen == nil {
		seen = make(map[dashboard.Ref][]score.Reading)
	}
	for _, a := range doc.Accounts {
		for _, w := range a.Windows {
			start, _, ok := w.Span()
			if !ok || a.HasLapsed(w) || a.FetchedAt.IsZero() {
				continue
			}
			ref := dashboard.Ref{Account: a.ID, Window: w.Key}
			seen[ref] = record(since(seen[ref], start), score.Reading{At: a.FetchedAt, Utilization: w.Utilization})
		}
	}
	h.seen = seen
	return h
}

// record adds r to readings, a copy: as the newest, unless it's no newer than
// the newest, or carrying the newest on where it reads the same use. The
// oldest go past mostSeen.
func record(readings []score.Reading, r score.Reading) []score.Reading {
	n := len(readings)
	switch {
	case n > 0 && !r.At.After(lastRead(readings[n-1])):
		return readings
	case n > 0 && readings[n-1].Utilization == r.Utilization:
		readings = slices.Clone(readings)
		readings[n-1].Last = r.At
		return readings
	default:
		readings = append(slices.Clone(readings), r)
		return readings[max(len(readings)-mostSeen, 0):]
	}
}

// drawn is the history the charts draw of doc's windows, each running now,
// its readings in order: those the router gave, from when it started, or
// where it gave none, those the readings history's files held; then those
// the watch has seen after the last of them; or those the watch has seen
// alone, where neither holds any.
func (h history) drawn(doc status.Document) dashboard.Trails {
	drawn := make(dashboard.Trails)
	for _, a := range doc.Accounts {
		for _, w := range a.Windows {
			start, _, ok := w.Span()
			if !ok || a.HasLapsed(w) {
				continue
			}
			ref := dashboard.Ref{Account: a.ID, Window: w.Key}
			readings, seen := h.routers(ref, start), since(h.seen[ref], start)
			if len(readings) == 0 {
				readings = slices.Clone(since(h.filed[ref], start))
			}
			if n := len(readings); n > 0 {
				seen = beyond(seen, readings[n-1].At)
			}
			if readings = append(readings, seen...); len(readings) > 0 {
				drawn[ref] = dashboard.Trail{Readings: readings}
			}
		}
	}
	return drawn
}

// routers are the readings of the window ref names, from start, as the
// router last gave them.
func (h history) routers(ref dashboard.Ref, start time.Time) []score.Reading {
	var readings []score.Reading
	for _, a := range h.router[ref.Window].Accounts {
		if a.ID != ref.Account {
			continue
		}
		for _, p := range a.Points {
			if !p.At.Before(start) {
				readings = append(readings, score.Reading{At: p.At, Utilization: p.Utilization})
			}
		}
	}
	return readings
}

// since are the readings, in order, first read at t or after.
func since(readings []score.Reading, t time.Time) []score.Reading {
	return readings[firstIndex(readings, func(r score.Reading) bool { return !r.At.Before(t) }):]
}

// beyond are the readings, in order, first read after t.
func beyond(readings []score.Reading, t time.Time) []score.Reading {
	return readings[firstIndex(readings, func(r score.Reading) bool { return r.At.After(t) }):]
}

// firstIndex is the index of the first of readings that is, or their length
// where none is.
func firstIndex(readings []score.Reading, is func(score.Reading) bool) int {
	if i := slices.IndexFunc(readings, is); i >= 0 {
		return i
	}
	return len(readings)
}

// lastRead is when a reading's use was last read so.
func lastRead(r score.Reading) time.Time {
	if r.Last.After(r.At) {
		return r.Last
	}
	return r.At
}
