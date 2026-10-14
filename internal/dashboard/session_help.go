package dashboard

import "strconv"

// helpName is the name of the page help explains, as HELP · <PAGE> names
// it: Session on a session's page, else the tab's, as the title row names
// it.
func (f Frame) helpName() string {
	return f.Shown().Title()
}

// sessionHelp are a session's page's parts as help explains them, after its
// keys, in the page's order: STORY, its scale, its moves and what they cached
// again, and its folds; the accounts it ran on, their ≈ points and worth at
// API prices; and TURNS, what a turn is and how its row reads; each with a
// key to its marks, drawn as the page draws them.
func sessionHelp() []helpPart {
	return []helpPart{
		{
			heading: "STORY",
			says: []line{
				said("Requests a minute over the session's whole life, its first request at the left and now, or its end, at the " +
					"right, the minutes shared out evenly across the width, so a young session takes a column a minute at most. A " +
					"column is the mean over the minutes it stands for, a request counting in the minute it arrived; only requests " +
					"that spend quota count, each once. The turn chosen is lit along it, as `↑` and `↓` move through the turns."),
				said("Its scale is the traces': the busiest column fills all five rows, never less than " + strconv.Itoa(leastScale) +
					" requests a minute, so a quiet session isn't magnified."),
				said("A move is marked where it came, its label saying what moved the session and where to. Moving, a session " +
					"writes its conversation into the new account's cache, about as many tokens as its last request sent, as no " +
					"account reads another's: the label says how many it cached again, and their worth at API prices. A move after " +
					"the session idled longer than its cache lasts, an hour, or five minutes for five-minute writes, says no cost, " +
					"as it would have written them again anyway."),
				said("A gap of an hour or more without a request folds into five columns, its length in whole hours on the axis, " +
					"then the time the session came back. Under the columns, a stretch for each time on an account."),
			},
			glyphs: []glyph{
				{drawn: line{{"┊", warningInk}}, means: "a move, with what it cached again, where it did"},
				{drawn: line{{"╱ ╱", faintInk}}, means: "an idle gap of an hour or more, folded"},
				{drawn: line{{"▁", faintInk}}, means: "a column with no requests"},
				{drawn: line{{"├─", faintInk}, {" side ", secondaryInk}, {"─┤", faintInk}}, means: "a time on one account"},
			},
		},
		{
			says: []line{
				said("Under the story, the accounts it ran on, a row each, in the order it first ran on them: its time there, its " +
					"requests, what it took of the account's windows, and its worth. A turn that moved splits between the two, each " +
					"request counted on the account that answered it."),
				said("≈ points are an estimate of what the session took of a window, as a window counts its account's whole use, " +
					"every session's: each rise in its use from one answer to the next is shared out among the requests that ended " +
					"between the two, of whichever session, by their tokens, every kind counted alike. A session's points are its " +
					"requests' shares summed, in whole points of the window, a reset starting it afresh."),
				said("Worth is what the same requests would cost through the API, at today's prices from the price table: never " +
					"what's billed. A model the table doesn't know is unpriced, never free, and a sum that leaves one out ends in +."),
			},
		},
		{
			heading: "TURNS",
			says: []line{
				said("A turn is a prompt and every request that serves it, as Claude Code tells them: its subagents' requests, a " +
					"compaction, and a side request for a title or a summary count in the turn of the prompt that started them. A " +
					"turn ends with its conversation's last answer. A request that names no prompt, as from an older Claude Code, " +
					"is in no turn."),
				said("Took is the minutes from the one a turn started in to the one it ended in, and while it runs, to now, an " +
					"ellipsis after them. Read, written and out are the tokens it read from the cache, wrote to it and put out; on, the account its " +
					"conversation went to, both for a turn that moved, its written, which holds the context cached again, in the " +
					"attention colour."),
				said("`enter` opens the turn chosen into its requests, as the Log draws them, `↑` and `↓` moving through them."),
			},
			glyphs: []glyph{
				{drawn: line{{"↑ asking", keyInk}}, means: "a request of the newest turn waiting for its answer"},
			},
		},
	}
}
