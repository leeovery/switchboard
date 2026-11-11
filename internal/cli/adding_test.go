package cli_test

import (
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/identity"
)

// workID is the id work's organization gives it, with personal and side
// configured.
var workID = identity.Record{}.Derive("org-work", []string{"personal", "side"})

// The accounts removed before work is added, each before switchboard made
// ids, so of no organization known: old work, seen last on 2 Sep 2026; and
// spare, seen last on 1 Aug 2026.
var (
	oldWork = identity.Entry{ID: "m2x7", Label: "Old work", Added: testNow.AddDate(-1, 0, 0), Removed: time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)}
	spare   = identity.Entry{ID: "r4n8", Label: "Spare", Added: testNow.AddDate(-1, 0, 0), Removed: time.Date(2026, 8, 1, 8, 0, 0, 0, time.UTC)}
)

// adding is a command's deps of personal and side configured, personal's
// token in its file, every token checked with the organizations API, and
// the accounts record holding entries.
func adding(t *testing.T, entries ...identity.Entry) (cli.Deps, string, *organizationsAPI) {
	t.Helper()
	api := newOrganizationsAPI(t)
	deps, path := accountsDeps(t, api.URL, personalAndSide)
	writeToken(t, deps, "personal", personalsToken)
	if len(entries) > 0 {
		writeRecord(t, deps, entries...)
	}
	return deps, path, api
}

// The token's organization decides the account's id: one this Mac has never
// had is added under the id it gives, the record holding it, added now; one
// it had, removed since, comes back under its id, the label it kept cleared,
// as the config's is the one.
func TestAccountsAddDecidesTheIDByTheOrganization(t *testing.T) {
	returned := identity.Entry{ID: "k3f9", Organization: "org-work", Label: "Old work", Added: testNow.AddDate(-1, 0, 0), Removed: testNow.AddDate(0, -1, 0)}
	tests := []struct {
		name    string
		args    []string
		record  []identity.Entry
		wantOut string
		table   string
		entry   identity.Entry
	}{
		{
			name:    "a new account",
			args:    []string{"accounts", "add", "Work"},
			wantOut: `added "Work" as ` + workID + "\n",
			table:   "[[account]]\nid    = \"" + workID + "\"\nlabel = \"Work\"\n",
			entry:   identity.Entry{ID: workID, Organization: "org-work", Added: testNow},
		},
		{
			name:    "a new account, the primary",
			args:    []string{"accounts", "add", "Work", "--primary"},
			wantOut: `added "Work" as ` + workID + ", the primary\n",
			table:   "[[account]]\nid      = \"" + workID + "\"\nlabel   = \"Work\"\nprimary = true\n",
			entry:   identity.Entry{ID: workID, Organization: "org-work", Added: testNow},
		},
		{
			name:    "an account this Mac had",
			args:    []string{"accounts", "add", "Work"},
			record:  []identity.Entry{returned},
			wantOut: `added "Work" as k3f9, an account this Mac had, so its history carries on` + "\n",
			table:   "[[account]]\nid    = \"k3f9\"\nlabel = \"Work\"\n",
			entry:   identity.Entry{ID: "k3f9", Organization: "org-work", Added: returned.Added},
		},
		{
			name:    "an answer given where the organization settles it",
			args:    []string{"accounts", "add", "Work", "--new"},
			record:  []identity.Entry{returned},
			wantOut: `added "Work" as k3f9, an account this Mac had, so its history carries on` + "\n",
			table:   "[[account]]\nid    = \"k3f9\"\nlabel = \"Work\"\n",
			entry:   identity.Entry{ID: "k3f9", Organization: "org-work", Added: returned.Added},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, path, _ := adding(t, tt.record...)
			before := readFile(t, path)

			got := runWithInput(t, deps, madeWork+"\n", tt.args...)

			if want := (result{stdout: tt.wantOut}); got != want {
				t.Errorf("switchboard %s = %+v, want %+v", strings.Join(tt.args, " "), got, want)
			}
			if want := before + "\n" + tt.table; readFile(t, path) != want {
				t.Errorf("the config reads\n%s\nwant\n%s", readFile(t, path), want)
			}
			checkTokenFile(t, deps, tt.entry.ID, madeWork+"\n")
			if got, ok := readRecord(t, deps).ByID(tt.entry.ID); !ok || got != tt.entry {
				t.Errorf("the record holds %+v (%v), want %+v", got, ok, tt.entry)
			}
		})
	}
}

