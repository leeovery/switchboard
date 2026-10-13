package views

import (
	"slices"

	"github.com/leeovery/switchboard/internal/status"
)

// autoOf is the account the router would send the next request of the
// session s to were its own pin cleared, of the model naming it: that model's
// assignment's Auto, as the router lists it. It's "" where the router lists
// no assignment of the model, or can't say, as where no account has room, or
// it's from before it said.
func autoOf(s status.Session, model string) string {
	if i := slices.IndexFunc(s.Assignments, func(a status.Assignment) bool { return a.Model == model }); i >= 0 {
		return s.Assignments[i].Auto
	}
	return ""
}
