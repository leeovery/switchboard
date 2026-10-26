package dashboard

import (
	"slices"
	"strconv"
)

// graphsHelp are Graphs' parts as help explains them, after its keys, in the
// page's order: the burn-down cards, WEEK's and 5-HOUR's, with the pool's
// line under them; POOL; GLANCE; WHEN YOU WORK; and the room the page gives
// its sections, and how it scrolls.
func (f Frame) graphsHelp() []helpPart {
	return []helpPart{f.burnDownsHelp(), f.poolStackHelp(), glanceHelp(), f.whenHelp(), graphsRoomHelp()}
}

// burnDownsHelp is WEEK's and 5-HOUR's cards as help explains them: what a
// card charts, and over what clock; its words, Runway's, and what changes
// them; where it's heading, why that's straight and its ✕; the reserve;
// the window's ink; an account with nothing to draw; and the pool's line.
// Then a key to a card's marks, each drawn as v0.1's legend draws it, the
// room left, where the look has colour, in each ink a window's state gives
// it.
func (f Frame) burnDownsHelp() helpPart {
	room := glyph{drawn: line{roomIn(lastingInk)}, means: "the room left, to now"}
	var inks []glyph
	if f.Look.coloured {
		room.means += ", of a window whose room lasts to its reset"
		inks = []glyph{
			{drawn: line{roomIn(warningInk)}, means: "of one that runs out at its pace before its reset, or its cap holds"},
			{drawn: line{roomIn(errorInk)}, means: "of one its limit holds, or whose token is refused"},
		}
	}
	glyphs := slices.Concat([]glyph{room}, inks, []glyph{
		{drawn: line{headingMark}, means: "where it's heading, at its recent rate"},
		{drawn: line{{outMark, outInk}}, means: "where it runs out, on the floor it meets; or where its cap or its limit took hold"},
		{drawn: line{reserveMark}, means: "its reserve, where it holds: the room its cap leaves"},
		{drawn: line{{nowLine, faintInk}}, means: "now"},
	})
	return helpPart{
		heading: GraphsWeek.heading() + " AND " + GraphsFiveHour.heading(),
		says: []line{
			said("A card an account, in the config's order, its room over its own window, from the window's start to its reset, " +
				"a column a cell: each card keeps its own clock, so its days or hours, and now's │, fall where its window puts " +
				"them, not where another card's do. The room left is filled to now, each column to the room its middle had, " +
				"as the readings history tells it."),
			said("Over the card, the account's number, name and marks, and its words: the room it has left, as 66% left, its " +
				"figure coloured as switchboard usage colours a use, then what Runway says under its lane in that window's " +
				"view, as at its cap 14:31  ·  back 17:10 or runs out ~Sat 06:00 at its pace, inked as Runway inks them, " +
				"whole parts going from the end where they don't fit. They're read from Runway's live view as Runway would " +
				"draw it at this width, so a hold shorter than one of its columns can go unsaid; a wider terminal, or " +
				"Runway's labels collapsed, gives finer columns, which can bring it out."),
			said("From now, a dotted line says where it's heading: straight, as it holds the rate the window's been used at " +
				"lately to its reset, the pace Runway's words go by. It falls from the room now to where the window runs out, " +
				"its ✕ there, then runs along the floor it meets, its cap or empty, to its reset; or it ends at the room the " +
				"window leaves at its reset. Where its cap or its limit already holds it, ✕ marks the column the hold began, " +
				"where that's told, and the line runs along that floor from now."),
			said("Where the account's reserve holds, so not while a pin spends it, a faint dotted line marks the room its cap " +
				"leaves. The window's ink is its state's, as Runway's lanes colour one, each window by its own: a week that " +
				"lasts is drawn so, though its 5-hour's cap holds the account now."),
			said("An account not read yet, or whose first read failed, has Runway's words for it and its chart empty but for " +
				"now's │; one the router doesn't hold yet, its label alone, numbered after the router's last, as Runway will " +
				"number it. One without a usable token has no room all along; a stale one is faded, and one out of the pool " +
				"faint."),
			said("Under the cards, Σ pool: the pool's room in each window, the week's then the 5-hour's, as Runway's pool lane " +
				"words it, as week 1.2 of 3 left, whichever sections show, and pinned after the first while a pin holds the " +
				"pool. With one account there's no pool, and no line."),
		},
		glyphs: glyphs,
	}
}

