package ledger

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

// Picodollars are an amount of US dollars, in millionths of a millionth of a
// dollar: fine enough that every price Anthropic gives a million tokens, to
// the cent, prices a token whole, and so does US-only inference's tenth more,
// so a worth is exact, never rounded.
type Picodollars int64

// perMTok is a token's price, given as Anthropic gives it: dollars a million
// tokens.
func perMTok(dollars float64) Picodollars {
	return Picodollars(math.Round(dollars * 1e6))
}

// perThousand is the price of one of something, given as dollars a thousand,
// as Anthropic gives a web search's.
func perThousand(dollars float64) Picodollars {
	return Picodollars(math.Round(dollars * 1e9))
}

// inDollars is an amount, given in dollars, as a plan's price a month is.
func inDollars(dollars float64) Picodollars {
	return Picodollars(math.Round(dollars * 1e12))
}

const (
	// dollar and cent are how many picodollars each is.
	dollar Picodollars = 1_000_000_000_000
	cent               = dollar / 100
)

// Cents shows the amount to the nearest cent, as "$412.53".
func (p Picodollars) Cents() string {
	cents := (p.abs() + cent/2) / cent
	return fmt.Sprintf("%s$%d.%02d", p.sign(), cents/100, cents%100)
}

// MarshalJSON gives the amount in dollars, exactly, as 412.532118.
func (p Picodollars) MarshalJSON() ([]byte, error) {
	dollars := fmt.Sprintf("%s%d", p.sign(), p.abs()/dollar)
	if part := p.abs() % dollar; part > 0 {
		dollars += strings.TrimRight(fmt.Sprintf(".%012d", part), "0")
	}
	return []byte(dollars), nil
}

// abs is the amount without its sign.
func (p Picodollars) abs() Picodollars {
	return max(p, -p)
}

// sign is the amount's sign, as it's written: "-" for less than none.
func (p Picodollars) sign() string {
	if p < 0 {
		return "-"
	}
	return ""
}

// Prices are a model's prices from the day they took effect: what each count
// its usage holds costs, as Anthropic's pricing page gives them.
type Prices struct {
	// From is the date of the day they took effect, as 2026-09-22.
	From string
	// Input, CacheWrite5m, CacheWrite1h, CacheRead and Output are a token's
	// prices: of input, of writing the prompt cache for five minutes and for
	// an hour, of reading it, and of output.
	Input, CacheWrite5m, CacheWrite1h, CacheRead, Output Picodollars
	// WebSearch is a web search's.
	WebSearch Picodollars
	// USOnlyPercent is what a token costs where a request asks for US-only
	// inference, as a percentage of its price: 0 where the model doesn't offer
	// it.
	USOnlyPercent int64
}

// Model is a model the price table knows: the ids requests ask for it by, and
// its prices, each from the day they took effect, oldest first.
type Model struct {
	IDs    []string
	Prices []Prices
}

// Table is a table of models' and plans' prices, as Anthropic gave them on
// the day AsOf names, and the version table beside it.
type Table struct {
	// AsOf is the date of the day the prices were read, as 2026-10-07.
	AsOf   string
	Models []Model
	Plans  []Plan
	// Versions name the models' versions, as the views show them.
	Versions []Version
	// Overridden is set where the config's prices stand in place of any of
	// the table's, as With puts them.
	Overridden bool
}

// Worth is what a usage would have cost through the API: Cost, of what its
// prices charge for; and Unpriced, the paths of the counts it holds that they
// can't price, as a server tool's the table doesn't know, which Cost leaves
// out rather than guess at, and noUsage where requests it's of went upstream
// without usage.
type Worth struct {
	Cost     Picodollars
	Unpriced []string
}

// noUsage is what a worth names among the counts it leaves unpriced where
// requests it's of went upstream but their answers gave no usage, as a
// summary's no_usage counts them: what they spent is unknown.
const noUsage = "no_usage"