// What adding an account refuses, writing nothing: a label that's none, or
// another configured account's, as each shows, ignoring case, or that would
// show wrong; a token the API refuses, or doesn't answer for, as its
// subscription isn't known; and a subscription configured already, named by
// its account's label and id, with what to do.
func TestAccountsAddRefuses(t *testing.T) {
	const taken = "another account has that label already, ignoring case: give each its own"
	tests := []struct {
		name    string
		label   string
		token   string
		wantErr string
	}{
		{name: "no label", label: "", token: madeWork, wantErr: "a label is required: every account has one"},
		{name: "another account's label", label: "Side", token: madeWork, wantErr: taken},
		{name: "another account's label in another case", label: "sIDE", token: madeWork, wantErr: taken},
		{name: "another account's label as it shows", label: " Side\u200b ", token: madeWork, wantErr: taken},
		{name: "a label of spaces", label: "   ", token: madeWork, wantErr: "label is blank, showing nothing: give one that shows"},
		{name: "a label of a zero-width joiner", label: "\u2060", token: madeWork, wantErr: "label is blank, showing nothing: give one that shows"},
		{name: "a label that isn't UTF-8", label: "a\xffb", token: madeWork, wantErr: "label isn't UTF-8 text, as a config's text must be"},
		{name: "a label hiding the character after a sign", label: "Work\u06dd1", token: madeWork, wantErr: "label holds a sign that joins the character after it, such as an Arabic number sign, which can hide that character"},
		{name: "a label shaped like a token", label: tokenShaped, token: madeWork, wantErr: "label looks like a token, which a label mustn't, as it shows wherever the account does"},
		{
			name: "a token the API refuses", label: "Work", token: staleSide,
			wantErr: "the API refused the token (HTTP 401 · Invalid bearer token), so nothing is saved: make another with claude setup-token, run while signed in to that subscription",
		},
		{
			name: "a token the API doesn't answer for", label: "Work", token: madeUnanswered,
			wantErr: "the API didn't answer for the token (HTTP 529 · Overloaded), so nothing is saved: an account is added only once it has, to know its subscription, so try again",
		},
		{
			name: "a subscription configured already", label: "Work", token: madeTwice,
			wantErr: `the token is for "Personal" (personal), configured already, so nothing is saved: give it a new token with switchboard accounts token personal`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, path, _ := adding(t, oldWork)
			before, record := readFile(t, path), readFile(t, recordPath(t, deps))

			got := runWithInput(t, deps, tt.token+"\n", "accounts", "add", tt.label)

			if want := (result{stderr: "Error: " + tt.wantErr + "\n", code: 1}); got != want {
				t.Errorf("switchboard accounts add = %+v, want %+v", got, want)
			}
			if readFile(t, path) != before || readFile(t, recordPath(t, deps)) != record {
				t.Errorf("the config reads\n%s\nand the record\n%s\nwant both as they were", readFile(t, path), readFile(t, recordPath(t, deps)))
			}
			checkNoTokenFile(t, deps, workID)
			checkNoToken(t, got, tt.token, tokenShaped)
		})
	}
}

// The choices where the record holds old work and spare, each removed
// before switchboard made ids, as the flags that give them.
const choicesSaid = "  --new             a new account\n" +
	"  --returning m2x7  \"Old work\", last seen 2 Sep 2026\n" +
	"  --returning r4n8  \"Spare\", last seen 1 Aug 2026"

