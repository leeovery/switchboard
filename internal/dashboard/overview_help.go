package dashboard

import (
	"slices"
	"strconv"
	"strings"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// overviewHelp are the Overview's parts as help explains them, a section
// each, in the page's order: what it shows, how to read it and why it's where
// it is; then a key to its glyphs and marks, each drawn as the page draws it.
func overviewHelp() []helpPart {
	return []helpPart{
		{
			heading: SectionUsage.heading(),
			says: []line{
				said("Every account's windows, as switchboard usage prints them, first as the rest of the page is read against them. " +
					"A bar fills along the theme's ramp as its window is used, and its percentage takes the ramp's second colour from 70% " +
					"and its last from 90%."),
				said("The words say what can be spent an hour, or a day of a week, and still last to the reset; out ~16:05 where the window " +
					"runs out before it resets, at its recent rate; at cap where its use has reached the cap; and limit while Claude's own " +
					"limit holds it."),
				said("Σ pool is the room new sessions can go to now, in accounts' worth: every account's on auto, the pinned accounts' alone " +
					"when pinned, each to its cap; all accounts, on Accounts and History, counts every account, to its limit. An account out " +
					"of the pool is faint."),
				said("`b` groups the windows by account instead, a block an account, ROUTING beside each, or while GRAPHS shows, its charts. " +
					"With more accounts than fit, USAGE " +
					"scrolls within itself, an account at a time, or a row at a time where not even one account a window fits; turned " +
					"from one to the other, as `b`, a section's key, a resize or what's read can turn it, it goes back to its top."),
			},
			glyphs: []glyph{
				{drawn: line{{paceMark, paceInk}}, means: "even pace: where you would be now, used evenly"},
				{drawn: line{{capMark, reserveInk}}, means: "the cap, where the account's reserve starts: past it, the router passes the account over"},
				{drawn: line{{"◆", primaryInk}}, means: "the primary, the account your browser and the Claude apps are signed into"},
				{drawn: line{{"▲", accentInk}}, means: "the account new sessions go to now"},
			},
		},
		{
			heading: SectionGraphs.heading(),
			says: []line{
				said("The focused account's use so far in each window, each chart under its window's column in USAGE: a column a cell from " +
					"the window's start to its reset, drawn to now. `↑` and `↓` move the focus through the accounts, in the config's order, " +
					"and USAGE lights the one focused."),
				said("`v` flips every chart between two. The burn-up fills to the use read at its time, against even pace. The burn-down is " +
					"the room left, falling from full as the window's used, then where it's heading, at the pace Runway's words go by: to ✕ " +
					"where it runs out, else to the room it leaves at its reset."),
				said("By account, from 150 columns, every account's charts sit beside its block, as many of its windows as fit, so no " +
					"account is focused, and ROUTING takes rows of its own under USAGE; narrower, and on a phone, GRAPHS charts the focused " +
					"account, as by window."),
			},
			glyphs: []glyph{
				{drawn: line{{paceDot, dimInk}}, means: "even pace, on the burn-up: a line to measure by, not a forecast"},
				{drawn: line{{capLine, reserveInk}}, means: "the cap, on the burn-up"},
				{drawn: line{{limitLine, faintInk}}, means: "along the chart's top: the limit, on the burn-up; full, on the burn-down"},
				{drawn: line{{nowLine, faintInk}}, means: "now"},
				{drawn: line{{headingKey, dimInk}}, means: "where the window's heading, on the burn-down"},
				{drawn: line{{outMark, outInk}}, means: "where it runs out, on the burn-down's floor: the cap's, where that holds the account back"},
				{drawn: line{{capKey, capDotInk}}, means: "the cap, on the burn-down"},
				{drawn: line{{floorLine, errorInk}}, means: "no room left: at the limit, and on along the floor till it lifts"},
			},
		},
		{
			heading: SectionRouting.heading(),
			says: []line{
				said("How each account stands with the router, after USAGE as what the router makes of it. On auto, a new session goes to " +
					"the account whose week most needs using, among those with room in every window its model counts against; pinned, to " +
					"the best of the accounts pinned, and elsewhere only once none of them has room. `p` changes it, in the routing card."),
				said("At its cap, an account's reserve holds it back, and new sessions go elsewhere until the window resets, unless the pin " +
					"names it. Under pressure, its 5-hour window runs out before it resets, at its recent rate, so new sessions go to an " +
					"account that isn't."),
				said("A trace is an account's requests a minute over the last 30 minutes, the newest at the right, and its rate the last " +
					"minute's. Every trace on the page, a session's too, shares one scale, so any two compare: the busiest column fills a " +
					"trace's height, never less than " + strconv.Itoa(leastScale) + " requests a minute, so a quiet page isn't magnified. " +
					"Only requests that spend quota count, each once, on the account it went out on."),
			},
			glyphs: []glyph{
				stateGlyph(status.RoutingTokenless, "no usable token"),
				stateGlyph(status.RoutingLimited, "held back from every request, by a limit or its token refused"),
				stateGlyph(status.RoutingModelLimited, "held back from some models' requests alone, by a limit or a refusal"),
				stateGlyph(status.RoutingUnreadable, "its usage can't be read"),
				stateGlyph(status.RoutingPressed, "at its cap, or under pressure"),
				stateGlyph(status.RoutingOpen, "open"),
				stateGlyph(status.RoutingUnread, "not read yet, or idle, its 5-hour window lapsed"),
				{drawn: line{pinnedSpan}, means: "named by the global pin"},
			},
		},
		{
			heading: SectionSessions.heading(),
			says: []line{
				said("The sessions the router has routed in the last hour, by account, under ROUTING as the sessions behind its traces. " +
					"An id is bright while its session is busy, with a request in flight or one in the last minute; after it, why the " +
					"session is on its account, and its own trace, on ROUTING's scale."),
			},
			glyphs: []glyph{
				{drawn: line{{"↑ ask", doingInk}}, means: "asking: its request sent, its answer not begun"},
				{drawn: line{runningDoing(views.Activity{Doing: views.Answering, Chars: 1200 * views.CharsPerToken})}, means: "answering, its tokens so far at four characters a token"},
			},
		},
		{
			heading: SectionComing.heading(),
			says: []line{
				said("What happens next, soonest first, as the router expects it: a limit or a cap lifting, a window running out at its " +
					"pace or resetting, and an account primed. A time after ~ is projected. LATELY sits beside it, what's next beside " +
					"what's been."),
			},
		},
		{
			heading: SectionLately.heading(),
			says: []line{
				said("What the router has done, newest first, as the Log's events tell it. The sessions a limit, a cap or a refusal moved " +
					"fold into its line, counted."),
			},
			glyphs: latelyGlyphs(),
		},
	}
}

// said is a paragraph of help, in text.secondary, but for its keys, each
// between backquotes, which are bold accent.key, as everywhere.
func said(words string) line {
	var l line
	for i, part := range strings.Split(words, "`") {
		k := secondaryInk
		if i%2 == 1 {
			k = keyInk
		}
		if part != "" {
			l = append(l, span{part, k})
		}
	}
	return l
}

// stateGlyph is the glyph an account's state of the kind given leads with in
// ROUTING, in the colour ROUTING draws it in, and what it means.
func stateGlyph(kind status.RoutingKind, means string) glyph {
	return glyph{drawn: line{{kind.Glyph(), standingLead(kind, "")[0].ink}}, means: means}
}

// latelyGlyphs are LATELY's marks, each kind's as eventKinds has it, in its
// order, and what it means: a mark once, meaning what each kind drawing it
// is, joined, as "an account at its limit, or an account's token refused".
func latelyGlyphs() []glyph {
	var glyphs []glyph
	for _, k := range eventKinds {
		i := slices.IndexFunc(glyphs, func(g glyph) bool { return g.drawn[0] == k.mark })
		if i < 0 {
			glyphs = append(glyphs, glyph{drawn: line{k.mark}, means: k.means})
			continue
		}
		glyphs[i].means += ", or " + k.means
	}
	return glyphs
}
