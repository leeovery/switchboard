package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/notify"
	"github.com/leeovery/switchboard/internal/redact"
)

const (
	// defaultInterval is how often the dashboard reads usage in full unless
	// told otherwise.
	defaultInterval = 30 * time.Minute
	// minInterval is the shortest interval the dashboard takes: a full read
	// can probe every account, once for each model family.
	minInterval = 5 * time.Minute
)

// dashboardOptions are the dashboard command's flags, and its interval.
type dashboardOptions struct {
	noNotify bool
	probe    bool
	interval time.Duration
}

func newDashboardCommand(a *app) *cobra.Command {
	var opts dashboardOptions
	cmd := &cobra.Command{
		Use:   "dashboard [interval] [--no-notify] [--probe]",
		Short: "Show every account's usage as a dashboard, full screen",
		Long: `Show every account's usage as a dashboard, full screen until q: the router's,
while it answers, with its sessions and pin, else read by probing each
account, as --probe does whether the router runs or not. It needs a
terminal, and refuses without one.

It reads the router's usage every few seconds, and every interval has the
router probe the accounts it hasn't read in that time: 30m unless given, and
5m at the least. Give it as a duration, such as 15m or 1h, or as a number of
minutes. Should the router stop answering, its last document stays on screen
until it answers again, or the next full read, or r, probes the accounts
instead. Without the router it probes every account every interval, sooner
for a window that resets or an account that couldn't be read, and reads the
router again once it's back.

Keys: r refresh, t the theme picker, w the window every card features (auto,
for each its own, then the 5-hour window, the week, and any other in use),
g the chart every card draws of it (burn-down, burn rate, hourglass),
j, k, PgDn, PgUp and the wheel scroll the cards where they don't all fit,
? every key and what the glyphs mean, q quit. While it reads the router, and
the router answers, of more than one account, 1-9 pin new sessions to the
account in that place, beside those pinned already, or unpin it, a routes
every session automatically again, and m moves running sessions to the
pinned accounts. While the router runs, it posts the desktop notifications,
--probe or not; without it, the dashboard posts its own of an account with
room again and a window passing the warning, as the config's [notifications]
asks, unless --no-notify.

The dashboard is drawn in the theme chosen in the picker, or the pair of
tokyo-night-day for a light terminal and nord for a dark one, which the
terminal is asked. Themes of your own are <slug>.theme files in
$SWITCHBOARD_THEMES_DIR, else $XDG_CONFIG_HOME/switchboard/themes, else
~/.config/switchboard/themes. NO_COLOR draws it without colour.`,
		Args: opts.parseArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.dashboard(cmd.Context(), cmd.OutOrStdout(), opts)
		},
	}
	cmd.Flags().BoolVar(&opts.noNotify, "no-notify", false, "post no desktop notifications without the router")
	cmd.Flags().BoolVar(&opts.probe, "probe", false, "probe every account, even while the router runs")
	return cmd
}

// parseArgs reads the interval, if one is given.
func (o *dashboardOptions) parseArgs(_ *cobra.Command, args []string) error {
	switch len(args) {
	case 0:
		o.interval = defaultInterval
		return nil
	case 1:
		var err error
		o.interval, err = parseInterval(args[0])
		return err
	default:
		return fmt.Errorf("dashboard takes one interval, not %d", len(args))
	}
}

// parseInterval reads the dashboard's interval: a duration such as 15m or
// 1h, or a bare number of minutes.
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

// dashboard keeps the dashboard on screen, reading usage as each read falls
// due, until the user quits.
func (a *app) dashboard(ctx context.Context, out io.Writer, opts dashboardOptions) error {
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
		return errors.New("dashboard needs a terminal, and stdout isn't one")
	}
	return err
}
