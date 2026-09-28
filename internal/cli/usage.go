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
	"github.com/leeovery/switchboard/internal/dashboard/notify"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
)

const (
	// defaultWidth is the width the dashboard is drawn at when the terminal's
	// isn't known.
	defaultWidth = 80
	// defaultInterval is how often --watch reads usage unless told otherwise.
	defaultInterval = 30 * time.Minute
	// minInterval is the shortest interval --watch takes: every read probes
	// every account, once for each model family.
	minInterval = 5 * time.Minute
)

// usageOptions are the usage command's flags, and the interval --watch takes.
type usageOptions struct {
	watch    bool
	noNotify bool
	interval time.Duration
}

func newUsageCommand(a *app) *cobra.Command {
	var opts usageOptions
	cmd := &cobra.Command{
		Use:   "usage [--watch [interval]]",
		Short: "Show every account's usage as a dashboard",
		Long: `Show every account's usage as a dashboard.

With --watch the dashboard stays on screen and reads usage every interval: 30m
unless given, and 5m at the least. Give it as a duration, such as 15m or 1h, or
as a number of minutes. A window that resets, or an account that couldn't be
read, is read again sooner. Keys: r refresh, q quit.`,
		Args: opts.parseArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if opts.watch {
				return a.watchUsage(cmd.Context(), cmd.OutOrStdout(), opts)
			}
			return a.printUsage(cmd.Context(), cmd.OutOrStdout())
		},
	}
	cmd.Flags().BoolVarP(&opts.watch, "watch", "w", false, "stay on screen, reading usage every interval")
	cmd.Flags().BoolVar(&opts.noNotify, "no-notify", false, "with --watch, post no desktop notifications")
	return cmd
}

// parseArgs reads the interval, which only --watch takes.
func (o *usageOptions) parseArgs(_ *cobra.Command, args []string) error {
	switch {
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

// printUsage prints the dashboard once.
func (a *app) printUsage(ctx context.Context, out io.Writer) error {
	doc, err := a.collect(ctx)
	if err != nil {
		return err
	}
	frame := dashboard.Render(doc, a.Now(), dashboard.Options{Width: a.terminalWidth(out), Color: true})
	// The frame is drawn in full colour; the writer brings it down to what
	// the terminal shows, which is none when it isn't one.
	_, err = io.WriteString(colorprofile.NewWriter(out, a.Environ()), frame+"\n")
	return err
}

// watchUsage keeps the dashboard on screen, reading usage as each read falls
// due, until the user quits.
func (a *app) watchUsage(ctx context.Context, out io.Writer, opts usageOptions) error {
	source, err := a.source()
	if err != nil {
		return err
	}
	var notifier watch.Notifier = notify.NewDesktop()
	if opts.noNotify {
		notifier = notify.Off{}
	}
	cfg := watch.Config{Source: source, Notifier: notifier, Now: a.Now, Interval: opts.interval, Policy: policy}
	err = a.Watch(ctx, cfg, out, a.Environ())
	if errors.Is(err, watch.ErrNotTerminal) {
		return errors.New("--watch needs a terminal, and stdout isn't one")
	}
	return err
}

// terminalWidth is how many cells wide out is: its size when it's a terminal,
// else $COLUMNS, else defaultWidth.
func (a *app) terminalWidth(out io.Writer) int {
	if f, ok := out.(term.File); ok && term.IsTerminal(f.Fd()) {
		if width, _, err := term.GetSize(f.Fd()); err == nil && width > 0 {
			return width
		}
	}
	if columns, err := strconv.Atoi(a.Getenv("COLUMNS")); err == nil && columns > 0 {
		return columns
	}
	return defaultWidth
}
