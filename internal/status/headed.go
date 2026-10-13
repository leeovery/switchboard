package status

import "slices"

// What Detail's head says of an account under pressure that new sessions go
// to.
const stillComeHere = "new sessions still come here"

// Headed is how the account stands as Detail's head says it: ROUTING's kind,
// which its glyph and colour follow, and its first words, then, in place of
// ROUTING's clauses, where new sessions go, never when room returns. A limit
// holding back its every request, or its cap, sends them elsewhere, as in
// "limit reached · new sessions go elsewhere", but where ▲ marks it, as it
// does the one account there is, it says nothing of them. Its cap the global
// pin spends, a model's limit, cap or refusal, and its token refused say what
// ROUTING says but its back time, as in "Fable wk limit · other models still
// come here". Under pressure, new sessions still come here, and open, they
// come here, where ▲ marks it. Without a usable token, not read, unreadable
// or idle, it says all ROUTING says. Its short form is its first words alone.
func (a AccountNow) Headed() Routing {
	r := a.Routing
	switch r.Kind {
	case RoutingLimited, RoutingCapped:
		return r.headed(onlyIf(!a.Next, goElsewhere))
	case RoutingCappedPinned, RoutingModelLimited, RoutingModelCapped, RoutingRefused, RoutingModelRefused:
		return r.headed(r.unbacked())
	case RoutingPressed:
		return r.headed(onlyIf(a.Next, stillComeHere))
	case RoutingOpen, RoutingNext:
		return r.headed(onlyIf(a.Next, comeHere))
	}
	return r
}

// headed is r as Detail's head says it: its kind and first words, the
// clauses given after them, and its first words alone short.
func (r Routing) headed(clauses []string) Routing {
	return Routing{Kind: r.Kind, Says: r.Says, Clauses: clauses, Short: []string{r.Says}}
}

// unbacked are r's clauses but the one saying when room returns, the first of
// them where Back says there's one.
func (r Routing) unbacked() []string {
	if r.Back.IsZero() || len(r.Clauses) == 0 {
		return slices.Clone(r.Clauses)
	}
	return slices.Clone(r.Clauses[1:])
}

// onlyIf is the clause given, where so, else none.
func onlyIf(so bool, clause string) []string {
	if !so {
		return nil
	}
	return []string{clause}
}
