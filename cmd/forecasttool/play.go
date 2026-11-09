package main

import (
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
)

// history is what a state directory recorded, as the playback takes it in:
// the readings history, whole, in the order read, never empty; and the
// request ledger's lines, oldest first, read as they're taken in.
type history struct {
	readings []readings.Reading
	lines    iter.Seq[ledger.Held]
}

// historyIn is the history the state directory stateDir holds by now's
// clock, of the accounts configured, as policy names their windows, failing
// where it holds no readings.
func historyIn(stateDir string, configured config.Accounts, policy score.Policy, now func() time.Time) (history, error) {
	r := readings.NewReader(stateDir, logger)
	first, err := r.FirstDay()
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return history{}, fmt.Errorf("%s holds no readings history", stateDir)
	case err != nil:
		return history{}, err
	}
	read := slices.Collect(r.Between(first, now()))
	if len(read) == 0 {
		return history{}, fmt.Errorf("%s's readings history holds no reading", stateDir)
	}
	lines := ledger.NewReader(stateDir, now, ledger.CapsOf(configured, policy.Shared), logger).Lines(time.Time{})
	return history{readings: read, lines: lines}, nil
}

// warmUp is how far into a history its forecasts start to be scored: the two
// days of the readings history a restarted router seeds its state from, so
// none is scored off a colder start than the router's own.
const warmUp = 48 * time.Hour

// played are the forecasts made at the hours of a history that are scored:
// the router's, as status works them out of the document a playback builds
// then, and the baseline's, of that document without the router's demand, as
// a week goes at the pace its use since it started sets. It plays from the
// first hour after the history's first reading to the hour of its last, and
// scores from the later of twoDaysIn, the first hour warmUp after that
// reading, and allRead, the first hour every account configured whose week
// the history reads has had it read by, as an account idle since before the
// history began goes unread in it till then, though the router knew it. An
// account whose week the history never reads, as unread names them, the
// router never read either, so the scores go without it. That first hour can
// be after the last, and is none, zero, where the history reads no account's
// week: then none is scored.
type played struct {
	forecasts, baseline []forecast
	from, to            time.Time
	twoDaysIn, allRead  time.Time
	unread              []string
}

// playBack plays h back through a playback of the accounts configured, as
// policy judges them, and returns the forecasts made at the hours scored, as
// played has them: at each hour, all that h holds by then taken in first.
func playBack(configured config.Accounts, policy score.Policy, h history) played {
	p := router.NewPlayback(configured, policy)
	next, stop := iter.Pull(h.lines)
	defer stop()
	line, more := next()
	read := h.readings
	first, last := read[0].At, read[len(read)-1].At
	pl := played{from: first.Truncate(time.Hour).Add(time.Hour), to: last.Truncate(time.Hour), twoDaysIn: hourFrom(first.Add(warmUp))}
	pl.allRead, pl.unread = readFrom(configured, policy.Perishable, read)
	for hour := pl.from; !hour.After(pl.to); hour = hour.Add(time.Hour) {
		for ; len(read) > 0 && !read[0].At.After(hour); read = read[1:] {
			p.Read(read[0])
		}
		for ; more && !line.At.After(hour); line, more = next() {
			p.Line(line.Line)
		}
		pl.take(p.Document(hour), policy, hour)
	}
	return pl
}

// take takes in the forecasts doc, built at hour, as policy judges it, makes,
// the router's and the baseline's, where hour is scored.
func (pl *played) take(doc status.Document, policy score.Policy, hour time.Time) {
	if from, ok := pl.scoredFrom(); !ok || hour.Before(from) {
		return
	}
	pl.forecasts = append(pl.forecasts, forecastsIn(doc, policy, hour)...)
	doc.Demand = nil
	pl.baseline = append(pl.baseline, forecastsIn(doc.WorkedOut(policy), policy, hour)...)
}

// scoredFrom is the first hour pl scores, as played says, reporting false
// where it scores none, as the history reads no account's week.
func (pl played) scoredFrom() (time.Time, bool) {
	if pl.allRead.IsZero() {
		return time.Time{}, false
	}
	if pl.allRead.After(pl.twoDaysIn) {
		return pl.allRead, true
	}
	return pl.twoDaysIn, true
}

// readFrom returns the first hour every account configured that read, in
// the order read, reads the window named key of has had it read by, none
// where read reads no account's, and the accounts it never reads it of, in
// the order configured.
func readFrom(configured config.Accounts, key string, read []readings.Reading) (time.Time, []string) {
	first := make(map[string]time.Time, len(configured))
	for _, r := range read {
		if _, seen := first[r.Account]; !seen && r.Key == key {
			first[r.Account] = r.At
		}
	}
	var last time.Time
	var unread []string
	for _, a := range configured {
		at, ok := first[a.ID]
		switch {
		case !ok:
			unread = append(unread, a.ID)
		case at.After(last):
			last = at
		}
	}
	return hourFrom(last), unread
}

// hourFrom is the first hour at t or after it.
func hourFrom(t time.Time) time.Time {
	hour := t.Truncate(time.Hour)
	if hour.Before(t) {
		return hour.Add(time.Hour)
	}
	return hour
}

// forecastsIn are where doc, at at, as policy judges it, has each account's
// shared week heading, of those it has a forecast of, as forecastOf says.
func forecastsIn(doc status.Document, policy score.Policy, at time.Time) []forecast {
	var forecasts []forecast
	for _, a := range doc.At(at, policy).Accounts {
		w, _ := a.Window(policy.Perishable)
		if f, ok := forecastOf(a.ID, w, at); ok {
			forecasts = append(forecasts, f)
		}
	}
	return forecasts
}

// forecastOf is where the account with the given id's week w, as it stands
// at at, is forecast to head, reporting false where nothing's forecast: of a
// week whose reset isn't known, or has passed since it was read; one that
// has reached its floor already, or holds its account back for any other
// reason; and one that heads nowhere the document says. A week that runs out
// ends then; one that heads short of its floor, at its reset.
func forecastOf(id string, w status.WindowNow, at time.Time) (forecast, bool) {
	f := forecast{week: weekOf(id, w.Read.ResetsAt), at: at, used: w.Used}
	switch {
	case w.Read.ResetsAt.IsZero() || w.ResetSinceRead:
		return forecast{}, false
	case w.Kind == status.WindowRunsOut:
		f.ends = w.Out.At
	case w.Kind == status.WindowOpen && w.Out.Kind == score.OnPace:
		f.ends = f.resets
	default:
		return forecast{}, false
	}
	return f, true
}
