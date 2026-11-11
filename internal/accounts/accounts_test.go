package accounts_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/tokens"
)

const (
	workToken = "test-token-work"
	sideToken = "test-token-side"
	// upstream is where the config has requests go.
	upstream = "http://127.0.0.1:4700"
	// sidePrompt is how the user at a terminal is asked for side's token,
	// with the newline that ends it, which doesn't show as it's typed; and
	// sideLabelPrompt for the token of an account labelled Side, to add.
	sidePrompt      = "Paste side's token, from claude setup-token run while signed in to that subscription (it won't show): \n"
	sideLabelPrompt = "Paste Side's token, from claude setup-token run while signed in to that subscription (it won't show): \n"
	// refusedErr is the error of a token the API refuses.
	refusedErr = "the API refused the token (HTTP 401 · Invalid bearer token), so nothing is saved: " +
		"make another with claude setup-token, run while signed in to that subscription"
)

// now is when the user gives a token, by the registry's clock.
var now = time.Date(2026, 10, 27, 9, 30, 0, 0, time.UTC)

// twoAccounts is a config of two accounts, work and personal.
var twoAccounts = fmt.Sprintf("upstream = %q\n\n[[account]]\nid    = \"work\"\nlabel = \"Work\"\n\n[[account]]\nid = \"personal\"\n", upstream)

// withSide is twoAccounts with side, labelled Side, configured before
// switchboard made ids.
var withSide = twoAccounts + "\n[[account]]\nid    = \"side\"\nlabel = \"Side\"\n"

// sideID is the id the organization side's tokens are in derives, and
// sideAdded twoAccounts once side, labelled Side, is added under it.
var (
	sideID    = identity.Record{}.Derive(organization, nil)
	sideAdded = twoAccounts + "\n[[account]]\nid    = \"" + sideID + "\"\nlabel = \"Side\"\n"
)

// users are the ways a user gives a token: typed at a terminal, or piped in.
var users = []struct {
	name       string
	atTerminal bool
}{
	{name: "piped in"},
	{name: "typed at a terminal", atTerminal: true},
}

func TestAddTakesTheTokenTheUserGives(t *testing.T) {
	for _, tt := range users {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, twoAccounts)
			user := &user{atTerminal: tt.atTerminal, gives: sideToken + "\n"}
			api := &fakeAPI{organization: organization}

			added, err := w.registry(user, api).Add(t.Context(), config.NewAccount{Label: "Side"}, nil)
			if want := (accounts.Added{ID: sideID}); err != nil || added != want {
				t.Fatalf("Add() = %+v, %v, want %+v", added, err, want)
			}
			w.checkConfig(t, sideAdded)
			w.checkToken(t, sideID, sideToken)
			w.checkTaken(t, sideID, sideToken, now)
			w.checkRecord(t, identity.Entry{ID: sideID, Organization: organization, Added: now})
			if want := []string{upstream + " " + sideToken}; !slices.Equal(api.asked, want) {
				t.Errorf("the API was asked about %q, want %q", api.asked, want)
			}
			if want := map[bool]string{true: sideLabelPrompt}[tt.atTerminal]; user.shown.String() != want {
				t.Errorf("the user was shown %q, want %q", user.shown.String(), want)
			}
		})
	}
}

func TestARegistryWithoutAClockTakesTheTokenAtTheSystemsTime(t *testing.T) {
	w := newWorld(t, twoAccounts)
	registry := w.registry(&user{gives: sideToken}, &fakeAPI{organization: organization})
	registry.Now = nil
	before := time.Now().Truncate(time.Second)

	if _, err := registry.Add(t.Context(), config.NewAccount{Label: "Side"}, nil); err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	at, err := w.tokens().Taken(sideID, tokenOf(t, sideToken))
	if err != nil || at.Before(before) || at.After(time.Now()) {
		t.Errorf("side's token was taken at %v (%v), want the system's time, from %v", at, err, before)
	}
}

func TestAddMakesTheFirstConfig(t *testing.T) {
	w := newWorld(t, "")
	w.config = filepath.Join(filepath.Dir(w.config), "config", "switchboard", "config.toml")
	api := &fakeAPI{organization: organization}

	_, err := w.registry(&user{gives: workToken}, api).Add(t.Context(), config.NewAccount{Label: "Work", Primary: true}, nil)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	w.checkConfig(t, "[[account]]\nid      = \""+sideID+"\"\nlabel   = \"Work\"\nprimary = true\n")
	w.checkToken(t, sideID, workToken)
	if want := []string{"https://api.anthropic.com " + workToken}; !slices.Equal(api.asked, want) {
		t.Errorf("the API was asked about %q, want %q: the config's default upstream", api.asked, want)
	}
}

func TestAddReplacesATokenFileItsIDHas(t *testing.T) {
	w := newWorld(t, twoAccounts)
	w.writeToken(t, sideID, "test-token-stale", 0o644)

	_, err := w.registry(&user{gives: sideToken}, &fakeAPI{organization: organization}).Add(t.Context(), config.NewAccount{Label: "Side"}, nil)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}
	w.checkConfig(t, sideAdded)
	w.checkToken(t, sideID, sideToken)
}

// An account isn't added until its organization is known: where the API
// doesn't answer for its token, nothing is saved, and Add says so, to try
// again.
func TestAddSavesNothingTheAPIDidntAnswerFor(t *testing.T) {
	w := newWorld(t, twoAccounts)
	log := logstest.Capture(t)

	_, err := w.registry(&user{gives: sideToken}, &fakeAPI{err: errors.New("timed out after 5s")}).Add(t.Context(), config.NewAccount{Label: "Side"}, nil)
	want := "the API didn't answer for the token (timed out after 5s), so nothing is saved: an account is added only once it has, to know its subscription, so try again"
	if _, ok := errors.AsType[*accounts.UnansweredError](err); !ok || err.Error() != want {
		t.Fatalf("Add() error = %v, want an UnansweredError saying %q", err, want)
	}
	w.checkConfig(t, twoAccounts)
	w.checkNoToken(t, sideID)
	w.checkNoRecord(t)
	if strings.Contains(log.String(), sideToken) {
		t.Errorf("the log shows the token:\n%s", log)
	}
}

func TestAddSavesNothingTheAPIRefuses(t *testing.T) {
	w := newWorld(t, twoAccounts)

	_, err := w.registry(&user{gives: sideToken}, &fakeAPI{err: refusal{}}).Add(t.Context(), config.NewAccount{Label: "Side"}, nil)
	if err == nil || err.Error() != refusedErr || !errors.Is(err, accounts.ErrRefused) {
		t.Errorf("Add() error = %v, want %q", err, refusedErr)
	}
	w.checkConfig(t, twoAccounts)
	w.checkNoToken(t, sideID)
	w.checkNoRecord(t)
}

