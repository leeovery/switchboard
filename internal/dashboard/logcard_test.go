package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// cardPage is a page of Requests of n lines of today's, from 09:00 a minute
// apart, at noon, following.
func cardPage(n int) LogPage {
	lines := make([]ledger.Line, n)
	for i := range lines {
		lines[i] = asked(fmt.Sprintf("0a0000%02d", i+1), logDay.Add(time.Duration(i)*time.Minute))
	}
	return requestsOf(views.LogSources{From: logDay, Now: logDayNoon}, lines)
}

// chosenOn is the frame of p, width × height, the line chosen that ups
// presses of ↑ choose.
func chosenOn(p LogPage, ups, width, height int) Frame {
	f := logFrameOf(p, width, height)
	for range ups {
		f.Log = f.Log.Chose(-1, f.LogRows())
	}
	return f
}

// opened is f with its line chosen's details card open over it, the table
// fitted to the card as the watch fits it.
func opened(f Frame) Frame {
	chosen, _ := f.Log.ChosenRequest()
	d, _ := chosen.Details()
	f.Log, f.Card = f.LogFitted(LogCard{Request: &d})
	return f
}

// cardPlaced is where the frame f, drawn as rows, has its card: the rows of
// its top edge and its foot, and of the line it's of, -1 where that's out of
// sight.
func cardPlaced(f Frame, rows []string) (top, foot, of int) {
	chosen, _ := f.Log.ChosenRequest()
	at := prose.ToTheSecond(logDayNoon, chosen.Row.At)
	top, foot, of = -1, -1, -1
	for y, row := range textOf(rows) {
		switch {
		case strings.Contains(row, "╭─ REQUEST"):
			top = y
		case strings.Contains(row, "╰─"):
			foot = y
		case strings.HasPrefix(strings.TrimLeft(row, " ▸"), at):
			of = y
		}
	}
	return top, foot, of
}

// A request's card sits under its line, a row between, where its foot is on
// the table's last row or above; else over it, a row between, where its top
// edge is on the table's first row or below; else the table scrolls the line
// up till the card fits under it, its foot on the table's last row; and
// where the table hasn't the rows for it even so, it's centred over the
// page, as help is. At 160 × 37, the table's rows are 8 to 34 and a card of
// these lines is 20 rows tall, so it fits under a line on rows 8 to 13, and
// over one on rows 29 to 34.
func TestALogCardSitsUnderItsLineElseOverIt(t *testing.T) {
	tests := []struct {
		name        string
		ups, height int
		want        string
	}{
		{name: "a line on the table's first row", ups: 27, height: 37, want: "under"},
		{name: "a line on the lowest row it fits under", ups: 22, height: 37, want: "under"},
		{name: "a line a row lower", ups: 21, height: 37, want: "lifted"},
		{name: "a line a row higher than the highest it fits over", ups: 7, height: 37, want: "lifted"},
		{name: "a line on the highest row it fits over", ups: 6, height: 37, want: "over"},
		{name: "a line on the table's last row", ups: 1, height: 37, want: "over"},
		{name: "a table without the rows for it", ups: 1, height: 24, want: "centred"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := chosenOn(cardPage(40), tt.ups, 160, tt.height)
			stood, _ := f.logLitRow()
			f = opened(f)
			rows := f.Draw(routerDoc("side", 0), logDayNoon)
			top, foot, of := cardPlaced(f, rows)
			_, last := f.logTableRows()
			var ok bool
			switch tt.want {
			case "under":
				ok = of == stood && top == of+2
			case "over":
				ok = of == stood && foot == of-2
			case "lifted":
				ok = of < stood && top == of+2 && foot == last
			case "centred":
				ok = top == (tt.height-(foot-top+1))/2
			}
			if !ok {
				t.Errorf("the line stood on row %d, and is drawn on row %d, the card from row %d to %d; want the card %s it, the table's last row %d\n%s",
					stood+1, of+1, top+1, foot+1, tt.want, last+1, strings.Join(textOf(rows), "\n"))
			}
		})
	}
}

