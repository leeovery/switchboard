package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

const (
	// defaultWidth and defaultHeight are the size usage and the dashboard
	// are drawn at when neither the terminal nor the environment gives it.
	defaultWidth  = 80
	defaultHeight = 24
)

// usageOptions are the usage command's flags. Their zero value prints every
// window, laid out as the config says.
type usageOptions struct {
	probe   bool
	refresh bool
	by      layoutFlag
	windows windowFlags
	form    formFlags
}

func newUsageCommand(a *app) *cobra.Command {
	var opts usageOptions
	cmd := &cobra.Command{
		Use:   "usage [--5h] [--week] [--<model>] [--by window|account] [--probe] [-r] [--json | --pretty]",
		Short: "Print every account's windows as they stand",
		Long: `Print every account's windows as they stand, once, into the scrollback, with
the time they were read, to hold against one printed earlier: the router's,
while it answers, else read by probing each account, as --probe does whether
the router runs or not.

--by lays the windows out by window, the accounts compared a window at a
time, or by account, each account's windows together, over the config's
[usage] by; by window where neither says. --5h, --week, and a flag for each
model's own window, named for its model, as --fable is for Fable's week,
print the windows given alone, in Claude's order. One that no account
reports fails, naming those there are.

On a terminal, usage prints the windows. Anywhere else, as in a pipe or an
agent's shell, it prints the status document as JSON in their place, as
status --json prints it, read as its flags say: --by and the window flags
shape the windows printed alone. --json prints the JSON, and --pretty the
windows, wherever stdout is: off a terminal, without colour, as wide as
$COLUMNS says, else 80 columns.

With --refresh, the router first reads every account it may, as the
dashboard's R has it do: each it hasn't read in the last minute, and each that
can take no request anyway, however lately it read it, as while a limit holds
back its every request, or a window every model shares reads spent. It passes
over one whose 5-hour window has lapsed, which a probe would start, unless it
can take no request anyway, and one it probed in the last minute. It waits for
those reads, ten seconds at most, then prints them, so once a limit is reset
by hand, the router sees it. Without the router, or with --probe, every
account is probed anyway, so --refresh changes nothing.

The windows are printed in the theme chosen in the dashboard's picker, or the
pair of tokyo-night-day for a light terminal and nord for a dark one, which
the terminal is asked, on the terminal's own background. NO_COLOR, or a
terminal that shows no colour, prints them without colour. Off a terminal, as
--pretty prints them, they have none either, unless CLICOLOR_FORCE asks for
it.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			if a.printsJSON(opts.form, out) {
				return a.printDocument(cmd.Context(), out, opts)
			}
			return a.printUsage(cmd.Context(), out, opts)
		},
	}
	opts.windows = addWindowFlags(cmd)
	cmd.Flags().Var(&opts.by, "by", "lay the windows out by window or by account, over the config's [usage] by")
	cmd.Flags().BoolVar(&opts.probe, "probe", false, "probe every account, even while the router runs")
	cmd.Flags().BoolVarP(&opts.refresh, "refresh", "r", false, "have the router read every account it may first, as the dashboard's R does")
	opts.form.add(cmd)
	return cmd
}

// printUsage prints every account's windows once, a blank line first, read
// as opts say: probing alone with --probe, and having the router refresh
// first with --refresh. It reads the status document alone, as status does,
// and prints the windows opts give, laid out as they say, else as the config
// does, as wide as the terminal, in the theme the preferences keep.
func (a *app) printUsage(ctx context.Context, out io.Writer, opts usageOptions) error {
	cfg, doc, err := a.collect(ctx, opts.probe, readOnce(opts.refresh))
	if err != nil {
		return err
	}
	windows, err := opts.windows.keys(doc)
	if err != nil {
		return err
	}
	// The printout is drawn in full colour; the writer brings it down to what
	// the terminal shows, which is none when it isn't one.
	colors := colorprofile.NewWriter(out, a.Environ())
	t := a.themes()
	width := a.terminalWidth(out)
	rows := dashboard.Printout{
		Width:   width,
		Look:    a.printLook(out, colors.Profile, t, t.kept().Choice),
		By:      opts.by.or(cfg.Usage.By),
		Windows: windows,
		Policy:  claude.Policy,
		Name:    claude.WindowName,
	}.Draw(doc, a.Now())
	logger.Debug("drew the printout", "width", width, "colors", colors.Profile.String())
	_, err = io.WriteString(colors, "\n"+strings.Join(rows, "\n")+"\n")
	return err
}

// printDocument prints the status document once, read as opts say, as
// status --json prints it: what usage prints with --json, or where stdout
// isn't a terminal, as for an agent or a pipe, which want data rather than a
// printout's bars.
func (a *app) printDocument(ctx context.Context, out io.Writer, opts usageOptions) error {
	_, doc, err := a.collect(ctx, opts.probe, readOnce(opts.refresh))
	if err != nil {
		return err
	}
	return writeJSON(out, doc)
}

// layoutFlag is --by: the layout given, or "" where none is.
type layoutFlag config.Layout

func (l *layoutFlag) String() string {
	return string(*l)
}

// Set takes a layout by its name. Cobra quotes what it was given in refusing
// it, which hideTokens hides where it's shaped like a token.
func (l *layoutFlag) Set(given string) error {
	layout, ok := config.ParseLayout(given)
	if !ok {
		return errors.New("must be window or account")
	}
	*l = layoutFlag(layout)
	return nil
}

func (l *layoutFlag) Type() string {
	return "window|account"
}

// or is the layout given, else the config's, as the printout draws it.
func (l *layoutFlag) or(configured config.Layout) dashboard.By {
	if *l != "" {
		configured = config.Layout(*l)
	}
	if configured == config.ByAccount {
		return dashboard.ByAccount
	}
	return dashboard.ByWindow
}

// windowFlag is one of usage's flags that print a window alone: --5h, --week,
// or one for a model's own window, named for its model, as --fable.
type windowFlag struct {
	name, key string
	// what names the window in a sentence, as "the Fable week".
	what  string
	given bool
}

// windowFlags are usage's window flags, in Claude's order.
type windowFlags []*windowFlag

// addWindowFlags registers usage's window flags on cmd, and returns them:
// --5h, --week, and one for each model's own window internal/claude knows,
// so each is known as the command line is parsed.
func addWindowFlags(cmd *cobra.Command) windowFlags {
	flags := windowFlags{
		{name: "5h", key: claude.FiveHourWindow, what: "the " + claude.WindowName(claude.FiveHourWindow) + " window"},
		{name: "week", key: claude.WeekWindow, what: "the week"},
	}
	for _, w := range claude.ModelWindows() {
		flags = append(flags, &windowFlag{name: strings.ToLower(w.Name), key: w.Key, what: "the " + claude.WindowName(w.Key)})
	}
	slices.SortStableFunc(flags, func(a, b *windowFlag) int { return quota.CompareKeys(a.key, b.key) })
	for _, f := range flags {
		cmd.Flags().BoolVar(&f.given, f.name, false, "print "+f.what+" alone, or with the others given")
	}
	return flags
}

// keys returns the keys of the windows given, in Claude's order: none where
// none is, which prints every window. It fails for a window no account in doc
// reports, naming those there are by their flags.
func (ws windowFlags) keys(doc status.Document) ([]string, error) {
	reported := make(map[string]bool)
	for _, account := range doc.Accounts {
		for _, w := range account.Windows {
			reported[w.Key] = true
		}
	}
	var keys []string
	for _, f := range ws {
		switch {
		case !f.given:
		case !reported[f.key]:
			return nil, ws.unreported(f, reported)
		default:
			keys = append(keys, f.key)
		}
	}
	return keys, nil
}

// unreported is the error of the window flag f, for a window no account
// reports, naming the flags of those reported, or where none is reported at
// all, saying so.
func (ws windowFlags) unreported(f *windowFlag, reported map[string]bool) error {
	var there []string
	for _, w := range ws {
		if reported[w.key] {
			there = append(there, "--"+w.name)
		}
	}
	switch {
	case len(there) > 0:
		return fmt.Errorf("no account reports %s: give %s", f.what, strings.Join(there, " or "))
	case len(reported) == 0:
		return fmt.Errorf("no account reports %s, nor any other window", f.what)
	default:
		return fmt.Errorf("no account reports %s", f.what)
	}
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
