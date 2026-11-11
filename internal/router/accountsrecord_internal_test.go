package router

import (
	"os"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

// livePersonal is personal as the accounts record keeps it while it's
// configured, its organization not known.
var livePersonal = identity.Entry{ID: "personal", Added: start}

// entryOf is the router's accounts record's account with the given id,
// failing the test where it has none.
func entryOf(t *testing.T, r *Router, id string) identity.Entry {
	t.Helper()
	record, err := identity.NewFile(r.cfg.StateDir).Read()
	if err != nil {
		t.Fatal(err)
	}
	e, ok := record.ByID(id)
	if !ok {
		t.Fatalf("the accounts record holds no %s, want it to", id)
	}
	return e
}

// configure has the router's config file make a config of the accounts
// given, as upkeep's look finds it, and upkeep keep the record in step.
func configure(r *Router, accounts config.Accounts) {
	r.upkeep.restarts.config.accounts, r.upkeep.restarts.config.changed = accounts, true
	r.upkeep.keepRecord()
}

// withAccount is testConfigured with the account given after its own.
func withAccount(a config.Account) config.Accounts {
	return append(slices.Clone(config.Accounts(testConfigured)), a)
}

// without is testConfigured without the account with the given id.
func without(id string) config.Accounts {
	return slices.DeleteFunc(slices.Clone(config.Accounts(testConfigured)), func(a config.Account) bool { return a.ID == id })
}

func TestAnAccountConfiguredIsRecordedAndItsTokenHeldToItsOrganization(t *testing.T) {
	log := logstest.Capture(t)
	clock := &testClock{now: start}
	prober := &stubProber{readings: map[string]quota.Probe{sideToken: orgProbed("test-org-side")}}
	r := newHeldRouter(t, clock, prober, testTokens.Read, heldWork)

	// As the router starts: the accounts it's configured with that the
	// record lacks are added, their organizations taken from their probes.
	clock.now = start.Add(time.Minute)
	r.upkeep.keepRecord()
	for _, id := range []string{"personal", "side"} {
		if e := entryOf(t, r, id); e != (identity.Entry{ID: id, Added: clock.now}) {
			t.Errorf("the record holds %+v, want %s added now, its organization not known", e, id)
		}
	}
	if !log.Has("level=INFO", `msg="account configured here added to the accounts record"`, "account=side") {
		t.Errorf("log reads\n%s\nwant side told added to the record", log)
	}
	probeNow(r, "side")
	if got := recorded(t, r, "side"); got != "test-org-side" {
		t.Errorf("the record holds side to %q, want test-org-side, as its probe named", got)
	}

	// Added to the config while the router runs, it's recorded at once, and
	// the router restarting onto the config takes its organization.
	clock.now = start.Add(2 * time.Minute)
	added := withAccount(config.Account{ID: "n4p8", Label: "Team"})
	configure(r, added)
	if e := entryOf(t, r, "n4p8"); e != (identity.Entry{ID: "n4p8", Added: clock.now}) {
		t.Errorf("the record holds %+v, want n4p8 added now", e)
	}
	cfg := testConfig(clock.read, &stubProber{readings: map[string]quota.Probe{"test-token-n4p8": orgProbed("test-org-team")}},
		tokenstest.Files{"work": workToken, "side": sideToken, "n4p8": "test-token-n4p8"}.Read)
	cfg.Accounts, cfg.StateDir = added, r.cfg.StateDir
	restarted := newTestRouterOf(t, cfg)
	probeNow(restarted, "n4p8")
	if got := recorded(t, restarted, "n4p8"); got != "test-org-team" {
		t.Errorf("the record holds n4p8 to %q, want test-org-team, as its probe named", got)
	}
}

func TestAnAccountThatLeftTheConfigIsMarkedRemovedWithTheLabelItLastHad(t *testing.T) {
	tests := []struct {
		name string
		// relabelled is side's label in a config made before the one without
		// it: "" where none was.
		relabelled string
		want       string
	}{
		{name: "as the router started", want: "Side"},
		{name: "relabelled since", relabelled: "Side project", want: "Side project"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			clock := &testClock{now: start}
			r := newHeldRouter(t, clock, &stubProber{}, testTokens.Read, heldWork, livePersonal, heldSide)
			r.upkeep.keepRecord()
			if tt.relabelled != "" {
				relabelled := slices.Clone(config.Accounts(testConfigured))
				relabelled[2].Label = tt.relabelled
				configure(r, relabelled)
			}

			clock.now = start.Add(time.Hour)
			configure(r, without("side"))
			want := heldSide
			want.Label, want.Removed = tt.want, clock.now
			if e := entryOf(t, r, "side"); e != want {
				t.Errorf("the record holds %+v, want %+v", e, want)
			}
			if !log.Has("level=INFO", `msg="account no longer configured marked removed in the accounts record"`, "account=side") {
				t.Errorf("log reads\n%s\nwant side told marked removed", log)
			}
			if e := entryOf(t, r, "work"); e != heldWork {
				t.Errorf("the record holds %+v, want work as it was", e)
			}
		})
	}
}

