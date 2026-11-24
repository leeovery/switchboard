package views

import (
	"testing"

	"github.com/leeovery/switchboard/internal/status"
)

// A request answered on the account a limit or a refusal befell tells
// otherwise of it, as known from judges, where its account answered it,
// however its answer then ended: broken off or cut off after it began,
// though its row failed, as much as answered whole; never one answered 400
// or over, or nothing.
func TestARequestItsAccountAnsweredToldOtherwiseHoweverItsAnswerEnded(t *testing.T) {
	for _, kind := range []string{status.EventLimit, status.EventRefused} {
		var d detailing
		d.event = status.Event{Kind: kind, Account: "work"}
		for _, e := range endings {
			t.Run(kind+", "+e.name, func(t *testing.T) {
				l := endedLine("r1", e.ending)
				rec := lineRecordOf(&l)
				logged := loggedLineOf(&rec, 0, l.At, Worth{}, nil, nil)
				want := e.ending.Status == 200
				if tells, otherwise := d.knowing(logged); tells || otherwise != want {
					t.Errorf("of a %s on work, it tells %t and otherwise %t, want false and %t", kind, tells, otherwise, want)
				}
			})
		}
	}
}
