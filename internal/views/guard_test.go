package views_test

import (
	"go/types"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/sourcecheck"
)

// These tests read the module's source, so a drawing package can't work out
// a fact of the ledger's lines, the router's events or sessions, or the
// request stream for itself, rather than read what views makes of them, where
// each rule two views share has one owner, without failing here. They stand
// beside the status guard, which holds every view to the moment, and live
// with views as that guard lives with status: the rule is the surface's.

// drawingPackages are the packages that draw what views make, by directory
// from the module's root: each reads the raw data through views alone.
var drawingPackages = []string{
	"internal/cli",
	"internal/dashboard",
	"internal/dashboard/watch",
}

// readersAllowed are the files of drawingPackages allowed to break the rule,
// by path from the module's root, and their functions, by path, a colon and
// the function's name, a method's its receiver's type's then its own. The
// list only shrinks: an entry must still be there, and still break the rule.
// Each reader's comment names the rules of what it reads, as the survey of
// milestone 7's shared rules names them.
var readersAllowed = []string{
	// Producers, which turn the raw data into what views read: the watch's
	// traffic, the request stream's events into views.Told and the Traffic;
	// its read loop, which counts the stream's lead of requests in flight,
	// and asks whether an event moves a number; and the verbs' JSON, which
	// passes lines and events through as the ledger and the router filed
	// them.
	"internal/dashboard/watch/traffic.go",
	"internal/dashboard/watch/stream.go:joined",
	"internal/dashboard/watch/stream.go:Model.numbering",
	"internal/cli/requests.go:writeLines",
	"internal/cli/events.go:eventsJSON.follow",
}

// callsAllowed are the functions and methods of rawPackages that take raw
// data, as their receiver or a parameter, that a drawing package may call all
// the same, each by its name, under the module's internal directory. Any
// other fails, as one added later would. The list only shrinks: one no
// drawing package calls fails.
var callsAllowed = []string{
	// status's text, the status verb's, which lists the router's sessions a
	// line each; and its --session's answer, the account a session's
	// last-used model goes to.
	"status.Moment.Text", "status.Session.AccountID",
}

