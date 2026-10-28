package router

import (
	"maps"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/notify"
)

// toldYear is what's been told of an account's token's year: the token it was
// told of, as its SHA-256 hash, in hex, never the token, and the days of the
// nearest warning told, as status.TokenYear.Warned gives them. Every warning
// of as many days or more counts as told with it.
type toldYear struct {
	SHA256 string `json:"sha256"`
	Within int    `json:"within"`
}

// covers reports whether the warning of days counts as told of the token
// with the given hash.
func (t toldYear) covers(sum string, days int) bool {
	return t.SHA256 == sum && t.Within <= days
}

// yearsTold holds, by account, what's been told of its token's year, which
// the state file keeps, so a restart tells nothing twice and a new token
// starts afresh. It's safe for concurrent use.
type yearsTold struct {
	mu   sync.Mutex
	told map[string]toldYear
	// changed notes a change for the state file to keep.
	changed func()
}

func newYearsTold(changed func()) *yearsTold {
	return &yearsTold{told: make(map[string]toldYear), changed: changed}
}

// due returns the notices of the accounts' tokens' years that the accounts,
// as they stand at now, call for: of each token whose year ends within a
// warning's days not told of it, the nearest, which counts as told from then
// on, with every farther one, whether or not it posts.
func (y *yearsTold) due(accounts standings, now time.Time) []notify.Notice {
	y.mu.Lock()
	defer y.mu.Unlock()
	var due []notify.Notice
	for _, s := range accounts {
		within, warned := s.TokenYear().Warned(now)
		if !warned || y.told[s.ID].covers(s.yearOf, within) {
			continue
		}
		y.told[s.ID] = toldYear{SHA256: s.yearOf, Within: within}
		due = append(due, yearNotice(s, now))
	}
	if len(due) > 0 {
		y.changed()
	}
	return due
}

// recall takes in what the state file kept, as the router starts, but for
// the accounts configured reports false of, and reports whether it left any
// out.
func (y *yearsTold) recall(saved map[string]toldYear, configured func(id string) bool) bool {
	y.mu.Lock()
	defer y.mu.Unlock()
	y.told = make(map[string]toldYear, len(saved))
	for id, told := range saved {
		if configured(id) {
			y.told[id] = told
		}
	}
	return len(y.told) < len(saved)
}

// saved is what the state file keeps of what's been told.
func (y *yearsTold) saved() map[string]toldYear {
	y.mu.Lock()
	defer y.mu.Unlock()
	return maps.Clone(y.told)
}
