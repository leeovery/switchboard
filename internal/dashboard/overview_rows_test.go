package dashboard

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/status"
)

// rowsDown is how many rows the units take laid out in turn: each its rows,
// or its heading's alone where it folds, and a blank row between two, but
// two folded.
func rowsDown(units []unit) int {
	n := 0
	for i, u := range units {
		n += u.taken()
		if i > 0 && (!units[i-1].fold || !u.fold) {
			n++
		}
	}
	return n
}

// nextStep is the fewest rows the unit grows by next, as growth grows it:
// USAGE and ROUTING to their next form whole, USAGE given way back a step
// among them, SESSIONS a whole session, any other a row.
func nextStep(u unit) int {
	if !u.ledBy(SectionUsage, SectionRouting, SectionSessions) {
		return 1
	}
	next := u.most()
	for _, s := range u.sections {
		for _, step := range slices.Concat(s.between, s.leaner, []int{u.least}) {
			if step > u.rows {
				next = min(next, step)
			}
		}
	}
	return next - u.rows
}

// leastShort is the fewest rows the unit takes short of folding a section
// for it: USAGE's first step short of its least, its accounts a row apart;
// any other's least.
func leastShort(u unit) int {
	if leaner := u.sections[0].leaner; len(leaner) > 0 {
		return leaner[0]
	}
	return u.least
}

// COMING UP and LATELY sit side by side where each one's least fits beside
// the other, COMING UP's 42 cells in round(w × 0.45) from 93 columns, a name
// counted at its least of 8 whatever its width, so the switch falls at 93
// for a name of 8 cells, 14 or 22 alike; under it, LATELY is under COMING UP.
func TestCOMINGUPAndLATELYSitSideBySideWhereTheirLeastsFitWhateverTheNames(t *testing.T) {
	if got := comingLeast(); got != 42 {
		t.Fatalf("COMING UP's least is %d cells, want 42: ~Sat 06:00, two blanks, a name at 8, is primed and its countdown's 12", got)
	}
	if got := latelyLeast(); got != 21 {
		t.Fatalf("LATELY's least is %d cells, want 21: Sun 02:00, two blanks, its mark and a blank, and a name at 8", got)
	}
	for _, name := range []string{"personal", "work-team-plan", "side-hustle-consulting"} {
		named := readAccount("work", weekOf(0.33, 5*quota.Day))
		named.Label = name
		doc := routerDoc("work", 1, named)
		for width, beside := range map[int]bool{52: false, 73: false, 92: false, 93: true, 100: true, 160: true} {
			f := frameOf(width, 60)
			units := f.unitsOf(f.sectionsOf(doc, now))
			last := units[len(units)-1]
			if got := len(last.sections) == 2; got != beside {
				t.Errorf("named %q at %d columns, COMING UP and LATELY side by side %v, want %v", name, width, got, beside)
			}
		}
	}
}

// Every height from 10 to 120 at the phone's widths, the iPad's, the
// landscape phone's and the desktop's, with the sections shown at first and
// with every one, of three accounts by window and by account, three of three
// windows by account, and six and ten by window: a unit folded never fits at
// its least with the others at theirs, and rows are left blank at the foot
// only where every unit not folded is at its most, or none's next step fits
// in them.
func TestNoUnitFoldsWhereItFitsAndNoRowIsLeftBlankWhereAStepFits(t *testing.T) {
	ten, _ := tenAccounts()
	fabled := threeRouted()
	fabled.Accounts = slices.Clone(fabled.Accounts)
	for i := range fabled.Accounts {
		a := &fabled.Accounts[i]
		a.Windows = append(slices.Clone(a.Windows), windowOf("7d_oi", "Fable week", 0.3, 4*quota.Day))
	}
	worlds := []struct {
		name string
		doc  status.Document
		by   By
	}{
		{name: "three by window", doc: threeRouted()},
		{name: "three by account", doc: threeRouted(), by: ByAccount},
		{name: "three of three windows by account", doc: fabled.WorkedOut(claudeLike), by: ByAccount},
		{name: "six by window", doc: manyRead(6)},
		{name: "ten by window", doc: ten},
	}
	for _, w := range worlds {
		for _, shown := range []Sections{{}, every} {
			for _, width := range []int{52, 73, 108, 138, 160, 220} {
				t.Run(fmt.Sprintf("%s, %v at %d", w.name, shown.Names(), width), func(t *testing.T) {
					t.Parallel()
					for height := 10; height <= 120; height++ {
						f := frameOf(width, height)
						f.Sections, f.Sessions, f.By = shown, flippingSessions(), w.by
						allottedFully(t, f, w.doc)
					}
				})
			}
		}
	}
}

