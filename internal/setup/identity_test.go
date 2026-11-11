package setup_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/setup"
)

// recordedBefore is when the accounts the tests' records have were added,
// and the removed ones removed.
var recordedBefore = time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

// spareRemoved is an account removed before switchboard made ids, its
// organization not known; otherRemoved one removed, of an organization
// known.
var (
	spareRemoved = identity.Entry{ID: "h7c2", Label: "Spare", Added: recordedBefore, Removed: recordedBefore, Was: "spare"}
	otherRemoved = identity.Entry{ID: "m2x7", Organization: "org-other", Label: "Old", Added: recordedBefore, Removed: recordedBefore}
)

// withTeam is doneConfig with team, labelled Team, configured under an id
// switchboard made.
const withTeam = doneConfig + "\n[[account]]\nid    = \"k3f9\"\nlabel = \"Team\"\n"

// writeRecord writes the accounts record, holding the accounts given.
func (w *world) writeRecord(t *testing.T, entries ...identity.Entry) {
	t.Helper()
	err := w.record().Update(func(r *identity.Record) error {
		r.Accounts = entries
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// readRecord returns the accounts the record holds.
func (w *world) readRecord(t *testing.T) []identity.Entry {
	t.Helper()
	r, err := w.record().Read()
	if err != nil {
		t.Fatal(err)
	}
	return r.Accounts
}

// A token made to add an account is saved, or not, as its organization
// says, as accounts add has it: a removed account's comes back under its id;
// a configured account's is refused, naming it by its label; and where the
// organization doesn't settle which account it is, the token's held, given
// back to copy should setup go, for the user to say, nothing written.
func TestMakeTokenToAddDecidesByTheOrganization(t *testing.T) {
	personalID := derivedID("org-personal")
	tests := []struct {
		name         string
		config       string
		record       []identity.Entry
		organization string
		want         setup.Tokened
		// wantAsked is the question asked, at TokenAsked.
		wantAsked accounts.Question
		// wantRecord is the record once it's saved.
		wantRecord []identity.Entry
	}{
		{
			name:         "a removed account's, back under its id",
			config:       doneConfig,
			record:       []identity.Entry{{ID: personalID, Organization: "org-personal", Label: "Old", Added: recordedBefore, Removed: recordedBefore}},
			organization: "org-personal",
			want:         setup.Tokened{Result: setup.TokenSaved, Checked: accounts.Checked{Organization: "org-personal"}, ID: personalID},
			wantRecord:   []identity.Entry{{ID: personalID, Organization: "org-personal", Added: recordedBefore}},
		},
		{
			name:         "a configured account's, as the record has it, named by its label",
			config:       withTeam,
			record:       []identity.Entry{{ID: "k3f9", Organization: "org-personal", Added: recordedBefore}},
			organization: "org-personal",
			want:         setup.Tokened{Result: setup.TokenSameAccount, SameAs: "Team"},
		},
		{
			name:         "an organization none's known to have, but one removed may: asked",
			config:       doneConfig,
			record:       []identity.Entry{spareRemoved, otherRemoved},
			organization: "org-personal",
			want:         setup.Tokened{Result: setup.TokenAsked, Shown: []string{madeToken}},
			wantAsked:    accounts.Question{Named: true, Choices: []identity.Entry{spareRemoved}},
		},
		{
			name:      "no organization named, with accounts removed: asked",
			config:    doneConfig,
			record:    []identity.Entry{otherRemoved},
			want:      setup.Tokened{Result: setup.TokenAsked, Shown: []string{madeToken}},
			wantAsked: accounts.Question{Choices: []identity.Entry{otherRemoved}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			w.writeConfig(t, tt.config)
			w.writeRecord(t, tt.record...)
			w.api.organizations = map[string]string{madeToken: tt.organization}
			r := w.tokenRig(t, &fakeMaker{})
			before := w.snapshot(t)

			got, err := r.s.MakeToken(t.Context(), adding, nil)

			if err != nil || got.Result != tt.want.Result || got.ID != tt.want.ID || got.SameAs != tt.want.SameAs ||
				got.Checked != tt.want.Checked || !slices.Equal(got.Shown, tt.want.Shown) {
				t.Fatalf("MakeToken() = %+v, %v; want %+v", got, err, tt.want)
			}
			if tt.want.Result == setup.TokenAsked {
				if got.Asked == nil || !sameQuestion(got.Asked.Question, tt.wantAsked) {
					t.Errorf("MakeToken() asked %+v, want %+v", got.Asked, tt.wantAsked)
				}
			} else if got.Asked != nil {
				t.Errorf("MakeToken() asked %+v, want nothing asked", got.Asked)
			}
			if tt.want.Result != setup.TokenSaved {
				w.checkUnchanged(t, before)
				return
			}
			w.checkTakenToday(t, tt.want.ID, madeToken)
			if got := w.readRecord(t); !slices.EqualFunc(got, tt.wantRecord, sameEntry) {
				t.Errorf("the accounts record holds %+v, want %+v", got, tt.wantRecord)
			}
		})
	}
}

// A token pasted to add an account whose organization doesn't settle which
// it is, is held as a made one is, but not given back to copy, as it was
// copied to be pasted.
func TestPasteTokenToAddAsksAsAMadeOneDoes(t *testing.T) {
	w := newWorld(t)
	w.done(t)
	w.writeRecord(t, spareRemoved)
	w.api.organizations = map[string]string{madeToken: "org-personal"}
	r := w.tokenRig(t, &fakeMaker{})

	got, err := r.s.PasteToken(t.Context(), adding, madeToken)

	if err != nil || got.Result != setup.TokenAsked || got.Asked == nil || len(got.Shown) > 0 {
		t.Errorf("PasteToken() = %+v, %v; want it asked, the token not given back", got, err)
	}
}

// Answer adds the account the asked token is for as the user says: a new
// account under the id its organization derives; one of the choices under
// its id, its organization recorded; and, where the answer's one the token
// can't be, as one configured since it was asked, asks afresh, of the
// accounts it may be now, saving nothing.
func TestAnswerAddsTheAccountAsTheUserSays(t *testing.T) {
	personalID := derivedID("org-personal")
	spareConfigured := "\n[[account]]\nid    = \"h7c2\"\nlabel = \"Spare\"\n"
	tests := []struct {
		name   string
		answer accounts.Answer
		// meanwhile is what's added to the config once the token's asked.
		meanwhile  string
		wantID     string
		wantAsked  *accounts.Question
		wantRecord []identity.Entry
	}{
		{
			name:       "a new account",
			answer:     accounts.Answer{New: true},
			wantID:     personalID,
			wantRecord: []identity.Entry{spareRemoved, {ID: personalID, Organization: "org-personal", Added: tokenTaken}},
		},
		{
			name:       "one removed, its organization recorded",
			answer:     accounts.Answer{ID: "h7c2"},
			wantID:     "h7c2",
			wantRecord: []identity.Entry{{ID: "h7c2", Organization: "org-personal", Added: recordedBefore, Was: "spare"}},
		},
		{
			name:      "one it can't be",
			answer:    accounts.Answer{ID: "zzzz"},
			wantAsked: &accounts.Question{Named: true, Choices: []identity.Entry{spareRemoved}},
		},
		{
			name:      "one configured since it was asked",
			answer:    accounts.Answer{ID: "h7c2"},
			meanwhile: spareConfigured,
			wantAsked: &accounts.Question{Named: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWorld(t)
			w.done(t)
			w.writeRecord(t, spareRemoved)
			w.api.organizations = map[string]string{madeToken: "org-personal"}
			r := w.tokenRig(t, &fakeMaker{})
			asked, err := r.s.MakeToken(t.Context(), adding, nil)
			if err != nil || asked.Asked == nil {
				t.Fatalf("MakeToken() = %+v, %v; want it asked", asked, err)
			}
			w.writeConfig(t, w.readConfig(t)+tt.meanwhile)
			before := w.snapshot(t)

			got, err := r.s.Answer(t.Context(), asked.Asked, tt.answer)

			if tt.wantAsked != nil {
				if err != nil || got.Result != setup.TokenAsked || got.Asked == nil || !sameQuestion(got.Asked.Question, *tt.wantAsked) ||
					!slices.Equal(got.Shown, []string{madeToken}) {
					t.Errorf("Answer() = %+v, %v; want it asked afresh, %+v, the token given back", got, err, *tt.wantAsked)
				}
				w.checkUnchanged(t, before)
				return
			}
			want := setup.Tokened{Result: setup.TokenSaved, Checked: accounts.Checked{Organization: "org-personal"}, ID: tt.wantID}
			checkTokened(t, got, err, want)
			w.checkTakenToday(t, tt.wantID, madeToken)
			if !strings.Contains(w.readConfig(t), `id    = "`+tt.wantID+`"`) {
				t.Errorf("the config reads\n%s\nwant Personal added as %s", w.readConfig(t), tt.wantID)
			}
			if got := w.readRecord(t); !slices.EqualFunc(got, tt.wantRecord, sameEntry) {
				t.Errorf("the accounts record holds %+v, want %+v", got, tt.wantRecord)
			}
		})
	}
}

// An answer that can't be saved gives a token made back to copy, as it
// lasts a year on the account, as MakeToken does one it can't save, logged
// by the label of the account it was to add, as it has no id; a token
// pasted, still the user's, fails as it is. A save begun is a save, however
// the caller's context ends.
func TestAnAnswerThatCantBeSaved(t *testing.T) {
	takenMeanwhile := "\n[[account]]\nid    = \"k3f9\"\nlabel = \"PERSONAL\"\n"
	ask := func(t *testing.T, w *world, r tokenRig, pasted bool) *setup.Asked {
		t.Helper()
		var asked setup.Tokened
		var err error
		if pasted {
			asked, err = r.s.PasteToken(t.Context(), adding, madeToken)
		} else {
			asked, err = r.s.MakeToken(t.Context(), adding, nil)
		}
		if err != nil || asked.Asked == nil {
			t.Fatalf("the token's save = %+v, %v; want it asked", asked, err)
		}
		w.writeConfig(t, w.readConfig(t)+takenMeanwhile)
		return asked.Asked
	}
	t.Run("made", func(t *testing.T) {
		w := newWorld(t)
		w.done(t)
		w.writeRecord(t, spareRemoved)
		w.api.organizations = map[string]string{madeToken: "org-personal"}
		r := w.tokenRig(t, &fakeMaker{})
		asked := ask(t, w, r, false)
		before := w.snapshot(t)
		log := logstest.Capture(t)

		got, err := r.s.Answer(t.Context(), asked, accounts.Answer{New: true})

		if err != nil || got.Result != setup.TokenNotSaved || !slices.Equal(got.Shown, []string{madeToken}) || !strings.Contains(got.Why, accounts.ErrLabelTaken.Error()) {
			t.Errorf("Answer() = %+v, %v; want the token given back, not saved, saying why", got, err)
		}
		if !log.Has("level=WARN", `msg="claude setup-token made a token switchboard couldn't save"`, "label=Personal") || strings.Contains(log.String(), "account=") {
			t.Errorf("the log reads\n%s\nwant the token not saved, logged by the label of the account it was to add", log)
		}
		w.checkUnchanged(t, before)
	})
	t.Run("made, a configured account's since it was asked", func(t *testing.T) {
		w := newWorld(t)
		w.done(t)
		w.writeRecord(t, spareRemoved)
		w.api.organizations = map[string]string{madeToken: "org-personal"}
		r := w.tokenRig(t, &fakeMaker{})
		asked, err := r.s.MakeToken(t.Context(), adding, nil)
		if err != nil || asked.Asked == nil {
			t.Fatalf("MakeToken() = %+v, %v; want it asked", asked, err)
		}
		w.writeConfig(t, w.readConfig(t)+"\n[[account]]\nid    = \"k3f9\"\nlabel = \"Team\"\n")
		w.writeRecord(t, spareRemoved, identity.Entry{ID: "k3f9", Organization: "org-personal", Added: recordedBefore})
		before := w.snapshot(t)

		got, err := r.s.Answer(t.Context(), asked.Asked, accounts.Answer{New: true})

		if err != nil || got.Result != setup.TokenNotSaved || !slices.Equal(got.Shown, []string{madeToken}) || !strings.Contains(got.Why, `the token is for "Team" (k3f9), configured already`) {
			t.Errorf("Answer() = %+v, %v; want the token given back, not saved, as Team's", got, err)
		}
		w.checkUnchanged(t, before)
	})
	t.Run("pasted", func(t *testing.T) {
		w := newWorld(t)
		w.done(t)
		w.writeRecord(t, spareRemoved)
		w.api.organizations = map[string]string{madeToken: "org-personal"}
		r := w.tokenRig(t, &fakeMaker{})
		asked := ask(t, w, r, true)
		before := w.snapshot(t)

		got, err := r.s.Answer(t.Context(), asked, accounts.Answer{New: true})

		if !errors.Is(err, accounts.ErrLabelTaken) {
			t.Errorf("Answer() = %+v, %v; want it failing as the label's taken", got, err)
		}
		w.checkUnchanged(t, before)
	})
	t.Run("the caller gone", func(t *testing.T) {
		w := newWorld(t)
		w.done(t)
		w.writeRecord(t, spareRemoved)
		w.api.organizations = map[string]string{madeToken: "org-personal"}
		r := w.tokenRig(t, &fakeMaker{})
		asked, err := r.s.MakeToken(t.Context(), adding, nil)
		if err != nil || asked.Asked == nil {
			t.Fatalf("MakeToken() = %+v, %v; want it asked", asked, err)
		}
		gone, cancel := context.WithCancel(t.Context())
		cancel()

		got, err := r.s.Answer(gone, asked.Asked, accounts.Answer{New: true})

		if err != nil || got.Result != setup.TokenSaved {
			t.Errorf("Answer() = %+v, %v; want the token saved", got, err)
		}
	})
}

// A token replacing an account's must be for the account's organization, as
// the accounts record has it, made or pasted: another's is refused, saving
// nothing, naming the account the record has of it, where the account's own
// isn't known.
func TestATokenReplacingAnAccountsIsHeldToItsOrganization(t *testing.T) {
	tests := []struct {
		name   string
		record []identity.Entry
		want   setup.Tokened
	}{
		{
			name:   "another subscription's than the account's",
			record: []identity.Entry{{ID: "k3f9", Organization: "org-team", Added: recordedBefore}},
			want:   setup.Tokened{Result: setup.TokenOtherSubscription},
		},
		{
			name:   "an account's the record has, the account's own not known",
			record: []identity.Entry{{ID: "k3f9", Added: recordedBefore}, otherRemoved},
			want:   setup.Tokened{Result: setup.TokenOtherSubscription, SameAs: "Old"},
		},
	}
	ways := []struct {
		name    string
		replace func(t *testing.T, r tokenRig) (setup.Tokened, error)
	}{
		{name: "made", replace: func(t *testing.T, r tokenRig) (setup.Tokened, error) {
			return r.s.MakeToken(t.Context(), replacing("k3f9"), nil)
		}},
		{name: "pasted", replace: func(t *testing.T, r tokenRig) (setup.Tokened, error) {
			return r.s.PasteToken(t.Context(), replacing("k3f9"), madeToken)
		}},
	}
	for _, way := range ways {
		for _, tt := range tests {
			t.Run(way.name+", "+tt.name, func(t *testing.T) {
				w := newWorld(t)
				w.done(t)
				w.writeConfig(t, withTeam)
				w.writeToken(t, "k3f9", "test-token-team")
				w.writeRecord(t, tt.record...)
				w.api.organizations = map[string]string{madeToken: "org-other"}
				r := w.tokenRig(t, &fakeMaker{})
				before := w.snapshot(t)

				got, err := way.replace(t, r)

				checkTokened(t, got, err, tt.want)
				w.checkUnchanged(t, before)
			})
		}
	}
}

// sameQuestion reports whether a and b ask the same.
func sameQuestion(a, b accounts.Question) bool {
	return a.Named == b.Named && slices.EqualFunc(a.Choices, b.Choices, sameEntry)
}

// sameEntry reports whether a and b are the same account, as the record keeps
// it.
func sameEntry(a, b identity.Entry) bool {
	return a.ID == b.ID && a.Organization == b.Organization && a.Label == b.Label && a.Was == b.Was &&
		a.Added.Equal(b.Added) && a.Removed.Equal(b.Removed)
}
