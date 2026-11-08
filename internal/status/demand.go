package status

// Demand is the pool's use as the router's forecast takes it.
type Demand struct {
	// Day is the pool's day rate, in Pro weeks an hour, as forecast.Day
	// measures it; or, where that has too few hours with room, the pool's
	// fast rate, every account's week's fast rate in Pro weeks summed.
	Day float64 `json:"day"`
	// Hours are the hours with room Day is measured over, 24 at the most: 0
	// where it's the pool's fast rate.
	Hours float64 `json:"hours"`
	// Hold is the hold curve, the share of an account's fast rate still on
	// it so many hours on, as forecast.Hold keeps it: nil until the router
	// keeps one.
	Hold []HoldPoint `json:"hold,omitempty"`
}

// HoldPoint is a point of the hold curve: the share of an account's fast rate
// still on it after Hours.
type HoldPoint struct {
	Hours float64 `json:"hours"`
	Share float64 `json:"share"`
}
