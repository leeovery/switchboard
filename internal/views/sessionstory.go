package views

import (
	"iter"
	"slices"
	"time"

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
	Sent      SentTimes
	Folds     []Fold
	Moves     []SessionMove
	Stretches []Stretch
}

// SentTimes are when a story's requests went upstream, oldest first: those
// its page sealed, as it holds them, and the rest, each oldest first, so a
// story told again copies none of those sealed.
type SentTimes struct {
	sealed upstreamTimes
	open   []time.Time
}

// SentAt are the times given, oldest first, as a story's.
func SentAt(times ...time.Time) SentTimes {
	return SentTimes{open: times}
}

// Len is how many times s holds.
func (s SentTimes) Len() int {
	return s.sealed.len() + len(s.open)
}

// All gives every time s holds, oldest first.
func (s SentTimes) All() iter.Seq[time.Time] {
	return func(yield func(time.Time) bool) {
		i, j, sealed := 0, 0, s.sealed.len()
		for i < sealed || j < len(s.open) {
			var t time.Time
			if i < sealed {
				t = s.sealed.at(i)
			}
			if i == sealed || j < len(s.open) && s.open[j].Before(t) {
				t, j = s.open[j], j+1
			} else {
				i++
			}
			if !yield(t) {
				return
			}
		}
	}
}

// Fold is an idle gap in a session's story, as idled has it: from its last
// request before the gap to its first after, as it came back.
type Fold struct {
	From, To time.Time
}

// Stretch is a time a session's main thread spent on an account: from its
// first request there, or from the session's start, or its coming back
// after a fold, as a session's stretch begins with it, until the next. Its
// stretches are its own conversation's requests', or every request's where
// none says it's of its own conversation, as of a Claude Code from before
// they did: one begins as the session starts and as it comes back after a
// fold, on the account its main thread goes to first then, else the one it
// was on, and another each time its main thread goes to an account other
// than the one it's on. One the router answered itself goes to none.
type Stretch struct {
	Account string
	From    time.Time
}

// Story is the story of the session h holds, of what h holds of its lines
// and what the stream told, as live gives it, its page page: its requests
// as its turns take them in, their gaps folded as idled finds them, its
// main thread's stretches as the naming rule follows it, and its named
// model's moves, those row 6 tells, with those the stream told of whose
// lines are yet to come.
func (h HeldPage) Story(page SessionPage, live PageLive) Story {
	return h.worked(live).story(page)
}

// story is the story Story tells, of w, its page page.
func (w worked) story(page SessionPage) Story {
	p, now := w.tally, w.live.Now
	if !p.begun {
		return Story{}
	}
	s := Story{
		Start: p.first, End: p.last.In(now.Location()), Ended: !page.Runs(), Sent: w.upstream(), Folds: p.folds, Moves: w.movesOf(page),
		Stretches: p.stretches(),
	}
	if !s.Ended {
		s.End = score.Later(s.End, now)
	}
	return s
}

// movesOf are the moves of the model page names the session w is of by, in
// the order they came: those its lines made, as its page has them; and
// those the stream told of, by none of its lines held, each as the router
// told it, what it wrote again yet to be known.
func (w worked) movesOf(page SessionPage) []SessionMove {
	moves := page.modelMoves()
	for _, m := range w.live.Stream.Moves {
		if m.Session != w.h.id || m.Model != page.Model || w.rec.linedAt(m.Request, m.At) {
			continue
		}
		moves = append(moves, SessionMove{At: m.At.UTC(), Model: m.Model, From: m.From, To: m.To, Reason: m.Reason})
	}
	slices.SortStableFunc(moves, func(a, b SessionMove) int { return a.At.Compare(b.At) })
	return moves
}

// upstream are when the session's requests, by their lines and as the
// stream told of them, went upstream, oldest first, as the traces count
// them: each by its line, as its page tallies them, those sealed as the
// store sealed them, the open taken in since; and each the stream told of
// whose line the session doesn't hold, once.
func (w worked) upstream() SentTimes {
	open := slices.Clone(w.tally.sentSince)
	told := make(map[string]bool)
	for _, t := range w.told {
		if _, ok := toldUpstream(t); !ok || told[t.Request] || w.rec.lined(t) {
			continue
		}
		told[t.Request] = true
		open = append(open, t.Sent)
	}
	slices.SortFunc(open, time.Time.Compare)
	return SentTimes{sealed: w.tally.upstream, open: open}
}
