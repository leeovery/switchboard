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
// names it, with why. As the plan has it, the slice that makes a view so
// adds it here, with its reason: that's the rule working, not a loosening.
// The cards are never judged: the sweep closes what's open over the page.
var natural = map[string]string{}

// known is a rule the views of the fixtures named are known to break at the
// owner's sizes given, as "73x61", and the slice of the fluid pass, as the
// plan numbers them, that clears it.
type known struct {
	slice    string
	rule     fluidRule
	sizes    []string
	fixtures []string
}

// knownBreaks are what the views break today, by the slice that clears
// them. The list only shrinks: a slice deletes what it clears, as a fixture
// named at a size where it no longer breaks the rule fails the sweep.
var knownBreaks = []known{
	// S5, the Switchboard's calls (F18, F19): its directories, and an account's
	// name in a phone's lanes, cut with room; at 15 rows, an account out of view
	// over blank rows.
	{
		slice: "S5", rule: cutWithRoom, sizes: []string{"108x44", "108x24", "109x53"},
		fixtures: []string{
			"page-sessions-120x30", "page-sessions-chosen", "page-sessions-not-answering",
			"page-sessions-short", "sessions-108x44", "sessions-moved-160", "sessions-phone-52",
			"sessions-refused-160", "sessions-switchboard-160",
		},
	},
	{
		slice: "S5", rule: cutWithRoom, sizes: []string{"108x44", "109x53"},
		fixtures: []string{"sessions-pinned-160"},
	},
	{
		slice: "S5", rule: cutWithRoom, sizes: []string{"73x61", "73x36", "108x24"},
		fixtures: []string{"sessions-ten-accounts-160"},
	},
	{
		slice: "S5", rule: cutWithRoom, sizes: []string{"73x61", "73x36", "138x30", "138x15", "108x44", "108x24", "173x53", "109x53"},
		fixtures: []string{"sessions-switchboard-long-names-108x44"},
	},
	{
		slice: "S5", rule: foldWithRowsSpare, sizes: []string{"138x15"},
		fixtures: []string{"page-sessions-short"},
	},
	// S6, the Log on flex (F30): its sessions' directories and accounts cut with
	// room.
	{
		slice: "S6", rule: cutWithRoom, sizes: []string{"73x61", "73x36"},
		fixtures: []string{
			"log-event-details-160", "log-events-160", "log-events-filter-160",
		},
	},
	{
		slice: "S6", rule: cutWithRoom, sizes: []string{"73x61", "73x36", "138x30", "138x15", "108x44", "108x24", "173x53", "109x53"},
		fixtures: []string{
			"log-requests-long-names-108x44", "long-names-log-events",
		},
	},
	// S7, the List on flex (F21): its sessions' directories cut with room.
	{
		slice: "S7", rule: cutWithRoom, sizes: []string{"73x61", "73x36", "138x30", "138x15", "108x44", "108x24", "173x53", "109x53"},
		fixtures: []string{"sessions-list-long-names-108x44"},
	},
	// S10, the Overview's rows on flex (F1): sections folded with rows spare.
	{
		slice: "S10", rule: foldWithRowsSpare, sizes: []string{"73x61"},
		fixtures: []string{
			"overview-model-nearing-160", "overview-models-160",
		},
	},
	{
		slice: "S10", rule: foldWithRowsSpare, sizes: []string{"73x61", "108x44"},
		fixtures: []string{
			"overview-by-account-burn-down-220x50", "overview-by-account-graphs-160", "overview-pool-220x50",
			"overview-pool-burn-down-160",
		},
	},
	{
		slice: "S10", rule: foldWithRowsSpare, sizes: []string{"73x61", "109x53"},
		fixtures: []string{"overview-six-accounts-160"},
	},
	// S11, SESSIONS takes the rows left (F2): it folds unless every session
	// fits.
	{
		slice: "S11", rule: foldWithRowsSpare, sizes: []string{"73x61", "108x44", "109x53"},
		fixtures: []string{
			"long-names-overview", "overview-burn-down-160x39", "overview-everything-160x39",
			"overview-everything-216x106", "overview-sessions-graphs-216",
		},
	},
	{
		slice: "S11", rule: foldWithRowsSpare, sizes: []string{"73x61", "109x53"},
		fixtures: []string{"overview-sessions-216"},
	},
	// S12, USAGE at short heights (F3): no account at 15 rows.
	{
		slice: "S12", rule: nothingDrawn, sizes: []string{"138x15"},
		fixtures: []string{
			"long-names-overview", "overview-108x44", "overview-138x15", "overview-73x61",
			"overview-bad-hour-160", "overview-burn-down-160x39", "overview-default-160",
			"overview-default-216", "overview-everything-160x39", "overview-everything-216x106",
			"overview-model-nearing-160", "overview-models-160", "overview-phone-52",
			"overview-phone-burn-down-52", "overview-pinned-160", "overview-sessions-216",
			"overview-sessions-graphs-216", "overview-six-accounts-160", "overview-ten-accounts-160",
			"page-help-scrolled", "page-not-answering", "page-routing", "page-routing-160x16",
			"page-routing-phone", "routing-auto-160", "routing-limit-160", "routing-move-160",
			"routing-two-160",
		},
	},
	{
		slice: "S12", rule: wastedThird, sizes: []string{"138x15"},
		fixtures: []string{"overview-phone-burn-down-52"},
	},
	// S13, GRAPHS fluid (F7): no room for its charts with rows spare.
	{
		slice: "S13", rule: foldWithRowsSpare, sizes: []string{"73x61", "108x44"},
		fixtures: []string{
			"overview-by-account-graphs-140", "overview-phone-burn-down-52",
		},
	},
	// S14, USAGE's parts, its names' column at its natural width, at most 24, at
	// every width, as the plan's rule of labels has it.
	{
		slice: "S14", rule: cutWithRoom, sizes: []string{"73x61", "138x30", "108x44", "108x24", "173x53", "109x53"},
		fixtures: []string{"long-names-overview"},
	},
	{
		slice: "S14", rule: cutWithRoom, sizes: []string{"73x61", "73x36"},
		fixtures: []string{"overview-ten-accounts-160"},
	},
	// S15, Compare fluid (F9, F10): its names cut with room.
	{
		slice: "S15", rule: cutWithRoom, sizes: []string{"73x61", "73x36", "108x44", "108x24", "109x53"},
		fixtures: []string{"long-names-accounts-compare"},
	},
	// S16, Detail's stack fluid (F11, F13): its parts 52-column constants.
	{
		slice: "S16", rule: cutWithRoom, sizes: []string{"73x61", "73x36", "108x44", "109x53"},
		fixtures: []string{"long-names-accounts-detail"},
	},
	// S17, Detail's wide form from where it fits (F11, F12): its names cut with
	// room, and its stack drawn at 138 columns.
	{
		slice: "S17", rule: cutWithRoom, sizes: []string{"173x53"},
		fixtures: []string{"long-names-accounts-detail"},
	},
	{
		slice: "S17", rule: wastedThird, sizes: []string{"138x15"},
		fixtures: []string{"accounts-detail-one-160"},
	},
	// S18, the Year (F14): 22 weeks at every width to 159.
	{
		slice: "S18", rule: wastedThird, sizes: []string{"138x30", "138x15", "108x44", "108x24", "109x53"},
		fixtures: []string{
			"history-phone-year-52", "history-year-138x30", "history-year-160", "history-year-opus-160",
			"history-year-plan-unknown-160", "long-names-history-year", "page-year-160x24",
		},
	},
	// S19, Days (F15): its weeks at 15 rows, and its names cut with room.
	{
		slice: "S19", rule: cutWithRoom, sizes: []string{"173x53"},
		fixtures: []string{"long-names-history-days"},
	},
	{
		slice: "S19", rule: wastedThird, sizes: []string{"138x15"},
		fixtures: []string{
			"history-days-138x30", "history-days-160", "history-days-ten-160", "long-names-history-days",
		},
	},
	// S20, Weeks (F16): its names cut with room.
	{
		slice: "S20", rule: cutWithRoom, sizes: []string{"73x61", "73x36", "173x53"},
		fixtures: []string{"long-names-history-weeks"},
	},
	// S21, Tokens (F17): its names' column natural.
	{
		slice: "S21", rule: cutWithRoom, sizes: []string{"73x61", "73x36", "138x30", "138x15", "108x44", "108x24", "173x53", "109x53"},
		fixtures: []string{"long-names-history-tokens"},
	},
	// S22, a session's turns on flex (F31): its accounts cut with room.
	{
		slice: "S22", rule: cutWithRoom, sizes: []string{"138x30", "173x53"},
		fixtures: []string{"long-names-session"},
	},
	// S23, a session's page at short heights (F32): nothing at 15 rows; and an
	// opened turn's requests out of view over blank rows.
	{
		slice: "S23", rule: foldWithRowsSpare, sizes: []string{"73x61", "173x53", "109x53"},
		fixtures: []string{"session-turn-opened-160"},
	},
	{
		slice: "S23", rule: nothingDrawn, sizes: []string{"138x15"},
		fixtures: []string{
			"long-names-session", "page-session-loading", "routing-session-160", "session-108x44",
			"session-ended-160", "session-moved-160", "session-phone-52", "session-quiet-160",
			"session-resumed-160", "session-turn-opened-160",
		},
	},
	{
		slice: "S23", rule: wastedThird, sizes: []string{"138x15"},
		fixtures: []string{
			"long-names-session", "page-session-loading", "routing-session-160", "session-108x44",
			"session-ended-160", "session-moved-160", "session-phone-52", "session-quiet-160",
			"session-resumed-160", "session-turn-opened-160",
		},
	},
}

