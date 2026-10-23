package capture

import (
	"cmp"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// ownersSizes are the terminals the fluid sweep draws every view at, as the
// owner uses them: a phone upright, tall and short, and on its side, tall and
// short; an iPad, tall and short; a desktop; and a MacBook's side screen.
var ownersSizes = []watch.Size{
	{Width: 73, Height: 61}, {Width: 73, Height: 36}, {Width: 138, Height: 30}, {Width: 138, Height: 15},
	{Width: 108, Height: 44}, {Width: 108, Height: 24}, {Width: 173, Height: 53}, {Width: 109, Height: 53},
}

// longNamesViews are the world of long names on each view no fixture draws
// it on, so a view's cut name shows at the owner's sizes: the Overview with
// every section shown, Compare, Detail of work, History's four views,
// Runway, the Log's events, and d28c's page.
var longNamesViews = []page{
	{name: "long-names-overview", world: accountWorlds.longNames, keys: keysOf("g", "s")},
	{name: "long-names-accounts-compare", world: accountWorlds.longNames, keys: toAccounts()},
	{name: "long-names-accounts-detail", world: accountWorlds.longNames, keys: toDetail(downKey)},
	{name: "long-names-history-year", world: accountWorlds.longNames, keys: toHistory()},
	{name: "long-names-history-weeks", world: accountWorlds.longNames, keys: toHistory("]")},
	{name: "long-names-history-days", world: accountWorlds.longNames, keys: toHistory("]", "]")},
	{name: "long-names-history-tokens", world: accountWorlds.longNames, keys: toHistory("]", "]", "]")},
	{name: "long-names-runway", world: accountWorlds.longNames, keys: toRunway()},
	{name: "long-names-log-events", world: accountWorlds.longNames, keys: toLog("]")},
	{name: "long-names-session", world: accountWorlds.longNames, keys: toSession("d28c"), scale: storyScale},
}

// fluidRule is a rule of the fluid layout a view breaks, as the sweep finds
// it.
type fluidRule string

const (
	// cutWithRoom is a known name cut, as "work-tea…", on a row with more
	// blank cells at its end than the cut took.
	cutWithRoom fluidRule = "a cut name with room"
	// foldWithRowsSpare is a part out of view, folded or scrolled, or with no
	// room at the frame's height, while foldSpare rows or more are blank at
	// the view's foot.
	foldWithRowsSpare fluidRule = "a fold with rows spare"
	// nothingDrawn is a view with leastView rows or more drawing none of
	// what it's for: Runway a lane, a session's page a turn, the Overview
	// an account.
	nothingDrawn fluidRule = "nothing drawn"
	// wastedThird is a view whose content, leaving out rules, reaches less
	// than two thirds of the width.
	wastedThird fluidRule = "a wasted third"
)

// foldSpare is how many blank rows at a view's foot make a fold one with
// rows spare.
const foldSpare = 3

// leastView is how many rows of view, its heading's to the key line's, a
// view draws what it's for in.
const leastView = 6

// natural are the views whose parts are all at their natural widths, so
// short of two thirds of the width is no waste, each by name, as viewOf
// names it, with why: a view added here with its reason is the rule
// working, not a loosening. The cards are never judged: the sweep closes
// what's open over the page.
var natural = map[string]string{}

// known is what the design draws that the sweep would take for a rule the
// views of the fixtures named break at the owner's sizes given, as "73x61":
// byDesign, what the design draws; for a cut with room, what it draws cut,
// which the sweep can't tell from a cut with room, and cuts, those cuts as
// drawn, as "switchboard…", the only cuts it passes over; for a fold with
// rows spare, what's left at the foot, short of every folded section's
// least.
type known struct {
	byDesign string
	cuts     []string
	rule     fluidRule
	sizes    []string
	fixtures []string
}

// drawnByDesign reports whether the break is one the design draws: cuts with
// room it names, or a fold with rows spare, with no cuts.
func (k known) drawnByDesign() bool {
	switch {
	case k.byDesign == "":
		return false
	case k.rule == cutWithRoom:
		return len(k.cuts) > 0
	}
	return k.rule == foldWithRowsSpare && len(k.cuts) == 0
}

// breaking is what the break passes over: its rule, or the design's cuts,
// each.
func (k known) breaking() []string {
	if len(k.cuts) == 0 {
		return []string{"the rule of " + string(k.rule)}
	}
	return k.cuts
}

// knownBreaks are what the design draws that the sweep would take for a
// break. A fixture named at a size where it no longer draws it fails the
// sweep, so the list holds only what's drawn.
var knownBreaks = []known{
	// The Switchboard's directories at their most, as the List's; and at 108
	// and 109, where the cells past the leasts go to the directory alone, the
	// directory at 14 and 15 and the account a call came from at its least,
	// as the design gives them at 108.
	{
		byDesign: "the Switchboard's directories at their most", cuts: dirCuts(views.SessionDirMost), rule: cutWithRoom,
		sizes:    []string{"73x61", "73x36", "138x30", "138x15", "173x53"},
		fixtures: []string{"sessions-switchboard-long-names-108x44"},
	},
	{
		byDesign: "the Switchboard's names past its leasts at 108", cuts: append(dirCuts(14), "work-…"), rule: cutWithRoom,
		sizes: []string{"108x44", "108x24"}, fixtures: []string{"sessions-switchboard-long-names-108x44"},
	},
	{
		byDesign: "the Switchboard's names past its leasts at 109", cuts: append(dirCuts(15), "work-…"), rule: cutWithRoom,
		sizes: []string{"109x53"}, fixtures: []string{"sessions-switchboard-long-names-108x44"},
	},
	{
		byDesign: "the account a call came from at its least", cuts: []string{"perso…"}, rule: cutWithRoom,
		sizes: []string{"108x24", "108x44", "109x53"}, fixtures: []string{"sessions-moved-160", "sessions-pinned-160"},
	},
	// The Log's sessions' directories, as views.SessionDirName cuts them, with
	// room at every size they show at with room; and the Requests' on, its
	// share past the columns' leasts at 73, 108 and 109, as the design gives
	// it.
	{
		byDesign: "the Log's directories at their most", cuts: logDirCuts(), rule: cutWithRoom,
		sizes:    []string{"138x30", "138x15", "108x44", "108x24", "173x53", "109x53"},
		fixtures: []string{"log-requests-long-names-108x44"},
	},
	{
		byDesign: "the Log's directories at their most", cuts: logDirCuts(), rule: cutWithRoom,
		sizes: []string{"138x30", "138x15", "173x53"}, fixtures: []string{"long-names-log-events"},
	},
	{
		byDesign: "on's share at 73", cuts: []string{"personal-res…"}, rule: cutWithRoom,
		sizes: []string{"73x61", "73x36"}, fixtures: []string{"log-requests-long-names-108x44"},
	},
	{
		byDesign: "on's share at 108", cuts: []string{"personal-rese…", "side-hustle-c…"}, rule: cutWithRoom,
		sizes: []string{"108x44", "108x24"}, fixtures: []string{"log-requests-long-names-108x44"},
	},
	{
		byDesign: "on's share at 109", cuts: []string{"personal-resea…", "side-hustle-co…"}, rule: cutWithRoom,
		sizes: []string{"109x53"}, fixtures: []string{"log-requests-long-names-108x44"},
	},
	// Detail's list, which gives way first, its names cut to what the panel
	// leaves them, down to 14 cells, before its weeks narrow (the owner's call,
	// 15 October 2026); and its names at their most, 24 cells.
	{
		byDesign: "Detail's list giving way at 138", cuts: []string{"work-team-p…"}, rule: cutWithRoom,
		sizes: []string{"138x30", "138x15"}, fixtures: []string{"long-names-accounts-detail"},
	},
	{
		byDesign: "Detail's list giving way at 108", cuts: []string{"work-team-pl…"}, rule: cutWithRoom,
		sizes: []string{"108x44", "108x24"}, fixtures: []string{"long-names-accounts-detail"},
	},
	{
		byDesign: "Detail's list's names at their most", cuts: []string{"side-hustle-consult…"}, rule: cutWithRoom,
		sizes: []string{"173x53"}, fixtures: []string{"long-names-accounts-detail"},
	},
	// The List's directories at their most, where the session's column is
	// whole and the rows end blank.
	{
		byDesign: "the List's directories at their most", cuts: dirCuts(views.SessionDirMost), rule: cutWithRoom,
		sizes: []string{"138x30", "138x15", "173x53"}, fixtures: []string{"sessions-list-long-names-108x44"},
	},
	// What's left at the foot where USAGE takes the rows it must, the foot's
	// blank row among them, short of every folded section's least: of six
	// accounts at 73x61, stacked, ROUTING open under it; by account at 138x30,
	// 2 rows, where ROUTING unfolded takes 6 more and COMING UP and LATELY 5,
	// a blank row before each among them, and the blank rows between its
	// accounts give way only 2; and of ten accounts side by side, their blank
	// rows given way, 3 rows: at 138x30, where ROUTING takes 13 more and COMING
	// UP and LATELY 5, its headings' blank rows and its key giving way only
	// where no section is open; and at 108x44, ROUTING open, where COMING UP
	// and LATELY take 5.
	{
		byDesign: "2 rows under ROUTING, short of every folded section's least", rule: foldWithRowsSpare,
		sizes: []string{"73x61"}, fixtures: []string{"overview-six-running-173x53"},
	},
	{
		byDesign: "2 rows under USAGE by account, short of every folded section's least", rule: foldWithRowsSpare,
		sizes: []string{"138x30"}, fixtures: []string{"overview-by-account-160"},
	},
	{
		byDesign: "3 rows under USAGE's ten accounts side by side, short of every folded section's least", rule: foldWithRowsSpare,
		sizes: []string{"138x30", "108x44"}, fixtures: []string{"overview-ten-accounts-160"},
	},
	// What's left at the foot at 73x36, every other section folded, where
	// USAGE takes the least far step that fits whole, as it keeps its key
	// only where it shows: three accounts stacked, their blank rows given
	// way, 3 rows, where they take 4 more and LATELY 5, and GRAPHS alone
	// shown, more; three by account, of three windows, their key given way,
	// 2 rows, where the key takes 3 more and LATELY 5; six stacked, their
	// headings' blank rows given way too, 2 rows, where those take 3 more and
	// LATELY 5.
	{
		byDesign: "the rows under USAGE whole, short of its step before and every folded section's least", rule: foldWithRowsSpare,
		sizes: []string{"73x36"}, fixtures: []string{
			"overview-everything-160x39", "overview-everything-216x106", "overview-burn-down-160x39", "overview-sessions-graphs-216",
			"overview-phone-burn-down-52", "long-names-overview", "overview-pool-220x50", "overview-by-account-burn-down-220x50",
			"overview-six-accounts-160",
		},
	},
}

// Every fixture's view, and the long-names world's, at the owner's sizes,
// with nothing open over its page: no known name cut with room at its row's
// end; no part out of view with rows blank at its foot; Runway, a session's
// page and the Overview drawing a lane, a turn and an account; and the view
// reaching two thirds of the width, but where it's natural. What
// knownBreaks names is passed over, and fails where it no longer breaks: a
// rule, or a cut the design draws.
func TestEveryViewIsFluidAtTheOwnersSizes(t *testing.T) {
	for _, f := range sweptFixtures(t) {
		t.Run(f.Name, func(t *testing.T) {
			t.Parallel()
			m := withNothingOver(t, f.settle(f.Size), f.Size)
			view := viewOf(t, m, f.Size)
			names := knownNames(f)
			for _, size := range ownersSizes {
				d, at := fluidAt(m, size), sizeName(size)
				designed := designedAt(knownBreaks, f.Name, at)
				unknown, cleared := judged(d.breaks(view, names, cutsOf(designed)), brokenAt(knownBreaks, f.Name, at))
				for _, b := range unknown {
					t.Errorf("%s at %s breaks the rule of %s, as %s:\n%s", view, at, b.rule, b.shown, strings.Join(d.rows, "\n"))
				}
				for _, rule := range cleared {
					t.Errorf("%s at %s no longer breaks the rule of %s: delete it from knownBreaks", view, at, rule)
				}
				for _, k := range undrawn(designed, d.cutsWithRoom(names)) {
					t.Errorf("%s at %s no longer draws %s, any of %q, with room: delete it from knownBreaks", view, at, k.byDesign, k.cuts)
				}
			}
		})
	}
}

// designedAt are the cuts the design draws that breaks has the fixture named
// cut with room at the size named.
func designedAt(breaks []known, name, at string) []known {
	var designed []known
	for _, k := range breaks {
		if len(k.cuts) > 0 && slices.Contains(k.fixtures, name) && slices.Contains(k.sizes, at) {
			designed = append(designed, k)
		}
	}
	return designed
}

// cutsOf are the cuts of each of the design's given.
func cutsOf(designed []known) []string {
	var cuts []string
	for _, k := range designed {
		cuts = append(cuts, k.cuts...)
	}
	return cuts
}

// undrawn are the design's cuts given none of whose cuts is of those drawn.
func undrawn(designed []known, drawn []fluidCut) []known {
	return slices.DeleteFunc(slices.Clone(designed), func(k known) bool {
		return slices.ContainsFunc(drawn, func(d fluidCut) bool { return slices.Contains(k.cuts, d.cut) })
	})
}

// logDirCuts are the long-names world's directories as the Log draws them,
// views.SessionDirName's cut of each.
func logDirCuts() []string {
	var cuts []string
	for _, dir := range longDirs {
		cuts = append(cuts, views.SessionDirName(dir))
	}
	slices.Sort(cuts)
	return cuts
}

// dirCuts are the long-names world's directories cut to cells, as
// status.DirName cuts them, those it cuts: at views.SessionDirMost, as the
// List and the Switchboard draw them at their most.
func dirCuts(cells int) []string {
	var cuts []string
	for _, dir := range longDirs {
		if cut := status.DirName(dir, cells); cut != path.Base(dir) {
			cuts = append(cuts, cut)
		}
	}
	slices.Sort(cuts)
	return cuts
}

// brokenAt are the rules known has the fixture named break at the size
// named, the design's folds.
func brokenAt(known []known, name, at string) []fluidRule {
	var rules []fluidRule
	for _, k := range known {
		if len(k.cuts) == 0 && slices.Contains(k.fixtures, name) && slices.Contains(k.sizes, at) {
			rules = append(rules, k.rule)
		}
	}
	return rules
}

// judged are the breaks given of none of the rules known, and the rules
// known that none of them is of.
func judged(breaks []fluidBreak, known []fluidRule) (unknown []fluidBreak, cleared []fluidRule) {
	for _, b := range breaks {
		if !slices.Contains(known, b.rule) {
			unknown = append(unknown, b)
		}
	}
	for _, rule := range known {
		if !slices.ContainsFunc(breaks, func(b fluidBreak) bool { return b.rule == rule }) {
			cleared = append(cleared, rule)
		}
	}
	return unknown, cleared
}

// Each of knownBreaks names what the design draws, its cuts, as a cut with
// room, or its fold, and fixtures the sweep draws at the owner's sizes, each
// fixture and size once a rule, or a cut.
func TestEveryKnownBreakIsOfAViewTheSweepDraws(t *testing.T) {
	swept := make(map[string]bool)
	for _, f := range sweptFixtures(t) {
		swept[f.Name] = true
	}
	sizes := make([]string, len(ownersSizes))
	for i, s := range ownersSizes {
		sizes[i] = sizeName(s)
	}
	named := make(map[string]bool)
	for _, k := range knownBreaks {
		if !k.drawnByDesign() {
			t.Errorf("knownBreaks names %q, the cuts %q, as %s: want the design's cuts, or its fold", k.byDesign, k.cuts, k.rule)
		}
		for _, name := range k.fixtures {
			if !swept[name] {
				t.Errorf("%q names %s, which the sweep doesn't draw", k.byDesign, name)
			}
			for _, at := range k.sizes {
				if !slices.Contains(sizes, at) {
					t.Errorf("%q names %s at %s, not one of the owner's sizes", k.byDesign, name, at)
				}
				for _, what := range k.breaking() {
					if once := name + " " + what + " " + at; named[once] {
						t.Errorf("knownBreaks name %s at %s breaking %s twice", name, at, what)
					} else {
						named[once] = true
					}
				}
			}
		}
	}
}

// sizeName names a terminal's size, as "73x61".
func sizeName(s watch.Size) string {
	return fmt.Sprintf("%dx%d", s.Width, s.Height)
}

// sweptFixtures are every fixture the watch draws, but usage's printouts,
// which have no view, then longNamesViews, each settled at 160 by 37, as most
// frames are, before the sweep sizes it.
func sweptFixtures(t *testing.T) []Fixture {
	t.Helper()
	var swept []Fixture
	for _, name := range Names() {
		if f := fixture(t, name); !f.Printout {
			swept = append(swept, f)
		}
	}
	for _, p := range longNamesViews {
		p.size = wide(37)
		swept = append(swept, p.fixture(worldsNow()))
	}
	return swept
}

// knownNames are the names the fixture's world gives that a view may cut:
// its accounts' labels, else their ids, and the directories its sessions run
// in, and its ledger's lines.
func knownNames(f Fixture) []string {
	names := accountNames(f)
	for _, s := range f.router.sessions {
		for _, a := range s.Assignments {
			names = append(names, path.Base(a.Dir))
		}
	}
	for _, l := range f.records.lines {
		names = append(names, path.Base(l.Dir))
	}
	names = slices.DeleteFunc(names, func(n string) bool { return n == "." || n == "" })
	slices.Sort(names)
	return slices.Compact(names)
}

// fluidFrame is a frame as the sweep reads it: its size, its rows as plain
// text, and how many rows its chrome takes over the view's heading.
type fluidFrame struct {
	size   watch.Size
	rows   []string
	chrome int
}

// fluidAt is m drawn at size.
func fluidAt(m watch.Model, size watch.Size) fluidFrame {
	return fluidFrame{size: size, rows: plainRows(m, size.Width, size.Height)[:size.Height], chrome: chromeOf(size.Width)}
}

// view are the rows from the view's heading to the one over the key line.
func (d fluidFrame) view() []string {
	return d.rows[d.chrome+1 : d.size.Height-1]
}

// body are the view's rows under its heading.
func (d fluidFrame) body() []string {
	return d.view()[1:]
}

// fluidBreak is a rule a view breaks, and what shows it does.
type fluidBreak struct {
	rule  fluidRule
	shown string
}

// breaks are the rules the view named breaks as drawn, of the names given,
// but for the cuts excused: one still loading draws nothing and so wastes no
// width.
func (d fluidFrame) breaks(view string, names, excused []string) []fluidBreak {
	var breaks []fluidBreak
	cuts := slices.DeleteFunc(d.cutsWithRoom(names), func(c fluidCut) bool { return slices.Contains(excused, c.cut) })
	if len(cuts) > 0 {
		breaks = append(breaks, fluidBreak{cutWithRoom, cuts[0].shown})
	}
	if out := d.outOfView(); out != "" && d.footBlank() >= foldSpare {
		breaks = append(breaks, fluidBreak{foldWithRowsSpare, fmt.Sprintf("%q over %d blank rows", out, d.footBlank())})
	}
	if d.loading() {
		return breaks
	}
	if len(d.view()) >= leastView && !d.drawsWhatItsFor(view, names) {
		breaks = append(breaks, fluidBreak{nothingDrawn, fmt.Sprintf("none in %d rows of view", len(d.view()))})
	}
	if _, ok := natural[view]; !ok && d.reach()*3 < d.size.Width*2 {
		breaks = append(breaks, fluidBreak{wastedThird, fmt.Sprintf("its content reaches %d of %d columns", d.reach(), d.size.Width)})
	}
	return breaks
}

// loading reports whether the view's heading says it's still reading what
// it's for, so it has none to draw.
func (d fluidFrame) loading() bool {
	return strings.Contains(d.view()[0], "loading…")
}

// fluidCut is a name cut short as drawn, as "work-tea…", and what shows it
// had room.
type fluidCut struct {
	cut   string
	shown string
}

// cutsWithRoom are where the rows of the view cut one of the names given,
// with more blank cells at its row's end than the cut took, of the names a
// cut may be of, the longest's. An ellipsis after anything but a name's
// start, as a running turn's "3m…", or words cut, is no name cut.
func (d fluidFrame) cutsWithRoom(names []string) []fluidCut {
	var cuts []fluidCut
	for _, row := range d.view() {
		spare := d.size.Width - ansi.StringWidth(strings.TrimRight(row, " "))
		for _, cut := range cutsIn(row) {
			took := 0
			for _, name := range names {
				if len(name) > len(cut) && strings.HasPrefix(name, cut) {
					took = max(took, utf8.RuneCountInString(name)-utf8.RuneCountInString(cut))
				}
			}
			if took > 0 && spare > took {
				cuts = append(cuts, fluidCut{cut + "…", fmt.Sprintf("%q, %d cells cut, with %d blank at its row's end", cut+"…", took, spare)})
			}
		}
	}
	return cuts
}

// cutText is what's left of a word cut short, as a name is: what runs
// unbroken into the ellipsis, two cells or more, as one alone can't be told
// from a word of prose cut short: "its p…" is its pace.
var cutText = regexp.MustCompile(`([\p{L}\p{N}._-]{2,})…`)

// cutsIn are what's left of each word the row cuts short.
func cutsIn(row string) []string {
	var cuts []string
	for _, m := range cutText.FindAllStringSubmatch(row, -1) {
		cuts = append(cuts, m[1])
	}
	return cuts
}

// footBlank is how many rows are blank at the view's foot, over the key
// line.
func (d fluidFrame) footBlank() int {
	view := d.view()
	n := 0
	for n < len(view) && strings.TrimSpace(view[len(view)-1-n]) == "" {
		n++
	}
	return n
}

var (
	// noRoom is a part with no room at the frame's height.
	noRoom = regexp.MustCompile(`no room at [0-9]+ rows`)
	// foldedHeading is a section's heading folded, holding what it lists on
	// its own row: its first line, then an ellipsis for the rest, two cells
	// on, or cut short where its rule and key follow, as "COMING UP   16:30
	// personal's 5-hour resets  …" or "… ── c". Words cut, or a week still
	// running, as "34%…", end in an ellipsis of their own.
	foldedHeading = regexp.MustCompile(`^[A-Z][A-Z0-9-]*[A-Z]( [A-Z0-9-]+)* {2,}.*(  …( ─+ \S)?|… ─+ \S)( {2,}|$)`)
)

// outOfView is the first row of the view's body saying a part of it is out
// of view, trimmed: with no room at the frame's height, a section folded to
// its heading, or how many more are above or below; "" where none does.
func (d fluidFrame) outOfView() string {
	for _, row := range d.body() {
		if row = strings.TrimSpace(row); noRoom.MatchString(row) || foldedHeading.MatchString(row) || outLine.MatchString(row) {
			return row
		}
	}
	return ""
}

var (
	// laneShown is a row of Runway's lanes: an account's number, and its bar.
	laneShown = regexp.MustCompile(`^ ?[0-9]+ .*[█─]`)
	// turnShown is a row of a session's turns: its number and when it started.
	turnShown = regexp.MustCompile(`^ *[0-9]+ +[0-9]{1,2}:[0-9]{2} `)
	// numbered is a row leading with an account's number, then its name.
	numbered = regexp.MustCompile(`^ *[0-9]+ (.*)$`)
)

// drawsWhatItsFor reports whether the view named draws what it's for, of the
// names given: Runway a lane, a session's page a turn, and the Overview an
// account; any other view, whatever it draws.
func (d fluidFrame) drawsWhatItsFor(view string, names []string) bool {
	switch view {
	case "Runway":
		return slices.ContainsFunc(d.body(), laneShown.MatchString)
	case sessionPage:
		return slices.ContainsFunc(d.body(), turnShown.MatchString)
	case "Overview":
		return slices.ContainsFunc(d.body(), func(row string) bool {
			m := numbered.FindStringSubmatch(row)
			return m != nil && slices.ContainsFunc(names, func(name string) bool { return leadsWith(m[1], name) })
		})
	}
	return true
}

// ruleGlyphs are the glyphs a rule is drawn in, across a view or down it,
// and where rules meet or a tick marks one.
const ruleGlyphs = "─━┄┅┈┉╌╍═│┃┆┊╎┬┴┼├┤╵╷╴╶"

// ruledKey is a section's key at the end of its heading's rule, as "── u".
var ruledKey = regexp.MustCompile(`─ \S( |$)`)

// reach is how far across the view's body its content runs, leaving out
// rules and the keys at their ends: the cell after the last it draws on.
func (d fluidFrame) reach() int {
	most := 0
	for _, row := range d.body() {
		cells := []rune(row)
		for _, at := range ruledKey.FindAllStringIndex(row, -1) {
			cells[utf8.RuneCountInString(row[:at[0]])+2] = ' '
		}
		end := len(cells)
		for end > 0 && (cells[end-1] == ' ' || strings.ContainsRune(ruleGlyphs, cells[end-1])) {
			end--
		}
		most = max(most, end)
	}
	return most
}

// tabs are the dashboard's tabs, each with its views where it has more than
// one, as its heading names them.
var tabs = map[string][]string{
	"Overview": nil,
	"Accounts": {"Compare", "Detail"},
	"History":  {"Year", "Weeks", "Days", "Tokens"},
	"Sessions": {"Switchboard", "List"},
	"Runway":   nil,
	"Log":      {"Requests", "Events"},
}

// sessionPage is what viewOf names a session's page.
const sessionPage = "a session's page"

// viewOf names the view m draws at size: a session's page, by its heading;
// else the tab shown, bold in the title, and of a tab with more than one
// view, the one bold in its heading, as "History Days".
func viewOf(t *testing.T, m watch.Model, size watch.Size) string {
	t.Helper()
	g, err := parse(deliver(m, tea.WindowSizeMsg{Width: size.Width, Height: size.Height}).View().Content)
	if err != nil {
		t.Fatal(err)
	}
	heading := chromeOf(size.Width) + 1
	if strings.HasPrefix(g.text()[heading], "SESSION ") {
		return sessionPage
	}
	tab := cmp.Or(boldOf(g[0], tabNames()), boldOf(g[1], tabNames()))
	if tab == "" {
		t.Fatalf("no tab is bold in the title:\n%s", strings.Join(g.text(), "\n"))
	}
	if views := tabs[tab]; views != nil {
		return tab + " " + boldOf(g[heading], views)
	}
	return tab
}

// tabNames are the tabs' names.
func tabNames() []string {
	names := make([]string, 0, len(tabs))
	for name := range tabs {
		names = append(names, name)
	}
	return names
}

// boldOf is the one of the words given drawn bold on the row, "" where none
// is.
func boldOf(row []cell, words []string) string {
	var bold strings.Builder
	for _, c := range row {
		if c.bold {
			bold.WriteRune(c.glyph)
		} else {
			bold.WriteRune(' ')
		}
	}
	for word := range strings.FieldsSeq(bold.String()) {
		if slices.Contains(words, word) {
			return word
		}
	}
	return ""
}

func TestTheFluidSweepFindsWhatItSweepsFor(t *testing.T) {
	names := []string{"personal", "side", "work", "work-team-plan"}
	across := strings.Repeat("■", 120)
	for _, tt := range []struct {
		name    string
		view    string
		rows    []string
		excused []string
		want    []fluidRule
	}{
		{name: "a view across the width", view: "Log Requests", rows: []string{"LOG", "", across}},
		{
			name: "a name cut with more blank cells than the cut took", view: "Log Requests",
			rows: []string{"LOG", "", across, "14:40  d28c  work-tea…"}, want: []fluidRule{cutWithRoom},
		},
		{
			name: "a name cut with room as the design draws it", view: "Log Requests",
			rows: []string{"LOG", "", across, "14:40  d28c  work-tea…"}, excused: []string{"work-tea…"},
		},
		{
			name: "a name cut with room beside one the design draws", view: "Log Requests",
			rows: []string{"LOG", "", across, "14:40  d28c  work-tea…  perso…"}, excused: []string{"work-tea…"}, want: []fluidRule{cutWithRoom},
		},
		{
			name: "a name cut with room, where the design draws another", view: "Log Requests",
			rows: []string{"LOG", "", across, "14:40  d28c  work-te…"}, excused: []string{"work-tea…"}, want: []fluidRule{cutWithRoom},
		},
		{
			name: "a name cut with no more blank cells than the cut took", view: "Log Requests",
			rows: []string{"LOG", "", across, "work-tea…" + strings.Repeat(" ", 104) + "x     "},
		},
		{name: "a word cut to a letter", view: "Log Requests", rows: []string{"LOG", "", across, "runs out at its p…"}},
		{name: "a turn still running", view: "Log Requests", rows: []string{"LOG", "", across, "13 14:40   3m…   6 req"}},
		{
			name: "no room at the frame's height over blank rows", view: "Log Requests",
			rows: []string{"LOG", across, "GRAPHS   work   no room at 13 rows ── g", "", "", ""}, want: []fluidRule{foldWithRowsSpare},
		},
		{
			name: "no room at the frame's height over two blank rows", view: "Log Requests",
			rows: []string{"LOG", across, "GRAPHS   work   no room at 13 rows ── g", "", ""},
		},
		{
			name: "a section folded to its heading over blank rows", view: "Log Requests",
			rows: []string{"LOG", across, "COMING UP   16:30  personal's 5-hour resets  …", "", "", ""}, want: []fluidRule{foldWithRowsSpare},
		},
		{
			name: "a section folded and ruled over blank rows", view: "Log Requests",
			rows: []string{"LOG", across, "COMING UP   16:30  personal's 5-hour resets… ── c", "", "", ""}, want: []fluidRule{foldWithRowsSpare},
		},
		{
			name: "more below over blank rows", view: "Log Requests",
			rows: []string{"LOG", across, "▼ 5 more below · j/k", "", "", ""}, want: []fluidRule{foldWithRowsSpare},
		},
		{name: "a week still running over blank rows", view: "Log Requests", rows: []string{"LOG", across, "WEEKS       74%   73%   59%…", "", "", ""}},
		{name: "Runway with a lane", view: "Runway", rows: []string{"RUNWAY", "", "1 work        ─███" + across[18:]}},
		{name: "Runway without a lane", view: "Runway", rows: []string{"RUNWAY", "", across, "", "", ""}, want: []fluidRule{nothingDrawn}},
		{name: "Runway without a lane in five rows of view", view: "Runway", rows: []string{"RUNWAY", "", across, "", ""}},
		{name: "a session's page with a turn", view: sessionPage, rows: []string{"SESSION  d28c", across, "13 14:40   3m…   6 req"}},
		{name: "a session's page without a turn", view: sessionPage, rows: []string{"SESSION  d28c", across, "", "", "", ""}, want: []fluidRule{nothingDrawn}},
		{
			name: "a session's page loading", view: sessionPage,
			rows: []string{"SESSION  d28c  switchboard  loading…", "", "", "", "", ""},
		},
		{name: "the Overview with an account", view: "Overview", rows: []string{"USAGE", across, " 2 personal ◆   ┃   66%"}},
		{name: "the Overview with an account cut", view: "Overview", rows: []string{"USAGE", " 1 work-team-…" + strings.Repeat(" ", 103) + "34%"}},
		{name: "the Overview without an account", view: "Overview", rows: []string{"USAGE", across, "WEEK", "", "", ""}, want: []fluidRule{nothingDrawn}},
		{
			name: "content short of two thirds, but for a rule and its key", view: "Log Requests",
			rows: []string{"LOG", strings.Repeat("■", 79), "ROUTING   auto " + strings.Repeat("─", 103) + " r", strings.Repeat("┄", 120)},
			want: []fluidRule{wastedThird},
		},
		{name: "content at two thirds", view: "Log Requests", rows: []string{"LOG", strings.Repeat("■", 80)}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			var got []fluidRule
			for _, b := range sweptView(120, tt.rows...).breaks(tt.view, names, tt.excused) {
				got = append(got, b.rule)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("breaks %q, want %q", got, tt.want)
			}
		})
	}
}

