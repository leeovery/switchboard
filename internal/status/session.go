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
	// Pinned is set when the session's own pin put it on the account.
	Pinned     bool      `json:"pinned"`
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
	var accounts []string
	models := make(map[string][]string)
	for _, a := range s.Assignments {
		if _, ok := models[a.Account]; !ok {
			accounts = append(accounts, a.Account)
		}
		if name := a.Name(); !slices.Contains(models[a.Account], name) {
			models[a.Account] = append(models[a.Account], name)
		}
	}
	on := make([]string, len(accounts))
	for i, account := range accounts {
		on[i] = prose.List(models[account]) + " on " + Clean(account)
	}
	return strings.Join(on, ", ")
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
