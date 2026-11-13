package status_test

import (
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/status"
)

// Acme, q7mt, holds work; Default, b2n8, the first pool, holds side; the
// accounts record keeps lapsed, removed from Acme, and spare, removed from no
// pool, as before pools.
const (
	acme      = "q7mt"
	firstPool = "b2n8"
)

var (
	pooledAccounts = config.Accounts{{ID: "work", Pool: acme}, {ID: "side", Pool: firstPool}}
	keptRecord     = identity.Record{Accounts: []identity.Entry{
		{ID: "lapsed", Label: "lapsed", Pool: acme, Removed: time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)},
		{ID: "spare", Label: "spare", Removed: time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)},
	}}
)

func TestAnEventIsOfThePoolItRecordedElseOfItsAccountsElseOfEvery(t *testing.T) {
	poolOf := status.AccountPools(pooledAccounts, func() identity.Record { return keptRecord })
	tests := []struct {
		name  string
		event status.Event
		first string
		// in are the pools, of Acme and Default, the event is of.
		in []string
	}{
		{name: "a start in the pool it recorded, on an account in another", event: status.Event{Kind: status.EventStarted, Account: "side", Pool: "Q7MT"}, first: firstPool, in: []string{acme}},
		{name: "a move in the pool it recorded", event: status.Event{Kind: status.EventMoved, From: "work", To: "side", Pool: acme}, first: firstPool, in: []string{acme}},
		{name: "a pin in the pool it recorded", event: status.Event{Kind: status.EventPin, Accounts: []string{"side"}, Pool: acme}, first: firstPool, in: []string{acme}},
		{name: "an auto in the pool it recorded", event: status.Event{Kind: status.EventAuto, Pool: firstPool}, first: firstPool, in: []string{firstPool}},
		{name: "a start naming no pool, as before pools, is the first's", event: status.Event{Kind: status.EventStarted, Account: "work"}, first: firstPool, in: []string{firstPool}},
		{name: "a move naming no pool is the first's", event: status.Event{Kind: status.EventMoved, From: "side", To: "work"}, first: firstPool, in: []string{firstPool}},
		{name: "a pin naming no pool is the first's", event: status.Event{Kind: status.EventPin, Accounts: []string{"work"}}, first: firstPool, in: []string{firstPool}},
		{name: "an auto naming no account nor pool is the first's", event: status.Event{Kind: status.EventAuto}, first: firstPool, in: []string{firstPool}},
		{name: "a start naming no pool, the first unknown, is in none", event: status.Event{Kind: status.EventStarted, Account: "work"}},
		{name: "an auto naming no account nor pool, the first unknown, is in none", event: status.Event{Kind: status.EventAuto}},
		{name: "a start in a pool since removed is in none", event: status.Event{Kind: status.EventStarted, Account: "work", Pool: "x9z9"}, first: firstPool},
		{name: "a pin in a pool since removed is in none", event: status.Event{Kind: status.EventPin, Accounts: []string{"work"}, Pool: "x9z9"}, first: firstPool},
		{name: "a limit goes by its account's pool now", event: status.Event{Kind: status.EventLimit, Account: "work"}, first: firstPool, in: []string{acme}},
		{name: "a cap goes by its account's pool now", event: status.Event{Kind: status.EventCap, Account: "side"}, first: firstPool, in: []string{firstPool}},
		{name: "a refusal of an account removed goes by the pool the record keeps it in", event: status.Event{Kind: status.EventRefused, Account: "lapsed"}, first: firstPool, in: []string{acme}},
		{name: "a limit of an account the record places in none is every pool's", event: status.Event{Kind: status.EventLimit, Account: "spare"}, first: firstPool, in: []string{acme, firstPool}},
		{name: "a limit of an account none knows is every pool's", event: status.Event{Kind: status.EventLimit, Account: "unknown"}, first: firstPool, in: []string{acme, firstPool}},
		{name: "the router's own restart is every pool's", event: status.Event{Kind: status.EventRestart}, first: firstPool, in: []string{acme, firstPool}},
		{name: "the router's own health is every pool's", event: status.Event{Kind: status.EventHealth}, in: []string{acme, firstPool}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, pool := range []string{acme, firstPool} {
				if got, want := tt.event.InPool(pool, tt.first, poolOf), slices.Contains(tt.in, pool); got != want {
					t.Errorf("InPool(%s, first %q) = %v, want %v", pool, tt.first, got, want)
				}
			}
		})
	}
}

func TestAnAccountsPoolIsTheConfigsElseTheRecordsKeptForItOnceRemoved(t *testing.T) {
	read := 0
	poolOf := status.AccountPools(pooledAccounts, func() identity.Record { read++; return keptRecord })
	tests := []struct {
		account string
		pool    string
		placed  bool
	}{
		{account: "WORK", pool: acme, placed: true},
		{account: "lapsed", pool: acme, placed: true},
		{account: "spare"},
		{account: "unknown"},
	}
	for _, tt := range tests {
		if pool, placed := poolOf(tt.account); pool != tt.pool || placed != tt.placed {
			t.Errorf("poolOf(%s) = %q, %v; want %q, %v", tt.account, pool, placed, tt.pool, tt.placed)
		}
	}
	if read != 3 {
		t.Errorf("the record was read %d times, want 3: once an account the config doesn't configure, never for one it does", read)
	}
}