// What breaks the rule, each by its package's name under the module's
// internal directory, then its type's, where it's a method, then its own.
var (
	// rawTypes are the raw data views read, whose fields hold the facts as
	// they were written, for views to make something of: a drawing package
	// reads none of their fields.
	rawTypes = []string{
		"events.Held", "events.Line",
		"ledger.AccountDay", "ledger.Answer", "ledger.Held", "ledger.Hints", "ledger.Line", "ledger.ModelDay",
		"ledger.Priced", "ledger.PricedModel", "ledger.Reply", "ledger.Shape", "ledger.Spend", "ledger.Summary", "ledger.Tried",
		"quota.Tokens",
		"readings.Reading",
		"router.StreamEvent",
		"status.Assignment", "status.Event", "status.Session",
	}
	// readsRaw are the functions that work a fact out of raw data: a drawing
	// package calls none, whether or not it takes raw data.
	readsRaw = []string{
		"claude.LimitStatus", "claude.LimitWindows", "claude.WindowsRead",
		"claude.KeptWindows", "claude.WindowsReadOf",
		"ledger.Failing", "ledger.Line.Upstream",
		"ledger.Reply.CacheLife", "ledger.Reply.Tokens", "ledger.Spend.CacheLife", "ledger.Spend.Tokens", "ledger.SpendOf",
		"ledger.Table.CacheWrite", "ledger.Table.CacheWriteSpend", "ledger.Table.Priced", "ledger.Table.Request", "ledger.Table.Rewrite",
		"ledger.Table.RewriteSpend", "ledger.Table.SamePrices", "ledger.Table.Summed", "ledger.Table.SummedSpend", "ledger.Table.Worth",
	}
	// absorbed are what a rule's one owner took the place of, each with its
	// owner: a drawing package calls none, but its owner.
	absorbed = map[string]string{
		// A time, 16:30 today and Sun 21:00 another day.
		"prose.When": "prose.Dated",
		// A session's id, as the dashboard names a session by it.
		"status.ShortID": "status.ShownID",
		// A window's name where no document labels it, as "5-hour" or
		// "Week", never Claude's own label.
		"claude.WindowLabel": "status.NameOfKey",
		// How long a session's turn took, the minutes it spans, both ends
		// counted, as "9m".
		"prose.Minutes": "views.Turn.Took",
		// A session's time on an account, as "yesterday 16:40 – 18:05".
		"prose.Stint": "views.SessionPage.Stays",
		// Where a session runs and how it came there, as row 6 says it.
		"status.SessionWords": "views.SessionPage.Where",
		// Why a session runs where it does, short, as a phone's row 6 says
		// it: a move's label, short, is views.SessionMove.Said's.
		"status.ShortWords": "views.SessionPage.Where",
		// The account a forced move left, and whether a reason moved a
		// session at all, as row 6 tells how it came where it runs.
		"status.MovedFrom":    "views.SessionPage.Where",
		"status.MovedNothing": "views.SessionPage.Where",
		// When an ended session's move came, its day named only off the day
		// it started, as "from 16:02".
		"prose.Later": "views.SessionPage.Where",
		// What moved a session and where to, as its story's label says it:
		// an event's own words are views.Telling's.
		"status.EventWords": "views.SessionMove.Said",
		// A fold's length on a story's axis, in whole hours, as "15h".
		"prose.Hours": "views.Story.Columns",
		// What an account's week had left as it last reset, and the day it ran
		// out, as Accounts says them, whatever its period.
		"views.AccountTotals.LeftAtReset": "views.AccountsRow.WeekLeft",
		"views.AccountTotals.RanOut":      "views.AccountsRow.RanOut",
		// How an account stood at a moment gone, and the holds the router
		// told of, as Runway's lanes draw the past, live or stepped back to.
		"status.Moment.Then": "views.Lanes.Look",
		"status.Moment.Told": "views.Lanes.Look",
		// How many limits were reached, every window's together, and each
		// window's, as Accounts, history and the Year count and say them.
		"views.AccountTotals.Limits": "views.AccountTotals' LimitCount and LimitsHit",
		"views.Month.Limits":         "views.Month.LimitCount",
		// The time spent at a cap or a limit, together, and whether it's
		// known.
		"views.AccountTotals.MinutesAtCap":   "views.AccountTotals.AtCapOrLimit",
		"views.AccountTotals.MinutesAtLimit": "views.AccountTotals.AtCapOrLimit",
	}
	// absorbedIn are the packages, by directory from the module's root, what
	// absorbed names is absorbed in, where it isn't in every drawing package.
	absorbedIn = map[string][]string{
		// The verbs keep 8 characters of an id, as logs and status's text do.
		"status.ShortID": {"internal/dashboard", "internal/dashboard/watch"},
		// history's text and JSON give every window's, over the days asked
		// for.
		"views.AccountTotals.LeftAtReset": {"internal/dashboard", "internal/dashboard/watch"},
		"views.AccountTotals.RanOut":      {"internal/dashboard", "internal/dashboard/watch"},
	}
	// builtInTheWatch are what the watch builds once each change, as each
	// reads the ledger or the router's events, places every line, or keeps
	// who sits where on Sessions' Switchboard from look to look, for the
	// dashboard to draw as it's held: the dashboard calls none. Each is one
	// views declares, or the guard fails, as a ban on a name that's gone
	// passes whatever the dashboard builds.
	builtInTheWatch = []string{
		"views.CallsOf", "views.DaySessions", "views.EventsRead.Back", "views.EventsRead.Look", "views.EventsRead.Refiled",
		"views.EventsRead.Skips", "views.EventsRead.Took", "views.EventsSince", "views.EventsTable.Details",
		"views.FindSessions", "views.HeldSessions.Awaited", "views.HeldSessions.Fed", "views.HeldSessions.Holding",
		"views.HeldSessions.List", "views.HeldSessions.Unread", "views.HeldSessions.With", "views.HeldSessions.Without",
		"views.HoldSessions", "views.Lanes.Look", "views.ListSessions", "views.ListWanted", "views.LogEvents", "views.LogFilter.KeptFrom",
		"views.LogRows", "views.LogSessions", "views.Patch", "views.ReadEvents",
		"views.RunwayReadings", "views.StubsOf", "views.Today", "views.Took", "views.TotalsOn",
		"views.EventsRead.From", "views.HeldSessions.Unheld", "views.LogLines.Rows", "views.LogLinesOf", "views.LogRead.From", "views.LogRead.Gen", "views.LogRead.HoldsTold",
		"views.LogRead.Taken", "views.LogRead.Wanted", "views.ReadLog", "views.SessionGains.Retell", "views.StreamWanted",
	}
)

