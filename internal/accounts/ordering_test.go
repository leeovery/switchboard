package accounts_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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
// record's lock: where the record or the config can't be written, the token
// written is removed, the config left as it was, and the record put back as
// it was, a removed account keeping the label it kept.
func TestAddingWritesTheRecordBeforeTheConfig(t *testing.T) {
	removed := identity.Entry{ID: sideID, Organization: organization, Label: "Old side", Added: before, Removed: before}
	tests := []struct {
		name   string
		record []identity.Entry
		// fail makes the world one the record, or the config, can't be
		// written in.
		fail func(t *testing.T, w *world)
		// wantErr is part of what the add fails with.
		wantErr string
	}{
		{name: "the config not saved, a new account", fail: func(t *testing.T, w *world) { readOnly(t, w.configApart(t)) }, wantErr: "write config"},
		{name: "the config not saved, an account this Mac had", record: []identity.Entry{removed}, fail: func(t *testing.T, w *world) { readOnly(t, w.configApart(t)) }, wantErr: "write config"},
		{name: "the record not written", fail: func(t *testing.T, w *world) { w.stateReadOnly(t) }, wantErr: "write the accounts record"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, twoAccounts)
			w.writeRecord(t, tt.record...)
			tt.fail(t, w)

			_, err := w.unasked(t, &user{}).AddChecked(t.Context(), config.NewAccount{Label: "Side"}, tokenOf(t, sideToken),
				accounts.Checked{Organization: organization}, nil, accounts.Answer{})

			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("AddChecked() = %v, want an error saying %q", err, tt.wantErr)
			}
			w.checkConfig(t, twoAccounts)
			w.checkNoToken(t, sideID)
			w.checkRecord(t, tt.record...)
		})
	}
}

// Removing marks the account removed in the record, saves the config, then
// removes its token file, all under the record's lock: where the record or
// the config can't be written, nothing is, the record put back as it was.
func TestRemovingWritesTheRecordBeforeTheConfig(t *testing.T) {
	side := identity.Entry{ID: "side", Organization: organization, Added: before}
	tests := []struct {
		name    string
		fail    func(t *testing.T, w *world)
		wantErr string
	}{
		{name: "the config not saved", fail: func(t *testing.T, w *world) { readOnly(t, w.configApart(t)) }, wantErr: "write config"},
		{name: "the record not written", fail: func(t *testing.T, w *world) { w.stateReadOnly(t) }, wantErr: "write the accounts record"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, withSide)
			w.writeRecord(t, side)
			w.writeToken(t, "side", sideToken, 0o600)
			tt.fail(t, w)

			if _, err := w.registry(&user{}, nil).Remove("side"); err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Remove() = %v, want an error saying %q", err, tt.wantErr)
			}
			w.checkConfig(t, withSide)
			w.checkToken(t, "side", sideToken)
			w.checkRecord(t, side)
		})
	}
}

// A replaced token whose organization the record can't be written to keep
// is saved all the same, written first, and that's said.
func TestAReplacedTokenWhoseOrganizationIsntRecordedSaysSo(t *testing.T) {
	sideAt := twoAccounts + "\n[[account]]\nid    = \"k3f9\"\nlabel = \"Side\"\n"
	w := newWorld(t, sideAt)
	w.writeRecord(t, identity.Entry{ID: "k3f9", Added: now})
	w.writeToken(t, "k3f9", "test-token-stale", 0o600)
	readOnly(t, w.state)

	err := w.unasked(t, &user{}).SetTokenChecked(t.Context(), "k3f9", tokenOf(t, sideToken), accounts.Checked{Organization: organization})

	if err == nil || !strings.Contains(err.Error(), "k3f9's token is saved, but its organization isn't recorded") {
		t.Errorf("SetTokenChecked() = %v, want it saying the token's saved and the record isn't", err)
	}
	w.checkToken(t, "k3f9", sideToken)
	w.checkRecord(t, identity.Entry{ID: "k3f9", Added: now})
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
