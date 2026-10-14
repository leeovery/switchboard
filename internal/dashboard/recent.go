package dashboard

import "github.com/leeovery/switchboard/internal/status"

// quiet is what LATELY says where it lists no events, of the document as m
// has it: that the router isn't running, or answering as it should, while
// probing for want of it; that probing as asked reads none; that the router,
// from before it told of events, wants restarting; or that it has told of
// none lately. It's nil while nothing has been read.
func (f Frame) quiet(m status.Moment) line {
	switch {
	case m.Routed && f.Outdated:
		return line{{"restart the router for recent events", dimInk}}
	case m.Routed:
		return line{{"nothing lately", dimInk}}
	case !m.Probed:
		return nil
	case m.Fallback.Router == status.RouterNotRunning:
		return line{{"the router isn't running", dimInk}}
	case m.Fallback.Router == status.RouterUnhealthy:
		return line{{"the router isn't answering", dimInk}}
	default:
		return line{{"none while probing, as asked", dimInk}}
	}
}

// quietly draws why LATELY lists no events, of the document as m has
// it, from x along row y until the column end, where it says, and returns
// the lines it drew.
func (f Frame) quietly(c *canvas, m status.Moment, x, y, end int) int {
	quiet := f.quiet(m)
	if quiet == nil {
		return 0
	}
	c.line(x, y, quiet.fit(end-x))
	return 1
}

// freshen picks out the row of the event with the given id, from x along
// row y until the column end, while it's fresh: cued, its cue fading back.
func (f Frame) freshen(c *canvas, id, x, y, end int) {
	if fade, ok := f.Fresh[id]; ok {
		cue(c, f.Look, x, y, end-x, fade)
	}
}
