package views_test

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

// sidedLedger is a ledger holding a day's head of each day, its side the
// one given, by its date.
type sidedLedger struct {
	ledger.Empty
	sides map[string]json.RawMessage
}

func (l sidedLedger) DaySide(date string, _ int) (json.RawMessage, bool) {
	side, ok := l.sides[date]
	return side, ok
}

// yearOfSides are the sides of each day of the year to now, as the views'
// Digests build them, of ten sessions a day across eight projects, each
// asking for three models on two accounts, twice the sessions the busiest
// year the frames draw holds.
func yearOfSides(b *testing.B, now time.Time) sidedLedger {
	b.Helper()
	l := sidedLedger{sides: make(map[string]json.RawMessage)}
	models := []string{opus, haiku, "claude-sonnet-5-5"}
	for day := now.AddDate(-1, 0, 0); day.Before(now); day = day.AddDate(0, 0, 1) {
		build := views.Digests{}.Day()
		for s := range 10 {
			id := fmt.Sprintf("%s-%02d", day.Format(time.DateOnly), s)
			for i, model := range models {
				build.Take(ledger.Line{
					At: day.Add(time.Duration(s*30+i) * time.Minute).UTC(), Request: fmt.Sprintf("%s-%d", id, i), Kind: ledger.KindMessage, Session: id,
					Dir: fmt.Sprintf("/code/project-%d", s%8), Model: model, Account: []string{"work", "side"}[i%2], Status: 200, Class: ledger.ClassMain,
					Usage: json.RawMessage(hourUsage),
				})
			}
		}
		built, err := build.Built()
		if err != nil {
			b.Fatal(err)
		}
		l.sides[dayfile.DateOf(day)] = built.Side
	}
	return l
}

// Reading a year of days' sides, as the watch reads them once.
func BenchmarkReadingAYearOfSessionDays(b *testing.B) {
	now := october(7, 13, 12)
	l := yearOfSides(b, now)
	for b.Loop() {
		views.ReadSessionDays(l, now.AddDate(-1, 0, 0), now, views.PoolRead{})
	}
}

// Projects' page over the last 12 months, its list and its first
// project's page, of a year of days held.
func BenchmarkProjectsOverTwelveMonths(b *testing.B) {
	now := october(7, 13, 12)
	days := views.ReadSessionDays(yearOfSides(b, now), now.AddDate(-1, 0, 0), now, views.PoolRead{})
	in := projectsIn(views.LengthTwelveMonths.Now(), now)
	for b.Loop() {
		page := days.Projects(in)
		page.Project(page.Rows[0].Path)
	}
}

// Sessions' log over the last 12 months, of a year of days held, as the
// watch builds it as the List shows.
func BenchmarkSessionLogOverTwelveMonths(b *testing.B) {
	now := october(7, 13, 12)
	days := views.ReadSessionDays(yearOfSides(b, now), now.AddDate(-1, 0, 0), now, views.PoolRead{})
	in := projectsIn(views.LengthTwelveMonths.Now(), now)
	for b.Loop() {
		days.SessionLog(in)
	}
}

// busyLists are two Lists of the year's sessions of today, as yearOfSides
// names them, running, the first asking in one and idle in the other, so
// each tells otherwise than the other of what it's doing.
func busyLists(now time.Time) [2]views.SessionList {
	var listed []views.ListedSession
	for s := range 10 {
		listed = append(listed, listedRunning(fmt.Sprintf("%s-%02d", now.Format(time.DateOnly), s), fmt.Sprintf("/code/project-%d", s%8), ""))
	}
	asking := relisted(listed, listed[0].Session, views.Activity{Doing: views.Asking, Since: now, LastSeen: now})
	return [2]views.SessionList{{Sessions: listed}, {Sessions: asking}}
}

// Projects' page over the last 12 months, and its first project's page,
// told what a session running is doing, as the watch tells them in place of
// building them again.
func BenchmarkProjectsLiveOverTwelveMonths(b *testing.B) {
	now := october(7, 13, 12)
	days := views.ReadSessionDays(yearOfSides(b, now), now.AddDate(-1, 0, 0), now, views.PoolRead{})
	lists := busyLists(now)
	page := days.Projects(projectsIn(views.LengthTwelveMonths.Now(), now, lists[0].Sessions...))
	project, _ := page.Project(page.Rows[0].Path)
	i := 0
	for b.Loop() {
		i++
		page, _ = page.Live(lists[i%2])
		project = page.ProjectLive(project)
	}
}

// Sessions' log over the last 12 months told what a session running is
// doing, as the watch tells it in place of building it again.
func BenchmarkSessionLogLiveOverTwelveMonths(b *testing.B) {
	now := october(7, 13, 12)
	days := views.ReadSessionDays(yearOfSides(b, now), now.AddDate(-1, 0, 0), now, views.PoolRead{})
	lists := busyLists(now)
	log := days.SessionLog(projectsIn(views.LengthTwelveMonths.Now(), now, lists[0].Sessions...))
	i := 0
	for b.Loop() {
		i++
		log, _ = log.Live(lists[i%2])
	}
}
