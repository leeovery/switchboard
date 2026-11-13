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