// sweptView is a frame width cells wide of the view's rows given, its heading
// first, under the chrome and over a key line.
func sweptView(width int, view ...string) fluidFrame {
	chrome := chromeOf(width)
	rows := slices.Concat(make([]string, chrome+1), view, []string{"tab views   q quit"})
	return fluidFrame{size: watch.Size{Width: width, Height: len(rows)}, rows: rows, chrome: chrome}
}

func TestAKnownBreakPassesWhereItBreaksAndFailsWhereItsCleared(t *testing.T) {
	known := []known{
		{byDesign: "3 rows, short of LATELY's least", rule: foldWithRowsSpare, sizes: []string{"138x15"}, fixtures: []string{"runway-week-160", "runway-phone-52"}},
		{byDesign: "4 rows, short of ROUTING's least", rule: foldWithRowsSpare, sizes: []string{"138x30"}, fixtures: []string{"runway-week-160"}},
		{byDesign: "work at its most", cuts: []string{"work-tea…"}, rule: cutWithRoom, sizes: []string{"138x30"}, fixtures: []string{"runway-week-160"}},
		{byDesign: "2 rows, short of GRAPHS' least", rule: foldWithRowsSpare, sizes: []string{"73x61"}, fixtures: []string{"runway-phone-52"}},
	}
	cut, fold, none := fluidBreak{rule: cutWithRoom}, fluidBreak{rule: foldWithRowsSpare}, fluidBreak{rule: nothingDrawn}
	for _, tt := range []struct {
		name, fixture, at string
		breaks            []fluidBreak
		unknown           []fluidBreak
		cleared           []fluidRule
	}{
		{name: "breaking what's known", fixture: "runway-phone-52", at: "138x15", breaks: []fluidBreak{fold}},
		{name: "breaking what's known and more", fixture: "runway-week-160", at: "138x30", breaks: []fluidBreak{cut, fold}, unknown: []fluidBreak{cut}},
		{name: "breaking what's known elsewhere", fixture: "runway-phone-52", at: "138x30", breaks: []fluidBreak{fold}, unknown: []fluidBreak{fold}},
		{name: "breaking another rule than what's known", fixture: "runway-phone-52", at: "138x15", breaks: []fluidBreak{none}, unknown: []fluidBreak{none}, cleared: []fluidRule{foldWithRowsSpare}},
		{name: "no longer breaking what's known", fixture: "runway-week-160", at: "138x15", cleared: []fluidRule{foldWithRowsSpare}},
		{name: "breaking nothing, with nothing known", fixture: "runway-week-160", at: "73x61"},
		{name: "folding as the design leaves it", fixture: "runway-phone-52", at: "73x61", breaks: []fluidBreak{fold}},
		{name: "no longer folding as the design leaves it", fixture: "runway-phone-52", at: "73x61", cleared: []fluidRule{foldWithRowsSpare}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			unknown, cleared := judged(tt.breaks, brokenAt(known, tt.fixture, tt.at))
			if !slices.Equal(unknown, tt.unknown) || !slices.Equal(cleared, tt.cleared) {
				t.Errorf("judged %v unknown and %q cleared, want %v and %q", unknown, cleared, tt.unknown, tt.cleared)
			}
		})
	}
}

