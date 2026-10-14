package watch

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// A look on the Log whose tail read gains no line costs what one does of a
// Log holding a few lines, however many it holds, as it makes its Requests
// again only as the lines and figures they're made of change: in the bytes
// it allocates, a tenth more at most.
func TestALookOnTheLogCostsNoMoreForTheLinesItHolds(t *testing.T) {
	few, many := lookOnTheLog(t, 20), lookOnTheLog(t, 2000)
	if many > few+few/10 {
		t.Errorf("a look on the Log holding 2,000 lines allocates %d bytes, one holding 20 %d: its Requests are made again at each look", many, few)
	}
}

// lookOnTheLog is the bytes a look allocates, on the Log, the router
// answering, of a ledger whose lines today are as many as given, none gained.
func lookOnTheLog(t *testing.T, lines int) uint64 {
	t.Helper()
	state := t.TempDir()
	held := make([]ledger.Held, lines)
	for i := range held {
		held[i] = arrived(fmt.Sprintf("%08x", i), "work", -time.Duration(lines-i)*time.Second)
	}
	writeLines(t, state, held...)
	doc := routerDocument(three()...)
	h := routedHarness(t, doc)
	onTheLog(h, state)
	h.start()
	return lookBytes(h, doc, 10)
}

// lookBytes is the bytes each of runs looks reading doc allocates, a look
// first, as the Log's first look builds what later ones hold.
func lookBytes(h *harness, doc status.Document, runs int) uint64 {
	h.read(doc)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for range runs {
		h.read(doc)
	}
	runtime.ReadMemStats(&after)
	return (after.TotalAlloc - before.TotalAlloc) / uint64(runs)
}
