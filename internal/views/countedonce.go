package views

// countedOnce tells, line by line, a read's lines given again from those new
// to what holds some, as a request is counted, by their keys, as turnKey
// knows each, against how many of the lines held have each key, as held
// counts them, none where it's nil: one with an id once, given again where
// one is held or was given before in the read; one without as often as
// given, given again as often as one is held alike. So a page's points, a
// turn's lines and a session's requests count a line the ledger's files
// hold twice alike. It notes only what it must, made as it first must, for
// a read of size lines: the ids given of those held none, and how often
// each key without an id is given of those held some.
type countedOnce struct {
	held   func(turnKey) int
	size   int
	ids    map[lineKey]bool
	idless map[turnKey]int
}

// countedOnceOf is a countedOnce of a read of size lines, against held.
func countedOnceOf(held func(turnKey) int, size int) *countedOnce {
	return &countedOnce{held: held, size: size}
}

// again reports whether the line with the key k, the read's next, is given
// again, as countedOnce tells it, noting it given.
func (c *countedOnce) again(k turnKey) bool {
	held := 0
	if c.held != nil {
		held = c.held(k)
	}
	if k.request != "" {
		return held > 0 || c.idGiven(k.lineKey)
	}
	if held == 0 {
		return false
	}
	if c.idless == nil {
		c.idless = make(map[turnKey]int)
	}
	given := c.idless[k]
	c.idless[k] = given + 1
	return given < held
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