func TestAddRefuses(t *testing.T) {
	failed := errors.New("input/output error")
	side := config.NewAccount{Label: "Side"}
	tests := []struct {
		name    string
		account config.NewAccount
		user    *user
		wantErr string
		wantIs  error
		// wantAsked is set when the user is asked for a token first.
		wantAsked bool
	}{
		{
			name:    "no label",
			user:    &user{gives: sideToken},
			wantErr: "a label is required: every account has one",
			wantIs:  accounts.ErrNoLabel,
		},
		{
			name:    "a label that shows nothing",
			account: config.NewAccount{Label: " \u200b "},
			user:    &user{gives: sideToken},
			wantErr: "label is blank, showing nothing",
			wantIs:  config.ErrBlankLabel,
		},
		{
			name:    "a label shaped like a token",
			account: config.NewAccount{Label: "sk-ant-oat01-fake_token-shaped"},
			user:    &user{gives: sideToken},
			wantErr: "label looks like a token, which a label mustn't, as it shows wherever the account does",
		},
		{
			name:    "another account's label, in another case",
			account: config.NewAccount{Label: "WORK"},
			user:    &user{gives: sideToken},
			wantErr: "another account has that label already, ignoring case: give each its own",
			wantIs:  accounts.ErrLabelTaken,
		},
		{
			name:    "the label an account shows by its id, as it shows",
			account: config.NewAccount{Label: "  Personal "},
			user:    &user{gives: sideToken},
			wantErr: "another account has that label already, ignoring case: give each its own",
			wantIs:  accounts.ErrLabelTaken,
		},
		{
			name:      "nothing piped in",
			account:   side,
			user:      &user{gives: " \n"},
			wantErr:   "no token given",
			wantAsked: true,
		},
		{
			name:      "nothing typed",
			account:   side,
			user:      &user{atTerminal: true},
			wantErr:   "no token given",
			wantAsked: true,
		},
		{
			name:      "the terminal closed before anything was typed",
			account:   side,
			user:      &user{atTerminal: true, fails: io.EOF},
			wantErr:   "no token given",
			wantAsked: true,
		},
		{
			name:      "more than a token",
			account:   side,
			user:      &user{gives: "export CLAUDE_CODE_OAUTH_TOKEN=" + sideToken},
			wantErr:   "what was given is more than a token: give the token alone",
			wantAsked: true,
		},
		{
			name:      "a terminal that can't be read",
			account:   side,
			user:      &user{atTerminal: true, gives: sideToken, fails: failed},
			wantErr:   "read the token: input/output error",
			wantIs:    failed,
			wantAsked: true,
		},
		{
			name:      "the user interrupting the token's typing",
			account:   side,
			user:      &user{atTerminal: true, gives: sideToken, fails: accounts.ErrInterrupted},
			wantErr:   "read the token: interrupted",
			wantIs:    accounts.ErrInterrupted,
			wantAsked: true,
		},
		{
			name:      "a pipe that can't be read",
			account:   side,
			user:      &user{gives: sideToken, fails: failed},
			wantErr:   "read the token: input/output error",
			wantIs:    failed,
			wantAsked: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, twoAccounts)
			api := &fakeAPI{organization: organization}

			_, err := w.registry(tt.user, api).Add(t.Context(), tt.account, nil)
			if err == nil || err.Error() != tt.wantErr {
				t.Errorf("Add() error = %v, want %q", err, tt.wantErr)
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("Add() error = %v, want one matching %v", err, tt.wantIs)
			}
			if tt.user.asked != tt.wantAsked || len(api.asked) > 0 {
				t.Errorf("the user was asked: %v, want %v; the API about %q, want nothing", tt.user.asked, tt.wantAsked, api.asked)
			}
			w.checkConfig(t, twoAccounts)
			w.checkNoToken(t, sideID)
			w.checkNoRecord(t)
		})
	}
}

// CheckNew checks an account to add as Add does before it takes the token,
// writing nothing: a label is required, taken as config takes one, and no
// other configured account's, as each shows, ignoring case; a label no
// configured account has can be given, whoever had it before.
func TestCheckNew(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		label   string
		wantErr error
	}{
		{name: "a label of its own", config: twoAccounts, label: "Side"},
		{name: "a label with no config", label: "Side"},
		{name: "a label no configured account has", config: twoAccounts, label: "side"},
		{name: "no label", config: twoAccounts, wantErr: accounts.ErrNoLabel},
		{name: "a blank label", config: twoAccounts, label: "\u200b", wantErr: config.ErrBlankLabel},
		{name: "another's label", config: twoAccounts, label: "work", wantErr: accounts.ErrLabelTaken},
		{name: "another's label, as it shows once tidied", config: twoAccounts, label: "\u200bWork\u200b", wantErr: accounts.ErrLabelTaken},
		{name: "the id another without a label shows as, in another case", config: twoAccounts, label: "Personal", wantErr: accounts.ErrLabelTaken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, tt.config)

			err := w.registry(&user{}, &fakeAPI{}).CheckNew(config.NewAccount{Label: tt.label})
			if !errors.Is(err, tt.wantErr) || (err == nil) != (tt.wantErr == nil) {
				t.Errorf("CheckNew() error = %v, want %v", err, tt.wantErr)
			}
			if tt.config != "" {
				w.checkConfig(t, tt.config)
			}
			w.checkNoRecord(t)
		})
	}
}

// A config that already has two accounts showing one label, edited by hand,
// still loads, and takes an edit that gives neither a label.
func TestAConfigOfOneLabelTwiceTakesEditsOfAnythingButALabel(t *testing.T) {
	twice := twoAccounts + "\n[[account]]\nid    = \"side\"\nlabel = \"WORK\"\n"
	w := newWorld(t, twice)

	if _, err := w.registry(&user{}, &fakeAPI{}).Edit("side", accounts.Change{Plan: new("max")}); err != nil {
		t.Fatalf("Edit() error = %v, want the plan set", err)
	}
	w.checkConfig(t, twice+"plan = \"max\"\n")
}

func TestSetToken(t *testing.T) {
	for _, tt := range users {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, withSide)
			w.writeToken(t, "side", "test-token-stale", 0o600)
			user := &user{atTerminal: tt.atTerminal, gives: sideToken}
			api := &fakeAPI{}

			taken, err := w.registry(user, api).SetToken(t.Context(), w.load(t), "side")
			if err != nil || taken != (accounts.Taken{}) {
				t.Fatalf("SetToken() = %+v, %v, want the token taken and checked", taken, err)
			}
			w.checkToken(t, "side", sideToken)
			w.checkTaken(t, "side", sideToken, now)
			w.checkConfig(t, withSide)
			if want := []string{upstream + " " + sideToken}; !slices.Equal(api.asked, want) {
				t.Errorf("the API was asked about %q, want %q", api.asked, want)
			}
			if want := map[bool]string{true: sidePrompt}[tt.atTerminal]; user.shown.String() != want {
				t.Errorf("the user was shown %q, want %q", user.shown.String(), want)
			}
		})
	}
}

