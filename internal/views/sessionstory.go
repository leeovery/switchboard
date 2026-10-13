package views

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/score"
)

// Story is a session's life as its page's STORY tells it, at any width:
// from its first request to now while it runs, else to its last, its end in
// the time zone its times are said in; when each of its requests that went
// upstream was sent, oldest first, as the traces count them; its idle gaps,
// each folded; the moves of the model naming it, as row 6 tells them; and
// its main thread's stretches on its accounts. Columns lays it across a
// width.
type Story struct {
	Start, End time.Time
	// Ended is set once it's over, as its page's Runs says.
	Ended     bool
	Sent      []time.Time
	Folds     []Fold
	Moves     []SessionMove
	Stretches []Stretch
}

// Fold is an idle gap in a session's story, as idled has it: from its last
// request before the gap to its first after, as it came back.
type Fold struct {
	From, To time.Time
}

// Stretch is a time a session's main thread spent on an account: from its
// first request there, or from the session's start, or its coming back
// after a fold, as a session's stretch begins with it, until the next.
type Stretch struct {
	Account string
	From    time.Time
}

// Story is the story of the session h holds, of what h holds of its lines
// and what the stream told, as live gives it, its page page: its requests
// as its turns take them in, their gaps folded as resumedAt finds them, its
// main thread's stretches as the naming rule follows it, and its named
// model's moves, those row 6 tells, with those the stream told of whose
// lines are yet to come.
func (h HeldPage) Story(page SessionPage, live PageLive) Story {
	told := live.Stream.hearing().told[h.id]
	all := sentOf(h.requests, told)
	if len(all) == 0 {
		return Story{}
	}
	s := Story{Start: all[0].at, End: all[len(all)-1].at.In(live.Now.Location()), Ended: !page.Runs(),
		Sent: upstreamOf(h.requests, told), Folds: foldsOf(all), Moves: h.movesOf(page, live.Stream), Stretches: stretchesOf(all)}
	if !s.Ended {
		s.End = score.Later(s.End, live.Now)
	}
	return s
}

// movesOf are the moves of the model page names the session h holds by, in
// the order they came: those its lines made, as its page has them; and
// those the stream s told of, by none of its lines held, each as the router
// told it, what it wrote again yet to be known.
func (h HeldPage) movesOf(page SessionPage, s Stream) []SessionMove {
	moves := page.modelMoves()
	for _, m := range s.Moves {
		if m.Session != h.id || m.Model != page.Model || slices.ContainsFunc(h.requests, func(r request) bool { return sameRequest(r.Line, m.Request, m.At) }) {
			continue
		}
		moves = append(moves, SessionMove{At: m.At.UTC(), Model: m.Model, From: m.From, To: m.To, Reason: m.Reason})
	}
	slices.SortStableFunc(moves, func(a, b SessionMove) int { return a.At.Compare(b.At) })
	return moves
}

// upstreamOf are when the requests of a session, by their lines and as the
// stream told of them, went upstream, oldest first, each once, as the traces
// count them.
func upstreamOf(requests []request, told []Told) []time.Time {
	var sent []upstream
	for _, r := range requests {
		if u, ok := lineUpstream(r.Line); ok {
			sent = append(sent, u)
		}
	}
	for _, t := range told {
		if u, ok := toldUpstream(t); ok {
			sent = append(sent, u)
		}
	}
	var at []time.Time
	for _, held := range counting(nil, sent, time.Time{}) {
		for _, c := range held {
			at = append(at, c.at)
		}
	}
	slices.SortFunc(at, time.Time.Compare)
	return at
}

// foldsOf are the idle gaps between requests, a session's, oldest first, as
// idled finds them.
func foldsOf(requests []sent) []Fold {
	var folds []Fold
	for i := 1; i < len(requests); i++ {
		if idled(requests[i-1].at, requests[i].at) {
			folds = append(folds, Fold{From: requests[i-1].at, To: requests[i].at})
		}
	}
	return folds
}

// stretchesOf are the stretches of requests, a session's, oldest first, on
// the accounts its main thread went to, as the naming rule follows it: its
// own conversation's requests, or every request where none says it's of its
// own conversation, as of a Claude Code from before they did. One begins as
// the session starts and as it comes back after a fold, on the account its
// main thread goes to first then, else the one it was on, and another each
// time its main thread goes to an account other than the one it's on. One
// the router answered itself goes to none.
func stretchesOf(requests []sent) []Stretch {
	byMain := slices.ContainsFunc(requests, func(r sent) bool { return r.class == ledger.ClassMain })
	var stretches []Stretch
	open := false
	for i, r := range requests {
		if i == 0 || idled(requests[i-1].at, r.at) {
			var on string
			if i > 0 {
				on = stretches[len(stretches)-1].Account
			}
			stretches, open = append(stretches, Stretch{Account: on, From: r.at}), true
		}
		if r.account == "" || byMain && r.class != ledger.ClassMain {
			continue
		}
		s := &stretches[len(stretches)-1]
		switch {
		case open:
			s.Account, open = r.account, false
		case s.Account != r.account:
			stretches = append(stretches, Stretch{Account: r.account, From: r.at})
		}
	}
	return slices.DeleteFunc(stretches, func(s Stretch) bool { return s.Account == "" })
}
