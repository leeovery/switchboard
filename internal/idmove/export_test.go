package idmove

import "time"

// StopAfter has the move stop once each step stop fails for is done, as one
// stopped there, by power cut or crash, stops: stop is called with each
// step's name, and each day's file's, as the history is moved.
func (m *Mover) StopAfter(stop func(step string) error) {
	m.stop = stop
}

// OnLookingAgain has the move call looking with an account's id as it's
// about to look again at the account's token, as one being written now.
func (m *Mover) OnLookingAgain(looking func(id string)) {
	m.lookingAgain = looking
}

// ClockedBy has the move time the API's answers naming no organization by
// clock.
func (m *Mover) ClockedBy(clock func() time.Time) {
	m.clock = clock
}
