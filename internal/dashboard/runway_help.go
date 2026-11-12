package dashboard

import (
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/views"
)

// runwayHelp are Runway's parts as help explains them, after its keys, in
// the design's order: the lanes, their marks and colours and the words under
// them; the pool's lane and its room; stepping back and how far it
// reaches; how long a column is; COMING UP; and LATELY. Where there's no
// pool's lane, as with one account, the pool's part goes, as it does from
// the page.
func (f Frame) runwayHelp() []helpPart {
	parts := []helpPart{f.lanesHelp()}
	if lanes := f.Runway.Lanes; lanes == nil || lanes.Pool.Of > 0 {
		parts = append(parts, poolHelp())
	}
	return append(parts, steppingBackHelp(), f.columnsHelp(), runwayComingHelp(), runwayLatelyHelp())
}

// lanesHelp is the lanes as help explains them: what a column is drawn by,
// and, where the look blends, that the past is dimmed; the label and the use
// beside each, and the label collapsed on a; the words under each; what each column is read from, a cap
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
			said("A lane an account, across the week from a day before now. Each column is drawn by how the account stood at " +
				"the column's middle, and now's column, picked out under now, by how it stands now" + dimmed + ". Every window " +
				"every model shares counts, a model's own window not."),
			said("Beside each lane, the account's name and marks, as switchboard usage prints them, and under them its use " +
				"of the week, its percentage taking the ramp's second colour from 70% and its last from 90%; read 14:20 " +
				"where its last read failed; and not read before its first, its lane blank, saying why where that read failed. " +
				"A stale account's lane and use are faded, as the Overview fades a stale account, and the use of one " +
				"whose token is refused."),
			said("`a` collapses the labels, to give the lanes more room: the lanes start at the left edge, with no names or " +
				"marks, the status line saying where new sessions go, and under the start of each its week's use, as 66%, " +
				"or — where there's no use to show, faint where the pool leaves its account out. The words " +
				"under a lane start two cells after its use where they'd otherwise meet it. `a` again expands them. It's kept " +
				"for next time."),
			said("Under each lane, words say where each stretch without room starts and when room is back: at its cap, at its " +
				"limit, its token refused, as capped 14:31 and out 13:58; ahead, out ~16:05 where it runs out, or caps ~15:45 " +
				"where it reaches its cap, ~ marking a time projected at its pace, as switchboard usage projects it. Where " +
				"another stretch holds the account on, a stretch says lifts 19:00, when its own hold lifts, and only the last " +
				"says back. Where room returns as the week resets, a cap's or a run-out's end says resets Sun 21:00 in " +
				"place of back. Else resets Sun 21:00 where its week reaches no limit, or room all week. Words that don't " +
				"fit from where a stretch starts shift left until they do, never over the words " +
				"before, and drop parts from the end where even that can't fit them."),
			said("Now's column and those after it are read from the router at each look; those before it from the readings " +
				"history, each by the last reading of each window by the column's middle, a window whose reset had passed by then " +
				"counted reset. A column with no reading of its account by then is blank, as an account not read yet. A cap is " +
				"drawn by the reserve the config gives now, as the readings keep none: change a reserve, and the caps of days gone " +
				"are drawn again by it."),
			said("The legend sits at the foot, over the key line. As rows run short, LATELY under COMING UP goes first; then an " +
				"account takes two rows, the blank rows " +
				"go and COMING UP shows the lines it can; then the legend gives way, then COMING UP, then the blank row under " +
				"the axis; only then do the lanes scroll with `j` and `k`, the axis and the pool's lane holding, and what's out " +
				"of view said beside the pool's room."),
		},
		glyphs: []glyph{
			{drawn: line{f.Look.laneMark(views.CellRoom)}, means: "room"},
			{drawn: line{f.Look.laneMark(views.CellRunsOut)}, means: "room, but at its pace it runs out before its window resets"},
			{drawn: line{f.Look.laneMark(views.CellCapped)}, means: "at its cap, where its reserve starts: the router passes it over, unless a pin spends it"},
			{drawn: line{f.Look.laneMark(views.CellLimited)}, means: "at its limit, its token refused, or without a usable token: held back from every request"},
			{drawn: line{{resetCell, dimInk}}, means: "where its " + runwayWeek + " resets"},
		},
	}
}

