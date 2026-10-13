package status_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/status/statustest"
)

// said is how Accounts says an account stands: its state in a word, as
// Compare's head and Detail's list say it, and Detail's head, each after its
// glyph.
type said struct {
	state, head string
}

// saidOf is how Accounts says the account a stands.
func saidOf(a status.AccountNow) said {
	state, head := a.Routing.Kind.State(), a.Headed()
	return said{state: state.Glyph() + " " + state.Word(), head: head.Kind.Glyph() + " " + head.String()}
}

// At each hard moment, Accounts says how each account stands as its state
// in a word and Detail's head: every hard moment's every account listed.
func TestAccountsSaysHowEachAccountStandsAtEachHardMoment(t *testing.T) {
	want := map[string]map[string]said{
		"just after a reset, before the reread": {
			"work": {state: "● open", head: "○ idle · window starts with its next request"},
			"side": {state: "● open", head: "● open · new sessions come here"},
		},
		"a failed read after an earlier one": {
			"work": {state: "● pressure", head: "● under pressure"},
			"side": {state: "● open", head: "● open · new sessions come here"},
		},
		"its token refused": {
			"work": {state: "■ refused", head: "■ refused (401) · its token needs renewing"},
			"side": {state: "● open", head: "● open · new sessions come here"},
		},
		"a stale document, built three hours before now": {
			"work": {state: "● open", head: "○ idle · window starts with its next request"},
			"side": {state: "● open", head: "● open · new sessions come here"},
		},
		"its session lapsed": {
			"work": {state: "● open", head: "○ idle · window starts at its prime, 13:30"},
			"side": {state: "● open", head: "● open"},
		},
		"a week at its reserve when built": {
			"work": {state: "● at cap", head: "● at its cap · new sessions go elsewhere"},
			"side": {state: "● open", head: "● open · new sessions come here"},
		},
		"a week at its reserve when built, then reset": {
			"work": {state: "● open", head: "● open"},
			"side": {state: "● open", head: "● open · new sessions come here"},
		},
		"a week at its reserve when built, the global pin naming it": {
			"work": {state: "● at cap", head: "● at its cap · pinned, so new sessions still come here"},
			"side": {state: "● open", head: "● open"},
		},
		"the global pin naming both, the better under pressure only at its reserve": {
			"work": {state: "● open", head: "● open"},
			"side": {state: "● open", head: "● open · new sessions come here"},
		},
		"a limit holding": {
			"work": {state: "■ limit", head: "■ limit reached · new sessions go elsewhere"},
			"side": {state: "● open", head: "● open · new sessions come here"},
		},
		"a limit holding, then lifted": {
			"work": {state: "● open", head: "○ idle · window starts with its next request"},
			"side": {state: "● open", head: "● open · new sessions come here"},
		},
		"a limit naming no window holding": {
			"work": {state: "■ limit", head: "■ limit reached · new sessions go elsewhere"},
			"side": {state: "● open", head: "● open · new sessions come here"},
		},
		"the global pin's one account held back, new sessions going elsewhere": {
			"work": {state: "■ limit", head: "■ limit reached · new sessions go elsewhere"},
			"side": {state: "● open", head: "● open · new sessions come here"},
		},
		"no account able to take a request": {
			"work": {state: "■ limit", head: "■ limit reached · new sessions go elsewhere"},
			"side": {state: "■ refused", head: "■ refused (401) · its token needs renewing"},
		},
		"its token refused, a limit naming no window holding": {
			"work": {state: "■ limit", head: "■ limit reached · new sessions go elsewhere"},
			"side": {state: "● open", head: "● open · new sessions come here"},
		},
		"one account without a token, another not read yet": {
			"work": {state: "○ not read", head: "✕ no token · switchboard accounts token work"},
			"side": {state: "○ not read", head: "○ not read yet"},
		},
		"its first read failed, nothing read before": {
			"work": {state: "○ not read", head: "! can't read it · HTTP 529 · Overloaded"},
			"side": {state: "● open", head: "● open · new sessions come here"},
		},
		"probed, of several accounts": {
			"work": {state: "● open", head: "● open"},
			"side": {state: "● open", head: "● open"},
		},
		"probed, of one account, with no room": {
			"work": {state: "■ limit", head: "■ limit reached"},
		},
		"a week at its limit, reset since read, before the reread": {
			"work": {state: "● open", head: "● open"},
			"side": {state: "● open", head: "● open · new sessions come here"},
		},
		"a model's own window at its limit, its account open to the rest": {
			"work": {state: "● open", head: "■ Fable wk limit · other models still come here"},
			"side": {state: "● open", head: "● open"},
		},
		"the global pin set from the command line to an account at its limit": {
			"work": {state: "■ limit", head: "■ limit reached · new sessions go elsewhere"},
			"side": {state: "● open", head: "● open · new sessions come here"},
		},
		"the global pin naming one account, a pool of one": {
			"work": {state: "● open", head: "● open"},
			"side": {state: "● open", head: "● open · new sessions come here"},
		},
	}
	for _, hm := range statustest.HardMoments(policy) {
		t.Run(hm.Name, func(t *testing.T) {
			wanted, ok := want[hm.Name]
			if !ok {
				t.Fatal("no account's said wanted at this moment")
			}
			m := hm.Doc.At(hm.Now, policy)
			if len(m.Accounts) != len(wanted) {
				t.Errorf("%d accounts, want %d", len(m.Accounts), len(wanted))
			}
			for _, a := range m.Accounts {
				if got := saidOf(a); got != wanted[a.ID] {
					t.Errorf("%s is said %+v, want %+v", a.ID, got, wanted[a.ID])
				}
			}
		})
	}
}

