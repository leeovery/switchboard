package views

// WeeksPage is Weeks as the dashboard draws it: the eight weeks to this one,
// by their first days; each account's peaks in them and its verdict, and
// every account's; and the capacity verdict over the last 7 whole weeks.
type WeeksPage struct {
	// Weeks are the eight weeks' first days' dates, oldest first, this week
	// last.
	Weeks []string
	// Accounts are each configured account's row, in the config's order,
	// and All every account's, each week the mean of their peaks.
	Accounts []WeeksRow
	All      WeeksRow
	Capacity Capacity
}

// WeeksRow is an account's row of Weeks, or every account's: its verdict, as
// Capacity gives it, and its peak in each of the eight weeks, as Weeks lists
// them.
type WeeksRow struct {
	AccountVerdict
	Peaks []WeekPeak
}

// WeekPeak is how full an account's week got: Share, its peak, from 0 to 1,
// nil where it isn't known; and Open, set for this week's, its highest so
// far.
type WeekPeak struct {
	Share *float64
	Open  bool
}
