package cli

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// sessionPage prints the page of the session given names, telling errOut
// where the router can't say whether it runs. One read of the ledger gives
// both the sessions given is named among, as views.NamedAmong names it, and
// the page: given is read too, as the whole id of a session of an earlier
// day.
func (a *app) sessionPage(ctx context.Context, out, errOut io.Writer, given string, asJSON bool) error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	readers, err := a.newStateReaders(cfg)
	if err != nil {
		return err
	}
	running, unlisted := a.routedSessions(ctx, errOut, "showing the session from the ledger alone, as not running")
	now := a.Now()
	read := views.ReadSessions(readers.ledger, running, now, given)
	named := views.NamedAmong(given, read.Named(running))
	if named.Several != nil {
		return several(given, named.Several)
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
		return fmt.Errorf("no session %s in the ledger, or among the router's sessions", prose.Clean(redact.Text(given)))
	}
	if asJSON {
		return writeJSON(out, page)
	}
	return writeSessionPage(out, page, now)
}

// writeSessionPage writes p as a session's page's text, at now: its id cut
// short and its directory; what it's doing, where it runs and why, and when
// it started; its models, while it runs, else how long its lines are kept;
// the accounts it ran on, a row each; and its totals, then its turns, a row
// each, newest first.
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
		goes[i] = ledger.ShortModelID(prose.Clean(g.Model)) + " → " + prose.Clean(g.Account)
	}
	return strings.Join(goes, "   ")
}

// ranOnRows are the rows of the accounts the session p ran on, at now: each
// with its time there, as its Stays say it; its requests; the points of each
// window they took, as PointsSaid says them; and their worth.
func ranOnRows(p views.SessionPage, now time.Time) [][]string {
	var rows [][]string
	for _, s := range p.Stays(now) {
		rows = append(rows, []string{prose.Clean(s.Account), s.Said, prose.Counted(s.Requests, "request"), s.PointsSaid(), s.Cents()})
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
			prose.Tokens(t.Read), prose.Tokens(t.Written), prose.Tokens(t.Out), t.Cents(), accountsSaid(t.Accounts)})
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
// page draws a turn that moved: "work → side".
func accountsSaid(accounts []string) string {
	said := make([]string, len(accounts))
	for i, a := range accounts {
		said[i] = prose.Clean(a)
	}
	return strings.Join(said, " → ")
}
