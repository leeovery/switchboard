package status

import "slices"

// What Detail's head says of an account under pressure that new sessions go
// to.
const stillComeHere = "new sessions still come here"

// Headed is how the account stands as Detail's head says it: ROUTING's kind,
// which its glyph and colour follow, and its first words, then, in place of
// ROUTING's clauses, where new sessions go, never when room returns. A limit
// holding back its every request, or its cap, sends them elsewhere where ▲
// marks another account, as Elsewhere says, as in "limit reached · new
// sessions go elsewhere"; where it marks this one, the one account there is,
// or none, as where no account has room, it says nothing of them, as there's
// nowhere else they go. Its cap the global pin spends, a model's limit, cap
// or refusal, and its token refused say what ROUTING says but its back time,
// as in "Fable wk limit · other models still come here". Under pressure, new
// sessions still come here, and open, they come here, where ▲ marks it.
// Without a usable token, not read, unreadable or idle, it says all ROUTING
// says. Its short form is its first words alone.
func (a AccountNow) Headed() Routing {
	r := a.Routing
	switch r.Kind {
	case RoutingLimited, RoutingCapped:
		return r.headed(onlyIf(a.Elsewhere, goElsewhere))
	case RoutingCappedPinned, RoutingModelLimited, RoutingModelCapped, RoutingRefused, RoutingModelRefused:
		return r.headed(r.unbacked())
	case RoutingPressed:
		return r.headed(onlyIf(a.Next, stillComeHere))
	case RoutingOpen, RoutingNext:
		return r.headed(onlyIf(a.Next, comeHere))
	}
	return r
}

// Unpinned is how the account stands where a mark beside its words says the
// global pin names it: its clauses and its short form, after its first
// words, without those that say so, as its cap the pin spends has them.
func (r Routing) Unpinned() Routing {
	saysPin := func(s string) bool { return s == stillPinned || s == pinnedShort }
	r.Clauses = slices.DeleteFunc(slices.Clone(r.Clauses), saysPin)
	if len(r.Short) > 0 {
		r.Short = append([]string{r.Short[0]}, slices.DeleteFunc(slices.Clone(r.Short[1:]), saysPin)...)
	}
	return r
}

// headed is r as Detail's head says it: its kind and first words, the
// clauses given after them, and its first words alone short.
func (r Routing) headed(clauses []string) Routing {
	return Routing{Kind: r.Kind, Says: r.Says, Clauses: clauses, Short: []string{r.Says}}
}

// unbacked are r's clauses but the one saying when room returns, as backed
// says.
func (r Routing) unbacked() []string {
	if !r.backed() {
		return slices.Clone(r.Clauses)
	}
	return slices.Clone(r.Clauses[1:])
}

// backed reports whether r's first clause says when room returns, as Back
// says there's a time it does.
func (r Routing) backed() bool {
	return !r.Back.IsZero() && len(r.Clauses) > 0
}

// Timed reports whether r says a time, counting two: when it's back, in its
// first clause and its short form's second part, where it has one, as
// backed says; and, under pressure, when its pressure window runs out, in
// its first clause. When a prime starts an idle account's window isn't
// counted: an idle account takes requests now, and its prime says only when
// its window starts, which ranks below the pin's mark.
func (r Routing) Timed() bool {
	return r.backed() || (r.Kind == RoutingPressed && len(r.Clauses) > 0)
}
