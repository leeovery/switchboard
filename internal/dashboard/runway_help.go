package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/views"
)

// runwayHelp are Runway's parts as help explains them, after its keys, in
// the design's order: the lanes, their marks and colours and the words under
// them; the pool's lane and its words; stepping back and how far it
// reaches; how long a column is; and COMING UP. With one account, which has
// no pool's lane, the pool's part goes, as it does from the page.
func (f Frame) runwayHelp() []helpPart {
	parts := []helpPart{f.lanesHelp()}
	if lanes := f.Runway.Lanes; lanes == nil || len(lanes.Accounts) != 1 {
		parts = append(parts, poolHelp())
	}
	return append(parts, steppingBackHelp(), f.columnsHelp(), runwayComingHelp())
}

// lanesHelp is the lanes as help explains them: what a column is drawn by,
// and, where the look blends, that the past is dimmed; the label and the use
// beside each; the words under each; what each column is read from, a cap
// by the reserve the config gives now, so a reserve changed redraws the
// caps of days gone; how the page compacts; and a key to the lanes' marks,
// each drawn as a lane draws it.
func (f Frame) lanesHelp() helpPart {
	dimmed := ""
	if f.Look.blends() {
		dimmed = ", and everything before it dimmed"
	}
	return helpPart{
		heading: "LANES",
		says: []line{
			said("A lane an account, across the window the view is named for: the 5-hour view from the hour before now, the week " +
				"view from a day before now. Each column is drawn by how the account stood at the column's middle, and now's " +
				"column, picked out under now, by how it stands now" + dimmed + ". Every window every model shares counts, a model's " +
				"own window not, but the 5-hour view leaves out a week running out: its lane has room until the week's limit."),
			said("Beside each lane, the account's number, name and marks, as switchboard usage prints them, and under them its use " +
				"of the view's window, its percentage taking the ramp's second colour from 70% and its last from 90%; not read " +
				"since 14:20 where its last read failed; and not read yet before its first, its lane blank, saying why where that " +
				"read failed. A stale account's lane and use are faded, as the Overview fades a stale account, and the use of one " +
				"whose token is refused."),
			said("Under each lane, words say where each stretch without room starts and when room is back: at its cap, at its " +
				"limit, its token refused, or runs out ~16:05 at its pace, ~ marking a time projected at its pace, as switchboard " +
				"usage projects it. A cap's or a run-out's back adds as its 5-hour resets, or as its week resets, where room comes " +
				"back as the view's window resets. In the week view, resets Sun 21:00 where its week reaches no limit; else room " +
				"all day, or room all week."),
			said("Now's column and those after it are read from the router at each look; those before it from the readings " +
				"history, each by the last reading of each window by the column's middle, a window whose reset had passed by then " +
				"counted reset. A column with no reading of its account by then is blank, as an account not read yet. A cap is " +
				"drawn by the reserve the config gives now, as the readings keep none: change a reserve, and the caps of days gone " +
				"are drawn again by it."),
			said("As rows run short, an account takes two rows, the blank rows go and COMING UP folds; past that, the lanes " +
				"scroll with `j` and `k`, the axis and the pool's lane holding."),
		},
		glyphs: []glyph{
			{drawn: line{f.Look.laneMark(views.CellRoom)}, means: "room"},
			{drawn: line{f.Look.laneMark(views.CellRunsOut)}, means: "room, but at its pace it runs out before its window resets"},
			{drawn: line{f.Look.laneMark(views.CellCapped)}, means: "at its cap, where its reserve starts: the router passes it over, unless a pin spends it"},
			{drawn: line{f.Look.laneMark(views.CellLimited)}, means: "at its limit, its token refused, or without a usable token: held back from every request"},
			{drawn: line{{resetCell, dimInk}}, means: "where its " + runwayNames[f.Runway.Window] + " resets"},
		},
	}
}

