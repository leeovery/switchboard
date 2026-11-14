package cli

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// sessionPage prints the page of the session given names, in the pool pooled
// names, as sessionsPool has it, among the router's sessions there, telling
// errOut where the router can't say whether it runs: its work in that pool
// alone, and of one with none there, that there's no such session in it.
// Where pooled names none, it's of every pool's sessions, the page of the
// session's work in the pool it's in now, as sessionIn reads it. One read
// of the ledger gives both the sessions given is named among, as
// views.NamedAmong names it, and the page, but of a session split between
// pools, read again in its pool: given is read too, as the whole id of a
// session of an earlier day.
func (a *app) sessionPage(ctx context.Context, out, errOut io.Writer, given, pooled string, asJSON bool) error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	readers, err := a.newStateReaders(ctx, cfg)
	if err != nil {
		return err
	}
	pool, err := a.sessionsPool(ctx, cfg, readers.dir, pooled)
	if err != nil {
		return err
	}
	running, unlisted := a.routedSessions(ctx, errOut, pool, "showing the session from the ledger alone, as not running")
	now := a.Now()
	read := views.ReadSessions(pool.sessions(readers), running, now, given)
	named := views.NamedAmong(given, read.Named(running))
	if named.Several != nil {
		return several(given, named.Several)
	}
	if err := a.runsElsewhere(ctx, pool.ID, running, unlisted, named.ID); err != nil {
		return err
	}
	if pooled == "" {
		if read, pool, err = a.sessionIn(ctx, cfg, readers, read, named.ID, now); err != nil {
			return err
		}
	}
	page, ok := views.SessionPageOf(named.ID, views.PageSources{
		Routed:   views.Routed(running, named.ID),
		Unlisted: unlisted,
		Read:     read,
		Prices:   pricing(cfg),
		Keep:     cfg.Ledger.Keep,
		Now:      now,
	})
	if !ok {
		return fmt.Errorf("no session %s in the ledger, or among the router's sessions%s", prose.Clean(redact.Text(given)), inNamed(pool.named))
	}
	page = page.Named(status.NamesOf(cfg.Accounts, readers.removed))
	if asJSON {
		return writeJSON(out, page)
	}
	return writeSessionPage(out, page, now)
}

// sessionsPool returns the pool a session given is looked for in, named or
// not: none, for whatever pool it's in, where none is named, and in a config
// with no pools; else the one named, as filedPool has it of cfg, of the files
// in the state directory dir, failing as it does.
func (a *app) sessionsPool(ctx context.Context, cfg *config.Config, dir, named string) (filedPool, error) {
	if named == "" {
		return filedPool{}, nil
	}
	pool, _, err := a.filedPool(ctx, cfg, dir, named)
	return pool, err
}

// runsElsewhere fails where the session with the given id, looked for in the
// pool with the id pool, isn't among those the router lists running there,
// running, but is among those it lists in every pool, so its page isn't
// shown as ended or idle there: as findSession refuses a session the router
// hasn't seen in the pool named. It's nil where no pool is named, and where
// the router can't say, as unlisted says, or lists it nowhere, the page
// then the ledger's.
func (a *app) runsElsewhere(ctx context.Context, pool string, running []status.Session, unlisted bool, id string) error {
	if pool == "" || unlisted || views.Routed(running, id) != nil {
		return nil
	}
	every, err := a.routerSessions(ctx, func(c *router.Client) ([]status.Session, error) { return c.SessionsInEveryPool(ctx) })
	if err != nil {
		logger.Debug("can't ask the router its sessions in every pool", "error", err)
		return nil
	}
	if views.Routed(every, id) == nil {
		return nil
	}
	return fmt.Errorf("%w %s in pool %s: it runs in another; name that one with --pool, or none", router.ErrUnknownSession, redact.Text(id), prose.Clean(pool))
}

