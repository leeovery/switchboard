package cli

import (
	"fmt"
	"io"
	"iter"
	"slices"
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/status"
)

// requestsOptions are the requests command's flags.
type requestsOptions struct {
	session, account, since string
	form                    formFlags
}

// idFlags are the flags that take an id, and what each is the id of, for the
// error that says so when one is given none.
var idFlags = []struct{ flag, of string }{{flag: "session", of: "a session"}, {flag: "account", of: "an account"}}

func newRequestsCommand(a *app) *cobra.Command {
	var opts requestsOptions
	cmd := &cobra.Command{
		Use:   "requests",
		Short: "List the requests the router has routed, from its request ledger",
		Long: `List the requests the router has routed, from its request ledger: today's,
unless --since reaches further back, oldest first, a line each with when it
arrived, its session's id cut short, the model it asked for, the account that
answered it, its status, its tokens in and out, and how long it took. It reads
the ledger's files, so it needs no router.

--since starts at a day, as 2026-10-01, a time today, as 14:00, or how long
ago, as 3h or 2d. --session keeps a session's own requests, named by its id or
as much of it as is unique among the sessions read, and --account an
account's.

On a terminal, requests prints its lines as text. Anywhere else, as in a pipe
or an agent's shell, it prints each as the ledger holds it, a JSON object a
line, for an agent or a script to read. --json prints the JSON, and --pretty
the text, wherever stdout is.`,
		Args: a.ledgerArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.requests(cmd.OutOrStdout(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.session, "session", "", "keep session `ID`'s requests")
	cmd.Flags().StringVar(&opts.account, "account", "", "keep account `ID`'s requests")
	cmd.Flags().StringVar(&opts.since, "since", "", "start at `WHEN`: "+sinceForms+" (default today's start)")
	opts.form.add(cmd)
	return cmd
}

// ledgerArgs checks the command line of a command that reads the ledger: it
// fails for a flag that takes an id given none, a --since it can't read, and
// any argument.
func (a *app) ledgerArgs(cmd *cobra.Command, args []string) error {
	for _, id := range idFlags {
		if f := cmd.Flags().Lookup(id.flag); f != nil && f.Changed && f.Value.String() == "" {
			return fmt.Errorf("--%s takes the id of %s", id.flag, id.of)
		}
	}
	if f := cmd.Flags().Lookup("since"); f != nil && f.Changed {
		if _, err := since(f.Value.String(), a.Now()); err != nil {
			return err
		}
	}
	return noArgs(cmd, args)
}

// ledgerReader returns a reader of the request ledger in the state directory,
// by the commands' clock, which summarises days with the caps of the accounts
// configured, and the config. It fails without a config, as every command
// that reads one does.
func (a *app) ledgerReader() (*ledger.Reader, *config.Config, error) {
	cfg, err := a.loadConfig()
	if err != nil {
		return nil, nil, err
	}
	dir, err := config.StateDir(a.Getenv, a.HomeDir)
	if err != nil {
		return nil, nil, err
	}
	return ledger.NewReader(dir, a.Now, caps(cfg), logger), cfg, nil
}

// pricing is the price table, cfg's prices in place of its own, warning of
// each model cfg prices that the table doesn't know but whose every price it
// doesn't give, which is left unpriced.
func pricing(cfg *config.Config) ledger.Table {
	table, passed := ledger.Pricing.With(cfg.Prices)
	for _, id := range passed {
		logger.Warn("the config prices a model the price table doesn't know, but not all five of its prices, so it's left unpriced",
			"model", redact.Text(id), "needs", "input, output, cache_read, cache_write_5m and cache_write_1h")
	}
	return table
}

// requests prints the ledger's lines as opts ask.
func (a *app) requests(out io.Writer, opts requestsOptions) error {
	reader, _, err := a.ledgerReader()
	if err != nil {
		return err
	}
	now := a.Now()
	from, err := sinceOr(opts.since, now, startOfDay(now, 0))
	if err != nil {
		return err
	}
	lines := reader.Lines(from)
	if opts.session != "" {
		if lines, err = sessionLines(lines, opts.session); err != nil {
			return err
		}
	}
	lines = accountLines(lines, opts.account)
	if a.printsJSON(opts.form, out) {
		return writeLines(out, lines)
	}
	return writeRequests(out, lines, from, now)
}

// sessionLines returns those of lines of the session given names among
// theirs, as sessionNamed says. It reads lines once, holding the lines of
// one session at most, as sessionHold does.
func sessionLines(lines iter.Seq[ledger.Held], given string) (iter.Seq[ledger.Held], error) {
	s := newSessionHold(given)
	for h := range lines {
		s.add(h)
	}
	if _, ok := s.named(); !ok {
		return nil, several(given, s.ids, status.Clean)
	}
	return slices.Values(s.held), nil
}

// sessionHold holds, as lines are read, the lines of the session given
// names among theirs so far, as naming says: those of one session at most,
// those of the session given names once every line is read.
type sessionHold struct {
	*naming
	holding string
	held    []ledger.Held
}

// newSessionHold returns a hold of the lines of the session given names,
// before any line is read.
func newSessionHold(given string) *sessionHold {
	return &sessionHold{naming: newNaming(given)}
}

// add takes the line h in, holding it where it's of the session named so
// far, and letting go of those held where that's no longer the session
// they're of.
func (s *sessionHold) add(h ledger.Held) {
	s.note(h.Session)
	if id, _ := s.named(); id != s.holding {
		s.holding, s.held = id, nil
	}
	if h.Session == s.holding {
		s.held = append(s.held, h)
	}
}

// accountLines returns those of lines of the account with the given id, or
// every one for "".
func accountLines(lines iter.Seq[ledger.Held], account string) iter.Seq[ledger.Held] {
	if account == "" {
		return lines
	}
	return func(yield func(ledger.Held) bool) {
		for h := range lines {
			if h.Account == account && !yield(h) {
				return
			}
		}
	}
}

// writeLines writes each of lines as the ledger holds it, a line each.
func writeLines(out io.Writer, lines iter.Seq[ledger.Held]) error {
	for h := range lines {
		if _, err := fmt.Fprintf(out, "%s\n", h.JSON); err != nil {
			return err
		}
	}
	return nil
}

// writeRequests writes lines as requests' text, a table a day, a row each,
// times in now's time zone, or says there were none since from.
func writeRequests(out io.Writer, lines iter.Seq[ledger.Held], from, now time.Time) error {
	at := func(h ledger.Held) time.Time { return h.At }
	row := func(h ledger.Held, at time.Time) []string { return requestRow(&h, at) }
	_, err := writeDays(out, lines, at, row, "requests", from, now)
	return err
}

// requestRow is the row of the request h holds, which arrived at at: its
// time, its session's id cut short, the model it asked for, the account that
// answered it, its status, its tokens in and out, and how long it took.
func requestRow(h *ledger.Held, at time.Time) []string {
	return []string{at.Format(time.TimeOnly), orNone(status.ShortID(status.Clean(h.Session))), orNone(status.Clean(h.Model)),
		orNone(status.Clean(h.Account)), answered(&h.Line), tokensInOut(h.Reply), took(h.TotalMS)}
}

// answered says how a request was answered: its status, and whether its
// client went away before its end, or the router cut it off, as "200
// canceled".
func answered(l *ledger.Line) string {
	answer := strconv.Itoa(l.Status)
	switch {
	case l.Canceled:
		answer += " canceled"
	case l.CutOff:
		answer += " cut off"
	}
	return answer
}

// tokensInOut says how many tokens a reply's usage counts in and out, as
// "185k in, 845 out": "" for one that gave none.
func tokensInOut(r ledger.Reply) string {
	t, ok := r.Tokens()
	if !ok {
		return ""
	}
	return prose.Count(tokensIn(t)) + " in, " + prose.Count(t.Output) + " out"
}

// tokensIn counts the tokens t counts in: those read from the prompt cache,
// and written to it, among them.
func tokensIn(t quota.Tokens) int {
	return t.Input + t.CacheRead + t.CacheWrite
}

// took says how long a request took, from its milliseconds, as briefly as a
// row has room for: "812ms", "14.2s", or "2m3s", each form for what rounds
// to it, so 59.95 seconds is "1m0s".
func took(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	switch tenths := d.Round(100 * time.Millisecond); {
	case d < time.Second:
		return strconv.FormatInt(ms, 10) + "ms"
	case tenths < time.Minute:
		return strconv.FormatFloat(tenths.Seconds(), 'f', 1, 64) + "s"
	}
	return d.Round(time.Second).String()
}

// orNone is text, or a dash standing for none.
func orNone(text string) string {
	if text == "" {
		return "-"
	}
	return text
}
