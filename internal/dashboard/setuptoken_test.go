package dashboard

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/theme"
)

// tokenScreen is the Accounts page of a first run, work added, + add an
// account chosen, the token card open as card is.
func tokenScreen(card SetupTokenCard) Setup {
	s := accountsScreen(setup.Review{At: now, Accounts: frame3Review().Accounts[:1]}, 1)
	s.Accounts.FirstRun, s.Accounts.Token = true, &card
	return s
}

// frame2Card is personal's token card as frame 2 has it: setup-token waiting
// on the approval for 21 seconds, the address opened in a private Firefox
// window.
func frame2Card() SetupTokenCard {
	return SetupTokenCard{
		ID: "personal", Stage: TokenWaiting, Waited: 21 * time.Second, Private: true,
		Opening: "Your browser is signed in to Work, so setup opened a private Firefox window. Sign in to Personal there, then approve.",
	}
}

// frame10Card is personal's token card as frame 10 has it: setup-token
// stopped with an error, what it showed last after it.
func frame10Card() SetupTokenCard {
	return SetupTokenCard{ID: "personal", Stage: TokenUnsaved, Unsaved: setup.Tokened{
		Result: setup.TokenNotMade,
		Shown:  []string{" OAuth error: Request failed with status code 400", " Press Enter to retry."},
	}.Unsaved()}
}

// cardTop is the top row of a card of the rows given at 160×37, centred down
// the screen by the dashboard Card's one rule.
func cardTop(rows int) int {
	return (37 - rows) / 2
}

// tokenCardRows are the card's rows from its top, rows tall, at 160×37, as
// setupCardRow reads each.
func tokenCardRows(c *canvas, rows int) []string {
	top := cardTop(rows)
	out := make([]string, rows)
	for i := range out {
		out[i] = setupCardRow(c, top+i)
	}
	return out
}

// inside is a row of a card 80 wide: its line, content as given, padded out,
// and its line.
func inside(content string) string {
	return "│" + content + blanks(78-len([]rune(content))) + "│"
}

