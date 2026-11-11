package idmove

// StopAfter has the move stop once each step stop fails for is done, as one
// stopped there, by power cut or crash, stops: stop is called with each
// step's name, and each day's file's, as the history is moved.
func (m *Mover) StopAfter(stop func(step string) error) {
	m.stop = stop
}
