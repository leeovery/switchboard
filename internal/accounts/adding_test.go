package accounts_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/identity"
)

// before is when the accounts the record has were added, and the removed ones
// removed.
var before = now.Add(-30 * 24 * time.Hour)

// Adding an account comes to what the organization its token's answer named
// says, as the accounts record and the configured accounts' organizations
// known have it: a new account, under the id it derives, or a random one
// where none was named; a removed account coming back under its id, with the
// label given now, its history carrying on; one configured already, refused,
// named by its label and id; or, where the organization doesn't settle it,
// the question of which account it is, answered by a new account or one of
// its choices, every account the record has, configured no longer, where
// none was named, or those whose organization isn't known where one was.
func TestAddCheckedDecidesTheIDByTheOrganization(t *testing.T) {
	spare := identity.Entry{ID: "h7c2", Label: "Spare", Added: before, Removed: before}
	side := identity.Entry{ID: "m2x7", Organization: "test-org-other", Label: "Old side", Added: before, Removed: before}
	withAt := func(id string) string {
		return twoAccounts + "\n[[account]]\nid    = \"" + id + "\"\nlabel = \"Side\"\n"
	}
	tests := []struct {
		name string
		// config is the config before, twoAccounts where it's "".
		config       string
		record       []identity.Entry
		known        accounts.Organizations
		organization string
		answer       accounts.Answer
		want         accounts.Added
		wantErr      error
		// wantRecord is the record after: as it was, where the account isn't
		// added.
		wantRecord []identity.Entry
	}{
		{
			name:         "a new account, under the id its organization derives",
			record:       []identity.Entry{side},
			organization: organization,
			want:         accounts.Added{ID: sideID},
			wantRecord:   []identity.Entry{side, {ID: sideID, Organization: organization, Added: now}},
		},
		{
			name:         "a removed account's, back under its id",
			record:       []identity.Entry{{ID: sideID, Organization: organization, Label: "Old side", Added: before, Removed: before}},
			organization: organization,
			want:         accounts.Added{ID: sideID, Returning: true},
			wantRecord:   []identity.Entry{{ID: sideID, Organization: organization, Added: before}},
		},
		{
			name:         "a configured account's, as its token's answer named it",
			known:        accounts.Organizations{"work": organization},
			organization: organization,
			wantErr:      &accounts.ConfiguredError{ID: "work", Label: "Work"},
		},
		{
			name:         "a configured account's, as the record has it",
			config:       twoAccounts + "\n[[account]]\nid    = \"k3f9\"\nlabel = \"Team\"\n",
			record:       []identity.Entry{{ID: "k3f9", Organization: organization, Added: before}},
			organization: organization,
			wantErr:      &accounts.ConfiguredError{ID: "k3f9", Label: "Team"},
		},
		{
			name:         "an organization the record hasn't, where it has accounts of none known, unanswered",
			record:       []identity.Entry{spare, side},
			organization: organization,
			wantErr:      &accounts.QuestionError{Question: accounts.Question{Named: true, Choices: []identity.Entry{spare}}},
		},
		{
			name:         "an organization the record hasn't, answered a new account",
			record:       []identity.Entry{spare},
			organization: organization,
			answer:       accounts.Answer{New: true},
			want:         accounts.Added{ID: sideID},
			wantRecord:   []identity.Entry{spare, {ID: sideID, Organization: organization, Added: now}},
		},
		{
			name:         "an organization the record hasn't, answered one of none known, in another case",
			record:       []identity.Entry{spare},
			organization: organization,
			answer:       accounts.Answer{ID: "H7C2"},
			want:         accounts.Added{ID: "h7c2", Returning: true},
			wantRecord:   []identity.Entry{{ID: "h7c2", Organization: organization, Added: before}},
		},
		{
			name:         "an organization the record hasn't, answered an account it can't be",
			record:       []identity.Entry{spare, side},
			organization: organization,
			answer:       accounts.Answer{ID: "m2x7"},
			wantErr:      &accounts.NotAChoiceError{ID: "m2x7", Question: accounts.Question{Named: true, Choices: []identity.Entry{spare}}},
		},
		{
			name:         "an organization the record hasn't, where it has no accounts of none known, answered an account, as one asked of before",
			record:       []identity.Entry{side},
			organization: organization,
			answer:       accounts.Answer{ID: "h7c2"},
			wantErr:      &accounts.NotAChoiceError{ID: "h7c2", Question: accounts.Question{Named: true}},
		},
		{
			name:       "none named, with no account the record has, a new account under a random id",
			want:       accounts.Added{ID: "random"},
			wantRecord: []identity.Entry{{ID: "random", Added: now}},
		},
		{
			name:    "none named, unanswered",
			record:  []identity.Entry{side},
			wantErr: &accounts.QuestionError{Question: accounts.Question{Choices: []identity.Entry{side}}},
		},
		{
			name:       "none named, answered a new account",
			record:     []identity.Entry{side},
			answer:     accounts.Answer{New: true},
			want:       accounts.Added{ID: "random"},
			wantRecord: []identity.Entry{side, {ID: "random", Added: now}},
		},
		{
			name:       "none named, answered an account the record has, its organization kept",
			record:     []identity.Entry{side},
			answer:     accounts.Answer{ID: "m2x7"},
			want:       accounts.Added{ID: "m2x7", Returning: true},
			wantRecord: []identity.Entry{{ID: "m2x7", Organization: "test-org-other", Added: before}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			was := tt.config
			if was == "" {
				was = twoAccounts
			}
			w := newWorld(t, was)
			w.writeRecord(t, tt.record...)
			want, wantRecord := tt.want, tt.wantRecord
			if want.ID == "random" {
				want.ID = randomID(t)
				wantRecord = slices.Clone(wantRecord)
				wantRecord[len(wantRecord)-1].ID = want.ID
			}

			got, err := w.unasked(t, &user{}).AddChecked(t.Context(), accounts.Adding{Account: config.NewAccount{Label: "Side"}}, tokenOf(t, sideToken),
				accounts.Checked{Organization: tt.organization}, tt.known, tt.answer)
			if got != want || !sameError(err, tt.wantErr) {
				t.Fatalf("AddChecked() = %+v, %#v; want %+v, %#v", got, err, want, tt.wantErr)
			}
			if tt.wantErr != nil {
				w.checkConfig(t, was)
				w.checkRecord(t, tt.record...)
				return
			}
			w.checkConfig(t, withAt(want.ID))
			w.checkToken(t, want.ID, sideToken)
			w.checkRecord(t, wantRecord...)
		})
	}
}

