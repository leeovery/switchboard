// Command capturetool draws a fixture of switchboard's dashboard, for the
// visual capture harness in testdata/vhs: a named, deterministic moment, drawn
// by the dashboard's own watch model, built through watch.New with every seam
// faked (internal/capture). It never dials the router, probes, touches the
// network, reads or writes the real config, state, prefs or tokens, or runs
// another process.
//
// Usage:
//
//	capturetool --fixture accounts-3          full screen, at the terminal's size
//	capturetool --fixture accounts-3 --print  its frame as text, at its own size
//	capturetool --fixture accounts-3 --print --ansi --size 120x40
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/term"

	"github.com/leeovery/switchboard/internal/capture"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, "capturetool:", err)
		os.Exit(1)
	}
}

// options are what the command line asks for.
type options struct {
	fixture     string
	print, ansi bool
	size        string
}

// run draws the fixture args name: printed once to stdout with --print, else
// full screen in the terminal stdout is.
func run(args []string, stdout, stderr io.Writer) error {
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
	if !o.print {
		return live(f, stdout)
	}
	return printFrame(f, o, stdout)
}

// parse reads the command line, refusing what only --print takes without it.
func parse(args []string, stderr io.Writer) (options, error) {
	var o options
	flags := flag.NewFlagSet("capturetool", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&o.fixture, "fixture", "", "the fixture to draw: "+strings.Join(capture.Names(), ", "))
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
