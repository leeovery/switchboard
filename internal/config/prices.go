package config

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// Plans are the plans a subscription can be on, by the names the config
// gives them.
var Plans = []string{PlanPro, PlanMax, PlanUltra}

// The plans, each by the name the config gives it.
const (
	PlanPro   = "pro"
	PlanMax   = "max"
	PlanUltra = "ultra"
)

// planAliases are the names plans had before Anthropic renamed them on 30
// October 2026, each by the name it has now: a config that still gives one
// loads as if it gave the new.
var planAliases = map[string]string{"max5x": PlanMax, "max20x": PlanUltra}

// planNamed returns plan by the name Plans give it: an alias's new name,
// else plan as it is.
func planNamed(plan string) string {
	if now, ok := planAliases[plan]; ok {
		return now
	}
	return plan
}

// isPlan reports whether plan names one of Plans, by its name or an alias.
func isPlan(plan string) bool {
	return slices.Contains(Plans, planNamed(plan))
}

// planNames names Plans, as "pro, max or ultra".
func planNames() string {
	last := len(Plans) - 1
	return strings.Join(Plans[:last], ", ") + " or " + Plans[last]
}

// Prices are the config's prices in place of those switchboard carries, each
// in US dollars.
type Prices struct {
	// Plans are plans' prices, a month, by their names, each one of Plans:
	// the file may give an alias, which loading renames.
	Plans map[string]float64 `toml:"plans"`
	// Models are models' prices, by their ids as the API names them.
	Models map[string]ModelPrices `toml:"models"`
}

// ModelPrices are a model's prices, a million tokens, by the kind of token:
// nil where the config leaves one out.
type ModelPrices struct {
	Input        *float64 `toml:"input"`
	Output       *float64 `toml:"output"`
	CacheRead    *float64 `toml:"cache_read"`
	CacheWrite5m *float64 `toml:"cache_write_5m"`
	CacheWrite1h *float64 `toml:"cache_write_1h"`
}

// Complete reports whether the config gives the price of every kind of
// token.
func (m ModelPrices) Complete() bool {
	return !slices.ContainsFunc(m.kinds(), func(k tokenPrice) bool { return k.price == nil })
}

// tokenPrice is a kind of token's price, by the name the config gives the
// kind.
type tokenPrice struct {
	kind  string
	price *float64
}

// kinds are the model's prices, a kind of token each, in the order the
// config file's example gives them.
func (m ModelPrices) kinds() []tokenPrice {
	return []tokenPrice{
		{kind: "input", price: m.Input}, {kind: "output", price: m.Output}, {kind: "cache_read", price: m.CacheRead},
		{kind: "cache_write_5m", price: m.CacheWrite5m}, {kind: "cache_write_1h", price: m.CacheWrite1h},
	}
}

// checkPrices keeps each price the config gives a number from 0 to
// priceMost, and each plan it prices one of Plans, priced by one name only,
// a plan it doesn't know being an unknown key. A kind of token it doesn't
// know is an unknown key too, which decoding leaves unused.
func checkPrices(p Prices) error {
	var errs []error
	for _, plan := range slices.Sorted(maps.Keys(p.Plans)) {
		key := planPriceKey(plan)
		if !isPlan(plan) {
			errs = append(errs, unknownKey(key))
			continue
		}
		errs = append(errs, checkPrice(key, p.Plans[plan], "the plan's price in US dollars a month"), checkPricedOnce(p, plan))
	}
	for _, model := range slices.Sorted(maps.Keys(p.Models)) {
		for _, k := range p.Models[model].kinds() {
			if k.price != nil {
				errs = append(errs, checkPrice(toml.Key{"prices", "models", model, k.kind}.String(), *k.price, "the price in US dollars a million tokens"))
			}
		}
	}
	return errors.Join(errs...)
}

// checkPricedOnce keeps plan, where it's an alias, from being priced by its
// new name as well.
func checkPricedOnce(p Prices, plan string) error {
	now := planNamed(plan)
	if _, both := p.Plans[now]; now == plan || !both {
		return nil
	}
	return fmt.Errorf("%s and %s price the same plan: give it one price, by %s", planPriceKey(plan), planPriceKey(now), planPriceKey(now))
}

// planPriceKey is the key in [prices.plans] that prices plan.
func planPriceKey(plan string) string {
	return toml.Key{"prices", "plans", plan}.String()
}

// renamed returns the prices with each plan's by the name Plans give it, in
// place of any alias, checkPrices having kept a plan from being priced
// twice.
func (p Prices) renamed() Prices {
	if p.Plans == nil {
		return p
	}
	plans := make(map[string]float64, len(p.Plans))
	for plan, price := range p.Plans {
		plans[planNamed(plan)] = price
	}
	p.Plans = plans
	return p
}

// priceMost is the most a price the config gives can be, in US dollars, a
// million tokens' or a plan's month: far past any real price, so each
// converts to picodollars, as worth counts them in an int64, and fits.
const priceMost = 1_000_000

// checkPrice keeps the price the config gives key a number from 0 to
// priceMost, the error saying what it's the price of as of does.
func checkPrice(key string, price float64, of string) error {
	if price >= 0 && price <= priceMost {
		return nil
	}
	return wrongValue(key, strconv.FormatFloat(price, 'g', -1, 64), "must be a number from 0 to 1,000,000: "+of)
}
