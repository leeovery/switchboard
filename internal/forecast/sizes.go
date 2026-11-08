package forecast

// Sizes returns each account's size, in Pro weeks, by its plan, in the order
// given: a Pro week is 1, a Max week 5 and an Ultra week 20, the plans'
// published multiples. Where any account's plan isn't one of them, as where
// it isn't known, every account is 1, as the pool's room counts them.
func Sizes(plans []string) []float64 {
	sizes := make([]float64, len(plans))
	for i, plan := range plans {
		size, ok := sizeOf(plan)
		if !ok {
			for j := range sizes {
				sizes[j] = 1
			}
			return sizes
		}
		sizes[i] = size
	}
	return sizes
}

// sizeOf is the size of a week on plan, by the name the config gives it, in
// Pro weeks. It reports false for a plan it doesn't know.
func sizeOf(plan string) (float64, bool) {
	switch plan {
	case "pro":
		return 1, true
	case "max":
		return 5, true
	case "ultra":
		return 20, true
	}
	return 0, false
}
