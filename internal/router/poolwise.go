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

// standing returns the id of the pool that the pool with the given id, as it
// was recorded, stands as in t: itself, where requests can be routed within
// it, as within says, spelt as t spells it; or, where it's "", the one pool
// of a config from before pools, and t has pools, the first, as all work
// before pools was. It reports false where it stands as none, as a pool
// since removed does.
func (t *poolTable) standing(pool string) (string, bool) {
	if pool == "" && t.pooled() {
		pool = t.first
	}
	if !t.within(pool) {
		return "", false
	}
	if p, ok := t.pools.ByID(pool); ok {
		return p.ID, true
	}
	return pool, true
}
