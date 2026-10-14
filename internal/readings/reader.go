package readings

import (
	"fmt"
	"io/fs"
	"iter"
	"log/slog"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
)

// Reader reads the readings history back where it lies, in its directory of
// the state directory, with no router.
type Reader struct {
	files *dayfile.Files
}

// NewReader returns a reader of the readings history in the state directory
// stateDir, what it can't read logged to logger.
func NewReader(stateDir string, logger *slog.Logger) *Reader {
	return &Reader{files: Files(Dir(stateDir), logger)}
}

// Between returns the readings the history holds of times from from up to
// to, one at a time, as Between gives them.
func (r *Reader) Between(from, to time.Time) iter.Seq[Reading] {
	return Between(r.files, from, to, nil)
}

// Tail returns a read of the readings from start up to end, each window's
// run from from up to start carried in, as CarriedRuns has them, for Runway:
// the first call reads them whole, and each after only what the files
// gained since, as a Day reads them.
func (r *Reader) Tail(from, start, end time.Time) func() []Reading {
	return newRunsDay(r.files, from, start, end).Read
}

// FirstDay returns the start of the first local day the history holds
// readings of, by the dates its files' names give. It fails with
// fs.ErrNotExist where it holds none.
func (r *Reader) FirstDay() (time.Time, error) {
	dates, err := r.files.All()
	switch {
	case err != nil:
		return time.Time{}, fmt.Errorf("list the readings history: %w", err)
	case len(dates) == 0:
		return time.Time{}, fs.ErrNotExist
	}
	start, _, _ := dayfile.Day(dates[0])
	return start, nil
}

// Empty reads as a readings history that holds no reading, with no files to
// read: for one who reads the history where there's none to find.
type Empty struct{}

// Between returns no reading.
func (Empty) Between(time.Time, time.Time) iter.Seq[Reading] {
	return func(func(Reading) bool) {}
}

// Tail returns a read that gives no reading.
func (Empty) Tail(time.Time, time.Time, time.Time) func() []Reading {
	return func() []Reading { return nil }
}

// FirstDay fails with fs.ErrNotExist, as the history holds no day.
func (Empty) FirstDay() (time.Time, error) {
	return time.Time{}, fs.ErrNotExist
}
