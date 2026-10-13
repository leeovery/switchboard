package dashboard

import (
	"cmp"
	"fmt"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// name is what the dashboard calls an account: its label, which is its id
// unless the config gives another, cleaned.
func name(a status.AccountNow) string {
	return labelled(a.Label, a.ID)
}

// labelled is what the dashboard calls the account with the given label
// and id: its label, else its id, cleaned.
func labelled(label, id string) string {
	return prose.Clean(cmp.Or(label, id))
}

// named is what the dashboard calls the account of m with the given id: its
// name, or, where m lacks it, its id, cleaned.
func named(m status.Moment, id string) string {
	if a, ok := m.Account(id); ok {
		return name(a)
	}
	return prose.Clean(id)
}

// names names the accounts of m with the given ids as a list, as in "work and
// side".
func names(m status.Moment, ids []string) string {
	listed := make([]string, len(ids))
	for i, id := range ids {
		listed[i] = named(m, id)
	}
	return prose.List(listed)
}

// place is the account's place in m, counting from 1 in the order they're
// configured, as the digit that pins it numbers it: 0 where m lacks it.
func place(m status.Moment, id string) int {
	a, _ := m.Account(id)
	return a.Place
}

// either lists the names as prose lists what's chosen between, each in k
// and the words between them in between: "work", "work or side", "work,
// side or personal".
func either(names []string, k, between ink) line {
	var l line
	for i, n := range names {
		switch {
		case i == 0:
		case i == len(names)-1:
			l = append(l, span{" or ", between})
		default:
			l = append(l, span{", ", between})
		}
		l = append(l, span{n, k})
	}
	return l
}

// brief shows t in now's time zone as briefly as a bar line has room for:
// its time of day, such as 15:54, on now's day; else its weekday alone, such
// as Fri.
func brief(now, t time.Time) string {
	if t = t.In(now.Location()); t.Format(time.DateOnly) == now.Format(time.DateOnly) {
		return t.Format("15:04")
	}
	return t.Format("Mon")
}

// day is a day's length.
const day = 24 * time.Hour

// spanOf names a window by its length, as in "5-hour window" or "7-day
// window", or by its key, cleaned, where that doesn't give its length.
func spanOf(key string) string {
	length, ok := quota.Length(key)
	switch {
	case !ok:
		return prose.Clean(key)
	case length%(24*time.Hour) == 0:
		return fmt.Sprintf("%d-day window", length/(24*time.Hour))
	default:
		return fmt.Sprintf("%d-hour window", length/time.Hour)
	}
}

// lately says the span a recent rate measured at at is over, from since, as
// an adjective: "last-30-min", "last-18-min", or across a gap in its
// readings, "last-2h".
func lately(since, at time.Time) string {
	return strings.ReplaceAll(status.Over(since, at), " ", "-")
}

// because follows what with why, cleaned, where there's a why.
func because(what, why string) string {
	if why = prose.Clean(why); why == "" {
		return what
	}
	return what + ": " + why
}
