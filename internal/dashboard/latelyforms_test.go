package dashboard

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// latelyForm is the form a LATELY line tells its sentence in, the fullest
// first: in full, short, or short and cut.
type latelyForm int

const (
	inFull latelyForm = iota
	inShort
	inShortCut
)

// formsOf are the forms latelyTold tells m's sentences in, in cells cells.
func formsOf(m status.Moment, sentences []views.Sentence, cells int) []latelyForm {
	names, leads := overviewNames(m), latelyLeads(m, sentences)
	if len(leads) > 0 {
		leads[0][0].ink = secondaryInk
	}
	forms := make([]latelyForm, len(sentences))
	for i, told := range latelyTold(m, sentences, cells) {
		switch s := sentences[i]; {
		case slices.Equal(told, slices.Concat(leads[i], saidLine(m, names, s.Full))):
			forms[i] = inFull
		case slices.Equal(told, slices.Concat(leads[i], saidLine(m, names, s.Short))):
			forms[i] = inShort
		default:
			forms[i] = inShortCut
		}
	}
	return forms
}

// latelyEvents are events of every kind whose short form says less than its
// full, and a start, of the accounts given, the newest a minute before now.
func latelyEvents(ids []string) []status.Event {
	first, last := ids[0], ids[len(ids)-1]
	events := []status.Event{
		{Kind: status.EventCap, Account: first, Count: 3, To: last},
		{Kind: status.EventPrimed, Account: last, Windows: []string{"5h"}},
		{Kind: status.EventLimit, Account: last, Windows: []string{"5h"}},
		{Kind: status.EventPressure, Account: first, Windows: []string{"5h"}, Until: now.Add(time.Hour).UTC()},
		{Kind: status.EventPin, Account: last, Accounts: []string{last}},
		{Kind: status.EventRoom, Account: first, Windows: []string{"7d"}},
		{Kind: status.EventStarted, Account: last, Session: "c61b2f8d", Reason: status.ReasonNew},
	}
	for i := range events {
		events[i].ID = len(events) - i
		events[i].At = now.Add(-time.Duration(i+1) * time.Minute).UTC()
	}
	return events
}

func TestLATELYTellsEachSentenceInTheFullestOfItsFormsThatFitsWhole(t *testing.T) {
	doc := threeRouted()
	doc.Events = latelyEvents([]string{"work", "personal", "side"})
	m := doc.At(now, claudeLike)
	sentences := views.Lately(m, mostLines)
	names, leads := overviewNames(m), latelyLeads(m, sentences)
	for cells := range 100 {
		for i, form := range formsOf(m, sentences, cells) {
			s, lead := sentences[i], leads[i].width()
			full, short := lead+saidLine(m, names, s.Full).width(), lead+saidLine(m, names, s.Short).width()
			want := inShortCut
			switch {
			case full <= cells:
				want = inFull
			case short <= cells:
				want = inShort
			}
			if form != want {
				t.Errorf("in %d cells, %q (%d cells, short %d) is told in form %d, want %d: the fullest that fits whole, its names and figures never cut while a shorter form fits", cells, s.Full, full, short, form, want)
			}
		}
	}
}

