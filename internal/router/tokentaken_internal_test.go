package router

import (
	"testing"
	"time"

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
	records := func(_ string, token tokens.Token) (time.Time, bool) {
		at, ok := map[string]time.Time{workToken: taken}[token.Reveal()]
		return at, ok
	}

	as := resolve(testConfigured, testTokens.Read, records)
	for _, a := range as {
		want := map[string]time.Time{"work": taken}[a.ID]
		if got, ok := a.secret.tokenTaken(); !got.Equal(want) || ok == want.IsZero() {
			t.Errorf("%s's token was taken at %v (%v), want %v", a.ID, got, ok, want)
		}
	}
}
