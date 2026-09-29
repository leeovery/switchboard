package router

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/tokens"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

const (
	// personalToken is personal's token, when a test gives it one.
	personalToken = "test-token-personal"
	// notAToken is what a token file holds that holds more than a token.
	notAToken = "test-token-one test-token-two"
)

func TestTokenFilesAreTakenUpAsTheyChange(t *testing.T) {
	tests := []struct {
		name string
		// before is what the token files hold as the router starts, testTokens
		// unless given, and after what they hold as it looks again.
		before, after tokenstest.Files
		// wantSendable are the accounts requests can go out on after, and
		// wantWork the token work goes out on.
		wantSendable []string
		wantWork     string
		// wantWorkRouted is whether a request carrying the token work had as
		// the router started is routed as work's.
		wantWorkRouted bool
		// wantLog is the line the change is logged with, if any.
		wantLog []string
		// wantKept is whether the state file hears of the change, and
		// wantReplanned whether the priming schedule is worked out again.
		wantKept, wantReplanned bool
	}{
		{
			name:           "none changed",
			after:          testTokens,
			wantSendable:   []string{"work", "side"},
			wantWork:       workToken,
			wantWorkRouted: true,
		},
		{
			name:           "work's holding another token",
			after:          tokenstest.Files{"work": renewedToken, "side": sideToken},
			wantSendable:   []string{"work", "side"},
			wantWork:       renewedToken,
			wantWorkRouted: true,
			wantLog:        []string{"level=INFO", `msg="token replaced; the one before still counts as the account's"`, "account=work"},
			wantKept:       true,
		},
		{
			name:           "personal's appearing",
			after:          tokenstest.Files{"work": workToken, "personal": personalToken, "side": sideToken},
			wantSendable:   []string{"work", "personal", "side"},
			wantWork:       workToken,
			wantWorkRouted: true,
			wantLog:        []string{"level=INFO", `msg="account has a usable token; requests can go out on it"`, "account=personal"},
			wantKept:       true,
			wantReplanned:  true,
		},
		{
			name:           "personal's put right",
			before:         tokenstest.Files{"work": workToken, "personal": notAToken, "side": sideToken},
			after:          tokenstest.Files{"work": workToken, "personal": personalToken, "side": sideToken},
			wantSendable:   []string{"work", "personal", "side"},
			wantWork:       workToken,
			wantWorkRouted: true,
			wantLog:        []string{"level=INFO", `msg="account has a usable token; requests can go out on it"`, "account=personal"},
			wantKept:       true,
			wantReplanned:  true,
		},
		{
			name:         "work's gone",
			after:        tokenstest.Files{"side": sideToken},
			wantSendable: []string{"side"},
			wantWork:     workToken,
			wantLog: []string{"level=INFO", `msg="account has no usable token; nothing will go out on it until it's back"`, "account=work",
				`error="` + tokenstest.Missing("work").Error() + `"`},
			wantKept:      true,
			wantReplanned: true,
		},
		{
			name:         "work's holding more than a token",
			after:        tokenstest.Files{"work": notAToken, "side": sideToken},
			wantSendable: []string{"side"},
			wantWork:     workToken,
			wantLog: []string{"level=INFO", `msg="account has no usable token; nothing will go out on it until it's back"`, "account=work",
				`error="` + tokens.ErrNotAToken.Error() + `"`},
			wantKept:      true,
			wantReplanned: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			before := tt.before
			if before == nil {
				before = testTokens
			}
			f := newTestTokenFiles(before, tt.after)

			f.look()
			if got := f.accounts.sendable().configured().IDs(); !slices.Equal(got, tt.wantSendable) {
				t.Errorf("requests can go out on %q, want %q", got, tt.wantSendable)
			}
			if work, _ := f.accounts.byID("work"); work.token().Reveal() != tt.wantWork {
				t.Errorf("work holds %q, want %q", work.token().Reveal(), tt.wantWork)
			}
			if _, routed := f.accounts.byToken(workToken, start); routed != tt.wantWorkRouted {
				t.Errorf("a request carrying work's token as it was is routed as an account's: %v, want %v", routed, tt.wantWorkRouted)
			}
			if tt.wantLog != nil && !log.Has(tt.wantLog...) {
				t.Errorf("log reads\n%s\nwant a line with %q", log, tt.wantLog)
			}
			if tt.wantLog == nil && log.Has("component=router") {
				t.Errorf("log reads\n%s\nwant nothing of the router's", log)
			}
			if (f.kept > 0) != tt.wantKept || (f.replanned > 0) != tt.wantReplanned {
				t.Errorf("the state file heard of %d changes, and the schedule %d, want any: %v and %v", f.kept, f.replanned, tt.wantKept, tt.wantReplanned)
			}
			checkNoToken(t, log)
		})
	}
}