// Worth returns what usage, of a request for the model with the given id that
// asked for its inference to run in the geo given, "" where it asked for
// none, would have cost through the API at the prices in effect on the local
// day with the given date. It reports false for a model the table doesn't
// know, or a geo it doesn't price the model in: never zero.
func (t Table) Worth(model, geo string, usage json.RawMessage, date string) (Worth, bool) {
	return t.counted(model, geo, countsOf(usage), date)
}

// Request returns what the line's request would have cost through the API at
// the prices in effect on the local day with the given date, as Worth says:
// one that went upstream whose answer gave no usage unpriced, never free.
func (t Table) Request(l *Line, date string) (Worth, bool) {
	return t.worth(l.Model, l.Shape.InferenceGeo, countsOf(l.Usage), l.unmetered(), date)
}

// CacheWrite returns what the line's request's writes to the prompt cache
// alone would have cost through the API, as Request prices them: each at its
// model's price of a write that lasts as long as it does, five minutes or an
// hour, writes its usage doesn't break down by how long they last unpriced.
// It's what a move cost, as a request that moved its session wrote its
// context again on the account it moved to.
func (t Table) CacheWrite(l *Line, date string) (Worth, bool) {
	return t.worth(l.Model, l.Shape.InferenceGeo, countsOf(l.Usage).only(cacheWrites, cacheTTLs), l.unmetered(), date)
}

// Summed returns what the line's request adds to a sum of requests' worth at
// the prices in effect on the local day with the given date: its worth, as
// Request gives it; or, where the table can't price it, as of a model it
// doesn't know, nothing, every count its usage holds that costs anything
// named unpriced, and noUsage where it went upstream without usage, so the
// sum names what it leaves out.
func (t Table) Summed(l *Line, date string) Worth {
	if w, ok := t.Request(l, date); ok {
		return w
	}
	var w Worth
	countsOf(l.Usage).each("", func(path string, n int64) {
		if n != 0 && !free(path) {
			w.Unpriced = append(w.Unpriced, path)
		}
	})
	if l.unmetered() {
		w.Unpriced = append(w.Unpriced, noUsage)
	}
	slices.Sort(w.Unpriced)
	return w
}

// Rewrite returns what writing the line's request's whole prompt into the
// prompt cache again would cost through the API, at the prices in effect on
// the local day with the given date: its input, reads from the cache and
// writes to it together, each at its model's price of a write that lasts as
// long as life says, five minutes or an hour. It's what moving its session
// would cost, as the account it moved to holds none of its cache. It reports
// false where the table can't price the request, and where its answer gave
// no usage, as what it was then is unknown.
func (t Table) Rewrite(l *Line, life time.Duration, date string) (Picodollars, bool) {
	tokens, ok := l.Tokens()
	prices, priced := t.prices(l.Model, date)
	if priced {
		prices, priced = prices.in(l.Shape.InferenceGeo)
	}
	if !ok || !priced {
		return 0, false
	}
	price := prices.CacheWrite1h
	if life == ShortCache {
		price = prices.CacheWrite5m
	}
	return Picodollars(tokens.Input+tokens.CacheRead+tokens.CacheWrite) * price, true
}

// ShortCache and LongCache are how long the prompt cache's writes last: five
// minutes, or an hour, as Claude Code has a subscription's last, and as
// they're taken to where nothing says.
const (
	ShortCache = 5 * time.Minute
	LongCache  = time.Hour
)

// CacheLife returns how long the prompt cache's writes of the reply's usage
// last: five minutes where every write it breaks down by how long it lasts is
// a five-minute one, else an hour, as Claude Code has a subscription's last,
// those it doesn't break down among them. It reports false where the reply
// wrote nothing to the cache, as one that only read from it, which says
// nothing of how long its writes last.
func (r Reply) CacheLife() (time.Duration, bool) {
	held := countsOf(r.Usage)
	written, _ := held[cacheWrites].(int64)
	ttls, _ := held[cacheTTLs].(counts)
	short, _ := ttls[shortWrites].(int64)
	switch broken := held.total(cacheTTLs); {
	case written == 0 && broken == 0:
		return 0, false
	case short > 0 && short == broken:
		return ShortCache, true
	}
	return LongCache, true
}