// allottedFully holds the units of doc as the frame allots them to the
// rules: none folded that fits at its least, the others at theirs, USAGE's
// blank rows between its accounts given way; and no row blank at the foot
// that a unit not folded, short of its most, could take its next step in.
func allottedFully(t *testing.T, f Frame, doc status.Document) {
	t.Helper()
	units, room := f.allotted(doc, now), f.viewRows()
	for i, u := range units {
		if !u.fold {
			continue
		}
		atLeast := slices.Clone(units)
		for j := range atLeast {
			atLeast[j].rows = leastShort(atLeast[j])
		}
		atLeast[i].fold = false
		if rowsDown(atLeast) <= room {
			t.Errorf("at %dx%d, %s is folded, but fits at its least, %d rows, in %d", f.Width, f.Height, u.sections[0].heading(), u.least, room)
		}
	}
	spare := room - rowsDown(units)
	for _, u := range units {
		if !u.fold && u.rows < u.most() && nextStep(u) <= spare {
			t.Errorf("at %dx%d, %s takes %d rows of its most %d, with %d blank at the foot that its next step of %d fits in",
				f.Width, f.Height, u.sections[0].heading(), u.rows, u.most(), spare, nextStep(u))
		}
	}
}

func TestSESSIONSRowsAreASessionAndHowManyMoreToEveryOneASessionAtATime(t *testing.T) {
	tests := []struct {
		name         string
		running, per int
		least, most  int
		between      []int
	}{
		{name: "none running: the row saying so", running: 0, per: 1, least: 3, most: 3},
		{name: "one: it alone", running: 1, per: 1, least: 3, most: 3},
		{name: "one, two rows a session", running: 1, per: 2, least: 4, most: 4},
		{name: "two: one and how many more is no fewer than both", running: 2, per: 1, least: 4, most: 4},
		{name: "six, a row a session", running: 6, per: 1, least: 4, most: 8, between: []int{5, 6, 7}},
		{name: "six, two rows a session", running: 6, per: 2, least: 5, most: 14, between: []int{7, 9, 11, 13}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			least, most, between := sessionsRows(tt.running, tt.per)
			if least != tt.least || most != tt.most || !slices.Equal(between, tt.between) {
				t.Errorf("sessionsRows = %d, %d, %v; want %d, %d, %v", least, most, between, tt.least, tt.most, tt.between)
			}
		})
	}
}

// SESSIONS' least is a session and the row saying how many more, or every
// one where that's no fewer: it folds only where even that doesn't fit, beside
// USAGE's 13 rows and the blank row between.
func TestSESSIONSFoldsOnlyWhereASessionAndHowManyMoreDontFit(t *testing.T) {
	tests := []struct {
		name         string
		running, per int
		rows, given  int
		fold         bool
	}{
		{name: "a row a session: one, and how many more", running: 6, per: 1, rows: 18, given: 4},
		{name: "a row a session: a row short", running: 6, per: 1, rows: 17, fold: true},
		{name: "two rows a session: one, and how many more", running: 6, per: 2, rows: 19, given: 5},
		{name: "two rows a session: a row short", running: 6, per: 2, rows: 18, fold: true},
		{name: "one running: it alone", running: 1, per: 1, rows: 17, given: 3},
		{name: "one running: a row short", running: 1, per: 1, rows: 16, fold: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			laid := planOf(frameOf(160, 50), []section{sized(SectionUsage, 13, 13), sessionsSized(tt.running, tt.per)}, 0, tt.rows)
			if got := rowsGiven(laid)[SectionSessions]; laid[1].fold != tt.fold || !tt.fold && got != tt.given {
				t.Errorf("in %d rows, SESSIONS folds %v, taking %d; want it folded %v, taking %d", tt.rows, laid[1].fold, got, tt.fold, tt.given)
			}
		})
	}
}