// Where the token's organization doesn't settle which account it is, as
// where the API names none, or names one the record doesn't know while it
// holds accounts of none known, away from a terminal it's refused, saying
// the choices and the flags that answer; and --new or --returning answers,
// adding a new account, or bringing that one back.
func TestAccountsAddAsksWhereTheOrganizationDoesntSettleIt(t *testing.T) {
	const (
		unnamedAsked = "the API named no organization for the token, so whether it's a new account or one this Mac had is yours to say"
		namedAsked   = "the token's subscription is none this Mac's accounts are known to have, but may be one removed before switchboard made ids, so which it is is yours to say"
	)
	tests := []struct {
		name    string
		token   string
		flags   []string
		wantErr string
		// id is the id the account's added under, "" where it's new and
		// no organization named it, so its id is drawn at random.
		id       string
		wantOut  string
		returned bool
	}{
		{name: "no organization named, no answer", token: madeUnnamed, wantErr: unnamedAsked + ": answer with one of\n" + choicesSaid},
		{name: "an organization the record doesn't know, no answer", token: madeWork, wantErr: namedAsked + ": answer with one of\n" + choicesSaid},
		{name: "an answer none of the choices", token: madeWork, flags: []string{"--returning", "zzzz"}, wantErr: `"zzzz" isn't one of the accounts the token may be: answer with one of` + "\n" + choicesSaid},
		{name: "no organization named, --new", token: madeUnnamed, flags: []string{"--new"}},
		{name: "an organization the record doesn't know, --new", token: madeWork, flags: []string{"--new"}, id: workID, wantOut: `added "Work" as ` + workID + "\n"},
		{name: "--returning", token: madeWork, flags: []string{"--returning", "m2x7"}, id: "m2x7", wantOut: `added "Work" as m2x7, an account this Mac had, so its history carries on` + "\n", returned: true},
		{name: "--returning in another case", token: madeUnnamed, flags: []string{"--returning", "M2X7"}, id: "m2x7", wantOut: `added "Work" as m2x7, an account this Mac had, so its history carries on` + "\n", returned: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, path, _ := adding(t, oldWork, spare)
			before, record := readFile(t, path), readFile(t, recordPath(t, deps))

			got := runWithInput(t, deps, tt.token+"\n", append([]string{"accounts", "add", "Work"}, tt.flags...)...)

			if tt.wantErr != "" {
				if want := (result{stderr: "Error: " + tt.wantErr + "\n", code: 1}); got != want {
					t.Errorf("switchboard accounts add = %+v, want %+v", got, want)
				}
				if readFile(t, path) != before || readFile(t, recordPath(t, deps)) != record {
					t.Errorf("the config reads\n%s\nand the record\n%s\nwant both as they were", readFile(t, path), readFile(t, recordPath(t, deps)))
				}
				return
			}
			id := tt.id
			if id == "" {
				id = strings.TrimSuffix(strings.TrimPrefix(got.stdout, `added "Work" as `), "\n")
				tt.wantOut = got.stdout
			}
			if want := (result{stdout: tt.wantOut}); got != want || id == "" {
				t.Fatalf("switchboard accounts add = %+v, want %+v", got, want)
			}
			if want := before + "\n[[account]]\nid    = \"" + id + "\"\nlabel = \"Work\"\n"; readFile(t, path) != want {
				t.Errorf("the config reads\n%s\nwant\n%s", readFile(t, path), want)
			}
			checkTokenFile(t, deps, id, tt.token+"\n")
			entry, ok := readRecord(t, deps).ByID(id)
			if !ok || entry.Label != "" || !entry.Removed.IsZero() || entry.Added.Equal(testNow) == tt.returned {
				t.Errorf("the record holds %+v (%v), want it configured, added now where it's new", entry, ok)
			}
		})
	}
}

// At a terminal, where the token's organization doesn't settle which
// account it is, it asks, numbering the answers, and asks again till it's
// given one of them.
func TestAccountsAddAsksAtATerminal(t *testing.T) {
	tests := []struct {
		name    string
		answers string
		id      string
		out     string
	}{
		{name: "an account this Mac had", answers: "2\n", id: "m2x7", out: `added "Work" as m2x7, an account this Mac had, so its history carries on` + "\n"},
		{name: "a new account, after answers that aren't", answers: "4\nm2x7\n1\n", id: workID, out: `added "Work" as ` + workID + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, _, _ := adding(t, oldWork, spare)
			deps.TerminalInput = func(io.Reader) bool { return true }
			deps.Hidden = func(io.Reader) (func() ([]byte, error), bool) {
				return func() ([]byte, error) { return []byte(madeWork), nil }, true
			}

			got := runWithInput(t, deps, tt.answers, "accounts", "add", "Work")

			asked := "Which is it (1-3)? "
			want := result{
				stdout: tt.out,
				stderr: "Paste Work's token, from claude setup-token run while signed in to that subscription (it won't show): \n" +
					"switchboard: the token's subscription is none this Mac's accounts are known to have, but may be one removed before switchboard made ids, so which it is is yours to say\n" +
					"  1. a new account\n" +
					"  2. \"Old work\", last seen 2 Sep 2026\n" +
					"  3. \"Spare\", last seen 1 Aug 2026\n" +
					strings.Repeat(asked, strings.Count(tt.answers, "\n")),
			}
			if got != want {
				t.Errorf("switchboard accounts add = %+v, want %+v", got, want)
			}
			checkTokenFile(t, deps, tt.id, madeWork+"\n")
		})
	}
}

