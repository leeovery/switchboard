package watch

import (
	"maps"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/notify"
	"github.com/leeovery/switchboard/internal/status"
)

// lastReads are the looks that last read each account, by its id, each at
// the moment it was taken, and shared by every account it last read, and
// where the last look read its document from. A look is compared with them
// rather than with the look before it, so an account that couldn't be read
// for a while is compared with how it last stood.
type lastReads struct {
	from  origin
	looks map[string]*status.Moment
}

// alerts are the notifications look, read from read, calls for, of those
// settings asks for: each account that has room again since it was last
// read, as roomAgain has it, and each window that has passed the warning, as
// passed has it. An account is only compared with an earlier reading, so the
// first look calls for none; nor does a look that doesn't follow on from the
// last, as follows says, as a reading from elsewhere, as the router's limit
// that probing can't see, says nothing of how this one has changed. The
// warnings take each last look to look's moment once, for every account it
// last read to share, and none where settings warn of none.
func (r lastReads) alerts(look status.Moment, read origin, settings config.Notifications) []notify.Notice {
	if !read.follows(r.from) {
		return nil
	}
	warns, then := notify.Warns(settings.Warning), takenTo(look.Now)
	var alerts []notify.Notice
	for _, a := range look.Accounts {
		last, ok := r.looks[a.ID]
		if !ok || !a.Read {
			continue
		}
		if settings.Room && roomAgain(*last, a) {
			alerts = append(alerts, notify.RoomAgain(a.ID, a.Title()))
		}
		if !warns {
			continue
		}
		for _, w := range passed(then(last), a, settings.Warning) {
			alerts = append(alerts, notify.Warning(a.ID, a.Title(), w.Label, w.Used))
		}
	}
	return alerts
}

// takenTo takes a look to the moment now, as At has it: each look once, the
// first time it's asked, as it's asked again for every account it last read.
func takenTo(now time.Time) func(look *status.Moment) status.Moment {
	taken := make(map[*status.Moment]status.Moment)
	return func(look *status.Moment) status.Moment {
		m, ok := taken[look]
		if !ok {
			m = look.At(now)
			taken[look] = m
		}
		return m
	}
}

// with returns the readings updated with each account look reads, look read
// from read: those before are kept only where it follows on from them, as
// follows says, and otherwise they start afresh from it.
func (r lastReads) with(look status.Moment, read origin) lastReads {
	updated := lastReads{from: read, looks: make(map[string]*status.Moment)}
	if read.follows(r.from) {
		maps.Copy(updated.looks, r.looks)
	}
	for _, a := range look.Accounts {
		if a.Read {
			updated.looks[a.ID] = &look
		}
	}
	return updated
}

// roomAgain reports whether account a, as a look has it, has room again
// since last, the look that last read it: its quota had run out then, and it
// can take a request now, its token not refused. A refusal isn't quota, so
// one lifting is no news. Each look is judged at its own moment: judged now,
// a window spent then that has since reset would count as having had room,
// and the account's return would pass unannounced.
func roomAgain(last status.Moment, a status.AccountNow) bool {
	was, _ := last.Account(a.ID)
	return was.OutOfQuota && a.CanRise
}

// passed lists the windows of account a, as a look has it, that have passed
// warning since then, the look that last read it, taken to the same moment:
// each window, in both, as it stands at that moment, so one that has reset
// since either was read counts from empty. Passing warning in its period
// after the reset is news, however much of the period before it had used; a
// reading still of the period before, as one taken before the window is read
// again carries, is none.
func passed(then status.Moment, a status.AccountNow, warning float64) []status.WindowNow {
	was, _ := then.Account(a.ID)
	var up []status.WindowNow
	for _, w := range a.Windows {
		if before, ok := was.Window(w.Key); ok && notify.Passed(before.Used, w.Used, warning) {
			up = append(up, w)
		}
	}
	return up
}