// The rows to spare go to SESSIONS first, a whole session at a time, two rows
// a session here, to every one, its row saying how many more going once every
// one shows; a row left that's less than a session goes on to GRAPHS, then
// COMING UP and LATELY, never left blank.
func TestSESSIONSGrowsFirstAWholeSessionAtATimeWhatsLeftGoingOn(t *testing.T) {
	sections := []section{
		sized(SectionUsage, 13, 35), sized(SectionGraphs, 9, 23), sessionsSized(6, 2),
		sized(SectionComing, 5, 9), sized(SectionLately, 5, 14),
	}
	tests := []struct {
		name                     string
		rows                     int
		sessions, graphs, coming int
	}{
		{name: "none to spare: a session and how many more", rows: 35, sessions: 5, graphs: 9, coming: 5},
		{name: "a row, less than a session: to GRAPHS", rows: 36, sessions: 5, graphs: 10, coming: 5},
		{name: "a session's two rows: to SESSIONS, before GRAPHS", rows: 37, sessions: 7, graphs: 9, coming: 5},
		{name: "a session's two rows and one: the one to GRAPHS", rows: 38, sessions: 7, graphs: 10, coming: 5},
		{name: "five sessions and how many more", rows: 43, sessions: 13, graphs: 9, coming: 5},
		{name: "every session, the last's second row where how many more was", rows: 44, sessions: 14, graphs: 9, coming: 5},
		{name: "every session, and a row to GRAPHS", rows: 45, sessions: 14, graphs: 10, coming: 5},
		{name: "GRAPHS at its most too, a row on to COMING UP and LATELY", rows: 59, sessions: 14, graphs: 23, coming: 6},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			laid := planOf(frameOf(160, 120), sections, 0, tt.rows)
			want := map[Section]int{SectionUsage: 13, SectionGraphs: tt.graphs, SectionSessions: tt.sessions, SectionComing: tt.coming, SectionLately: tt.coming}
			if got := rowsGiven(laid); !maps.Equal(got, want) {
				t.Errorf("in %d rows, the sections take %v, want %v", tt.rows, got, want)
			}
			if got := laid[len(laid)-1].lastRow() + 1; got != tt.rows {
				t.Errorf("in %d rows, the last section ends on row %d, want no row left blank", tt.rows, got)
			}
		})
	}
}

// leanUsage is USAGE sized at 13 rows, giving way to 11, its accounts a row
// apart; to 10, its headings' blank rows gone; and to 8, its key gone. At
// each step it shows its three accounts in its rows, two a row short of
// them, one two rows short, and none shorter.
func leanUsage() section {
	usage := sized(SectionUsage, 13, 13)
	usage.leaner = []int{11, 10, 8}
	steps := slices.Concat([]int{usage.least}, usage.leaner)
	usage.accounts = func(given giving, rows int) int {
		return max(0, 3-max(steps[given]-rows, 0))
	}
	return usage
}

