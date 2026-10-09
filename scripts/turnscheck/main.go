// Command turnscheck checks how a session's turns are told from its lines,
// against the request ledger of the machine it runs on: it prints counts
// alone, never anything of a request. The owner runs it by hand, through
// scripts/turns-check; no test or agent does.
package main

import (
	"context"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"os"
	"sync/atomic"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/ledger"
)

func main() {
	if err := run(os.Stdout, os.Getenv, os.UserHomeDir, time.Now); err != nil {
		fmt.Fprintln(os.Stderr, "turns-check:", err)
		os.Exit(1)
	}
}

// run prints to out the tally of the request ledger's lines, every one it
// holds until now, in the state directory switchboard finds by getenv and
// homeDir.
func run(out io.Writer, getenv func(string) string, homeDir func() (string, error), now func() time.Time) error {
	stateDir, err := config.StateDir(getenv, homeDir)
	if err != nil {
		return err
	}
	warned := &hush{}
	reader := ledger.NewReader(stateDir, now, ledger.Caps{}, slog.New(warned))
	return write(out, count(linesOf(reader.Lines(time.Time{}))), warned)
}

// linesOf returns the lines held, as they read.
func linesOf(held iter.Seq[ledger.Held]) iter.Seq[ledger.Line] {
	return func(yield func(ledger.Line) bool) {
		for h := range held {
			if !yield(h.Line) {
				return
			}
		}
	}
}

// hush handles the reader's logs: it prints nothing, as a warning names a day
// or a file, but counts the warnings, and the lines they tell went unread.
type hush struct {
	warnings, unread atomic.Int64
}

func (h *hush) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelWarn
}

func (h *hush) Handle(_ context.Context, r slog.Record) error {
	h.warnings.Add(1)
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == "lines" && a.Value.Kind() == slog.KindInt64 {
			h.unread.Add(a.Value.Int64())
		}
		return true
	})
	return nil
}

func (h *hush) WithAttrs([]slog.Attr) slog.Handler { return h }

func (h *hush) WithGroup(string) slog.Handler { return h }