// grown is c, a request's card of a line just come, as its line tells of
// the move it made and the windows its answer left: a row taller.
func grown(c LogCard) LogCard {
	d := *c.Request
	cost := ledger.Picodollars(1_820_000_000_000)
	d.Left = status.Words{{Text: "work", Kind: status.PartAccount}, {Text: ", at its cap"}}
	d.Row.Routing = views.RoutingSaid{Cost: &cost}
	d.Windows = []views.WindowThen{{Key: "5h", Name: "5-hour", Use: 0.09}, {Key: "7d", Name: "week", Use: 0.77}}
	c.Request = &d
	return c
}

// A card placed by its line keeps its place as it grows, the edge by its
// line where it was: under its line, it grows down, and over it, up, as far
// as the table's rows on that side, and past that its body scrolls within
// it. At 160 × 37, the table's rows are 8 to 34, and the card grows from 20
// rows to 21.
func TestALogCardKeepsItsPlaceAsItGrows(t *testing.T) {
	tests := []struct {
		name      string
		ups       int
		top, foot int
		scrolls   bool
	}{
		{name: "under its line on row 13, its foot on the table's last row", ups: 22, top: 14, foot: 33, scrolls: true},
		{name: "under its line on the table's first row", ups: 27, top: 9, foot: 29},
		{name: "over its line on the table's last row", ups: 1, top: 11, foot: 31},
		{name: "over its line on row 29, its top on the table's first row", ups: 6, top: 7, foot: 26, scrolls: true},
	}
	doc := routerDoc("side", 0)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := opened(chosenOn(cardPage(40), tt.ups, 160, 37))
			_, _, was := cardPlaced(f, f.Draw(doc, logDayNoon))
			f.Card = grown(f.Card.(LogCard))
			rows := f.Draw(doc, logDayNoon)
			top, foot, of := cardPlaced(f, rows)
			scrolls := strings.Contains(textOf(rows)[foot-1], "j k scroll")
			if of != was || top != tt.top || foot != tt.foot || scrolls != tt.scrolls {
				t.Errorf("grown, the card is drawn from row %d to %d, scrolling %v, its line on row %d; want from %d to %d, scrolling %v, its line still on row %d\n%s",
					top+1, foot+1, scrolls, of+1, tt.top+1, tt.foot+1, tt.scrolls, was+1, strings.Join(textOf(rows), "\n"))
			}
		})
	}
}

// As the card that lifted the table closes, the table's view comes back down
// to end at its paused foot, the line chosen still in it; a view scrolled up
// short of its foot, and one following, stand as they are. 40 lines in a
// view of 27, the paused foot the 40th.
func TestALiftedTableComesBackDownAsItsCardCloses(t *testing.T) {
	tests := []struct {
		name        string
		f           Frame
		first, last int
	}{
		{name: "lifted, the card under the 29th line", f: opened(chosenOn(cardPage(40), 12, 160, 37)), first: 13, last: 39},
		{name: "scrolled up to the 10th line", f: chosenOn(cardPage(40), 31, 160, 37), first: 9, last: 35},
		{name: "following", f: logFrameOf(cardPage(40), 160, 37), first: 13, last: 39},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rows := tt.f.LogRows()
			p := tt.f.Log.Unlifted(rows)
			lines, s := p.lines(), p.stand()
			first, last := p.window(lines, s, rows)
			at, chosen := p.lit(lines, s)
			if first != tt.first || last != tt.last || chosen && (at < first || at > last) {
				t.Errorf("the view runs from line %d to %d, the line chosen %d; want from %d to %d, the line chosen in it", first+1, last+1, at+1, tt.first+1, tt.last+1)
			}
		})
	}
}

// The line a card is of stays lit as the page fades behind the card: its row
// is drawn as the page draws it without the card, where the rows around it
// fade.
func TestALogCardLeavesItsLineLit(t *testing.T) {
	f := chosenOn(cardPage(40), 22, 160, 37)
	f.Look = Screen(builtin(t, "nord"))
	y, _ := f.logLitRow()
	page := screenOf(t, f.Draw(routerDoc("side", 0), logDayNoon))
	behind := screenOf(t, opened(f).Draw(routerDoc("side", 0), logDayNoon))

	if !slices.Equal(behind[y], page[y]) {
		t.Errorf("the line's row is drawn\n%v\nwant it as the page draws it without the card\n%v", behind[y], page[y])
	}
	if slices.Equal(behind[y-1], page[y-1]) {
		t.Errorf("the row over the line is drawn as the page draws it, want it faded behind the card")
	}
}