// USAGE's blank rows between its accounts give way only where, keeping them,
// a section open would fold, the rows they free going on to growth; where
// every section open stays open with them, they stay. Every other section
// folded, short of its least, USAGE takes the step that shows the most
// accounts, its blank rows between accounts given way, then those under its
// headings, then its key, the rows a step leaves blank at the foot, and
// scrolls where none fits. Here ROUTING takes 6 rows to 11, and COMING UP
// and LATELY, side by side, 5 to 14.
func TestUSAGEsBlankRowsGiveWayOnlyWhereASectionWouldFoldOrAtTheShortestHeights(t *testing.T) {
	sections := []section{leanUsage(), sized(SectionRouting, 6, 11), sized(SectionComing, 5, 9), sized(SectionLately, 5, 14)}
	others := []Section{SectionRouting, SectionComing, SectionLately}
	tests := []struct {
		name            string
		rows            int
		usage           int
		lean            giving
		routing, coming int
		folds           []Section
	}{
		{name: "every section fits: the blank rows stay", rows: 26, usage: 13, routing: 6, coming: 5},
		{name: "a row short: they give way, the row they free over going on to COMING UP and LATELY", rows: 25, usage: 11, lean: givenApart, routing: 6, coming: 6},
		{name: "two rows short: they give way, and nothing folds", rows: 24, usage: 11, lean: givenApart, routing: 6, coming: 5},
		{name: "COMING UP and LATELY fold all the same: they stay", rows: 23, usage: 13, routing: 6, coming: 1, folds: others[1:]},
		{name: "ROUTING folds all the same: they stay", rows: 16, usage: 13, routing: 1, coming: 1, folds: others},
		{name: "USAGE itself a row short of its least: they give way, as that shows every account", rows: 15, usage: 11, lean: givenApart, routing: 1, coming: 1, folds: others},
		{name: "two rows short: they give way", rows: 14, usage: 11, lean: givenApart, routing: 1, coming: 1, folds: others},
		{name: "a row shorter: its headings' blank rows give way", rows: 13, usage: 10, lean: givenHeads, routing: 1, coming: 1, folds: others},
		{name: "then its key, a row left blank at the foot", rows: 12, usage: 8, lean: givenKey, routing: 1, coming: 1, folds: others},
		{name: "past every step, it scrolls in the rest", rows: 10, usage: 7, lean: givenKey, routing: 1, coming: 1, folds: others},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			units := allot(frameOf(160, 50).unitsOf(sections), tt.rows)
			laid := frameOf(160, 50).place(units, 0)
			want := map[Section]int{SectionUsage: tt.usage, SectionRouting: tt.routing, SectionComing: tt.coming, SectionLately: tt.coming}
			if got := rowsGiven(laid); !maps.Equal(got, want) {
				t.Errorf("in %d rows, the sections take %v, want %v", tt.rows, got, want)
			}
			if got := folds(laid); !slices.Equal(got, tt.folds) {
				t.Errorf("in %d rows, %v fold, want %v", tt.rows, got, tt.folds)
			}
			if got := units[0].lean; got != tt.lean {
				t.Errorf("in %d rows, USAGE has given way %d steps, want %d", tt.rows, got, tt.lean)
			}
		})
	}
}

// USAGE's heading keeps the blank row under it while its block keeps those
// under its windows' headings, and gives it way with them: its body starts
// on the row under that blank, or straight under its heading.
func TestUSAGEsHeadingGivesWayItsBlankRowWithItsWindowsHeadings(t *testing.T) {
	f := frameOf(138, 60)
	usage := f.usageSection(threeRouted(), now)
	if len(usage.leaner) != 3 {
		t.Fatalf("USAGE gives way in %v, want three steps", usage.leaner)
	}
	for _, tt := range []struct {
		lean  giving
		rows  int
		heads int
	}{
		{lean: givenNone, rows: usage.least, heads: sectionHead},
		{lean: givenApart, rows: usage.leaner[0], heads: sectionHead},
		{lean: givenHeads, rows: usage.leaner[1], heads: 1},
		{lean: givenKey, rows: usage.leaner[2], heads: 1},
		{lean: givenKey, rows: 4, heads: 1},
	} {
		u := unit{sections: []section{usage}, least: usage.least, rows: tt.rows, lean: tt.lean}
		if got := f.place([]unit{u}, 0)[0].body.y; got != tt.heads {
			t.Errorf("given way %d steps, in %d rows, USAGE's body starts on row %d, want %d", tt.lean, tt.rows, got, tt.heads)
		}
	}
}

// threeAccountsRow matches the row of one of threeRouted's accounts in USAGE's
// block.
var threeAccountsRow = regexp.MustCompile(`^\s?[1-3] (work|personal|side)`)