func TestAnAccountPutBackInTheConfigIsLiveAgain(t *testing.T) {
	log := logstest.Capture(t)
	clock := &testClock{now: start}
	removedSide := heldSide
	removedSide.Label, removedSide.Removed = "Side", start.Add(-time.Hour)
	r := newHeldRouter(t, clock, &stubProber{}, testTokens.Read, heldWork, livePersonal, removedSide)

	// Configured as the router starts.
	r.upkeep.keepRecord()
	if e := entryOf(t, r, "side"); e != heldSide {
		t.Errorf("the record holds %+v, want side live again, its organization kept", e)
	}
	if !log.Has("level=INFO", `msg="account configured again; the accounts record has it so"`, "account=side") {
		t.Errorf("log reads\n%s\nwant side told configured again", log)
	}

	// Removed from the config, then put back, while the router runs.
	configure(r, without("side"))
	configure(r, config.Accounts(testConfigured))
	if e := entryOf(t, r, "side"); e != heldSide {
		t.Errorf("the record holds %+v, want side live again", e)
	}
}

func TestTheRecordIsKeptInStepWithTheConfigOnlyOnceTheMoveIsDone(t *testing.T) {
	clock := &testClock{now: start}
	r := newHeldRouter(t, clock, &stubProber{}, testTokens.Read, heldWork, heldSide)
	r.moving.standing(status.Moving{Stage: status.MoveRunning})
	path := identity.NewFile(r.cfg.StateDir).Path()
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	r.upkeep.keepRecord()
	configure(r, withAccount(config.Account{ID: "n4p8", Label: "Team"}))
	if after, err := os.ReadFile(path); err != nil || string(after) != string(before) {
		t.Fatalf("the record reads\n%s\nwant it as it was while the move is owed\n%s", after, before)
	}

	// The move done, the next look keeps it in step.
	r.moving.stands.Store(nil)
	r.upkeep.keepRecord()
	for _, id := range []string{"personal", "n4p8"} {
		if e := entryOf(t, r, id); !e.Removed.IsZero() {
			t.Errorf("the record holds %+v, want %s live", e, id)
		}
	}
}

func TestAnAccountConfiguredUnderAnIDTheRecordHoldsForAnotherIsLeftOut(t *testing.T) {
	log := logstest.Capture(t)
	clock := &testClock{now: start}
	// An account moved onto its id from side, before side was configured.
	movedFromSide := identity.Entry{ID: "h7c2", Label: "Spare", Added: start, Removed: start, Was: "side"}
	r := newHeldRouter(t, clock, &stubProber{}, testTokens.Read, heldWork, livePersonal, movedFromSide)

	r.upkeep.keepRecord()
	record, err := identity.NewFile(r.cfg.StateDir).Read()
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(record.Accounts, []identity.Entry{heldWork, livePersonal, movedFromSide}) {
		t.Errorf("the record holds %+v, want it as it was", record.Accounts)
	}
	if !log.Has("level=WARN", "left out of the record", "account=side") {
		t.Errorf("log reads\n%s\nwant side told left out", log)
	}
}
