package launch

import (
	"context"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens"
)

// standing is how the accounts a launch can start on stand, beyond their
// tokens, as far as the launch can know: routed, as the router's status
// document of the session's pool has them; with the router down, as its
// state file keeps what holds them back. The zero standing knows nothing,
// and has nothing against any account.
type standing struct {
	// doc is the router's status document of the session's pool, and read
	// whether it was had: where it was, an account it doesn't name the router
	// doesn't route yet, as one the config added since it started.
	doc  status.Document
	read bool
	// kept is what holds the account with the given id back from every
	// request, on token, as the router's state file keeps it, as Route's
	// Held reads it: nil where it isn't to be read.
	kept func(id string, token tokens.Token) router.KeptHold
}

// against says why Claude Code isn't to start on token, the account's with
// the given id, while another account's will do: the router doesn't route
// the account yet, so every request on its token would pass the router by,
// unrouted and unrecorded; or it holds the account back from every request,
// as whatever went out on its token would be refused, or go out on another
// subscription: routed, where the account doesn't take requests, as
// status.Account's Takes judges it as the document was built; with the
// router down, where the state file keeps its last read refused, or its
// token another subscription's. "" where nothing's known against it.
func (s standing) against(id string, token tokens.Token) string {
	switch {
	case s.read:
		i := slices.IndexFunc(s.doc.Accounts, func(a status.Account) bool { return strings.EqualFold(a.ID, id) })
		if i < 0 {
			return "the router doesn't route it yet"
		}
		if a := s.doc.Accounts[i]; !a.Takes(s.doc.GeneratedAt) {
			return heldBack(a.Error)
		}
	case s.kept != nil:
		return keptBack(s.kept(id, token))
	}
	return ""
}

// heldBack says the router holds an account back, and why, as its error
// says, where it says.
func heldBack(why string) string {
	if why == "" {
		return "the router holds it back"
	}
	return "the router holds it back: " + why
}

// keptBack says what holds an account back, as hold, the state file's, has
// it: "" where nothing does.
func keptBack(hold router.KeptHold) string {
	switch {
	case hold.Foreign:
		return "the router kept its token as another subscription's"
	case hold.RefusedRead != "":
		return "the router kept its last read refused (" + string(hold.RefusedRead) + ")"
	}
	return ""
}

// routersPrimary is the id of the pool's primary, as the router's document
// names it: "" where the document wasn't had.
func (s standing) routersPrimary() string {
	return s.doc.Primary
}

// standing returns how the accounts of in's pool stand, as far as a launch,
// the router as state says, can know: routed, as the router's status
// document of the pool has them, asked for once the router has answered, as
// Document asks; else as the router's state file keeps what holds them back,
// as Held reads it. Where the document can't be had, it knows nothing.
func (r Route) standing(ctx context.Context, state routerState, in place) standing {
	if !state.routable() {
		return standing{kept: r.Held}
	}
	if r.Document == nil {
		return standing{}
	}
	doc, err := r.Document(ctx, in.id())
	if err != nil {
		logger.Info("can't ask the router how the pool's accounts stand", "router", state.name, "pool", in.id(), "error", err)
		return standing{}
	}
	return standing{doc: doc, read: true}
}
