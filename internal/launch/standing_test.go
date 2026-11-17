package launch_test

import (
	"errors"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

const (
	personalToken = "test-token-personal"
	renewedToken  = "test-token-work-renewed"
)

// everyToken has every account a token: work, the primary, personal and
// side, in the config's order.
var everyToken = tokenstest.Files{"work": workToken, "personal": personalToken, "side": sideToken}

// routing returns r, its status document naming the accounts with the given
// ids, in their order, as the router routes them, each whose id refused has
// with why its last read was refused.
func (r *fakeRouter) routing(ids []string, refused map[string]quota.ReadRefusal) *fakeRouter {
	r.doc = status.Document{}
	for _, id := range ids {
		r.doc.Accounts = append(r.doc.Accounts, status.Account{ID: id, Label: id, RefusedRead: refused[id]})
	}
	return r
}

// keptRefusals is why the router's state file keeps each account's last read
// refused, by its id, on the token everyToken gives it, as Route's Refused
// reads it: on any other token, none. reads counts the times it's read.
type keptRefusals struct {
	refused map[string]quota.ReadRefusal
	reads   int
}

func (k *keptRefusals) of(id string, token tokens.Token) quota.ReadRefusal {
	k.reads++
	if token.Reveal() != everyToken[id] {
		return ""
	}
	return k.refused[id]
}

// Claude Code's own token passes over an account the router doesn't route
// yet, as one the config made the primary since the router started, whose
// token would pass the router by, or one it holds back from every request,
// its last read refused, as what goes out on that token would be refused
// too: routed, as the router's status document says; with the router down,
// as its state file keeps it, on the token the account holds now. A pin
// direct is the whole session's, as asked. Where every account is passed
// over, it's chosen as if nothing were known.
func TestClaudeCodesOwnTokenPassesOverAnAccountTheRouterHoldsBack(t *testing.T) {
	ended := map[string]quota.ReadRefusal{"work": quota.SubscriptionEnded}
	all := []string{"work", "personal", "side"}
	tests := []struct {
		name string
		// down has the router not running, going direct, as the state file
		// keeps the reads refused, kept; else the router routes the accounts
		// routes names, every one where it's nil, those refused names held.
		down    bool
		routes  []string
		refused map[string]quota.ReadRefusal
		kept    map[string]quota.ReadRefusal
		docErr  error
		// tokens are the token files there are, everyToken where nil.
		tokens  tokenstest.Files
		account string
		want    string
	}{
		{name: "routed, the primary's", want: workToken},
		{name: "routed, the primary's subscription ended: the first held back by nothing", refused: ended, want: personalToken},
		{name: "routed, the primary's token refused: the first held back by nothing", refused: map[string]quota.ReadRefusal{"work": quota.TokenRefused}, want: personalToken},
		{name: "routed, the primary held, pinned: the pinned account's", refused: ended, account: "side", want: sideToken},
		{
			name:    "routed, the primary and the pinned account held: the first held back by nothing",
			refused: map[string]quota.ReadRefusal{"work": quota.SubscriptionEnded, "side": quota.TokenRefused},
			account: "side",
			want:    personalToken,
		},
		{name: "routed, the primary not routed yet: the first the router routes", routes: []string{"personal", "side"}, want: personalToken},
		{name: "routed, the primary held, pinned to an account not routed yet: the first the router routes", routes: []string{"work", "personal"}, refused: ended, account: "side", want: personalToken},
		{
			name:    "routed, every account held: the primary's, as if nothing were known",
			refused: map[string]quota.ReadRefusal{"work": quota.SubscriptionEnded, "personal": quota.TokenRefused, "side": quota.SubscriptionEnded},
			want:    workToken,
		},
		{name: "routed, the router routing none: the primary's, as if nothing were known", routes: []string{}, want: workToken},
		{name: "routed, a refusal this build doesn't know: the primary's", refused: map[string]quota.ReadRefusal{"work": "something_new"}, want: workToken},
		{name: "routed, the router's document not to be had: the primary's", refused: ended, docErr: errors.New("no answer within 5s"), want: workToken},
		{name: "routed, the primary held, the first held back by nothing without a token: the next", refused: ended, tokens: tokenstest.Files{"work": workToken, "side": sideToken}, want: sideToken},
		{name: "direct, the primary's", down: true, want: workToken},
		{name: "direct, the primary's read kept refused: the first kept by nothing", down: true, kept: ended, want: personalToken},
		{
			name:   "direct, the primary's read kept refused on a token since replaced: the primary's new one",
			down:   true,
			kept:   ended,
			tokens: tokenstest.Files{"work": renewedToken, "personal": personalToken, "side": sideToken},
			want:   renewedToken,
		},
		{name: "direct, pinned to an account kept refused: the pinned account's, as asked", down: true, kept: map[string]quota.ReadRefusal{"side": quota.SubscriptionEnded}, account: "side", want: sideToken},
		{
			name: "direct, every account kept refused: the primary's, as if nothing were known",
			down: true,
			kept: map[string]quota.ReadRefusal{"work": quota.SubscriptionEnded, "personal": quota.TokenRefused, "side": quota.SubscriptionEnded},
			want: workToken,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			routes, files := tt.routes, tt.tokens
			if routes == nil {
				routes = all
			}
			if files == nil {
				files = everyToken
			}
			rt := healthy().routing(routes, tt.refused)
			rt.docErr = tt.docErr
			if tt.down {
				rt = notRunning()
			}
			kept := &keptRefusals{refused: tt.kept}
			r := route(rt, tt.account)
			r.Document, r.Refused, r.Token = rt.Status, kept.of, files.Read

			if err := h.launcher.Run(t.Context(), r, nil); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got := h.environment(t)["CLAUDE_CODE_OAUTH_TOKEN"]; got != tt.want {
				t.Errorf("started on %q, want %q", got, tt.want)
			}
			switch {
			case tt.down && len(rt.docsOf) > 0:
				t.Errorf("with the router down, its document asked for, of the pools %q, want none", rt.docsOf)
			case !tt.down && !slices.Equal(rt.docsOf, []string{""}):
				t.Errorf("routed, the router's document asked for of the pools %q, want once, of none, as without pools", rt.docsOf)
			case !tt.down && kept.reads > 0:
				t.Errorf("routed, the state file read %d times, want none: the router says how the accounts stand", kept.reads)
			}
		})
	}
}

