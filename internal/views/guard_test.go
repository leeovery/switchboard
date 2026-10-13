package views_test

import (
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
	// traffic and traces, the request stream's events into the Traffic and
	// views.Sent; its read loop, which counts the stream's lead of requests
	// in flight, and asks whether an event moves a number; and the verbs'
	// JSON, which passes lines and events through as the ledger and the
	// router filed them.
	"internal/dashboard/watch/traffic.go",
	"internal/dashboard/watch/traces.go",
	"internal/dashboard/watch/stream.go:joined",
	"internal/dashboard/watch/stream.go:Model.numbering",
	"internal/cli/requests.go:writeLines",
	"internal/cli/events.go:eventsJSON.follow",
	// Milestone 5's views, which stages 12 and 13 delete, and what they alone
	// read. Its panel.go and plain.go read none.
	"internal/dashboard/cords.go",
	"internal/dashboard/log.go",
	"internal/dashboard/recent_log.go",
	"internal/dashboard/seats.go",
	"internal/dashboard/sessions.go",
	// requests' text: a request's row (R1), its status and whether it failed
	// (R1b), its tokens (F1), the session it names (I1), the lines of an
	// account or a session, and the day each falls on (T6).
	"internal/cli/requests.go:accountLines",
	"internal/cli/requests.go:answered",
	"internal/cli/requests.go:requestRow",
	"internal/cli/requests.go:sessionHold.add",
	"internal/cli/requests.go:tokensIn",
	"internal/cli/requests.go:tokensInOut",
	"internal/cli/requests.go:writeRequests",
	// events' text: an event's line (R3), the session it names (I1), and the
	// day it falls on (T6).
	"internal/cli/events.go:eventsText.follow",
	"internal/cli/events.go:eventsText.list",
	"internal/cli/events.go:eventsText.row",
	// history's text: a day's figures, as the summary has them (R4), its
	// tokens (F1), worth (F2) and models' names (I4), its limits reached and
	// windows named (W4, W5), and the moves of its sessions.
	"internal/cli/history.go:accountNote",
	"internal/cli/history.go:eventful",
	"internal/cli/history.go:highestOf",
	"internal/cli/history.go:historyTable",
	"internal/cli/history.go:historyTables",
	"internal/cli/history.go:limitsOf",
	"internal/cli/history.go:modelName",
	"internal/cli/history.go:modelRows",
	"internal/cli/history.go:requestCount",
	"internal/cli/history.go:weekResets",
	"internal/cli/history.go:worthOf",
	// A session named among the router's by its id (R2), those an id starts
	// listed as status's text lists them.
	"internal/cli/session.go:app.findSession",
	"internal/cli/sessionpage.go:listed",
	// The sessions running, as the router listed them and the request stream
	// has moved and seen them since: when each was last seen (J6), and the
	// sessions on each account (F12).
	"internal/dashboard/overview_sessions.go:Frame.seenListing",
	"internal/dashboard/traffic.go:Frame.listing",
	// The events newer than the last look's, by their ids (J8).
	"internal/dashboard/watch/news.go:news.looked",
	// Whether the router's sessions are placed as they were, for the routing
	// card to price a move afresh (F4).
	"internal/dashboard/watch/routing.go:samePlace",
	"internal/dashboard/watch/routing.go:samePlaces",
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
		"ledger.Priced", "ledger.PricedModel", "ledger.Reply", "ledger.Shape", "ledger.Summary", "ledger.Tried",
		"quota.Tokens",
		"readings.Reading",
		"router.StreamEvent",
		"status.Assignment", "status.Event", "status.Session",
	}
	// readsRaw are the functions that work a fact out of raw data: a drawing
	// package calls none, whether or not it takes raw data.
	readsRaw = []string{
		"claude.LimitWindows",
		"ledger.Line.Upstream",
		"ledger.Reply.CacheLife", "ledger.Reply.Tokens",
		"ledger.Table.CacheWrite", "ledger.Table.Priced", "ledger.Table.Request", "ledger.Table.Rewrite",
		"ledger.Table.Summed", "ledger.Table.Worth",
	}
	// absorbed are what a rule's one owner took the place of, each with its
	// owner: a drawing package calls none, but its owner.
	absorbed = map[string]string{
		// A time, 16:30 today and Sun 21:00 another day.
		"status.When": "status.Dated",
	}
	// absorbedIn are the packages, by directory from the module's root, what
	// absorbed names is absorbed in, where it isn't in every drawing package.
	absorbedIn = map[string][]string{}
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
)

// absorbedSays says how using what's absorbed breaks the rule, and its owner.
func absorbedSays(name, owner string) string {
	return name + " is absorbed by " + owner + "; call " + owner + ", the rule's one owner"
}