func TestSetTokenWhenTheAPIDoesntSayYes(t *testing.T) {
	unanswered := errors.New("timed out after 5s")
	tests := []struct {
		name   string
		answer error
		// wantUnchecked is why the token went unchecked, as Taken says.
		wantUnchecked string
		wantErr       string
		wantToken     string
		// wantAt is when the token in the file was taken, as its record says:
		// zero where that isn't known.
		wantAt time.Time
	}{
		{name: "refusing the token, which isn't saved", answer: refusal{}, wantErr: refusedErr, wantToken: "test-token-stale"},
		{name: "not answering, the token saved all the same", answer: unanswered, wantUnchecked: unanswered.Error(), wantToken: sideToken, wantAt: now},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, withSide)
			w.writeToken(t, "side", "test-token-stale", 0o600)

			taken, err := w.registry(&user{gives: sideToken}, &fakeAPI{err: tt.answer}).SetToken(t.Context(), w.load(t), "side")
			if errorText(taken.Unchecked) != tt.wantUnchecked || errorText(err) != tt.wantErr {
				t.Errorf("SetToken() = %+v, %v, want unchecked %q, %q", taken, err, tt.wantUnchecked, tt.wantErr)
			}
			w.checkToken(t, "side", tt.wantToken)
			w.checkTaken(t, "side", tt.wantToken, tt.wantAt)
		})
	}
}

// sameErr is the error of a token whose subscription is work's already.
const sameErr = "work has that subscription already, so nothing is saved: " +
	"make another with claude setup-token, run while signed in to the other subscription"

// others is a Registry's Others answering with organizations, or failing
// with err, noting each id it's asked for.
type others struct {
	organizations accounts.Organizations
	err           error
	asked         []string
}

func (o *others) of(_ context.Context, id string) (accounts.Organizations, error) {
	o.asked = append(o.asked, id)
	return o.organizations, o.err
}

// A token given to replace an account's is refused where its organization
// is another account's, saving nothing, and saved where it's of an
// organization of its own, or none is known.
func TestATokenGivenIsComparedWithTheOtherAccounts(t *testing.T) {
	unanswered := errors.New("timed out after 5s")
	failed := errors.New("the config can't be read")
	tests := []struct {
		name   string
		answer error
		// organization is the one the API names the token in.
		organization string
		others       others
		wantErr      string
		wantAsked    bool
	}{
		{name: "another account's organization", organization: "org-work", others: others{organizations: accounts.Organizations{"work": "org-work", "personal": "org-personal"}}, wantErr: sameErr, wantAsked: true},
		{name: "an organization of its own", organization: "org-side", others: others{organizations: accounts.Organizations{"work": "org-work", "personal": ""}}, wantAsked: true},
		{name: "its own organization, the account's", organization: "org-side", others: others{organizations: accounts.Organizations{"side": "org-side"}}, wantAsked: true},
		{name: "no organization named", others: others{organizations: accounts.Organizations{"work": ""}}},
		{name: "the API not answering", answer: unanswered, others: others{organizations: accounts.Organizations{"work": ""}}},
		{name: "the other accounts' not known", organization: "org-work", others: others{err: failed}, wantErr: "compare the token with the other accounts': " + failed.Error(), wantAsked: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, withSide)
			r := w.registry(&user{gives: sideToken}, &fakeAPI{err: tt.answer, organization: tt.organization})
			r.Others = tt.others.of

			_, err := r.SetToken(t.Context(), w.load(t), "side")

			if errorText(err) != tt.wantErr {
				t.Fatalf("taking side's token: %v, want %q", err, tt.wantErr)
			}
			if same, ok := errors.AsType[*accounts.SameSubscriptionError](err); ok != (tt.wantErr == sameErr) || ok && same.Other != "work" {
				t.Errorf("taking side's token: %#v, want a *SameSubscriptionError naming work alone where it's work's", err)
			}
			if want := map[bool][]string{true: {"side"}}[tt.wantAsked]; !slices.Equal(tt.others.asked, want) {
				t.Errorf("the other accounts' organizations were asked for %q, want %q", tt.others.asked, want)
			}
			w.checkConfig(t, withSide)
			if tt.wantErr != "" {
				w.checkNoToken(t, "side")
				return
			}
			w.checkToken(t, "side", sideToken)
		})
	}
}