func TestLATELYFoldedSaysItsFirstSentenceInTheFullestFormItsHeadingHoldsWhole(t *testing.T) {
	capped := status.Event{At: now.Add(-time.Minute).UTC(), Kind: status.EventCap, Account: "work", Count: 3, To: "side"}
	room := status.Event{At: now.Add(-2 * time.Minute).UTC(), Kind: status.EventRoom, Account: "side"}
	full, short := "13:11  ╎ work reached its cap: 3 sessions moved to side", "13:11  ╎ work at its cap: 3 moved to side"
	more := "  " + prose.Ellipsis
	width := func(s string) int { return line{{s, dimInk}}.width() }
	tests := []struct {
		name   string
		events []status.Event
		cells  int
		want   string
	}{
		{name: "alone, its full form whole", events: []status.Event{capped}, cells: width(full), want: full},
		{name: "alone, a cell short of its full form", events: []status.Event{capped}, cells: width(full) - 1, want: short},
		{name: "alone, its short form whole", events: []status.Event{capped}, cells: width(short), want: short},
		{name: "with more, its full form and the ellipsis whole", events: []status.Event{capped, room}, cells: width(full + more), want: full + more},
		{name: "with more, a cell short of its full form and the ellipsis", events: []status.Event{capped, room}, cells: width(full+more) - 1, want: short + more},
		{name: "with more, its short form and the ellipsis whole", events: []status.Event{capped, room}, cells: width(short + more), want: short + more},
	}
	f := frameOf(160, 40)
	for _, tt := range tests {
		p := planned{section: f.latelySection(eventsOf(tt.events...), now), fold: true}
		if got := f.says(p, tt.cells).plain(); got != tt.want {
			t.Errorf("%s: folded in %d cells, LATELY's heading says %q, want %q", tt.name, tt.cells, got, tt.want)
		}
	}
}

// latelyWorld is a world of n accounts, of the ids accountIDs gives, each
// labelled with its id, or where long, a longer name of it, with
// latelyEvents' events of the first and last of them: one at its cap, a
// model window listed, and sessions started and moved.
func latelyWorld(n int, long bool) status.Document {
	ids := accountIDs[:n]
	accounts := make([]status.Account, n)
	for i, id := range ids {
		accounts[i] = readAccount(id, sessionOf(0.3, 3*time.Hour), weekOf(0.4, 4*quota.Day), quota.Window{Key: "7d_opus", Label: "Opus week", Utilization: 0.5, ResetsAt: now.Add(48 * time.Hour).UTC()})
		if long {
			accounts[i].Label = id + "-consulting-team"
		}
	}
	accounts[0] = readAccount(ids[0], sessionOf(0.95, 2*time.Hour), weekOf(0.4, 4*quota.Day))
	accounts[0].Reserve = 0.05
	doc := routerDoc(ids[n-1], 4, accounts...)
	doc.Events = latelyEvents(ids)
	return doc
}

// accountIDs are the ids of up to ten accounts.
var accountIDs = []string{"work", "personal", "side", "client", "spare", "lab", "team", "extra", "ops", "studio"}

// latelyRoom is the cells LATELY's sentences are told in on the frame, of
// doc at now, its body's width; whether it sits beside COMING UP; and
// whether it shows unfolded at all.
func latelyRoom(f Frame, doc status.Document) (cells int, beside, shown bool) {
	for _, p := range f.laidOut(doc, now, 0) {
		if p.Section != SectionLately || p.fold {
			continue
		}
		return p.body.width, p.body.x > 0, true
	}
	return 0, false, false
}

func TestAWiderFrameNeverGivesLATELYAShorterForm(t *testing.T) {
	for n := 1; n <= len(accountIDs); n++ {
		for _, long := range []bool{false, true} {
			t.Run(fmt.Sprintf("%d accounts, long names %t", n, long), func(t *testing.T) {
				t.Parallel()
				doc := latelyWorld(n, long)
				m := doc.At(now, claudeLike)
				sentences := views.Lately(m, mostLines)
				var was []latelyForm
				wasBeside := false
				for width := 40; width <= 220; width++ {
					cells, beside, shown := latelyRoom(frameOf(width, 120), doc)
					if !shown {
						continue
					}
					forms := formsOf(m, sentences, cells)
					// Where LATELY moves beside COMING UP its room halves, so a
					// sentence wider than the half takes a shorter form there:
					// the one width where it may, as listsBeside switches once.
					if was != nil && beside == wasBeside {
						for i := range forms {
							if forms[i] > was[i] {
								t.Errorf("at %d columns, %q is told in form %d, want %d at the most, as a column narrower", width, sentences[i].Full, forms[i], was[i])
							}
						}
					}
					was, wasBeside = forms, beside
				}
			})
		}
	}
}