// What's said of where new sessions go, by Detail's head.
const (
	comeHere      = "new sessions come here"
	stillComeHere = "new sessions still come here"
	goElsewhere   = "new sessions go elsewhere"
)

// At each hard moment, what Accounts says of each account agrees with
// ROUTING and with ▲: Detail's head is of ROUTING's kind, its glyph and
// colour, and its first words, never says when room returns, and says new
// sessions come to an account open or under pressure where ▲ marks it alone,
// and go elsewhere where it doesn't; the pool's states counted, as ROUTING
// counts them, are each of its accounts' once, in the state its kind wants;
// and all accounts takes every account's windows whose use is known, whatever
// the pool, or with one account, none.
func TestAccountsAgreesWithROUTINGAtEachHardMoment(t *testing.T) {
	for _, hm := range statustest.HardMoments(policy) {
		t.Run(hm.Name, func(t *testing.T) {
			m := hm.Doc.At(hm.Now, policy)
			for _, a := range m.Accounts {
				checkHeadAgrees(t, a)
			}
			checkPoolsStatesCounted(t, m, hm.Want)
			checkAllTakesEveryAccount(t, m)
		})
	}
}

// checkHeadAgrees checks Detail's head of the account a against ROUTING's
// words and ▲.
func checkHeadAgrees(t *testing.T, a status.AccountNow) {
	t.Helper()
	head, routing := a.Headed(), a.Routing
	if head.Kind != routing.Kind || head.Says != routing.Says {
		t.Errorf("%s: the head is %q of kind %d, want ROUTING's first words, %q, and kind, %d", a.ID, head, head.Kind, routing.Says, routing.Kind)
	}
	if slices.ContainsFunc(head.Clauses, func(c string) bool { return strings.HasPrefix(c, "back ") }) {
		t.Errorf("%s: the head says %q, when room returns", a.ID, head)
	}
	comes := slices.Contains(head.Clauses, comeHere) || slices.Contains(head.Clauses, stillComeHere)
	opens := slices.Contains([]status.RoutingKind{status.RoutingOpen, status.RoutingNext, status.RoutingPressed}, routing.Kind)
	if comes != (opens && a.Next) {
		t.Errorf("%s: the head says %q, new sessions coming here: %v; ▲ marks it: %v", a.ID, head, comes, a.Next)
	}
	if slices.Contains(head.Clauses, goElsewhere) && a.Next {
		t.Errorf("%s: the head says %q, but ▲ marks it", a.ID, head)
	}
}

// checkPoolsStatesCounted checks the moment m's pool's states counted, as
// ROUTING counts them, against the pool's accounts, each in the state its
// kind, as wanted, has it.
func checkPoolsStatesCounted(t *testing.T, m status.Moment, want statustest.Facts) {
	t.Helper()
	var pooled []status.AccountNow
	var wanted [status.StateNotRead + 1]int
	for _, a := range m.Accounts {
		if a.InPool {
			pooled = append(pooled, a)
			wanted[want.Accounts[a.ID].Routing.State()]++
		}
	}
	counts := status.CountStates(pooled)
	if _, of := counts.OpenOf(); of != len(m.Pool().Accounts) {
		t.Errorf("the pool's states counted are %+v, %d of them; want its %d accounts", counts, of, len(m.Pool().Accounts))
	}
	for _, c := range counts {
		if c.N != wanted[c.State] {
			t.Errorf("the pool's states counted are %+v; want %d %s", counts, wanted[c.State], c.State.Word())
		}
	}
}

// checkAllTakesEveryAccount checks all accounts' windows at the moment m
// against every account's: each window counts every account whose use of it
// is known, whatever the pool; and with one account, there are none.
func checkAllTakesEveryAccount(t *testing.T, m status.Moment) {
	t.Helper()
	all := m.All()
	if len(m.Accounts) < 2 {
		if len(all.Windows) > 0 {
			t.Errorf("with one account, all accounts has windows %+v, want none", all.Windows)
		}
		return
	}
	for _, w := range all.Windows {
		known := 0
		for _, a := range m.Accounts {
			if aw, ok := a.Window(w.Key); ok && aw.Kind != status.WindowTokenless && aw.Kind != status.WindowUnread {
				known++
			}
		}
		if w.Of != known || w.Of == 0 {
			t.Errorf("all accounts' %s counts %d accounts, want the %d whose use of it is known, of %d, whatever the pool's %d", w.Key, w.Of, known, len(m.Accounts), len(m.Pool().Accounts))
		}
	}
}
