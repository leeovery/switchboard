package views

// LogFilter is what the Log's Requests show, as its f panel picks it. A
// choice ticked keeps those ticked; none ticked is every one: so the zero
// LogFilter shows every line, and the Log opens on LogOpening's.
type LogFilter struct {
	// Accounts, Models and Sessions keep the lines of the accounts, the
	// models and the sessions ticked, each by its whole id: a line of none,
	// as a request the router answered itself is on no account, is kept by
	// none ticked.
	Accounts, Models, Sessions []string
	// Kinds keeps the lines of what the kinds ticked asked.
	Kinds []RowKind
	// Failed keeps only those that failed.
	Failed bool
}

// LogOpening is the filter the Log opens on, as the filters aren't
// remembered: its requests alone, Claude Code's quota checks and its counts
// of tokens hidden until they're shown.
func LogOpening() LogFilter {
	return LogFilter{Kinds: []RowKind{RowRequest}}
}

// rowKinds are the kinds of RowKind, in the panel's order.
var rowKinds = [...]RowKind{RowRequest, RowCheck, RowCount}

// hiding is what of the filter f hides the row r: none; its kind alone; or a
// choice of another, its kind kept or not.
func (f LogFilter) hiding(r RequestRow) hidden {
	switch {
	case !f.chose(r):
		return hiddenChosen
	case !ticked(f.Kinds, r.Kind):
		return hiddenKind
	}
	return notHidden
}

// chose reports whether the filter's choices but its kinds keep the row r.
func (f LogFilter) chose(r RequestRow) bool {
	return ticked(f.Accounts, r.Account) && ticked(f.Models, r.Model) && ticked(f.Sessions, r.Session) && (!f.Failed || r.Failed)
}

// hidden is what of a filter hides a row.
type hidden int

const (
	// notHidden is a row the filter shows.
	notHidden hidden = iota
	// hiddenKind is a row only its kind hides.
	hiddenKind
	// hiddenChosen is a row a choice of another hides.
	hiddenChosen
)

// LogCount is what a day's lines count, each once: those of each kind, in
// flight included, together each of the day's lines; those that failed; by
// their models, accounts and sessions; and those the table shows of them.
// The counts but Shown are the day's whatever the filter, as the f panel
// counts them, so with Shown, the heading's hidden make the day's lines.
type LogCount struct {
	// Requests, Checks and Counts are the day's lines of what each asked:
	// those the views count among the requests, Claude Code's quota checks,
	// and its counts of tokens.
	Requests, Checks, Counts int
	// Failed are those that failed, as RequestRow's Failed has it: 400 and
	// over, cut off or given up, and lost.
	Failed int
	// Models, Accounts and Sessions count the lines by the model each asked
	// for, the account it's on, as RequestRow's Account has it, and the
	// session it's of, each by its whole id: one of none is counted by none.
	Models, Accounts, Sessions map[string]int
	// Shown are those the table shows.
	Shown int
	// byKind are those its kind alone hides, by the kind; and chosen, those
	// a choice of another hides.
	byKind [len(rowKinds)]int
	chosen int
}

// Lines is how many of the day's lines there are: each kind's together.
func (c LogCount) Lines() int {
	return c.Requests + c.Checks + c.Counts
}

// Hidden is how many of the day's lines the table hides.
func (c LogCount) Hidden() int {
	return c.Lines() - c.Shown
}

// HiddenBy are the kinds that hide the day's lines hidden, in the panel's
// order, where their kinds alone hide them, as the heading names them, as
// ": checks and counts": none where a choice of another hides any, as the
// heading then says how many alone, or where none is hidden.
func (c LogCount) HiddenBy() []RowKind {
	if c.chosen > 0 {
		return nil
	}
	var by []RowKind
	for _, k := range rowKinds {
		if c.byKind[k] > 0 {
			by = append(by, k)
		}
	}
	return by
}

// add counts the row r, the filter hiding it as given.
func (c *LogCount) add(r RequestRow, hid hidden) {
	switch r.Kind {
	case RowCheck:
		c.Checks++
	case RowCount:
		c.Counts++
	default:
		c.Requests++
	}
	if r.Failed {
		c.Failed++
	}
	c.Models, c.Accounts, c.Sessions = tally(c.Models, r.Model), tally(c.Accounts, r.Account), tally(c.Sessions, r.Session)
	switch hid {
	case notHidden:
		c.Shown++
	case hiddenKind:
		c.byKind[r.Kind]++
	case hiddenChosen:
		c.chosen++
	}
}

// tally is counted with one more of the id given, made where it's nil: as
// it was for none.
func tally(counted map[string]int, id string) map[string]int {
	if id == "" {
		return counted
	}
	if counted == nil {
		counted = make(map[string]int)
	}
	counted[id]++
	return counted
}