// rawPackages are the packages, by name under the module's internal
// directory, whose functions and methods that take raw data a drawing package
// calls only where callsAllowed names them.
var rawPackages = []string{"events", "ledger", "quota", "readings", "router", "status"}

// The fix each kind of breach gives.
const (
	rawFix         = "read what views makes of it, where each rule two views share has one owner"
	readsRawFix    = "read the fact views works out of it"
	callFix        = "read what views makes of it, or name the call in callsAllowed in internal/views/guard_test.go, saying why a drawing package may make it"
	allowedFix     = "remove it from readersAllowed in internal/views/guard_test.go"
	allowedCallFix = "remove it from callsAllowed in internal/views/guard_test.go"
	builtFix       = "remove it from builtInTheWatch in internal/views/guard_test.go"
)

// absorbedSays says how using what's absorbed breaks the rule, and its owner.
func absorbedSays(name, owner string) string {
	return name + " is absorbed by " + owner + "; use " + owner + ", the rule's one owner"
}

// builtSays says how using what the watch builds breaks the rule.
func builtSays(name string) string {
	return name + " is built in the watch once each change; draw what it holds"
}

func TestDrawingReadsTheViewsAlone(t *testing.T) {
	root := sourcecheck.ModuleRoot(t)
	sourcecheck.ReportEach(t, drawingBreaches(t, root, sourcecheck.ModulePath(t, root), drawingPackages, readersAllowed, callsAllowed, builtInTheWatch))
}