// A replaced token must be for the account's own subscription, as the record
// has its organization: another's is refused, nothing saved. Where the
// record doesn't know it, the token's is recorded, but where another account
// holds it, which is named.
func TestAccountsTokenIsHeldToTheAccountsOrganization(t *testing.T) {
	sideAdded := testNow.AddDate(-1, 0, 0)
	otherSide := identity.Entry{ID: "side", Organization: "org-other", Added: sideAdded}
	unknownSide := identity.Entry{ID: "side", Added: sideAdded}
	spareSide := identity.Entry{ID: "r4n8", Organization: "org-side", Label: "Spare", Added: sideAdded, Removed: testNow.AddDate(0, -1, 0)}
	tests := []struct {
		name    string
		record  []identity.Entry
		wantErr string
		// recorded is side's entry as the record holds it once the token's
		// saved.
		recorded identity.Entry
	}{
		{name: "another organization recorded", record: []identity.Entry{otherSide}, wantErr: "the token is for another subscription, so nothing is saved: add it as an account of its own, with switchboard accounts add <label>"},
		{name: "its organization held by another account", record: []identity.Entry{unknownSide, spareSide}, wantErr: `the token is for another subscription, that of "Spare" (r4n8), so nothing is saved: add it as an account of its own, with switchboard accounts add <label>`},
		{name: "no organization recorded", record: []identity.Entry{unknownSide}, recorded: identity.Entry{ID: "side", Organization: "org-side", Added: sideAdded}},
		{name: "its own organization recorded", record: []identity.Entry{{ID: "side", Organization: "org-side", Added: sideAdded}}, recorded: identity.Entry{ID: "side", Organization: "org-side", Added: sideAdded}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps, _, _ := adding(t, tt.record...)
			writeToken(t, deps, "side", staleSide)
			record := readFile(t, recordPath(t, deps))

			got := runWithInput(t, deps, madeSide+"\n", "accounts", "token", "side")

			if tt.wantErr != "" {
				if want := (result{stderr: "Error: " + tt.wantErr + "\n", code: 1}); got != want {
					t.Errorf("switchboard accounts token side = %+v, want %+v", got, want)
				}
				checkTokenFile(t, deps, "side", staleSide+"\n")
				if readFile(t, recordPath(t, deps)) != record {
					t.Errorf("the record reads\n%s\nwant it as it was", readFile(t, recordPath(t, deps)))
				}
				return
			}
			if want := (result{stdout: "saved side's token\n"}); got != want {
				t.Errorf("switchboard accounts token side = %+v, want %+v", got, want)
			}
			checkTokenFile(t, deps, "side", madeSide+"\n")
			if got, _ := readRecord(t, deps).ByID("side"); got != tt.recorded {
				t.Errorf("the record holds side as %+v, want %+v", got, tt.recorded)
			}
		})
	}
}

// A token replaced for an account the record doesn't hold records nothing.
func TestAccountsTokenOfAnAccountTheRecordDoesntHoldRecordsNothing(t *testing.T) {
	deps, _, _ := adding(t)
	writeToken(t, deps, "side", staleSide)

	if got := runWithInput(t, deps, madeSide+"\n", "accounts", "token", "side"); got != (result{stdout: "saved side's token\n"}) {
		t.Errorf("switchboard accounts token side = %+v, want side's token saved", got)
	}
	checkNoFile(t, recordPath(t, deps))
}

// Removing an account keeps it in the record, marked removed now, with the
// label it showed; one the record doesn't hold records nothing.
func TestAccountsRemoveMarksTheAccountRemovedInTheRecord(t *testing.T) {
	added := testNow.AddDate(-1, 0, 0)
	t.Run("held", func(t *testing.T) {
		deps, _, _ := adding(t, identity.Entry{ID: "side", Organization: "org-side", Added: added})

		if got := run(t, deps, "accounts", "remove", "side"); got.code != 0 {
			t.Fatalf("switchboard accounts remove side = %+v, want side removed", got)
		}
		want := identity.Entry{ID: "side", Organization: "org-side", Label: "Side", Added: added, Removed: testNow}
		if got, _ := readRecord(t, deps).ByID("side"); got != want {
			t.Errorf("the record holds side as %+v, want %+v", got, want)
		}
	})
	t.Run("not held", func(t *testing.T) {
		deps, _, _ := adding(t)

		if got := run(t, deps, "accounts", "remove", "side"); got.code != 0 {
			t.Fatalf("switchboard accounts remove side = %+v, want side removed", got)
		}
		checkNoFile(t, recordPath(t, deps))
	})
}

// recordPath is the accounts record's path, as commands run with deps find
// it.
func recordPath(t *testing.T, deps cli.Deps) string {
	t.Helper()
	return identity.NewFile(stateDir(t, deps)).Path()
}

// writeRecord writes the accounts record, as commands run with deps find it,
// holding entries.
func writeRecord(t *testing.T, deps cli.Deps, entries ...identity.Entry) {
	t.Helper()
	err := identity.NewFile(stateDir(t, deps)).Update(func(r *identity.Record) error {
		r.Accounts = entries
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// readRecord reads the accounts record, as commands run with deps find it.
func readRecord(t *testing.T, deps cli.Deps) identity.Record {
	t.Helper()
	record, err := identity.NewFile(stateDir(t, deps)).Read()
	if err != nil {
		t.Fatal(err)
	}
	return record
}

// checkNoFile checks nothing is at path.
func checkNoFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); err == nil {
		t.Errorf("%s is there, want nothing written", path)
	}
}
