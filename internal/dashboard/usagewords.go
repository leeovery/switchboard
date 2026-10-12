package dashboard

import (
	"strconv"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
)

// usageWords are what a window's words say of it, in full and short, as the
// usage block draws them: the full form from fullWords columns, the short
// below.
type usageWords struct {
	full, short line
}

// fullWords is the fewest columns the usage block draws its words in full at.
const fullWords = 70

// The cells the words take in each form.
const (
	fullCells  = 22
	shortCells = 14
)

// alike is words that read the same in either form, in one ink.
func alike(words string, k ink) usageWords {
	l := line{{words, k}}
	return usageWords{full: l, short: l}
}

// wordsOf are the words of a window standing as st says at now, its
// allowance a: what holds it back, and until when; when it runs out; that it
// isn't started; or the allowance, to its reset. A time reads as Dated has it
// in full, and short, its time today and only its weekday for another day.
func wordsOf(st status.WindowState, a quota.Allowance, now time.Time) usageWords {
	switch st.Kind {
	case status.WindowTokenless:
		return alike("no token", errorInk)
	case status.WindowUnread:
		return usageWords{full: line{{"not read yet", dimInk}}, short: line{{"not read", dimInk}}}
	case status.WindowStale:
		return usageWords{full: line{{"not read since " + status.Dated(now, st.At), dimInk}}, short: line{{"read " + brief(now, st.At), dimInk}}}
	case status.WindowLimited:
		return heldBack("limit", st.At, now, errorInk)
	case status.WindowCapped:
		return heldBack("at cap", st.At, now, warningInk)
	case status.WindowRunsOut:
		return usageWords{full: line{{"out ~" + status.Dated(now, st.At), warningInk}}, short: line{{"out ~" + brief(now, st.At), warningInk}}}
	case status.WindowNotStarted:
		return alike("not started", mutedInk)
	default:
		return allowed(a, st.At, now)
	}
}

// heldBack is the words of a window held back as what says, until back, zero
// where that's unknown: as in "at cap · back 17:10", and short, "back 17:10".
func heldBack(what string, back time.Time, now time.Time, k ink) usageWords {
	if back.IsZero() {
		return alike(what, k)
	}
	return usageWords{full: line{{what + " · back " + status.Dated(now, back), k}}, short: line{{"back " + brief(now, back), k}}}
}

// allowed is the words of a window's allowance a, to its reset: as in
// "19%/day till Sun 21:00" or, where the reset comes within the allowance's
// span, the room itself, "34% till 16:30"; short, "19%/day · Sun". There are
// none where it has no allowance.
func allowed(a quota.Allowance, reset time.Time, now time.Time) usageWords {
	if a == (quota.Allowance{}) {
		return usageWords{}
	}
	rate := span{status.Percent(a.Share) + perWords[a.Per], mutedInk}
	return usageWords{
		full:  line{rate, {" till ", dimInk}, {status.Dated(now, reset), mutedInk}},
		short: line{rate, {" · ", dimInk}, {brief(now, reset), mutedInk}},
	}
}

// perWords say the span an allowance is given over, after its share.
var perWords = map[string]string{quota.PerHour: "/h", quota.PerDay: "/day"}

// poolWords are the words of the pool's window: the room its accounts have
// left in it, to a tenth, of how many there are, as in "1.5 of 3 left", and
// where pinned is set, " · pinned" after.
func poolWords(room float64, accounts int, pinned bool) usageWords {
	l := line{
		{strconv.FormatFloat(float64(halfEven(room*10))/10, 'f', 1, 64), strongInk},
		{" of " + strconv.Itoa(accounts) + " left", mutedInk},
	}
	if pinned {
		l = append(l, span{" · pinned", mutedInk})
	}
	return usageWords{full: l, short: l}
}

// pick is the words as a block width columns wide draws them, in room cells,
// those up to what follows them: in full from 70 columns, where they fit;
// else short, whole spans left off their end where even that doesn't fit, or
// cut short.
func (w usageWords) pick(width, room int) line {
	if width >= fullWords && w.full.width() <= room {
		return w.full
	}
	return w.short.fitWhole(room)
}

// percentage is the bar's use as the usage block shows it: as a whole
// percentage, in bold, in the ramp's last stop from 90%, its second from 70%,
// and its first below, judged on the percentage shown; a stale reading's in
// text.subtle, not bold; and of an empty bar, none, as — in text.faint.
func (b usageBar) percentage() line {
	shown := status.WholePercent(b.used)
	k := ink{token: theme.VizRamp1, bold: true}
	switch {
	case b.empty:
		return line{{"—", faintInk}}
	case b.stale:
		k = dimInk
	case shown >= 90:
		k.token = theme.VizRamp4
	case shown >= 70:
		k.token = theme.VizRamp2
	}
	return line{{strconv.Itoa(shown) + "%", k}}
}

// percentCells are the cells a window's percentage is right-aligned in.
const percentCells = 4
