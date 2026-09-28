package router

// Event is news from the router that something outside it may want to act
// on, such as by notifying the user: LimitReached, Moved, Refused or
// HealthChanged. Config.Events hears each as it happens.
type Event interface {
	event()
}

// LimitReached is an account's limit reached, as the upstream answered a
// request on it: Windows are the keys of those it rejected, which can be
// none when only its overall verdict did.
type LimitReached struct {
	Account string
	Windows []string
}

// Moved is a session's requests of a model moving to another account, and
// why, as the routed line's reason says.
type Moved struct {
	Session string
	Model   string
	From    string
	To      string
	Reason  string
}

// Refused is the upstream refusing an account's token, answering with Status.
type Refused struct {
	Account string
	Status  int
}

// HealthChanged is the router turning unhealthy, saying why, or healthy
// again.
type HealthChanged struct {
	Healthy bool
	Reason  string
}

func (LimitReached) event()  {}
func (Moved) event()         {}
func (Refused) event()       {}
func (HealthChanged) event() {}
