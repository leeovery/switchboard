package status

import (
	"cmp"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
)

const (
	// idShort is how much of a session's id logs, status's text and the
	// verbs show: enough to tell the sessions of a day apart at a glance.
	idShort = 8
	// IDShown is how much of a session's id the dashboard names it by: its
	// first 4 characters.
	IDShown = 4
)

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
	// ReasonSticky is the session's account, kept while its cache there is
	// warm.
	ReasonSticky = "sticky"
	// ReasonBound is the session's account, kept once its cache has gone
	// cold, as its model's thinking is bound to it.
	ReasonBound = "bound"
	// ReasonRescored starts the reason for choosing afresh the account of a
	// session that idled, as Rescored gives it.
	ReasonRescored = "rescored after "
	// ReasonBack is a session going back to the account it was on before a
	// request every account it went out on refused.
	ReasonBack = "back where it was before its request"
)

// rescoredIdle ends the reason Rescored gives.
const rescoredIdle = " idle"

// Rescored is the reason for choosing afresh the account of a session that
// idled for as long as idle, as in "rescored after 15h idle", the span as
// idled says it.
func Rescored(idle time.Duration) string {
	return ReasonRescored + idled(idle) + rescoredIdle
}

// idled says how long a session idled, as the reason Rescored gives files it:
// in whole units, "5d 12h" from a day, "4h 57m" from an hour, else "7m", a
// second unit of zero left off, as in "6d" and "4h"; "now" for no time at
// all. The ledger and the router's events keep the reason, which its words
// read back, so the form is the reason's own, held apart from every form a
// span is shown in: changed, it would file reasons that read apart from those
// filed before.
func idled(d time.Duration) string {
	hours, minutes := int(d/time.Hour), int(d%time.Hour/time.Minute)
	switch {
	case d <= 0:
		return "now"
	case hours >= 24:
		return idledIn(hours/24, "d", hours%24, "h")
	case hours >= 1:
		return idledIn(hours, "h", minutes, "m")
	}
	return strconv.Itoa(minutes) + "m"
}

// idledIn says a count of a unit, then what's left over of the next unit
// down, unless nothing is: "5d 12h", but "6d".
func idledIn(count int, unit string, rest int, restUnit string) string {
	said := strconv.Itoa(count) + unit
	if rest == 0 {
		return said
	}
	return said + " " + strconv.Itoa(rest) + restUnit
}

// Why an account can't take a request, as the reasons for leaving it, after
// ReasonMovedOff or ReasonPinYields and the account, give it.
const (
	// WhyReserve is an account held back by its reserve alone.
	WhyReserve = "is at its reserve"
	// WhyLimit is an account that answered the request with a limit reached.
	WhyLimit = "hit its limit"
	// WhyRefused is an account that refused the request.
	WhyRefused = "was refused"
	// WhyNoRoom is an account with no room for the request otherwise.
	WhyNoRoom = "has no room"
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

// ShortID cuts a session's id to as much as logs, status's text and the verbs
// show of it, such as 0b5c6f2e.
func ShortID(id string) string {
	return prose.Truncate(id, idShort)
}

// ShownID is a session's id as the dashboard names a session by it, cleaned:
// its first IDShown characters, such as d28c.
func ShownID(id string) string {
	return prose.Truncate(prose.Clean(id), IDShown)
}

// DirName names the directory a session was started in, cleaned, as the
// dashboard names a session by it: its last part, cut with an ellipsis to the
// cells given, keeping its start, as "switchboard" or "switchboa…"; "" for
// none.
func DirName(dir string, cells int) string {
	if dir = prose.Clean(dir); dir == "" {
		return ""
	}
	return prose.Cut(path.Base(dir), cells)
}

// DirPath shows the directory a session was started in, cleaned, as a
// session's page heads it: whole, the home directory given as ~, as
// "~/Code/switchboard", and where it's longer than the cells given, cut from
// its start with an ellipsis, keeping its name; "" for none.
func DirPath(dir, home string, cells int) string {
	dir, home = prose.Clean(dir), strings.TrimSuffix(prose.Clean(home), "/")
	if rest, ok := strings.CutPrefix(dir, home); home != "" && ok && (rest == "" || rest[0] == '/') {
		dir = "~" + rest
	}
	return prose.CutFront(dir, cells)
}

// Line says where the session's requests go, as status lists the router's
// sessions: its id cut short, the account each of its models goes to, its own
// pin, if it has one, and when it was last seen, before now, such as
// "0b5c6f2e  opus and haiku on work  ·  pinned to work  ·  seen 2m ago". What
// came from elsewhere shows cleaned.
func (s Session) Line(now time.Time) string {
	parts := []string{ShortID(prose.Clean(s.ID)) + "  " + s.where()}
	if s.Pin != "" {
		parts = append(parts, "pinned to "+prose.Clean(s.Pin))
	}
	if len(s.Assignments) > 0 {
		parts = append(parts, "seen "+ago(now, s.Assignments[0].LastSeen))
	}
	return strings.Join(parts, prose.Separator)
}

// where says which account each of the session's models goes to, those on
// one account together, the account used last first: "opus and haiku on
// work, sonnet on side".
func (s Session) where() string {
	byAccount := s.ByAccount()
	on := make([]string, len(byAccount))
	for i, models := range byAccount {
		on[i] = prose.List(models.Names) + " on " + prose.Clean(models.Account)
	}
	return strings.Join(on, ", ")
}

// Models are the models of a session whose requests go to one account: the
// account, by its id, and the models, each named as Name names it, once.
type Models struct {
	Account string
	Names   []string
}

// AccountID is the id of the account the session's last-used model went to,
// as the router answered for the session: "" where it lists sessions, rather
// than answering for one.
func (s Session) AccountID() string {
	return s.Account.ID
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
	return cmp.Or(prose.Clean(a.Family), prose.Clean(a.Model), "unknown model")
}

// ago says how long before now t was, such as "2m ago", or "just now" within
// the minute.
func ago(now, t time.Time) string {
	if now.Sub(t) < time.Minute {
		return "just now"
	}
	return prose.Countdown(t, now) + " ago"
}
