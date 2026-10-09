package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/notify"
	"github.com/leeovery/switchboard/internal/redact"
)

const (
	// defaultWidth and defaultHeight are the size the dashboard is drawn at
	// when neither the terminal nor the environment gives it.
	defaultWidth  = 80
	defaultHeight = 24
	// defaultInterval is how often --watch reads usage in full unless told
	// otherwise.
	defaultInterval = 30 * time.Minute
	// minInterval is the shortest interval --watch takes: a full read can
	// probe every account, once for each model family.
	minInterval = 5 * time.Minute
)

// usageOptions are the usage command's flags, and the interval --watch takes.
type usageOptions struct {
	watch    bool
	noNotify bool
	probe    bool
	refresh  bool
	form     formFlags
	interval time.Duration
}

func newUsageCommand(a *app) *cobra.Command {
	var opts usageOptions
	cmd := &cobra.Command{
		Use:   "usage [--watch [interval]] [--no-notify] [--probe] [--refresh] [--json | --pretty]",
		Short: "Show every account's usage as a dashboard",
		Long: `Show every account's usage as a dashboard: the router's, while it runs, with
its sessions and pin, else read by probing each account, as --probe does
whether the router runs or not.

On a terminal, usage prints the dashboard once. Anywhere else, as in a pipe
or an agent's shell, it prints the status document as JSON in its place, as
status --json prints it, read as its flags say. --json prints the JSON, and
--pretty the dashboard, wherever stdout is: off a terminal, without colour,
as wide as $COLUMNS says, else 80 columns. --watch needs a terminal, and
takes no --json.

With --refresh, the router first reads every account it may, as the
dashboard's r has it do: each it hasn't read in the last minute, and each that
can take no request anyway, however lately it read it, as while a limit holds
back its every request, or a window every model shares reads spent. It passes
over one whose 5-hour window has lapsed, which a probe would start, unless it
can take no request anyway, and one it probed in the last minute. It waits for
those reads, ten seconds at most, then shows them, so once a limit is reset by
hand, the router sees it. Without the router, or with --probe, every account
is probed anyway, so --refresh changes nothing. It reads once, so it takes no
--watch: in a watch, r refreshes.

With --watch the dashboard stays on screen. It reads the router's usage every
few seconds, and every interval has the router probe the accounts it hasn't
read in that time: 30m unless given, and 5m at the least. Give it as a
duration, such as 15m or 1h, or as a number of minutes. Should the router
stop answering, its last document stays on screen until it answers again, or
the next full read, or r, probes the accounts instead. Without the router it
probes every account every interval, sooner for a window that resets or an
account that couldn't be read, and reads the router again once it's back.

Keys: r refresh, t the theme picker, w the window every card features (auto,
for each its own, then the 5-hour window, the week, and any other in use),
g the chart every card draws of it (burn-down, burn rate, hourglass),
j, k, PgDn, PgUp and the wheel scroll the cards where they don't all fit,
? every key and what the glyphs mean, q quit. While it reads the router, and
the router answers, of more than one account, 1-9 pin new sessions to the
account in that place, beside those pinned already, or unpin it, a routes
every session automatically again, and m moves running sessions to the
pinned accounts. While the router runs, it
posts the desktop notifications, --probe or not; without it, the dashboard
posts its own of an account with room again and a window passing the warning,
as the config's [notifications] asks, unless --no-notify.

The dashboard is drawn in the theme chosen in the picker, or the pair of
tokyo-night-day for a light terminal and nord for a dark one, which the
terminal is asked. Themes of your own are <slug>.theme files in
$SWITCHBOARD_THEMES_DIR, else $XDG_CONFIG_HOME/switchboard/themes, else
~/.config/switchboard/themes. NO_COLOR draws it without colour. Off a
terminal, as --pretty prints it, it has none either, unless CLICOLOR_FORCE
asks for it.`,
		Args: opts.parseArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			switch out := cmd.OutOrStdout(); {
			case opts.watch:
				return a.watchUsage(cmd.Context(), out, opts)
			case a.printsJSON(opts.form, out):
				return a.printDocument(cmd.Context(), out, opts)
			default:
				return a.printUsage(cmd.Context(), out, opts)
			}
		},
	}
	cmd.Flags().BoolVarP(&opts.watch, "watch", "w", false, "stay on screen, reading usage every interval")
	cmd.Flags().BoolVar(&opts.noNotify, "no-notify", false, "with --watch, post no desktop notifications")
	cmd.Flags().BoolVar(&opts.probe, "probe", false, "probe every account, even while the router runs")
	cmd.Flags().BoolVarP(&opts.refresh, "refresh", "r", false, "have the router read every account it may first, as the dashboard's r does")
	opts.form.add(cmd)
	return cmd
}

// parseArgs reads the interval, which only --watch takes, and refuses
// --refresh and --json with --watch.
func (o *usageOptions) parseArgs(_ *cobra.Command, args []string) error {
	switch {
	case o.refresh && o.watch:
		return errors.New("--refresh reads once, so it takes no --watch: in a watch, r refreshes")
	case o.form.json && o.watch:
		return errors.New("--json prints the status document once, so it takes no --watch")
	case len(args) > 0 && !o.watch:
		return fmt.Errorf("unexpected argument %q: only --watch takes an interval", redact.Text(args[0]))
	case len(args) > 1:
		return fmt.Errorf("--watch takes one interval, not %d", len(args))
	case len(args) == 1:
		var err error
		o.interval, err = parseInterval(args[0])
		return err
	default:
		o.interval = defaultInterval
		return nil
	}
}

