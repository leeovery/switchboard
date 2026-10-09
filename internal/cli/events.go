package cli

import (
	"context"
	"fmt"
	"io"
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
			ctx := cmd.Context()
			if opts.follow {
				var stop context.CancelFunc
				ctx, stop = signal.NotifyContext(ctx, os.Interrupt)
				defer stop()
			}
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
	printer, tomorrow := a.eventsPrinter(out, opts.form), startOfDay(now, -1)
	if err := printer.list(events.NewReader(dir, a.Now, logger), from, tomorrow, now); err != nil || follower == nil {
		return err
	}
	return follower.run(ctx, logs.PollEvery(a.FollowEvery), printer.follow)
}

// eventsPrinter prints the router's events in a form.
type eventsPrinter interface {
	// list prints the events reader reads that happened from from up to to,
	// read at now.
	list(reader *events.Reader, from, to, now time.Time) error
	// follow prints a line as it's filed, after the listing.
	follow(h events.Held) error
}

// eventsPrinter returns the printer of the form f chooses, printing to out.
func (a *app) eventsPrinter(out io.Writer, f formFlags) eventsPrinter {
	if a.printsJSON(f, out) {
		return eventsJSON{out: out}
	}
	return &eventsText{out: out, now: a.Now, telling: views.NewTelling(claude.WindowInProse)}
}

// eventsJSON prints each event as the router files it, a JSON object a line,
// fields a later release added included.
type eventsJSON struct {
	out io.Writer
}

func (p eventsJSON) list(reader *events.Reader, from, to, _ time.Time) error {
	for h := range reader.HeldBetween(from, to) {
		if err := p.follow(h); err != nil {
			return err
		}
	}
	return nil
}

func (p eventsJSON) follow(h events.Held) error {
	data, err := h.Filed()
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
	// day is the day of the last heading printed: zero before one is. widths
	// are the columns' as the last rows printed were aligned, which a line
	// followed is aligned to, at the least.
	day    time.Time
	widths []int
}

func (p *eventsText) list(reader *events.Reader, from, to, now time.Time) error {
	var lines views.Events = reader
	at := func(line events.Line) time.Time { return line.At }
	table, err := writeDays(p.out, lines.Between(from, to), at, p.row, "events", from, now)
	p.day, p.widths = table.day, widths(nil, table.indented()...)
	return err
}

// follow prints h's row, aligned to the rows before it, under its day's
// heading where that isn't the last printed, as a change of day, or a
// changed event of an earlier day, brings.
func (p *eventsText) follow(h events.Held) error {
	now := p.now()
	at := h.At.In(now.Location())
	row := indented(p.row(h.Line, at))
	if p.day.IsZero() || !(dayTable{day: p.day}).of(at) {
		p.day = at
		if err := (dayTable{day: at}).writeHeading(p.out, now, false); err != nil {
			return err
		}
	}
	p.widths = widths(p.widths, row)
	return writeRow(p.out, row, p.widths)
}

// row is line's row, at at: its time, its session's id cut short, its kind,
// its account and what happened, as views.Telling tells them.
func (p *eventsText) row(line events.Line, at time.Time) []string {
	told := p.telling.Line(line.Event, at)
	return []string{at.Format(time.TimeOnly), status.ShortID(status.Clean(line.Session)), told.Kind, told.Account.String(), told.What.String()}
}

// eventsFollower follows the router's events' files, a day's at a time, as
// they grow, reading on from where it last ended.
type eventsFollower struct {
	files *dayfile.Files
	now   func() time.Time
	date  string
	tail  *dayfile.Tail[events.Held]
	// handed counts the lines of the day handed on, or passed over as listed
	// already, and read those read since its files were last read afresh.
	handed, read int
}

// newEventsFollower returns a follower of files, the router's events', from
// the end of what today's, by now's clock, holds already.
func newEventsFollower(files *dayfile.Files, now func() time.Time) *eventsFollower {
	f := &eventsFollower{files: files, now: now}
	f.open(dayfile.DateOf(now()))
	f.readOn(func(events.Held) {})
	return f
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