// worth returns what the counts held would have cost, as Worth says, naming
// noUsage among what it leaves unpriced where unmetered says requests they're
// of went upstream without usage.
func (t Table) worth(model, geo string, held counts, unmetered bool, date string) (Worth, bool) {
	w, ok := t.counted(model, geo, held, date)
	if ok && unmetered {
		w.Unpriced = append(w.Unpriced, noUsage)
		slices.Sort(w.Unpriced)
	}
	return w, ok
}

// counted returns what the counts held, of a request for the model with the
// given id that asked for the geo given, would have cost, as Worth says.
func (t Table) counted(model, geo string, held counts, date string) (Worth, bool) {
	prices, ok := t.prices(model, date)
	if ok {
		prices, ok = prices.in(geo)
	}
	if !ok {
		return Worth{}, false
	}
	return prices.worth(held), true
}

// countsOf returns the counts usage holds, as add reads them.
func countsOf(usage json.RawMessage) counts {
	held := make(counts)
	held.add(usage)
	return held
}

// only returns the counts c holds by the names given, and those within them.
func (c counts) only(names ...string) counts {
	kept := make(counts, len(names))
	for _, name := range names {
		if v, ok := c[name]; ok {
			kept[name] = v
		}
	}
	return kept
}

// prices returns the prices of the model with the given id in effect on the
// local day with the given date, as inEffect finds them. It reports false
// for a model the table doesn't know.
func (t Table) prices(model, date string) (Prices, bool) {
	i := slices.IndexFunc(t.Models, func(m Model) bool { return slices.Contains(m.IDs, model) })
	if i < 0 || len(t.Models[i].Prices) == 0 {
		return Prices{}, false
	}
	return inEffect(t.Models[i].Prices, date), true
}

// dated is a price that took effect on a day of its own.
type dated interface {
	// effective is the date of the day it took effect.
	effective() string
}

func (p Prices) effective() string { return p.From }

// inEffect returns the price of all, oldest first, in effect on the local
// day with the given date: the one that took effect last, on it or before;
// or, on a day before any did, the first, which launched what it prices, as
// nothing priced it before. all holds one at least.
func inEffect[P dated](all []P, date string) P {
	in := all[0]
	for _, p := range all[1:] {
		if p.effective() <= date {
			in = p
		}
	}
	return in
}

// in returns the prices of a request that asked for its inference to run in
// the geo given: as they are for none, or "global", and each token's raised
// for US-only inference, "us", where the model offers it. It reports false
// for a geo they don't price.
func (p Prices) in(geo string) (Prices, bool) {
	switch {
	case geo == "" || geo == "global":
		return p, true
	case geo == "us" && p.USOnlyPercent > 0:
		raise := func(price Picodollars) Picodollars { return price * Picodollars(p.USOnlyPercent) / 100 }
		p.Input, p.CacheWrite5m, p.CacheWrite1h = raise(p.Input), raise(p.CacheWrite5m), raise(p.CacheWrite1h)
		p.CacheRead, p.Output = raise(p.CacheRead), raise(p.Output)
		return p, true
	}
	return Prices{}, false
}

const (
	// cacheWrites is the path of the count of the prompt cache's writes as a
	// whole, which the counts within cacheTTLs break down by how long each
	// write lasts.
	cacheWrites = "cache_creation_input_tokens"
	cacheTTLs   = "cache_creation"
	// shortWrites is the name, within cacheTTLs, of the count of the writes
	// that last five minutes.
	shortWrites = "ephemeral_5m_input_tokens"
)

// uncharged are the counts a usage holds that cost nothing of their own, by
// their paths, or the paths they're within, ending in a dot: web fetches,
// which the API charges nothing for beyond their tokens; the part of the
// output its thinking was; and the turns of the model asked for, as an
// answer's iterations give them, whose counts the usage's own sum.
var uncharged = []string{"server_tool_use.web_fetch_requests", "output_tokens_details.", "iterations.message."}