// Every fixture's view, and the long-names world's, at the owner's sizes,
// with nothing open over its page: no known name cut with room at its row's
// end; no part out of view with rows blank at its foot; Runway, a session's
// page and the Overview drawing a lane, a turn and an account; and the view
// reaching two thirds of the width, but where it's natural. What
// knownBreaks names is passed over, and fails where it no longer breaks.
func TestEveryViewIsFluidAtTheOwnersSizes(t *testing.T) {
	for _, f := range sweptFixtures(t) {
		t.Run(f.Name, func(t *testing.T) {
			t.Parallel()
			m := withNothingOver(t, f.settle(f.Size), f.Size)
			view := viewOf(t, m, f.Size)
			names := knownNames(f)
			for _, size := range ownersSizes {
				d, at := fluidAt(m, size), sizeName(size)
				unknown, cleared := judged(d.breaks(view, names), brokenAt(knownBreaks, f.Name, at))
				for _, b := range unknown {
					t.Errorf("%s at %s breaks the rule of %s, as %s:\n%s", view, at, b.rule, b.shown, strings.Join(d.rows, "\n"))
				}
				for _, rule := range cleared {
					t.Errorf("%s at %s no longer breaks the rule of %s: delete it from knownBreaks", view, at, rule)
				}
			}
		})
	}
}