// parseInterval reads a watch interval: a duration such as 15m or 1h, or a
// bare number of minutes.
func parseInterval(s string) (time.Duration, error) {
	interval, err := time.ParseDuration(s)
	if err != nil {
		interval, err = time.ParseDuration(s + "m")
	}
	switch {
	case err != nil:
		return 0, fmt.Errorf("invalid interval %q: give a duration, such as 15m or 1h, or a number of minutes", redact.Text(s))
	case interval < minInterval:
		return 0, fmt.Errorf("interval %s is too short: the shortest is %dm", s, minInterval/time.Minute)
	}
	return interval, nil
}

// printUsage prints the dashboard once, read as opts say: probing alone with
// --probe, and having the router refresh first with --refresh; from the
// router, with its sessions and its history. It's the Accounts view, as wide
// as the terminal, every card at its fullest, in the theme the preferences
// keep, featuring the window they keep, its chart in the style they keep.
func (a *app) printUsage(ctx context.Context, out io.Writer, opts usageOptions) error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	snap, err := watch.Once(ctx, a.source(cfg, opts.probe), readOnce(opts.refresh))
	if err != nil {
		return err
	}
	// The frame is drawn in full colour; the writer brings it down to what
	// the terminal shows, which is none when it isn't one.
	colors := colorprofile.NewWriter(out, a.Environ())
	t := a.themes()
	kept, width := t.kept(), a.terminalWidth(out)
	rows := dashboard.Frame{
		Width: width, Look: a.printLook(out, colors.Profile, t, kept.Choice), View: dashboard.Accounts,
		Outdated: snap.Outdated, History: snap.History, Featured: dashboard.Feature(kept.Featured), Chart: dashboard.Chart(kept.Chart),
		Sessions: snap.Sessions, Policy: claude.Policy,
	}.Draw(snap.Doc, a.Now())
	logger.Debug("drew the dashboard", "width", width, "colors", colors.Profile.String())
	_, err = io.WriteString(colors, strings.Join(rows, "\n")+"\n")
	return err
}

// printDocument prints the status document once, read as opts say, as
// status --json prints it: what usage prints with --json, or where stdout
// isn't a terminal, as for an agent or a pipe, which want data rather than a
// dashboard's boxes and bars.
func (a *app) printDocument(ctx context.Context, out io.Writer, opts usageOptions) error {
	doc, err := a.collect(ctx, opts.probe, readOnce(opts.refresh))
	if err != nil {
		return err
	}
	return writeJSON(out, doc)
}

// watchUsage keeps the dashboard on screen, reading usage as each read falls
// due, until the user quits.
func (a *app) watchUsage(ctx context.Context, out io.Writer, opts usageOptions) error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	notifier := a.Notifier
	if opts.noNotify {
		notifier = notify.Off{}
	}
	t := a.themes()
	kept := t.kept()
	wc := watch.Config{
		Source:        a.source(cfg, opts.probe),
		Notifier:      notifier,
		Notifications: cfg.Notifications,
		Now:           a.Now,
		Interval:      opts.interval,
		Policy:        claude.Policy,
		Size:          a.environSize(),
		View:          dashboard.View(kept.View),
		Featured:      dashboard.Feature(kept.Featured),
		Chart:         dashboard.Chart(kept.Chart),
	}
	a.readers(&wc, cfg)
	if t.prefs != nil {
		wc.Prefs = t.prefs
	}
	if !a.noColour() {
		wc.Choice = kept.Choice
		wc.Pair = t.library.Pair(wc.Choice)
		wc.Themes = t
	}
	err = a.Watch(ctx, wc, out, a.Environ())
	if errors.Is(err, watch.ErrNotTerminal) {
		return errors.New("--watch needs a terminal, and stdout isn't one")
	}
	return err
}

// IsTerminal reports whether out is a terminal: Deps.Terminal, for the
// process's own output.
func IsTerminal(out io.Writer) bool {
	f, ok := out.(term.File)
	return ok && term.IsTerminal(f.Fd())
}

// terminalWidth is how many cells wide out is: its size when it's a terminal,
// else the environment's.
func (a *app) terminalWidth(out io.Writer) int {
	if f, ok := out.(term.File); ok && term.IsTerminal(f.Fd()) {
		if width, _, err := term.GetSize(f.Fd()); err == nil && width > 0 {
			return width
		}
	}
	return a.environSize().Width
}

// environSize is the terminal's size as $COLUMNS and $LINES give it, each
// where it's a positive whole number, else 80 by 24.
func (a *app) environSize() watch.Size {
	return watch.Size{Width: a.positiveEnv("COLUMNS", defaultWidth), Height: a.positiveEnv("LINES", defaultHeight)}
}

// positiveEnv is the environment variable key as a positive whole number, or
// fallback when it isn't one.
func (a *app) positiveEnv(key string, fallback int) int {
	if n, err := strconv.Atoi(a.Getenv(key)); err == nil && n > 0 {
		return n
	}
	return fallback
}
