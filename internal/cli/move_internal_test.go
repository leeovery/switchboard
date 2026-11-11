package cli

import (
	"context"
	"errors"
	"os"
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
