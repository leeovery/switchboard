package status

import "slices"

// AllNow is every account at a moment, taken together, as all accounts has
// them: whatever the global pin names, and whatever holds any of them back.
type AllNow struct {
	// Windows are a window each that the use of is known on any account, in
	// the order the owner reads windows listed, as CompareWindows has it.
	Windows []AllWindowNow
}

// AllWindowNow is a window of every account at a moment, taken together.
type AllWindowNow struct {
	WindowName
	// Of is how many accounts it counts: those whose use of it is known at
	// Now, a stale account's as last read among them; not one without the
	// window, without a usable token, or nothing read of it.
	Of int
	// Used is the mean of their use of it as it stands at Now; Room what each
	// has left of it, to its limit, summed in accounts' worth, 1.2 being an
	// account and a fifth; and Pace the mean of the even paces of those whose
	// window is running: nil where none is.
	Used, Room float64
	Pace       *float64
	// Stale is set where every account it counts is stale: its token refused,
	// or its last read failed.
	Stale bool
}

// All is every account at Now, taken together, as all accounts has them,
// each window as allWindowOf has it: zero with one account, which all
// accounts would only repeat.
func (m Moment) All() AllNow {
	if len(m.Accounts) < 2 {
		return AllNow{}
	}
	var keys []string
	for _, a := range m.Accounts {
		for _, w := range a.Windows {
			if !slices.Contains(keys, w.Key) {
				keys = append(keys, w.Key)
			}
		}
	}
	slices.SortFunc(keys, m.CompareWindows)
	var all AllNow
	for _, key := range keys {
		if w := allWindowOf(m.Accounts, key); w.Of > 0 {
			all.Windows = append(all.Windows, w)
		}
	}
	return all
}

// allWindowOf is the accounts' window with the given key, taken together, of
// those whose use of it is known: its Kind neither tokenless nor unread.
func allWindowOf(accounts []AccountNow, key string) AllWindowNow {
	var all AllWindowNow
	var used, paces float64
	var running, stale int
	for _, a := range accounts {
		w, ok := a.Window(key)
		if !ok || w.Kind == WindowTokenless || w.Kind == WindowUnread {
			continue
		}
		all.WindowName = w.WindowName
		all.Of++
		used += w.Used
		all.Room += max(0, 1-w.Used)
		if w.Pace != nil {
			paces, running = paces+*w.Pace, running+1
		}
		if w.Kind == WindowStale {
			stale++
		}
	}
	if all.Of == 0 {
		return all
	}
	all.Used, all.Stale = used/float64(all.Of), stale == all.Of
	if running > 0 {
		all.Pace = new(paces / float64(running))
	}
	return all
}
