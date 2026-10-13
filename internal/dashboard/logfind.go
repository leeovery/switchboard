package dashboard

// LogFinding is the Log's find, as / opens it, while it's open: none yet,
// the find being built after the page.
type LogFinding struct{}

// findLine is the find's line, width cells wide, which the state line's row
// shows in its place while the find is open, reporting false while it
// isn't.
func (p LogPage) findLine(int) (line, bool) {
	return nil, false
}
