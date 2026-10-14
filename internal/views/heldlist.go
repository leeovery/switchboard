package views

import (
	"maps"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// HeldList is Sessions' List as its lines build it, held between looks, so
// today's lines are read as they're appended, never afresh at each look, and
// a look, a stream event or a tick works out only what they change. It holds
// each session's requests it has read, or taken in since, each priced once at
// the day it's held at, and each one's story; and which sessions it has read
// whole, every line of them whatever day it's of, so their starts are found
// once. Take takes in the lines a day gained since; Found takes in a read of
// sessions whole; List builds the List of what's held, what the router lists
// and what the request stream told, as ListSessions builds it of the same
// lines. What changes it returns afresh, so a copy holding it never sees it
// change.
type HeldList struct {
	day    LedgerDay
	prices ledger.Table
	// held are the lines taken, by keyOf: none where they have no id.
	held     map[lineKey]bool
	whole    map[string]bool
	requests map[string][]request
	stories  map[string]*story
}

// ListRead is a read of the ledger for the List, as ReadList reads it: every
// line of each session it read, whatever day it's of, by its id, oldest
// first; and the ids of the sessions it read whole, those it read no line of
// among them.
type ListRead struct {
	lines map[string][]ledger.Line
	whole []string
}

// ListLive is what the List is built of beside its lines: the sessions the
// router lists, as GET /sessions gives them, none without the router;
// whether no router lists them, as SessionSources' Unlisted has it; what the
// request stream told; and now.
type ListLive struct {
	Running  []status.Session
	Unlisted bool
	Stream   Stream
	Now      time.Time
}

// ReadList reads, through l, the sessions a List lists whole, as the
// ledger's Sessions reads them: today's, those running gives, as the router
// lists them, and those with the given ids, each from its first line, on the
// first day the summaries' session ids name it.
func ReadList(l LedgerSessions, running []status.Session, ids ...string) ListRead {
	wanted := append(runningIDs(running), ids...)
	lines := l.Sessions(wanted, nil)
	return ListRead{lines: lines, whole: append(wanted, slices.Collect(maps.Keys(lines))...)}
}

// HoldList holds the List of what read holds, at now, its requests priced by
// prices at the day now falls on.
func HoldList(read ListRead, prices ledger.Table, now time.Time) HeldList {
	return HeldList{day: Today(now), prices: prices}.Found(read)
}

// Found is h with the sessions read whole taken in, each line once, whether
// it held it already or not: so their starts, and whatever else their
// earlier days tell, are found once, never at each look.
func (h HeldList) Found(read ListRead) HeldList {
	h = h.cloned()
	for _, id := range read.whole {
		h.whole[id] = true
	}
	for id, lines := range read.lines {
		h.take(id, lines)
	}
	return h
}

// Take is h with the lines given taken in, those a day, of the given date,
// gained since it was held, as the ledger's Today gives them: each request of
// a session's, priced once, each line once, whether it was held already or
// not. A session first met among them is held, but not as read whole, as
// Unfound has it. A date after the day h is held at rolls it on to that day,
// its requests priced afresh, as List would price them every look till it's
// held there, and those of the sessions whose last request ended an
// IdleHour or more before that day began let go of, as no List of that day
// lists them but as they ask again, which reads them whole once more; an
// earlier date, of a ledger yet to turn its day, never rolls it back. It's h
// itself where none is new and the date is no later than its day.
func (h HeldList) Take(date string, lines []ledger.Held) HeldList {
	gained := make(map[string][]ledger.Line)
	for _, l := range lines {
		if k, ok := keyOf(&l.Line); isRequest(&l.Line) && (!ok || !h.held[k]) {
			gained[l.Session] = append(gained[l.Session], l.Line)
		}
	}
	rolls := date > h.day.Date
	switch {
	case len(gained) == 0 && !rolls:
		return h
	case rolls:
		h = h.on(dayOf(date))
	default:
		h = h.cloned()
	}
	for id, lines := range gained {
		h.take(id, lines)
	}
	return h
}

// List is the List of what h holds and live gives, as ListSessions builds it
// of the same lines: the sessions the router lists, and those with requests
// today, at the day live's now falls on.
func (h HeldList) List(live ListLive) SessionList {
	h = h.at(live.Now)
	return listOf(h.stories, newLister(live.Running, live.Unlisted, live.Stream, live.Now), h.prices)
}

// Unfound are the ids of the sessions the List of what h holds and live
// gives lists that h hasn't read whole, in the order of their ids: those
// first met in the lines it took in, and those the router lists that it
// holds no read of, whose earlier days, their starts among them, a read of
// them whole, ReadList's, finds once, as Found takes it in.
func (h HeldList) Unfound(live ListLive) []string {
	h = h.at(live.Now)
	routed := make(map[string]status.Session, len(live.Running))
	for _, s := range live.Running {
		routed[s.ID] = s
	}
	var unfound []string
	for _, id := range listedIDs(h.stories, routed) {
		_, running := routed[id]
		if s := h.stories[id]; !h.whole[id] && (running || s != nil && s.requests > 0) {
			unfound = append(unfound, id)
		}
	}
	slices.Sort(unfound)
	return unfound
}

// Awaited are the dates of the days before the day live's now falls on that
// the lines of requests the stream told ended are filed on, as Told.FiledOn
// has it, their lines yet to come, of whichever session, each once, the
// oldest first: a later day's tail of the ledger never reads them, as a
// held page awaits its own.
func (h HeldList) Awaited(live ListLive) []string {
	today := Today(live.Now)
	var dates []string
	for id, told := range live.Stream.hearing().told {
		for _, s := range sentOf(h.requests[id], told) {
			if s.told == nil || toldRow(*s.told, live.Now).Stage != RowEnded {
				continue
			}
			if day, ok := s.told.FiledOn(); ok && day.Start.Before(today.Start) {
				dates = append(dates, day.Date)
			}
		}
	}
	return slices.Compact(slices.Sorted(slices.Values(dates)))
}

// Unread are the dates of those of the days missed, the days the ledger's
// tail followed no further, oldest first, that a read of it reads again
// whole for the List h holds: the day h is held at and those after it, as it
// read or took in every line filed on a day before it.
func (h HeldList) Unread(missed []string) []string {
	return slices.DeleteFunc(slices.Clone(missed), func(date string) bool { return date < h.day.Date })
}

// take holds the lines given of the session with the given id, those it
// holds already passed over, with its requests, each priced once, in the
// order they arrived, and tells its story afresh. It changes h's maps, which
// must be its own.
func (h HeldList) take(id string, lines []ledger.Line) {
	var gained []ledger.Line
	for i := range lines {
		k, ok := keyOf(&lines[i])
		if ok && h.held[k] {
			continue
		}
		if ok {
			h.held[k] = true
		}
		gained = append(gained, lines[i])
	}
	if added := requestsOf(gained, h.prices, h.day.Date); len(added) > 0 {
		h.requests[id] = inArrival(h.requests[id], added)
		h.stories[id] = storyOf(h.requests[id], h.prices, h.day)
	}
}

// at is h at now: as it's held, where now falls on the day it's held at,
// else rolled on to the day now falls on, as on has it.
func (h HeldList) at(now time.Time) HeldList {
	if day := Today(now); day.Date != h.day.Date {
		return h.on(day)
	}
	return h
}

// on is h held at the day given, its sessions' requests priced afresh at it,
// and their stories told afresh, but for those of the sessions done before
// it began, whose last request ended an IdleHour or more before then, let go
// of: a session no List of the day lists, but as it asks again, when it's
// read whole once more.
func (h HeldList) on(day LedgerDay) HeldList {
	next := HeldList{day: day, prices: h.prices}.cloned()
	for id, requests := range h.requests {
		if !ended(requests[len(requests)-1].Line).After(day.Start.Add(-IdleHour)) {
			continue
		}
		lines := make([]ledger.Line, len(requests))
		for i, r := range requests {
			lines[i] = *r.Line
		}
		next.whole[id] = h.whole[id]
		next.take(id, lines)
	}
	return next
}

// cloned is h with maps of its own, for what changes it.
func (h HeldList) cloned() HeldList {
	h.held, h.whole = cloneOrMake(h.held), cloneOrMake(h.whole)
	h.requests, h.stories = cloneOrMake(h.requests), cloneOrMake(h.stories)
	return h
}

// cloneOrMake is a copy of m, or, where it's nil, a map of its own.
func cloneOrMake[K comparable, V any](m map[K]V) map[K]V {
	if m == nil {
		return make(map[K]V)
	}
	return maps.Clone(m)
}

// storyOf is the story h holds of the session with the given id: one of no
// requests where it holds none.
func (h HeldList) storyOf(id string) *story {
	if s, ok := h.stories[id]; ok {
		return s
	}
	return newStory(h.prices, h.day)
}
