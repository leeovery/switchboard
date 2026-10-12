package views

// YearAsk is what the Year is asked to show: the last 12 months, or a
// calendar year; and all use, or a version's alone, or an account's.
type YearAsk struct {
	// Year is the calendar year shown, as 2025: 0 for the last 12 months.
	Year int
	// Version is the version shown alone, by its name as the version table
	// gives it, as Opus 5.5, and Account the account, by its id: neither for
	// all use.
	Version, Account string
}

// YearPage is the Year as the dashboard draws it: its grid of days, a column
// a week, and the cuts between their shades; the months' names over it;
// LAST 12 MONTHS' numbers, or a calendar year's; BY MONTH's rows; and what v
// cycles through.
type YearPage struct {
	// Weeks are the grid's columns, oldest first, each a week from its first
	// day, as week_starts sets it.
	Weeks []GridWeek
	// Cuts are the cuts between the levels, as quarterCuts gives them, in
	// requests a day; KeyCuts are them as the key gives them, rounded to 50.
	Cuts, KeyCuts []int
	// Months are the months' names over the grid, oldest first.
	Months []MonthLabel
	// Numbers are the period's numbers.
	Numbers YearNumbers
	// ByMonth are BY MONTH's rows, oldest first; and Before, filtered to a
	// version, the row for the months before it was first used: nil where
	// it's none.
	ByMonth []MonthRow
	Before  *MonthsBefore
	// Versions are the versions v cycles through, by their names, in the
	// version table's order, and Accounts the accounts, by their ids, in the
	// config's order.
	Versions, Accounts []string
}

// GridWeek is a column of the Year's grid: a week, by its first day's date,
// and its seven days, from its first.
type GridWeek struct {
	First string
	Days  []GridDay
}

// GridDay is a day of the Year's grid: its requests and its level, and
// whether it's left out, as a day still to come is, or one outside the
// calendar year shown.
type GridDay struct {
	YearDay
	LeftOut bool
}

// MonthLabel is a month's name over the Year's grid: the month, as Month
// names it, 2026-10; the column of the week it's over, by its index among
// the grid's weeks; and whether it's the current month.
type MonthLabel struct {
	Month   string
	Week    int
	Current bool
}

// YearNumbers are the numbers beside the Year's grid: its requests, its
// sessions, each counted once, the limits hit, by window, what it was worth,
// what the plans cost and the worth against it, its busiest day and its
// longest run of days with any request. Filtered to a version, Share is its
// share of all use's worth and FirstUsed the day it was first used, by its
// date.
type YearNumbers struct {
	Requests int
	Sessions int
	Limits   map[string]int
	Worth
	Against
	BusiestDay string
	LongestRun int
	Share      *float64
	FirstUsed  string
}

// MonthRow is a row of BY MONTH: the month, as Month counts it; filtered to a
// version, Share, the version's share of the month's worth; From, the day the
// ledger began recording, or the version was first used, where that was
// part-way through the month, by its date; and SoFar, set for the month so
// far.
type MonthRow struct {
	Month
	Share *float64
	From  string
	SoFar bool
}

// MonthsBefore is the row, filtered to a version, for the months before it
// was first used: the first and last of them, as Month names them, and the
// version of its family used then, by its name: "" for none.
type MonthsBefore struct {
	From, To string
	Then     string
}