func TestDrawingReadsTheViewsAlone(t *testing.T) {
	root := sourcecheck.ModuleRoot(t)
	sourcecheck.ReportEach(t, drawingBreaches(t, root, sourcecheck.ModulePath(t, root), drawingPackages, readersAllowed, callsAllowed))
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
	}, []string{"ledger.Held.Filed", "status.Session.Line", "ledger.Held.Gone"})
	want := []string{
		"internal/cli/calls.go:14: ledger.Table.Summed works a fact out of raw data; " + readsRawFix,
		"internal/cli/calls.go:14: ledger.Held.Line reads raw data's fields; " + rawFix,
		"internal/cli/calls.go:15: ledger.Line.Upstream works a fact out of raw data; " + readsRawFix,
		"internal/cli/calls.go:16: ledger.Held.Reply reads raw data's fields; " + rawFix,
		"internal/cli/calls.go:16: ledger.Reply.Tokens works a fact out of raw data; " + readsRawFix,
		"internal/cli/calls.go:17: claude.LimitWindows works a fact out of raw data; " + readsRawFix,
		"internal/cli/calls.go:18: ledger.Line.Failed takes raw data, ledger.Line; " + callFix,
		"internal/cli/calls.go:19: ledger.Count takes raw data, ledger.Line; " + callFix,
		"internal/cli/calls.go:20: status.Grouped takes raw data, status.Session; " + callFix,
		"internal/cli/calls.go:21: " + absorbedSays("status.When", "status.Dated"),
		"internal/cli/calls.go:22: " + absorbedSays("status.When", "status.Dated"),
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
		"internal/dashboard/dashboard.go:9: status.Event.ID reads raw data's fields; " + rawFix,
		"internal/cli/kept.go: readersAllowed allows it, but it keeps the rule; " + allowedFix,
		"internal/cli/gone.go: readersAllowed allows it, but it's gone; " + allowedFix,
		"internal/cli/readers.go:keptReader: readersAllowed allows it, but it keeps the rule; " + allowedFix,
		"internal/cli/readers.go:goneReader: readersAllowed allows it, but it's gone; " + allowedFix,
		"ledger.Held.Gone: callsAllowed allows it, but no drawing package calls it; " + allowedCallFix,
	}
	if !slices.Equal(got, want) {
		t.Errorf("found\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// drawingFixture is a module of the raw data's types, the functions that
// work facts out of them, what's absorbed and its owner, and functions and
// methods that take raw data, as one added later would; views, whose
// functions take raw data, as the surface does; and two drawing packages:
// the CLI, with a file of each kind of breach, one that keeps the rule,
// calling what's allowed, a file and functions the allow-list allows, and
// ones it allows that keep the rule; and the dashboard, with a breach of its
// own.
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

func When(now, t time.Time) string          { return "" }
func Dated(now, t time.Time) string         { return "" }
func Clean(text string) string              { return text }
func (s Session) Line(now time.Time) string { return s.ID }
func Grouped(by map[string]Session) int     { return len(by) }
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

func RowOf(h ledger.Held) Row { return Row{Session: h.Session} }
`,
	"internal/cli/calls.go": `package cli

import (
	"time"

	"example.com/fixture/internal/claude"
	"example.com/fixture/internal/ledger"
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
		status.When(now, now),
		status.When,
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
	"example.com/fixture/internal/status"
	"example.com/fixture/internal/views"
)

func clean(r ledger.Reader, s status.Session, now time.Time) string {
	rows := make([]views.Row, 0)
	for _, h := range r.Lines() {
		rows = append(rows, views.RowOf(h))
		_ = h.Filed()
	}
	return rows[0].Session + status.Dated(now, now) + status.Clean("") + s.Line(now)
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

import "example.com/fixture/internal/status"

func drawn(events []status.Event) int {
	if len(events) == 0 {
		return 0
	}
	return events[0].ID
}
`,
}

// drawingBreaches are where the non-test files of the packages in dirs, of
// the module at root with the given path, break the rule, but in the files
// and functions allowed: each read of raw data's field, through any field
// it's embedded in; each use of a function that works a fact out of raw
// data, or of what an owner absorbed, called or not; and each call of any
// other function or method of rawPackages that takes raw data, but the calls
// allowed. Then each file or function allowed that's gone, or keeps the rule,
// and each call allowed that none of the rest makes.
func drawingBreaches(t *testing.T, root, module string, dirs, allowed, calls []string) []string {
	t.Helper()
	return sourcecheck.Check(t, root, module, drawingRule(module, calls), dirs, sourcecheck.Allowance{
		Entries: allowed, List: "readersAllowed", Fix: allowedFix,
		Calls: "callsAllowed", Callers: "drawing package", CallFix: allowedCallFix,
	})
}

// drawingRule is the views' rule, of the module with the given path, the
// calls given allowed: no raw data's field read; no fact worked out of raw
// data, nor what an owner absorbed; and no other function or method of
// rawPackages called that takes raw data.
func drawingRule(module string, calls []string) sourcecheck.Rule {
	used := make(map[string]sourcecheck.Ban)
	for _, name := range readsRaw {
		used[name] = sourcecheck.Ban{Says: name + " works a fact out of raw data; " + readsRawFix}
	}
	for name, owner := range absorbed {
		used[name] = sourcecheck.Ban{Says: absorbedSays(name, owner), In: absorbedIn[name]}
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
