package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/notify"
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
	interval time.Duration
}

// read is what usage asks of the source it reads: with --refresh, the read
// the dashboard's r asks for, else the document as it stands.
func (o usageOptions) read() watch.Read {
	if o.refresh {
		return watch.Fresh()
	}
	return watch.Read{Probe: true}
}

func newUsageCommand(a *app) *cobra.Command {
	var opts usageOptions
	cmd := &cobra.Command{
		Use:   "usage [--watch [interval]] [--no-notify] [--probe] [--refresh]",
		Short: "Show every account's usage as a dashboard",
		Long: `Show every account's usage as a dashboard: the router's, while it runs, with
its sessions and pin, else read by probing each account, as --probe does
whether the router runs or not.

With --refresh, the router first reads every account it may, as the
dashboard's r has it do: each it hasn't read in the last minute, but for one
whose 5-hour window has lapsed, which a probe would start, unless a limit
holds back its every request. It waits for those reads, ten seconds at most,
then shows them, so once a limit is reset by hand, the router sees it.
Without the router, or with --probe, every account is probed anyway, so
--refresh changes nothing. It reads once, so it takes no --watch: in a watch,
r refreshes.

With --watch the dashboard stays on screen. It reads the router's usage every
few seconds, and every interval has the router probe the accounts it hasn't
read in that time: 30m unless given, and 5m at the least. Give it as a
duration, such as 15m or 1h, or as a number of minutes. Without the router it
probes every account every interval, sooner for a window that resets or an
account that couldn't be read, and reads the router again once it's back.

Keys: r refresh, q quit. While it reads the router, 1-9 pin new sessions to
the account in that place, beside those pinned already, or unpin it, a routes
every session automatically again, and m moves running sessions to the pinned
accounts. While the router runs, it posts the desktop notifications, --probe
or not; without it, the dashboard posts its own of an account with room again
and a window passing the warning, as the config's [notifications] asks, unless
--no-notify.`,
		Args: opts.parseArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if opts.watch {
				return a.watchUsage(cmd.Context(), cmd.OutOrStdout(), opts)
			}
			return a.printUsage(cmd.Context(), cmd.OutOrStdout(), opts)
		},
	}
	cmd.Flags().BoolVarP(&opts.watch, "watch", "w", false, "stay on screen, reading usage every interval")
	cmd.Flags().BoolVar(&opts.noNotify, "no-notify", false, "with --watch, post no desktop notifications")
	cmd.Flags().BoolVar(&opts.probe, "probe", false, "probe every account, even while the router runs")
	cmd.Flags().BoolVarP(&opts.refresh, "refresh", "r", false, "have the router read every account it may first, as the dashboard's r does")
	return cmd
}

// parseArgs reads the interval, which only --watch takes, and refuses
// --refresh with --watch.
func (o *usageOptions) parseArgs(_ *cobra.Command, args []string) error {
	switch {
	case o.refresh && o.watch:
		return errors.New("--refresh reads once, so it takes no --watch: in a watch, r refreshes")
	case len(args) > 0 && !o.watch:
		return fmt.Errorf("unexpected argument %q: only --watch takes an interval", args[0])
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
		return 0, fmt.Errorf("invalid interval %q: give a duration, such as 15m or 1h, or a number of minutes", s)
	case interval < minInterval:
		return 0, fmt.Errorf("interval %s is too short: the shortest is %dm", s, minInterval/time.Minute)
	}
	return interval, nil
}

// printUsage prints the dashboard once, read as opts say: probing alone with
// --probe, and having the router refresh first with --refresh.
func (a *app) printUsage(ctx context.Context, out io.Writer, opts usageOptions) error {
	doc, err := a.collect(ctx, opts.probe, opts.read())
	if err != nil {
		return err
	}
	width := a.terminalWidth(out)
	frame := dashboard.Render(doc, a.Now(), dashboard.Options{Width: width, Color: true})
	// The frame is drawn in full colour; the writer brings it down to what
	// the terminal shows, which is none when it isn't one.
	colors := colorprofile.NewWriter(out, a.Environ())
	logger.Debug("drew the dashboard", "width", width, "colors", colors.Profile.String())
	_, err = io.WriteString(colors, frame+"\n")
	return err
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
	err = a.Watch(ctx, watch.Config{
		Source:        a.source(cfg, opts.probe),
		Notifier:      notifier,
		Notifications: cfg.Notifications,
		Now:           a.Now,
		Interval:      opts.interval,
		Policy:        policy,
		Size:          a.environSize(),
	}, out, a.Environ())
	if errors.Is(err, watch.ErrNotTerminal) {
		return errors.New("--watch needs a terminal, and stdout isn't one")
	}
	return err
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