// writeSessionPage writes p as a session's page's text, at now: its id cut
// short and its directory; what it's doing, where it runs and why, and when
// it started; its models, while it runs, else how long its lines are kept;
// the accounts it ran on, a row each; and its totals, then its turns, a row
// each, newest first; each account named as the page names it.
func writeSessionPage(out io.Writer, p views.SessionPage, now time.Time) error {
	heading := "session " + status.ShortID(prose.Clean(p.Session))
	if p.Dir != "" {
		heading += "  " + prose.Clean(p.Dir)
	}
	doingNow := []string{p.Activity.Said(now)}
	if where := p.Where(now).Words().String(); where != "" {
		doingNow = append(doingNow, where)
	}
	if !p.Started.IsZero() {
		doingNow = append(doingNow, startedSaid(p, now))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n%s\n%s\n", heading, strings.Join(doingNow, prose.Separator), goesOrKept(p, now))
	if len(p.Accounts) > 0 {
		b.WriteString("\n")
		if err := writeTable(&b, ranOnRows(p, now)); err != nil {
			return err
		}
	}
	t := p.Totals
	fmt.Fprintf(&b, "\n%s%s%s%s%s from cache%s%s at API prices\n", prose.Counted(t.Turns, "turn"), prose.Separator, prose.Counted(t.Requests, "request"), prose.Separator,
		prose.Percent(t.FromCache), prose.Separator, t.Cents())
	if len(p.Turns) > 0 {
		if err := writeTable(&b, turnRows(p, now)); err != nil {
			return err
		}
	}
	_, err := io.WriteString(out, b.String())
	return err
}

// startedSaid says when the session p started, at now, and how it has run
// since, as row 6 says them: "started 12:02 · 2h 40m", "started yesterday
// 16:40 · resumed 09:12", or "started yesterday 13:20 · ran 5h 27m".
func startedSaid(p views.SessionPage, now time.Time) string {
	return "started " + p.StartedSaid(now) + " · " + p.RanSaid(now)
}

// goesOrKept says, of the session p while it runs, as its Runs says, where
// each of its models goes, as its Goes has them, as the page's row 7 names
// them: "opus-5-5 → side   haiku-4-5 → side"; of one ended, until when the
// ledger keeps the lines of its first day, as views.KeptUntil says it.
func goesOrKept(p views.SessionPage, now time.Time) string {
	if !p.Runs() {
		return views.KeptUntil(p.KeptUntil, now)
	}
	goes := make([]string, len(p.Goes))
	for i, g := range p.Goes {
		goes[i] = ledger.ShortModelID(g.Model) + " → " + g.Said()
	}
	return strings.Join(goes, "   ")
}

// ranOnRows are the rows of the accounts the session p ran on, at now: each
// with its time there, as its Stays say it; its requests; the points of each
// window they took, as PointsSaid says them; and their worth.
func ranOnRows(p views.SessionPage, now time.Time) [][]string {
	var rows [][]string
	for _, s := range p.Stays(now) {
		rows = append(rows, []string{s.AccountName.Said(), s.Said, prose.Counted(s.Requests, "request"), s.PointsSaid(), s.Cents()})
	}
	return rows
}

// turnHeads are the heads of the columns of a session's turns.
var turnHeads = []string{"#", "started", "took", "requests", "tools", "read", "written", "out", "worth", "on"}

// turnRows are the rows of the session p's turns, newest first, at now,
// under their heads.
func turnRows(p views.SessionPage, now time.Time) [][]string {
	rows := [][]string{turnHeads}
	for _, t := range p.Turns {
		rows = append(rows, []string{strconv.Itoa(t.Turn), prose.Past(now, t.Started), turnTook(t, now), prose.Grouped(t.Requests), t.ToolsSaid(),
			prose.Tokens(t.Read), prose.Tokens(t.Written), prose.Tokens(t.Out), t.Cents(), accountsSaid(t.Names)})
	}
	return rows
}

// turnTook says how long a turn took, at now, as its Took says it, as "3m";
// and of one still going, "…" after it.
func turnTook(t views.Turn, now time.Time) string {
	minutes, going := t.Took(now)
	if going {
		return minutes + prose.Ellipsis
	}
	return minutes
}

// accountsSaid says the accounts a turn went to, in the order it did, as the
// page draws a turn that moved, each as its Said names it: "work → side".
func accountsSaid(names []status.AccountName) string {
	said := make([]string, len(names))
	for i, n := range names {
		said[i] = n.Said()
	}
	return strings.Join(said, " → ")
}
