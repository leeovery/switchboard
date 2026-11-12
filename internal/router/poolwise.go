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

// in returns the id of the pool the account with the given id is in, one t
// has, as configured says: "", the one pool there is, in a config from
// before pools; reporting false where it's in none, as one that has left a
// config with pools is, and every account of a closed table is.
func (t *poolTable) in(id string) (string, bool) {
	if !t.pooled() {
		return "", t.closed == ""
	}
	pool, ok := t.poolOf[id]
	return pool, ok && t.configured(pool)
}

// within reports whether requests can be routed within the pool with the
// given id: one t has, as configured says, or, in a config from before
// pools, "", the one pool there is; none in a closed table.
func (t *poolTable) within(pool string) bool {
	if !t.pooled() {
		return pool == "" && t.closed == ""
	}
	return t.configured(pool)
}
