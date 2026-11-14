package cli

import (
	"context"
	"fmt"
	"io"
	"iter"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// requestsOptions are the requests command's flags.
type requestsOptions struct {
	session, account, since, pool string
	form                          formFlags
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
account's, by its id: a label is refused.

It lists one pool's requests, each by the pool it was routed in at the time,
one from before pools the first pool's. ` + readingAPool + ` A session's
requests are those in the pool it's in now, the one its newest request was
routed in, and an account's those in the pool it's in now, or with --pool,
in that pool alone, never every pool's together.

` + readingAKeptPool + `

On a terminal, requests prints its lines as text. Anywhere else, as in a pipe
or an agent's shell, it prints each as the ledger holds it, with "schema": 1
put first, a JSON object a line, for an agent or a script to read. --json
prints the JSON, and --pretty the text, wherever stdout is.`,
		Args: a.ledgerArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.requests(cmd.Context(), cmd.OutOrStdout(), opts)
		},
	}
	cmd.Flags().StringVar(&opts.session, "session", "", "keep session `ID`'s requests")
	cmd.Flags().StringVar(&opts.account, "account", "", "keep account `ID`'s requests")
	cmd.Flags().StringVar(&opts.since, "since", "", "start at `WHEN`: "+sinceForms+" (default today's start)")
	addPoolFlag(cmd, &opts.pool, "list the requests of")
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

// ledgerState returns the config and the state directory, which the request
// ledger and the readings history are in. It fails without a config, as
// every command that reads one does, where the state directory can't be
// found, or while the history waits on its move onto the accounts' ids.
func (a *app) ledgerState() (*config.Config, string, error) {
	cfg, err := a.loadConfig()
	if err != nil {
		return nil, "", err
	}
	dir, err := config.StateDir(a.Getenv, a.HomeDir)
	if err != nil {
		return nil, "", err
	}
	if err := historyMoved(dir); err != nil {
		return nil, "", err
	}
	return cfg, dir, nil
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

// requestsRead is what requests reads as its options ask: the pool whose
// lines it reads, the config as its pools stand, and whether the pool is
// still to be told by the lines read, as linesIn tells it.
type requestsRead struct {
	pool   filedPool
	cfg    *config.Config
	untold bool
}

// requestsPool returns what requests reads as opts ask, of the files in the
// state directory dir: the pool named, else the one the verbs act on, as
// filedPool has it of cfg. A verb given a session or an account, and no
// pool, reads the pool it's in now, cfg as pooledConfig has it, failing
// where the pools can't be told: a session's, told by its lines; an
// account's, as accountIn places it, else told by its lines.
func (a *app) requestsPool(ctx context.Context, cfg *config.Config, dir string, opts requestsOptions) (requestsRead, error) {
	if opts.pool != "" || opts.session == "" && opts.account == "" {
		pool, pooled, err := a.filedPool(ctx, cfg, dir, opts.pool)
		return requestsRead{pool: pool, cfg: pooled}, err
	}
	pooled, err := a.pooledConfig(ctx, cfg)
	if err != nil {
		return requestsRead{}, err
	}
	if opts.session != "" {
		return requestsRead{cfg: pooled, untold: true}, nil
	}
	pool, placed, err := a.accountIn(pooled, dir, opts.account)
	return requestsRead{pool: pool, cfg: pooled, untold: !placed}, err
}

// requests prints the ledger's lines as opts ask.
func (a *app) requests(ctx context.Context, out io.Writer, opts requestsOptions) error {
	cfg, dir, err := a.ledgerState()
	if err != nil {
		return err
	}
	read, err := a.requestsPool(ctx, cfg, dir, opts)
	if err != nil {
		return err
	}
	now := a.Now()
	from, err := sinceOr(opts.since, now, startOfDay(now, 0))
	if err != nil {
		return err
	}
	names := sync.OnceValue(func() status.Names { return namesIn(dir, read.cfg) })
	if opts.account != "" {
		if err := notALabel(names(), opts.account); err != nil {
			return err
		}
	}
	lines, pool, err := a.requested(read, dir, opts, from)
	if err != nil {
		return err
	}
	if a.printsJSON(opts.form, out) {
		return writeLines(out, lines, poolLabelsIn(dir, read.cfg))
	}
	if _, err := io.WriteString(out, pool.heading()); err != nil {
		return err
	}
	return writeRequests(out, views.RequestLines(lines, names()), from, now)
}

// requested are the lines of the files in the state directory dir that
// requests prints, read as read has them, arrived from from on, kept as
// opts ask, and the pool they're of: of a pool still to be told, every
// pool's lines, those of the pool the lines telling it tell, as linesIn has
// them. It fails where a session is named by the start of several
// sessions' ids, and where the pool told can't be read.
func (a *app) requested(read requestsRead, dir string, opts requestsOptions, from time.Time) (iter.Seq[ledger.Held], filedPool, error) {
	pool := read.pool
	lines := a.requestLines(pool, dir, read.cfg, from)
	if opts.session != "" {
		named, held := views.NamedLines(lines, opts.session)
		if named.Several != nil {
			return nil, pool, several(opts.session, named.Several)
		}
		lines = slices.Values(held)
	}
	if read.untold {
		held, told, err := a.linesIn(read.cfg, dir, slices.Collect(telling(lines, opts)))
		if err != nil {
			return nil, pool, err
		}
		lines, pool = slices.Values(held), told
	}
	return views.OnAccount(lines, opts.account), pool, nil
}

// telling are those of lines that tell the pool a verb given a session or
// an account, and no pool, reads: all of them, a session's, where one is
// given, as a session's pool now is its own, whatever account is given
// beside it; else the account's.
func telling(lines iter.Seq[ledger.Held], opts requestsOptions) iter.Seq[ledger.Held] {
	if opts.session != "" {
		return lines
	}
	return views.OnAccount(lines, opts.account)
}

// writeLines writes each of lines as the ledger holds it, its schema first, a
// line each, the label of the pool it names after it, as label labels it.
func writeLines(out io.Writer, lines iter.Seq[ledger.Held], label func(pool string) string) error {
	for h := range lines {
		var poolLabel string
		if h.Pool != "" {
			poolLabel = label(h.Pool)
		}
		if err := writeStoredInPool(out, h.JSON, poolLabel); err != nil {
			return err
		}
	}
	return nil
}

// writeRequests writes rows as requests' text, a table a day, a row each,
// times in now's time zone, or says there were none since from.
func writeRequests(out io.Writer, rows iter.Seq[views.RequestLine], from, now time.Time) error {
	at := func(r views.RequestLine) time.Time { return r.At }
	row := func(r views.RequestLine) []string { return requestRow(r, now) }
	_, err := writeDays(out, rows, at, row, "requests", from, now, nil)
	return err
}

// requestRow is the row of the request r, at now: when it arrived, to the
// second in now's time zone, its session's id cut short, the model it asked
// for, the account that answered it, as its Said names it, its status, its
// tokens in and out, and how long it took.
func requestRow(r views.RequestLine, now time.Time) []string {
	return []string{prose.ToTheSecond(now, r.At), orNone(status.ShortID(prose.Clean(r.Session))), orNone(prose.Clean(r.Model)),
		orNone(r.Said()), answered(r.RequestRow), tokensInOut(r), took(r.Took)}
}

// answered says how a request was answered: its status, and whether its
// client went away before its end, the router cut it off, or the upstream
// broke its answer off, as "200 canceled".
func answered(r views.RequestRow) string {
	answer := strconv.Itoa(r.Status)
	switch {
	case r.Canceled:
		answer += " canceled"
	case r.CutOff:
		answer += " cut off"
	case r.Broken:
		answer += " broken off"
	}
	return answer
}

// tokensInOut says how many tokens a request's answer's usage counts in and
// out, as "185k in, 845 out": "" for one that gave none.
func tokensInOut(r views.RequestLine) string {
	if !r.Metered {
		return ""
	}
	return prose.Tokens(r.In) + " in, " + prose.Tokens(r.Out) + " out"
}

// took says how long a request took, as prose.Seconds says it: a dash where
// it isn't known.
func took(d *time.Duration) string {
	if d == nil {
		return dash
	}
	return prose.Seconds(*d)
}

// dash stands, in the data verbs' text, for what there's none of, or isn't
// known.
const dash = "-"

// orNone is text, or a dash standing for none.
func orNone(text string) string {
	if text == "" {
		return dash
	}
	return text
}
