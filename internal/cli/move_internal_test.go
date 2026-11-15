package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/tokens"
)

// probing is a prober that answers every token as given.
type probing struct {
	probe quota.Probe
	err   error
}

func (p probing) Probe(context.Context, string) (quota.Probe, error) {
	return p.probe, p.err
}

func TestAnAnswerNamingAnOrganizationNamesItWindowsOrNot(t *testing.T) {
	refused := errors.New("HTTP 401: invalid token")
	tests := []struct {
		name    string
		prober  probing
		want    string
		wantErr error
	}{
		{"an answer reading windows", probing{probe: quota.Probe{Organization: "org-work", Usage: quota.Usage{Windows: []quota.Window{{Key: "five_hour"}}}}}, "org-work", nil},
		{"an answer of success reading none", probing{probe: quota.Probe{Organization: "org-work"}, err: errors.New("HTTP 200: no window read")}, "org-work", nil},
		{"a refusal naming none", probing{err: refused}, "", refused},
	}
	token, err := tokens.Parse("test-token-work")
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := organizations{tt.prober}.Organization(t.Context(), token)
			if got != tt.want || !errors.Is(err, tt.wantErr) {
				t.Errorf("Organization() = %q, %v, want %q, %v", got, err, tt.want, tt.wantErr)
			}
		})
	}
}

func TestTheRouterKnowsAnAccountByTheIDItHadBeforeIDsWereMade(t *testing.T) {
	dir := t.TempDir()
	if err := identity.NewFile(dir).Update(func(r *identity.Record) error {
		r.Move("b6s7", "org-work", "work", time.Now())
		r.Move("qgr2", "org-side", "side", time.Now())
		r.Remove("qgr2", "Side", time.Now())
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	of := formerly(dir)
	for _, tt := range []struct {
		was, want string
		ok        bool
	}{
		{was: "work", want: "b6s7", ok: true},
		{was: "WORK", want: "b6s7", ok: true},
		{was: "side"},
		{was: "personal"},
	} {
		if got, ok := of(tt.was); ok != tt.ok || ok && got != tt.want {
			t.Errorf("formerly(%q) = %q, %t, want %q, %t", tt.was, got, ok, tt.want, tt.ok)
		}
	}
	if _, ok := formerly(t.TempDir())("work"); ok {
		t.Error("with no accounts record, an id was known as another's")
	}
	if err := os.WriteFile(identity.NewFile(dir).Path(), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, ok := formerly(dir)("work"); ok {
		t.Error("with the accounts record damaged, an id was known as another's")
	}
}

func TestTheHealAsServeStartsWritesNoPoolsWhereItCantWriteTheIDsBack(t *testing.T) {
	dir := t.TempDir()
	added := time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC)
	if err := identity.NewFile(dir).Update(func(r *identity.Record) error {
		r.Accounts = []identity.Entry{
			{ID: "work", Added: added, Was: "old-work", Pool: "b2n8"},
			{ID: "side", Added: added, Was: "old-side", Pool: "b2n8"},
		}
		r.Pools = []identity.PoolEntry{{ID: "b2n8", Added: added, First: true, Here: true, Written: identity.PoolWritten{Label: "Default"}}}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	for _, mark := range []string{"accounts.moved", "pools.moved"} {
		if err := os.WriteFile(filepath.Join(dir, mark), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// Put back from before both moves, old-work's id under a key written so
	// the config's edit can't write its new id back.
	const unwritable = "[[account]]\n\"i\\u0064\" = \"old-work\"\nlabel = \"old-work\"\n\n" +
		"[[account]]\nid = \"old-side\"\nlabel = \"old-side\"\n"
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(unwritable), 0o600); err != nil {
		t.Fatal(err)
	}
	home := t.TempDir()

	err := heal(path, dir, func() (string, error) { return home, nil })

	if err == nil {
		t.Error("heal() = nil, want the ids' failure, for the router to tell of and try again")
	}
	if got, readErr := os.ReadFile(path); readErr != nil || string(got) != unwritable {
		t.Errorf("the config reads\n%s\n(%v), want it as it was, no pools written under the ids from before\n%s", got, readErr, unwritable)
	}
}
