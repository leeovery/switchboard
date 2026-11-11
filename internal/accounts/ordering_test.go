package accounts_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
)

// readOnly makes dir one nothing can be written into till the test ends.
func readOnly(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
}

// configApart moves the world's config into a directory of its own, so it
// can be made one the config can't be saved in, as readOnly does, and
// returns that directory.
func (w *world) configApart(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(filepath.Dir(w.config), "config")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	moved := filepath.Join(dir, "config.toml")
	if err := os.Rename(w.config, moved); err != nil {
		t.Fatal(err)
	}
	w.config = moved
	return dir
}

// stateReadOnly makes the world's state directory one the accounts record
// can't be written in, its lock and the tokens' directory there already, as
// a token's written in that directory of its own.
func (w *world) stateReadOnly(t *testing.T) {
	t.Helper()
	w.writeToken(t, "work", "test-token-work", 0o600)
	if err := identity.NewFile(w.state).Update(func(*identity.Record) error { return nil }); err != nil {
		t.Fatal(err)
	}
	readOnly(t, w.state)
}

// Adding writes the token, then the record, then the config, all under the
// record's lock: where the config can't be saved, the record is left ahead,
// holding the account, and the token written for it is removed; adding it
// again then brings it back under its id, the record put right. Where the
// record can't be written, nothing is: the token's removed, the config left
// as it was.
func TestAddingWritesTheRecordBeforeTheConfig(t *testing.T) {
	add := func(t *testing.T, w *world) (accounts.Added, error) {
		t.Helper()
		return w.unasked(t, &user{}).AddChecked(t.Context(), config.NewAccount{Label: "Side"}, tokenOf(t, sideToken),
			accounts.Checked{Organization: organization}, nil, accounts.Answer{})
	}
	t.Run("the config not saved", func(t *testing.T) {
		w := newWorld(t, twoAccounts)
		readOnly(t, w.configApart(t))

		if _, err := add(t, w); err == nil {
			t.Fatal("AddChecked() with the config unsaved succeeded, want an error")
		}
		w.checkConfig(t, twoAccounts)
		w.checkNoToken(t, sideID)
		w.checkRecord(t, identity.Entry{ID: sideID, Organization: organization, Added: now})

		if err := os.Chmod(filepath.Dir(w.config), 0o700); err != nil {
			t.Fatal(err)
		}
		if got, err := add(t, w); err != nil || got != (accounts.Added{ID: sideID, Returning: true}) {
			t.Fatalf("AddChecked() again = %+v, %v; want the account back under its id", got, err)
		}
		w.checkConfig(t, twoAccounts+"\n[[account]]\nid    = \""+sideID+"\"\nlabel = \"Side\"\n")
		w.checkToken(t, sideID, sideToken)
		w.checkRecord(t, identity.Entry{ID: sideID, Organization: organization, Added: now})
	})
	t.Run("the record not written", func(t *testing.T) {
		w := newWorld(t, twoAccounts)
		w.stateReadOnly(t)

		if _, err := add(t, w); err == nil {
			t.Fatal("AddChecked() with the record unwritten succeeded, want an error")
		}
		w.checkConfig(t, twoAccounts)
		w.checkNoToken(t, sideID)
		w.checkRecord(t)
	})
}

// Removing marks the account removed in the record, then saves the config,
// both under the record's lock, then removes its token file: where the
// config can't be saved, the account's left configured, its token kept,
// marked removed, and removing it again removes it. Where the record can't
// be written, nothing is.
func TestRemovingWritesTheRecordBeforeTheConfig(t *testing.T) {
	side := identity.Entry{ID: "side", Organization: organization, Added: before}
	removed := identity.Entry{ID: "side", Organization: organization, Label: "Side", Added: before, Removed: now}
	t.Run("the config not saved", func(t *testing.T) {
		w := newWorld(t, withSide)
		w.writeRecord(t, side)
		w.writeToken(t, "side", sideToken, 0o600)
		readOnly(t, w.configApart(t))

		if _, err := w.registry(&user{}, nil).Remove("side"); err == nil {
			t.Fatal("Remove() with the config unsaved succeeded, want an error")
		}
		w.checkConfig(t, withSide)
		w.checkToken(t, "side", sideToken)
		w.checkRecord(t, removed)

		if err := os.Chmod(filepath.Dir(w.config), 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := w.registry(&user{}, nil).Remove("side"); err != nil {
			t.Fatalf("Remove() again = %v, want side removed", err)
		}
		w.checkConfig(t, twoAccounts)
		w.checkNoToken(t, "side")
		w.checkRecord(t, removed)
	})
	t.Run("the record not written", func(t *testing.T) {
		w := newWorld(t, withSide)
		w.writeRecord(t, side)
		w.writeToken(t, "side", sideToken, 0o600)
		w.stateReadOnly(t)

		if _, err := w.registry(&user{}, nil).Remove("side"); err == nil {
			t.Fatal("Remove() with the record unwritten succeeded, want an error")
		}
		w.checkConfig(t, withSide)
		w.checkToken(t, "side", sideToken)
		w.checkRecord(t, side)
	})
}

// A replaced token is held to the account's organization as the record has
// it under its lock, where nothing's to be recorded as much as where
// something is: a change of the record's that's under way when the token's
// given is waited for, and the token held to what it leaves.
func TestAReplacedTokenIsHeldToTheRecordUnderItsLock(t *testing.T) {
	sideAt := twoAccounts + "\n[[account]]\nid    = \"k3f9\"\nlabel = \"Side\"\n"
	w := newWorld(t, sideAt)
	w.writeRecord(t, identity.Entry{ID: "k3f9", Organization: organization, Added: now})
	w.writeToken(t, "k3f9", "test-token-stale", 0o600)
	changing, release := make(chan struct{}), make(chan struct{})
	changed := make(chan error, 1)
	go func() {
		changed <- identity.NewFile(w.state).Update(func(r *identity.Record) error {
			close(changing)
			<-release
			r.Accounts[0].Organization = "test-org-other"
			return nil
		})
	}()
	<-changing
	replaced := make(chan error, 1)
	go func() {
		replaced <- w.unasked(t, &user{}).SetTokenChecked(t.Context(), "k3f9", tokenOf(t, sideToken), accounts.Checked{Organization: organization})
	}()
	select {
	case err := <-replaced:
		t.Fatalf("SetTokenChecked() = %v while the record was being changed, want it waiting on the record's lock", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-changed; err != nil {
		t.Fatal(err)
	}
	if _, ok := errors.AsType[*accounts.OtherSubscriptionError](<-replaced); !ok {
		t.Error("SetTokenChecked() saved the token, want it refused as the record has the account's organization now")
	}
	w.checkToken(t, "k3f9", "test-token-stale")
}