// A token replacing an account's must be for the account's organization, as
// the accounts record has it: another's is refused, saving nothing, as
// another subscription's; and where the record doesn't know the account's
// yet, the token's is recorded as its own, unless the record has another
// account of it, named in the refusal. Where the API names none, or the
// record hasn't the account, as one from before ids were made, the token's
// saved, and nothing recorded.
func TestAReplacedTokenMustBeForTheAccountsOrganization(t *testing.T) {
	sideAt := twoAccounts + "\n[[account]]\nid    = \"k3f9\"\nlabel = \"Side\"\n"
	spare := identity.Entry{ID: "h7c2", Organization: organization, Label: "Spare", Added: now, Removed: now}
	tests := []struct {
		name   string
		config string
		// id is the account given the token, whose organization the API
		// names as organization.
		id, organization string
		record           []identity.Entry
		wantErr          string
		wantRecord       []identity.Entry
	}{
		{
			name: "the account's own", config: sideAt, id: "k3f9", organization: organization,
			record:     []identity.Entry{{ID: "k3f9", Organization: organization, Added: now}},
			wantRecord: []identity.Entry{{ID: "k3f9", Organization: organization, Added: now}},
		},
		{
			name: "another subscription's", config: sideAt, id: "k3f9", organization: "test-org-other",
			record:     []identity.Entry{{ID: "k3f9", Organization: organization, Added: now}},
			wantErr:    "the token is for another subscription, so nothing is saved: add it as an account of its own",
			wantRecord: []identity.Entry{{ID: "k3f9", Organization: organization, Added: now}},
		},
		{
			name: "one whose account's the record doesn't know, recorded", config: sideAt, id: "k3f9", organization: organization,
			record:     []identity.Entry{{ID: "k3f9", Added: now}},
			wantRecord: []identity.Entry{{ID: "k3f9", Organization: organization, Added: now}},
		},
		{
			name: "one whose account's the record doesn't know, another account's", config: sideAt, id: "k3f9", organization: organization,
			record:     []identity.Entry{{ID: "k3f9", Added: now}, spare},
			wantErr:    `the token is for another subscription, that of "Spare" (h7c2), so nothing is saved: add it as an account of its own`,
			wantRecord: []identity.Entry{{ID: "k3f9", Added: now}, spare},
		},
		{
			name: "one whose account's the record doesn't know, a configured account's, named by the config's label", config: sideAt + "\n[[account]]\nid    = \"m2x7\"\nlabel = \"Team\"\n",
			id: "k3f9", organization: organization,
			record:     []identity.Entry{{ID: "k3f9", Added: now}, {ID: "m2x7", Organization: organization, Added: now}},
			wantErr:    `the token is for another subscription, that of "Team" (m2x7), so nothing is saved: add it as an account of its own`,
			wantRecord: []identity.Entry{{ID: "k3f9", Added: now}, {ID: "m2x7", Organization: organization, Added: now}},
		},
		{
			name: "none named", config: sideAt, id: "k3f9",
			record:     []identity.Entry{{ID: "k3f9", Organization: organization, Added: now}},
			wantRecord: []identity.Entry{{ID: "k3f9", Organization: organization, Added: now}},
		},
		{
			name: "one the record hasn't the account of, from before ids were made", config: withSide, id: "side", organization: organization,
			record:     []identity.Entry{spare},
			wantRecord: []identity.Entry{spare},
		},
	}
	ways := []struct {
		name    string
		replace func(t *testing.T, w *world, r accounts.Registry, id, organization string) error
	}{
		{name: "given", replace: func(t *testing.T, w *world, r accounts.Registry, id, _ string) error {
			_, err := r.SetToken(t.Context(), w.load(t), id)
			return err
		}},
		{name: "checked", replace: func(t *testing.T, _ *world, r accounts.Registry, id, organization string) error {
			return r.SetTokenChecked(t.Context(), id, tokenOf(t, sideToken), accounts.Checked{Organization: organization})
		}},
	}
	for _, way := range ways {
		for _, tt := range tests {
			t.Run(way.name+", "+tt.name, func(t *testing.T) {
				w := newWorld(t, tt.config)
				w.writeRecord(t, tt.record...)
				w.writeToken(t, tt.id, "test-token-stale", 0o600)
				r := w.registry(&user{gives: sideToken}, &fakeAPI{organization: tt.organization})

				err := way.replace(t, w, r, tt.id, tt.organization)
				if errorText(err) != tt.wantErr {
					t.Fatalf("replacing %s's token: %v, want %q", tt.id, err, tt.wantErr)
				}
				if _, ok := errors.AsType[*accounts.OtherSubscriptionError](err); ok != (tt.wantErr != "") {
					t.Errorf("replacing %s's token: %#v, want an *OtherSubscriptionError where it's refused", tt.id, err)
				}
				want := sideToken
				if tt.wantErr != "" {
					want = "test-token-stale"
				}
				w.checkToken(t, tt.id, want)
				w.checkConfig(t, tt.config)
				w.checkRecord(t, tt.wantRecord...)
			})
		}
	}
}

func TestSetTokenRefusesAnAccountNotConfigured(t *testing.T) {
	w := newWorld(t, twoAccounts)
	user := &user{gives: sideToken}
	api := &fakeAPI{}

	_, err := w.registry(user, api).SetToken(t.Context(), w.load(t), "side")
	if want := `account "side" is not configured`; err == nil || err.Error() != want || !errors.Is(err, config.ErrNotConfigured) {
		t.Errorf("SetToken() error = %v, want %q", err, want)
	}
	if user.asked || len(api.asked) > 0 {
		t.Errorf("the user was asked: %v; the API about %q; want neither", user.asked, api.asked)
	}
	w.checkNoToken(t, "side")
}

func TestRemove(t *testing.T) {
	tests := []struct {
		name string
		// token is set when the account has a token file.
		token       bool
		wantRemoved bool
	}{
		{name: "with its token file", token: true, wantRemoved: true},
		{name: "without a token file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, withSide)
			w.writeToken(t, "work", workToken, 0o600)
			if tt.token {
				w.writeToken(t, "side", sideToken, 0o600)
			}

			removed, err := w.registry(&user{}, &fakeAPI{}).Remove("side")
			if want := (accounts.Removed{File: tt.wantRemoved}); err != nil || removed != want {
				t.Fatalf("Remove() = %+v, %v, want %+v", removed, err, want)
			}
			w.checkConfig(t, twoAccounts)
			w.checkNoToken(t, "side")
			w.checkToken(t, "work", workToken)
		})
	}
}

// The accounts record keeps an account removed, marked removed when it was,
// with the label it showed, configured or not, as by its id; an account it
// hasn't, as one from before ids were made, it records nothing of.
func TestRemoveMarksTheAccountRemovedInTheRecord(t *testing.T) {
	removing := now.Add(time.Hour)
	spare := identity.Entry{ID: "h7c2", Label: "Spare", Added: now, Removed: now}
	tests := []struct {
		name, config, id string
		record, want     []identity.Entry
	}{
		{
			name:   "an account the record has, labelled",
			config: sideAdded, id: sideID,
			record: []identity.Entry{{ID: sideID, Organization: organization, Added: now}, spare},
			want:   []identity.Entry{{ID: sideID, Organization: organization, Label: "Side", Added: now, Removed: removing}, spare},
		},
		{
			name:   "an account the record has, shown by its id",
			config: twoAccounts + "\n[[account]]\nid = \"" + sideID + "\"\n", id: sideID,
			record: []identity.Entry{{ID: sideID, Organization: organization, Added: now}},
			want:   []identity.Entry{{ID: sideID, Organization: organization, Label: sideID, Added: now, Removed: removing}},
		},
		{
			name:   "an account the record hasn't, from before ids were made",
			config: withSide, id: "side",
			record: []identity.Entry{spare},
			want:   []identity.Entry{spare},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, tt.config)
			w.writeRecord(t, tt.record...)
			r := w.registry(&user{}, &fakeAPI{})
			r.Now = func() time.Time { return removing }

			if _, err := r.Remove(tt.id); err != nil {
				t.Fatalf("Remove() error = %v", err)
			}
			w.checkConfig(t, twoAccounts)
			w.checkRecord(t, tt.want...)
		})
	}
}

// Nothing is removed where the accounts record can't be read, as where it's
// damaged: the account stays, as does its token.
func TestRemoveRemovesNothingWhereTheRecordIsDamaged(t *testing.T) {
	w := newWorld(t, withSide)
	w.writeToken(t, "side", sideToken, 0o600)
	if err := os.MkdirAll(w.state, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(identity.NewFile(w.state).Path(), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := w.registry(&user{}, &fakeAPI{}).Remove("side")
	if !errors.Is(err, identity.ErrDamaged) {
		t.Errorf("Remove() error = %v, want one matching %v", err, identity.ErrDamaged)
	}
	w.checkConfig(t, withSide)
	w.checkToken(t, "side", sideToken)
}

func TestRemoveSaysWhichAccountIsThePrimaryOnceTheOneThatWasGoes(t *testing.T) {
	tests := []struct {
		name   string
		config string
		// id is the account removed, and primary the primary once it's gone.
		id, primary string
		// wantPrimary is the primary Remove says of, "" for none.
		wantPrimary string
	}{
		{name: "the first, the primary for want of one marked", config: withSide, id: "work", primary: "personal", wantPrimary: "personal"},
		{
			name:        "the one marked, leaving the first the primary",
			config:      withSide + "primary = true\n",
			id:          "side",
			primary:     "work",
			wantPrimary: "work",
		},
		{name: "another than the primary", config: withSide, id: "side", primary: "work"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, tt.config)
			w.writeToken(t, tt.id, sideToken, 0o600)

			removed, err := w.registry(&user{}, &fakeAPI{}).Remove(tt.id)
			if want := (accounts.Removed{File: true, Primary: tt.wantPrimary}); err != nil || removed != want {
				t.Fatalf("Remove() = %+v, %v, want %+v", removed, err, want)
			}
			if got := w.load(t).Accounts.Primary().ID; got != tt.primary {
				t.Errorf("the config's primary is %s, want %s", got, tt.primary)
			}
		})
	}
}

