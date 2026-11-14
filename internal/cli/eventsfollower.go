package cli

import (
	"context"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
)

// The follower of the router's events reads their files raw: a verb makes
// one, and follows it, through poolfiles.go alone, as
// poolfiles_guard_test.go holds it.

// eventsFollower follows the router's events' files, a day's at a time, as
// they grow, reading on from where it last ended.
type eventsFollower struct {
	files *dayfile.Files
	now   func() time.Time
	date  string
	tail  *dayfile.Tail[events.Held]
	// handed counts the lines of the day handed on, to the listing or
	// printed, and read those read since its files were last read afresh.
	handed, read int
}

// newEventsFollower returns a follower of files, the router's events', from
// the start of the local day with the given date, today's, none of its lines
// read yet: the listing reads them through it, as listing gives them, so it
// follows on from exactly where the listing ended, and no line is printed
// twice, nor missed.
func newEventsFollower(files *dayfile.Files, today string, now func() time.Time) *eventsFollower {
	f := &eventsFollower{files: files, now: now}
	f.open(today)
	return f
}

// listing is the day followed, as the listing reads it, through the
// follower.
func (f *eventsFollower) listing() events.Tailed {
	return events.Tailed{Date: f.date, Read: func(take func(events.Held)) int {
		f.readOn(take)
		_, unread := f.tail.Lines()
		return unread
	}}
}

// open follows the files of the local day with the given date, none of its
// lines handed on yet.
func (f *eventsFollower) open(date string) {
	f.date, f.tail, f.handed, f.read = date, dayfile.NewTail(f.files, date, events.HeldIn), 0, 0
}

// run hands print each line filed after those the follower began from,
// every interval given, until ctx ends or print fails.
func (f *eventsFollower) run(ctx context.Context, every time.Duration, print func(events.Held) error) error {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			var err error
			f.poll(func(h events.Held) {
				if err == nil {
					err = print(h)
				}
			})
			if err != nil {
				return err
			}
		}
	}
}

// poll hands take each line filed since the last poll: the rest of the day
// followed, then, a day at a time, each later day the router has filed
// under, up to today, read to its end, the last of them followed from then
// on.
func (f *eventsFollower) poll(take func(events.Held)) {
	for {
		next := f.nextDay()
		f.readOn(take)
		if next == "" {
			return
		}
		f.open(next)
	}
}

// readOn hands take each line the day's files gained since the last read,
// as a dayfile.Tail reads them. Lines are only ever appended to a day, so of
// its files read afresh, from their start, as when the day is compressed,
// as many as were handed on before are passed over.
func (f *eventsFollower) readOn(take func(events.Held)) {
	f.tail.Read(func() { f.read = 0 }, func(h events.Held) {
		if f.read++; f.read <= f.handed {
			return
		}
		f.handed++
		take(h)
	})
}

// nextDay is the date of the first day after the one followed, up to today
// by the follower's clock, that the router has filed under: "" where there's
// none yet.
func (f *eventsFollower) nextDay() string {
	_, end, ok := dayfile.Day(f.date)
	if !ok {
		return ""
	}
	for _, date := range dayfile.Span(end, f.now()) {
		if _, err := f.files.Stat(date); err == nil {
			return date
		}
	}
	return ""
}
