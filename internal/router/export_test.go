package router

// NotificationQueue is how many events can wait for notifications to deal
// with them, for tests that fill the queue.
const NotificationQueue = queueSize

// SetChooser has the router choose accounts with c, for tests that choose
// them themselves.
func (r *Router) SetChooser(c Chooser) {
	r.proxy.chooser = c
}
