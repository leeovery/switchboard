package watch

import (
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

func TestPlanAt(t *testing.T) {
	p := plan{interval: interval, due: at(13, 42, 0), next: at(13, 12, 5), reset: at(13, 21, 0), asked: at(13, 12, 0)}
	full := status.Read{Refresh: interval, Probe: true}
	tests := []struct {
		name   string
		now    time.Time
		routed bool
		want   status.Read
		wantOK bool
	}{
		{name: "reading the router, before the next look", now: at(13, 12, 4), routed: true},
		{name: "reading the router, a look once it's due, which never probes", now: at(13, 12, 5), routed: true, want: status.Read{}, wantOK: true},
		{name: "reading the router, a look having it refresh once a window on screen has reset", now: at(13, 21, 0), routed: true, want: status.Read{Refresh: status.FreshFor}, wantOK: true},
		{name: "reading the router, a refresh once it's due", now: at(13, 42, 0), routed: true, want: full, wantOK: true},
		{name: "probing, in the minute the router was last asked after", now: at(13, 12, 59), routed: false},
		{name: "probing, a question after the router in the next minute", now: at(13, 13, 0), routed: false, want: status.Read{Refresh: interval}, wantOK: true},
		{name: "probing, a probe once it's due", now: at(13, 42, 0), routed: false, want: full, wantOK: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := p.at(tt.now, tt.routed)
			if got != tt.want || ok != tt.wantOK {
				t.Errorf("at() = %+v, %v; want %+v, %v", got, ok, tt.want, tt.wantOK)
			}
		})
	}
}

func TestPlanLanded(t *testing.T) {
	now := at(13, 20, 0)
	before := plan{interval: interval, due: at(13, 42, 0), next: at(13, 12, 5), reset: at(13, 13, 0), asked: at(13, 12, 0), failures: 1}
	unread := routerDocument(account("work", "Work", session(0.25, 3*time.Hour), week(0.5)), unreadable("side", "Side"))
	// Of three's windows, side's session resets first, two hours on.
	threeReset := start.Add(2*time.Hour + resetGrace).UTC()
	tests := []struct {
		name string
		read status.Read
		doc  status.Document
		want plan
	}{
		{
			name: "a look at the router's document",
			read: status.Read{},
			doc:  routerDocument(three()...),
			want: plan{interval: interval, due: at(13, 42, 0), next: now.Add(lookEvery), reset: threeReset, asked: now, failures: 1},
		},
		{
			name: "a look at the router's document, with a window that has reset",
			read: status.Read{},
			doc:  routerDocument(account("work", "Work", session(0.25, -time.Hour), week(0.5))),
			want: plan{interval: interval, due: at(13, 42, 0), next: now.Add(lookEvery), reset: start.Add(-time.Hour + resetGrace).UTC(), asked: now, failures: 1},
		},
		{
			name: "the router refreshing",
			read: status.Read{Refresh: interval, Probe: true},
			doc:  routerDocument(three()...),
			want: plan{interval: interval, due: now.Add(interval), next: now.Add(lookEvery), reset: threeReset, asked: now},
		},
		{
			name: "the router refreshing what it hasn't read in the last minute",
			read: status.Read{Refresh: status.FreshFor, Probe: true},
			doc:  routerDocument(account("work", "Work", session(0.25, -time.Hour), week(0.5))),
			want: plan{interval: interval, due: now.Add(interval), next: now.Add(lookEvery), reset: start.Add(3*day + resetGrace).UTC(), fresh: now, asked: now},
		},
		{
			name: "the router refreshing, and failing to read an account",
			read: status.Read{Refresh: interval},
			doc:  unread,
			want: plan{interval: interval, due: now.Add(2 * retryReadAfter), next: now.Add(lookEvery), reset: start.Add(3*time.Hour + resetGrace).UTC(), asked: now, failures: 2},
		},
		{
			name: "probing",
			read: status.Read{Probe: true},
			doc:  probedWithoutTheRouter(),
			want: plan{interval: interval, due: now.Add(interval), next: at(13, 12, 5), reset: at(13, 13, 0), asked: now},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := before.landed(tt.read, tt.doc, now); got != tt.want {
				t.Errorf("landed() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestPlanFailedAndMissed(t *testing.T) {
	now := at(13, 20, 0)
	before := plan{interval: interval, due: at(13, 42, 0), next: at(13, 12, 5), asked: at(13, 12, 0)}

	want := plan{interval: interval, due: now.Add(retryReadAfter), next: at(13, 12, 5), asked: now, failures: 1}
	if got := before.failed(calm(), now); got != want {
		t.Errorf("failed() = %+v, want %+v", got, want)
	}
	want = plan{interval: interval, due: at(13, 42, 0), next: now.Add(lookEvery), asked: now}
	if got := before.missed(now); got != want {
		t.Errorf("missed() = %+v, want %+v: asked after the router, and the next look lookEvery on", got, want)
	}
}
