package setup_test

import (
	"context"
	"errors"
	"maps"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/setup"
)

// A token made or pasted for an account is compared with every other
// account's organization, as the API names it checking each one's token:
// never the account's own, which isn't asked about, nor one the API refuses,
// doesn't answer for, or has no token to ask about.
func TestOtherOrganizationsAreTheOtherAccountsAsTheAPINamesThem(t *testing.T) {
	tests := []struct {
		id        string
		want      setup.Organizations
		wantAsked []string
	}{
		{id: "personal", want: setup.Organizations{"work": "org-work"}, wantAsked: []string{downToken, sideToken, workToken}},
		{id: "work", want: setup.Organizations{"personal": "org-personal"}, wantAsked: []string{downToken, personalToken, sideToken}},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			w := frame3(t)
			w.writeConfig(t, frameConfig+"\n[[account]]\nid = \"down\"\n\n[[account]]\nid = \"none\"\n")
			w.writeToken(t, "down", downToken)
			api := &checkingAPI{}
			before := w.snapshot(t)

			got, err := w.reviewing(t, api).OtherOrganizations(t.Context(), tt.id)

			if err != nil || !maps.Equal(got, tt.want) {
				t.Errorf("OtherOrganizations(%s) = %v, %v; want %v", tt.id, got, err, tt.want)
			}
			if asked := slices.Sorted(slices.Values(api.askedOf())); !slices.Equal(asked, tt.wantAsked) {
				t.Errorf("the API was asked about %q, want %q", asked, tt.wantAsked)
			}
			w.checkUnchanged(t, before)
		})
	}
}

// The other accounts' tokens are checked all at once, so the wait is the
// slowest check's, not their sum.
func TestOtherOrganizationsChecksTheTokensAllAtOnce(t *testing.T) {
	w := frame3(t)
	api := &meetingAPI{all: 2, met: make(chan struct{})}
	s := w.reviewing(t, &checkingAPI{})
	s.Accounts.Prober = api.proberAt

	got, err := s.OtherOrganizations(t.Context(), "side")

	if want := (setup.Organizations{"work": "org", "personal": "org"}); err != nil || !maps.Equal(got, want) {
		t.Errorf("OtherOrganizations(side) = %v, %v; want %v, each check meeting the other", got, err, want)
	}
}

func TestOtherOrganizationsFailsOnceItsContextIsDone(t *testing.T) {
	w := frame3(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if got, err := w.reviewing(t, &checkingAPI{}).OtherOrganizations(ctx, "work"); !errors.Is(err, context.Canceled) {
		t.Errorf("OtherOrganizations once its context is done = %v, %v; want %v", got, err, context.Canceled)
	}
}

// meetingAPI names every token's organization "org", but only once all its
// checks have come, so a check made while another waits for it to end waits
// out endsWithin, and the API doesn't answer for it.
type meetingAPI struct {
	all int
	met chan struct{}

	mu   sync.Mutex
	came int
}

func (a *meetingAPI) proberAt(string) accounts.Prober {
	return a
}

func (a *meetingAPI) Probe(ctx context.Context, _ string) (quota.Probe, error) {
	a.mu.Lock()
	if a.came++; a.came == a.all {
		close(a.met)
	}
	a.mu.Unlock()
	select {
	case <-a.met:
		return quota.Probe{Organization: "org"}, nil
	case <-ctx.Done():
		return quota.Probe{}, ctx.Err()
	case <-time.After(endsWithin):
		return quota.Probe{}, errors.New("the other checks never came")
	}
}
