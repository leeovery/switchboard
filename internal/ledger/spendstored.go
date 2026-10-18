package ledger

import (
	"encoding/json"
	"fmt"
	"hash/maphash"

	"github.com/leeovery/switchboard/internal/quota"
)

// storedSpend is a Spend as a file stores it, as MarshalJSON writes it and
// UnmarshalJSON reads it back: the counts its bill charges for, each by its
// charge's name, so the charges' order can't renumber what's stored, and the
// paths of those it leaves unpriced; the prompt cache's writes as a whole and
// broken down; whether its usage decoded into tokens, held a count that
// costs anything, was a quota check's, and was missing from a request that
// went upstream; and what its usage gives otherwise than its counts derive,
// where it does. It holds counts alone, never a price, and leaves out every
// zero, as a file stores one for each of a day's requests.
type storedSpend struct {
	storedBill
	Written    int64      `json:"w,omitzero"`
	BrokenDown int64      `json:"b,omitzero"`
	Decoded    bool       `json:"d,omitzero"`
	Costly     bool       `json:"c,omitzero"`
	Check      bool       `json:"q,omitzero"`
	Unmetered  bool       `json:"n,omitzero"`
	Odd        *storedOdd `json:"x,omitempty"`
}

// storedBill is a bill as a file stores it: how many it counts of each
// charge, and the paths of the counts it leaves unpriced, in order.
type storedBill struct {
	Input     int64    `json:"i,omitzero"`
	Write5m   int64    `json:"5m,omitzero"`
	Write1h   int64    `json:"1h,omitzero"`
	Read      int64    `json:"r,omitzero"`
	Output    int64    `json:"o,omitzero"`
	WebSearch int64    `json:"s,omitzero"`
	Unpriced  []string `json:"u,omitempty"`
}

// storedOdd is an oddSpend as a file stores it: its tokens, and the bill of
// its writes to the cache alone.
type storedOdd struct {
	Input      int        `json:"i,omitzero"`
	Output     int        `json:"o,omitzero"`
	CacheRead  int        `json:"r,omitzero"`
	CacheWrite int        `json:"w,omitzero"`
	Writes     storedBill `json:"b,omitzero"`
}

// MarshalJSON writes s in its stored form.
func (s Spend) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(storedSpendOf(s))
	if err != nil {
		return nil, fmt.Errorf("encode a spend: %w", err)
	}
	return b, nil
}

// UnmarshalJSON reads s back from its stored form, as MarshalJSON wrote it:
// the Spend it was, exactly, so every call over it gives what it gave.
func (s *Spend) UnmarshalJSON(b []byte) error {
	var stored storedSpend
	if err := json.Unmarshal(b, &stored); err != nil {
		return fmt.Errorf("decode a spend: %w", err)
	}
	*s = stored.spend()
	return nil
}

// Sum is a sum of all s holds, as its stored form has it, seeded by seed:
// two spends stored alike sum alike, and two stored otherwise sum otherwise,
// but by a chance of one in 2^64. So a Spend read back from its stored form
// sums as the one written did, given the same seed; and two usages that
// decode into the same Spend, as the same counts written in another order
// do, sum alike.
func (s Spend) Sum(seed maphash.Seed) uint64 {
	var h maphash.Hash
	h.SetSeed(seed)
	maphash.WriteComparable(&h, struct {
		written, brokenDown                      int64
		decoded, costly, check, unmetered, isOdd bool
	}{s.written, s.brokenDown, s.decoded, s.costly, s.check, s.unmetered, s.odd != nil})
	s.all.summed(&h)
	if s.odd != nil {
		maphash.WriteComparable(&h, s.odd.tokens)
		s.odd.writes.summed(&h)
	}
	return h.Sum64()
}

// summed writes b into h: its counts of each charge, how many paths it
// leaves unpriced, then each.
func (b bill) summed(h *maphash.Hash) {
	maphash.WriteComparable(h, struct {
		charged  [charges]int64
		unpriced int
	}{b.charged, len(b.unpriced)})
	for _, path := range b.unpriced {
		maphash.WriteComparable(h, path)
	}
}

// storedSpendOf is s as a file stores it.
func storedSpendOf(s Spend) storedSpend {
	stored := storedSpend{
		storedBill: storedBillOf(s.all), Written: s.written, BrokenDown: s.brokenDown,
		Decoded: s.decoded, Costly: s.costly, Check: s.check, Unmetered: s.unmetered,
	}
	if s.odd != nil {
		t := s.odd.tokens
		stored.Odd = &storedOdd{Input: t.Input, Output: t.Output, CacheRead: t.CacheRead, CacheWrite: t.CacheWrite, Writes: storedBillOf(s.odd.writes)}
	}
	return stored
}

// spend is the Spend s stores.
func (s storedSpend) spend() Spend {
	spend := Spend{
		all: s.bill(), written: s.Written, brokenDown: s.BrokenDown,
		decoded: s.Decoded, costly: s.Costly, check: s.Check, unmetered: s.Unmetered,
	}
	if o := s.Odd; o != nil {
		tokens := quota.Tokens{Input: o.Input, Output: o.Output, CacheRead: o.CacheRead, CacheWrite: o.CacheWrite}
		spend.odd = &oddSpend{tokens: tokens, writes: o.Writes.bill()}
	}
	return spend
}

// storedBillOf is b as a file stores it.
func storedBillOf(b bill) storedBill {
	c := b.charged
	return storedBill{
		Input: c[chargeInput], Write5m: c[chargeWrite5m], Write1h: c[chargeWrite1h], Read: c[chargeRead], Output: c[chargeOutput], WebSearch: c[chargeWebSearch],
		Unpriced: b.unpriced,
	}
}

// bill is the bill s stores, leaving none unpriced where it names none, as
// billOf does.
func (s storedBill) bill() bill {
	b := bill{charged: [charges]int64{
		chargeInput: s.Input, chargeWrite5m: s.Write5m, chargeWrite1h: s.Write1h, chargeRead: s.Read, chargeOutput: s.Output, chargeWebSearch: s.WebSearch,
	}}
	if len(s.Unpriced) > 0 {
		b.unpriced = s.Unpriced
	}
	return b
}
