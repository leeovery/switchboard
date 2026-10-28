package setup

import "time"

// WaitForRouter has Standing wait wait for a router to answer, in place of
// launch.AskTimeout, so a test sees the limit pass without waiting it out.
func (s *Setup) WaitForRouter(wait time.Duration) {
	s.routerWait = wait
}