// poolStackHelp is POOL as help explains it: its span; each account's room
// stacked, the total its top edge; where it's heading, faint; how two
// accounts share a cell; and what it draws without colour.
func (f Frame) poolStackHelp() helpPart {
	return helpPart{
		heading: GraphsPool.heading(),
		says: []line{
			said("The pool's room over Runway's week view, from a day before now for a week, its columns as Runway's lanes " +
				"have them at this width, under Runway's axis. Each of the pool's accounts' room is stacked from the foot, in " +
				"the config's order, each in its own colour, as the key names them, so the stack's top edge is the pool's " +
				"room, full being every account's. The pool's first " + strconv.Itoa(accountHues) + " accounts each have a colour " +
				"of their own; a fifth on takes the colour four before it, fainter."),
			said("Before now, each account's room as it stood, judged as the Overview's pool charts judge it; from now, faint, " +
				"where it's heading at its recent rate: falling to where it runs out, along its floor to its reset, then full, " +
				"as nothing is known of its next window. Where two accounts meet in a cell, the lower's share is drawn on the " +
				"upper's colour, and the top cell is the colour of the account with most of it, so the top edge reads as the " +
				"pool's total."),
			said("Without colour the accounts can't be told apart, so POOL draws the pool's room alone, in █, as its key says."),
		},
	}
}

// glanceHelp is GLANCE as help explains it: a row an account and the pool,
// each window a sparkline, its room and usage's words; and how it narrows.
func glanceHelp() helpPart {
	return helpPart{
		heading: GraphsGlance.heading(),
		says: []line{
			said("Every account and the pool in a few rows, as switchboard usage lists them: for each, the week and then the " +
				"5-hour, each a burn-down a row high. A sparkline of " + strconv.Itoa(sparkCells) + " cells is the window's " +
				"room from its start to its reset, in the window's ink as the cards have it, faint from now where it's " +
				"heading, and ✕ where it runs out; then the room left now, as 66%; then switchboard usage's words for the " +
				"window, as 19%/day till Sun 21:00, out ~Sat 06:00 or at cap · back 17:10."),
			said("Under the accounts, Σ pool: the pool's room over each window, as POOL counts it, from its accounts' earliest " +
				"start to their latest reset, and its room, as 1.2 of 3 left."),
			said("GLANCE takes the width it needs whole, and no more. Narrower, its words take switchboard usage's short forms, " +
				"then its names narrow; narrower still, its windows stack, the week's rows over the 5-hour's. It sits beside " +
				"WHEN YOU WORK where both show and both fit whole, 10 cells apart; elsewhere WHEN YOU WORK comes under it."),
		},
	}
}

// whenHelp is WHEN YOU WORK as help explains it: its grid, the weeks it
// counts and the requests it counts; then its shades, each drawn as the grid
// draws an hour at its level.
func (f Frame) whenHelp() helpPart {
	grid := hoursGrid{look: f.Look}
	return helpPart{
		heading: GraphsWhen.heading(),
		says: []line{
			said("When the requests come: a row a weekday, from the first day of the week, as week_starts begins weeks, and " +
				"its 24 hours across, each shaded by its requests, summed over the four whole weeks before today and divided " +
				"by the days of that weekday known. A day the ledger keeps no hours of is left out, and the heading says the " +
				"weeks counted, as the 4 weeks to Wed 30 Sep, or fewer where fewer are known; with none, the heading says no " +
				"weeks, and the section " + nothingRecorded + "."),
			said("The requests counted are those that spent quota and went upstream, each in the local hour it arrived in: an " +
				"hour the clocks skip has none that day, and one they repeat counts both."),
			said("The shades are History's: an hour with nothing on it has its own, and the active hours are split into " +
				"quarters, on one scale for the grid, the key under it giving each quarter's bounds in requests an hour."),
		},
		glyphs: shadesHelp("an hour", "hours", func(level int) line { return line{grid.tile(level, 1)} }),
	}
}

// graphsRoomHelp is the room Graphs gives its sections as help explains it:
// each shown at any size, in its order, and kept; where rows to spare go;
// how the page scrolls short of room; and the charts moving on with the
// clock.
func graphsRoomHelp() helpPart {
	return helpPart{
		heading: "ROOM",
		says: []line{
			said("`w`, `f`, `o`, `g` and `h` show and hide WEEK, 5-HOUR, POOL, GLANCE and WHEN YOU WORK, at any size, always " +
				"in that order, and they're kept for next time: the size decides how much room each gets, never whether it " +
				"shows."),
			said("Rows to spare go first to each card's words, a row of their own, then to the charts, the cards' and POOL's, " +
				"every one the same height, up to " + strconv.Itoa(mostChart) + " rows. Short of room, the page scrolls whole, a row at a time, with `j` " +
				"and `k`, a page with PgUp and PgDn, three rows with the wheel, the row over the key line saying what's out of " +
				"view; the legend stays at " +
				"the foot. The layout changes only on a key, a resize or the config, never as the data does."),
			said("The charts move on with the clock: now's column, the room to it and where it's heading, a column at a time " +
				"as each column's moment passes."),
		},
	}
}