func TestAnAccountWhoseTokenFileIsBackGoesOutOnWhatItHolds(t *testing.T) {
	log := logstest.Capture(t)
	f := newTestTokenFiles(testTokens, tokenstest.Files{"side": sideToken})
	f.look()
	work, _ := f.accounts.byID("work")
	if work.hasToken() || work.problem() != tokenstest.Missing("work").Error() {
		t.Fatalf("work, its token file gone, has a token: %v, or a problem of %q, want none, and why", work.hasToken(), work.problem())
	}

	f.files.set(testTokens)
	f.look()
	if !work.hasToken() || work.token().Reveal() != workToken || work.problem() != "" {
		t.Errorf("work, its token file back as it was, has a token: %v, holding %q, with a problem of %q, want %q alone", work.hasToken(), work.token().Reveal(), work.problem(), workToken)
	}
	if log.Has(`msg="token replaced; the one before still counts as the account's"`) {
		t.Errorf("log reads\n%s\nwant no token replaced: work's file is back as it was", log)
	}

	f.files.set(tokenstest.Files{"side": sideToken})
	f.look()
	f.files.set(tokenstest.Files{"work": renewedToken, "side": sideToken})
	f.look()
	if !work.hasToken() || work.token().Reveal() != renewedToken {
		t.Errorf("work, its token file back with another token, has a token: %v, holding %q, want %q", work.hasToken(), work.token().Reveal(), renewedToken)
	}
	if a, ok := f.accounts.byToken(workToken, start); !ok || a.ID != "work" {
		t.Errorf("a request carrying work's token as it was is routed as %q (%v), want work's: it counts as work's for a week", a.ID, ok)
	}
	for _, want := range [][]string{
		{"level=INFO", `msg="token replaced; the one before still counts as the account's"`, "account=work"},
		{"level=INFO", `msg="account has a usable token; requests can go out on it"`, "account=work"},
	} {
		if !log.Has(want...) {
			t.Errorf("log reads\n%s\nwant a line with %q", log, want)
		}
	}
	if n := strings.Count(log.String(), `msg="account has a usable token; requests can go out on it"`); n != 2 {
		t.Errorf("log reads\n%s\nwant work's token back twice, not %d times", log, n)
	}
	checkNoToken(t, log)
}

func TestTokenFilesAreTakenUpForTheStateFile(t *testing.T) {
	f := newTestTokenFiles(testTokens, tokenstest.Files{"work": renewedToken, "side": sideToken})
	was := f.accounts.kept()

	f.look()
	kept := f.accounts.kept()
	if kept["work"].SHA256 != hash(renewedToken) {
		t.Errorf("the state file keeps work's token as %q, want the new one's hash", kept["work"].SHA256)
	}
	if former := kept["work"].Former; len(former) != 1 || former[0].SHA256 != was["work"].SHA256 || !former[0].ReplacedAt.Equal(start) {
		t.Errorf("the state file keeps work's former tokens as %+v, want the one before, replaced now", former)
	}

	f.files.set(tokenstest.Files{"side": sideToken})
	f.look()
	if _, ok := f.accounts.kept()["work"]; ok {
		t.Errorf("the state file keeps %+v of work, its token file gone, want nothing", f.accounts.kept()["work"])
	}
}

// testTokenFiles takes up what the token files hold, counting the changes it
// tells of.
type testTokenFiles struct {
	*tokenFiles
	files           *changingFiles
	kept, replanned int
}

// newTestTokenFiles returns what takes up testConfigured's token files, as
// after holds them, having read them first as before held them, at start's
// time.
func newTestTokenFiles(before, after tokenstest.Files) *testTokenFiles {
	f := &testTokenFiles{files: &changingFiles{files: after}}
	f.tokenFiles = &tokenFiles{
		accounts: resolve(testConfigured, before.Read),
		read:     f.files.read,
		now:      at(start),
		kept:     func() { f.kept++ },
		sendable: func() { f.replanned++ },
	}
	return f
}

// changingFiles are token files a test changes as it goes.
type changingFiles struct {
	files tokenstest.Files
}

func (c *changingFiles) set(files tokenstest.Files) {
	c.files = files
}

func (c *changingFiles) read(id string) (tokens.Token, error) {
	return c.files.Read(id)
}

// checkNoToken checks the log shows no token.
func checkNoToken(t *testing.T, log *logstest.Log) {
	t.Helper()
	for _, token := range []string{workToken, sideToken, renewedToken, personalToken} {
		if strings.Contains(log.String(), token) {
			t.Errorf("log reads\n%s\nwhich shows a token", log)
		}
	}
}