// movedRequest is the details of d28c's request on side at 14:31:13, that
// moved it there from work at its cap, as its answer left side's windows.
func movedRequest() views.RequestDetails {
	first, took, stream := 2900*time.Millisecond, 14600*time.Millisecond, true
	cost := ledger.Picodollars(1_820_000_000_000)
	return views.RequestDetails{
		Row: views.RequestRow{
			Stage: views.RowLined, At: time.Date(2026, 10, 1, 14, 31, 13, 0, now.Location()), Request: "3f2a91c4",
			Session: "d28c5e17-0000-4000-8000-000000000001", Dir: "~/Code/switchboard", Model: "claude-opus-5-5", Account: "side",
			Status: 200, Attempts: 1, First: &first, Took: &took, Metered: true, Input: 3, Written: 182_004, Out: 1_214, Rewrote: true,
			Routing: views.RoutingSaid{Cost: &cost},
		},
		Answer: "req_011CQx4n7RkV2bW9pT3sYd6M", Served: "claude-opus-5-5", Betas: []string{"context-1m"},
		Left:    status.Words{{Text: "work", Kind: status.PartAccount}, {Text: ", at its cap"}},
		Windows: []views.WindowThen{{Key: "5h", Name: "5-hour", Use: 0.09}, {Key: "7d", Name: "week", Use: 0.77}},
		Stream:  &stream,
	}
}

// headingsOf are the rows of body each heading given starts, as the plain
// text of each row has it, and the column it starts at: -1 for one not
// found.
func headingsOf(body []line, headings ...string) (rows, columns []int) {
	for _, h := range headings {
		row, column := -1, -1
		for y, l := range body {
			if x := strings.Index(l.plain(), h); x >= 0 {
				row, column = y, len([]rune(l.plain()[:x]))
				break
			}
		}
		rows, columns = append(rows, row), append(columns, column)
	}
	return rows, columns
}

// Where the frame has room for the card's 128 columns, its sections stand in
// three columns, from 0, 50 and 75 of its content; where it hasn't, as on a
// phone, they're stacked one under another, in the columns' order, the
// card as wide as the frame less a cell either side. Nothing runs past its
// content.
func TestALogCardStacksItsSectionsWhereItHasntRoomForItsColumns(t *testing.T) {
	d := movedRequest()
	headings := []string{"REQUEST", "ROUTED", "SIDE'S WINDOWS AFTER IT", "ASKED", "ANSWERED", "TOKENS"}
	for _, width := range []int{52, 80, 100, 120, 129, 130, 160, 200} {
		t.Run(fmt.Sprint(width), func(t *testing.T) {
			form := logFrameOf(LogPage{}, width, 37).logCardForm()
			body := LogCard{Request: &d}.body(form, status.Moment{})
			if wide := width >= 130; form.wide != wide || form.width != min(logCardWidth, width-2) {
				t.Fatalf("the card is %d wide, its columns standing %v; want %d, %v", form.width, form.wide, min(logCardWidth, width-2), wide)
			}
			for y, l := range body {
				if l.width() > form.content {
					t.Errorf("row %d is %d cells, past the card's %d of content: %q", y+1, l.width(), form.content, l.plain())
				}
			}
			rows, columns := headingsOf(body, headings...)
			if form.wide {
				if got := []int{rows[0], columns[0], rows[3], columns[3], rows[4], columns[4]}; !slices.Equal(got, []int{0, 0, 0, 50, 0, 75}) {
					t.Errorf("REQUEST, ASKED and ANSWERED start on rows and columns %v, want each on the first row, at 0, 50 and 75", got)
				}
				return
			}
			if !slices.IsSorted(rows) || slices.Contains(rows, -1) || slices.ContainsFunc(columns, func(x int) bool { return x != 0 }) {
				t.Errorf("the sections start on rows %v, at columns %v; want each under the one before, from the content's first column", rows, columns)
			}
		})
	}
}

