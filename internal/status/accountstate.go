package status

// AccountState is an account's state in a word, as Accounts says it and
// counts its accounts in: how it stands with the router, in fewer words than
// ROUTING's.
type AccountState int

// The states, in the order an account's are counted in.
const (
	// StateOpen is an account new sessions can go to: with room, idle, or
	// held back from some models' requests alone, which is open to the rest.
	StateOpen AccountState = iota + 1
	// StatePressure is an account under pressure.
	StatePressure
	// StateAtCap is an account at its cap, which holds it back, or the global
	// pin spends.
	StateAtCap
	// StateLimit is an account a limit holds back from every request.
	StateLimit
	// StateRefused is an account whose token was refused.
	StateRefused
	// StateNotRead is an account whose usage isn't read: without a usable
	// token, not read yet, or unreadable.
	StateNotRead
)

// Word is the state in a word, as in "at cap".
func (s AccountState) Word() string {
	switch s {
	case StateOpen:
		return "open"
	case StatePressure:
		return "pressure"
	case StateAtCap:
		return "at cap"
	case StateLimit:
		return "limit"
	case StateRefused:
		return "refused"
	case StateNotRead:
		return "not read"
	}
	return ""
}

// Glyph is the glyph the state's word leads with: ■ held back from every
// request by a limit or a refusal; ○ not read; and ● otherwise.
func (s AccountState) Glyph() string {
	switch s {
	case StateLimit, StateRefused:
		return "■"
	case StateNotRead:
		return "○"
	}
	return "●"
}

// State is the state in a word of an account that stands as k with the
// router: held back from some models' requests alone, by a limit, a refusal
// or its cap in a model's own window, it's open, as it is to the rest, and
// idle, open; its cap, the global pin spending it or not, at cap; and
// without a usable token, or nothing read of it, not read.
func (k RoutingKind) State() AccountState {
	switch k {
	case RoutingOpen, RoutingNext, RoutingIdle, RoutingModelLimited, RoutingModelRefused, RoutingModelCapped:
		return StateOpen
	case RoutingPressed:
		return StatePressure
	case RoutingCapped, RoutingCappedPinned:
		return StateAtCap
	case RoutingLimited:
		return StateLimit
	case RoutingRefused:
		return StateRefused
	case RoutingTokenless, RoutingUnread, RoutingUnreadable:
		return StateNotRead
	}
	return 0
}

// StateCount is how many accounts are in a state.
type StateCount struct {
	State AccountState
	N     int
}

// StateCounts are accounts' states counted, as in "2 open · 1 at cap".
type StateCounts []StateCount

// CountStates are the accounts' states counted, each as its RoutingKind's
// State has it, in AccountState's order, the states none of them is in left
// out.
func CountStates(accounts []AccountNow) StateCounts {
	states := make([]AccountState, len(accounts))
	for i, a := range accounts {
		states[i] = a.Routing.Kind.State()
	}
	return counted(states)
}

// StateInAWord is the state in a word of the account at the moment with the
// given id, as its RoutingKind's State has it: not read where the moment
// doesn't have it, as an account the config has just gained.
func (m Moment) StateInAWord(id string) AccountState {
	if a, ok := m.Account(id); ok {
		return a.Routing.Kind.State()
	}
	return StateNotRead
}

// CountStatesOf are the states of the accounts with the ids given counted,
// as CountStates counts them, each as StateInAWord has it: those the config
// has, say, whatever the moment holds.
func (m Moment) CountStatesOf(ids []string) StateCounts {
	states := make([]AccountState, len(ids))
	for i, id := range ids {
		states[i] = m.StateInAWord(id)
	}
	return counted(states)
}

// counted are the states given counted, in AccountState's order, the states
// none is in left out.
func counted(states []AccountState) StateCounts {
	var n [StateNotRead + 1]int
	for _, s := range states {
		n[s]++
	}
	var counts StateCounts
	for s := StateOpen; s <= StateNotRead; s++ {
		if n[s] > 0 {
			counts = append(counts, StateCount{State: s, N: n[s]})
		}
	}
	return counts
}

// OpenOf is how many of the accounts counted are open, pressure counting as
// open, as their states counted too wide for their room say it, as in "8 of
// 10 open", and how many were counted.
func (c StateCounts) OpenOf() (open, of int) {
	for _, sc := range c {
		if sc.State == StateOpen || sc.State == StatePressure {
			open += sc.N
		}
		of += sc.N
	}
	return open, of
}