func TestTheDrawingGuardFindsWhatItGuardsAgainst(t *testing.T) {
	root := sourcecheck.WriteModule(t, drawingFixture)
	got := drawingBreaches(t, root, "example.com/fixture", []string{"internal/cli", "internal/dashboard"}, []string{
		"internal/cli/allowed.go",
		"internal/cli/kept.go",
		"internal/cli/gone.go",
		"internal/cli/readers.go:allowedReader",
		"internal/cli/readers.go:printer.allowedMethod",
		"internal/cli/readers.go:keptReader",
		"internal/cli/readers.go:goneReader",
	}, []string{"ledger.Held.Filed", "status.Session.Line", "ledger.Held.Gone"}, []string{
		"views.LogRows", "views.LogRead.Grown", "views.Gone", "views.LogRead.Gone", "views.Gone.Grown", "views.Row", "views.Row.Session",
	})
	want := []string{
		"internal/cli/calls.go:15: ledger.Table.Summed works a fact out of raw data; " + readsRawFix,
		"internal/cli/calls.go:15: ledger.Held.Line reads raw data's fields; " + rawFix,
		"internal/cli/calls.go:16: ledger.Line.Upstream works a fact out of raw data; " + readsRawFix,
		"internal/cli/calls.go:17: ledger.Held.Reply reads raw data's fields; " + rawFix,
		"internal/cli/calls.go:17: ledger.Reply.Tokens works a fact out of raw data; " + readsRawFix,
		"internal/cli/calls.go:18: claude.LimitWindows works a fact out of raw data; " + readsRawFix,
		"internal/cli/calls.go:19: ledger.Line.Failed takes raw data, ledger.Line; " + callFix,
		"internal/cli/calls.go:20: ledger.Count takes raw data, ledger.Line; " + callFix,
		"internal/cli/calls.go:21: status.Grouped takes raw data, status.Session; " + callFix,
		"internal/cli/calls.go:22: " + absorbedSays("prose.When", "prose.Dated"),
		"internal/cli/calls.go:23: " + absorbedSays("prose.When", "prose.Dated"),
		"internal/cli/fields.go:13: ledger.Held.At reads raw data's fields; " + rawFix,
		"internal/cli/fields.go:14: ledger.Held.Usage reads raw data's fields; " + rawFix,
		"internal/cli/fields.go:15: ledger.Line.Status reads raw data's fields; " + rawFix,
		"internal/cli/fields.go:16: quota.Tokens.Input reads raw data's fields; " + rawFix,
		"internal/cli/fields.go:17: router.StreamEvent.Kind reads raw data's fields; " + rawFix,
		"internal/cli/fields.go:18: status.Session.Assignments reads raw data's fields; " + rawFix,
		"internal/cli/fields.go:18: status.Assignment.Account reads raw data's fields; " + rawFix,
		"internal/cli/fields.go:20: events.Held.Line reads raw data's fields; " + rawFix,
		"internal/cli/fields.go:20: events.Line.Event reads raw data's fields; " + rawFix,
		"internal/cli/fields.go:20: status.Event.ID reads raw data's fields; " + rawFix,
		"internal/cli/fields.go:25: ledger.Held.Session reads raw data's fields; " + rawFix,
		"internal/cli/readers.go:13: ledger.Held.Session reads raw data's fields; " + rawFix,
		"internal/dashboard/dashboard.go:12: status.Event.ID reads raw data's fields; " + rawFix,
		"internal/dashboard/dashboard.go:15: " + absorbedSays("status.ShortID", "status.ShownID"),
		"internal/dashboard/dashboard.go:17: " + builtSays("views.LogRows"),
		"internal/dashboard/dashboard.go:19: " + builtSays("views.LogRead.Grown"),
		"internal/cli/kept.go: readersAllowed allows it, but it keeps the rule; " + allowedFix,
		"internal/cli/gone.go: readersAllowed allows it, but it's gone; " + allowedFix,
		"internal/cli/readers.go:keptReader: readersAllowed allows it, but it keeps the rule; " + allowedFix,
		"internal/cli/readers.go:goneReader: readersAllowed allows it, but it's gone; " + allowedFix,
		"ledger.Held.Gone: callsAllowed allows it, but no drawing package calls it; " + allowedCallFix,
		"views.Gone: builtInTheWatch bans it, but views declares no such function; " + builtFix,
		"views.LogRead.Gone: builtInTheWatch bans it, but views declares no such function; " + builtFix,
		"views.Gone.Grown: builtInTheWatch bans it, but views declares no such function; " + builtFix,
		"views.Row: builtInTheWatch bans it, but views declares no such function; " + builtFix,
		"views.Row.Session: builtInTheWatch bans it, but views declares no such function; " + builtFix,
	}
	if !slices.Equal(got, want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// drawingFixture is a module of the raw data's types, the functions that
// work facts out of them, what's absorbed and its owner, and functions and
// methods that take raw data, as one added later would; views, whose
// functions take raw data, as the surface does, and build what the watch
// builds, beside a type and its field, which are no functions; and two
// drawing packages: the CLI, with a file of each kind of
// breach, one that keeps the rule, calling what's allowed and building the
// Log's table, a file and functions the allow-list allows, and ones it
// allows that keep the rule; and the dashboard, with a breach of its own,
// and the Log's table built and grown in it.
var drawingFixture = map[string]string{
	"go.mod": "module example.com/fixture\n",
	"internal/quota/quota.go": `package quota

type Tokens struct{ Input, Output int }
`,
	"internal/ledger/ledger.go": `package ledger

import (
	"time"

	"example.com/fixture/internal/quota"
)

type Line struct {
	At      time.Time
	Session string
	Status  int
	Reply
}

type Reply struct{ Usage []byte }

type Held struct {
	Line
	JSON []byte
}

type Table struct{}

type Reader struct{}

func (t Table) Summed(l *Line, date string) (int, bool) { return 0, false }
func (l *Line) Upstream() bool                         { return false }
func (l *Line) Failed() bool                           { return l.Status >= 400 }
func (r Reply) Tokens() (quota.Tokens, bool)           { return quota.Tokens{}, false }
func (h Held) Filed() []byte                           { return h.JSON }
func (h Held) Gone()                                   {}
func (r Reader) Lines() []Held                         { return nil }
func Count(lines []Line) int                           { return len(lines) }
`,
	"internal/events/events.go": `package events

import "example.com/fixture/internal/status"

type Line struct{ Event status.Event }

type Held struct{ Line }
`,
	"internal/status/status.go": `package status

import "time"

type Event struct{ ID int }

type Session struct {
	ID          string
	Assignments []Assignment
}

type Assignment struct{ Account string }

func ShortID(id string) string              { return id }
func ShownID(id string) string              { return id }
func (s Session) Line(now time.Time) string { return s.ID }
func Grouped(by map[string]Session) int     { return len(by) }
`,
	"internal/prose/prose.go": `package prose

import "time"

func When(now, t time.Time) string  { return "" }
func Dated(now, t time.Time) string { return "" }
func Clean(text string) string      { return text }
`,
	"internal/router/router.go": `package router

type StreamEvent struct{ Kind string }
`,
	"internal/claude/claude.go": `package claude

func LimitWindows(limits map[string]string) []string { return nil }
`,
	"internal/views/views.go": `package views

import "example.com/fixture/internal/ledger"

type Row struct{ Session string }

type LogRead struct{}

func RowOf(h ledger.Held) Row    { return Row{Session: h.Session} }
func (r LogRead) Grown() LogRead { return r }
func LogRows(r LogRead) []Row    { return nil }
`,
	"internal/cli/calls.go": `package cli

import (
	"time"

	"example.com/fixture/internal/claude"
	"example.com/fixture/internal/ledger"
	"example.com/fixture/internal/prose"
	"example.com/fixture/internal/status"
)

func calls(h ledger.Held, by map[string]status.Session, now time.Time) []any {
	var table ledger.Table
	return []any{
		table.Summed(&h.Line, ""),
		h.Upstream(),
		h.Reply.Tokens,
		claude.LimitWindows,
		h.Failed(),
		ledger.Count(nil),
		status.Grouped(by),
		prose.When(now, now),
		prose.When,
	}
}
`,
	"internal/cli/fields.go": `package cli

import (
	"example.com/fixture/internal/events"
	"example.com/fixture/internal/ledger"
	"example.com/fixture/internal/quota"
	"example.com/fixture/internal/router"
	"example.com/fixture/internal/status"
)

func fields(h *ledger.Held, l ledger.Line, t quota.Tokens, e router.StreamEvent, s status.Session, held []events.Held) []any {
	return []any{
		h.At,
		h.Usage,
		l.Status,
		t.Input,
		e.Kind,
		s.Assignments[0].Account,
		func() int {
			return held[0].Line.Event.ID
		}(),
	}
}

var named = ledger.Held{}.Session
`,
	"internal/cli/clean.go": `package cli

import (
	"time"

	"example.com/fixture/internal/ledger"
	"example.com/fixture/internal/prose"
	"example.com/fixture/internal/status"
	"example.com/fixture/internal/views"
)

func clean(r ledger.Reader, s status.Session, now time.Time) string {
	rows := views.LogRows(views.LogRead{}.Grown())
	for _, h := range r.Lines() {
		rows = append(rows, views.RowOf(h))
		_ = h.Filed()
	}
	return rows[0].Session + prose.Dated(now, now) + prose.Clean("") + s.Line(now) + status.ShortID(rows[0].Session)
}
`,
	"internal/cli/readers.go": `package cli

import "example.com/fixture/internal/ledger"

type printer struct{ h ledger.Held }

func allowedReader(h ledger.Held) string { return h.Session + string(h.Filed()) }

func (p printer) allowedMethod() string { return p.h.Session }

func keptReader() string { return "" }

func unallowed(h ledger.Held) string { return h.Session }
`,
	"internal/cli/allowed.go": `package cli

import "example.com/fixture/internal/ledger"

func allowed(h ledger.Held) int { return h.Status }
`,
	"internal/cli/kept.go": `package cli

func kept() {}
`,
	"internal/cli/cli_test.go": `package cli

import "example.com/fixture/internal/ledger"

func tested(h ledger.Held) int { return h.Status }
`,
	"internal/dashboard/dashboard.go": `package dashboard

import (
	"example.com/fixture/internal/status"
	"example.com/fixture/internal/views"
)

func drawn(events []status.Event) int {
	if len(events) == 0 {
		return 0
	}
	return events[0].ID
}

func named(id string) string { return status.ShortID(id) + status.ShownID(id) }

func frame(r views.LogRead) int { return len(views.LogRows(r)) }

var grow = views.LogRead.Grown
`,
}

// drawingBreaches are where the non-test files of the packages in dirs, of
// the module at root with the given path, break the rule, but in the files
// and functions allowed: each read of raw data's field, through any field
// it's embedded in; each use of a function that works a fact out of raw
// data, of what an owner absorbed, or, in the dashboard, of what built names
// the watch building, called or not; and each call of any other function or
// method of rawPackages that takes raw data, but the calls allowed. Then each
// file or function allowed that's gone, or keeps the rule; each call allowed
// that none of the rest makes; and each of built views doesn't declare.
func drawingBreaches(t *testing.T, root, module string, dirs, allowed, calls, built []string) []string {
	t.Helper()
	problems := sourcecheck.Check(t, root, module, drawingRule(module, calls, built), dirs, sourcecheck.Allowance{
		Entries: allowed, List: "readersAllowed", Fix: allowedFix,
		Calls: "callsAllowed", Callers: "drawing package", CallFix: allowedCallFix,
	})
	return append(problems, builtGone(t, root, module, built, "builtInTheWatch", builtFix)...)
}

// builtGone are how each of built, as Names names a function, that views, of
// the module at root with the given path, doesn't declare breaks the rule,
// as the list named bans it, fixed as fix says.
func builtGone(t *testing.T, root, module string, built []string, list, fix string) []string {
	t.Helper()
	views := imported(t, root, module, "views")
	var gone []string
	for _, name := range built {
		if _, ok := lookup(views, name).(*types.Func); !ok {
			gone = append(gone, name+": "+list+" bans it, but views declares no such function; "+fix)
		}
	}
	return gone
}

// imported is the package by its path under the internal directory of the
// module at root with the given path, type-checked from its source.
func imported(t *testing.T, root, module, pkg string) *types.Package {
	t.Helper()
	imported, err := sourcecheck.NewLoader(t, root, module, nil).Import(module + "/internal/" + pkg)
	if err != nil {
		t.Fatal(err)
	}
	return imported
}

// lookup is what pkg declares by the name given, as Names names it:
// "views.LogRows", or, of a method or a field, "views.LogRead.Grown"; nil
// where it declares none.
func lookup(pkg *types.Package, name string) types.Object {
	declared, ok := strings.CutPrefix(name, pkg.Name()+".")
	if !ok {
		return nil
	}
	typ, member, isMember := strings.Cut(declared, ".")
	if !isMember {
		return pkg.Scope().Lookup(declared)
	}
	named, ok := pkg.Scope().Lookup(typ).(*types.TypeName)
	if !ok {
		return nil
	}
	found, _, _ := types.LookupFieldOrMethod(named.Type(), true, pkg, member)
	return found
}

// drawingRule is the views' rule, of the module with the given path, the
// calls given allowed: no raw data's field read; no fact worked out of raw
// data, nor what an owner absorbed; nothing built names the watch building
// built in the dashboard; and no other function or method of rawPackages
// called that takes raw data.
func drawingRule(module string, calls, built []string) sourcecheck.Rule {
	used := make(map[string]sourcecheck.Ban)
	for _, name := range readsRaw {
		used[name] = sourcecheck.Ban{Says: name + " works a fact out of raw data; " + readsRawFix}
	}
	for name, owner := range absorbed {
		used[name] = sourcecheck.Ban{Says: absorbedSays(name, owner), In: absorbedIn[name]}
	}
	for _, name := range built {
		used[name] = sourcecheck.Ban{Says: builtSays(name), In: []string{"internal/dashboard"}}
	}
	return sourcecheck.Rule{
		Names:   sourcecheck.NamesIn(module),
		Fields:  rawTypes,
		Read:    func(field string) string { return field + " reads raw data's fields; " + rawFix },
		Used:    used,
		Owners:  rawPackages,
		Takes:   func(fn, raw string) string { return fn + " takes raw data, " + raw + "; " + callFix },
		Allowed: calls,
	}
}