// itemsOf are the items of the tree's sections, each as its section's
// heading and its name, "ASKED model", to the text of its value.
func itemsOf(tree [3][]cardSection) map[string]string {
	items := make(map[string]string)
	for _, column := range tree {
		for _, s := range column {
			for _, it := range s.items {
				items[s.heading.plain()+" "+it.name] = it.value.plain()
			}
		}
	}
	return items
}

// Of a request in flight, what its line alone gives is blank till it comes;
// once it's come, what it gives none of is —.
func TestARequestsCardLeavesBlankWhatItsLineIsYetToGive(t *testing.T) {
	asking := views.RequestDetails{Row: views.RequestRow{Stage: views.RowAsking, Request: "3f2a91c4", Model: "claude-opus-5-5", Account: "side"}}
	lined := asking
	lined.Row.Stage, lined.Row.Status = views.RowLined, 200
	tests := []struct {
		name string
		d    views.RequestDetails
		want map[string]string
	}{
		{name: "in flight", d: asking, want: map[string]string{
			"REQUEST id": "3f2a91c4", "REQUEST Anthropic's": "", "ROUTED to": "side", "ROUTED upstream": "",
			"SIDE'S WINDOWS AFTER IT status": "", "ASKED model": "opus-5-5", "ASKED size": "", "ASKED stream": "",
			"ANSWERED status": "", "ANSWERED first byte": "", "ANSWERED ended": "", "TOKENS in": "", "TOKENS worth": "",
		}},
		{name: "its line come", d: lined, want: map[string]string{
			"REQUEST id": "3f2a91c4", "REQUEST Anthropic's": "—", "ROUTED to": "side", "ROUTED upstream": "—",
			"SIDE'S WINDOWS AFTER IT status": "—", "ASKED model": "opus-5-5", "ASKED size": "—", "ASKED stream": "—",
			"ANSWERED status": "200", "ANSWERED first byte": "—", "ANSWERED ended": "—", "TOKENS in": "—", "TOKENS worth": "$0.00 at API prices",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := itemsOf(requestTree(tt.d, status.Moment{}))
			for item, want := range tt.want {
				if got[item] != want {
					t.Errorf("%s reads %q, want %q", item, got[item], want)
				}
			}
		})
	}
}

// The card of a request answering counts its characters as it's drawn, as
// the stream the card holds then counts them, its details made once: not as
// the page behind it holds its own, which stands still as the card opened.
func TestARequestsCardInFlightCountsItsCharactersAsItsDrawn(t *testing.T) {
	told := views.Told{
		Request: "3f2a91c4", Session: "d28c5e17-0000-4000-8000-000000000001", Model: "claude-opus-5-5", Account: "side",
		Sent: logDay, First: logDay.Add(time.Second), Chars: 1000,
	}
	stream := views.Stream{Live: true, Requests: map[string]views.Told{told.Request: told}}
	d := views.RequestDetails{Row: views.LogRows(views.LogSources{From: logDay, Now: logDay.Add(2 * time.Second), Stream: stream}).Lines[0].Row}
	f := logFrameOf(cardPage(1), 160, 37)
	f.Log.Stream = stream
	for _, chars := range []int{1000, 5100} {
		streamed := told
		streamed.Chars = chars
		body := plainLines(LogCard{Request: &d, Stream: views.Stream{Live: true, Requests: map[string]views.Told{told.Request: streamed}}}.card(f).body)
		want := "answering · " + prose.Characters(chars) + " characters"
		if !slices.ContainsFunc(body, func(l string) bool { return strings.Contains(l, want) }) {
			t.Errorf("%d characters streamed, the card doesn't say %q:\n%s", chars, want, strings.Join(body, "\n"))
		}
	}
}

