package dashboard

import (
	"strings"

	"github.com/leeovery/switchboard/internal/views"
)

// ledgerNeverKeeps are what of a request the ledger never keeps, as a
// request's details say at their foot and help explains.
const ledgerNeverKeeps = "the messages, the system prompt, the tools' definitions, their inputs and results"

// logHelp are the Log's parts as help explains them, after its keys, in the
// design's order: the marks, Requests' and then the event kinds', each drawn
// as the table draws it; the columns, each by its head; the find; why
// checks and counts are hidden; worth at API prices; and what the ledger
// never keeps.
func (f Frame) logHelp() []helpPart {
	return []helpPart{
		{
			heading: logSubTabHeading(LogRequests),
			says: []line{
				said("A line a request the router routes, the newest at the foot, back through the days as the table scrolls, a rule " +
					"between them. `↑` pauses the table on its newest line, and `end` follows again; paused, the lines come since are " +
					"counted, not drawn. A request in flight sits where it arrived, and fills in as its line is written."),
			},
			glyphs: rowMarkGlyphs(),
		},
		{
			heading: logSubTabHeading(LogEvents),
			says: []line{
				said("A line a thing the router decided or noticed, a moment between requests; every move it makes is among them, " +
					"with why. A move's cost is what writing its session's context again on the new account was worth."),
			},
			glyphs: latelyGlyphs(),
		},
		{
			heading: "COLUMNS",
			says: []line{
				said("`f` shows and hides the columns, those after out hidden at first, and picks which lines show. The columns " +
					"shown are kept for next time; what lines show isn't, so the Log opens on everything."),
			},
			glyphs: columnGlyphs(),
		},
		{
			heading: "FIND",
			says: []line{
				said("`/` finds a session by its id's start or its directory's name, today's first, then back a day at a time " +
					"through the days the ledger keeps. `↑` and `↓` pick among those it finds, and `enter` shows the one picked's " +
					"lines alone, in both tables; `esc` cancels it. With a filter on and nothing open, `esc` clears it."),
			},
		},
		{
			heading: "CHECKS AND COUNTS",
			says: []line{
				said("Claude Code's quota checks, a request of one token it sends to read its account's usage, and its counts of " +
					"tokens, which spend nothing, are hidden until `f` shows them: neither is work done for you, and they would " +
					"crowd the lines that are. The heading counts them, and none counts among the requests, here or anywhere."),
			},
		},
		{
			heading: "WORTH AT API PRICES",
			says: []line{
				said("What the same tokens would have cost through the API, each version at its own prices, at today's, from the " +
					"dated price table, worked out as it's read and never stored. A model the table doesn't know is unpriced, never " +
					"free."),
			},
		},
		{
			heading: "NEVER KEPT",
			says: []line{
				said("The ledger keeps what a request was, never what it said: never kept are " + ledgerNeverKeeps + "."),
			},
		},
	}
}

// logSubTabHeading is the heading help explains a sub-tab of the Log's
// under: its name, as the heading row names it, in capitals.
func logSubTabHeading(v LogView) string {
	return strings.ToUpper(logViews[v])
}

// rowMarkMeans are what each mark a request's row leads with means, in the
// order the table picks the first that applies.
var rowMarkMeans = []struct {
	mark  views.RowMark
	means string
}{
	{views.MarkAsking, "in flight, asking: its answer's first byte not come"},
	{views.MarkAnswering, "in flight, answering"},
	{views.MarkFailed, "failed: 400 and over, cut off or given up"},
	{views.MarkMoved, "it moved its session to another account"},
	{views.MarkNew, "a session's first request"},
	{views.MarkHidden, "a quota check or a count of tokens, shown"},
}

// rowMarkGlyphs are the marks a request's row leads with, each drawn as the
// table draws it, and what it means.
func rowMarkGlyphs() []glyph {
	glyphs := make([]glyph, len(rowMarkMeans))
	for i, m := range rowMarkMeans {
		glyphs[i] = glyph{drawn: line{rowMark(views.RequestRow{Mark: m.mark})}, means: m.means}
	}
	return glyphs
}

// logColumnMeans are what each of the Requests' columns holds.
var logColumnMeans = map[LogColumn]string{
	LogAt:        "when it arrived",
	LogSession:   "its session's id, and the last part of its directory",
	LogModel:     "the model it asked for",
	LogOn:        "the account whose answer the client got",
	LogStatus:    "its answer's status, bold red from 400",
	LogFirst:     "the time to its answer's first byte",
	LogTook:      "its whole time",
	LogRead:      "the tokens it read from the cache",
	LogWritten:   "the tokens it wrote to the cache, bold where it moved its session and wrote its whole context again",
	LogOut:       "the tokens it put out",
	LogWorth:     "what it would have cost through the API",
	LogInput:     "its input tokens, those the cache neither read nor wrote",
	LogAttempts:  "the times it was sent upstream",
	LogSize:      "the size of what it sent",
	LogMessages:  "the messages it sent",
	LogTools:     "the tools it offered",
	LogThinking:  "its thinking budget",
	LogAgent:     "the client that sent it, by its name and version",
	LogRequestID: "the router's id for it",
	LogStopped:   "why its answer stopped, and the tool it called",
	LogRouting:   "how the router chose its account, and what a move cost; blank where it stayed",
}

// columnGlyphs are the Requests' columns, each by its head, drawn as the
// table draws it, and what it holds: those shown at first, in the table's
// order, then the extras f shows.
func columnGlyphs() []glyph {
	var shown, extras []glyph
	for _, c := range EveryLogColumn() {
		g := glyph{drawn: line{{c.Name(), dimInk}}, means: logColumnMeans[c]}
		if c.extra() {
			extras = append(extras, g)
			continue
		}
		shown = append(shown, g)
	}
	return append(shown, extras...)
}
