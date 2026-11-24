package dashboard

import (
	"slices"
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
// them in, else short, or narrower still, where they have narrower forms,
// as shortIn picks them; and the figure they lead with in any, where they
// lead with one, an allowance or the pool's room.
type usageWords struct {
	full, short line
	// narrower are the forms narrower than short, the narrowest last: none
	// but a held window's, as heldBack has them.
	narrower []line
	leads    NumberKind
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
// within the week, as Dated shows it with its weekday, both within a day, as
// a run-out's has it, and two days on; a tokenless one's as its token is
// another subscription's, the wider; and an allowance at its most, the whole
// window in a day. A reading over a week back, said by its date, is wider:
// where its words don't fit, they're short, which says the date still.
var fullWidest = sync.OnceValue(func() int {
	now := time.Date(2026, time.September, 28, 12, 0, 0, 0, time.UTC)
	widest := 0
	for kind := status.WindowTokenless; kind <= status.WindowOpen; kind++ {
		for _, at := range []time.Time{now.Add(21 * time.Hour), now.AddDate(0, 0, 2).Add(9 * time.Hour), now.AddDate(0, 0, -6)} {
			w := status.WindowNow{Kind: kind, At: at, TokenForeign: true, Allowance: quota.Allowance{Share: 1, Per: quota.PerDay}}
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
// At say them: that its account has no token of its own, as prose.Tokenless
// says it; that its account has no such limit, or hasn't used a model's own,
// as Unused says, whatever holds its account; what holds it back, and until
// when; when it runs out; that it isn't started; or its allowance, to its
// reset. A time reads as Dated has it in full, and short, its time today and
// only its weekday for another day; but a reading's, as lastRead has it; a
// run-out's as RunOut has it in full; and a run-out's and a back's short as
// BriefDated dates them a week on. An allowance's reset keeps its weekday
// alone: its short cells can't hold a date beside the rate, and a rate isn't
// read as an end today.
func wordsOf(w status.WindowNow, now time.Time) usageWords {
	switch {
	case w.Kind == status.WindowTokenless:
		return alike(prose.Tokenless(w.TokenForeign), errorInk)
	case w.Kind == status.WindowUnread:
		return usageWords{full: line{{"not read yet", dimInk}}, short: line{{"not read", dimInk}}}
	case w.Kind == status.WindowNone:
		return lacked(status.InProse(w.Name))
	case w.Kind == status.WindowStale:
		return staleWords(w.Stale, w.At, now)
	case w.Unused:
		return unused(status.Headed(w.Name))
	case w.Kind == status.WindowLimited:
		return heldBack("limit", "limit", w.At, now, errorInk)
	case w.Kind == status.WindowCapped:
		return heldBack("at cap", "cap", w.At, now, warningInk)
	case w.Kind == status.WindowRunsOut:
		return usageWords{full: line{{"out ~" + prose.RunOut(now, w.At), warningInk}}, short: line{{"out ~" + prose.BriefDated(now, w.At), warningInk}}}
	case w.Kind == status.WindowNotStarted:
		return alike("not started", mutedInk)
	default:
		return allowed(w.Allowance, w.At, now)
	}
}

// lacked is the words saying an account has no limit in a window, named
// as a sentence names it, in text.subtle: "no 5-hour limit", and short, "no
// limit".
func lacked(window string) usageWords {
	return usageWords{full: line{{"no " + window + " limit", dimInk}}, short: line{{"no limit", dimInk}}}
}

// unused is the words saying an account hasn't used a model's own window,
// the model named as given, in text.subtle, on lacked's pattern: "no Fable
// use", and short, "no use".
func unused(model string) usageWords {
	return usageWords{full: line{{"no " + model + " use", dimInk}}, short: line{{"no use", dimInk}}}
}

// staleWords are the words of a window whose reading is stale, as cause says
// why, last read at: in state.destructive, its subscription ended or its
// token refused; else when it was last read, as prose.NotReadSince says it,
// in text.subtle.
func staleWords(cause status.StaleCause, at, now time.Time) usageWords {
	if full, short := cause.Said(); full != "" {
		return usageWords{full: line{{full, errorInk}}, short: line{{short, errorInk}}}
	}
	full, short := prose.NotReadSince(now, at)
	return usageWords{full: line{{full, dimInk}}, short: line{{short, dimInk}}}
}

// heldBack is the words of a window held back as what says in full, and
// word short, until back, zero where that's unknown: as in "at cap · back
// Mon 17:10", short, keeping what holds it, "cap · back Mon", as BriefDated
// shows the time; narrower, "cap · Mon", then "back Mon".
func heldBack(what, word string, back time.Time, now time.Time, k ink) usageWords {
	if back.IsZero() {
		return alike(what, k)
	}
	day := prose.BriefDated(now, back)
	return usageWords{
		full:     line{{what + " · " + status.Back(now, back), k}},
		short:    line{{word + " · back " + day, k}},
		narrower: []line{{{word + " · " + day, k}}, {{"back " + day, k}}},
	}
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
	narrower := make([]line, len(w.narrower))
	for i, l := range w.narrower {
		narrower[i] = inFaint(l)
	}
	w.narrower = narrower
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
// them: as fitted has them from 70 columns; else short, as shortIn has them.
func (w usageWords) pick(width, room int) line {
	if width < fullWords {
		return w.shortIn(room)
	}
	return w.fitted(room)
}

// fitted is the words in room cells: in full where they fit, else short, as
// shortIn has them.
func (w usageWords) fitted(room int) line {
	if w.full.width() <= room {
		return w.full
	}
	return w.shortIn(room)
}

// shortIn is the words short in room cells: the first of short and its
// narrower forms that fits, else the narrowest, whole spans left off its end
// where even that doesn't fit, or cut short.
func (w usageWords) shortIn(room int) line {
	forms := slices.Concat([]line{w.short}, w.narrower)
	for _, form := range forms[:len(forms)-1] {
		if form.width() <= room {
			return form
		}
	}
	return forms[len(forms)-1].fitWhole(room)
}

// percentage is the bar's use as the usage block shows it: as a whole
// percentage, in bold, in the ramp's last stop from 90%, its second from 70%,
// and its first below, judged on the percentage shown; on the Overview, of an
// account out of the pool, in bold text.faint, stale or not, as faint wins
// over any colour; a stale reading's in text.subtle, not bold; and of an
// empty bar, none, as — in text.faint.
func (b usageBar) percentage() line {
	if b.empty {
		return line{noFigure}
	}
	return line{{prose.Percent(b.used), figureInk(b.used, b.out, b.stale)}}
}

// figureInk is the ink a figure of a window's share used is shown in, as usage
// shows a use: of an account out of the pool, out, bold text.faint, stale or
// not, as faint wins over any colour; of a stale reading, text.subtle, not
// bold; else bold in its level's colour, as usedInk tones it.
func figureInk(used float64, out, stale bool) ink {
	switch {
	case out:
		return ink{token: theme.TextFaint, bold: true}
	case stale:
		return dimInk
	}
	return useInk(used)
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