// What's of an account is of the pool it's placed in, or of every pool where
// none places it, as an account removed before pools.
func TestWhatsOfAnAccountIsOfItsPoolElseOfEvery(t *testing.T) {
	poolOf := status.AccountPools(pooledAccounts, func() identity.Record { return keptRecord })
	tests := []struct {
		account string
		in      []string
	}{
		{account: "work", in: []string{acme}},
		{account: "side", in: []string{firstPool}},
		{account: "lapsed", in: []string{acme}},
		{account: "spare", in: []string{acme, firstPool}},
		{account: "unknown", in: []string{acme, firstPool}},
	}
	for _, tt := range tests {
		for _, pool := range []string{acme, firstPool} {
			if got, want := status.AccountIn(tt.account, pool, poolOf), slices.Contains(tt.in, pool); got != want {
				t.Errorf("AccountIn(%s, %s) = %v, want %v", tt.account, pool, got, want)
			}
		}
	}
}

func TestAnEventBefallsTheAccountItNamesElseItsPinsElseWhereItsSessionWent(t *testing.T) {
	tests := []struct {
		event status.Event
		want  string
	}{
		{event: status.Event{Account: "work", Accounts: []string{"side"}, To: "spare"}, want: "work"},
		{event: status.Event{Accounts: []string{"side", "work"}, To: "spare"}, want: "side"},
		{event: status.Event{From: "side", To: "work"}, want: "work"},
		{event: status.Event{From: "side"}, want: "side"},
		{event: status.Event{Kind: status.EventRestart}},
	}
	for _, tt := range tests {
		if got := tt.event.Befell(); got != tt.want {
			t.Errorf("Befell(%+v) = %q, want %q", tt.event, got, tt.want)
		}
	}
}

// A pool is labelled as the config labels it, else as the accounts record
// keeps it, of one removed, else by its id; the record is read for a pool
// the config doesn't configure alone.
func TestAPoolIsLabelledByTheConfigElseTheRecordElseItsID(t *testing.T) {
	read := 0
	label := status.PoolLabels(config.Pools{{ID: acme, Label: "Acme"}}, func() identity.Record {
		read++
		return identity.Record{Pools: []identity.PoolEntry{
			{ID: "x9z9", Label: "Client"}, {ID: "k3f9"},
			// Default, as this Mac's config last wrote it, now naming no pools.
			{ID: firstPool, First: true, Here: true, Written: identity.PoolWritten{Label: "Default"}},
		}}
	})
	tests := []struct{ id, want string }{
		{id: "Q7MT", want: "Acme"},
		{id: "x9z9", want: "Client"},
		{id: firstPool, want: "Default"},
		{id: "k3f9", want: "k3f9"},
		{id: "h4w2", want: "h4w2"},
	}
	for _, tt := range tests {
		if got := label(tt.id); got != tt.want {
			t.Errorf("label(%s) = %q, want %q", tt.id, got, tt.want)
		}
	}
	if read != 4 {
		t.Errorf("the record was read %d times, want 4: never for a pool the config configures", read)
	}
}

// A pool the accounts record keeps that the config doesn't configure is
// kept, labelled as PoolLabels labels it, and named marked: when it was
// removed, else that it isn't configured here. A configured pool, or one the
// record doesn't keep, isn't.
func TestAPoolTheRecordKeepsButTheConfigDoesntIsKept(t *testing.T) {
	removed := time.Date(2026, 10, 6, 18, 0, 0, 0, time.Local)
	// Each pool removed here is as KeepUp leaves it, no longer Here, but
	// as this Mac's config last wrote it.
	written := func(label string) identity.PoolWritten { return identity.PoolWritten{Label: label, PrimaryKnown: true} }
	record := identity.Record{Pools: []identity.PoolEntry{
		{ID: acme, Here: true, Written: written("Acme")},
		{ID: "x9z9", Label: "Acme old", Removed: removed, Written: written("Acme old")},
		{ID: "f4xk", Label: "Client"},
		{ID: firstPool, First: true, Label: "Default", Removed: removed, Written: written("Default")},
		// This Mac's, still Here, its config changed while no router kept
		// the record up with it, so the record notes no removal.
		{ID: "m3pq", Here: true, Written: written("Side")},
		// Another Mac's, removed there, as an import carries it in.
		{ID: "r7vb", Label: "Studio", Removed: removed},
	}}
	pools := config.Pools{{ID: acme, Label: "Acme"}}
	tests := []struct {
		id   string
		want string
		kept bool
	}{
		{id: "X9Z9", want: "pool Acme old (removed 6 Oct)", kept: true},
		{id: "f4xk", want: "pool Client (not configured here)", kept: true},
		{id: firstPool, want: "pool Default (removed 6 Oct)", kept: true},
		{id: "m3pq", want: "pool Side (removed)", kept: true},
		{id: "r7vb", want: "pool Studio (not configured here)", kept: true},
		{id: acme},
		{id: "h4w2"},
	}
	for _, tt := range tests {
		kept, ok := status.KeptPoolOf(record, pools, tt.id)
		if ok != tt.kept || ok && kept.Said() != tt.want {
			t.Errorf("KeptPoolOf(%s) = %+v, %t, saying %q; want kept %t, saying %q", tt.id, kept, ok, kept.Said(), tt.kept, tt.want)
		}
	}
}
