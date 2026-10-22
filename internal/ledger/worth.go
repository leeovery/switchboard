package ledger

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/quota"
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
	// Dollar and cent are how many picodollars each is.
	Dollar Picodollars = 1_000_000_000_000
	cent               = Dollar / 100
)

// Cents shows the amount to the nearest cent, as "$412.53".
func (p Picodollars) Cents() string {
	cents := p.wholeCents()
	return fmt.Sprintf("%s$%d.%02d", p.sign(), cents/100, cents%100)
}

// ToTheCent is the amount to the nearest cent, as Cents shows it, a half
// cent away from none.
func (p Picodollars) ToTheCent() Picodollars {
	if p < 0 {
		return -p.wholeCents() * cent
	}
	return p.wholeCents() * cent
}

// wholeCents is the amount without its sign, to the nearest cent.
func (p Picodollars) wholeCents() Picodollars {
	return (p.abs() + cent/2) / cent
}

// Dollars shows the amount to the nearest dollar, as "$413".
func (p Picodollars) Dollars() string {
	return fmt.Sprintf("%s$%d", p.sign(), p.wholeDollars())
}

// GroupedDollars shows the amount to the nearest dollar, its thousands set
// apart, as "$27,480".
func (p Picodollars) GroupedDollars() string {
	return p.sign() + "$" + prose.Grouped(int(p.wholeDollars()))
}

// wholeDollars is the amount without its sign, to the nearest dollar.
func (p Picodollars) wholeDollars() Picodollars {
	return (p.abs() + Dollar/2) / Dollar
}