// sameError reports whether got is the error wanted, an adding's: of its
// type, with its fields, and saying the same.
func sameError(got, want error) bool {
	if got == nil || want == nil {
		return got == want
	}
	if got.Error() != want.Error() {
		return false
	}
	switch want := want.(type) {
	case *accounts.QuestionError:
		q, ok := errors.AsType[*accounts.QuestionError](got)
		return ok && sameQuestion(q.Question, want.Question)
	case *accounts.NotAChoiceError:
		n, ok := errors.AsType[*accounts.NotAChoiceError](got)
		return ok && n.ID == want.ID && sameQuestion(n.Question, want.Question)
	case *accounts.ConfiguredError:
		c, ok := errors.AsType[*accounts.ConfiguredError](got)
		return ok && *c == *want
	}
	return false
}

// sameQuestion reports whether a and b ask the same.
func sameQuestion(a, b accounts.Question) bool {
	return a.Named == b.Named && slices.EqualFunc(a.Choices, b.Choices, sameEntry)
}

// What adding refuses with says why, naming what it names.
func TestAddingsRefusalsSayWhy(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{err: &accounts.ConfiguredError{ID: "k3f9", Label: "Side"}, want: `the token is for "Side" (k3f9), configured already, so nothing is saved`},
		{err: &accounts.QuestionError{Question: accounts.Question{Named: true}}, want: "the token's subscription is none this Mac's accounts are known to have, but may be one removed before switchboard made ids, so which it is is yours to say"},
		{err: &accounts.QuestionError{}, want: "the API named no organization for the token, so whether it's a new account or one this Mac had is yours to say"},
		{err: &accounts.NotAChoiceError{ID: "zzzz"}, want: `"zzzz" isn't one of the accounts the token may be`},
	}
	for _, tt := range tests {
		if got := tt.err.Error(); got != tt.want {
			t.Errorf("%#v says %q, want %q", tt.err, got, tt.want)
		}
	}
}

// Where the organization doesn't settle which account the token is, Add
// asks, once nothing is held, and adds the account as the answer says; it
// fails as asking fails, saving nothing, and with the question where there's
// no one to ask. It asks nothing where the organization settles it.
func TestAddAsksWhereTheOrganizationDoesntSettleIt(t *testing.T) {
	spare := identity.Entry{ID: "h7c2", Label: "Spare", Added: before, Removed: before}
	cantAsk := errors.New("stdin isn't a terminal")
	tests := []struct {
		name   string
		record []identity.Entry
		answer accounts.Answer
		// failing is what asking fails with, and noAsk set where there's no
		// one to ask.
		failing error
		noAsk   bool
		want    accounts.Added
		wantErr error
		// wantAsked is the question asked: nil for none.
		wantAsked *accounts.Question
	}{
		{
			name:      "answered one of its choices",
			record:    []identity.Entry{spare},
			answer:    accounts.Answer{ID: "h7c2"},
			want:      accounts.Added{ID: "h7c2", Returning: true},
			wantAsked: &accounts.Question{Named: true, Choices: []identity.Entry{spare}},
		},
		{
			name:      "unanswered, as it can't be asked",
			record:    []identity.Entry{spare},
			failing:   cantAsk,
			wantErr:   cantAsk,
			wantAsked: &accounts.Question{Named: true, Choices: []identity.Entry{spare}},
		},
		{
			name:    "with no one to ask",
			record:  []identity.Entry{spare},
			noAsk:   true,
			wantErr: &accounts.QuestionError{Question: accounts.Question{Named: true, Choices: []identity.Entry{spare}}},
		},
		{
			name: "settled by the organization",
			want: accounts.Added{ID: sideID},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t, twoAccounts)
			w.writeRecord(t, tt.record...)
			var asked []accounts.Question
			ask := func(q accounts.Question) (accounts.Answer, error) {
				asked = append(asked, q)
				return tt.answer, tt.failing
			}
			if tt.noAsk {
				ask = nil
			}

			got, err := w.registry(&user{gives: sideToken}, &fakeAPI{organization: organization}).Add(t.Context(), config.NewAccount{Label: "Side"}, ask)
			if got != tt.want || (err == nil) != (tt.wantErr == nil) || err != nil && err.Error() != tt.wantErr.Error() {
				t.Fatalf("Add() = %+v, %v; want %+v, %v", got, err, tt.want, tt.wantErr)
			}
			switch {
			case tt.wantAsked == nil && len(asked) > 0:
				t.Errorf("Add() asked %+v, want nothing asked", asked)
			case tt.wantAsked != nil && (len(asked) != 1 || !sameQuestion(asked[0], *tt.wantAsked)):
				t.Errorf("Add() asked %+v, want %+v asked once", asked, *tt.wantAsked)
			}
			if tt.wantErr != nil {
				w.checkConfig(t, twoAccounts)
				w.checkRecord(t, tt.record...)
			}
		})
	}
}
