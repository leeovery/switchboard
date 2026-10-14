package dashboard

import (
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// The sample sessions' ids, each a Claude Code session's.
const (
	idD28C = "d28c5e17-3a9b-4f60-8c21-7e4d9b0a6f13"
	idDB8A = "db8a71e3-9f04-4c6d-b2a8-5e1f7c39d860"
	id7F3A = "7f3a0c94-5d2e-4b18-a6f7-31c8e9d24b05"
	idC61B = "c61b2f8d-04a7-4e93-9b5c-d81e6a7f2c34"
	id41E0 = "41e0b6c2-8d3f-4a71-95e4-2c7b0f9a1d68"
)

// The sample sessions' models.
const (
	opus   = "claude-opus-5-5"
	sonnet = "claude-sonnet-5-5"
	haiku  = "claude-haiku-4-5"
)

// seatOf is a session's model on an account, as the router assigns it: put
// there made before now, for the reason given, and last seen seen before
// now.
func seatOf(model, account, reason string, made, seen time.Duration) status.Assignment {
	family := strings.Split(model, "-")[1]
	return status.Assignment{
		Model: model, Family: family, Account: account, Reason: reason,
		AssignedAt: now.Add(-made).UTC(), LastSeen: now.Add(-seen).UTC(),
	}
}

// flippingSessions are the sessions the router lists of threeRouted, the
// one seen last first: idDB8A, whose sonnet is on work and whose opus a
// limit moved to side; idD28C, on work since it started; idC61B, new on
// side; 41e0, which the limit moved to side, idle; and 7f3a, on work, idle.
func flippingSessions() []status.Session {
	return []status.Session{
		{ID: idDB8A, Assignments: []status.Assignment{
			seatOf(sonnet, "work", "new", 2*time.Hour, 3*time.Second),
			seatOf(opus, "side", "moved: personal has no room", 50*time.Minute, 4*time.Second),
		}},
		{ID: idC61B, Assignments: []status.Assignment{seatOf(opus, "side", "new", time.Minute, 5*time.Second)}},
		{ID: idD28C, Assignments: []status.Assignment{seatOf(opus, "work", "new", 2*time.Hour, 50*time.Second)}},
		{ID: id41E0, Assignments: []status.Assignment{seatOf(sonnet, "side", "moved: personal has no room", 50*time.Minute, 4*time.Minute)}},
		{ID: id7F3A, Assignments: []status.Assignment{seatOf(haiku, "work", "new", 3*time.Hour, 9*time.Minute)}},
	}
}

// rowOf is row y of the canvas as text, its trailing blanks trimmed.
func rowOf(c *canvas, y int) string {
	return strings.TrimRight(c.row(y, Look{}), " ")
}

// runwayThree is the router's document of three accounts, as Runway's frames
// have them: work's session running out at its pace at 14:42, back as it
// resets at 16:12; personal at its limit since 12:50, as the router told of
// it, until 14:12, its week running out at 01:12 on Tuesday, before it
// resets on Thursday; and side, where new sessions go, with room all along.
func runwayThree() status.Document {
	personal, reached := limitedAt("personal", clockAt(12, 50))
	side := readAccount("side", sessionOf(0.1, 4*time.Hour), weekOf(0.1, 5*quota.Day))
	return withEvents(routerDoc("side", 5, pressedAccount("work"), personal, side), reached)
}

// clockAt is a time on the day the frames are drawn.
func clockAt(hour, minute int) time.Time {
	return time.Date(now.Year(), now.Month(), now.Day(), hour, minute, 0, 0, now.Location())
}

// weekRunningOut is a week resetting in resets, used so that at the pace its
// use since it started sets, it runs out in out.
func weekRunningOut(out, resets time.Duration) quota.Window {
	passed := 7*quota.Day - resets
	return weekOf(passed.Hours()/(passed+out).Hours(), resets)
}

// limitedAt is an account whose session reached its limit at reached, as
// the router told of it, which holds until it resets in an hour, and whose
// week, resetting in three days, runs out in twelve hours.
func limitedAt(id string, reached time.Time) (status.Account, status.Event) {
	session := sessionOf(1, time.Hour)
	session.Status = quota.StatusRejected
	a := readAccount(id, session, weekRunningOut(12*time.Hour, 3*quota.Day))
	a.Limit = status.Limit{Windows: []string{"5h"}, Until: session.ResetsAt}
	return a, status.Event{ID: 1, At: reached.UTC(), Kind: status.EventLimit, Account: id, Windows: []string{"5h"}, Until: session.ResetsAt}
}

// withEvents is doc telling of events.
func withEvents(doc status.Document, events ...status.Event) status.Document {
	doc.Events = events
	return doc
}

// textOf is rows as text alone, each without the blanks at its end.
func textOf(rows []string) []string {
	text := make([]string, len(rows))
	for i, row := range rows {
		text[i] = strings.TrimRight(ansi.Strip(row), " ")
	}
	return text
}

// textsAt is a row of cells cells, blank but for the texts at their columns.
func textsAt(cells int, texts map[int]string) string {
	row := slices.Repeat([]string{" "}, cells)
	for col, text := range texts {
		for i, r := range []rune(text) {
			row[col+i] = string(r)
		}
	}
	return strings.TrimRight(strings.Join(row, ""), " ")
}
