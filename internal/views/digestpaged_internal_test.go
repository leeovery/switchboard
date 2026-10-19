package views

import (
	"encoding/json"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/ledger"
)

// daily are requests of a session s, one at noon on each of the days from
// 7 October, of the models given in turn, each on work, its answer reading
// the five-hour window and the week every model shares, and Claude Sonnet's
// week, as the answers of either model of Claude Sonnet's and Claude Opus's
// may, climbing a little each day.
func daily(models ...string) []ledger.Line {
	var lines []ledger.Line
	for day, model := range models {
		at := time.Date(2026, 10, 7+day, 12, 0, 0, 0, time.Local)
		use := strconv.FormatFloat(0.1*float64(day+1), 'f', 2, 64)
		resets := strconv.FormatInt(at.Add(4*time.Hour).Unix(), 10)
		lines = append(lines, ledger.Line{
			At: at.UTC(), Request: fmt.Sprintf("r%d", day), Kind: ledger.KindMessage, Session: "s", Model: model, Account: "work", Reason: "sticky",
			Status: 200, Attempts: 1, TotalMS: 3000, Class: ledger.ClassMain, Prompt: "p", Usage: json.RawMessage(foldUsages[2]),
			Limits: map[string]string{
				"5h-utilization": use, "5h-reset": resets, "7d-utilization": use, "7d-reset": resets, "7d_sonnet-utilization": use, "7d_sonnet-reset": resets,
			},
		})
	}
	return lines
}

// dateOn is the date of the day the given number of days after 7 October.
func dateOn(days int) string {
	return dayfile.DateOf(time.Date(2026, 10, 7+days, 12, 0, 0, 0, time.Local))
}

// A session whose page's points can't take a day in on the state of the day
// before, as a model first reads a window only some models share, which
// would count otherwise the requests they sealed, isn't folded on it: it's
// folded again whole, its part whole; and the day after, it rests on that
// state, its part folded on it. Its page restored from each day's chain is
// the page folded whole, and a reader resting it on its page rests it on
// the chain from that whole part.
func TestAPageItsPointsCantTakeADayInOnIsFoldedWholeThenRestedOn(t *testing.T) {
	c := newDigestChain(t, "a model first reading a window only some models share", 0, daily(foldSonnet, foldOpus, foldOpus))
	c.digestDayByDay(func(LedgerDay) {})
	if refused, again := c.digests.refused[dateOn(1)]["s"]; !again || refused.base != dateOn(0) || c.digests.bases[dateOn(1)]["s"] != nil {
		t.Errorf("s on %s: given no state resting on %q, %t, then folded on %+v; want it given none resting on %s, then folded whole",
			dateOn(1), refused.base, again, c.digests.bases[dateOn(1)]["s"], dateOn(0))
	}
	if base := c.digests.bases[dateOn(2)]["s"]; base == nil || base.Date != dateOn(1) {
		t.Errorf("s on %s: folded on %+v, want on its whole state of %s", dateOn(2), base, dateOn(1))
	}
	c.checkChained()
	if paged := c.checkPaged(); paged != 3 {
		t.Errorf("%d pages restored from their chains, want one of each day", paged)
	}
	c.checkRested(c.at, ledger.RestOnPage, dateOn(1), dateOn(2))
}

