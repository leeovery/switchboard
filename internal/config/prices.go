package config

import (
	"errors"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// Plans are the plans a subscription can be on, by the names the config
// gives them.
var Plans = []string{"pro", "max5x", "max20x"}

// planNames names Plans, as "pro, max5x or max20x".
func planNames() string {
	last := len(Plans) - 1
	return strings.Join(Plans[:last], ", ") + " or " + Plans[last]
}

// Prices are the config's prices in place of those switchboard carries, each
// in US dollars.
type Prices struct {
	// Plans are plans' prices, a month, by their names, each one of Plans.
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

// checkPrices keeps each price the config gives a number, 0 or more, and
// each plan it prices one of Plans, a plan it doesn't know being an unknown
// key. A kind of token it doesn't know is an unknown key too, which decoding
// leaves unused.
func checkPrices(p Prices) error {
	var errs []error
	for _, plan := range slices.Sorted(maps.Keys(p.Plans)) {
		key := toml.Key{"prices", "plans", plan}.String()
		if !slices.Contains(Plans, plan) {
			errs = append(errs, unknownKey(key))
			continue
		}
		errs = append(errs, checkPrice(key, p.Plans[plan], "the plan's price in US dollars a month"))
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

// checkPrice keeps the price the config gives key a number, 0 or more, the
// error saying what it's the price of as of does.
func checkPrice(key string, price float64, of string) error {
	if price >= 0 && !math.IsInf(price, 1) {
		return nil
	}
	return wrongValue(key, strconv.FormatFloat(price, 'g', -1, 64), "must be a number, 0 or more: "+of)
}
