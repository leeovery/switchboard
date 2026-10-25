package prose

import (
	"math"
	"strconv"
	"strings"
)

// Percent shows a utilization as a whole percentage, such as "23%".
func Percent(utilization float64) string {
	return strconv.Itoa(WholePercent(utilization)) + "%"
}

// PercentWhole is Percent of a window used whole, "100%": the widest of its
// use to its limit, as a window's figure is held to.
func PercentWhole() string {
	return Percent(1)
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

// Ratio writes a ratio, as a worth against its price, then ×: to a tenth, as
// Tenths writes it, as "3.8×"; and once its tenth reaches 100, to the nearest
// whole, as "123×", so 99.95 reads "100×", never "100.0×". So it's 5 cells
// at most up to 9999×.
func Ratio(x float64) string {
	if NearestTenth(x) < ratioWholeFrom {
		return Tenths(x) + "×"
	}
	return strconv.FormatFloat(math.RoundToEven(x), 'f', 0, 64) + "×"
}

// ratioWholeFrom is the ratio Ratio writes whole from.
const ratioWholeFrom = 100

// GroupedTo writes x to the places given, its whole part's thousands set
// apart by commas, as Grouped sets them: "1,240", "12.5" or "0.25".
func GroupedTo(x float64, places int) string {
	whole, fraction, _ := strings.Cut(strconv.FormatFloat(x, 'f', places, 64), ".")
	n, _ := strconv.Atoi(whole)
	if fraction == "" {
		return Grouped(n)
	}
	return Grouped(n) + "." + fraction
}