// A state left out mid-chain, as a session the Digests gave no state of on
// a day, or one whose page JSON can't carry, its state kept without it and
// with no part, has the day after's state folded on the state before it,
// its part on that one: the next day's page rests on the chain past it. A
// reader resting the session on its state alone rests it on the state kept
// without its page; one resting it on its page, on the state before it. Its
// page restored from each day's chain is the page folded whole.
func TestAStateLeftOutMidChainHasTheNextFoldedOnTheOneBefore(t *testing.T) {
	tests := []struct {
		name  string
		alter func(folded map[string]ledger.Folded)
		state string
	}{
		{name: "no state given", alter: func(folded map[string]ledger.Folded) { delete(folded, "s") }, state: dateOn(0)},
		{name: "a page JSON can't carry", alter: func(folded map[string]ledger.Folded) { folded["s"] = pageless(t, folded["s"]) }, state: dateOn(1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := newDigestChain(t, tt.name, 0, daily(foldSonnet, foldSonnet, foldSonnet, foldSonnet))
			c.digests.alter = func(date string, folded map[string]ledger.Folded) {
				if date == dateOn(1) {
					tt.alter(folded)
				}
			}
			c.digestDayByDay(func(day LedgerDay) {
				if day.Date == dateOn(2) {
					now := day.Start.Add(12 * time.Hour)
					c.checkRested(now, ledger.RestOnState, tt.state)
					c.checkRested(now, ledger.RestOnPage, dateOn(0))
				}
			})
			if base := c.digests.bases[dateOn(2)]["s"]; base == nil || base.Date != dateOn(0) {
				t.Errorf("s on %s: folded on %+v, want on its state of %s", dateOn(2), base, dateOn(0))
			}
			if paged := c.checkPaged(); paged != 3 {
				t.Errorf("%d pages restored from their chains, want one of each day but the one left out", paged)
			}
			c.checkRested(c.at, ledger.RestOnPage, dateOn(0), dateOn(2), dateOn(3))
		})
	}
}

// pageless is f with its page left out, as of a page JSON can't carry: its
// state without what the page carries, and no part.
func pageless(t *testing.T, f ledger.Folded) ledger.Folded {
	t.Helper()
	var st storedSession
	if err := json.Unmarshal(f.State, &st); err != nil {
		t.Fatal(err)
	}
	st.Page = nil
	state, err := st.put()
	if err != nil {
		t.Fatal(err)
	}
	return ledger.Folded{State: state}
}

// checkRested checks a Follower of the ledger's files at now, asked to rest
// s as rest says, rests it on the state of the last of the dates given, with
// the parts of the days of every one of them, oldest first, as a chain of
// them, where it's asked to rest on its page; else none.
func (c *digestChain) checkRested(now time.Time, rest ledger.Rest, dates ...string) {
	c.t.Helper()
	follower := ledger.NewFollower(c.state, func() time.Time { return now }, ledger.CapsOf(nil, nil), DigestVersion, quiet)
	from := follower.SessionsOf([]string{"s"}, func(string) ledger.Rest { return rest }, nil)["s"]
	var chain []string
	for _, p := range from.Parts {
		chain = append(chain, p.Date)
	}
	want := dates
	if rest != ledger.RestOnPage {
		want = nil
	}
	if baseDateOf(from) != dates[len(dates)-1] || !slices.Equal(chain, want) {
		c.t.Errorf("%s: s rested %s at %v on the state of %q, its chain of the parts of %q; want on that of %s, its chain %q",
			c.name, rest, now, baseDateOf(from), chain, dates[len(dates)-1], want)
	}
}

// The answers a read takes are each answered once of its line, every
// account's, of no session; given for a session's page, those of the days
// after the date it asks after, every day's of none, each is as answeredOf
// makes it of that page, the session's own ours and summed so, whichever
// page asks first, and a day none of whose answers are its own shares the
// answers taken. Of generated worlds of a page's, its answers among other
// sessions'.
func TestAnswersTakenAreEachAsAnsweredOfThePageAsked(t *testing.T) {
	for seed := range uint64(20) {
		lines, _ := pagedRun(seed)
		byDate := linesByDate(lines)
		taken := make(takenAnswers)
		for _, date := range slices.Sorted(maps.Keys(byDate)) {
			for _, l := range byDate[date] {
				taken.take(date, l)
			}
		}
		for _, id := range append(sessionsIn(lines), "none") {
			for _, after := range append([]string{""}, slices.Sorted(maps.Keys(byDate))...) {
				got := taken.of(id, after)
				want := answersOf(byDate, id)
				maps.DeleteFunc(want, func(date string, _ []answered) bool { return date <= after })
				if !reflect.DeepEqual(got, want) {
					t.Errorf("seed %d, session %s, after %q: the answers taken are\n%v\nwant, as answered of its page\n%v", seed, id, after, got, want)
				}
				for date, answers := range got {
					if !slices.ContainsFunc(answers, func(a answered) bool { return a.ours }) && &answers[0] != &taken[date].answers[0] {
						t.Errorf("seed %d, session %s: the answers of %s, none its own, are a copy, want those taken", seed, id, date)
					}
				}
			}
		}
	}
}
