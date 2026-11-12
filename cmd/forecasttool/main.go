// Command forecasttool scores switchboard's forecasts of where each
// account's week runs out against a history: it plays the readings history
// and the request ledger of a state directory back, as the router took them
// in, through the router's own state (router.Playback); forecasts each
// account's week at each hour, as internal/status does from the document the
// router would have built then; and scores each forecast against when the
// week reached where its account's room ends, or that it didn't, beside the
// pace the week's use since it started sets. It's a program of its own,
// never a switchboard command, as cmd/capturetool is. It reads the state
// directory and the config file named on its command line alone, with no
// default for either, so it never reads the real install by accident; it
// never reads a token, dials the router or touches the network.
//
// Usage:
//
//	forecasttool --state <dir> --config <file>          the scores as a table
//	forecasttool --state <dir> --config <file> --json   the scores as JSON
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs"
)

// logger is the tool's, which the readers of the state directory say what
// they can't read through.
var logger = logs.For("forecasttool")

func main() {
	logTo(os.Stderr)
	status := 0
	if err := run(os.Args[1:], os.Stdout, os.Stderr, time.Now, os.UserHomeDir); err != nil {
		fmt.Fprintln(os.Stderr, "forecasttool:", err)
		status = 1
	}
	logs.Close(status)
	os.Exit(status)
}

// logTo starts the tool's logging, to stderr alone: it keeps no log file, as
// it has no state directory of its own to keep one in.
func logTo(stderr io.Writer) {
	logs.Init(logs.Options{MirrorOnly: true, Mirror: stderr, Command: "forecasttool"})
}

// errUnnamed is the error of a command line that doesn't name what the tool
// reads, which it has no default for.
var errUnnamed = errors.New("is needed, and has no default")

// options are what the command line asks for.
type options struct {
	state, config string
	json          bool
}

// help is what --help says before the flags.
const help = `forecasttool plays a state directory's readings history and request ledger
back through the router's own state, forecasts each account's week at each
hour as the router would have, and scores each forecast against when the week
reached where its account's room ends: its reserve where it has one, else its
limit, a rejection counting as reached.

Usage: forecasttool --state <dir> --config <file> [--json]

Both --state and --config are needed, and neither has a default: it reads
what they name alone. What the history doesn't record, the playback has none
of: no account reaches a limit or has its token refused, as none has for a
router just started, and no global pin holds, as none of the pin's history is
kept. Hours the router was down are forecast from what it read last, as it
would have forecast them, but for a week that has reset since, which isn't.

It scores only the forecasts made from two days after the history's first
reading on, and from once every account's week has been read: two days is
what a restarted router seeds its state from, so none is scored off a cold
start, and the history keeps only readings that changed, so an account idle
since before it began can go days unread in it, though the router knew its
week, out of the play till then. Where the history never reads some
account's week, nothing is scored, as the playback can't stand in for what
the router knew of it: a config without the account plays the rest. Of a
history that ends first, it says nothing could be scored too. A forecast
scores against the run of its week it was made in, which a reset by hand
starts afresh, and only while that run is yet to end; and it pairs with the
hour before's forecast of its run, moved or not. What the readers can't read
of the state directory, they say on stderr.

`

// run scores the forecasts of the history in the state directory args name,
// of the accounts their config file configures, read in the home directory
// homeDir returns, by now's clock, and prints the scores to stdout: as a
// table, or as JSON with --json.
func run(args []string, stdout, stderr io.Writer, now func() time.Time, homeDir func() (string, error)) error {
	o, err := parse(args, stderr)
	if errors.Is(err, flag.ErrHelp) {
		return nil
	}
	if err != nil {
		return err
	}
	cfg, err := config.Load(o.config, homeDir)
	if err != nil {
		return err
	}
	policy := claude.Policy()
	h, err := historyIn(o.state, cfg.Accounts, policy, now)
	if err != nil {
		return err
	}
	r := reportOf(cfg.Accounts, playBack(cfg.Accounts, policy, h), outcomesOf(cfg.Accounts, policy.Perishable, h.readings))
	if o.json {
		return writeJSON(stdout, r)
	}
	return writeTable(stdout, r)
}

// parse reads the command line, refusing one that doesn't name both the
// state directory and the config file, or names anything more.
func parse(args []string, stderr io.Writer) (options, error) {
	var o options
	flags := flag.NewFlagSet("forecasttool", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.Usage = func() {
		_, _ = fmt.Fprint(flags.Output(), help)
		flags.PrintDefaults()
	}
	flags.StringVar(&o.state, "state", "", "the state directory whose readings history and request ledger it plays back")
	flags.StringVar(&o.config, "config", "", "the config file that configures the accounts")
	flags.BoolVar(&o.json, "json", false, "print the scores as JSON")
	if err := flags.Parse(args); err != nil {
		return options{}, err
	}
	switch {
	case o.state == "":
		return options{}, fmt.Errorf("--state %w: the state directory to play back", errUnnamed)
	case o.config == "":
		return options{}, fmt.Errorf("--config %w: the config file of its accounts", errUnnamed)
	case flags.NArg() > 0:
		return options{}, fmt.Errorf("%q is more than it takes", flags.Args())
	}
	return o, nil
}
