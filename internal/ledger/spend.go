package ledger

import (
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
)

// Spend is what a line's request spent, its usage decoded once into what the
// calls priced over it read: each gives exactly what the line's own call
// gives, which decodes the usage every time. So a view that holds a request
// holds none of its raw usage. Its zero value is the spend of a line without
// usage, of a request that never went upstream.
type Spend struct {
	// all is its usage's bill, as Request prices it.
	all bill
	// written is the count of the prompt cache's writes as a whole, and
	// brokenDown the sum of those its cache_creation breaks down by how long
	// each lasts, as CacheLife reads them.
	written, brokenDown int64
	// decoded is set where its usage decodes into tokens, as Reply.Tokens
	// has it; costly where its usage holds a count that costs anything;
	// check where it's a quota check's, whose usage worth prices as none, as
	// worthUsage has it; and unmetered as Line.unmetered has it.
	decoded, costly, check, unmetered bool
	// odd is what its usage gives otherwise than all derives it, as few
	// usages do: nil where none of it is.
	odd *oddSpend
}

// oddSpend is what a usage gives otherwise than its bill derives it: its
// tokens, where Reply.Tokens reads them otherwise than its counts, as of a
// count named in capitals, which it takes for the API's own name; and the
// bill of its writes to the cache alone, as CacheWrite prices them, where a
// count beside cache_creation's names a path within it.
type oddSpend struct {
	tokens quota.Tokens
	writes bill
}

// SpendOf returns what the line's request spent, its usage decoded once.
func SpendOf(l *Line) Spend {
	held := countsOf(l.Usage)
	s := Spend{all: billOf(held), brokenDown: held.total(cacheTTLs), costly: held.costly(), check: l.Kind == KindCheck, unmetered: l.unmetered()}
	s.written, _ = held[cacheWrites].(int64)
	var tokens quota.Tokens
	tokens, s.decoded = tokensIn(l.Usage)
	if writes := billOf(held.only(cacheWrites, cacheTTLs)); tokens != s.plainTokens() || !writes.same(s.plainWrites()) {
		s.odd = &oddSpend{tokens: tokens, writes: writes}
	}
	return s
}

// plainTokens are the tokens s's counts give, as nearly every usage gives
// them: none where they don't decode.
func (s Spend) plainTokens() quota.Tokens {
	if !s.decoded {
		return quota.Tokens{}
	}
	return quota.Tokens{
		Input: int(s.all.charged[chargeInput]), Output: int(s.all.charged[chargeOutput]),
		CacheRead: int(s.all.charged[chargeRead]), CacheWrite: int(s.written),
	}
}

// plainWrites is the bill of s's writes to the cache alone as all gives it,
// as nearly every usage gives it: its writes of each last, and what it leaves
// unpriced of the cache's writes.
func (s Spend) plainWrites() bill {
	var writes bill
	writes.charged[chargeWrite5m], writes.charged[chargeWrite1h] = s.all.charged[chargeWrite5m], s.all.charged[chargeWrite1h]
	for _, path := range s.all.unpriced {
		if ofCacheWrites(path) {
			writes.unpriced = append(writes.unpriced, path)
		}
	}
	return writes
}

// ofCacheWrites reports whether the count at path is of the prompt cache's
// writes: their count as a whole, or one within cache_creation.
func ofCacheWrites(path string) bool {
	return path == cacheWrites || path == cacheTTLs || strings.HasPrefix(path, cacheWrites+".") || strings.HasPrefix(path, cacheTTLs+".")
}

// Tokens returns the tokens s's usage counts, as Reply.Tokens gives them.
func (s Spend) Tokens() (quota.Tokens, bool) {
	if s.odd != nil {
		return s.odd.tokens, s.decoded
	}
	return s.plainTokens(), s.decoded
}

// CacheLife returns how long the prompt cache's writes of s's usage last, as
// Reply.CacheLife gives it.
func (s Spend) CacheLife() (time.Duration, bool) {
	short := s.all.charged[chargeWrite5m]
	if s.odd != nil {
		short = s.odd.writes.charged[chargeWrite5m]
	}
	return cacheLife(s.written, s.brokenDown, short)
}

// cacheWrites is the bill of s's writes to the cache alone, as CacheWrite
// prices it.
func (s Spend) cacheWrites() bill {
	if s.odd != nil {
		return s.odd.writes
	}
	return s.plainWrites()
}

// worthy is s as Request prices it: none of a quota check's usage, as
// worthUsage has it.
func (s Spend) worthy() Spend {
	if s.check {
		return Spend{}
	}
	return s
}

// SummedSpend returns what a request that spent s, of the model with the
// given id that asked for the geo given, adds to a sum of requests' worth at
// the prices in effect on the local day with the given date, as Summed gives
// it of the request's line.
func (t Table) SummedSpend(model, geo string, s Spend, date string) (Worth, bool) {
	s = s.worthy()
	if w, ok := t.priced(model, geo, s.all, s.unmetered, date); ok {
		return w, true
	}
	return Worth{Unpriced: t.wholly(model, geo, s.costly, s.unmetered, date)}, false
}

// CacheWriteSpend returns what the writes to the prompt cache of a request
// that spent s, of the model with the given id that asked for the geo given,
// cost, as CacheWrite gives it of the request's line.
func (t Table) CacheWriteSpend(model, geo string, s Spend, date string) (Worth, bool) {
	return t.priced(model, geo, s.cacheWrites(), s.unmetered, date)
}

// RewriteSpend returns what writing the whole prompt of a request that spent
// s, of the model with the given id that asked for the geo given, into the
// prompt cache again would cost, as Rewrite gives it of the request's line.
func (t Table) RewriteSpend(model, geo string, s Spend, life time.Duration, date string) (Picodollars, bool) {
	tokens, metered := s.Tokens()
	return t.rewrite(model, geo, tokens, metered, life, date)
}