// MarshalJSON gives the amount in dollars, exactly, as 412.532118.
func (p Picodollars) MarshalJSON() ([]byte, error) {
	dollars := fmt.Sprintf("%s%d", p.sign(), p.abs()/Dollar)
	if part := p.abs() % Dollar; part > 0 {
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
	// AsOf is the date of the day the prices were read, as 2026-10-11.
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
	return t.priced(model, geo, billOf(countsOf(usage)), false, date)
}

// Request returns what the line's request would have cost through the API at
// the prices in effect on the local day with the given date, as Worth says,
// of the usage worth prices, as worthUsage gives it, so a quota check is
// worth nothing: one that went upstream whose answer gave no usage unpriced,
// never free. Only tests reach it, directly or through Summed, the reference
// SummedSpend is held to.
func (t Table) Request(l *Line, date string) (Worth, bool) {
	return t.priced(l.Model, l.Shape.InferenceGeo, billOf(countsOf(l.worthUsage())), l.unmetered(), date)
}

// CacheWrite returns what the line's request's writes to the prompt cache
// alone would have cost through the API, as Request prices them: each at its
// model's price of a write that lasts as long as it does, five minutes or an
// hour, writes its usage doesn't break down by how long they last unpriced.
// It's what a move cost, as a request that moved its session wrote its
// context again on the account it moved to.
func (t Table) CacheWrite(l *Line, date string) (Worth, bool) {
	return t.priced(l.Model, l.Shape.InferenceGeo, billOf(countsOf(l.Usage).only(cacheWrites, cacheTTLs)), l.unmetered(), date)
}

// Summed returns what the line's request adds to a sum of requests' worth at
// the prices in effect on the local day with the given date, reporting
// whether the table priced it: its worth, as Request gives it; or, where the
// table can't price it, as of a model it doesn't know, none, naming what it
// leaves unpriced as a summary's model's day names it, as wholly says, so the
// sum names what it leaves out. Only tests call it, the reference
// SummedSpend is held to.
func (t Table) Summed(l *Line, date string) (Worth, bool) {
	if w, ok := t.Request(l, date); ok {
		return w, true
	}
	return Worth{Unpriced: t.wholly(l.Model, l.Shape.InferenceGeo, countsOf(l.worthUsage()).costly(), l.unmetered(), date)}, false
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
	tokens, metered := l.Tokens()
	return t.rewrite(l.Model, l.Shape.InferenceGeo, tokens, metered, life, date)
}

// rewrite returns what writing a prompt of the tokens given into the prompt
// cache again would cost, as Rewrite says, of a request for the model with
// the given id that asked for the geo given, metered where its answer gave
// usage.
func (t Table) rewrite(model, geo string, tokens quota.Tokens, metered bool, life time.Duration, date string) (Picodollars, bool) {
	prices, priced := t.pricesIn(model, geo, date)
	if !metered || !priced {
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
// last: five minutes where every write is a five-minute one, else an hour, as
// Claude Code has a subscription's last, writes it doesn't break down by how
// long they last counting as an hour's. It reports false where the reply
// wrote nothing to the cache, as one that only read from it, which says
// nothing of how long its writes last. Only tests call it, the reference
// Spend's CacheLife is held to.
func (r Reply) CacheLife() (time.Duration, bool) {
	held := countsOf(r.Usage)
	written, _ := held[cacheWrites].(int64)
	ttls, _ := held[cacheTTLs].(counts)
	short, _ := ttls[shortWrites].(int64)
	return cacheLife(written, held.total(cacheTTLs), short)
}

// cacheLife returns how long the prompt cache's writes of a usage last, as
// CacheLife says, of the count of its writes as a whole, written; of those
// its cache_creation breaks down by how long each lasts, brokenDown; and of
// those it says last five minutes, short.
func cacheLife(written, brokenDown, short int64) (time.Duration, bool) {
	switch written = max(written, brokenDown); {
	case written == 0:
		return 0, false
	case short == written:
		return ShortCache, true
	}
	return LongCache, true
}

// priced returns what the bill given, of a request for the model with the
// given id that asked for the geo given, would have cost, as Worth says,
// naming noUsage among what it leaves unpriced where unmetered says requests
// it's of went upstream without usage.
func (t Table) priced(model, geo string, billed bill, unmetered bool, date string) (Worth, bool) {
	prices, ok := t.pricesIn(model, geo, date)
	if !ok {
		return Worth{}, false
	}
	w := prices.worth(billed)
	if unmetered {
		w.Unpriced = append(w.Unpriced, noUsage)
		slices.Sort(w.Unpriced)
	}
	return w, true
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

// pricesIn returns the prices of the model with the given id in effect on the
// local day with the given date, as prices finds them, in the geo given, as
// in has them. It reports false for a model the table doesn't know, or a geo
// it doesn't price the model in.
func (t Table) pricesIn(model, geo, date string) (Prices, bool) {
	prices, ok := t.prices(model, date)
	if !ok {
		return Prices{}, false
	}
	return prices.in(geo)
}

// SamePrices reports whether the model with the given id costs alike on the
// local days with the dates a and b: the prices in effect on each, as prices
// finds them, the same but for the day they took effect, so whatever is
// priced on one is worth as much on the other. A model the table doesn't know
// costs alike, as none, on any day.
func (t Table) SamePrices(model, a, b string) bool {
	onA, _ := t.prices(model, a)
	onB, _ := t.prices(model, b)
	onA.From, onB.From = "", ""
	return onA == onB
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

// charge is a count a price charges for, as chargeOf finds it by its path.
type charge int

// The counts a price charges for: input; writes to the prompt cache that
// last five minutes, and an hour; reads from it; output; and web searches.
const (
	chargeInput charge = iota
	chargeWrite5m
	chargeWrite1h
	chargeRead
	chargeOutput
	chargeWebSearch
	charges
)

// chargeOf returns the charge of the count at path, reporting false for a
// count no price charges for.
func chargeOf(path string) (charge, bool) {
	switch path {
	case "input_tokens":
		return chargeInput, true
	case cacheTTLs + "." + shortWrites:
		return chargeWrite5m, true
	case cacheTTLs + ".ephemeral_1h_input_tokens":
		return chargeWrite1h, true
	case "cache_read_input_tokens":
		return chargeRead, true
	case "output_tokens":
		return chargeOutput, true
	case "server_tool_use.web_search_requests":
		return chargeWebSearch, true
	}
	return 0, false
}

// byCharge are what each of every count p charges for costs, by its charge.
func (p Prices) byCharge() [charges]Picodollars {
	return [charges]Picodollars{
		chargeInput: p.Input, chargeWrite5m: p.CacheWrite5m, chargeWrite1h: p.CacheWrite1h,
		chargeRead: p.CacheRead, chargeOutput: p.Output, chargeWebSearch: p.WebSearch,
	}
}

// bill is a usage's counts as its prices bill them: how many it counts of
// each charge, every path giving it summed, and the paths of those no price
// charges for, in order, as billOf has them.
type bill struct {
	charged  [charges]int64
	unpriced []string
}

// billOf returns the bill of usage, as counts: each count a price charges
// for, the prompt cache's writes as how long each lasts breaks them down. Any
// other count but one of none is unpriced, unless it costs nothing of its
// own, as uncharged says, or it's the cache's writes as a whole, as far as
// they're broken down: those beyond may have lasted either long.
func billOf(usage counts) bill {
	var b bill
	brokenDown := usage.total(cacheTTLs)
	usage.each("", func(path string, n int64) {
		switch c, charged := chargeOf(path); {
		case charged:
			b.charged[c] += n
		case n != 0 && !free(path) && (path != cacheWrites || n > brokenDown):
			b.unpriced = append(b.unpriced, path)
		}
	})
	slices.Sort(b.unpriced)
	return b
}

// same reports whether b and o are the same bill.
func (b bill) same(o bill) bool {
	return b.charged == o.charged && slices.Equal(b.unpriced, o.unpriced)
}

// worth returns what the bill given costs at p: each count it charges for
// at its price, and what it leaves unpriced.
func (p Prices) worth(billed bill) Worth {
	prices := p.byCharge()
	w := Worth{Unpriced: slices.Clone(billed.unpriced)}
	for c, n := range billed.charged {
		w.Cost += Picodollars(n) * prices[c]
	}
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
// table doesn't know; and what the worth leaves out, as wholly unpriced says
// of one that has none, and otherwise the paths of the counts in its usage
// that can't be priced, no_usage among them where some of its requests went
// upstream without usage.
type PricedModel struct {
	ModelDay
	Worth    *Picodollars `json:"worth,omitempty"`
	Unpriced []string     `json:"unpriced,omitempty"`
}

// What a model's day with no worth names as unpriced, as wholly unpriced
// says: noModel where its requests named no model, and the geo they asked
// for, after inferenceGeo, as inference_geo:eu, where the table doesn't price
// the model in it.
const (
	noModel      = "no_model"
	inferenceGeo = "inference_geo:"
)

// Priced returns the summary priced at the prices in effect on the local day
// with the given date.
func (t Table) Priced(s Summary, date string) Priced {
	priced := Priced{Summary: s}
	for _, a := range s.Accounts {
		account := PricedAccount{AccountDay: a}
		for _, m := range a.Models {
			model := PricedModel{ModelDay: m}
			held := countsOf(m.Usage)
			if w, ok := t.priced(m.Model, m.InferenceGeo, billOf(held), m.NoUsage > 0, date); ok {
				model.Worth, model.Unpriced = &w.Cost, w.Unpriced
			} else {
				model.Unpriced = t.wholly(m.Model, m.InferenceGeo, held.costly(), m.NoUsage > 0, date)
			}
			account.Models = append(account.Models, model)
		}
		priced.Accounts = append(priced.Accounts, account)
	}
	return priced
}

// wholly names why requests the table prices none of, of the model with the
// given id that asked for the geo given, are unpriced on the local day with
// the given date, costly where their usage holds a count that costs
// anything, unmetered where some went upstream without usage: noModel where
// they named no model; the geo, as inference_geo:eu, where the table knows
// the model but doesn't price it there; else the model's id; and noUsage
// beside it where unmetered. It's none where they spent nothing, neither
// costly nor unmetered, so their worth is none, not unknown.
func (t Table) wholly(model, geo string, costly, unmetered bool, date string) []string {
	var named string
	_, known := t.prices(model, date)
	switch {
	case !costly && !unmetered:
		return nil
	case model == "":
		named = noModel
	case known:
		named = inferenceGeo + geo
	default:
		named = model
	}
	unpriced := []string{named}
	if unmetered {
		unpriced = append(unpriced, noUsage)
	}
	slices.Sort(unpriced)
	return unpriced
}

// costly reports whether c holds a count that costs anything.
func (c counts) costly() bool {
	costly := false
	c.each("", func(path string, n int64) { costly = costly || n != 0 && !free(path) })
	return costly
}
