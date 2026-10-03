// Command capturetool draws a fixture of switchboard's dashboard, for the
// visual capture harness in testdata/vhs: a named, deterministic moment, drawn
// by the dashboard's own watch model, built through watch.New with every seam
// faked (internal/capture). It never dials the router, probes, touches the
// network, reads or writes the real config, state, prefs or tokens, looks in
// the themes directory, or runs another process. NO_COLOR draws it without
// colour, as it does the dashboard.
//
// Usage:
//
//	capturetool --fixture accounts-3          full screen, at the terminal's size
//	capturetool --fixture accounts-3 --print  its frame as text, at its own size
//	capturetool --fixture accounts-3 --print --ansi --size 120x40
//	capturetool --fixture accounts-3 --theme amber
//	capturetool --fixture accounts-3 --theme ~/themes/lake.theme
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
	"github.com/charmbracelet/x/ansi"
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
	fixture     string
	print, ansi bool
	size        string
	theme       string
}

// run draws the fixture args name, in the theme they name, or without colour
// where getenv has NO_COLOR set: printed once to stdout with --print, else
// full screen in the terminal stdout is.
func run(args []string, stdout, stderr io.Writer, getenv func(string) string) error {
	o, err := parse(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
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
	if !o.print {
		return live(f, stdout)
	}
	return printFrame(f, o, stdout)
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

// parse reads the command line, refusing what only --print takes without it.
func parse(args []string, stderr io.Writer) (options, error) {
	var o options
	flags := flag.NewFlagSet("capturetool", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&o.fixture, "fixture", "", "the fixture to draw: "+strings.Join(capture.Names(), ", "))
	flags.StringVar(&o.theme, "theme", theme.DefaultDark, "the theme to draw it in: a built-in's slug, or the path of a .theme file")
	flags.BoolVar(&o.print, "print", false, "print its frame once, as text, rather than run it full screen")
	flags.StringVar(&o.size, "size", "", "with --print, the terminal's size as WxH, such as 160x34: the fixture's own unless given")
	flags.BoolVar(&o.ansi, "ansi", false, "with --print, print its colours too")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	switch {
	case flags.NArg() > 0:
		return options{}, fmt.Errorf("unexpected argument %q", flags.Arg(0))
	case !o.print && o.size != "":
		return options{}, errors.New("--size takes --print: full screen, the terminal gives the size")
	case !o.print && o.ansi:
		return options{}, errors.New("--ansi takes --print: full screen, the frame is drawn in colour")
	}
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
		frame = plain(frame)
	}
	_, err := io.WriteString(out, frame)
	return err
}

// live runs the fixture full screen in the terminal out is, at whatever size
// it gives, until q.
func live(f capture.Fixture, out io.Writer) error {
	if file, ok := out.(term.File); !ok || !term.IsTerminal(file.Fd()) {
		return errors.New("full screen, it needs a terminal: --print draws without one")
	}
	_, err := tea.NewProgram(f.Model(), tea.WithOutput(out)).Run()
	if err != nil && !errors.Is(err, tea.ErrInterrupted) {
		return fmt.Errorf("run the fixture: %w", err)
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

// plain is a frame as text alone, as it diffs against a reference: its
// escape codes stripped, and each line's trailing spaces trimmed.
func plain(frame string) string {
	lines := strings.Split(strings.TrimSuffix(frame, "\n"), "\n")
	for i, line := range lines {
		lines[i] = strings.TrimRight(ansi.Strip(line), " ")
	}
	return strings.Join(lines, "\n") + "\n"
}
