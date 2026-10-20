package views

// countedOnce tells, line by line, a read's lines given again from those new
// to what holds some, as a request is counted, by their keys, as turnKey
// knows each, and how many lines alike the holder holds of each: one with
// an id once, given again where one is held, as idHeld has it, or was given
// before in the read; one without as often as given, given again as often
// as one is held alike. So a page's points, a turn's lines, a session's
// requests and the Log's lines count a line the ledger's files hold twice
// alike. It notes only what it must, made as it first must, for a read of
// size lines: the ids given of those held none, and how often each key
// without an id is given of those held some.
type countedOnce struct {
	size   int
	ids    map[lineKey]bool
	idless map[turnKey]int
}

// countedOnceOf is a countedOnce of a read of size lines.
func countedOnceOf(size int) *countedOnce {
	return &countedOnce{size: size}
}

// again reports whether the line with the key k, the read's next, of which
// its holder holds held alike, is given again, as countedOnce tells it,
// noting it given.
func (c *countedOnce) again(k turnKey, held int) bool {
	switch {
	case idHeld(k, held):
		return true
	case k.request != "":
		return c.idGiven(k.lineKey)
	case held == 0:
		return false
	}
	if c.idless == nil {
		c.idless = make(map[turnKey]int)
	}
	given := c.idless[k]
	c.idless[k] = given + 1
	return given < held
}

// idHeld reports whether the line with the key k, of which its holder holds
// held alike, is one with an id held, given again however often a read
// gives it.
func idHeld(k turnKey, held int) bool {
	return k.request != "" && held > 0
}

// idGiven reports whether the id and arrival k were given before in the
// read, noting them given.
func (c *countedOnce) idGiven(k lineKey) bool {
	if c.ids[k] {
		return true
	}
	if c.ids == nil {
		c.ids = make(map[lineKey]bool, c.size)
	}
	c.ids[k] = true
	return false
}
