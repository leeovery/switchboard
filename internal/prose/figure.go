package prose

import (
	"math"
	"strconv"
)

// Percent shows a utilization as a whole percentage, such as "23%".
func Percent(utilization float64) string {
	return strconv.Itoa(WholePercent(utilization)) + "%"
}

// WholePercent is a utilization as the whole percentage Percent shows, a half
// rounded to the even one.
func WholePercent(utilization float64) int {
	return int(math.RoundToEven(utilization * 100))
}

// NearestTenth is x to the nearest tenth, a half rounded to the even one, as
// Tenths and Ratio write it, so what's judged of a figure is judged of it as
// it's written.
func NearestTenth(x float64) float64 {
	// Adding 0 makes a -0, as a sliver under nothing rounds to, a 0, which
	// would write as -0.0.
	return math.RoundToEven(x*10)/10 + 0
}

// Tenths writes x to a tenth, as NearestTenth rounds it, as "1.5" or "0.0".
func Tenths(x float64) string {
	return strconv.FormatFloat(NearestTenth(x), 'f', 1, 64)
}

// Ratio writes a ratio, as a worth against its price, to a tenth, as Tenths
// writes it, then ×, as "3.8×".
func Ratio(x float64) string {
	return Tenths(x) + "×"
}
