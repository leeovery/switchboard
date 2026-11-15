package idmove

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
)

// historyIn is the history's files in the state directory stateDir, opened
// afresh, by now's clock, as where no router keeps them.
func historyIn(stateDir string, now func() time.Time) History {
	files := readings.Files(readings.Dir(stateDir), logger)
	return History{
		Ledger:   ledger.Open(ledger.Dir(stateDir), dayfile.Forever, now, ledger.HistoryIn(files), ledger.Caps{}, logger),
		Readings: files,
		Events:   events.Files(ledger.Dir(stateDir), logger),
	}
}

// rewriteHistory writes the history in h anew, as rename has it, each line
// as rewritingLines rewrites it: the readings history first, which the
// request ledger's summaries read, then the router's events, then the
// ledger's days, a day's lines with its summary. A kind of file whose lines
// rename leaves as they are is passed over. stopped is told of each day's
// files once they're written, by step, then their name and their date. It
// stops, failing, once ctx ends.
func rewriteHistory(ctx context.Context, h History, rename ledger.Renaming, step string, stopped func(string) error) error {
	for _, kind := range []struct {
		files *dayfile.Files
		names names
	}{{h.Readings, readingNames}, {h.Events, eventNames}} {
		if !kind.names.rewrittenBy(rename) {
			continue
		}
		if err := rewriteAll(ctx, kind.files, rewritingLines(kind.names, rename), step, stopped); err != nil {
			return err
		}
	}
	dates, err := ledgerDates(h.Ledger)
	if err != nil {
		return err
	}
	for _, date := range dates {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := h.Ledger.RenameDay(date, rewritingLines(lineNames, rename), rename); err != nil {
			return err
		}
		if err := stopped(step + h.Ledger.Files().Name + " " + date); err != nil {
			return err
		}
	}
	return nil
}

// rewriteAll writes each day's files anew, each line as line makes it,
// telling stopped of each, as rewriteHistory says. It stops, failing, once
// ctx ends.
func rewriteAll(ctx context.Context, files *dayfile.Files, line func([]byte) []byte, step string, stopped func(string) error) error {
	dates, err := files.All()
	if err != nil {
		return fmt.Errorf("move the %s: %w", files.Name, err)
	}
	for _, date := range dates {
		if err := ctx.Err(); err != nil {
			return err
		}
		if _, err := files.Rewrite(date, line); err != nil {
			return err
		}
		if err := stopped(step + files.Name + " " + date); err != nil {
			return err
		}
	}
	return nil
}

// ledgerDates are the dates of the days the ledger holds lines or a summary
// of, oldest first, each once.
func ledgerDates(l *ledger.Ledger) ([]string, error) {
	lined, err := l.Files().All()
	if err != nil {
		return nil, fmt.Errorf("move the request ledger: %w", err)
	}
	summarised, err := ledger.SummaryDates(l.Files().Dir)
	if err != nil {
		return nil, fmt.Errorf("move the request ledger: %w", err)
	}
	return sortedOnce(append(lined, summarised...)), nil
}

// rewritingLines rewrites a line as rename has it, n saying where it names
// accounts and gives its pool: each account it names renamed, then, where it
// names no pool, given the pool rename gives the work of the requests that
// named none.
func rewritingLines(n names, rename ledger.Renaming) func([]byte) []byte {
	return func(line []byte) []byte {
		if rename.Account != nil {
			line, _ = n.renamed(line, rename.Account)
		}
		if rename.Unpooled != "" {
			line, _ = n.pool.pooled(line, rename.Unpooled)
		}
		return line
	}
}

// readClock reads now, else the system's clock.
func readClock(now func() time.Time) time.Time {
	if now == nil {
		return time.Now()
	}
	return now()
}

// drawnFrom is what random ids are drawn from: r, else crypto/rand's.
func drawnFrom(r io.Reader) io.Reader {
	if r == nil {
		return rand.Reader
	}
	return r
}

// markDone marks a move done in the state directory stateDir, writing the
// time given, in UTC, to the file of the given name, as Moved and PoolsMoved
// read it.
func markDone(stateDir, name string, at time.Time) error {
	return atomicfile.Write(filepath.Join(stateDir, name), []byte(at.UTC().Format(time.RFC3339)+"\n"), 0o600)
}
