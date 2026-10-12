package watch

import (
	"maps"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/notify"
	"github.com/leeovery/switchboard/internal/status"
)

// lastReads are the looks that last read each account, by its id, each at
// the moment it was taken. A look is compared with them rather than with the
// look before it, so an account that couldn't be read for a while is
// compared with how it last stood.
type lastReads map[string]status.Moment

// alerts are the notifications look calls for, of those settings asks for:
// each account that has room again since it was last read, as roomAgain has
// it, and each window that has passed the warning, as passed has it. An
// account is only compared with an earlier reading, so the first look calls
// for none.
func (r lastReads) alerts(look status.Moment, settings config.Notifications) []notify.Notice {
	var alerts []notify.Notice
	for _, a := range look.Accounts {
		last, ok := r[a.ID]
		if !ok || !a.Read {
			continue
		}
		if settings.Room && roomAgain(last, a) {
			alerts = append(alerts, notify.RoomAgain(a.ID, a.Title()))
		}
		for _, w := range passed(last.At(look.Now), a, settings.Warning) {
			alerts = append(alerts, notify.Warning(a.ID, a.Title(), w.Label, w.Used))
		}
	}
	return alerts
}

// with returns the readings updated with each account look read.
func (r lastReads) with(look status.Moment) lastReads {
	updated := make(lastReads, len(r))
	maps.Copy(updated, r)
	for _, a := range look.Accounts {
		if a.Read {
			updated[a.ID] = look
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
