package router

// each returns the ids of the pools requests are routed within, in the
// config's order: "", the one pool there is, in a config from before pools.
func (t *poolTable) each() []string {
	if !t.pooled() {
		return []string{""}
	}
	ids := make([]string, len(t.pools))
	for i, p := range t.pools {
		ids[i] = p.ID
	}
	return ids
}

// in returns the id of the pool the account with the given id is in, of
// those each gives, as holds says: "", the one pool there is, in a config
// from before pools; reporting false where it's in none, as one that has
// left a config with pools is.
func (t *poolTable) in(id string) (string, bool) {
	for _, pool := range t.each() {
		if t.holds(pool, id) {
			return pool, true
		}
	}
	return "", false
}

// within reports whether requests can be routed within the pool with the
// given id, as the pools spell it, as of does: one t has, as configured
// says, or, in a config from before pools, "", the one pool there is.
func (t *poolTable) within(pool string) bool {
	return t.of(pool) == pool && (pool == "" || t.configured(pool))
}
