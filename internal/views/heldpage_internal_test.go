package views

import (
	"strconv"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

// Past midnight, a held page is built afresh once, as the first look takes
// in the new day's lines, none here, and rolls it on to the new day: no look
// after builds it again, each its story as held.
func TestAHeldPagePastMidnightIsBuiltOnceForAllItsLooks(t *testing.T) {
	before := time.Date(2026, 10, 7, 23, 50, 0, 0, time.Local)
	after := before.Add(20 * time.Minute)
	lines := []ledger.Line{{
		Request: "r1", At: before, Kind: ledger.KindMessage, Session: "s1", Model: "claude-opus-5-5", Account: "work",
		Status: 200, Attempts: 1, TotalMS: 4000, Usage: []byte(`{"input_tokens":1000}`),
	}}
	h := HoldPage("s1", SessionsRead{day: Today(before), lines: map[string][]ledger.Line{"s1": lines}}, ledger.Pricing, 0, before)
	built := map[*story]bool{}
	for range 2 {
		h = h.Take(Today(after).Date, nil)
		built[h.story] = true
		built[h.at(after).story] = true
	}
	if len(built) != 1 || h.day != Today(after) {
		t.Errorf("two looks past midnight built the page %d times, held at %s; want it built once, held at %s", len(built), h.day.Date, Today(after).Date)
	}
	if lags := h.Take(Today(before).Date, nil); lags.story != h.story || lags.day != h.day {
		t.Errorf("a look of the day before, as a ledger yet to turn gives, held the page at %s, built afresh %t; want it as held", lags.day.Date, lags.story != h.story)
	}
}

// A held page takes in other sessions' lines working out only what they
// change. Those of an account the session never ran on change nothing of
// it: its story and its accounts are the page's it took them from, and the
// keys of the lines it held before are shared with that page, not copied, as
// much allocated however many requests it holds. Those of one it ran on, on
// a day it made requests on, change its points alone, its story as it was.
// Its own build it afresh.
func TestAHeldPageTakingOtherSessionsLinesWorksOutOnlyWhatTheyChange(t *testing.T) {
	at := time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local)
	date := Today(at).Date
	reset := strconv.FormatInt(at.Add(3*time.Hour).Unix(), 10)
	lineOf := func(request, session, account string, d time.Duration, used string) ledger.Line {
		return ledger.Line{
			Request: request, At: at.Add(d), Kind: ledger.KindMessage, Session: session, Model: "claude-opus-5-5", Account: account,
			Status: 200, Attempts: 1, TotalMS: 4000, Usage: []byte(`{"input_tokens":1000}`),
			Limits: map[string]string{"5h-utilization": used, "5h-reset": reset},
		}
	}
	holding := func(n int) HeldPage {
		lines := make([]ledger.Line, n)
		for i := range lines {
			used := strconv.FormatFloat(0.10+0.05*float64(i)/float64(n-1), 'f', 4, 64)
			lines[i] = lineOf("s"+strconv.Itoa(i), "s1", "work", time.Duration(i-n)*time.Minute, used)
		}
		read := SessionsRead{day: Today(at), lines: map[string][]ledger.Line{"s1": lines}, requests: map[string][]ledger.Line{date: lines}}
		return HoldPage("s1", read, ledger.Pricing, 0, at)
	}
	theirs := func(request, account string, d time.Duration, used string) []ledger.Held {
		return []ledger.Held{{Line: lineOf(request, "s2", account, d, used)}}
	}

	// Its answer counts no tokens and reads no windows: under the race
	// detector, the pools reading them draw on allocate unevenly.
	side := []ledger.Held{{
		Request: "o1", At: at.Add(time.Second), Kind: ledger.KindMessage, Session: "s2", Model: "claude-opus-5-5", Account: "side", Status: 200, Attempts: 1, TotalMS: 4000,
	}}
	few, many := holding(50), holding(500)
	elsewhere := many.Take(date, side)
	if elsewhere.story != many.story || &elsewhere.accounts[0] != &many.accounts[0] || elsewhere.held.under != many.held {
		t.Errorf("taking another's line on side, the story was built afresh %t, the accounts %t, the keys held copied %t; want none of them",
			elsewhere.story != many.story, &elsewhere.accounts[0] != &many.accounts[0], elsewhere.held.under != many.held)
	}
	fewer := testing.AllocsPerRun(10, func() { few.Take(date, side) })
	if got := testing.AllocsPerRun(10, func() { many.Take(date, side) }); got != fewer {
		t.Errorf("taking another's line on side allocates %.0f times holding 500 requests, %.0f holding 50; want as many, whatever it holds", got, fewer)
	}

	h := holding(2)
	if got := h.accounts[0].Points["5h"]; got != 5 {
		t.Fatalf("held, s1 took %d points of work's 5-hour, want 5", got)
	}
	shared := h.Take(date, theirs("o3", "work", -90*time.Second, "0.12"))
	if got := shared.accounts[0].Points["5h"]; shared.story != h.story || got != 3 {
		t.Errorf("taking another's line on work between its two, the story was built afresh %t, its points of work's 5-hour %d; want the story as it was, the points 3, worked out afresh",
			shared.story != h.story, got)
	}
	if own := shared.Take(date, []ledger.Held{{Line: lineOf("s9", "s1", "work", 2*time.Second, "0.40")}}); own.story == shared.story {
		t.Errorf("taking a line of its own, the story is as it was, want it built afresh")
	}
}