func TestPassingOverAnAccountForClaudeCodesOwnTokenSaysWhyInTheLog(t *testing.T) {
	tests := []struct {
		name   string
		routes []string
		want   string
	}{
		{name: "not routed yet", routes: []string{"personal", "side"}, want: `reason="the router doesn't route it yet"`},
		{name: "its read refused", routes: []string{"work", "personal", "side"}, want: `reason="its last read was refused (subscription_ended)"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			h := newHarness(t)
			rt := healthy().routing(tt.routes, map[string]quota.ReadRefusal{"work": quota.SubscriptionEnded})
			r := route(rt, "")
			r.Document, r.Token = rt.Status, everyToken.Read

			if err := h.launcher.Run(t.Context(), r, nil); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			want := []string{"level=INFO", `msg="passing over an account for Claude Code's own token"`, "account=work", `chosen="the primary"`, tt.want}
			if !log.Has(want...) {
				t.Errorf("log reads\n%s\nwant a line with %q", log, want)
			}
		})
	}
}

// Where the config can't be read, Claude Code's own token passes over an
// account the router holds back as Run's does.
func TestRunUnreadPassesOverAnAccountTheRouterHoldsBack(t *testing.T) {
	h := newHarness(t, "HOME=/home/tester")
	rt := healthy().naming("side", accounts...)
	rt.doc.Accounts[2].RefusedRead = quota.SubscriptionEnded

	if err := h.launcher.RunUnread(t.Context(), unreadRoute(rt, ""), nil, errUnread); err != nil {
		t.Fatalf("RunUnread() error = %v", err)
	}
	if got := h.environment(t)["CLAUDE_CODE_OAUTH_TOKEN"]; got != workToken {
		t.Errorf("started on %q, want %q: side, the router's primary, held back, work the first with a usable token", got, workToken)
	}
}

// In a pool, the router's document of the session's pool says how its
// accounts stand: one the config made the pool's primary since the router
// started, which it doesn't route yet, is passed over for the first it
// routes.
func TestRunStartsOnAnAccountOfThePoolTheRouterRoutes(t *testing.T) {
	before := pooledConfig()
	cfg := pooledConfig()
	cfg.Accounts[3].Primary = false
	cfg.Accounts = append(cfg.Accounts, config.Account{ID: "acme3", Label: "Acme 3", Pool: acmePool.ID, Primary: true})
	tests := []struct {
		name     string
		dir      string
		wantPool string
		want     string
	}{
		{name: "in the pool with a new primary", dir: acmeDir, wantPool: acmePool.ID, want: acme1Token},
		{name: "in another pool", dir: elsewhere, wantPool: defaultPool.ID, want: workToken},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			h.launcher.Dir = tt.dir
			rt := knowing(before)
			var routes []string
			for _, a := range before.Accounts.InPool(tt.wantPool) {
				routes = append(routes, a.ID)
			}
			rt.routing(routes, nil)
			r := pooledRoute(cfg, rt, "")
			r.Document = rt.Status
			r.Token = tokenstest.Files{"work": workToken, "acme1": acme1Token, "acme2": acme2Token, "acme3": "test-token-acme3"}.Read

			if err := h.launcher.Run(t.Context(), r, nil); err != nil {
				t.Fatalf("Run() error = %v", err)
			}
			if got := h.environment(t)["CLAUDE_CODE_OAUTH_TOKEN"]; got != tt.want {
				t.Errorf("started on %q, want %q", got, tt.want)
			}
			if !slices.Equal(rt.docsOf, []string{tt.wantPool}) {
				t.Errorf("the router's document asked for of the pools %q, want once, of %q, the session's", rt.docsOf, tt.wantPool)
			}
		})
	}
}
