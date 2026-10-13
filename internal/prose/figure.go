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