// Each step USAGE's block gives way takes its own part, and keeps the rest:
// as it is, its accounts two rows apart, a blank row under its windows'
// headings, and its key; its blank rows between accounts given way, its
// accounts a row apart, the rest kept; its headings' blank rows given way
// too, its key kept; and its key given way too. In the matrix, stacked, and
// on a phone.
func TestEachStepUSAGEGivesWayTakesItsOwnPart(t *testing.T) {
	tests := []struct {
		given      giving
		apart      int
		blankUnder bool
		key        bool
	}{
		{given: givenNone, apart: 2, blankUnder: true, key: true},
		{given: givenApart, apart: 1, blankUnder: true, key: true},
		{given: givenHeads, apart: 1, blankUnder: false, key: true},
		{given: givenKey, apart: 1, blankUnder: false, key: false},
	}
	for _, width := range []int{160, 108, 52} {
		views := frameOf(width, 60).overviewBlock(threeRouted(), now).views()
		for _, tt := range tests {
			b := views.given(tt.given).block
			c := growing(width)
			b.key(c, b.draw(c, 0)+1)
			rows := textOf(c.rows(Look{}))
			read := strings.Join(rows, "\n")
			heading := slices.IndexFunc(rows, func(r string) bool { return strings.Contains(r, "WEEK") })
			first := slices.IndexFunc(rows, threeAccountsRow.MatchString)
			if heading < 0 || first < 0 || first+tt.apart >= len(rows) {
				t.Fatalf("at %d, given way %d steps, USAGE's block reads\n%s\nwant its windows' headings and its accounts", width, tt.given, read)
			}
			if got := strings.TrimSpace(rows[heading+1]) == ""; got != tt.blankUnder {
				t.Errorf("at %d, given way %d steps, a blank row under its first window's heading is %t, want %t:\n%s", width, tt.given, got, tt.blankUnder, read)
			}
			if !threeAccountsRow.MatchString(rows[first+tt.apart]) || tt.apart > 1 && strings.TrimSpace(rows[first+1]) != "" {
				t.Errorf("at %d, given way %d steps, its accounts aren't %d rows apart:\n%s", width, tt.given, tt.apart, read)
			}
			if got := strings.Contains(read, "even pace"); got != tt.key {
				t.Errorf("at %d, given way %d steps, its key is drawn %t, want %t:\n%s", width, tt.given, got, tt.key, read)
			}
		}
	}
}

// At every height from 10 to 60, at the phone's widths, the iPad's, the
// landscape phone's and the desktops', with the sections shown at first and
// with every one, USAGE gives way in its steps' order: its blank rows
// between accounts, with a section open, only where it doesn't fit with
// them, and never further; every other section folded, at the step that
// shows the most accounts in its rows, the least far of those that tie, but
// for its key, kept only where it shows; and the accounts it shows never fall
// as the rows grow.
func TestUSAGEGivesWayInItsStepsOrder(t *testing.T) {
	for _, shown := range []Sections{{}, every} {
		for _, width := range []int{52, 73, 108, 138, 160, 216} {
			t.Run(fmt.Sprintf("%v at %d", shown.Names(), width), func(t *testing.T) {
				t.Parallel()
				fewest := 0
				for height := 10; height <= 60; height++ {
					f := frameOf(width, height)
					f.Sections, f.Sessions = shown, flippingSessions()
					units, room := f.allotted(threeRouted(), now), f.viewRows()
					i := slices.IndexFunc(units, func(u unit) bool { return u.ledBy(SectionUsage) })
					usage := units[i]
					s := usage.sections[0]
					folded := !slices.ContainsFunc(units, func(u unit) bool { return !u.fold && !u.ledBy(SectionUsage) })
					back := slices.Clone(units)
					for j := range back {
						back[j].rows = back[j].least
					}
					if usage.lean > givenApart {
						back[i].rows = s.leaner[usage.lean-2]
					}
					open := room - (rowsDown(back) - back[i].rows)
					switch {
					case !folded && usage.lean > givenNone && open >= back[i].rows:
						t.Errorf("at %dx%d, USAGE has given way %d steps, but fits a step back", width, height, usage.lean)
					case !folded && usage.lean > givenApart:
						t.Errorf("at %dx%d, USAGE has given way %d steps, its headings' blank rows among them, with a section open", width, height, usage.lean)
					}
					if folded {
						shows := s.accounts(usage.lean, open)
						for given := givenNone; int(given) <= len(s.leaner); given++ {
							if other := s.accounts(given, open); other > shows || other == shows && given < usage.lean && !keyHidden(s, given, open) {
								t.Errorf("at %dx%d, USAGE has given way %d steps, showing %d accounts in %d rows, where %d steps show %d", width, height, usage.lean, shows, open, given, other)
							}
						}
						if keyHidden(s, usage.lean, open) {
							t.Errorf("at %dx%d, USAGE keeps its key in %d rows, out of view under the rows it takes", width, height, open)
						}
					}
					accounts := s.accounts(usage.lean, usage.rows)
					if accounts < fewest {
						t.Errorf("at %dx%d, USAGE shows %d accounts, fewer than the %d a row shorter", width, height, accounts, fewest)
					}
					fewest = accounts
				}
			})
		}
	}
}

