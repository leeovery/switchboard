package views

import (
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/status"
)

// sighting is a request of a session as what names the session reads it:
// its model, the account it went to, and when it came.
type sighting struct {
	model, account string
	at             time.Time
}

// naming is what names a session, of its requests, as its lines and the
// request stream tell: the latest of its own conversation, its main thread,
// that went to an account; and the latest that went to one, whatever its
// class. A request the router answered itself went to none.
type naming struct {
	main, latest sighting
}

// see takes in a request of the session, of the model and class given, gone
// to the account given at at: none where it went to none.
func (n *naming) see(model, account, class string, at time.Time) {
	if account == "" {
		return
	}
	s := sighting{model: model, account: account, at: at}
	if !at.Before(n.latest.at) {
		n.latest = s
	}
	if class == ledger.ClassMain && !at.Before(n.main.at) {
		n.main = s
	}
}

// heard returns n, the session's requests the stream told of, told, taken in
// too, each as it was sent, on the account it went out on last.
func (n naming) heard(told []Told) naming {
	for _, t := range told {
		n.see(t.Model, t.Account, t.Class, t.Sent)
	}
	return n
}

// names returns the account and model naming the session, as the List, a
// session's page and SESSIONS name it, its models going where goes has them,
// as the router lists them and the request stream has moved them since: its
// main thread's, the model of its latest main request, and the account that
// model goes to now, else the one that request went to. Where none of its
// requests is its main thread's, as of a Claude Code from before requests
// said their class, or none of them yet read or told of, it's the model
// seen last, as goes has them, and the account it goes to; else, where the
// router lists none, its latest request's.
func (n naming) names(goes []status.Assignment) (account, model string) {
	if !n.main.at.IsZero() {
		if i := slices.IndexFunc(goes, func(a status.Assignment) bool { return a.Model == n.main.model }); i >= 0 {
			return goes[i].Account, n.main.model
		}
		return n.main.account, n.main.model
	}
	if len(goes) > 0 {
		last := slices.MaxFunc(goes, func(a, b status.Assignment) int { return a.LastSeen.Compare(b.LastSeen) })
		return last.Account, last.Model
	}
	return n.latest.account, n.latest.model
}