func TestRemoveLeavesWhereATokenFilesLinkLeads(t *testing.T) {
	w := newWorld(t, withSide)
	// side's token file, a link to a file kept elsewhere, as by a dotfiles
	// step.
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "side")
	if err := os.WriteFile(target, []byte(sideToken+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(w.tokens().Path("side")), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, w.tokens().Path("side")); err != nil {
		t.Fatal(err)
	}

	removed, err := w.registry(&user{}, &fakeAPI{}).Remove("side")
	if want := (accounts.Removed{File: true, LinkedTo: target}); err != nil || removed != want {
		t.Fatalf("Remove() = %+v, %v, want %+v", removed, err, want)
	}
	w.checkNoToken(t, "side")
	if data, err := os.ReadFile(target); err != nil || string(data) != sideToken+"\n" {
		t.Errorf("where the link led holds %q (%v), want the token still", data, err)
	}
}

func TestRemoveRefuses(t *testing.T) {
	tests := []struct {
		name    string
		config  string
		id      string
		wantErr string
		wantIs  error
	}{
		{name: "an account not configured", config: twoAccounts, id: "side", wantErr: `account "side" is not configured`, wantIs: config.ErrNotConfigured},
		{name: "the only account", config: "[[account]]\nid = \"side\"\n", id: "side", wantErr: `account "side" is the only one, and a config needs one at least`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, tt.config)
			w.writeToken(t, "side", sideToken, 0o600)

			_, err := w.registry(&user{}, &fakeAPI{}).Remove(tt.id)
			if err == nil || err.Error() != tt.wantErr {
				t.Errorf("Remove() error = %v, want %q", err, tt.wantErr)
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("Remove() error = %v, want one matching %v", err, tt.wantIs)
			}
			w.checkConfig(t, tt.config)
			w.checkToken(t, "side", sideToken)
		})
	}
}

func TestRemoveThatCantDeleteTheTokenFileSaysTheAccountIsTakenOut(t *testing.T) {
	w := newWorld(t, withSide)
	// A directory, with something in it, where the token file goes.
	file := w.tokens().Path("side")
	if err := os.MkdirAll(file, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(file, "kept"), []byte("kept"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := w.registry(&user{}, &fakeAPI{}).Remove("side")
	left, ok := errors.AsType[*accounts.TokenFileLeft](err)
	want := "side is taken out of the config, but its token file, " + file + ", isn't deleted: directory not empty"
	if !ok || left.ID != "side" || err.Error() != want {
		t.Fatalf("Remove() error = %v, want a TokenFileLeft saying %q", err, want)
	}
	w.checkConfig(t, twoAccounts)
}

func TestChangesAreLoggedWithoutTheToken(t *testing.T) {
	w := newWorld(t, twoAccounts)
	log := logstest.Capture(t)
	registry := w.registry(&user{gives: sideToken}, &fakeAPI{organization: organization})

	if _, err := registry.Add(t.Context(), config.NewAccount{Label: "Side", Primary: true}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := registry.Remove(sideID); err != nil {
		t.Fatal(err)
	}
	for _, want := range [][]string{
		{"level=INFO", `msg="saved a token"`, "account=" + sideID, "checked=true"},
		{"level=INFO", `msg="added an account"`, "account=" + sideID, "primary=true", "returning=false"},
		{"level=INFO", `msg="removed an account"`, "account=" + sideID, "token_file=true"},
	} {
		if !log.Has(want...) {
			t.Errorf("the log reads\n%s\nwant a line with %q", log, want)
		}
	}
	if strings.Contains(log.String(), sideToken) {
		t.Errorf("the log shows the token:\n%s", log)
	}
}

// organization is a placeholder id of the organization an account is in.
const organization = "test-org-side"

func TestCheck(t *testing.T) {
	// echoed is an API error message that quotes the token, as no answer
	// should, to show none of it gets through.
	echoed := "bad bearer token " + sideToken
	tests := []struct {
		name    string
		answer  answerWith
		want    accounts.Checked
		wantErr string
		wantIs  error
	}{
		{name: "taken, naming its organization", answer: answerWith{status: http.StatusOK, organization: organization}, want: accounts.Checked{Organization: organization}},
		{name: "taken, naming no organization", answer: answerWith{status: http.StatusOK}, want: accounts.Checked{}},
		{
			name:    "refused as a token, answering 401",
			answer:  answerWith{status: http.StatusUnauthorized, message: echoed},
			wantErr: "the API refused the token (HTTP 401 · bad bearer token [redacted]), so nothing is saved: make another with claude setup-token, run while signed in to that subscription",
			wantIs:  accounts.ErrRefused,
		},
		{
			name:    "refused as a request, answering 403",
			answer:  answerWith{status: http.StatusForbidden, message: echoed},
			wantErr: "the API refused the token (HTTP 403 · bad bearer token [redacted]), so nothing is saved: make another with claude setup-token, run while signed in to that subscription",
			wantIs:  accounts.ErrRefused,
		},
		{
			name:   "unanswered, the API failing",
			answer: answerWith{status: http.StatusInternalServerError, message: echoed},
			want:   accounts.Checked{Unchecked: true, Why: "HTTP 500 · bad bearer token [redacted]"},
		},
		{
			name:   "unanswered, the API overloaded",
			answer: answerWith{status: 529},
			want:   accounts.Checked{Unchecked: true, Why: "HTTP 529"},
		},
		{
			name:   "unanswered, the API taking too long",
			answer: answerWith{hang: true},
			want:   accounts.Checked{Unchecked: true, Why: "timed out after 50ms"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, strings.Replace(twoAccounts, upstream, newRealAPI(t, tt.answer), 1))
			log := logstest.Capture(t)

			got, err := realRegistry(w).Check(t.Context(), tokenOf(t, sideToken))
			if got != tt.want || errorText(err) != tt.wantErr {
				t.Errorf("Check() = %+v, %v, want %+v, %q", got, err, tt.want, tt.wantErr)
			}
			if tt.wantIs != nil && !errors.Is(err, tt.wantIs) {
				t.Errorf("Check() error = %v, want one matching %v", err, tt.wantIs)
			}
			checkNoneOfTheToken(t, sideToken, got.Why, errorText(err), log.String())
		})
	}
}

func TestCheckWithNoNetwork(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	w := newWorld(t, fmt.Sprintf("upstream = %q\n", srv.URL))
	log := logstest.Capture(t)

	got, err := realRegistry(w).Check(t.Context(), tokenOf(t, sideToken))
	if err != nil || !got.Unchecked || got.Why == "" {
		t.Errorf("Check() = %+v, %v, want it unchecked, saying why", got, err)
	}
	checkNoneOfTheToken(t, sideToken, got.Why, log.String())
}

func TestCheckAsksTheConfigsUpstream(t *testing.T) {
	tests := []struct {
		name   string
		config string
		want   string
	}{
		{name: "the config's, its accounts set up", config: twoAccounts, want: upstream},
		{name: "the config's, before any account", config: fmt.Sprintf("upstream = %q\n", upstream), want: upstream},
		{name: "the default, without a config file", want: "https://api.anthropic.com"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, tt.config)
			api := &fakeAPI{}

			if _, err := w.registry(&user{}, api).Check(t.Context(), tokenOf(t, sideToken)); err != nil {
				t.Fatalf("Check() error = %v", err)
			}
			if want := []string{tt.want + " " + sideToken}; !slices.Equal(api.asked, want) {
				t.Errorf("the API was asked about %q, want %q", api.asked, want)
			}
		})
	}
}