// A request's status is bold state.destructive where views says it failed,
// as the table has it, whatever it was answered.
func TestARequestsCardSaysItFailedAsItsRowDoes(t *testing.T) {
	tests := []struct {
		name string
		row  views.RequestRow
		want line
	}{
		{name: "failed", row: views.RequestRow{Status: 500, Failed: true}, want: line{{"500", exhaustedInk}}},
		{name: "cut off, answered 200", row: views.RequestRow{Status: 200, Failed: true}, want: line{{"200", exhaustedInk}}},
		{name: "answered 200", row: views.RequestRow{Status: 200}, want: line{{"200", secondaryInk}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := statusSaid(tt.row); !slices.Equal(got, tt.want) {
				t.Errorf("its status is drawn %+v, want %+v", got, tt.want)
			}
		})
	}
}

// A request that moved its session says where from and why, and what the
// move cost, of its worth, or unpriced where the price table can't price it;
// one that moved none says neither.
func TestARequestsCardSaysTheMoveItMade(t *testing.T) {
	moved := movedRequest()
	unpriced := moved
	unpriced.Row.Routing = views.RoutingSaid{Rewrote: true}
	stayed := moved
	stayed.Left = nil
	for _, tt := range []struct {
		name string
		d    views.RequestDetails
		want map[string]string
	}{
		{name: "moved", d: moved, want: map[string]string{"ROUTED from": "work, at its cap", "TOKENS the move": "$1.82 of it"}},
		{name: "moved, on a model the price table can't price", d: unpriced,
			want: map[string]string{"ROUTED from": "work, at its cap", "TOKENS the move": "unpriced"}},
		{name: "stayed", d: stayed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := itemsOf(requestTree(tt.d, status.Moment{}))
			for _, item := range []string{"ROUTED from", "TOKENS the move"} {
				if said, ok := got[item]; said != tt.want[item] || ok != (tt.want[item] != "") {
					t.Errorf("%s reads %q, there %v; want %q", item, said, ok, tt.want[item])
				}
			}
		})
	}
}

// eventAt is when the events of the cards' tests happened: Thursday 1
// October, 14:31:03.
var eventAt = time.Date(2026, 10, 1, 14, 31, 3, 0, now.Location())

// cents is an amount of cents, as picodollars.
func cents(n int64) *ledger.Picodollars {
	p := ledger.Picodollars(n * 10_000_000_000)
	return &p
}

// treeRows are the rows of the tree's column given as a column of the
// wide card's third's width draws them, their blanks at the end trimmed.
func treeRows(tree [3][]cardSection, column int) []string {
	var rows []string
	for _, l := range treeColumn(tree[column], logCardWidth-2*cardInset-logCardColumns[2]) {
		rows = append(rows, strings.TrimRight(l.plain(), " "))
	}
	return rows
}