// charge returns what each of the count at path costs at p, reporting false
// for a count p doesn't charge for.
func (p Prices) charge(path string) (Picodollars, bool) {
	switch path {
	case "input_tokens":
		return p.Input, true
	case cacheTTLs + "." + shortWrites:
		return p.CacheWrite5m, true
	case cacheTTLs + ".ephemeral_1h_input_tokens":
		return p.CacheWrite1h, true
	case "cache_read_input_tokens":
		return p.CacheRead, true
	case "output_tokens":
		return p.Output, true
	case "server_tool_use.web_search_requests":
		return p.WebSearch, true
	}
	return 0, false
}

// worth returns what usage, as counts, costs at p: each count p charges for
// at its price, the prompt cache's writes as how long each lasts breaks them
// down. Any other count but one of none is unpriced, unless it costs nothing
// of its own, as uncharged says, or it's the cache's writes as a whole, as
// far as they're broken down: those beyond may have lasted either long.
func (p Prices) worth(usage counts) Worth {
	var w Worth
	brokenDown := usage.total(cacheTTLs)
	usage.each("", func(path string, n int64) {
		switch price, charged := p.charge(path); {
		case charged:
			w.Cost += Picodollars(n) * price
		case n != 0 && !free(path) && (path != cacheWrites || n > brokenDown):
			w.Unpriced = append(w.Unpriced, path)
		}
	})
	slices.Sort(w.Unpriced)
	return w
}

// free reports whether the count at path costs nothing of its own, as
// uncharged says.
func free(path string) bool {
	return slices.ContainsFunc(uncharged, func(within string) bool {
		return path == within || strings.HasSuffix(within, ".") && strings.HasPrefix(path, within)
	})
}

// each calls f with each count c holds, by its path, its name after those
// it's within, each followed by a dot, after prefix.
func (c counts) each(prefix string, f func(path string, n int64)) {
	for name, v := range c {
		switch v := v.(type) {
		case int64:
			f(prefix+name, v)
		case counts:
			v.each(prefix+name+".", f)
		}
	}
}

// total is the sum of the counts c holds directly within name.
func (c counts) total(name string) int64 {
	within, _ := c[name].(counts)
	var sum int64
	for _, v := range within {
		n, _ := v.(int64)
		sum += n
	}
	return sum
}

// Priced is a day's summary with what each model's requests were worth, as
// history gives it: worked out as it's read, never written.
type Priced struct {
	Summary
	Accounts []PricedAccount `json:"accounts,omitempty"`
}

// PricedAccount is an account's day, its models' priced.
type PricedAccount struct {
	AccountDay
	Models []PricedModel `json:"models,omitempty"`
}

// PricedModel is a model's day on an account, with what its requests would
// have cost through the API: none where the model is unpriced, as one the
// table doesn't know; and the paths of the counts in its usage the worth
// leaves out, as they can't be priced, no_usage among them where some of its
// requests went upstream without usage.
type PricedModel struct {
	ModelDay
	Worth    *Picodollars `json:"worth,omitempty"`
	Unpriced []string     `json:"unpriced,omitempty"`
}

// Priced returns the summary priced at the prices in effect on the local day
// with the given date.
func (t Table) Priced(s Summary, date string) Priced {
	priced := Priced{Summary: s}
	for _, a := range s.Accounts {
		account := PricedAccount{AccountDay: a}
		for _, m := range a.Models {
			model := PricedModel{ModelDay: m}
			if w, ok := t.worth(m.Model, m.InferenceGeo, countsOf(m.Usage), m.NoUsage > 0, date); ok {
				model.Worth, model.Unpriced = &w.Cost, w.Unpriced
			}
			account.Models = append(account.Models, model)
		}
		priced.Accounts = append(priced.Accounts, account)
	}
	return priced
}