// keyHidden reports whether USAGE's section s, at the step given in rows
// rows, keeps its key out of view: the step keeps it, and takes more rows
// whole than it's given.
func keyHidden(s section, given giving, rows int) bool {
	return given < givenKey && s.rowsAt(given) > rows
}

// Squeezed, USAGE keeps its key only where it shows: of the steps that show
// the most accounts, it takes the least far whose key isn't left out of view
// under its rows, the step that gives the key way where none keeps it in
// view, whichever step would have won the tie.
func TestUSAGEKeepsItsKeyOnlyWhereItShows(t *testing.T) {
	// The steps' rows, as a matrix of three accounts and the pool has them:
	// as it is, 14; its accounts a row apart, 12; its headings' blank rows
	// gone, 10; its key gone, 8. As it is, it shows one account under 13
	// rows; its accounts a row apart show two in 9 or 10 rows, three from 11.
	usage := unit{sections: []section{{Section: SectionUsage, least: 14, leaner: []int{12, 10, 8}, accounts: func(given giving, rows int) int {
		switch {
		case given == givenNone && rows < 13:
			return 1
		case given == givenApart && rows < 11:
			return 2
		}
		return 3
	}}}}
	tests := []struct {
		name string
		rows int
		want giving
	}{
		{name: "every step keeping the key leaves it out of view: given way", rows: 9, want: givenKey},
		{name: "its headings' blank rows given way, the key showing: kept", rows: 10, want: givenHeads},
		{name: "a row apart, the key out of view: the step that shows it", rows: 11, want: givenHeads},
		{name: "a row apart, the key showing", rows: 12, want: givenApart},
		{name: "as it is, the key out of view: the least far that shows it", rows: 13, want: givenApart},
		{name: "as it is, the key showing", rows: 14, want: givenNone},
	}
	for _, tt := range tests {
		if got := usage.showsMost(tt.rows); got != tt.want {
			t.Errorf("%s: in %d rows, USAGE takes step %d, want %d", tt.name, tt.rows, got, tt.want)
		}
	}
}

// At 15 rows, USAGE's blank rows and its key give way, so its accounts show:
// side by side at 138 columns, its windows' headings straight under USAGE's,
// and an account a row under them.
func TestAt15RowsUSAGEShowsItsAccounts(t *testing.T) {
	rows := textOf(frameOf(138, 15).Draw(threeRouted(), now))
	at := slices.IndexFunc(rows, func(r string) bool { return strings.HasPrefix(r, "USAGE ") })
	if at < 0 || at+4 >= len(rows) {
		t.Fatalf("at 138x15, the Overview reads\n%s\nwant USAGE's heading", strings.Join(rows, "\n"))
	}
	if got, want := strings.Fields(rows[at+1]), []string{"WEEK", "5-HOUR"}; !slices.Equal(got, want) {
		t.Errorf("at 138x15, the row under USAGE's heading reads %q, want its windows' headings %q:\n%s", rows[at+1], want, strings.Join(rows, "\n"))
	}
	for i, want := range []string{"1 work", "2 personal", "3 side"} {
		if !strings.HasPrefix(rows[at+2+i], want) {
			t.Errorf("at 138x15, row %d under USAGE's heading reads %q, want it to start %q:\n%s", i+2, rows[at+2+i], want, strings.Join(rows, "\n"))
		}
	}
	if slices.ContainsFunc(rows, func(r string) bool { return strings.Contains(r, "even pace") }) {
		t.Errorf("at 138x15, the Overview reads\n%s\nwant USAGE's key given way", strings.Join(rows, "\n"))
	}
	if got, want := strings.TrimSpace(rows[at+5]), "▼ 2 more below"; got != want {
		t.Errorf("at 138x15, the row under USAGE's accounts reads %q, want %q, the pool's rule and row alone out of view:\n%s", got, want, strings.Join(rows, "\n"))
	}
}
