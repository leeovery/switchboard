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

// PercentSet shows a share set, such as a reserve, as a percentage to a
// hundredth of a point at most, as GroupedTo writes it, so a share between
// whole percentages shows as it was set: "5%", "2.5%".
func PercentSet(share float64) string {
	return GroupedTo(share*100, 2) + "%"
}

// PercentWhole is Percent of a window used whole, "100%": the widest of its
// use to its limit, as a window's figure is held to.
func PercentWhole() string {
	return Percent(1)
}

// PercentLeft is the share of a window left where utilization of it is used,
// as a whole percentage, such as "77%": what the whole percentage Percent
// shows of the use leaves of 100, so the two, each as said, sum to 100%
// within its limit; and past it, as of a window read at 102%, "0%", never
// below.
func PercentLeft(utilization float64) string {
	return strconv.Itoa(max(100-WholePercent(utilization), 0)) + "%"
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
	return toPlaces(NearestTenth(x), 1)
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

// GroupedTo writes x to the places given at most, as toPlaces writes it, but
// with no trailing zeros, its whole part's thousands set apart by commas, as
// Grouped sets them: "1,240", "12.5", "0.25", or 100.0 to a place "100".
func GroupedTo(x float64, places int) string {
	figure := toPlaces(x, places)
	if strings.Contains(figure, ".") {
		figure = strings.TrimSuffix(strings.TrimRight(figure, "0"), ".")
	}
	if figure == "-0" {
		figure = "0"
	}
	return withThousands(figure)
}

// toPlaces writes x to the places given, rounded as its decimal is nearest,
// as every figure with places is written.
func toPlaces(x float64, places int) string {
	return strconv.FormatFloat(x, 'f', places, 64)
}

// withThousands sets apart the thousands of the figure written by commas,
// its sign and its places after the point kept as they are.
func withThousands(figure string) string {
	sign, digits := "", figure
	if unsigned, ok := strings.CutPrefix(figure, "-"); ok {
		sign, digits = "-", unsigned
	}
	whole, places, pointed := strings.Cut(digits, ".")
	var b strings.Builder
	b.WriteString(sign)
	for i, d := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(d)
	}
	if pointed {
		b.WriteString("." + places)
	}
	return b.String()
}
