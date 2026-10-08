package router

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/notify"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// maxMessage is the longest a limit's notification runs to saying when the
// account is back, which goes unsaid past it: roughly as much as a banner
// shows before it cuts a message short.
const maxMessage = 100

// standing is how an account stands, as notifications judge it: its status;
// whether its quota leaves it room for a request of any model, which is known
// once its usage has been read or a limit has barred it, and the keys of the
// windows that leave it none, where it has none; and whether its token was
// refused too lately for anything to go out on it.
type standing struct {
	status.Account
	quota, known, refused bool
	held                  []string
}

// windowLabel names the account's window with the given key by its label, or
// by its key when it hasn't been read.
func (s standing) windowLabel(key string) string {
	i := slices.IndexFunc(s.Windows, func(w quota.Window) bool { return w.Key == key })
	if i < 0 {
		return key
	}
	return s.Windows[i].Label
}

// standings are the accounts with a token, as they stand.
type standings []standing

// of returns how the account with the given id stands.
func (ss standings) of(id string) (standing, bool) {
	i := slices.IndexFunc(ss, func(s standing) bool { return s.ID == id })
	if i < 0 {
		return standing{}, false
	}
	return ss[i], true
}

// title names the account with the given id by its id and label, as a
// notification does, or by its id alone when it isn't among them.
func (ss standings) title(id string) string {
	if s, ok := ss.of(id); ok {
		return s.Title()
	}
	return id
}

// fullBut reports whether no account but the one with the given id has room
// for a request of any model, as far as anyone knows: one whose quota isn't
// known might have some, unless its token is refused.
func (ss standings) fullBut(id string) bool {
	return !slices.ContainsFunc(ss, func(s standing) bool {
		return s.ID != id && !s.refused && (s.quota || !s.known)
	})
}

// byID names an account by its id alone, as the log does.
func byID(id string) string {
	return id
}

// limitNotices gathers each limit an account reaches with the sessions the
// limit moves, for one notification of them all: one a limit, however many
// requests reach it, and whatever order the news of it and its moves comes
// in.
type limitNotices struct {
	// gathering holds, by identity, the limits whose notifications wait for
	// the sessions they move.
	gathering map[int]*gathering
	// heard holds, by account, the identity of the newest of its limits
	// heard of; early, by identity, the moves of limits yet to be heard of,
	// which wait for their limit to be counted in.
	heard map[string]int
	early map[int][]Moved
}

func newLimitNotices() *limitNotices {
	return &limitNotices{gathering: make(map[int]*gathering), heard: make(map[string]int), early: make(map[int][]Moved)}
}

// gathering is a limit an account reached, and the sessions it has moved so
// far.
type gathering struct {
	*limitMoves
	// due is when its notification goes out.
	due time.Time
}

// reached takes in a limit an account reached at now, by the limit's
// identity: one gathering joins it, one newer than every limit heard of the
// account starts gathering the sessions it moves for gatherFor, counting
// those it moved before it was heard of, and one heard of before, whose
// notification has gone, is no news.
func (l *limitNotices) reached(e LimitReached, now time.Time) {
	if g, ok := l.gathering[e.Limit]; ok {
		g.join(e)
		return
	}
	if e.Limit <= l.heard[e.Account] {
		return
	}
	l.heard[e.Account] = e.Limit
	g := &gathering{limitMoves: newLimitMoves(e), due: now.Add(gatherFor)}
	for _, m := range l.early[e.Limit] {
		g.add(m.Session, m.To)
	}
	delete(l.early, e.Limit)
	l.gathering[e.Limit] = g
}

// moved takes in a move, and reports whether it's news of a limit: one of
// the account it left, which held its request back there, moved the session.
// It counts where its limit gathers, or, where its limit is yet to be heard
// of, once it is. A limit whose notification has gone counts no more.
func (l *limitNotices) moved(e Moved) bool {
	switch g, ok := l.gathering[e.Limit]; {
	case e.Limit == 0:
		return false
	case ok:
		g.add(e.Session, e.To)
	case e.Limit > l.heard[e.From]:
		l.early[e.Limit] = append(l.early[e.Limit], e)
	default:
		return false
	}
	return true
}