// poolHelp is the pool's lane as help explains it: its columns, how they're
// counted and coloured; its room under Σ pool, or collapsed, under its start; and
// which accounts the pool is.
func poolHelp() helpPart {
	return helpPart{
		heading: "POOL",
		says: []line{
			said("Beside Σ pool, under the lanes: a column for each of the lanes', filled to how many of the pool's accounts have " +
				"room then, and coloured as switchboard usage colours the pool's use, the ramp's second colour from 70% used and " +
				"its last from 90%, ▃ at the least where any has room, and where none has room, ▂ along its foot, in a " +
				"limit's red. An account at its cap or its limit counts without room, one running out at its pace with room " +
				"until it does, and one not known then without. Under Σ pool, the pool's room in the week, as " +
				"switchboard usage gives it; with the labels collapsed, without Σ pool, under the lane's start, of how many of " +
				"the pool's accounts have the week, as 1.2/3."),
			said("The pool is every account on auto, and the pin's while one holds, as pinned says after its room where it fits; " +
				"an account out of it is faded, the words under its lane not."),
		},
	}
}

// steppingBackHelp is stepping back as help explains it: what ← and → step
// to; how far back ← reaches; how a week stepped back to is drawn; and that
// a step back is never kept.
func steppingBackHelp() helpPart {
	return helpPart{
		heading: "STEPPING BACK",
		says: []line{
			said("`←` steps back, first to this week from its first day, as week_starts begins weeks, then a week at a time. " +
				"`→` steps forward, and from this week to the live view: nothing lies ahead to step to."),
			said("`←` reaches back as far as the day summaries, which are kept for good; a week older than the readings " +
				"history's keep draws its lanes from its summaries' limits and resets alone. A week stepped back to is drawn as " +
				"it was, with no now, nothing dimmed, no COMING UP, no LATELY and no room line, under each name its highest " +
				"use; this week keeps now, what's past dimmed and the room line. The pool's lane counts the pool as it is now."),
			said("A step back is never kept, so Runway opens live."),
		},
	}
}

// columnsHelp is how long a column is as help explains it: the week cut
// evenly among the lanes' columns, so a column's length follows the width;
// and how long it is at the frame's width, as columnLength says it.
func (f Frame) columnsHelp() helpPart {
	return helpPart{
		heading: "COLUMNS",
		says: []line{
			said("The week's seven days are cut evenly among the lanes' columns, so a column's length follows the width: at " +
				"this width, a column is " + columnLength(f.Runway.Columns(f.Width)) + ", as the legend says."),
		},
	}
}

// columnLength is how long a column of the live week is across the columns
// given, as the legend says it.
func columnLength(columns int) string {
	// A live week's span is the same at any moment: the Unix epoch serves.
	return prose.ColumnLength(views.LiveTimeline(time.Unix(0, 0).UTC(), columns).Step())
}

// runwayComingHelp is Runway's COMING UP as help explains it: what it lists,
// and what it leaves out; where it sits and how it cuts its lines; and that
// a step back shows none.
func runwayComingHelp() helpPart {
	return helpPart{
		heading: SectionComing.heading(),
		says: []line{
			said("What happens next, soonest first, as the Overview's COMING UP has it: an account back from its cap, its limit or " +
				"its refused token, whichever lifts last, a window running out at its pace, ~ marking it projected, and each " +
				"other reset of a window every model shares, a week's where its │ stands. No prime is listed. It sits under the " +
				"pool's lane, or at the right beside LATELY, and takes the rows left; where its lines don't all fit, its last " +
				"says how many more. Stepped back, there's none."),
		},
	}
}

// runwayLatelyHelp is Runway's LATELY as help explains it: what it tells, and
// what it leaves out; how a line reads; where it sits at each width; and that
// a step back shows none.
func runwayLatelyHelp() helpPart {
	return helpPart{
		heading: SectionLately.heading(),
		says: []line{
			said("What befell the accounts over the week gone, newest first, as the Overview's LATELY tells it: a cap, a limit, " +
				"a refused token, coming under pressure and room again, the sessions each moved said in its line. No session's " +
				"start or move shows alone. Each line ends with how long ago it was, as 11m ago or 1d 4h ago, its words in full " +
				"where they fit beside it."),
			said("It takes only the rows the lanes and COMING UP leave. Where the frame holds both, and those rows hold COMING " +
				"UP's every line in one column, LATELY sits at the left and COMING UP at the right, the past and what's coming " +
				"as the lanes lie, each down to the legend; else under COMING UP, where its heading and two lines fit: COMING " +
				"UP in its two columns where they fit, LATELY across the frame under them, or else the two sharing one " +
				"column, as wide as their lines need to read whole, from 72 to the frame's; else it's gone, the first to give " +
				"way as rows run short. Stepped back, there's none."),
		},
	}
}