func TestCheckRefusesAnUpstreamAConfigCantHave(t *testing.T) {
	w := newWorld(t, "upstream = \"http://192.0.2.1:4700\"\n")
	api := &fakeAPI{}

	_, err := w.registry(&user{}, api).Check(t.Context(), tokenOf(t, sideToken))
	if err == nil || !strings.HasPrefix(err.Error(), "invalid config "+w.config+":\nupstream ") {
		t.Errorf("Check() error = %v, want the upstream refused", err)
	}
	if len(api.asked) > 0 {
		t.Errorf("the API was asked about %q, want nothing", api.asked)
	}
}

// checkedWrites are the ways to save a token Check has checked: with the
// account it's added for, under the id its organization derives, or as an
// account's token in place of the one it had. Each writes to id's token
// file, and leaves the config reading want. savesUnchecked is set where it
// saves a token the API didn't answer for.
var checkedWrites = []struct {
	name, config, id, want string
	savesUnchecked         bool
	write                  func(ctx context.Context, r accounts.Registry, token tokens.Token, checked accounts.Checked) error
}{
	{
		name:   "AddChecked",
		config: twoAccounts, id: sideID, want: sideAdded,
		write: func(ctx context.Context, r accounts.Registry, token tokens.Token, checked accounts.Checked) error {
			_, err := r.AddChecked(ctx, config.NewAccount{Label: "Side"}, token, checked, nil, accounts.Answer{})
			return err
		},
	},
	{
		name:   "SetTokenChecked",
		config: withSide, id: "side", want: withSide,
		savesUnchecked: true,
		write: func(ctx context.Context, r accounts.Registry, token tokens.Token, checked accounts.Checked) error {
			return r.SetTokenChecked(ctx, "side", token, checked)
		},
	},
}

func TestTheCheckedWritesSaveWithoutAskingTheAPI(t *testing.T) {
	checks := []struct {
		name    string
		checked accounts.Checked
		// wantWarned is set when the write warns the token went unchecked.
		wantWarned bool
	}{
		{name: "checked", checked: accounts.Checked{Organization: organization}},
		{name: "unchecked", checked: accounts.Checked{Unchecked: true, Why: "timed out after 5s"}, wantWarned: true},
	}
	for _, write := range checkedWrites {
		for _, c := range checks {
			if c.checked.Unchecked && !write.savesUnchecked {
				continue
			}
			t.Run(write.name+", "+c.name, func(t *testing.T) {
				w := newWorld(t, write.config)
				w.writeToken(t, write.id, "test-token-stale", 0o600)
				log := logstest.Capture(t)
				user := &user{atTerminal: true}

				if err := write.write(t.Context(), w.unasked(t, user), tokenOf(t, sideToken), c.checked); err != nil {
					t.Fatalf("%s() error = %v", write.name, err)
				}
				w.checkConfig(t, write.want)
				w.checkToken(t, write.id, sideToken)
				w.checkTaken(t, write.id, sideToken, now)
				warned := log.Has("level=WARN", `msg="couldn't check a token with the API"`, "account="+write.id, `error="timed out after 5s"`)
				if warned != c.wantWarned {
					t.Errorf("the log reads\n%s\nwant the unchecked warning: %v", log, c.wantWarned)
				}
				if want := fmt.Sprintf("checked=%t", !c.checked.Unchecked); !log.Has("level=INFO", `msg="saved a token"`, "account="+write.id, want) {
					t.Errorf("the log reads\n%s\nwant the token saved, %s", log, want)
				}
				if user.asked {
					t.Error("the user was asked for a token, want the one given taken")
				}
			})
		}
	}
}

// AddChecked saves nothing where the API didn't answer for the token, as an
// account isn't added until its organization is known; nor where the account
// isn't one CheckNew takes.
func TestAddCheckedRefuses(t *testing.T) {
	tests := []struct {
		name    string
		label   string
		checked accounts.Checked
		wantIs  error
	}{
		{name: "a token the API didn't answer for", label: "Side", checked: accounts.Checked{Unchecked: true, Why: "timed out after 5s"}},
		{name: "no label", checked: accounts.Checked{Organization: organization}, wantIs: accounts.ErrNoLabel},
		{name: "another account's label", label: "Work", checked: accounts.Checked{Organization: organization}, wantIs: accounts.ErrLabelTaken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, twoAccounts)

			_, err := w.unasked(t, &user{}).AddChecked(t.Context(), config.NewAccount{Label: tt.label}, tokenOf(t, sideToken), tt.checked, nil, accounts.Answer{})
			_, unanswered := errors.AsType[*accounts.UnansweredError](err)
			if err == nil || unanswered != tt.checked.Unchecked || (tt.wantIs != nil && !errors.Is(err, tt.wantIs)) {
				t.Errorf("AddChecked() error = %v, want one matching %v, an UnansweredError where the API didn't answer", err, tt.wantIs)
			}
			w.checkConfig(t, twoAccounts)
			w.checkNoToken(t, sideID)
			w.checkNoRecord(t)
		})
	}
}

func TestSetTokenCheckedRefusesAnAccountNotConfigured(t *testing.T) {
	for _, given := range []string{twoAccounts, ""} {
		w := newWorld(t, given)

		err := w.unasked(t, &user{}).SetTokenChecked(t.Context(), "side", tokenOf(t, sideToken), accounts.Checked{})
		if want := `account "side" is not configured`; err == nil || err.Error() != want || !errors.Is(err, config.ErrNotConfigured) {
			t.Errorf("SetTokenChecked() with the config %q: error = %v, want %q", given, err, want)
		}
		w.checkNoToken(t, "side")
	}
}

