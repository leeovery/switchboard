package router

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/tokens"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

func TestATokensDateIsReadAgainWithTheToken(t *testing.T) {
	taken, renewedAt := start.Add(-90*24*time.Hour), start.Add(-time.Minute)
	renewed := tokenstest.Files{"work": renewedToken, "side": sideToken}
	// filesNow are what the token files hold at a look, and when each token
	// was taken, by the token, as the records beside them say.
	type filesNow struct {
		files   tokenstest.Files
		records map[string]time.Time
	}
	tests := []struct {
		name string
		// first and then are what's there at a look, and at the next.
		first, then filesNow
		// want is when work's token was taken, as the router holds it after:
		// zero where that isn't known.
		want time.Time
	}{
		{
			name:  "its record there all along",
			first: filesNow{testTokens, map[string]time.Time{workToken: taken}},
			then:  filesNow{testTokens, map[string]time.Time{workToken: taken}},
			want:  taken,
		},
		{
			name:  "its record landing after the token was read",
			first: filesNow{testTokens, nil},
			then:  filesNow{testTokens, map[string]time.Time{workToken: taken}},
			want:  taken,
		},
		{
			name:  "its record gone",
			first: filesNow{testTokens, map[string]time.Time{workToken: taken}},
			then:  filesNow{testTokens, nil},
		},
		{
			name:  "another token, with its record",
			first: filesNow{testTokens, map[string]time.Time{workToken: taken}},
			then:  filesNow{renewed, map[string]time.Time{workToken: taken, renewedToken: renewedAt}},
			want:  renewedAt,
		},
		{
			name:  "another token, before its record lands",
			first: filesNow{testTokens, map[string]time.Time{workToken: taken}},
			then:  filesNow{renewed, map[string]time.Time{workToken: taken}},
		},
		{
			name:  "a look that misses the token, which keeps its date",
			first: filesNow{testTokens, map[string]time.Time{workToken: taken}},
			then:  filesNow{tokenstest.Files{"side": sideToken}, nil},
			want:  taken,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newTestTokenFiles(testTokens, tt.first.files)
			f.records = tt.first.records
			f.look()

			f.files.set(tt.then.files)
			f.records = tt.then.records
			f.look()
			work, _ := f.accounts.byID("work")
			if got, ok := work.secret.tokenTaken(); !got.Equal(tt.want) || ok == tt.want.IsZero() {
				t.Errorf("work's token was taken at %v (%v), want %v", got, ok, tt.want)
			}
		})
	}
}

func TestAnAccountsDateIsReadWithItsTokenAsTheRouterStarts(t *testing.T) {
	taken := start.Add(-90 * 24 * time.Hour)
	as := resolve(testConfigured, testTokens.Read, recordsOf(map[string]time.Time{workToken: taken}))
	for _, a := range as {
		want := map[string]time.Time{"work": taken}[a.ID]
		if got, ok := a.secret.tokenTaken(); !got.Equal(want) || ok == want.IsZero() {
			t.Errorf("%s's token was taken at %v (%v), want %v", a.ID, got, ok, want)
		}
	}
}

func TestATokensDateCountsOnlyForTheTokenItWasReadWith(t *testing.T) {
	as := resolve(testConfigured, testTokens.Read, recordsOf(map[string]time.Time{workToken: start}))
	work, _ := as.byID("work")
	renewed, err := tokens.Parse(renewedToken)
	if err != nil {
		t.Fatal(err)
	}

	// Replaced, as a 401 or a look replaces it, before its record is read.
	work.secret.replace(renewed, start)
	if at, ok := work.secret.tokenTaken(); ok {
		t.Errorf("work's new token was taken at %v, want it not known: the date was its old token's", at)
	}
}

func TestWhyARecordCantBeGoneByIsLoggedOnceUntilItChanges(t *testing.T) {
	log := logstest.Capture(t)
	as := resolve(testConfigured, testTokens.Read, unrecorded)
	work, _ := as.byID("work")
	token := work.token()
	failing := func(err error) TakenFunc {
		return func(string, tokens.Token) (time.Time, error) { return time.Time{}, err }
	}
	another := errors.New("the record tokens/work.taken is of another token")
	notOne := errors.New("the record tokens/work.taken isn't one of when a token was taken")

	for _, err := range []error{another, another, another, tokens.ErrUnrecorded, notOne, notOne, another} {
		work.noteTaken(token, failing(err))
	}
	var logged []string
	for _, line := range log.Lines() {
		if strings.Contains(line, `msg="not going by the record of when the token was taken"`) {
			logged = append(logged, line)
		}
	}
	if len(logged) != 3 || !strings.Contains(logged[0], "another token") || !strings.Contains(logged[1], "isn't one") || !strings.Contains(logged[2], "another token") {
		t.Errorf("logged\n%s\nwant why at each change alone: of another token, not one, of another token", strings.Join(logged, "\n"))
	}
	checkNoToken(t, log)
}

// recordsOf reads when each token was taken, by the token, as records has
// it, failing with tokens.ErrUnrecorded of a token it hasn't.
func recordsOf(records map[string]time.Time) TakenFunc {
	return func(_ string, token tokens.Token) (time.Time, error) {
		taken, ok := records[token.Reveal()]
		if !ok {
			return time.Time{}, tokens.ErrUnrecorded
		}
		return taken, nil
	}
}
