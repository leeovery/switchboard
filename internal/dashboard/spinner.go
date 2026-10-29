package dashboard

import "time"

// spinStep is how long the spinner's dot stays in each of its places.
const spinStep = 250 * time.Millisecond

// spinAt are the places the spinner's dot takes in turn, among its three
// cells: along them and back.
var spinAt = []int{0, 1, 2, 1}

// waiting is what's said while something is under way, at now: a spinner,
// its dot bold accent.mode in the place now has it at, its other cells
// text.faint, then two cells on, the words, bold text.primary.
func waiting(now time.Time, words string) line {
	at := spinAt[now.UnixMilli()/spinStep.Milliseconds()%int64(len(spinAt))]
	l := make(line, 0, 5)
	for i := range 3 {
		if i == at {
			l = append(l, span{"●", nameInk})
		} else {
			l = append(l, span{"∙", faintInk})
		}
	}
	return append(l, span{"  ", faintInk}, span{words, titleInk})
}

// spinDelay is how long from now until a spinner moves on: just past its
// dot's next place, as TickSlack has the clock's ticks.
func spinDelay(now time.Time) time.Duration {
	step := spinStep.Milliseconds()
	next := time.UnixMilli((now.UnixMilli()/step + 1) * step)
	return next.Sub(now) + TickSlack
}