// The design's cuts are passed over at the sizes it names them at alone,
// and fail where none of them is drawn with room any longer.
func TestADesignedCutPassesWhereItsDrawnAndFailsWhereItsNot(t *testing.T) {
	designed := known{byDesign: "names at their most", cuts: []string{"work-tea…", "perso…"}, rule: cutWithRoom, sizes: []string{"138x30"}, fixtures: []string{"log-requests-160"}}
	known := []known{designed, {byDesign: "2 rows left", rule: foldWithRowsSpare, sizes: []string{"138x30"}, fixtures: []string{"log-requests-160"}}}
	drawn := func(cuts ...string) []fluidCut {
		var drawn []fluidCut
		for _, c := range cuts {
			drawn = append(drawn, fluidCut{cut: c})
		}
		return drawn
	}
	for _, tt := range []struct {
		name, at string
		drawn    []fluidCut
		excused  []string
		cleared  int
	}{
		{name: "both drawn", at: "138x30", drawn: drawn("perso…", "work-tea…"), excused: designed.cuts},
		{name: "one drawn", at: "138x30", drawn: drawn("work-tea…", "side…"), excused: designed.cuts},
		{name: "neither drawn", at: "138x30", drawn: drawn("side…"), excused: designed.cuts, cleared: 1},
		{name: "at a size it names none", at: "73x61", drawn: drawn("work-tea…")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			at := designedAt(known, "log-requests-160", tt.at)
			if got, cleared := cutsOf(at), undrawn(at, tt.drawn); !slices.Equal(got, tt.excused) || len(cleared) != tt.cleared {
				t.Errorf("excused %q and %d cleared, want %q and %d", got, len(cleared), tt.excused, tt.cleared)
			}
		})
	}
}