// brokenAt are the rules known has the fixture named break at the size
// named.
func brokenAt(known []known, name, at string) []fluidRule {
	var rules []fluidRule
	for _, k := range known {
		if slices.Contains(k.fixtures, name) && slices.Contains(k.sizes, at) {
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

// Each of knownBreaks names the slice that clears it, a rule, and fixtures
// the sweep draws at the owner's sizes, each fixture, rule and size once.
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
		if !planSlice.MatchString(k.slice) {
			t.Errorf("knownBreaks names %q, not a slice of the plan", k.slice)
		}
		if !slices.Contains([]fluidRule{cutWithRoom, foldWithRowsSpare, nothingDrawn, wastedThird}, k.rule) {
			t.Errorf("%s's knownBreaks name %q, not a rule the sweep holds", k.slice, k.rule)
		}
		for _, name := range k.fixtures {
			if !swept[name] {
				t.Errorf("%s's knownBreaks name %s, which the sweep doesn't draw", k.slice, name)
			}
			for _, at := range k.sizes {
				if !slices.Contains(sizes, at) {
					t.Errorf("%s's knownBreaks name %s at %s, not one of the owner's sizes", k.slice, name, at)
				}
				if once := name + " " + string(k.rule) + " " + at; named[once] {
					t.Errorf("knownBreaks name %s at %s breaking the rule of %s twice", name, at, k.rule)
				} else {
					named[once] = true
				}
			}
		}
	}
}

// planSlice is a slice of the fluid pass, as the plan numbers them.
var planSlice = regexp.MustCompile(`^S[0-9]+$`)

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

// breaks are the rules the view named breaks as drawn, of the names given:
// one still loading draws nothing and so wastes no width.
func (d fluidFrame) breaks(view string, names []string) []fluidBreak {
	var breaks []fluidBreak
	if cut := d.cutWithRoom(names); cut != "" {
		breaks = append(breaks, fluidBreak{cutWithRoom, cut})
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

// cutWithRoom says where a row of the view cuts one of the names given, as
// "work-tea…", with more blank cells at its end than the cut took, of the
// names a cut may be of, the longest's: "" where none does. An ellipsis
// after anything but a name's start, as a running turn's "3m…", or words
// cut, is no name cut.
func (d fluidFrame) cutWithRoom(names []string) string {
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
				return fmt.Sprintf("%q, %d cells cut, with %d blank at its row's end", cut+"…", took, spare)
			}
		}
	}
	return ""
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
		name string
		view string
		rows []string
		want []fluidRule
	}{
		{name: "a view across the width", view: "Log Requests", rows: []string{"LOG", "", across}},
		{
			name: "a name cut with more blank cells than the cut took", view: "Log Requests",
			rows: []string{"LOG", "", across, "14:40  d28c  work-tea…"}, want: []fluidRule{cutWithRoom},
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
			for _, b := range sweptView(120, tt.rows...).breaks(tt.view, names) {
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
		{slice: "S9", rule: nothingDrawn, sizes: []string{"138x15"}, fixtures: []string{"runway-week-160", "runway-phone-52"}},
		{slice: "S9", rule: foldWithRowsSpare, sizes: []string{"138x30"}, fixtures: []string{"runway-week-160"}},
	}
	cut, fold, none := fluidBreak{rule: cutWithRoom}, fluidBreak{rule: foldWithRowsSpare}, fluidBreak{rule: nothingDrawn}
	for _, tt := range []struct {
		name, fixture, at string
		breaks            []fluidBreak
		unknown           []fluidBreak
		cleared           []fluidRule
	}{
		{name: "breaking what's known", fixture: "runway-phone-52", at: "138x15", breaks: []fluidBreak{none}},
		{name: "breaking what's known and more", fixture: "runway-week-160", at: "138x30", breaks: []fluidBreak{cut, fold}, unknown: []fluidBreak{cut}},
		{name: "breaking what's known elsewhere", fixture: "runway-phone-52", at: "138x30", breaks: []fluidBreak{fold}, unknown: []fluidBreak{fold}},
		{name: "no longer breaking what's known", fixture: "runway-week-160", at: "138x15", cleared: []fluidRule{nothingDrawn}},
		{name: "breaking nothing, with nothing known", fixture: "runway-week-160", at: "73x61"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			unknown, cleared := judged(tt.breaks, brokenAt(known, tt.fixture, tt.at))
			if !slices.Equal(unknown, tt.unknown) || !slices.Equal(cleared, tt.cleared) {
				t.Errorf("judged %v unknown and %q cleared, want %v and %q", unknown, cleared, tt.unknown, tt.cleared)
			}
		})
	}
}
