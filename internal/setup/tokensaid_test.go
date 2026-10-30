package setup_test

import (
	"reflect"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/setuptoken"
)

func TestTheTokenCardSaysHowTheSignInAddressOpened(t *testing.T) {
	ready := func(opened setuptoken.Opened, browser, signedIn string) setup.Making {
		return setup.Making{Step: setup.AddressReady, Opened: opened, Browser: browser, For: "Personal", SignedIn: signedIn}
	}
	tests := []struct {
		name string
		at   setup.Making
		want string
	}{
		{name: "in the browser as it is: nothing more", at: ready(setuptoken.OpenedInBrowser, "", ""), want: ""},
		{
			name: "in a private window, the browser named",
			at:   ready(setuptoken.OpenedPrivately, "Firefox", "Work"),
			want: "Your browser is signed in to Work, so setup opened a private Firefox window. Sign in to Personal there, then approve.",
		},
		{
			name: "in a private window of a browser not named",
			at:   ready(setuptoken.OpenedPrivately, "", "Work"),
			want: "Your browser is signed in to Work, so setup opened a private window. Sign in to Personal there, then approve.",
		},
		{
			name: "no private window opened: the address to open in one",
			at:   ready(setuptoken.NotOpened, "Safari", "Work"),
			want: "Your browser is signed in to Work, and setup couldn't open a private Safari window. Open this address in one, sign in to Personal there, then approve:",
		},
		{
			name: "the browser as it is not opened: the address to open in it",
			at:   ready(setuptoken.NotOpened, "", ""),
			want: "Setup couldn't open your browser. Open this address in it, signed in to Personal, then approve:",
		},
		{name: "a step past the address", at: setup.Making{Step: setup.AwaitingApproval, For: "Personal", SignedIn: "Work"}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.at.Opening(); got != tt.want {
				t.Errorf("Opening() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTheTokenCardSaysHowAMakingSavedNothing(t *testing.T) {
	const tryAgain = "Try again, or paste a token you made with claude setup-token."
	tests := []struct {
		name    string
		tokened setup.Tokened
		want    setup.Unsaved
	}{
		{
			name: "setup-token stopping with an error: the last lines it showed, without their blanks",
			tokened: setup.Tokened{Result: setup.TokenNotMade, Why: "setup-token showed an error", Shown: []string{
				" Claude Code", " Opening browser to sign in…", " Paste code here if prompted >", " Signing in…",
				" OAuth error: Request failed with status code 400", "  Press Enter to retry.",
			}},
			want: setup.Unsaved{
				Mark: setup.Failed, Headline: "The token wasn't created", Says: "claude setup-token stopped with an error. What it showed:",
				Shown: []string{"Paste code here if prompted >", "Signing in…", "OAuth error: Request failed with status code 400", "Press Enter to retry."},
				Then:  tryAgain, Again: true,
			},
		},
		{
			name:    "setup-token showing fewer lines than the card shows: all of them",
			tokened: setup.Tokened{Result: setup.TokenNotMade, Shown: []string{" The sign-in timed out."}},
			want: setup.Unsaved{
				Mark: setup.Failed, Headline: "The token wasn't created", Says: "claude setup-token stopped with an error. What it showed:",
				Shown: []string{"The sign-in timed out."}, Then: tryAgain, Again: true,
			},
		},
		{
			name:    "the API refusing the token",
			tokened: setup.Tokened{Result: setup.TokenRefused, Why: "the API refused the token"},
			want: setup.Unsaved{
				Mark: setup.Failed, Headline: "The token was refused", Says: "The API refused the token, so it isn't saved.",
				Then: tryAgain, Again: true,
			},
		},
		{
			name:    "another account's subscription, named",
			tokened: setup.Tokened{Result: setup.TokenSameAccount, SameAs: "work"},
			want: setup.Unsaved{
				Mark: setup.Failed, Headline: "The token is for work's subscription", Says: "work has that subscription already, so the token isn't saved.",
				Then: "Try again, signed in to the other subscription, or paste a token you made with claude setup-token.", Again: true,
			},
		},
		{
			name:    "no Claude Code: why",
			tokened: setup.Tokened{Result: setup.NoClaudeCode, Why: "can't find claude: it isn't on PATH"},
			want: setup.Unsaved{
				Mark: setup.Failed, Headline: "There's no Claude Code to create the token with", Says: "can't find claude: it isn't on PATH",
				Then: "Install Claude Code, then try again, or paste a token you made with claude setup-token.", Again: true,
			},
		},
		{
			name:    "the token made, but unread: all setup-token showed, to copy it from, and no other made",
			tokened: setup.Tokened{Result: setup.TokenUnread, Shown: []string{" Your OAuth token (valid for 1 year):", " test-token-personal", " Store this token securely."}},
			want: setup.Unsaved{
				Mark: setup.Warning, Headline: "The token was created, but setup couldn't read it",
				Says:  "It lasts a year on the account, so copy it from what claude setup-token showed:",
				Shown: []string{"Your OAuth token (valid for 1 year):", "test-token-personal", "Store this token securely."},
				Then:  "Then press p to paste it.",
			},
		},
		{
			name:    "the token made, but not saved: why, and the token, to copy and paste, and no other made",
			tokened: setup.Tokened{Result: setup.TokenNotSaved, Why: "write the token file: permission denied", Shown: []string{"test-token-personal"}},
			want: setup.Unsaved{
				Mark: setup.Warning, Headline: "The token was created, but setup couldn't save it",
				Says: "write the token file: permission denied", Shown: []string{"test-token-personal"},
				Then: "It lasts a year on the account, so copy it, then press p to paste it once that's put right.",
			},
		},
		{
			name:    "the token made, but never read: why, nothing to copy, and enter trying again",
			tokened: setup.Tokened{Result: setup.TokenNotSaved, Why: "read setup-token's terminal: input/output error"},
			want: setup.Unsaved{
				Mark: setup.Warning, Headline: "The token was created, but setup couldn't save it",
				Says: "read setup-token's terminal: input/output error", Then: tryAgain, Again: true,
			},
		},
		{name: "saved", tokened: setup.Tokened{Result: setup.TokenSaved}},
		{name: "cancelled", tokened: setup.Tokened{Result: setup.TokenCancelled}},
		{name: "interrupted", tokened: setup.Tokened{Result: setup.TokenInterrupted}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tokened.Unsaved(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Unsaved() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

// Once setup's screen has gone, a making that saved nothing is printed as
// the token card tells it, a line each, made safe to show, what setup-token
// showed set in by two spaces, but for what to do now: there's no card to
// try again or paste on, so setup is run again. Every other end prints
// nothing.
func TestAMakingThatSavedNothingIsPrintedOnceTheScreenHasGone(t *testing.T) {
	tests := []struct {
		name    string
		tokened setup.Tokened
		want    []string
	}{
		{
			name:    "the token made, but unread: all setup-token showed, the token pasted once setup runs again",
			tokened: setup.Tokened{Result: setup.TokenUnread, Shown: []string{" Your OAuth token (valid for 1 year):", "", " test-token-personal"}},
			want: []string{
				"The token was created, but setup couldn't read it",
				"It lasts a year on the account, so copy it from what claude setup-token showed:",
				"  Your OAuth token (valid for 1 year):",
				"  test-token-personal",
				"Then run switchboard setup again, and press p on its token card to paste it.",
			},
		},
		{
			name:    "the token made, but not saved: why, the token, pasted once that's put right and setup runs again",
			tokened: setup.Tokened{Result: setup.TokenNotSaved, Why: "write the token file: permission denied", Shown: []string{"test-token-personal"}},
			want: []string{
				"The token was created, but setup couldn't save it",
				"write the token file: permission denied",
				"  test-token-personal",
				"It lasts a year on the account, so copy it, then once that's put right, run switchboard setup again, and press p on its token card to paste it.",
			},
		},
		{
			name:    "the token made, but never read: why, and trying again",
			tokened: setup.Tokened{Result: setup.TokenNotSaved, Why: "read setup-token's terminal: input/output error"},
			want: []string{
				"The token was created, but setup couldn't save it",
				"read setup-token's terminal: input/output error",
				"Run switchboard setup again to try again, or to paste a token you made with claude setup-token.",
			},
		},
		{
			name:    "the API refusing the token",
			tokened: setup.Tokened{Result: setup.TokenRefused},
			want: []string{
				"The token was refused",
				"The API refused the token, so it isn't saved.",
				"Run switchboard setup again to try again, or to paste a token you made with claude setup-token.",
			},
		},
		{
			name:    "setup-token stopping with an error: the last lines it showed",
			tokened: setup.Tokened{Result: setup.TokenNotMade, Shown: []string{" The sign-in timed out."}},
			want: []string{
				"The token wasn't created",
				"claude setup-token stopped with an error. What it showed:",
				"  The sign-in timed out.",
				"Run switchboard setup again to try again, or to paste a token you made with claude setup-token.",
			},
		},
		{
			name:    "no Claude Code: why",
			tokened: setup.Tokened{Result: setup.NoClaudeCode, Why: "can't find claude: it isn't on PATH"},
			want: []string{
				"There's no Claude Code to create the token with",
				"can't find claude: it isn't on PATH",
				"Install Claude Code, then run switchboard setup again to try again, or to paste a token you made with claude setup-token.",
			},
		},
		{
			name:    "another account's subscription, named, made safe to show",
			tokened: setup.Tokened{Result: setup.TokenSameAccount, SameAs: "work\x1b[2J"},
			want: []string{
				"The token is for work [2J's subscription",
				"work [2J has that subscription already, so the token isn't saved.",
				"Run switchboard setup again to try again, signed in to the other subscription, or to paste a token you made with claude setup-token.",
			},
		},
		{
			name:    "what setup-token showed made safe to show, its spacing kept",
			tokened: setup.Tokened{Result: setup.TokenUnread, Shown: []string{"test-token-\x1b[31mpersonal"}},
			want: []string{
				"The token was created, but setup couldn't read it",
				"It lasts a year on the account, so copy it from what claude setup-token showed:",
				"  test-token- [31mpersonal",
				"Then run switchboard setup again, and press p on its token card to paste it.",
			},
		},
		{name: "saved", tokened: setup.Tokened{Result: setup.TokenSaved}},
		{name: "cancelled", tokened: setup.Tokened{Result: setup.TokenCancelled}},
		{name: "interrupted", tokened: setup.Tokened{Result: setup.TokenInterrupted}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tokened.Printed(); !slices.Equal(got, tt.want) {
				t.Errorf("Printed() = %q, want %q", got, tt.want)
			}
		})
	}
}

// The paste card says why it refuses a paste in setup's words: what's
// pasted no token, the API refusing it, and not answering, as each is told;
// another account's subscription, named. Every other end is no refusal.
func TestThePasteCardSaysWhyItRefusesAPaste(t *testing.T) {
	const why = "what's pasted holds more than the token"
	tests := []struct {
		name    string
		tokened setup.Tokened
		want    string
	}{
		{name: "no token", tokened: setup.Tokened{Result: setup.TokenNotGiven, Why: why}, want: why},
		{name: "refused", tokened: setup.Tokened{Result: setup.TokenRefused, Why: "the API refused the token (HTTP 401)"}, want: "the API refused the token (HTTP 401)"},
		{name: "unanswered", tokened: setup.Tokened{Result: setup.TokenUnanswered, Why: "the API didn't answer (HTTP 529)"}, want: "the API didn't answer (HTTP 529)"},
		{name: "another account's", tokened: setup.Tokened{Result: setup.TokenSameAccount, SameAs: "work"}, want: "work has that subscription already, so the token isn't saved"},
		{name: "saved", tokened: setup.Tokened{Result: setup.TokenSaved}},
		{name: "cancelled", tokened: setup.Tokened{Result: setup.TokenCancelled}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.tokened.Refusal(); got != tt.want {
				t.Errorf("Refusal() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestTheReviewsOrganizationsAreThoseKnown(t *testing.T) {
	review := setup.Review{Accounts: []setup.Reviewed{
		{ID: "work", Token: setup.TokenStanding{State: setup.TokenWorking, Organization: "org-work"}},
		{ID: "personal", Token: setup.TokenStanding{State: setup.TokenRead}},
		{ID: "side", Token: setup.TokenStanding{State: setup.TokenWorking, Organization: "org-side"}},
	}}
	want := setup.Organizations{"work": "org-work", "side": "org-side"}
	if got := review.Organizations(); !reflect.DeepEqual(got, want) {
		t.Errorf("Organizations() = %v, want %v", got, want)
	}
}
