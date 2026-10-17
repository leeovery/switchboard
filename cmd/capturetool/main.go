// Command capturetool draws a fixture of switchboard's dashboard, for the
// visual capture harness in testdata/vhs: a named, deterministic moment, drawn
// by the dashboard's own watch model, built through watch.New with every seam
// faked (internal/capture), or a printout of usage, drawn through the path
// usage prints through. Or it plays a scenario through the same model in
// real time, for the README's demos in demo/: the clock running, the timers
// firing, and the fake router telling of what its cues have it do. It never
// dials the router, probes, touches the network, reads or writes the real
// config, state, prefs or tokens, looks in the themes directory, or runs
// another process. NO_COLOR draws it without colour, as it does the
// dashboard. Or it writes the reference frames from their fixtures, as the
// capture test holds them: every frame a directory keeps, or the ones named.
//
// Usage:
//
//	capturetool --fixture overview-default-160          full screen, at the terminal's size
//	capturetool --fixture overview-default-160 --print  its frame as text, at its own size
//	capturetool --fixture overview-default-160 --print --ansi --size 120x40
//	capturetool --fixture overview-default-160 --theme amber
//	capturetool --fixture overview-default-160 --theme ~/themes/lake.theme
//	capturetool --scenario routing                      played full screen, in real time
//	capturetool --frames testdata/vhs/reference         every frame it keeps, written afresh
//	capturetool --frames testdata/vhs/reference usage-160 runway-week-160
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"

	"github.com/leeovery/switchboard/internal/capture"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/theme"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr, os.Getenv); err != nil {
		fmt.Fprintln(os.Stderr, "capturetool:", err)
		os.Exit(1)
	}
}

// options are what the command line asks for.
type options struct {
	fixture, scenario string
	print, ansi       bool
	size              string
	theme             string
	// frames is the directory to write the frames into, and named those
	// to write, or every one it keeps where none is named.
	frames string
	named  []string
}

