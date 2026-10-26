package dashboard

import (
	"strconv"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// usageWords are what a window's words say of it, in full and short, as the
// usage block draws them: in full wherever they fit the cells its row lays
// them in, else short; and the figure they lead with in either, where they
// lead with one, an allowance or the pool's room.
type usageWords struct {
	full, short line
	leads       NumberKind
}

// fullWords is the fewest columns the views that draw usage's gauges in rows
// of their own draw its words in full at, as pick picks them.
const fullWords = 70

// The cells the words take in each form: a row of the usage block lays them
// out at the full form's, narrowing toward the short's.
const (
	fullCells  = 22
	shortCells = 14
)

// widestWords are the most cells the block's words take in the form it
// draws them in, as wordCells chooses it: in full, as fullWidest has them;
// short, shortCells.
func (b usageBlock) widestWords() int {
	if b.wordCells() == fullCells {
		return fullWidest()
	}
	return shortCells
}

// fullWidest are the most cells a window's words take in full: the widest of
// every kind wordsOf words, each with its time at its widest, on another day
// within the week, as Dated shows it with its weekday, and an allowance at
// its most, the whole window in a day. A reading over a week back, said by
// its date, is wider: where its words don't fit, they're short, which says
// the date still.
var fullWidest = sync.OnceValue(func() int {
	now := time.Date(2026, time.September, 28, 12, 0, 0, 0, time.UTC)
	widest := 0
	for kind := status.WindowTokenless; kind <= status.WindowOpen; kind++ {
		for _, at := range []time.Time{now.AddDate(0, 0, 2).Add(9 * time.Hour), now.AddDate(0, 0, -6)} {
			w := status.WindowNow{Kind: kind, At: at, Allowance: quota.Allowance{Share: 1, Per: quota.PerDay}}
			widest = max(widest, wordsOf(w, now).full.width())
		}
	}
	return widest
})

// alike is words that read the same in either form, in one ink.
func alike(words string, k ink) usageWords {
	l := line{{words, k}}
	return usageWords{full: l, short: l}
}

// wordsOf are the words of the window w as it stands at now, as its Kind and
// At say them: what holds it back, and until when; when it runs out; that it
// isn't started; or its allowance, to its reset. A time reads as Dated has it
// in full, and short, its time today and only its weekday for another day;
// but a reading's, as lastRead has it.
func wordsOf(w status.WindowNow, now time.Time) usageWords {
	switch w.Kind {
	case status.WindowTokenless:
		return alike("no token", errorInk)
	case status.WindowUnread:
		return usageWords{full: line{{"not read yet", dimInk}}, short: line{{"not read", dimInk}}}
	case status.WindowStale:
		full, short := lastRead(now, w.At)
		return usageWords{full: line{{"not read since " + full, dimInk}}, short: line{{"read " + short, dimInk}}}
	case status.WindowLimited:
		return heldBack("limit", w.At, now, errorInk)
	case status.WindowCapped:
		return heldBack("at cap", w.At, now, warningInk)
	case status.WindowRunsOut:
		return usageWords{full: line{{"out ~" + prose.Dated(now, w.At), warningInk}}, short: line{{"out ~" + prose.Brief(now, w.At), warningInk}}}
	case status.WindowNotStarted:
		return alike("not started", mutedInk)
	default:
		return allowed(w.Allowance, w.At, now)
	}
}

// lastRead shows when a reading was taken, at t, before now, in full and
// short: within the week, as Dated and Brief show a time; else by its date,
// as Past shows a day over a week back, "22 Sep 14:20", and short, "22 Sep",
// as a weekday alone would read as this week's.
func lastRead(now, t time.Time) (full, short string) {
	if prose.WithinWeek(now, t) {
		return prose.Dated(now, t), prose.Brief(now, t)
	}
	return prose.Past(now, t), prose.DayMonth(t.In(now.Location()))
}

// heldBack is the words of a window held back as what says, until back, zero
// where that's unknown: as in "at cap · back 17:10", and short, "back 17:10",
// as Brief shows the time.
func heldBack(what string, back time.Time, now time.Time, k ink) usageWords {
	if back.IsZero() {
		return alike(what, k)
	}
	return usageWords{full: line{{what + " · " + status.Back(now, back), k}}, short: line{{"back " + prose.Brief(now, back), k}}}
}

// allowed is the words of a window's allowance a, to its reset: as in
// "19%/day till Sun 21:00" or, where the reset comes within the allowance's
// span, the room itself, "34% till 16:30"; short, "19%/day · Sun". There are
// none where it has no allowance.
func allowed(a quota.Allowance, reset time.Time, now time.Time) usageWords {
	if a == (quota.Allowance{}) {
		return usageWords{}
	}
	rate := span{prose.Percent(a.Share) + perWords[a.Per], mutedInk}
	return usageWords{
		full:  line{rate, {" till ", dimInk}, {prose.Dated(now, reset), mutedInk}},
		short: line{rate, {" · ", dimInk}, {prose.Brief(now, reset), mutedInk}},
		leads: NumberAllowance,
	}
}

// perWords say the span an allowance is given over, after its share.
var perWords = map[string]string{quota.PerHour: "/h", quota.PerDay: "/day"}

// poolWords are the words of the pool's window: the room its accounts have
// left in it, to a tenth, of how many there are, as in "1.5 of 3 left", and
// where pinned is set, " · pinned" after.
func poolWords(room float64, accounts int, pinned bool) usageWords {
	l := line{
		{prose.Tenths(room), strongInk},
		{" of " + strconv.Itoa(accounts) + " left", mutedInk},
	}
	if pinned {
		l = append(l, pinnedSaid)
	}
	return usageWords{full: l, short: l, leads: NumberRoom}
}

// pinnedSaid is what the pool's words say after its room where a pin holds
// it.
var pinnedSaid = span{" · pinned", mutedInk}

// faint is the words in text.faint, whatever their colours, as the Overview
// words a window of an account out of the pool.
func (w usageWords) faint() usageWords {
	w.full, w.short = inFaint(w.full), inFaint(w.short)
	return w
}

// inFaint is the line in text.faint, span for span.
func inFaint(l line) line {
	faint := make(line, len(l))
	for i, s := range l {
		faint[i] = span{s.text, faintInk}
	}
	return faint
}

// pick is the words as a view drawing usage's gauges in rows of its own
// draws them width columns wide, in room cells, those up to what follows
// them: as fitted has them from 70 columns; else short, whole spans left off
// their end where even that doesn't fit, or cut short.
func (w usageWords) pick(width, room int) line {
	if width < fullWords {
		return w.short.fitWhole(room)
	}
	return w.fitted(room)
}

// fitted is the words in room cells: in full where they fit, else short,
// whole spans left off their end where even that doesn't fit, or cut short.
func (w usageWords) fitted(room int) line {
	if w.full.width() <= room {
		return w.full
	}
	return w.short.fitWhole(room)
}

// percentage is the bar's use as the usage block shows it: as a whole
// percentage, in bold, in the ramp's last stop from 90%, its second from 70%,
// and its first below, judged on the percentage shown; on the Overview, of an
// account out of the pool, in bold text.faint, stale or not, as faint wins
// over any colour; a stale reading's in text.subtle, not bold; and of an
// empty bar, none, as — in text.faint.
func (b usageBar) percentage() line {
	var k ink
	switch {
	case b.empty:
		return line{noFigure}
	case b.out:
		k = ink{token: theme.TextFaint, bold: true}
	case b.stale:
		k = dimInk
	default:
		k = usedInk(b.used, ink{token: theme.VizRamp1, bold: true})
	}
	return line{{prose.Percent(b.used), k}}
}

// usedInk is the ink of a share used, as usage tones it, judged on the whole
// percentage prose.Percent shows: from 90%, the ramp's last stop, from 70%,
// its second, else below's.
func usedInk(used float64, below ink) ink {
	switch shown := prose.WholePercent(used); {
	case shown >= 90:
		below.token = theme.VizRamp4
	case shown >= 70:
		below.token = theme.VizRamp2
	}
	return below
}

// percentCells are the cells a window's percentage is right-aligned in.
const percentCells = 4
