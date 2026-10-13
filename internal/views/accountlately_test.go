package views_test

import (
	"cmp"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// toldLine is a line of Detail's LATELY: the kind its mark is drawn for, and
// its words, in full and short, short as full where it gives none.
type toldLine struct {
	kind, full, short string
}

// accountLately are doc's events as Detail's LATELY tells them of the account
// with the given id, every account's for "", at 16:00, new sessions going to
// the account best names.
func accountLately(doc status.Document, id, best string) []toldLine {
	doc.Best = best
	var told []toldLine
	for _, s := range views.AccountLately(doc.At(at(16, 0), claude.Policy), id, 12) {
		told = append(told, toldLine{kind: s.Kind, full: s.Full.String(), short: s.Short.String()})
	}
	return told
}

// Detail's LATELY tells each kind of event from the side of the account it's
// of, which it leaves unnamed: what befell it, the sessions another's cap,
// limit or refusal moved onto it, folded into one line as Lately folds them,
// or one move onto it alone where they went to several, the sessions that
// started on it, and the pins naming it, the others they name with it. Of
// every account, it tells the Overview's words, a session starting on the
// account new sessions go to "the best". Restarts, the router's health and
// routing given back to it name no account, so neither tells them.
func TestDetailsLATELYTellsEachKindFromTheAccountsSide(t *testing.T) {
	capped := func(id, count int) status.Event {
		return status.Event{ID: id, Kind: status.EventCap, Account: "work", Windows: []string{"5h"}, Count: count}
	}
	spread := status.Document{Source: status.SourceRouter, Events: []status.Event{
		{ID: 3, Kind: status.EventMoved, Session: "d28c0b1e", From: "work", To: "side", Reason: "moved: work is at its reserve", ForcedBy: 1},
		{ID: 2, Kind: status.EventMoved, Session: "9e21d4a8", From: "work", To: "spare", Reason: "moved: work is at its reserve", ForcedBy: 1},
		capped(1, 2),
	}}
	rescored := latelyOf(status.Event{Kind: status.EventMoved, Session: "db8a71e3", From: "side", To: "work", Reason: status.Rescored(15 * time.Hour)})
	started := func(account, reason string) status.Document {
		return latelyOf(status.Event{Kind: status.EventStarted, Account: account, Session: "c61b2f8d", Reason: reason})
	}
	pinnedBoth := latelyOf(status.Event{Kind: status.EventPin, Account: "personal", Accounts: []string{"personal", "side"}, By: "dashboard"})
	pinnedOwn := latelyOf(status.Event{Kind: status.EventPin, Account: "side", Session: "c61b2f8d"})
	unnamed := latelyOf(
		status.Event{Kind: status.EventAuto, Account: "side", Session: "c61b2f8d"},
		status.Event{Kind: status.EventAuto, Accounts: []string{"side"}, By: "dashboard"},
		status.Event{Kind: status.EventRestart, Reason: status.RestartForConfig},
		status.Event{Kind: status.EventHealth, Reason: "most requests failing"},
	)
	tests := []struct {
		name     string
		doc      status.Document
		id, best string
		want     []toldLine
	}{
		{name: "a cap, from the account it held back", doc: heldBackAt(status.EventCap, false), id: "work", want: []toldLine{
			{kind: status.EventCap, full: "reached its cap: 3 sessions moved to side", short: "at its cap: 3 moved to side"},
			{kind: status.EventStarted, full: "7f3a started here"}}},
		{name: "a cap, from the account its sessions moved to", doc: heldBackAt(status.EventCap, false), id: "side", want: []toldLine{
			{kind: status.EventStarted, full: "c61b started here"},
			{kind: status.EventMoved, full: "3 sessions came from work: work reached its cap", short: "3 came from work: work at its cap"}}},
		{name: "a limit, from the account it held back", doc: heldBackAt(status.EventLimit, false), id: "work", want: []toldLine{
			{kind: status.EventLimit, full: "reached its 5-hour limit: 3 sessions moved to side", short: "at its limit: 3 moved to side"},
			{kind: status.EventStarted, full: "7f3a started here"}}},
		{name: "a limit, from the account its sessions moved to", doc: heldBackAt(status.EventLimit, false), id: "side", want: []toldLine{
			{kind: status.EventStarted, full: "c61b started here"},
			{kind: status.EventMoved, full: "3 sessions came from work: work reached its 5-hour limit", short: "3 came from work: work at its limit"}}},
		{name: "a refusal, from the account it held back", doc: heldBackAt(status.EventRefused, false), id: "work", want: []toldLine{
			{kind: status.EventRefused, full: "refused (401): 3 sessions moved to side", short: "refused (401): 3 moved to side"},
			{kind: status.EventStarted, full: "7f3a started here"}}},
		{name: "a refusal, from the account its sessions moved to", doc: heldBackAt(status.EventRefused, false), id: "side", want: []toldLine{
			{kind: status.EventStarted, full: "c61b started here"},
			{kind: status.EventMoved, full: "3 sessions came from work: work refused (401)", short: "3 came from work: work refused (401)"}}},
		{name: "a cap moving sessions to several accounts, from the account it held back", doc: spread, id: "work", want: []toldLine{
			{kind: status.EventCap, full: "reached its cap: 2 sessions moved to other accounts", short: "at its cap: 2 moved to other accounts"}}},
		{name: "a cap moving sessions to several accounts, from one of them, its move alone", doc: spread, id: "side", want: []toldLine{
			{kind: status.EventMoved, full: "d28c came from work: work reached its cap"}}},
		{name: "a cap moving none, from the account it held back", doc: latelyOf(capped(0, 0)), id: "work", want: []toldLine{
			{kind: status.EventCap, full: "reached its cap", short: "at its cap"}}},
		{name: "a session rescored onto the account", doc: rescored, id: "work", want: []toldLine{
			{kind: status.EventMoved, full: "db8a came from side: rescored after 15h idle"}}},
		{name: "a session rescored off the account", doc: rescored, id: "side"},
		{name: "pressure, saying when the account runs out",
			doc: latelyOf(status.Event{Kind: status.EventPressure, Account: "personal", Windows: []string{"5h"}, Until: at(16, 5)}), id: "personal", want: []toldLine{
				{kind: status.EventPressure, full: "came under pressure: out ~16:05 at this pace", short: "under pressure: out ~16:05"}}},
		{name: "pressure, not saying when", doc: latelyOf(status.Event{Kind: status.EventPressure, Account: "personal", Windows: []string{"5h"}}), id: "personal", want: []toldLine{
			{kind: status.EventPressure, full: "came under pressure", short: "under pressure"}}},
		{name: "pressure on another account", doc: latelyOf(status.Event{Kind: status.EventPressure, Account: "personal"}), id: "side"},
		{name: "a session starting on the account new sessions go to", doc: started("side", status.ReasonNew), id: "side", best: "side", want: []toldLine{
			{kind: status.EventStarted, full: "c61b started here, the best"}}},
		{name: "a session starting on the account, new sessions going elsewhere", doc: started("side", status.ReasonNew), id: "side", best: "work", want: []toldLine{
			{kind: status.EventStarted, full: "c61b started here"}}},
		{name: "a session starting as pinned", doc: started("side", status.ReasonGlobalPin), id: "side", best: "side", want: []toldLine{
			{kind: status.EventStarted, full: "c61b started here, as pinned"}}},
		{name: "a session starting, passing over an account under pressure", doc: started("side", "new, personal under pressure"), id: "side", best: "side", want: []toldLine{
			{kind: status.EventStarted, full: "c61b started here: personal came under pressure"}}},
		{name: "a session starting on another account", doc: started("work", status.ReasonNew), id: "side", best: "work"},
		{name: "a prime", doc: latelyOf(status.Event{Kind: status.EventPrimed, Account: "side", Windows: []string{"5h"}}), id: "side", want: []toldLine{
			{kind: status.EventPrimed, full: "primed: its 5-hour window started", short: "primed"}}},
		{name: "room again", doc: latelyOf(status.Event{Kind: status.EventRoom, Account: "work", Windows: []string{"5h"}}), id: "work", want: []toldLine{
			{kind: status.EventRoom, full: "open again: its 5-hour window reset", short: "open again"}}},
		{name: "the global pin of two, from the first", doc: pinnedBoth, id: "personal", want: []toldLine{
			{kind: status.EventPin, full: "pinned with side, by hand", short: "pinned with side"}}},
		{name: "the global pin of two, from the second", doc: pinnedBoth, id: "side", want: []toldLine{
			{kind: status.EventPin, full: "pinned with personal, by hand", short: "pinned with personal"}}},
		{name: "the global pin of two, from neither", doc: pinnedBoth, id: "work"},
		{name: "the global pin of the account alone", doc: latelyOf(status.Event{Kind: status.EventPin, Account: "personal", Accounts: []string{"personal"}, Move: true}), id: "personal", want: []toldLine{
			{kind: status.EventPin, full: "pinned here, by hand", short: "pinned here"}}},
		{name: "a session's own pin", doc: pinnedOwn, id: "side", want: []toldLine{
			{kind: status.EventPin, full: "c61b pinned here, by hand", short: "c61b pinned here"}}},
		{name: "a session's own pin to another account", doc: pinnedOwn, id: "work"},
		{name: "routing given back, a restart and the router's health, from an account", doc: unnamed, id: "side"},
		{name: "every account's, a session starting on the best", doc: heldBackAt(status.EventCap, false), best: "side", want: []toldLine{
			{kind: status.EventStarted, full: "c61b started on side, the best"},
			{kind: status.EventCap, full: "work reached its cap: 3 sessions moved to side", short: "work at its cap: 3 moved to side"},
			{kind: status.EventStarted, full: "7f3a started on work"}}},
		{name: "every account's, a session starting as pinned", doc: started("side", status.ReasonPinned), best: "side", want: []toldLine{
			{kind: status.EventStarted, full: "c61b started on side, as pinned"}}},
		{name: "every account's, routing given back, a restart and the router's health", doc: unnamed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			want := slices.Clone(tt.want)
			for i := range want {
				want[i].short = cmp.Or(want[i].short, want[i].full)
			}
			if got := accountLately(tt.doc, tt.id, tt.best); !slices.Equal(got, want) {
				t.Errorf("%q's LATELY tells\n%q\nwant\n%q", tt.id, got, want)
			}
		})
	}
}

// Detail's LATELY of an account tells its last n events, the newest first,
// a line folding the moves it counts as Lately folds them.
func TestDetailsLATELYTellsTheAccountsLastNNewestFirst(t *testing.T) {
	doc := heldBackAt(status.EventCap, false)
	doc.Events = append(doc.Events, status.Event{ID: 22, Kind: status.EventPrimed, Account: "side", Windows: []string{"5h"}})
	var got []int
	for _, s := range views.AccountLately(doc.At(at(16, 0), claude.Policy), "side", 2) {
		got = append(got, s.ID)
	}
	if want := []int{28, heldLine}; !slices.Equal(got, want) {
		t.Errorf("side's LATELY of 2 tells the events %v, want %v", got, want)
	}
}