// run draws the fixture args name, in the theme they name, or without colour
// where getenv has NO_COLOR set: printed once to stdout with --print, else
// full screen in the terminal stdout is; or plays the scenario they name, so,
// full screen; or writes the frames they name.
func run(args []string, stdout, stderr io.Writer, getenv func(string) string) error {
	o, err := parse(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	switch {
	case o.frames != "":
		return writeFrames(o, stdout)
	case o.scenario != "":
		return play(o, stdout, getenv)
	}
	f, err := capture.ByName(o.fixture)
	if err != nil {
		return fmt.Errorf("--fixture: %w", err)
	}
	t, err := themeOf(o.theme)
	if err != nil {
		return err
	}
	f = f.InTheme(t)
	if getenv("NO_COLOR") != "" {
		f = f.WithoutColour()
	}
	return draw(f, o, stdout)
}

// draw draws the fixture as o asks: printed once to out with --print, else
// full screen in the terminal out is, as a printout of usage never is.
func draw(f capture.Fixture, o options, out io.Writer) error {
	switch {
	case o.print:
		return printFrame(f, o, out)
	case f.Printout:
		return errors.New("a printout of usage prints once: --print draws it")
	default:
		return fullScreen(f.Model(), out, "full screen, it needs a terminal: --print draws without one")
	}
}

// play plays the scenario o names, full screen in the terminal out is, in
// the theme o names, or without colour where getenv has NO_COLOR set.
func play(o options, out io.Writer, getenv func(string) string) error {
	s, err := capture.ScenarioNamed(o.scenario)
	if err != nil {
		return fmt.Errorf("--scenario: %w", err)
	}
	t, err := themeOf(o.theme)
	if err != nil {
		return err
	}
	s = s.InTheme(t)
	if getenv("NO_COLOR") != "" {
		s = s.WithoutColour()
	}
	return fullScreen(s.Model(), out, "a scenario plays full screen: it needs a terminal")
}

// themeOf is the theme --theme names, as an input, never looked for in the
// themes directory: a built-in, by its slug, or the theme in the file at a
// path, as one written and not yet put there, whatever it's named. One that
// doesn't load is an error, never a theme drawn in its place.
func themeOf(arg string) (theme.Theme, error) {
	if strings.ContainsRune(arg, filepath.Separator) || strings.HasSuffix(arg, ".theme") {
		t, err := theme.ReadFile(arg)
		if err != nil {
			return theme.Theme{}, fmt.Errorf("--theme %q doesn't load: %w", arg, err)
		}
		return t, nil
	}
	t, ok := theme.Builtin(arg)
	if !ok {
		var slugs []string
		for _, b := range theme.Builtins() {
			slugs = append(slugs, b.Slug)
		}
		return theme.Theme{}, fmt.Errorf("--theme %q names no built-in theme (built in: %s), nor a .theme file", arg, strings.Join(slugs, ", "))
	}
	return t, nil
}

// writeFrames writes the frames o names into its directory from their
// fixtures, in the frames' theme and in colour whatever NO_COLOR says, or
// every frame the directory keeps where o names none, saying each it writes
// on out. A name with no fixture is an error before any is written.
func writeFrames(o options, out io.Writer) error {
	names := o.named
	if len(names) == 0 {
		var err error
		if names, err = capture.FramesIn(o.frames); err != nil {
			return err
		}
	}
	if len(names) == 0 {
		return fmt.Errorf("--frames: %s keeps no frames: name those to write after it", o.frames)
	}
	fixtures := make([]capture.Fixture, len(names))
	for i, name := range names {
		f, err := capture.ByName(name)
		if err != nil {
			return fmt.Errorf("--frames: %w", err)
		}
		fixtures[i] = f
	}
	for _, f := range fixtures {
		if err := f.WriteFrame(o.frames); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(out, "wrote", f.Name); err != nil {
			return fmt.Errorf("say what's written: %w", err)
		}
	}
	return nil
}

// parse reads the command line, refusing a fixture and a scenario both, what
// only --print takes without it, and anything beside --frames but the names
// of the frames it writes.
func parse(args []string, stderr io.Writer) (options, error) {
	var o options
	flags := flag.NewFlagSet("capturetool", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&o.fixture, "fixture", "", "the fixture to draw: "+strings.Join(capture.Names(), ", "))
	flags.StringVar(&o.scenario, "scenario", "", "the scenario to play, full screen, in real time: "+strings.Join(capture.ScenarioNames(), ", "))
	flags.StringVar(&o.theme, "theme", theme.DefaultDark, "the theme to draw it in: a built-in's slug, or the path of a .theme file")
	flags.BoolVar(&o.print, "print", false, "print its frame once, as text, rather than run it full screen")
	flags.StringVar(&o.size, "size", "", "with --print, the terminal's size as WxH, such as 160x34: the fixture's own unless given")
	flags.BoolVar(&o.ansi, "ansi", false, "with --print, print its colours too")
	flags.StringVar(&o.frames, "frames", "", "write into the directory given the frames named after it, or every frame it keeps, from their fixtures")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	if o.frames != "" {
		return framesOnly(o, flags)
	}
	switch {
	case flags.NArg() > 0:
		return options{}, fmt.Errorf("unexpected argument %q", flags.Arg(0))
	case o.scenario != "" && o.fixture != "":
		return options{}, errors.New("--scenario and --fixture: name one")
	case o.scenario != "" && o.print:
		return options{}, errors.New("--print takes a fixture: a scenario plays in time, full screen")
	case !o.print && o.size != "":
		return options{}, errors.New("--size takes --print: full screen, the terminal gives the size")
	case !o.print && o.ansi:
		return options{}, errors.New("--ansi takes --print: full screen, the frame is drawn in colour")
	}
	return o, nil
}

// framesOnly is o with the frames named after --frames, which takes no other
// flag: the frames are written as the test holds them, in their own theme,
// at their own sizes.
func framesOnly(o options, flags *flag.FlagSet) (options, error) {
	var others []string
	flags.Visit(func(f *flag.Flag) {
		if f.Name != "frames" {
			others = append(others, "--"+f.Name)
		}
	})
	if len(others) > 0 {
		return options{}, fmt.Errorf("--frames takes the frames' names alone, not %s", strings.Join(others, ", "))
	}
	o.named = flags.Args()
	return o, nil
}

// printFrame writes the fixture's frame to out, as a terminal of the size asked
// shows it: as text alone, or with --ansi, in colour.
func printFrame(f capture.Fixture, o options, out io.Writer) error {
	size := f.Size
	if o.size != "" {
		var err error
		if size, err = parseSize(o.size); err != nil {
			return err
		}
	}
	frame := f.Frame(size)
	if !o.ansi {
		frame = capture.Plain(frame)
	}
	_, err := io.WriteString(out, frame)
	return err
}

// fullScreen runs m full screen in the terminal out is, at whatever size it
// gives, until q; where out is no terminal, it fails, saying so as refusal
// does.
func fullScreen(m tea.Model, out io.Writer, refusal string) error {
	if file, ok := out.(term.File); !ok || !term.IsTerminal(file.Fd()) {
		return errors.New(refusal)
	}
	_, err := tea.NewProgram(m, tea.WithOutput(out)).Run()
	if err != nil && !errors.Is(err, tea.ErrInterrupted) {
		return fmt.Errorf("run it full screen: %w", err)
	}
	return nil
}

// parseSize reads a terminal's size given as WxH, such as 160x34.
func parseSize(s string) (watch.Size, error) {
	w, h, cut := strings.Cut(s, "x")
	width, wErr := strconv.Atoi(w)
	height, hErr := strconv.Atoi(h)
	if !cut || wErr != nil || hErr != nil || width < 1 || height < 1 {
		return watch.Size{}, fmt.Errorf("--size %q: give it as WxH, such as 160x34", s)
	}
	return watch.Size{Width: width, Height: height}, nil
}
