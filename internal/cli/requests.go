package cli

import (
	"errors"
	"fmt"
	"io"
	"iter"
	"maps"
	"slices"
	"strconv"
	"strings"
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
	asJSON                  bool
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

With --json, print each line as the ledger holds it, a JSON object a line, for
an agent or a script to read.`,
		Args: a.ledgerArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.requests(cmd.OutOrStdout(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.session, "session", "", "keep session `ID`'s requests")
	cmd.Flags().StringVar(&opts.account, "account", "", "keep account `ID`'s requests")
	cmd.Flags().StringVar(&opts.since, "since", "", "start at `WHEN`: "+sinceForms+" (default today's start)")
	cmd.Flags().BoolVar(&opts.asJSON, "json", false, "print each line as the ledger holds it, a JSON object a line")
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
	return cobra.NoArgs(cmd, args)
}

// ledgerReader returns a reader of the request ledger in the state directory,
// by the commands' clock.
func (a *app) ledgerReader() (*ledger.Reader, error) {
	dir, err := config.StateDir(a.Getenv, a.HomeDir)
	if err != nil {
		return nil, err
	}
	return ledger.NewReader(dir, a.Now, logger), nil
}

// requests prints the ledger's lines as opts ask.
func (a *app) requests(out io.Writer, opts requestsOptions) error {
	reader, err := a.ledgerReader()
	if err != nil {
		return err
	}
	now := a.Now()
	from, err := sinceOr(opts.since, now, startOfDay(now, 0))
	if err != nil {
		return err
	}
	session := ""
	if opts.session != "" {
		if session, err = sessionAmong(reader.Lines(from), opts.session); err != nil {
			return err
		}
	}
	kept := keptLines(reader.Lines(from), session, opts.account)
	if opts.asJSON {
		return writeLines(out, kept)
	}
	return writeRequests(out, kept, from, now)
}

// keptLines returns those of lines of the session and the account with the
// given ids, each "" to keep any.
func keptLines(lines iter.Seq[ledger.Held], session, account string) iter.Seq[ledger.Held] {
	return func(yield func(ledger.Held) bool) {
		for h := range lines {
			if (session == "" || h.Session == session) && (account == "" || h.Account == account) && !yield(h) {
				return
			}
		}
	}
}

// sessionAmong returns the id of the session given names among those of
// lines: its id, or as much of it as is unique among them. Given one that
// starts none of their ids, it returns it as it is, which keeps no line. It
// fails, listing them, when given starts the ids of several.
func sessionAmong(lines iter.Seq[ledger.Held], given string) (string, error) {
	matches := make(map[string]bool)
	for h := range lines {
		switch {
		case h.Session == given:
			return given, nil
		case strings.HasPrefix(h.Session, given):
			matches[h.Session] = true
		}
	}
	ids := slices.Sorted(maps.Keys(matches))
	switch len(ids) {
	case 0:
		return given, nil
	case 1:
		return ids[0], nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "session %s could be any of these, so give more of its id:", status.Clean(redact.Text(given)))
	for _, id := range ids {
		fmt.Fprintf(&b, "\n  %s", status.Clean(id))
	}
	return "", errors.New(b.String())
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
	day, first := dayTable{}, true
	for h := range lines {
		at := h.At.In(now.Location())
		if len(day.rows) > 0 && !day.of(at) {
			if err := day.write(out, now, first); err != nil {
				return err
			}
			day.rows, first = nil, false
		}
		if len(day.rows) == 0 {
			day.day = at
		}
		day.rows = append(day.rows, requestRow(&h, at))
	}
	if len(day.rows) == 0 {
		_, err := fmt.Fprintf(out, "no requests since %s\n", from.In(now.Location()).Format(dayLayout+" 15:04"))
		return err
	}
	return day.write(out, now, first)
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
// row has room for: "812ms", "14.2s", or "2m3s".
func took(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	switch {
	case d < time.Second:
		return strconv.FormatInt(ms, 10) + "ms"
	case d < time.Minute:
		return strconv.FormatFloat(d.Seconds(), 'f', 1, 64) + "s"
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
