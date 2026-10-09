package main

import (
	"cmp"
	"fmt"
	"io"
	"iter"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/views"
)

const (
	// stopEndTurn is the stop reason of an answer that ended its turn as the
	// model chose.
	stopEndTurn = "end_turn"
	// stopToolUse is the stop reason of an answer that called a tool.
	stopToolUse = "tool_use"
	// anHour is how many minutes a turn ran for to have run over an hour.
	anHour = 60
	// unnamed stands for the model of a line that names none.
	unnamed = "(none named)"
)

// tally is what the turns check counts of the ledger's message lines.
type tally struct {
	// messages are the message lines read, sessionless those of them that
	// carry no session, which tell no turn, and unmeasured those that tell
	// one but whose thread length is unknown, their body unread.
	messages, sessionless, unmeasured int
	sessions                          int
	// requests are how many requests each turn held, a count a turn.
	requests []int
	// stops are the turns ended by each stop reason, canceled those ended by
	// a main-thread request canceled, and going those not ended.
	stops           map[string]int
	canceled, going int
	// passedOver are the side requests' end_turns, by the model each asked
	// for.
	passedOver map[string]int
	// otherwise are the main thread's answers that stopped other than at
	// tool_use, ending nothing, by stop reason.
	otherwise map[string]int
	// shorter are the turns whose first request is shorter than the main
	// thread before them, and overAnHour the turns ended that ran over an
	// hour.
	shorter, overAnHour int
}

// count returns the tally of lines, oldest first by arrival: each session's
// turns told from its message lines, as views.Turns tells them.
func count(lines iter.Seq[ledger.Line]) tally {
	c := tally{stops: map[string]int{}, passedOver: map[string]int{}, otherwise: map[string]int{}}
	sessions := map[string][]ledger.Line{}
	for line := range lines {
		if line.Kind != ledger.KindMessage {
			continue
		}
		c.messages++
		if line.Session == "" {
			c.sessionless++
			continue
		}
		sessions[line.Session] = append(sessions[line.Session], bare(line))
	}
	c.sessions = len(sessions)
	for _, lines := range sessions {
		c.add(views.Turns(lines))
	}
	return c
}

// bare returns of line only what telling and counting its turn reads, as the
// whole ledger's lines are held at once.
func bare(line ledger.Line) ledger.Line {
	return ledger.Line{
		At:       line.At,
		Kind:     line.Kind,
		Model:    line.Model,
		Canceled: line.Canceled,
		TotalMS:  line.TotalMS,
		Shape:    ledger.Shape{Messages: line.Shape.Messages},
		Answer:   ledger.Answer{Stop: line.Answer.Stop},
	}
}

// add counts a session's turns, oldest first.
func (c *tally) add(turns []views.Turn) {
	for i, turn := range turns {
		c.requests = append(c.requests, len(turn.Requests))
		c.ending(turn)
		c.perRequest(turn)
		if i > 0 && startsShorter(turn, turns[i-1]) {
			c.shorter++
		}
		if !turn.Going() && took(turn) > anHour {
			c.overAnHour++
		}
	}
}

// ending counts how turn ended.
func (c *tally) ending(turn views.Turn) {
	switch {
	case turn.Going():
		c.going++
	case turn.Canceled:
		c.canceled++
	default:
		c.stops[turn.Stop]++
	}
}

// perRequest counts the requests of turn whose thread length is unknown, the
// end_turns of those off its main thread, and the stops of those on it that
// end nothing, but tool_use, which leaves nearly every turn going.
func (c *tally) perRequest(turn views.Turn) {
	for _, r := range turn.Requests {
		stop := r.Answer.Stop
		switch {
		case r.Shape.Messages == 0:
			c.unmeasured++
		case !r.Main && stop == stopEndTurn:
			c.passedOver[cmp.Or(r.Model, unnamed)]++
		case r.Main && !r.Canceled && stop != "" && stop != stopToolUse && !views.Ends(stop):
			c.otherwise[stop]++
		}
	}
}

// startsShorter reports whether turn's first request, of a length known, is
// shorter than the main thread of the turn before it as it last stood.
func startsShorter(turn, before views.Turn) bool {
	first := turn.Requests[0].Shape.Messages
	return first > 0 && first < before.Thread()
}

// took returns how long a turn ended ran, as its page's took gives it: in
// minutes, from the minute it started to the one it ended in.
func took(turn views.Turn) int {
	return int(turn.Ended.Truncate(time.Minute).Sub(turn.Started.Truncate(time.Minute)) / time.Minute)
}

// write prints c to w, labelled, with what the reader warned of: counts
// alone, never anything of a line but its stop reasons and models.
func write(w io.Writer, c tally, warned *hush) error {
	least, median, most := spread(c.requests)
	_, err := fmt.Fprintf(w, `message lines read: %d
  without a session, left out: %d
  without a thread length: %d
the reader's warnings: %d
  of lines it couldn't read: %d

sessions: %d
turns: %d
requests a turn: least %d, median %s, most %d

turns ended by stop reason:
%s
turns ended by cancelling: %d
turns still going: %d

main-thread answers that stopped otherwise, but at tool_use, ending nothing, by stop reason:
%s
side requests' end_turns passed over, by model:
%s
turns whose first request is shorter than the main thread before them: %d
turns ended that ran over an hour: %d
`,
		c.messages, c.sessionless, c.unmeasured, warned.warnings.Load(), warned.unread.Load(),
		c.sessions, len(c.requests), least, median, most,
		listed(c.stops), c.canceled, c.going,
		listed(c.otherwise), listed(c.passedOver), c.shorter, c.overAnHour)
	return err
}

// spread returns the least, the median and the most of counts: each 0 for
// none, and the median the mean of the middle two of an even number.
func spread(counts []int) (least int, median string, most int) {
	if len(counts) == 0 {
		return 0, "0", 0
	}
	sorted := slices.Sorted(slices.Values(counts))
	mid := len(sorted) / 2
	middle := float64(sorted[mid])
	if len(sorted)%2 == 0 {
		middle = float64(sorted[mid-1]+sorted[mid]) / 2
	}
	return sorted[0], strconv.FormatFloat(middle, 'f', -1, 64), sorted[len(sorted)-1]
}

// listed returns counts a line each, indented, in the order of their names,
// or a line saying there are none.
func listed(counts map[string]int) string {
	if len(counts) == 0 {
		return "  none\n"
	}
	var lines strings.Builder
	for _, name := range slices.Sorted(maps.Keys(counts)) {
		fmt.Fprintf(&lines, "  %s: %d\n", name, counts[name])
	}
	return lines.String()
}
