package launch

import (
	"context"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens"
)

// standing is how the accounts a launch can start on stand, beyond their
// tokens, as far as the launch can know: routed, as the router's status
// document of the session's pool has them; with the router down, as its
// state file keeps their reads refused. The zero standing knows nothing, and
// has nothing against any account.
type standing struct {
	// doc is the router's status document of the session's pool, and read
	// whether it was had: where it was, an account it doesn't name the router
	// doesn't route yet, as one the config added since it started.
	doc  status.Document
	read bool
	// kept is why the API refused the last read of the account with the
	// given id, on token, as the router's state file keeps it, as Route's
	// Refused reads it: nil where it isn't to be read.
	kept func(id string, token tokens.Token) claude.ReadRefusal
}

// against says why Claude Code isn't to start on token, the account's with
// the given id, while another account's will do: the router doesn't route
// the account yet, so every request on its token would pass the router by,
// unrouted and unrecorded; or the account's last read was refused, as
// status.RefusedRead judges it, which holds it back from every request till
// a read comes back clean, as whatever went out on its token would only be
// refused. "" where nothing's known against it.
func (s standing) against(id string, token tokens.Token) string {
	var refused claude.ReadRefusal
	switch {
	case s.read:
		i := slices.IndexFunc(s.doc.Accounts, func(a status.Account) bool { return strings.EqualFold(a.ID, id) })
		if i < 0 {
			return "the router doesn't route it yet"
		}
		refused = s.doc.Accounts[i].RefusedRead
	case s.kept != nil:
		refused = s.kept(id, token)
	}
	if status.RefusedRead(refused) == 0 {
		return ""
	}
	return "its last read was refused (" + string(refused) + ")"
}

// standing returns how the accounts of in's pool stand, as far as a launch,
// the router as state says, can know: routed, as the router's status
// document of the pool has them, asked for once the router has answered, as
// Document asks; else as the router's state file keeps their reads refused,
// as Refused reads it. Where the document can't be had, it knows nothing.
func (r Route) standing(ctx context.Context, state routerState, in place) standing {
	if !state.routable() {
		return standing{kept: r.Refused}
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
