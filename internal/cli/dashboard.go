package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/notify"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/status"
)

const (
	// defaultInterval is how often the dashboard probes every account, while
	// it probes for itself, unless told otherwise.
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
		Long:  dashboardLong,
		Args:  opts.parseArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.dashboard(cmd.Context(), cmd.OutOrStdout(), opts)
		},
	}
	shown := cmd.HelpFunc()
	cmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		c.Long = strings.Replace(dashboardLong, keysPlace, dashboardKeys(), 1)
		shown(c, args)
	})
	cmd.Flags().BoolVar(&opts.noNotify, "no-notify", false, "post no desktop notifications without the router")
	cmd.Flags().BoolVar(&opts.probe, "probe", false, "probe every account, even while the router runs")
	return cmd
}

// keysPlace is where the dashboard's long help lists its keys, as
// dashboardKeys says them: worked out as the help is asked for, as every
// command builds the dashboard's, the launch of claude among them.
const keysPlace = "{{keys}}"

const dashboardLong = `Show every account's usage as a dashboard, full screen until q: the router's,
while it answers, with its sessions and pin, else read by probing each
account, as --probe does whether the router runs or not. It needs a
terminal, and refuses without one.

It reads the router's usage every few seconds, which the router keeps
current itself, reading every account nothing has read in the last 5
minutes; R has it read them at once. Should the router stop answering, its
last document stays on screen until it answers again, or the next full read,
or R, probes the accounts instead.

The interval is how often it probes every account when it probes for itself,
without the router or with --probe: 30m unless given, and 5m at the least.
Give it as a duration, such as 15m or 1h, or as a number of minutes. It
probes sooner for a window that resets or an account that couldn't be read,
and reads the router again once it's back.

Keys: tab and shift-tab move between its tabs, and ctrl-c quits wherever it
is. The rest are each page's, as its ? help lists them:

` + keysPlace + `

The Overview's sections and b, and Graphs' sections, are kept for the next
dashboard to open with, as is the view each tab shows, its period, window or
columns, but a calendar year stepped back to. esc closes the routing card,
help or the theme picker. A click on a tab shows it; on a key of the key
line, does what the key does, but p and q; on History, on a view's name
shows it, and on < or > by its period steps it; on Runway, on < or > by its
window, moves between the views; and on the Overview and Graphs, on a
section's heading hides it.

While the router runs, it posts the desktop notifications, --probe or not;
without it, the dashboard posts its own of an account with room again and a
window passing the warning, as the config's [notifications] asks, unless
--no-notify.

The dashboard is drawn in the theme chosen in the picker, or the pair of
tokyo-night-day for a light terminal and nord for a dark one, which the
terminal is asked. Themes of your own are <slug>.theme files in
$SWITCHBOARD_THEMES_DIR, else $XDG_CONFIG_HOME/switchboard/themes, else
~/.config/switchboard/themes. NO_COLOR draws it without colour.`

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
	// Without a home directory, a session's directory shows whole.
	home, _ := a.HomeDir()
	wc := watch.Config{
		Source:        a.source(cfg, opts.probe),
		ProbeOnly:     opts.probe,
		Notifier:      notifier,
		Notifications: cfg.Notifications,
		Now:           a.Now,
		Interval:      opts.interval,
		Policy:        claude.Policy(),
		WindowName:    status.NameOfKey,
		Accounts:      cfg.Accounts,
		WeekStarts:    cfg.WeekStarts,
		WeekWindow:    claude.WeekWindow,
		Family:        claude.Provider{}.Family,
		Size:          a.environSize(),
		View:          dashboard.View(kept.Tab),
		Sections:      dashboard.SectionsNamed(kept.Sections),
		By:            keptBy(kept.By, cfg.Usage.By),
		Models:        kept.Models,
		AccountsShown: dashboard.AccountsKept(kept.Accounts),
		SessionsShown: dashboard.SessionsKept(kept.Sessions),
		RunwayShown:   dashboard.RunwayKept(kept.Runway),
		History:       dashboard.HistoryKept(kept.History, cfg.Accounts),
		Graphs:        dashboard.GraphsSectionsNamed(kept.GraphsTab.Sections),
		Log:           dashboard.LogKept(kept.Log),
		LedgerKeep:    cfg.Ledger.Keep,
		Prices:        pricing(cfg),
		Home:          home,
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

// keptBy is how the Overview groups its windows: as the preferences keep it,
// where they name window or account, else as the config's [usage] by has it.
func keptBy(kept string, configured config.Layout) dashboard.By {
	layout, _ := config.ParseLayout(kept)
	given := layoutFlag(layout)
	return given.or(configured)
}