// A known break is the design's: cuts with room, or a fold with rows spare
// with no cuts, each saying what the design draws.
func TestAKnownBreakIsDrawnByDesign(t *testing.T) {
	cuts := []string{"work-tea…"}
	for _, tt := range []struct {
		name string
		k    known
		want bool
	}{
		{name: "the design's cuts", k: known{byDesign: "at its most", cuts: cuts, rule: cutWithRoom}, want: true},
		{name: "cuts, unsaid", k: known{cuts: cuts, rule: cutWithRoom}},
		{name: "the design's, without cuts", k: known{byDesign: "at its most", rule: cutWithRoom}},
		{name: "the design's, of another rule", k: known{byDesign: "at its most", cuts: cuts, rule: nothingDrawn}},
		{name: "the design's fold", k: known{byDesign: "2 rows left", rule: foldWithRowsSpare}, want: true},
		{name: "a fold, unsaid", k: known{rule: foldWithRowsSpare}},
		{name: "the design's fold, with cuts", k: known{byDesign: "2 rows left", cuts: cuts, rule: foldWithRowsSpare}},
		{name: "the design's, of another rule, without cuts", k: known{byDesign: "at its most", rule: nothingDrawn}},
		{name: "neither", k: known{rule: nothingDrawn}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.k.drawnByDesign(); got != tt.want {
				t.Errorf("drawnByDesign = %t, want %t", got, tt.want)
			}
		})
	}
}