// givingUp are the ways a caller gives up on a check under way: cancelling
// it once the API is asked, or a deadline sooner than the prober's own.
var givingUp = []struct {
	name   string
	ctx    func(t *testing.T, asked <-chan struct{}) context.Context
	wantIs error
}{
	{
		name: "cancelled mid-probe",
		ctx: func(t *testing.T, asked <-chan struct{}) context.Context {
			ctx, cancel := context.WithCancel(t.Context())
			t.Cleanup(cancel)
			go func() {
				<-asked
				cancel()
			}()
			return ctx
		},
		wantIs: context.Canceled,
	},
	{
		name: "a deadline passed",
		ctx: func(t *testing.T, _ <-chan struct{}) context.Context {
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
			t.Cleanup(cancel)
			return ctx
		},
		wantIs: context.DeadlineExceeded,
	},
}

func TestCheckFailsOnceTheCallerGivesUp(t *testing.T) {
	for _, up := range givingUp {
		t.Run(up.name, func(t *testing.T) {
			asked := make(chan struct{}, 1)
			w := newWorld(t, strings.Replace(twoAccounts, upstream, newRealAPI(t, answerWith{hang: true, asked: asked}), 1))

			got, err := patientRegistry(w).Check(up.ctx(t, asked), tokenOf(t, sideToken))
			if !errors.Is(err, up.wantIs) || !strings.HasPrefix(errorText(err), "check the token with the API: ") || got != (accounts.Checked{}) {
				t.Errorf("Check() = %+v, %v, want the caller's error, %v", got, err, up.wantIs)
			}
		})
	}
}

func TestTheCheckedWritesWriteNothingOnceTheContextIsDone(t *testing.T) {
	for _, write := range checkedWrites {
		t.Run(write.name, func(t *testing.T) {
			w := newWorld(t, write.config)
			w.writeToken(t, write.id, "test-token-stale", 0o600)
			ctx, cancel := context.WithCancel(t.Context())
			cancel()

			err := write.write(ctx, w.unasked(t, &user{}), tokenOf(t, sideToken), accounts.Checked{Organization: organization})
			if !errors.Is(err, context.Canceled) {
				t.Errorf("%s() error = %v, want one matching %v", write.name, err, context.Canceled)
			}
			w.checkConfig(t, write.config)
			w.checkToken(t, write.id, "test-token-stale")
			w.checkNoRecord(t)
		})
	}
}

// unasked returns the world's registry, as registry does, whose prober
// fails the test if it's asked anything.
func (w *world) unasked(t *testing.T, user *user) accounts.Registry {
	r := w.registry(user, nil)
	r.Prober = func(upstream string) accounts.Prober {
		t.Errorf("the API at %s was asked about a token, want nothing", upstream)
		return prober{api: &fakeAPI{}, upstream: upstream}
	}
	return r
}

func TestTheTokenGivenNeverShows(t *testing.T) {
	w := newWorld(t, withSide)
	log := logstest.Capture(t)
	quoting := errors.New("HTTP 500 · bad bearer token " + sideToken)
	registry := w.registry(&user{}, &fakeAPI{err: quoting})
	token := tokenOf(t, sideToken)

	checked, checkErr := registry.Check(t.Context(), token)
	if checkErr != nil {
		t.Fatal(checkErr)
	}
	_, unanswered := registry.AddChecked(t.Context(), config.NewAccount{Label: "Team"}, token, checked, nil, accounts.Answer{})
	if _, ok := errors.AsType[*accounts.UnansweredError](unanswered); !ok {
		t.Fatalf("AddChecked() error = %v, want the API's not answering said", unanswered)
	}
	typed := w.registry(&user{gives: sideToken}, &fakeAPI{err: quoting})
	retaken, retakeErr := typed.SetToken(t.Context(), w.load(t), "side")
	if retakeErr != nil {
		t.Fatal(retakeErr)
	}
	_, refuseErr := w.registry(&user{}, &fakeAPI{err: quotingRefusal{}}).Check(t.Context(), token)
	if !errors.Is(refuseErr, accounts.ErrRefused) {
		t.Errorf("Check() error = %v, want the token refused", refuseErr)
	}
	for _, why := range []string{checked.Why, errorText(retaken.Unchecked)} {
		if !strings.HasPrefix(why, "HTTP 500") {
			t.Errorf("why the token went unchecked = %q, want the API's failure", why)
		}
	}
	checkNoneOfTheToken(t, sideToken, checked.Why, errorText(unanswered), errorText(retaken.Unchecked), errorText(refuseErr), log.String())
}

// checkNoneOfTheToken checks that none of what's shown holds the token, or
// its first or last characters, as cutting it short from either end would
// leave them.
func checkNoneOfTheToken(t *testing.T, token string, shown ...string) {
	t.Helper()
	const part = 8
	for _, s := range shown {
		for _, piece := range []string{token, token[:part], token[len(token)-part:]} {
			if strings.Contains(s, piece) {
				t.Errorf("%q shows the token, or part of it: %q", s, piece)
			}
		}
	}
}

// quotingRefusal is the API refusing a token, its failure quoting it.
type quotingRefusal struct{}

func (quotingRefusal) Error() string { return "HTTP 401 · bad bearer token " + sideToken }

func (quotingRefusal) Refused() bool { return true }

// answerWith is how a messages API answers each probe: with a status, and
// the usage headers of a 200, the organization named, and an API error's
// message, where given; or, hanging, not until the probe gives up.
type answerWith struct {
	status       int
	organization string
	message      string
	hang         bool
	// asked, where given, is told when the API is first asked, unless it's
	// told already.
	asked chan<- struct{}
}

