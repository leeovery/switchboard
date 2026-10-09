package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// eventsOptions are the events command's flags.
type eventsOptions struct {
	since  string
	follow bool
	form   formFlags
}

func newEventsCommand(a *app) *cobra.Command {
	var opts eventsOptions
	cmd := &cobra.Command{
		Use:   "events",
		Short: "List the router's events: sessions started and moved, limits, caps, pins and more",
		Long: `List the router's events, from the files it keeps them in: today's, unless
--since reaches further back, oldest first, each once, as it last stood, a line
each with when it happened, its session's id cut short, if it's of one, what
kind it is, the account it befell, and what happened. It reads the events'
files, so it needs no router.

--since starts at a day, as 2026-10-01, a time today, as 14:00, or how long
ago, as 3h or 2d. -f, --follow then prints a line each time the router files
one, a changed event again, whole, until interrupted.

On a terminal, events prints its lines as text. Anywhere else, as in a pipe or
an agent's shell, it prints each event as the router files it, a JSON object a
line, with its run and id, so a reader of a changed one takes the last. --json
prints the JSON, and --pretty the text, wherever stdout is.`,
		Args: a.ledgerArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()
			return a.events(ctx, cmd.OutOrStdout(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.since, "since", "", "start at `WHEN`: "+sinceForms+" (default today's start)")
	cmd.Flags().BoolVarP(&opts.follow, "follow", "f", false, "go on printing events as they're filed, until interrupted")
	opts.form.add(cmd)
	return cmd
}

// events prints the router's events as opts ask, following their files
// until ctx ends where it's asked to.
func (a *app) events(ctx context.Context, out io.Writer, opts eventsOptions) error {
	dir, err := config.StateDir(a.Getenv, a.HomeDir)
	if err != nil {
		return err
	}
	now := a.Now()
	from, err := sinceOr(opts.since, now, startOfDay(now, 0))
	if err != nil {
		return err
	}
	var follower *eventsFollower
	if opts.follow {
		// Before the listing, so no event filed as it's read goes unprinted.
		follower = newEventsFollower(events.Files(ledger.Dir(dir), logger), a.Now)
	}
	var reader views.Events = events.NewReader(dir, a.Now, logger)
	printer, tomorrow := a.eventsPrinter(out, opts.form), startOfDay(now, -1)
	if err := printer.list(reader.Between(from, tomorrow), from, now); err != nil || follower == nil {
		return err
	}
	return follower.run(ctx, a.followEvery(), printer.follow)
}

// followEvery is how often a follower looks for new lines: as FollowEvery
// says, else as logs.PollEvery does.
func (a *app) followEvery() time.Duration {
	if a.FollowEvery > 0 {
		return a.FollowEvery
	}
	return logs.PollEvery
}

// eventsPrinter prints the router's events in a form.
type eventsPrinter interface {
	// list prints the events that happened since from, as read at now.
	list(lines iter.Seq[events.Line], from, now time.Time) error
	// follow prints a line as it's filed, after the listing.
	follow(line events.Line) error
}

// eventsPrinter returns the printer of the form f chooses, printing to out.
func (a *app) eventsPrinter(out io.Writer, f formFlags) eventsPrinter {
	if a.printsJSON(f, out) {
		return eventsJSON{out: out}
	}
	return &eventsText{out: out, now: a.Now, telling: views.NewTelling(windowInProse)}
}

// eventsJSON prints each event as the router files it, a JSON object a line.
type eventsJSON struct {
	out io.Writer
}

func (p eventsJSON) list(lines iter.Seq[events.Line], _, _ time.Time) error {
	for line := range lines {
		if err := p.follow(line); err != nil {
			return err
		}
	}
	return nil
}

func (p eventsJSON) follow(line events.Line) error {
	data, err := json.Marshal(line)
	if err != nil {
		return fmt.Errorf("print an event: %w", err)
	}
	_, err = fmt.Fprintf(p.out, "%s\n", data)
	return err
}

// eventsText prints the events as text, under the day each happened on, a
// line each, as views.Telling tells them.
type eventsText struct {
	out     io.Writer
	now     func() time.Time
	telling *views.Telling
	// day is the day of the last heading printed: zero before one is.
	day time.Time
}

func (p *eventsText) list(lines iter.Seq[events.Line], from, now time.Time) error {
	at := func(line events.Line) time.Time { return line.At }
	day, err := writeDays(p.out, lines, at, p.row, "events", from, now)
	p.day = day
	return err
}

// follow prints line's row, under its day's heading where that isn't the
// last printed, as a change of day, or a changed event of an earlier day,
// brings.
func (p *eventsText) follow(line events.Line) error {
	now := p.now()
	at := line.At.In(now.Location())
	table := dayTable{day: at, rows: [][]string{p.row(line, at)}}
	if !p.day.IsZero() && (dayTable{day: p.day}).of(at) {
		return table.writeRows(p.out)
	}
	p.day = at
	return table.write(p.out, now, false)
}

// row is line's row, at at: its time, its session's id cut short, its kind,
// its account and what happened, as views.Telling tells them.
func (p *eventsText) row(line events.Line, at time.Time) []string {
	told := p.telling.Line(line.Event, at)
	return []string{at.Format(time.TimeOnly), status.ShortID(status.Clean(line.Session)), told.Kind, told.Account.String(), told.What.String()}
}

// windowInProse names a window by its key as the views' words do: one of
// whole hours by them, as "5-hour"; any other by its label, as "week" or
// "Fable week".
func windowInProse(key string) string {
	if length, ok := quota.Length(key); ok && length >= time.Hour && length%time.Hour == 0 && length < 24*time.Hour {
		return fmt.Sprintf("%d-hour", length/time.Hour)
	}
	return status.InProse(claude.WindowLabel(key))
}

// eventsFollower follows the router's events' files, a day's at a time, as
// they grow, reading on from where it last ended.
type eventsFollower struct {
	files *dayfile.Files
	now   func() time.Time
	date  string
	tail  *dayfile.Tail[events.Line]
}

// newEventsFollower returns a follower of files, the router's events', from
// the end of what today's, by now's clock, holds already.
func newEventsFollower(files *dayfile.Files, now func() time.Time) *eventsFollower {
	f := &eventsFollower{files: files, now: now}
	f.open(localDate(now()))
	f.tail.Read(func() {}, func(events.Line) {})
	return f
}

// open follows the files of the local day with the given date.
func (f *eventsFollower) open(date string) {
	f.date, f.tail = date, dayfile.NewTail(f.files, date, events.In)
}

// run hands print each line filed after those the follower began from,
// every interval given, until ctx ends or print fails.
func (f *eventsFollower) run(ctx context.Context, every time.Duration, print func(events.Line) error) error {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			var err error
			f.poll(func(line events.Line) {
				if err == nil {
					err = print(line)
				}
			})
			if err != nil {
				return err
			}
		}
	}
}

// poll hands take each line the day's files gained since the last poll, as
// a dayfile.Tail reads them, those of files read afresh again. Once a later
// day has come and the router has filed under it, it reads the day it
// followed to its end, then the later day's, from its start, and follows
// that.
func (f *eventsFollower) poll(take func(events.Line)) {
	next := f.nextDay()
	f.tail.Read(func() {}, take)
	if next != "" {
		f.open(next)
		f.tail.Read(func() {}, take)
	}
}

// nextDay is the date of today, by the follower's clock, where it's later
// than the day followed and the router has filed under it: "" until then.
func (f *eventsFollower) nextDay() string {
	date := localDate(f.now())
	if date <= f.date {
		return ""
	}
	if _, err := f.files.Stat(date); err != nil {
		return ""
	}
	return date
}

// localDate is the date of the local day t falls on, as the files' names give
// it.
func localDate(t time.Time) string {
	return t.Local().Format(time.DateOnly)
}
