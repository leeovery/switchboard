package status

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
)

// idShown is how much of a session's id people are shown: enough to tell the
// sessions of a day apart at a glance.
const idShown = 8

// The reasons the router gives for where a session's requests go, as an
// Assignment's Reason and an Event's give them, that the dashboard reads
// too.
const (
	// ReasonNew is a new session's first account.
	ReasonNew = "new"
	// ReasonPinned is the session's own pin.
	ReasonPinned = "pinned"
	// ReasonGlobalPin is one of the global pin's accounts, chosen afresh.
	ReasonGlobalPin = "pinned (global)"
	// ReasonMovedByPin is a move by a global pin that moves running sessions.
	ReasonMovedByPin = "moved by pin"
	// ReasonMovedOff starts the reason for a move off the session's account,
	// as it can't take the request, which account and why following, as in
	// "moved: work hit its limit".
	ReasonMovedOff = "moved: "
	// ReasonPinYields starts the reason for going elsewhere than the
	// session's own pin sends it, as its account can't take the request,
	// which account and why following, as in "pin yields: side has no room".
	ReasonPinYields = "pin yields: "
)

// What a request of an assignment's model in flight is doing, as an
// Assignment's InFlight gives it.
const (
	// Asking is a request waiting for its answer's first byte.
	Asking = "asking"
	// Answering is a request whose answer streams to it.
	Answering = "answering"
)

// Session is what the router says of a Claude Code session, as GET
// /sessions/{id} answers: its own pin, and the account each of its models'
// requests go to.
type Session struct {
	ID string `json:"session"`
	// Pin is the session's own pin, "" when it has none: the one pin
	// --session gave it while it ran, else the one run --account gave it as
	// it started, as its requests last carried it.
	Pin string `json:"pin,omitempty"`
	// Assignments are the session's, a model each, the one used last first.
	Assignments []Assignment `json:"assignments"`
	// Account is the status of the account the session's last-used model went
	// to: zero where the router lists sessions, rather than answering for one.
	Account Account `json:"account,omitzero"`
}

// Assignment is the account a session's requests of one model go to, and why.
type Assignment struct {
	Model string `json:"model"`
	// Family is the model's family, such as opus, as the router counts its
	// requests against windows.
	Family  string `json:"family,omitempty"`
	Account string `json:"account"`
	// Dir is the directory the session's last request of the model to name
	// one was started in, as the request ledger gives it: "" while none has.
	Dir string `json:"dir,omitempty"`
	// InFlight is what a request of the model in flight is doing: Answering
	// while an answer streams to one, else Asking while one waits for its
	// answer's first byte, and "" while none is in flight.
	InFlight string `json:"in_flight,omitempty"`
	// Pinned is set when the session's own pin put it on the account.
	Pinned bool `json:"pinned"`
	// Yielded is set while the session's own pin has yielded, the account it
	// names having had no room for a request of it, and it stays where it
	// went: while its cache there is warm, or its model's thinking is bound
	// to the account, and it's given no pin since.
	Yielded bool `json:"yielded,omitempty"`
	// PinnedAt is when the session was given its own pin while it ran: zero
	// for the one it was launched with, and while it has none.
	PinnedAt   time.Time `json:"pinned_at,omitzero"`
	Reason     string    `json:"reason"`
	AssignedAt time.Time `json:"assigned_at"`
	LastSeen   time.Time `json:"last_seen"`
}

// ShortID cuts a session's id to as much as people are shown of it, such as
// 0b5c6f2e.
func ShortID(id string) string {
	return prose.Truncate(id, idShown)
}

// Line says where the session's requests go, as status lists the router's
// sessions: its id cut short, the account each of its models goes to, its own
// pin, if it has one, and when it was last seen, before now, such as
// "0b5c6f2e  opus and haiku on work  ·  pinned to work  ·  seen 2m ago". What
// came from elsewhere shows cleaned.
func (s Session) Line(now time.Time) string {
	parts := []string{ShortID(Clean(s.ID)) + "  " + s.where()}
	if s.Pin != "" {
		parts = append(parts, "pinned to "+Clean(s.Pin))
	}
	if len(s.Assignments) > 0 {
		parts = append(parts, "seen "+ago(now, s.Assignments[0].LastSeen))
	}
	return strings.Join(parts, Separator)
}

// where says which account each of the session's models goes to, those on
// one account together, the account used last first: "opus and haiku on
// work, sonnet on side".
func (s Session) where() string {
	byAccount := s.ByAccount()
	on := make([]string, len(byAccount))
	for i, models := range byAccount {
		on[i] = prose.List(models.Names) + " on " + Clean(models.Account)
	}
	return strings.Join(on, ", ")
}

// Models are the models of a session whose requests go to one account: the
// account, by its id, and the models, each named as Name names it, once.
type Models struct {
	Account string
	Names   []string
}

// ByAccount groups the session's models by the account their requests go
// to, the account used last first, and of each, the model used last first.
func (s Session) ByAccount() []Models {
	var grouped []Models
	for _, a := range s.Assignments {
		i := slices.IndexFunc(grouped, func(m Models) bool { return m.Account == a.Account })
		if i < 0 {
			grouped = append(grouped, Models{Account: a.Account})
			i = len(grouped) - 1
		}
		if name := a.Name(); !slices.Contains(grouped[i].Names, name) {
			grouped[i].Names = append(grouped[i].Names, name)
		}
	}
	return grouped
}

// Name names the assignment's model as briefly as tells it apart, cleaned: by
// its family, such as opus, else by its id.
func (a Assignment) Name() string {
	return cmp.Or(Clean(a.Family), Clean(a.Model), "unknown model")
}

// ago says how long before now t was, such as "2m ago", or "just now" within
// the minute.
func ago(now, t time.Time) string {
	if now.Sub(t) < time.Minute {
		return "just now"
	}
	return Countdown(t, now) + " ago"
}
