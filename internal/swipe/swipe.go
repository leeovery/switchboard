// Package swipe reads which way the Mac scrolls, as its scroll setting says:
// content following the fingers, natural, or the wheel, traditional.
package swipe

import (
	"bytes"
	"context"
	"time"
)

// Direction is which way the Mac scrolls. The zero Direction is Natural,
// macOS's own default.
type Direction int

const (
	// Natural scrolls content the way the fingers move, so fingers down on
	// the trackpad reach a terminal as the wheel turned up.
	Natural Direction = iota
	// Traditional scrolls content the way the wheel turns, so fingers down
	// reach a terminal as the wheel turned down.
	Traditional
)

// readTimeout bounds the read of the setting, which normally takes a
// moment, so the dashboard never waits long on it to open.
const readTimeout = 2 * time.Second

// Runner reads the setting, returning what it printed, as Defaults does.
type Runner func(ctx context.Context) ([]byte, error)

// Read is the way the Mac scrolls, as run reads it within readTimeout and
// Parse reads its answer: Natural where run is nil.
func Read(ctx context.Context, run Runner) Direction {
	if run == nil {
		return Natural
	}
	ctx, cancel := context.WithTimeout(ctx, readTimeout)
	defer cancel()
	return Parse(run(ctx))
}

// Parse is the way the Mac scrolls, as out, what reading the setting
// printed, and err, how the read ended, say: Traditional only for a read
// that printed 0; Natural for 1, the setting missing, which defaults exits
// non-zero for, and anything else, as macOS scrolls naturally by default.
func Parse(out []byte, err error) Direction {
	if err == nil && string(bytes.TrimSpace(out)) == "0" {
		return Traditional
	}
	return Natural
}

// Fingers is the way the fingers moved, -1 up or 1 down, for a turn of the
// wheel the way given, -1 up or 1 down, as the terminal reports it.
func (d Direction) Fingers(way int) int {
	if d == Natural {
		return -way
	}
	return way
}