// next reports when the first limit gathering is due, and false when none is.
func (l *limitNotices) next() (time.Time, bool) {
	var first time.Time
	found := false
	for _, g := range l.gathering {
		if !found || g.due.Before(first) {
			first, found = g.due, true
		}
	}
	return first, found
}

// due removes the limits gathering whose notifications are due at now, and
// returns them, the first due first.
func (l *limitNotices) due(now time.Time) []*gathering {
	return l.remove(func(g *gathering) bool { return !g.due.After(now) })
}

// all removes every limit gathering, and returns them, the first due first.
func (l *limitNotices) all() []*gathering {
	return l.remove(func(*gathering) bool { return true })
}

// remove removes the limits gathering that which picks, and returns them, the
// first due first.
func (l *limitNotices) remove(which func(g *gathering) bool) []*gathering {
	var removed []*gathering
	for id, g := range l.gathering {
		if which(g) {
			removed = append(removed, g)
			delete(l.gathering, id)
		}
	}
	slices.SortFunc(removed, func(a, b *gathering) int {
		return cmp.Or(a.due.Compare(b.due), cmp.Compare(a.Account, b.Account), cmp.Compare(a.Limit, b.Limit))
	})
	return removed
}

// notice is the limit's notification, as the accounts stand at now: the
// windows it was reached in; when the account is back, where that fits; and
// where the sessions it moved went, or else, when it's so, that no other
// account has room.
func (g *gathering) notice(accounts standings, now time.Time) notify.Notice {
	limited, _ := accounts.of(g.Account)
	text := limitText{moved: len(g.sessions), to: g.to, full: accounts.fullBut(g.Account)}
	for _, key := range g.Windows {
		text.windows = append(text.windows, limited.windowLabel(key))
	}
	if g.Until.After(now) {
		text.back = status.Clock(now, g.Until)
	}
	message := func() string { return accounts.title(g.Account) + " " + text.say(accounts.title) }
	if utf8.RuneCountInString(message()) > maxMessage {
		text.back = ""
	}
	return notify.Notice{Account: g.Account, News: text.say(byID), Message: message()}
}

// limitText is what a limit's notification says after naming the account.
type limitText struct {
	// windows are the labels of the windows the limit was reached in.
	windows []string
	// back is when the account has room again, such as "Mon 21:00", or "" to
	// leave it unsaid.
	back string
	// moved counts the sessions the limit moved, and to the accounts they
	// went to, by id.
	moved int
	to    []string
	// full is set when no other account has room.
	full bool
}

// say words the text, naming accounts as name does: "hit its Session limit,
// back at Mon 21:00 — 3 sessions moved to 1 · one".
func (t limitText) say(name func(id string) string) string {
	var b strings.Builder
	b.WriteString("hit its " + limitIn(t.windows))
	if t.back != "" {
		b.WriteString(", back at " + t.back)
	}
	switch {
	case t.moved > 0:
		to := make([]string, len(t.to))
		for i, id := range t.to {
			to[i] = name(id)
		}
		fmt.Fprintf(&b, " — %s moved to %s", status.SessionCount(t.moved), prose.List(to))
	case t.full:
		b.WriteString(" — no other account has room")
	}
	return b.String()
}

// moveNotice is the notification of a session's move, which is about the
// account the session left.
func moveNotice(e Moved, accounts standings) notify.Notice {
	say := func(name func(id string) string) string {
		return fmt.Sprintf("session %s moved from %s to %s (%s)", status.ShortID(e.Session), name(e.From), name(e.To), e.Reason)
	}
	return notify.Notice{Account: e.From, News: say(byID), Message: say(accounts.title)}
}

// limitIn names the limit reached in the windows with the given labels, such
// as "Session limit" or "Session and Week limits", or "limit" with none.
func limitIn(labels []string) string {
	switch len(labels) {
	case 0:
		return "limit"
	case 1:
		return labels[0] + " limit"
	default:
		return prose.List(labels) + " limits"
	}
}