// What followed an event is said a row each: of a move, its request, the
// tokens it cached again and what that cost; of a limit, a cap or a
// refusal, each session it moved, by its id, where it went and the move's
// cost three cells on; till when new sessions, or a family's requests, go
// elsewhere, a day named where it's not the event's; and what the moves
// cost in all. An event nothing followed has no such section.
func TestAnEventsCardSaysWhatFollowed(t *testing.T) {
	d28c, f7a3 := "d28c5e17-0000-4000-8000-000000000001", "7f3a0b2c-0000-4000-8000-000000000002"
	tests := []struct {
		name     string
		followed *views.Followed
		want     []string
	}{
		{name: "a cap's moves", followed: &views.Followed{
			Moves: []views.FollowedMove{
				{Session: d28c, To: "side", Move: &views.EventMove{Cost: cents(182)}},
				{Session: f7a3, To: "side"},
			},
			Till: eventAt.Add(2*time.Hour + 39*time.Minute), InAll: cents(182),
		}, want: []string{
			"WHAT FOLLOWED",
			"├ d28c         moved to side   $1.82",
			"├ 7f3a         moved to side",
			"├ new sessions go elsewhere till 17:10",
			"└ in all       $1.82 to cache them again",
		}},
		{name: "a limit's moves, one written again on a model the price table can't price", followed: &views.Followed{
			Moves: []views.FollowedMove{
				{Session: d28c, To: "side", Move: &views.EventMove{Cost: cents(182), Rewrote: true}},
				{Session: f7a3, To: "side", Move: &views.EventMove{Rewrote: true}},
			},
			InAll: cents(182), Unpriced: 1,
		}, want: []string{
			"WHAT FOLLOWED",
			"├ d28c         moved to side   $1.82",
			"├ 7f3a         moved to side   unpriced",
			"└ in all       $1.82+ to cache them again",
		}},
		{name: "a limit's moves, each unpriced", followed: &views.Followed{
			Moves: []views.FollowedMove{{Session: d28c, To: "side", Move: &views.EventMove{Rewrote: true}}}, Unpriced: 1,
		}, want: []string{"WHAT FOLLOWED", "├ d28c         moved to side   unpriced", "└ in all       unpriced to cache them again"}},
		{name: "a family's refusal, till the next day", followed: &views.Followed{
			Till: time.Date(2026, 10, 2, 1, 15, 0, 0, now.Location()), Family: "opus",
		}, want: []string{"WHAT FOLLOWED", "└ opus         go elsewhere till Fri 01:15"}},
		{name: "a move", followed: &views.Followed{
			Move: &views.EventMove{Request: "3f2a91c4", Written: 182_004, Cost: cents(182)},
		}, want: []string{"WHAT FOLLOWED", "├ request      3f2a91c4", "├ cached again 182,004", "└ the move     $1.82"}},
		{name: "a move written again on a model the price table can't price", followed: &views.Followed{
			Move: &views.EventMove{Request: "3f2a91c4", Written: 182_004, Rewrote: true},
		}, want: []string{"WHAT FOLLOWED", "├ request      3f2a91c4", "├ cached again 182,004", "└ the move     unpriced"}},
		{name: "nothing"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := views.EventDetails{Row: views.LogEvent{At: eventAt}, Followed: tt.followed}
			if got := treeRows(eventTree(d, status.Moment{Now: eventAt}), 2); !slices.Equal(got, tt.want) {
				t.Errorf("WHAT FOLLOWED reads\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

// The account an event befell is said as it stood then, headed by its name:
// its windows' use, when what the event tells of resets where that's known,
// and how many sessions were on it, none among them.
func TestAnEventsCardSaysHowItsAccountStoodThen(t *testing.T) {
	tests := []struct {
		name string
		then views.AccountThen
		want []string
	}{
		{name: "work at its cap", then: views.AccountThen{
			Account: "work", Windows: []views.WindowThen{{Key: "5h", Name: "5-hour", Use: 0.95}, {Key: "7d", Name: "week", Use: 0.34}},
			Resets: eventAt.Add(2*time.Hour + 39*time.Minute), Sessions: 3,
		}, want: []string{"WORK THEN", "├ 5-hour       95%", "├ week         34%", "├ resets       17:10", "└ sessions     3 on it"}},
		{name: "an account no answer read, and none on it", then: views.AccountThen{Account: "personal"},
			want: []string{"PERSONAL THEN", "└ sessions     0 on it"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := views.EventDetails{Row: views.LogEvent{At: eventAt}, Then: &tt.then}
			if got := treeRows(eventTree(d, status.Moment{Now: eventAt}), 1); !slices.Equal(got, tt.want) {
				t.Errorf("the account then reads\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

// A card's keys: ↑↓ stepping, an event's g, s where its line is of a
// session, as the request counts toward it, but over that session's own
// page (call 63), j k where its body scrolls, and esc. Where they don't fit,
// ↑↓ says what it steps to as the key line does; then they leave from the
// first, esc staying.
func TestALogCardsKeysFitItsContent(t *testing.T) {
	session := "d28c5e17-0000-4000-8000-000000000001"
	request := LogCard{Request: &views.RequestDetails{Row: views.RequestRow{Session: session, CountedSession: session}}}
	unshown := "\x1b\x07"
	tests := []struct {
		name    string
		card    LogCard
		content int
		scrolls bool
		paged   bool
		want    string
	}{
		{name: "a request of a session", card: request, content: 122, want: "↑↓ previous/next request   s its session   esc close"},
		{name: "a request over its session's page", card: request, content: 122, paged: true, want: "↑↓ previous/next request   esc close"},
		{name: "a request of none", card: LogCard{Request: &views.RequestDetails{}}, content: 122, want: "↑↓ previous/next request   esc close"},
		{name: "a quota check, of no session", card: LogCard{Request: &views.RequestDetails{Row: views.RequestRow{Session: session, Kind: views.RowCheck}}}, content: 122,
			want: "↑↓ previous/next request   esc close"},
		{name: "a session whose id shows as nothing, as the title has it", card: LogCard{Request: &views.RequestDetails{Row: views.RequestRow{Session: unshown, CountedSession: unshown}}},
			content: 122, want: "↑↓ previous/next request   esc close"},
		{name: "an event of none", card: LogCard{Event: &views.EventDetails{}}, content: 122, want: "↑↓ previous/next event   g in Requests   esc close"},
		{name: "an event of a session", card: LogCard{Event: &views.EventDetails{Row: views.LogEvent{Session: session}}}, content: 122,
			want: "↑↓ previous/next event   g in Requests   s its session   esc close"},
		{name: "scrolling", card: request, content: 122, scrolls: true, want: "↑↓ previous/next request   s its session   j k scroll   esc close"},
		{name: "short of room", card: request, content: 44, want: "↑↓ request   s its session   esc close"},
		{name: "shorter", card: request, content: 30, want: "s its session   esc close"},
		{name: "room for esc alone", card: request, content: 10, want: "esc close"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := keyRow(tt.card.keys(tt.content, tt.scrolls, !tt.paged)).plain(); got != tt.want {
				t.Errorf("the keys read %q, want %q", got, tt.want)
			}
		})
	}
}

// A centred card shows its body scrolled as far as it's asked, no further
// than its end, saying so with j k.
func TestACentredLogCardScrollsItsBody(t *testing.T) {
	f := chosenOn(cardPage(40), 1, 160, 20)
	chosen, _ := f.Log.ChosenRequest()
	d, _ := chosen.Details()
	for _, scroll := range []int{0, 3, 99} {
		t.Run(fmt.Sprint(scroll), func(t *testing.T) {
			c := LogCard{Request: &d, Scroll: scroll}
			whole, k := c.body(f.logCardForm(), status.Moment{}), c.card(f)
			from := min(scroll, len(whole)-len(k.body))
			if k.of.placed || k.scroll != (cardScroll{above: from, below: len(whole) - from - len(k.body)}) || k.body[0].plain() != whole[from].plain() {
				t.Errorf("scrolled %d, the card shows %d of %d rows from %q, scrolled %+v; want it centred, from row %d", scroll, len(k.body), len(whole), k.body[0].plain(), k.scroll, from+1)
			}
			if !slices.ContainsFunc(k.keys, func(key Key) bool { return key.Key == "j k" }) {
				t.Errorf("the keys are %v, want j k scroll among them", k.keys)
			}
		})
	}
}

// An event's card names the accounts it tells of as the document names them:
// in its title, its items, the account then and what followed.
func TestAnEventsCardNamesItsAccountsAsTheDocumentDoes(t *testing.T) {
	m := routerDoc("side", 0, status.Account{ID: "work", Label: "Studio"}, status.Account{ID: "side", Label: "Lab"}).At(eventAt, claudeLike)
	d := views.EventDetails{
		Row:      views.LogEvent{At: eventAt},
		Headline: status.Words{{Text: "work", Kind: status.PartAccount}, {Text: " reached its cap"}},
		Happened: []views.DetailItem{{Name: "from", Value: views.DetailWords{{Text: "work", Kind: views.DetailAccount}}}},
		Then:     &views.AccountThen{Account: "work"},
		Followed: &views.Followed{Moves: []views.FollowedMove{{Session: "d28c5e17-0000-4000-8000-000000000001", To: "side"}}},
	}
	tree := eventTree(d, m)
	items := itemsOf(tree)
	got := []string{LogCard{Event: &d}.title(m).plain(), items["WHAT HAPPENED from"], tree[1][0].heading.plain(), items["WHAT FOLLOWED d28c"]}
	if want := []string{"EVENT · 14:31:03 · Studio reached its cap", "Studio", "STUDIO THEN", "moved to Lab"}; !slices.Equal(got, want) {
		t.Errorf("the card says %q, want %q", got, want)
	}
}
