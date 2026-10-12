package watch

import (
	"time"

	"github.com/leeovery/switchboard/internal/dashboard"
)

// brightFor is how long a number that changes brightens, fading back as it
// goes.
const brightFor = time.Second

// numbers are when each number the page draws that a read or the request
// stream changed did, by the number it is, while it brightens, and where
// the last look read its document from.
type numbers struct {
	from  origin
	since map[dashboard.Number]time.Time
}

// looked takes in the numbers the page draws at now before and after a look
// from read, was and is, as changed does; but a look that doesn't follow on
// from the last, as follows says, brightens none: its numbers start afresh,
// as the cues do.
func (n numbers) looked(was, is map[dashboard.Number]string, read origin, now time.Time) numbers {
	if !read.follows(n.from) {
		return numbers{from: read}
	}
	n.from = read
	return n.changed(was, is, now)
}

// changed takes in the numbers the page draws at now as they were before a
// read or the request stream changed what they're drawn of, was, and as they
// are since, is: each in both that differs brightens from now, and each
// brightening already stays so while it does. Both are drawn at the one
// moment, so nothing the clock moves is a change, as a countdown, or a
// rate's last minute moving on; nor is a number in only one, appearing or
// going.
func (n numbers) changed(was, is map[dashboard.Number]string, now time.Time) numbers {
	n.since = lasting(n.since, now, brightFor)
	for key, drawn := range is {
		if before, ok := was[key]; ok && before != drawn {
			n.since[key] = now
		}
	}
	return n
}

// brightened is how bright each number changed is at now, by the number it
// is: from 1, just changed, to 0, faded back, as fading fades it over
// brightFor. It's nil when none brightens.
func (n numbers) brightened(now time.Time) map[dashboard.Number]float64 {
	bright := fading(n.since, now, brightFor)
	for key, faded := range bright {
		bright[key] = 1 - faded
	}
	return bright
}

// numbersDrawn are the numbers the page draws at now, as the model stands,
// by the number each is.
func (m Model) numbersDrawn(now time.Time) map[dashboard.Number]string {
	return m.frame(now).Numbers(m.doc, now)
}
