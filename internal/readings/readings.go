// Package readings is the readings history's format: a line for each reading
// that changed how a window of an account reads, its use, its reset or its
// status, in files a day in a directory of the state directory, as
// internal/dayfile keeps them, and reading them back. The router writes the
// history and takes it up as it starts, and the request ledger summarises its
// days with it, whether or not the router runs.
package readings

import (
	"encoding/json"
	"iter"
	"log/slog"
	"math"
	"path/filepath"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/quota"
)

const (
	// dirName is the readings history's directory in the state directory.
	dirName = "history"
	// lineMax is the longest line of the history read back: a reading's is a
	// couple of hundred bytes, and a longer line, which no reading makes, is
	// skipped without being held.
	lineMax = 4096
)

// Source is where a reading came from, as the readings history gives it.
type Source string

const (
	// FromAnswer is a reading off the answer to a routed request.
	FromAnswer Source = "answer"
	// FromProbe is a probe's reading.
	FromProbe Source = "probe"
	// FromPrime is a prime's reading.
	FromPrime Source = "prime"
)

// Reading is a line of the readings history: a window of an account as read
// when its reading changed, by the window's key, and where the reading came
// from. It holds the account's id and nothing else of the account: never its
// token or label. Fields may be added to it, never renamed.
type Reading struct {
	At          time.Time    `json:"at"`
	Account     string       `json:"account"`
	Key         string       `json:"window"`
	Utilization float64      `json:"utilization"`
	ResetsAt    time.Time    `json:"resets_at,omitzero"`
	Status      quota.Status `json:"status,omitempty"`
	Source      Source       `json:"source"`
}

// Of returns windows of the account with the given id, read at a time from a
// source, as lines of the readings history.
func Of(id string, windows []quota.Window, at time.Time, from Source) []Reading {
	read := make([]Reading, len(windows))
	for i, w := range windows {
		read[i] = Reading{At: at, Account: id, Key: w.Key, Utilization: w.Utilization, ResetsAt: w.ResetsAt, Status: w.Status, Source: from}
	}
	return read
}

// Window is the window the reading read.
func (r Reading) Window() quota.Window {
	return quota.Window{Key: r.Key, Utilization: r.Utilization, ResetsAt: r.ResetsAt, Status: r.Status}
}

// valid reports whether the reading, as the history holds it, can be taken
// up: it names an account and a window, and reads a use that can be.
func (r Reading) valid() bool {
	return r.Account != "" && r.Key != "" && !r.At.IsZero() &&
		!math.IsNaN(r.Utilization) && !math.IsInf(r.Utilization, 0) && r.Utilization >= 0
}

// In returns the reading a line of the history holds, reporting false for one
// that doesn't read as a reading that can be taken up.
func In(line []byte) (Reading, bool) {
	var r Reading
	if json.Unmarshal(line, &r) != nil || !r.valid() {
		return Reading{}, false
	}
	return r, true
}

// Dir is the readings history's directory in the state directory stateDir.
func Dir(stateDir string) string {
	return filepath.Join(stateDir, dirName)
}

// Files are the readings history's files in dir, as dayfile keeps them, what
// can't be done with them logged to logger.
func Files(dir string, logger *slog.Logger) *dayfile.Files {
	return &dayfile.Files{Dir: dir, Prefix: "readings", Name: "readings history", LineMax: lineMax, Logger: logger}
}

// Between returns the readings files hold of times from from up to to, one at
// a time, as Read gives them: those of the local days from the day before
// from's to the day after to's, as dayfile.Dates gives them, as a change of
// time zone can file a reading under a day beside its own. It holds the files
// from pruning and compressing while it reads them.
func Between(files *dayfile.Files, from, to time.Time) iter.Seq[Reading] {
	return func(yield func(Reading) bool) {
		Read(files, dayfile.Dates(from, to), func(r Reading) bool {
			switch {
			case !r.At.Before(to):
				return false
			case r.At.Before(from):
				return true
			}
			return yield(r)
		})
	}
}

// Read hands take each reading the files of the local days with the given
// dates, oldest first, hold, in the order they were read, whatever day's file
// each is in, as dayfile.Read reads them, until take reports false, and
// returns how many of their lines were unread: too long to hold, or not a
// reading that can be taken up, which are passed over.
func Read(files *dayfile.Files, dates []string, take func(Reading) bool) (unread int) {
	return dayfile.Read(files, dates, In, readAt, take)
}

// readAt is when r was read.
func readAt(r Reading) time.Time {
	return r.At
}