// poolHelp is the pool's lane as help explains it: its columns, how they're
// counted and coloured; which accounts the pool is; and the room and the
// words under it.
func poolHelp() helpPart {
	return helpPart{
		heading: "POOL",
		says: []line{
			said("Under the pool's rule, Σ pool: a column for each of the lanes', filled to how many of the pool's accounts have " +
				"room then, and coloured as switchboard usage colours the pool's use, the ramp's second colour from 70% used and " +
				"its last from 90%. An account at its cap or its limit counts without room, one running out at its pace with room " +
				"until it does, and one not known then without."),
			said("The pool is every account on auto, and the pin's while one holds, as pinned says beside its room; an account " +
				"out of it is faded, the words under its lane not."),
			said("Under its lane, the pool's room in the view's window, as switchboard usage gives it, then a phrase for each dip, " +
				"a stretch where fewer than all of the pool have room: 8 of 10 with room till 15:30 for one under way, ~16:05 – " +
				"16:55 for one ahead, and 15:02 – 17:10 on a day stepped back to, each change of its count chained on, as 9 till " +
				"~16:05. A dip too close to the phrase before for its own joins it with and, where its count is the same."),
		},
	}
}

// steppingBackHelp is stepping back as help explains it: what ← and → step
// to in each view; what < and > show; how far back ← reaches; how a day
// stepped back to is drawn; and that a step back is never kept.
func steppingBackHelp() helpPart {
	return helpPart{
		heading: "STEPPING BACK",
		says: []line{
			said("`←` steps back: in the 5-hour view, first to today from its midnight, then a day at a time; in the week view, " +
				"first to this week from its first day, as week_starts begins weeks, then a week at a time. `→` steps forward, and " +
				"from today, or this week, to the live view: nothing lies ahead to step to. `<` and `>` show the other window's " +
				"live view."),
			said("`←` reaches back as far as the day summaries, which are kept for good; a day older than the readings history's " +
				"keep draws its lanes from its summary's limits and resets alone. A day or a week stepped back to is drawn as it " +
				"was, with no now, nothing dimmed, no COMING UP and no room line, under each name its highest use; today and this " +
				"week keep now, what's past dimmed and the room line. The pool's lane counts the pool as it is now."),
			said("The window shown is kept for next time; a step back never is, so Runway opens live."),
		},
	}
}

// columnsHelp is how long a column is as help explains it: the views' spans
// cut evenly among the lanes' columns, so a column's length follows the
// width; and how long it is at the frame's width in each view, and on a day
// stepped back to, as columnLengths says them.
func (f Frame) columnsHelp() helpPart {
	fiveHour, week, day := columnLengths(RunwayColumns(f.Width))
	return helpPart{
		heading: "COLUMNS",
		says: []line{
			said("The 5-hour view's 22 hours 40 minutes, the week's seven days, and a day stepped back to, midnight to midnight, " +
				"are each cut evenly among the lanes' columns, so a column's length follows the width: at this width, a column is " +
				fiveHour + " in the 5-hour view, " + week + " in the week's and " + day + " on a day stepped back to, as the legend " +
				"says of the view shown."),
		},
	}
}

// columnLengths are how long a column is across the columns given, as the
// legend says it: in the live 5-hour view; in the live week; and on a day
// stepped back to of 24 hours.
func columnLengths(columns int) (fiveHour, week, day string) {
	// A live view's span is the same at any moment, and a day's in UTC, which
	// has no clock change, is 24 hours: the Unix epoch serves for each.
	at := time.Unix(0, 0).UTC()
	return prose.ColumnLength(views.LiveTimeline(views.RunwayFiveHour, at, columns).Step()),
		prose.ColumnLength(views.LiveTimeline(views.RunwayWeek, at, columns).Step()),
		prose.ColumnLength(views.Stepped(views.RunwayFiveHour, at, at, time.Monday, columns).Step())
}

// runwayComingHelp is Runway's COMING UP as help explains it: what it lists,
// in each view, and what it leaves out; how it folds; and that a step back
// shows none.
func runwayComingHelp() helpPart {
	return helpPart{
		heading: SectionComing.heading(),
		says: []line{
			said("What happens next, soonest first, as the Overview's COMING UP has it: an account back from its cap or its limit, " +
				"a window running out at its pace, ~ marking it projected, and each other reset of the view's window, where its │ " +
				"stands. The 5-hour view lists the 5-hour windows'; the week view adds each week's run-out and reset. No prime is " +
				"listed. As rows run short, it folds to its heading, which names the soonest; stepped back, there's none."),
		},
	}
}
