package cli

import (
	"context"
	"fmt"
	"io"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// sessionPage prints the page of the session given names, as sessionAmong
// says, telling errOut where the router can't say whether it runs.
func (a *app) sessionPage(ctx context.Context, out, errOut io.Writer, given string, asJSON bool) error {
	cfg, err := a.loadConfig()
	if err != nil {
		return err
	}
	readers, err := a.newStateReaders(cfg)
	if err != nil {
		return err
	}
	running := a.routedSessions(ctx, errOut, "showing the session from the ledger alone, as not running")
	id, err := sessionAmong(given, readers.ledger, running)
	if err != nil {
		return err
	}
	now := a.Now()
	page, ok := views.SessionPageOf(id, views.PageSources{
		Routed: listed(running, id),
		Ledger: readers.ledger,
		Prices: pricing(cfg),
		Keep:   cfg.Ledger.Keep,
		Now:    now,
	})
	if !ok {
		return fmt.Errorf("no session %s in the ledger, or among the router's sessions", status.Clean(redact.Text(given)))
	}
	if asJSON {
		return writeJSON(out, page)
	}
	return writeSessionPage(out, page, now)
}

// sessionAmong returns the id of the session given names among those read,
// those of today's lines and those the router lists, as sessionNamed says:
// given as it is where it starts none of their ids, as the whole id of a
// session of an earlier day.
func sessionAmong(given string, l views.Ledger, running []status.Session) (string, error) {
	todays, _, _ := l.Today(ledger.Mark{})
	ids := make([]string, 0, len(todays)+len(running))
	for _, h := range todays {
		ids = append(ids, h.Session)
	}
	for _, s := range running {
		ids = append(ids, s.ID)
	}
	return sessionNamed(given, ids, func(id string) string { return id }, status.Clean)
}

// listed returns the session with the given id among those the router lists:
// nil where it isn't one of them.
func listed(running []status.Session, id string) *status.Session {
	i := slices.IndexFunc(running, func(s status.Session) bool { return s.ID == id })
	if i < 0 {
		return nil
	}
	return &running[i]
}

// writeSessionPage writes p as a session's page's text, at now: its id cut
// short and its directory; what it's doing, where it runs and why, and when
// it started; its models, while it runs, else how long its lines are kept;
// the accounts it ran on, a row each; and its totals, then its turns, a row
// each, newest first.
func writeSessionPage(out io.Writer, p views.SessionPage, now time.Time) error {
	heading := "session " + status.ShortID(status.Clean(p.Session))
	if p.Dir != "" {
		heading += "  " + status.Clean(p.Dir)
	}
	doingNow := []string{doing(p.State, p.Running, p.LastSeen, p.Ended, now)}
	if where := whereRuns(p, now); where != "" {
		doingNow = append(doingNow, where)
	}
	if !p.Started.IsZero() {
		doingNow = append(doingNow, startedSaid(p, now))
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n%s\n%s\n", heading, strings.Join(doingNow, status.Separator), goesOrKept(p, now))
	if len(p.Accounts) > 0 {
		b.WriteString("\n")
		if err := writeTrimmed(&b, ranOnRows(p, now)); err != nil {
			return err
		}
	}
	t := p.Totals
	fmt.Fprintf(&b, "\n%s%s%s%s%d%% from cache%s%s at API prices\n", turnsCount(t.Turns), status.Separator, requestsCount(t.Requests), status.Separator,
		int(math.Round(t.FromCache*100)), status.Separator, worthSaid(t.Worth, t.Unpriced))
	if len(p.Turns) > 0 {
		if err := writeTrimmed(&b, turnRows(p, now)); err != nil {
			return err
		}
	}
	_, err := io.WriteString(out, b.String())
	return err
}

// whereRuns says where the session p runs and why, at now: on the account its
// last-used model goes to, since the move that brought it there, as "on side
// since 14:31, moved from work: work reached its cap", else since it
// started, as "on work since it started"; or, of one ended, where it ran, as
// ranOn says: "" where it ran on no account.
func whereRuns(p views.SessionPage, now time.Time) string {
	if !p.Running {
		return ranOn(p, now)
	}
	if len(p.Models) == 0 {
		return ""
	}
	on := p.Models[0]
	where := "on " + status.Clean(on.Account)
	if m, ok := lastMoveOnto(p.Moves, on.Account); ok {
		return where + " since " + status.Past(now, m.At) + ", " + movedWords(m, now)
	}
	words := status.SessionWords(on.Reason, status.Facts{To: on.Account, Now: now}).String()
	switch {
	case len(p.Accounts) > 0 && p.Accounts[0].Account != on.Account:
		return where + ": " + words
	case on.Reason == status.ReasonNew || words == "":
		return where + " since it started"
	}
	return where + " since it started: " + words
}

// lastMoveOnto returns the last of moves onto the account with the given id,
// reporting false where none was.
func lastMoveOnto(moves []views.SessionMove, account string) (views.SessionMove, bool) {
	for _, m := range slices.Backward(moves) {
		if m.To == account {
			return m, true
		}
	}
	return views.SessionMove{}, false
}

// movedWords says why m moved its session, as a session's page says it:
// "moved from work: work reached its cap" for a move off an account that
// couldn't take it, else as SessionWords has it, as "rescored after 15h
// idle: personal had the most room".
func movedWords(m views.SessionMove, now time.Time) string {
	words := status.SessionWords(m.Reason, status.Facts{From: m.From, To: m.To, Now: now}).String()
	if strings.HasPrefix(m.Reason, status.ReasonMovedOff) {
		return "moved from " + status.Clean(m.From) + ": " + words
	}
	return words
}

// ranOn says where the session p, ended, ran: on each account, in turn, with
// when each move came and why, as "ran on work, then side from 16:02: work
// reached its cap".
func ranOn(p views.SessionPage, now time.Time) string {
	if len(p.Accounts) == 0 {
		return ""
	}
	parts := []string{"ran on " + status.Clean(p.Accounts[0].Account)}
	for _, m := range p.Moves {
		why := status.EventWords(m.Reason, status.Facts{From: m.From, To: m.To, Now: now}).String()
		parts = append(parts, "then "+status.Clean(m.To)+" from "+status.Past(now, m.At)+": "+why)
	}
	return strings.Join(parts, ", ")
}

// startedSaid says when the session p started, at now, and how long it has
// run since, as "started 12:02 · 2h 40m"; when it came back after a while
// without a request, as "started yesterday 16:40 · resumed 09:12"; or how
// long it ran, of one ended, as "started yesterday 13:20 · ran 5h 27m".
func startedSaid(p views.SessionPage, now time.Time) string {
	started := "started " + status.Past(now, p.Started)
	switch {
	case !p.Running && !p.Ended.After(p.Started):
		return started + " · ran 0m"
	case !p.Running:
		return started + " · ran " + status.Countdown(p.Started, p.Ended)
	case !p.Resumed.IsZero():
		return started + " · resumed " + status.Past(now, p.Resumed)
	}
	return started + " · " + status.Countdown(p.Started, now)
}

// goesOrKept says, of the session p running, where each of its models goes,
// as "opus-5-5 → side   haiku-4-5 → side"; of one ended, until when the
// ledger keeps the lines of its first day, as "kept until 29 Dec", the year
// named where it isn't now's, or "kept for good".
func goesOrKept(p views.SessionPage, now time.Time) string {
	if p.Running {
		goes := make([]string, len(p.Models))
		for i, m := range p.Models {
			goes[i] = shortModel(m.Model) + " → " + status.Clean(m.Account)
		}
		return strings.Join(goes, "   ")
	}
	if p.KeptUntil == "" {
		return "kept for good"
	}
	until, err := time.ParseInLocation(time.DateOnly, p.KeptUntil, now.Location())
	if err != nil {
		return "kept until " + p.KeptUntil
	}
	layout := "2 Jan"
	if until.Year() != now.Year() {
		layout += " 2006"
	}
	return "kept until " + until.Format(layout)
}

// shortModel is a model's id less its "claude-", cleaned, as "opus-5-5".
func shortModel(model string) string {
	return strings.TrimPrefix(status.Clean(model), "claude-")
}

// ranOnRows are the rows of the accounts the session p ran on, at now: each
// with its time there, from its first request there to its last, or now, for
// the one it's on while it runs; its requests; the points of each window
// they took; and their worth.
func ranOnRows(p views.SessionPage, now time.Time) [][]string {
	var on string
	if p.Running && len(p.Models) > 0 {
		on = p.Models[0].Account
	}
	rows := make([][]string, len(p.Accounts))
	for i, a := range p.Accounts {
		to := status.Past(now, a.To)
		if a.Account == on {
			to = "now"
		}
		rows[i] = []string{status.Clean(a.Account), status.Past(now, a.From) + " – " + to, requestsCount(a.Requests), pointsSaid(a.Points),
			worthSaid(a.Worth, a.Unpriced)}
	}
	return rows
}

// pointsSaid says the points of an account's windows a session took, the
// shortest window first, each named as the views name it, as "≈ 41 points of
// its 5-hour · 6 of its week": "" for none.
func pointsSaid(points map[string]int) string {
	keys := slices.SortedFunc(maps.Keys(points), quota.CompareKeys)
	said := make([]string, len(keys))
	for i, key := range keys {
		of := " of its " + status.InProse(claude.WindowName(key))
		switch n := points[key]; {
		case i > 0:
			said[i] = strconv.Itoa(n) + of
		case n == 1:
			said[i] = "≈ 1 point" + of
		default:
			said[i] = "≈ " + strconv.Itoa(n) + " points" + of
		}
	}
	return strings.Join(said, " · ")
}

// turnHeads are the heads of the columns of a session's turns.
var turnHeads = []string{"#", "started", "took", "requests", "tools", "read", "written", "out", "worth", "on"}

// turnRows are the rows of the session p's turns, newest first, at now,
// under their heads.
func turnRows(p views.SessionPage, now time.Time) [][]string {
	rows := [][]string{turnHeads}
	for _, t := range p.Turns {
		rows = append(rows, []string{strconv.Itoa(t.Turn), status.Past(now, t.Started), turnTook(t, now), thousands(t.Requests), toolsSaid(t.Tools),
			prose.Count(t.Read), prose.Count(t.Written), prose.Count(t.Out), worthSaid(t.Worth, t.Unpriced), accountsSaid(t.Accounts)})
	}
	return rows
}

// turnTook says how long a turn took, in minutes, from the minute it started
// to the one it ended in, as "3m"; and of one still going, to the minute now
// is, "…" after it.
func turnTook(t views.Turn, now time.Time) string {
	end, going := t.Ended, t.Ended.IsZero()
	if going {
		end = now
	}
	minutes := strconv.Itoa(int(end.Truncate(time.Minute).Sub(t.Started.Truncate(time.Minute)) / time.Minute))
	if going {
		return minutes + "m…"
	}
	return minutes + "m"
}

// toolsSaid says the three tools a turn's answers called most, most first,
// with how many times each, as "Bash ×14, Edit ×9, Read ×8".
func toolsSaid(tools []views.ToolCalls) string {
	said := make([]string, 0, 3)
	for _, t := range tools[:min(len(tools), 3)] {
		said = append(said, status.Clean(t.Tool)+" ×"+strconv.Itoa(t.Times))
	}
	return strings.Join(said, ", ")
}

// accountsSaid says the accounts a turn went to, in the order it did, as the
// page draws a turn that moved: "work → side".
func accountsSaid(accounts []string) string {
	said := make([]string, len(accounts))
	for i, a := range accounts {
		said[i] = status.Clean(a)
	}
	return strings.Join(said, " → ")
}

// turnsCount counts turns in words, as "13 turns" or "1 turn".
func turnsCount(n int) string {
	if n == 1 {
		return "1 turn"
	}
	return thousands(n) + " turns"
}