func TestTheWaitingTokenCardIsFrame2s(t *testing.T) {
	c := setupDrawn(tokenScreen(frame2Card()), 160, 37)
	want := []string{
		"╭─ NEW TOKEN · PERSONAL " + strings.Repeat("─", 55) + "╮",
		inside(""),
		inside("  ●∙∙  Waiting for you to approve it in the browser                     0:21  "),
		inside(""),
		inside("       Your browser is signed in to Work, so setup opened a private Firefox"),
		inside("       window. Sign in to Personal there, then approve."),
		inside(""),
		inside("       Close any other private window first: they all share one sign-in."),
		inside(""),
		inside("       Setup saves the token automatically. There's nothing to copy."),
		inside(""),
		inside(blanks(30) + "o reopen window   p paste a token   esc cancel  "),
		"╰" + strings.Repeat("─", 78) + "╯",
	}
	if got := tokenCardRows(c, len(want)); !slices.Equal(got, want) {
		t.Errorf("the card reads\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got := rowOf(c, 36); got != "" {
		t.Errorf("the key line reads %q, want it empty: the card's keys are its own", got)
	}
}

func TestTheWaitingTokenCardsInks(t *testing.T) {
	c := setupDrawn(tokenScreen(frame2Card()), 160, 37)
	top, content := cardTop(13), editCardContent
	says := content + tokenSaysAt
	tests := []struct {
		name string
		x, y int
		want ink
	}{
		{name: "its title, as every card's", x: editCardX + 3, y: top, want: dimInk},
		{name: "the spinner's dot", x: content, y: top + 2, want: nameInk},
		{name: "the spinner's other cells", x: content + 1, y: top + 2, want: faintInk},
		{name: "the wait", x: says, y: top + 2, want: titleInk},
		{name: "how long it's waited", x: content + 73, y: top + 2, want: faintInk},
		{name: "how the address opened", x: says, y: top + 4, want: secondaryInk},
		{name: "that private windows share a sign-in", x: says, y: top + 7, want: warningInk},
		{name: "that setup saves the token", x: says, y: top + 9, want: mutedInk},
	}
	for _, tt := range tests {
		if got := inkAt(c, tt.x, tt.y); got != tt.want {
			t.Errorf("%s, at %d,%d, is in %+v, want %+v", tt.name, tt.x, tt.y, got, tt.want)
		}
	}
}

func TestTheFailedTokenCardIsFrame10s(t *testing.T) {
	c := setupDrawn(tokenScreen(frame10Card()), 160, 37)
	want := []string{
		"╭─ NEW TOKEN · PERSONAL " + strings.Repeat("─", 55) + "╮",
		inside(""),
		inside("  ✗    The token wasn't created"),
		inside("       claude setup-token stopped with an error. What it showed:"),
		inside(""),
		inside("       │ OAuth error: Request failed with status code 400"),
		inside("       │ Press Enter to retry."),
		inside(""),
		inside("       Try again, or paste a token you made with claude setup-token."),
		inside(""),
		inside(blanks(30) + "enter try again   p paste a token   esc cancel  "),
		"╰" + strings.Repeat("─", 78) + "╯",
	}
	if got := tokenCardRows(c, len(want)); !slices.Equal(got, want) {
		t.Errorf("the card reads\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	top, content := cardTop(len(want)), editCardContent
	says := content + tokenSaysAt
	for _, tt := range []struct {
		name string
		x, y int
		want ink
	}{
		{name: "its mark", x: content, y: top + 2, want: exhaustedInk},
		{name: "what came of it", x: says, y: top + 2, want: titleInk},
		{name: "what that means", x: says, y: top + 3, want: secondaryInk},
		{name: "the bar before what setup-token showed", x: says, y: top + 5, want: faintInk},
		{name: "what setup-token showed", x: says + 2, y: top + 5, want: mutedInk},
		{name: "what to do now", x: says, y: top + 8, want: mutedInk},
	} {
		if got := inkAt(c, tt.x, tt.y); got != tt.want {
			t.Errorf("%s, at %d,%d, is in %+v, want %+v", tt.name, tt.x, tt.y, got, tt.want)
		}
	}
}

// Every stage the card has no frame of is laid as frame 2's or frame 10's,
// in words of its own: starting, stopping and checking spinning, as the
// wait does, each with the keys it takes, none where nothing waits on one;
// the address shown to paste where it didn't open, c copying it in o's
// place; the token unread, all setup-token showed, marked to see to, with no
// try again; and each other end in its own words.
func TestTheTokenCardsOtherStages(t *testing.T) {
	notMade := func(result setup.TokenResult, changes ...func(*setup.Tokened)) SetupTokenCard {
		tokened := setup.Tokened{Result: result}
		for _, change := range changes {
			change(&tokened)
		}
		return SetupTokenCard{ID: "personal", Stage: TokenUnsaved, Unsaved: tokened.Unsaved()}
	}
	const address = "https://claude.example/oauth/authorize?code=true&client_id=testclient&state=teststatepersonal0123456789"
	tests := []struct {
		name string
		card SetupTokenCard
		want []string
	}{
		{
			name: "starting",
			card: SetupTokenCard{ID: "personal", Stage: TokenStarting},
			want: []string{
				"  ●∙∙  Starting claude setup-token",
				"",
				"       Setup opens its sign-in page in the browser once it's ready.",
				"",
				blanks(48) + "p paste a token   esc cancel",
			},
		},
		{
			name: "waiting, opened as it is: nothing more said",
			card: SetupTokenCard{ID: "work", Stage: TokenWaiting, Waited: 75 * time.Second},
			want: []string{
				"  ●∙∙  Waiting for you to approve it in the browser                     1:15",
				"",
				"       Setup saves the token automatically. There's nothing to copy.",
				"",
				blanks(30) + "o reopen window   p paste a token   esc cancel",
			},
		},
		{
			name: "waiting, not opened: the address to paste, broken, never cut, and c to copy it",
			card: SetupTokenCard{
				ID: "personal", Stage: TokenWaiting, Unopened: true, Address: address,
				Opening: "Your browser is signed in to Work, and setup couldn't open a private Safari window. Open this address in one, sign in to Personal there, then approve:",
			},
			want: []string{
				"  ●∙∙  Waiting for you to approve it in the browser                     0:00",
				"",
				"       Your browser is signed in to Work, and setup couldn't open a private",
				"       Safari window. Open this address in one, sign in to Personal there,",
				"       then approve:",
				"",
				"       " + address[:69],
				"       " + address[69:],
				"",
				"       Setup saves the token automatically. There's nothing to copy.",
				"",
				blanks(27) + "c copy the address   p paste a token   esc cancel",
			},
		},
		{
			name: "stopping: no key",
			card: SetupTokenCard{ID: "personal", Stage: TokenStopping},
			want: []string{
				"  ●∙∙  Stopping claude setup-token",
				"",
				"       Nothing is saved, unless it had already created the token.",
			},
		},
		{
			name: "checking: no key, as it's saved however setup's left",
			card: SetupTokenCard{ID: "personal", Stage: TokenChecking},
			want: []string{
				"  ●∙∙  Checking the token and saving it",
				"",
				"       claude setup-token created it. Setup checks it with the API, then",
				"       saves it, a year from today.",
				"",
				"       It's saved however you leave setup now, as it lasts a year on the",
				"       account.",
			},
		},
		{
			name: "refused",
			card: notMade(setup.TokenRefused),
			want: []string{
				"  ✗    The token was refused",
				"       The API refused the token, so it isn't saved.",
				"",
				"       Try again, or paste a token you made with claude setup-token.",
				"",
				blanks(30) + "enter try again   p paste a token   esc cancel",
			},
		},
		{
			name: "the same account, named",
			card: notMade(setup.TokenSameAccount, func(t *setup.Tokened) { t.SameAs = "work" }),
			want: []string{
				"  ✗    The token is for work's subscription",
				"       work has that subscription already, so the token isn't saved.",
				"",
				"       Try again, signed in to the other subscription, or paste a token you",
				"       made with claude setup-token.",
				"",
				blanks(30) + "enter try again   p paste a token   esc cancel",
			},
		},
		{
			name: "no Claude Code",
			card: notMade(setup.NoClaudeCode, func(t *setup.Tokened) { t.Why = "can't find claude: it isn't on PATH" }),
			want: []string{
				"  ✗    There's no Claude Code to create the token with",
				"       can't find claude: it isn't on PATH",
				"",
				"       Install Claude Code, then try again, or paste a token you made with",
				"       claude setup-token.",
				"",
				blanks(30) + "enter try again   p paste a token   esc cancel",
			},
		},
		{
			name: "unread: setup-token's screen, the token in it, and no try again",
			card: notMade(setup.TokenUnread, func(t *setup.Tokened) {
				t.Shown = []string{" Your OAuth token (valid for 1 year):", " test-token-personal"}
			}),
			want: []string{
				"  !    The token was created, but setup couldn't read it",
				"       It lasts a year on the account, so copy it from what claude",
				"       setup-token showed:",
				"",
				"       │ Your OAuth token (valid for 1 year):",
				"       │ test-token-personal",
				"",
				"       Then press p to paste it.",
				"",
				blanks(48) + "p paste a token   esc cancel",
			},
		},
		{
			name: "waiting, the screen saying the address didn't open: it shows, made safe to show",
			card: SetupTokenCard{ID: "personal", Stage: TokenWaiting, Unopened: true, Address: "https://claude.example/sign-in\x1b[31m"},
			want: []string{
				"  ●∙∙  Waiting for you to approve it in the browser                     0:00",
				"",
				"       https://claude.example/sign-in [31m",
				"",
				"       Setup saves the token automatically. There's nothing to copy.",
				"",
				blanks(27) + "c copy the address   p paste a token   esc cancel",
			},
		},
		{
			name: "waiting, the screen saying it opened: no address shown, whatever's given",
			card: SetupTokenCard{ID: "personal", Stage: TokenWaiting, Address: "https://claude.example/sign-in"},
			want: []string{
				"  ●∙∙  Waiting for you to approve it in the browser                     0:00",
				"",
				"       Setup saves the token automatically. There's nothing to copy.",
				"",
				blanks(30) + "o reopen window   p paste a token   esc cancel",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inner := append([]string{""}, tt.want...)
			if len(tt.card.keys()) == 0 {
				inner = append(inner, "")
			}
			rows := len(inner) + 2
			got := tokenCardRows(setupDrawn(tokenScreen(tt.card), 160, 37), rows)
			title := "╭─ NEW TOKEN · " + strings.ToUpper(tt.card.ID) + " ─"
			if !strings.HasPrefix(got[0], title) || got[rows-1] != "╰"+strings.Repeat("─", 78)+"╯" {
				t.Fatalf("the card's edges read\n%q\n%q\nwant it titled %q, %d rows tall", got[0], got[rows-1], title, rows)
			}
			want := make([]string, len(inner))
			for i, w := range inner {
				want[i] = inside(w)
			}
			if got := got[1 : rows-1]; !slices.Equal(got, want) {
				t.Errorf("the card reads\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
			}
		})
	}
}

func TestTheTokenCardsAddressAndItsMarks(t *testing.T) {
	card := SetupTokenCard{ID: "personal", Stage: TokenWaiting, Unopened: true, Address: "https://claude.example/sign-in", Opening: "Open this address in one:"}
	c := setupDrawn(tokenScreen(card), 160, 37)
	top := cardTop(12)
	if got := inkAt(c, editCardContent+tokenSaysAt, top+6); got != accentInk {
		t.Errorf("the address is in %+v, want accent.mode", got)
	}
	unread := SetupTokenCard{ID: "personal", Stage: TokenUnsaved, Unsaved: setup.Tokened{Result: setup.TokenUnread, Shown: []string{"test-token-personal"}}.Unsaved()}
	c = setupDrawn(tokenScreen(unread), 160, 37)
	if got := inkAt(c, editCardContent, cardTop(12)+2); got != alertInk {
		t.Errorf("the unread token's mark is in %+v, want bold accent.attention", got)
	}
}

// The token card's spinner moves while its making's under way, as the
// service step's does, and stands still once it's ended.
func TestTheTokenCardSpinsWhileItsMakingsUnderWay(t *testing.T) {
	for _, tt := range []struct {
		stage TokenStage
		spins bool
	}{
		{TokenStarting, true}, {TokenWaiting, true}, {TokenStopping, true}, {TokenChecking, true}, {TokenUnsaved, false},
	} {
		if _, got := tokenScreen(SetupTokenCard{ID: "personal", Stage: tt.stage}).SpinDelay(); got != tt.spins {
			t.Errorf("at stage %d, the spinner moves: %t, want %t", tt.stage, got, tt.spins)
		}
	}
}

// The token card offers its address to copy, as it's given, only while it
// waits on a sign-in the screen says didn't open, and Done's line isn't
// offered under it.
func TestTheTokenCardOffersItsAddressToCopyOnlyWhereItDidntOpen(t *testing.T) {
	const address = "https://claude.example/sign-in\x1b[31m"
	for _, tt := range []struct {
		name string
		card SetupTokenCard
		want string
	}{
		{name: "waiting, not opened: as it's given", card: SetupTokenCard{Stage: TokenWaiting, Unopened: true, Address: address}, want: address},
		{name: "waiting, opened", card: SetupTokenCard{Stage: TokenWaiting, Address: address}},
		{name: "ended", card: SetupTokenCard{Stage: TokenUnsaved, Unopened: true, Address: address}},
	} {
		if got := tokenScreen(tt.card).Copies("c"); got != tt.want {
			t.Errorf("%s: Copies(c) = %q, want %q", tt.name, got, tt.want)
		}
		if got := tokenScreen(tt.card).Copies("d"); got != "" {
			t.Errorf("%s: Copies(d) = %q, want nothing", tt.name, got)
		}
	}
}

func TestTheTokenCardIsWholeAtSetupsLeast(t *testing.T) {
	c := setupDrawn(tokenScreen(frame2Card()), 95, 25)
	top := (25 - 13) / 2
	x := (95 - setupCardWidth) / 2
	for _, tt := range []struct {
		y    int
		want string
	}{{top, "╭─ NEW TOKEN · PERSONAL"}, {top + 2, "0:21  │"}, {top + 11, "esc cancel  │"}, {top + 12, "╰──"}} {
		if row := string([]rune(rowOf(c, tt.y))[x:]); !strings.Contains(row, tt.want) {
			t.Errorf("at 95×25, row %d reads %q, want %q in it", tt.y, row, tt.want)
		}
	}
	if got := inkAt(c, x+cardInset, top+2); got != (ink{token: theme.AccentMode, bold: true}) {
		t.Errorf("the spinner's dot is in %+v, want bold accent.mode", got)
	}
}
