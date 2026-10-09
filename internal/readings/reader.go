package readings

import (
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
	return Between(r.files, from, to)
}

// Empty reads as a readings history that holds no reading, with no files to
// read: for one who reads the history where there's none to find.
type Empty struct{}

// Between returns no reading.
func (Empty) Between(time.Time, time.Time) iter.Seq[Reading] {
	return func(func(Reading) bool) {}
}
