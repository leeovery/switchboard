package watch

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
)

func TestOnceReadsTheRoutersDocumentSessionsAndHistory(t *testing.T) {
	doc := routerDocument(account("work", "Work", session(0.25, 3*time.Hour), week(0.5)))
	sessionStart := start.Add(-2 * time.Hour)
	source := &fakeSource{
		router:   &doc,
		sessions: []status.Session{{ID: "d28c5e17", Assignments: []status.Assignment{{Account: "work", LastSeen: start.UTC()}}}},
		history:  map[string]router.History{"5h": historyOf("work", sessionStart, 0.1, 0.2)},
	}

	snap, err := Once(context.Background(), source, Fresh())
	if err != nil {
		t.Fatalf("Once() error = %v", err)
	}
	if snap.Doc.Source != status.SourceRouter || len(snap.Sessions) != 1 || snap.Outdated {
		t.Errorf("Once() = %+v, want the router's document and its session", snap)
	}
	if want := []Read{Fresh()}; !slices.Equal(source.asked, want) {
		t.Errorf("Once() asked for %+v, want %+v, once", source.asked, want)
	}
	if want := []string{"5h 5m0s", "7d 30m0s"}; !slices.Equal(source.histories, want) {
		t.Errorf("Once() asked for the history of %q, want %q: every window, once, at its step", source.histories, want)
	}
	if got := snap.History[dashboard.Ref{Account: "work", Window: "5h"}].Readings; len(got) != 2 {
		t.Errorf("work's session has the readings %+v, want the router's two", got)
	}
}

func TestOnceProbingReadsNoHistory(t *testing.T) {
	source := &fakeSource{doc: probedWithoutTheRouter()}

	snap, err := Once(context.Background(), source, Read{Probe: true})
	if err != nil || snap.Doc.Source != status.SourceProbe {
		t.Fatalf("Once() = %+v, %v, want the document probed", snap, err)
	}
	if len(source.histories) > 0 || len(snap.History) > 0 || snap.Sessions != nil {
		t.Errorf("Once() asked for the history of %q, giving %+v, and sessions %+v; want none of either, probing", source.histories, snap.History, snap.Sessions)
	}
}

func TestOnceSaysWhenTheRouterIsFromBeforeItsHistory(t *testing.T) {
	doc := routerDocument(three()...)
	source := &fakeSource{router: &doc, historyErr: fmt.Errorf("ask the router: %w", router.ErrNoHistory)}

	snap, err := Once(context.Background(), source, Read{Probe: true})
	if err != nil || !snap.Outdated || len(snap.History) > 0 {
		t.Errorf("Once() = %+v, %v; want the router's document, outdated, without history", snap, err)
	}
	if len(source.histories) != 1 {
		t.Errorf("Once() asked for the history of %q, want one window asked, the router having none", source.histories)
	}
}

func TestOnceFailsAsItsReadDoes(t *testing.T) {
	failed := errors.New("connection refused")
	source := &fakeSource{err: failed}

	if _, err := Once(context.Background(), source, Read{Probe: true}); !errors.Is(err, failed) {
		t.Errorf("Once() error = %v, want %v", err, failed)
	}
	if _, err := Once(context.Background(), &fakeSource{}, Read{}); !errors.Is(err, ErrNoRouter) {
		t.Errorf("Once(), not to probe, without the router, error = %v, want ErrNoRouter", err)
	}
}