// newRealAPI serves a messages API over loopback that answers every probe
// as a says, and returns its URL.
func newRealAPI(t *testing.T, a answerWith) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		select {
		case a.asked <- struct{}{}:
		default:
		}
		if a.hang {
			<-r.Context().Done()
			return
		}
		if a.organization != "" {
			w.Header().Set("anthropic-organization-id", a.organization)
		}
		if a.status == http.StatusOK {
			for _, key := range []string{"5h", "7d", "7d_oi"} {
				w.Header().Set("anthropic-ratelimit-unified-"+key+"-utilization", "0.25")
			}
		}
		w.WriteHeader(a.status)
		if a.message != "" {
			_, _ = fmt.Fprintf(w, `{"type":"error","error":{"type":"api_error","message":%q}}`, a.message)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

// realRegistry is the world's registry, checking tokens with the Claude
// provider's prober, which gives up on a probe after 50ms.
func realRegistry(w *world) accounts.Registry {
	return registryProbingWithin(w, 50*time.Millisecond)
}

// patientRegistry is realRegistry with a prober that waits a minute, so the
// caller gives up first however slowly a loaded machine runs its cancel.
func patientRegistry(w *world) accounts.Registry {
	return registryProbingWithin(w, time.Minute)
}

func registryProbingWithin(w *world, limit time.Duration) accounts.Registry {
	return accounts.Registry{
		ConfigPath: w.config,
		Tokens:     w.tokens(),
		Record:     identity.NewFile(w.state),
		Prober: func(upstream string) accounts.Prober {
			return &claude.Prober{Upstream: upstream, Timeout: limit}
		},
		Now: func() time.Time { return now },
	}
}

// world is where a registry keeps the accounts: a config file and a state
// directory, the test's own.
type world struct {
	config string
	state  string
}

// newWorld returns a world whose config holds config, or that has no config
// file when config is empty.
func newWorld(t *testing.T, config string) *world {
	t.Helper()
	dir := t.TempDir()
	w := &world{config: filepath.Join(dir, "config.toml"), state: filepath.Join(dir, "state")}
	if config != "" {
		if err := os.WriteFile(w.config, []byte(config), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return w
}

// registry returns the world's registry, which takes tokens from user and
// checks them with api.
func (w *world) registry(user *user, api *fakeAPI) accounts.Registry {
	return accounts.Registry{
		ConfigPath: w.config, Tokens: w.tokens(), Record: identity.NewFile(w.state),
		Prober: api.proberAt, Input: user.input(), Now: func() time.Time { return now }, Random: drawn(),
	}
}

// drawn is what the world's random ids are drawn from: the same each time.
func drawn() io.Reader {
	return strings.NewReader(strings.Repeat("switchboard", 100))
}

// randomID is the id the world's registry draws first.
func randomID(t *testing.T) string {
	t.Helper()
	id, err := identity.Record{}.Random(drawn(), nil)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// writeRecord writes the accounts record, holding the accounts given.
func (w *world) writeRecord(t *testing.T, entries ...identity.Entry) {
	t.Helper()
	err := identity.NewFile(w.state).Update(func(r *identity.Record) error {
		r.Accounts = entries
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// checkRecord checks the accounts record holds the accounts wanted, in
// order.
func (w *world) checkRecord(t *testing.T, want ...identity.Entry) {
	t.Helper()
	r, err := identity.NewFile(w.state).Read()
	if err != nil || !slices.EqualFunc(r.Accounts, want, sameEntry) {
		t.Errorf("the accounts record holds %+v (%v), want %+v", r.Accounts, err, want)
	}
}

// sameEntry reports whether a and b are the same account, as the record keeps
// it.
func sameEntry(a, b identity.Entry) bool {
	return a.ID == b.ID && a.Organization == b.Organization && a.Label == b.Label && a.Was == b.Was &&
		a.Added.Equal(b.Added) && a.Removed.Equal(b.Removed)
}

// checkNoRecord checks nothing wrote the accounts record.
func (w *world) checkNoRecord(t *testing.T) {
	t.Helper()
	if _, err := os.Stat(identity.NewFile(w.state).Path()); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("the accounts record: %v, want none written", err)
	}
}

func (w *world) tokens() tokens.Store {
	return tokens.NewStore(w.state, os.Getuid())
}

func (w *world) load(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load(w.config)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func (w *world) writeToken(t *testing.T, id, secret string, perm fs.FileMode) {
	t.Helper()
	path := w.tokens().Path(id)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(secret+"\n"), perm); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, perm); err != nil {
		t.Fatal(err)
	}
}

func (w *world) checkConfig(t *testing.T, want string) {
	t.Helper()
	data, err := os.ReadFile(w.config)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != want {
		t.Errorf("the config reads\n%s\nwant\n%s", data, want)
	}
}

func (w *world) checkToken(t *testing.T, id, want string) {
	t.Helper()
	token, err := w.tokens().Read(id)
	if err != nil || token.Reveal() != want {
		t.Errorf("%s's token file holds %q (%v), want %q", id, token.Reveal(), err, want)
	}
}

// checkTaken checks the account's record says its token, secret, was taken
// at want: that it says nothing of it where want is zero.
func (w *world) checkTaken(t *testing.T, id, secret string, want time.Time) {
	t.Helper()
	if at, err := w.tokens().Taken(id, tokenOf(t, secret)); !at.Equal(want) || (err == nil) == want.IsZero() {
		t.Errorf("%s's token was taken at %v (%v), want %v", id, at, err, want)
	}
}

// tokenOf is the token secret is.
func tokenOf(t *testing.T, secret string) tokens.Token {
	t.Helper()
	token, err := tokens.Parse(secret)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func (w *world) checkNoToken(t *testing.T, id string) {
	t.Helper()
	if _, err := os.Lstat(w.tokens().Path(id)); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%s's token file: %v, want none", id, err)
	}
}

// user gives a token: typed at a terminal, when they're at one, or piped in.
type user struct {
	atTerminal bool
	gives      string
	// fails is what reading what they give fails with.
	fails error
	// shown is what they're shown, at a terminal.
	shown strings.Builder
	asked bool
	piped *strings.Reader
}

func (u *user) input() accounts.Input {
	if u.atTerminal {
		return accounts.Input{Hidden: u.typeIt, Prompt: &u.shown}
	}
	return accounts.Input{Piped: u}
}

// typeIt types what the user gives, unseen, as a terminal reads a line: without
// the newline that ends it.
func (u *user) typeIt() ([]byte, error) {
	u.asked = true
	return []byte(strings.TrimSuffix(u.gives, "\n")), u.fails
}

// Read pipes in what the user gives.
func (u *user) Read(p []byte) (int, error) {
	u.asked = true
	if u.fails != nil {
		return 0, u.fails
	}
	if u.piped == nil {
		u.piped = strings.NewReader(u.gives)
	}
	return u.piped.Read(p)
}

// fakeAPI answers every probe with err: nil takes the token, naming the
// organization given. It notes each token it's asked about, after the
// upstream the probe was made for.
type fakeAPI struct {
	err          error
	organization string
	asked        []string
}

func (a *fakeAPI) proberAt(upstream string) accounts.Prober {
	return prober{api: a, upstream: upstream}
}

type prober struct {
	api      *fakeAPI
	upstream string
}

func (p prober) Probe(_ context.Context, token string) (quota.Probe, error) {
	p.api.asked = append(p.api.asked, p.upstream+" "+token)
	if p.api.err != nil {
		return quota.Probe{}, p.api.err
	}
	return quota.Probe{Organization: p.api.organization}, nil
}

// refusal is a probe's failure as the API refuses the token, as the Claude
// provider reads one.
type refusal struct{}

func (refusal) Error() string { return "HTTP 401 · Invalid bearer token" }

func (refusal) Refused() bool { return true }

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
