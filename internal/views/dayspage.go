package views

// DaysPage is Days as the dashboard draws it: the eight weeks to this one, a
// day at a time; BY MODEL's strips, a family each, on one scale; and BY
// ACCOUNT's, an account each, on another.
type DaysPage struct {
	// Weeks are the eight weeks' first days' dates, oldest first, this week
	// last, and ToCome how many of this week's days are still to come.
	Weeks  []string
	ToCome int
	// ByModel are the families used in the eight weeks, a strip each, in the
	// version table's order; ByAccount the configured accounts, in the
	// config's.
	ByModel   []DaysStrip
	ByAccount []DaysStrip
}

// DaysStrip is a strip of Days, a family's or an account's: Of, the family's
// name, as the version table gives it, or the account's id; its days, the
// eight weeks' from the first, those still to come among them; of a family,
// Share, its share of the eight weeks' worth; and of an account, Limits, the
// 5-hour limits it hit.
type DaysStrip struct {
	Of     string
	Days   []StripDay
	Share  *float64
	Limits int
}

// StripDay is a day of a strip: its level, 0 to 4, on the strips' scale; and
// of an account's, whether it hit a 5-hour limit that day.
type StripDay struct {
	Level int
	Limit bool
}
