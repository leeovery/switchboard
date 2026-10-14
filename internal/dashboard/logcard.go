package dashboard

import (
	"cmp"
	"slices"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/theme"
	"github.com/leeovery/switchboard/internal/views"
)

// LogCard is the Log's details card of the line chosen, as enter opens it
// over the page: a request's details, or an event's, as the watch builds
// them of what it holds, afresh at each change, so the card follows each
// read; what the request stream tells as the card is drawn, which a request
// in flight's details are drawn as, while the page behind stands still as
// its own stream told it; how far its body is scrolled, where it hasn't the
// rows for it; and how many rows it took as LogFitted placed it, by which it
// keeps its side of its line as it grows, none where it's yet to be placed.
type LogCard struct {
	Request *views.RequestDetails
	Event   *views.EventDetails
	Stream  views.Stream
	Scroll  int
	Placed  int
}

// The card's wide form: logCardWidth cells wide, its edges included, as it
// sits from column 16 at 160; its tree's columns, from where its content
// starts, as the frames draw them from 19, 69 and 94.
const logCardWidth = 128

var logCardColumns = [3]int{0, 50, 75}

// An item of the tree: its branch, ├ or └, and a space, then its name,
// padded to itemName cells, its value after them.
const (
	itemName = 13
	itemLead = 2 + itemName
)

// cardForm is how the card is laid out on a frame: its width, its edges
// included, and its content's; and whether its tree is in its three
// columns, or stacked one under another, as on a phone.
type cardForm struct {
	width, content int
	wide           bool
}

// logCardForm is the card's form on the frame: wide where the frame has room
// for it, its wide form's columns, else stacked, as wide as the frame less a
// cell either side.
func (f Frame) logCardForm() cardForm {
	width := f.cardWidth(logCardWidth)
	return cardForm{width: width, content: width - 2*cardInset, wide: width == logCardWidth}
}

// card is the details card on the frame: titled as the line it's of is, its
// tree, and of a request, its foot; placed by its line, under it or over it,
// a row between, where it fits there as it was placed, else centred, as help
// is. Placed, it keeps the edge by its line where it was as it grows, as far
// as the table's rows on that side, so its line never moves under it; its
// body scrolls within it past the rows it has. Its line, wherever it's in
// view, is left bright as the page fades behind it. A request in flight's
// is as it stands at the moment drawn, as its row is, as the card's stream
// tells it.
func (c LogCard) card(f Frame) Card {
	form := f.logCardForm()
	m := f.heldMoment()
	if c.Request != nil {
		d := *c.Request
		d.Row = d.Row.Live(c.Stream, m.Now)
		c.Request = &d
	}
	body := c.body(form, m)
	h := len(body) + chrome(true)
	k := Card{titled: c.title(m), width: form.width}
	room := f.bodyRoom(2, true)
	if y, ok := f.cardLine(); ok {
		k.of = &ofLine{row: y}
		if from, to, up, placed := f.logCardSpan(y, cmp.Or(c.Placed, h)); placed {
			rows := min(h, to-from+1)
			k.of.top, k.of.placed, room = from, true, rows-chrome(true)
			if up {
				k.of.top = to - rows + 1
			}
		}
	}
	shown := min(max(room, 0), len(body))
	from := min(max(c.Scroll, 0), len(body)-shown)
	k.body, k.scroll = body[from:from+shown], cardScroll{above: from, below: len(body) - from - shown}
	k.keys = c.keys(form.content, k.scroll != cardScroll{}, f.Shown() != SessionPage)
	return k
}

// heldMoment is the moment of the document the frame holds, as Draw holds
// it before it draws a card over the page, which names its accounts and
// tells its time zone: the zero Moment, naming each account by its id,
// where it holds none, as a frame laid out but not drawn.
func (f Frame) heldMoment() status.Moment {
	if f.held == nil {
		return status.Moment{}
	}
	return f.held.Moment()
}

// body is the card's body, in its form, its accounts named as m names them:
// its tree, and of a request, the foot under it.
func (c LogCard) body(form cardForm, m status.Moment) []line {
	tree := c.tree(m)
	var body []line
	if form.wide {
		body = wideTree(tree, form.content)
	} else {
		body = treeColumn(slices.Concat(tree[:]...), form.content)
	}
	if c.Request != nil {
		body = append(body, neverKept.wrap(form.content)...)
	}
	return body
}

// neverKept is a request's foot: what the ledger keeps none of.
var neverKept = line{{"never kept: " + ledgerNeverKeeps, faintInk}}

// tree is the card's tree, its sections in its three columns.
func (c LogCard) tree(m status.Moment) [3][]cardSection {
	if c.Event != nil {
		return eventTree(*c.Event, m)
	}
	if c.Request != nil {
		return requestTree(*c.Request, m)
	}
	return [3][]cardSection{}
}

// title is the card's title, its parts · apart: a request's REQUEST, its
// line's time and its session as the table names it; an event's EVENT, its
// time and what happened, as its event item says it, an account it names
// white.
func (c LogCard) title(m status.Moment) line {
	switch {
	case c.Event != nil:
		l := line{{"EVENT · " + prose.ToTheSecond(m.Now, c.Event.Row.At) + " · ", dimInk}}
		for _, p := range c.Event.Headline {
			if p.Kind == status.PartAccount {
				l = append(l, span{named(m, p.Text), accountInk})
				continue
			}
			l = append(l, span{p.Text, dimInk})
		}
		return l
	case c.Request != nil:
		r := c.Request.Row
		title := "REQUEST · " + prose.ToTheSecond(m.Now, r.At)
		if prose.Clean(r.Session) != "" {
			title += " · " + sessionNamed(r.Session, r.Dir)
		}
		return line{{title, dimInk}}
	}
	return nil
}

// Session is the whole id of the session the card's line is of, whose page
// s opens, as sessionOfLine has it.
func (c LogCard) Session() string {
	if c.Event != nil {
		return sessionOfLine(nil, &c.Event.Row)
	}
	if c.Request != nil {
		return sessionOfLine(&c.Request.Row, nil)
	}
	return ""
}

// sessionOfLine is the whole id of the session a line of the Log's is of,
// whose page s opens, the line chosen in its table or its card's alike: a
// request's row's, as the row counts it, none for a quota check; an
// event's; none for an account's event, nor where the id shows as nothing,
// as the card's title and the Events have it.
func sessionOfLine(row *views.RequestRow, event *views.LogEvent) string {
	var id string
	switch {
	case event != nil:
		id = event.Session
	case row != nil:
		id = row.CountedSession
	}
	if prose.Clean(id) == "" {
		return ""
	}
	return id
}

// ChosenSession is the whole id of the session the line chosen of the table
// shown is of, whose page s opens, as sessionOfLine has it: "" where none is
// chosen.
func (p LogPage) ChosenSession() string {
	row, event, ok := p.Chosen()
	switch {
	case !ok:
		return ""
	case event != nil:
		return sessionOfLine(nil, event)
	}
	return sessionOfLine(&row, nil)
}

// keys are the card's keys, as its keys row lists them in content cells:
// ↑↓ stepping, an event's g, s where the line is of a session, as Session
// has it, but where sessions says s opens none, as over that session's own
// page (call 63), j k where its body scrolls, and esc. Where they don't fit,
// ↑↓ says what it steps to as the key line does, then the keys leave from
// the first, esc staying.
func (c LogCard) keys(content int, scrolls, sessions bool) []Key {
	noun := "request"
	var middle []Key
	if c.Event != nil {
		noun = "event"
		middle = append(middle, Key{Key: "g", Does: "in Requests"})
	}
	if c.Session() != "" && sessions {
		middle = append(middle, Key{Key: "s", Does: "its session"})
	}
	if scrolls {
		middle = append(middle, Key{Key: "j k", Does: "scroll"})
	}
	closing := Key{Key: "esc", Does: "close"}
	for _, steps := range []string{"previous/next " + noun, noun} {
		if keys := slices.Concat([]Key{{Key: "↑↓", Does: steps}}, middle, []Key{closing}); keyRow(keys).width() <= content {
			return keys
		}
	}
	keys := append(middle, closing)
	for len(keys) > 1 && keyRow(keys).width() > content {
		keys = keys[1:]
	}
	return keys
}

// cardSection is a section of a details card's tree: its heading, in
// text.subtle, an account it names in text.primary; and its items, a row
// each.
type cardSection struct {
	heading line
	items   []cardItem
}

// cardItem is an item of a section: its name, in text.muted; and its value,
// in the inks it's said in, blank where it isn't known yet, which a move's
// cost ends where costs says, kept whole as what comes before it is cut.
type cardItem struct {
	name  string
	value line
	costs bool
}

// sectionHeading is a section's heading of words alone.
func sectionHeading(text string) line {
	return line{{text, dimInk}}
}

// row is the item as its section's row width cells wide draws it: ├ in
// text.faint, └ where it's last; its name, padded; and its value, cut with
// an ellipsis to the room left, a move's cost kept whole.
func (it cardItem) row(last bool, width int) line {
	branch := "├ "
	if last {
		branch = "└ "
	}
	name := line{{it.name, mutedInk}}.fit(itemName - 1)
	lead := slices.Concat(line{{branch, faintInk}}, name, line{spaces(itemName - name.width())})
	return slices.Concat(lead, costKept(it.value, it.costs, width-itemLead))
}

// treeColumn is the sections as a column width cells wide draws them, a
// blank row between: each its heading, then its items, a row each.
func treeColumn(sections []cardSection, width int) []line {
	var rows []line
	for i, s := range sections {
		if i > 0 {
			rows = append(rows, nil)
		}
		rows = append(rows, s.heading.fit(width))
		for j, it := range s.items {
			rows = append(rows, it.row(j == len(s.items)-1, width))
		}
	}
	return rows
}

// wideTree is the tree's columns side by side, in content cells, each from
// its place in logCardColumns, as tall as the tallest; a column running no
// further than a cell before the next.
func wideTree(tree [3][]cardSection, content int) []line {
	var columns [3][]line
	height := 0
	for i, sections := range tree {
		width := content - logCardColumns[i]
		if i+1 < len(logCardColumns) {
			width = logCardColumns[i+1] - logCardColumns[i] - 1
		}
		columns[i] = treeColumn(sections, width)
		height = max(height, len(columns[i]))
	}
	rows := make([]line, height)
	for y := range rows {
		for i, column := range columns {
			if y < len(column) && len(column[y]) > 0 {
				rows[y] = slices.Concat(rows[y], line{spaces(logCardColumns[i] - rows[y].width())}, column[y])
			}
		}
	}
	return rows
}

// valued is a value as an item says it, in text.secondary: — in text.faint
// for none.
func valued(text string) line {
	if text == "" {
		return line{{"—", faintInk}}
	}
	return line{{text, secondaryInk}}
}

// lineGives says what of a request's details its line gives, once it has
// come: a value as valued says it; else blank, as it's yet to come.
type lineGives bool

func (lined lineGives) said(text string) line {
	if !lined {
		return nil
	}
	return valued(text)
}

// windowUse is a window's share used, bold in its level's colour, as usage
// colours it.
func windowUse(used float64) span {
	return span{prose.Percent(used), useInk(used)}
}

// useInk is the ink of a window's share used, bold in its level's colour.
func useInk(used float64) ink {
	return usedInk(used, ink{token: theme.VizRamp1, bold: true})
}

// givenAs is n as say words it: "" for none.
func givenAs(n int, say func(int) string) string {
	if n == 0 {
		return ""
	}
	return say(n)
}

// requestTree is the tree of the request's details d, its accounts named
// as m names them: REQUEST, ROUTED, and the windows of the account it went
// out on as its answer left them, in its first column; ASKED in its second;
// ANSWERED and TOKENS in its third. Of a request in flight, what its line
// alone gives is blank, filling in as it comes, with no row added but those
// of a move and of its windows, which only its line tells.
func requestTree(d views.RequestDetails, m status.Moment) [3][]cardSection {
	r := d.Row
	lined := lineGives(r.Stage == views.RowLined)
	moved := len(d.Left) > 0
	request := cardSection{heading: sectionHeading("REQUEST"), items: []cardItem{
		{name: "id", value: valued(prose.Clean(r.Request))},
		{name: "Anthropic's", value: lined.said(prose.Clean(d.Answer))},
		{name: "from", value: lined.said(prose.Clean(r.Agent))},
		{name: "betas", value: lined.said(strings.Join(cleanedAll(d.Betas), ", "))},
	}}
	routed := cardSection{heading: sectionHeading("ROUTED"), items: []cardItem{{name: "to", value: valued(named(m, r.Account))}}}
	if moved {
		routed.items = append(routed.items, cardItem{name: "from", value: accountsSaid(d.Left, m)})
	}
	routed.items = append(routed.items, cardItem{name: "upstream", value: lined.said(givenAs(r.Attempts, prose.Times))})
	windows := cardSection{heading: windowsHeading(named(m, r.Account))}
	for _, w := range d.Windows {
		windows.items = append(windows.items, cardItem{name: w.Name, value: line{windowUse(w.Use)}})
	}
	windows.items = append(windows.items, cardItem{name: "status", value: lined.said(prose.Clean(string(d.LimitStatus)))})
	return [3][]cardSection{{request, routed, windows}, {askedSection(d, lined)}, {answeredSection(d, lined), tokensSection(d, lined, moved)}}
}

// cleanedAll are the texts given, each cleaned.
func cleanedAll(texts []string) []string {
	cleaned := make([]string, len(texts))
	for i, t := range texts {
		cleaned[i] = prose.Clean(t)
	}
	return cleaned
}

// windowsHeading heads the windows of the account named as its answer left
// them: SIDE'S WINDOWS AFTER IT, the account white.
func windowsHeading(account string) line {
	if account == "" {
		return sectionHeading("WINDOWS AFTER IT")
	}
	return line{{strings.ToUpper(account), accountInk}, {"'S WINDOWS AFTER IT", dimInk}}
}

// accountsSaid is words naming accounts as an item says them: each account
// named as m names it, in text.secondary, the words around them muted.
func accountsSaid(w status.Words, m status.Moment) line {
	l := make(line, len(w))
	for i, p := range w {
		l[i] = span{p.Text, mutedInk}
		if p.Kind == status.PartAccount {
			l[i] = span{named(m, p.Text), secondaryInk}
		}
	}
	return l
}

// askedSection is ASKED: the model it asked for, and its shape, as its line
// gives it.
func askedSection(d views.RequestDetails, lined lineGives) cardSection {
	r := d.Row
	return cardSection{heading: sectionHeading("ASKED"), items: []cardItem{
		{name: "model", value: valued(modelNamed(r.Model))},
		{name: "size", value: lined.said(givenAs(r.Size, prose.Bytes))},
		{name: "messages", value: lined.said(givenAs(r.Messages, prose.Grouped))},
		{name: "system", value: lined.said(givenAs(d.System, systemBlocks))},
		{name: "tools", value: lined.said(givenAs(r.Tools, prose.Grouped))},
		{name: "max_tokens", value: lined.said(givenAs(d.MaxTokens, prose.Grouped))},
		{name: "thinking", value: lined.said(givenAs(r.Thinking, prose.Grouped))},
		{name: "effort", value: lined.said(prose.Clean(d.Effort))},
		{name: "stream", value: lined.said(yesOrNo(d.Stream))},
	}}
}

// systemBlocks counts a system prompt's blocks: "3 blocks".
func systemBlocks(n int) string {
	return prose.Counted(n, "block")
}

// yesOrNo says whether it asked to stream: "" where it didn't say.
func yesOrNo(asked *bool) string {
	switch {
	case asked == nil:
		return ""
	case *asked:
		return "yes"
	}
	return "no"
}

// answeredSection is ANSWERED: its status, the model that served it, its
// times, why it stopped, as the table words it, and its answer's blocks. Of
// a request in flight, its first byte once it's come, and what it's doing.
func answeredSection(d views.RequestDetails, lined lineGives) cardSection {
	r := d.Row
	return cardSection{heading: sectionHeading("ANSWERED"), items: []cardItem{
		{name: "status", value: statusSaid(r)},
		{name: "model", value: lined.said(modelNamed(d.Served))},
		{name: "first byte", value: firstSaid(r)},
		{name: "ended", value: endedSaid(r)},
		{name: "stopped", value: stoppedSaid(r, lined)},
		{name: "blocks", value: lined.said(blocksSaid(d.Blocks))},
	}}
}

// statusSaid is what the request was answered, once its line came or the
// stream told its end, bold state.destructive where its row says it failed:
// blank in flight, and — where it was lost.
func statusSaid(r views.RequestRow) line {
	switch {
	case r.Stage.InFlight():
		return nil
	case r.Stage == views.RowLost:
		return valued("")
	case r.Failed:
		return line{{prose.Grouped(r.Status), exhaustedInk}}
	}
	return valued(prose.Grouped(r.Status))
}

// firstSaid is how long after the request arrived its answer's first byte
// came, as the Log writes it: blank while it asks, and — where none came.
func firstSaid(r views.RequestRow) line {
	switch {
	case r.First != nil:
		return valued(prose.Seconds(*r.First))
	case r.Stage.InFlight():
		return nil
	}
	return valued("")
}

// endedSaid is how long the request took in all, as the Log writes it, once
// it ended: blank in flight, and — where it ended untold.
func endedSaid(r views.RequestRow) line {
	switch {
	case r.Stage.InFlight():
		return nil
	case r.Took != nil:
		return valued(prose.Seconds(*r.Took))
	}
	return valued("")
}

// stoppedSaid is why the request's answer stopped, as the table words it,
// the words in text.secondary: in flight, what it's doing.
func stoppedSaid(r views.RequestRow, lined lineGives) line {
	w := r.Stopped()
	if len(w) == 0 {
		return lined.said("")
	}
	l := make(line, len(w))
	for i, p := range w {
		l[i] = span{p.Text, cardPartInks[p.Kind]}
	}
	return l
}

// cardPartInks are the inks of a row's words as a card's value says them:
// what's said and what's named in text.secondary, what's between in
// text.faint, and what a request in flight is doing bold accent.key.
var cardPartInks = map[views.RowPartKind]ink{
	views.RowSaid: secondaryInk, views.RowBetween: faintInk, views.RowNamed: secondaryInk, views.RowDoing: keyInk, views.RowCost: warningInk,
}

// blocksSaid are an answer's blocks, counted by their kinds, as "2
// thinking, 1 text, 1 tool use".
func blocksSaid(blocks []views.DetailBlock) string {
	said := make([]string, len(blocks))
	for i, b := range blocks {
		said[i] = prose.Grouped(b.Count) + " " + strings.ReplaceAll(prose.Clean(b.Kind), "_", " ")
	}
	return strings.Join(said, ", ")
}

// tokensSection is TOKENS: what its answer's usage counts, exactly, and its
// writes to the cache, bold accent.attention where it wrote its session's
// context again, and how long they last; its worth, at API prices; and of a
// request that moved its session, the move, its cache write priced.
func tokensSection(d views.RequestDetails, lined lineGives, moved bool) cardSection {
	r := d.Row
	s := cardSection{heading: sectionHeading("TOKENS"), items: []cardItem{
		{name: "in", value: metered(r, r.Input, secondaryInk, lined)},
		{name: "from cache", value: metered(r, r.Read, secondaryInk, lined)},
		{name: "cached", value: cachedSaid(d, lined)},
		{name: "out", value: metered(r, r.Out, secondaryInk, lined)},
		{name: "worth", value: worthAtAPIPrices(r, lined)},
	}}
	if moved {
		s.items = append(s.items, cardItem{name: "the move", value: theMove(r.Routing.Cost)})
	}
	return s
}

// metered is n of the request's tokens, exactly, in the ink given, where its
// answer gave usage: blank before, — where it gave none.
func metered(r views.RequestRow, n int, k ink, lined lineGives) line {
	if !r.Metered {
		return lined.said("")
	}
	return line{{prose.Grouped(n), k}}
}

// cacheLives are how long a request's writes to the cache last, as its card
// words them.
var cacheLives = map[time.Duration]string{ledger.ShortCache: " for 5 minutes", ledger.LongCache: " for an hour"}

// cachedSaid is what the request wrote to the cache, bold accent.attention
// where it wrote its session's context again, then how long its writes
// last, where it wrote any.
func cachedSaid(d views.RequestDetails, lined lineGives) line {
	r, k := d.Row, secondaryInk
	if r.Rewrote {
		k = alertInk
	}
	l := metered(r, r.Written, k, lined)
	if life, ok := cacheLives[d.CacheLife]; ok && r.Metered && r.Written > 0 {
		l = append(l, span{life, mutedInk})
	}
	return l
}

// worthAtAPIPrices is what the request would have cost through the API, bold,
// once its line came.
func worthAtAPIPrices(r views.RequestRow, lined lineGives) line {
	if !lined {
		return nil
	}
	return line{{r.Worth.Cents(), strongInk}, {" at API prices", mutedInk}}
}

// theMove is what writing the session's context again on the account it
// moved to cost, bold accent.attention, of its worth: — where it's not
// known.
func theMove(cost *ledger.Picodollars) line {
	if cost == nil {
		return valued("")
	}
	return line{{views.Worth{Amount: cost}.Cents(), alertInk}, {" of it", mutedInk}}
}

// eventTree is the tree of the event's details d, its accounts named as m
// names them: WHAT HAPPENED in its first column; the account it befell, as
// it stood then, in its second; and WHAT FOLLOWED, where anything did, in
// its third.
func eventTree(d views.EventDetails, m status.Moment) [3][]cardSection {
	var tree [3][]cardSection
	happened := cardSection{heading: sectionHeading("WHAT HAPPENED")}
	for _, it := range d.Happened {
		happened.items = append(happened.items, cardItem{name: it.Name, value: detailSaid(it.Value, m)})
	}
	tree[0] = []cardSection{happened}
	if d.Then != nil {
		tree[1] = []cardSection{thenSection(*d.Then, d.Row.At, m)}
	}
	if followed := followedSection(d.Followed, d.Row.At, m); len(followed.items) > 0 {
		tree[2] = []cardSection{followed}
	}
	return tree
}

// detailSaid is a value of an event's item in its inks, its accounts named
// as m names them: a value in text.secondary, a window's use bold in its
// level's colour, and the words around them muted.
func detailSaid(w views.DetailWords, m status.Moment) line {
	l := make(line, len(w))
	for i, p := range w {
		switch p.Kind {
		case views.DetailValue:
			l[i] = span{p.Text, secondaryInk}
		case views.DetailAccount:
			l[i] = span{named(m, p.Text), secondaryInk}
		case views.DetailUse:
			l[i] = span{p.Text, useInk(p.Use)}
		default:
			l[i] = span{p.Text, mutedInk}
		}
	}
	return l
}

// thenSection is how the account an event befell at at stood then, headed
// WORK THEN, the account white: its windows' use; when what the event tells
// of resets, where that's known; and how many sessions were on it.
func thenSection(t views.AccountThen, at time.Time, m status.Moment) cardSection {
	s := cardSection{heading: line{{strings.ToUpper(named(m, t.Account)), accountInk}, {" THEN", dimInk}}}
	for _, w := range t.Windows {
		s.items = append(s.items, cardItem{name: w.Name, value: line{windowUse(w.Use)}})
	}
	if !t.Resets.IsZero() {
		s.items = append(s.items, cardItem{name: "resets", value: valued(prose.Dated(at, t.Resets))})
	}
	s.items = append(s.items, cardItem{name: "sessions", value: line{{prose.Grouped(t.Sessions), secondaryInk}, {" on it", mutedInk}}})
	return s
}

// followedSection is WHAT FOLLOWED an event at at: of a move, its request,
// the tokens it cached again and what that cost; of a limit, a cap or a
// refusal, a row each session it moved, named by its id, the account it
// went to and what writing its context again was worth, three cells on;
// till when new sessions go elsewhere, or a family's requests, named by the
// family; and the moves' sum. No items where nothing followed.
func followedSection(f *views.Followed, at time.Time, m status.Moment) cardSection {
	s := cardSection{heading: sectionHeading("WHAT FOLLOWED")}
	if f == nil {
		return s
	}
	if move := f.Move; move != nil {
		s.items = append(s.items, cardItem{name: "request", value: valued(prose.Clean(move.Request))})
		if move.Written > 0 {
			s.items = append(s.items, cardItem{name: "cached again", value: valued(prose.Grouped(move.Written))})
		}
		if move.Cost != nil {
			s.items = append(s.items, cardItem{name: "the move", value: line{{views.Worth{Amount: move.Cost}.Cents(), alertInk}}})
		}
	}
	for _, move := range f.Moves {
		value := line{{"moved to ", mutedInk}, {named(m, move.To), secondaryInk}}
		costs := move.Move != nil && move.Move.Cost != nil
		if costs {
			value = append(value, spaces(movedApart), span{views.Worth{Amount: move.Move.Cost}.Cents(), warningInk})
		}
		s.items = append(s.items, cardItem{name: status.ShownID(move.Session), value: value, costs: costs})
	}
	if !f.Till.IsZero() {
		name := "new sessions"
		if f.Family != "" {
			name = f.Family
		}
		s.items = append(s.items, cardItem{name: name, value: valued("go elsewhere till " + prose.Dated(at, f.Till))})
	}
	if f.InAll != nil {
		s.items = append(s.items, cardItem{name: "in all", value: line{{views.Worth{Amount: f.InAll}.Cents(), alertInk}, {" to cache them again", mutedInk}}})
	}
	return s
}

// logTableRows are the rows the Log's table takes on the frame, from its
// first, under its heads, to its last.
func (f Frame) logTableRows() (first, last int) {
	first = f.top() + 1
	return first, first + f.LogRows() - 1
}

// logLitRow is the row the Log's line chosen is drawn on: false where none
// is chosen, or it's out of the table's view, or the page has no room for
// the table.
func (f Frame) logLitRow() (int, bool) {
	p := f.Log
	if f.cramped() || !p.Read {
		return 0, false
	}
	lines, s := p.lines(), p.stand()
	at, ok := p.lit(lines, s)
	first, last := p.window(lines, s, f.LogRows())
	if !ok || at < first || at > last {
		return 0, false
	}
	top, _ := f.logTableRows()
	return top + at - first, true
}

// cardLine is the row the line the details card is of is drawn on: on a
// session's page, where one is open, its turn open's request chosen, as
// turnLitRow has it; else the Log's line chosen, as logLitRow has it.
func (f Frame) cardLine() (int, bool) {
	if f.Shown() == SessionPage {
		return f.turnLitRow()
	}
	return f.logLitRow()
}

// cardRows are the rows a details card placed by its line sits within: on a
// session's page, where one is open, as turnCardRows has them; else the
// Log's table's.
func (f Frame) cardRows() (first, last int) {
	if f.Shown() == SessionPage {
		return f.turnCardRows()
	}
	return f.logTableRows()
}

// logCardSpan is the rows a card placed rows tall by its line on row y sits
// within: under its line, a row between, from there to the last of the rows
// cardRows gives, where it fits there, growing down; else over it, a row
// between, from their first to there, growing up. It's false where it fits
// on neither side.
func (f Frame) logCardSpan(y, placed int) (from, to int, up, ok bool) {
	first, last := f.cardRows()
	switch {
	case y+1+placed <= last:
		return y + 2, last, false, true
	case y-1-placed >= first:
		return first, y - 2, true, true
	}
	return 0, 0, false, false
}

// LogFitted is the Log as the frame shows it with the card c open over it,
// and c placed there, holding the rows it takes: where c fits by the line
// chosen on neither side, the table scrolled, the line coming up its view
// until c fits under it, its bottom edge on the table's last row, any rows
// past the paused foot left blank under it; as it stands where c fits by
// its line already, or where the table hasn't the rows for c under a line
// on its first row, as c is then centred over the page. Over a session's
// page, c is of a request of its turn open, by cardLine, and the Log stands
// as it is, as the page's turns lift for no card.
func (f Frame) LogFitted(c LogCard) (LogPage, LogCard) {
	y, ok := f.cardLine()
	c.Placed = len(c.body(f.logCardForm(), f.heldMoment())) + chrome(true)
	first, last := f.cardRows()
	to := last - 1 - c.Placed
	if _, _, _, fits := f.logCardSpan(y, c.Placed); !ok || fits || to < first || f.Shown() == SessionPage {
		return f.Log, c
	}
	return f.Log.lifted(y-first, to-first, f.LogRows()), c
}

// lifted is p, the table shown in a view rows rows tall scrolled down, so
// the line on its view's row from comes up to its row to.
func (p LogPage) lifted(from, to, rows int) LogPage {
	lines, s := p.lines(), p.stand()
	first, _ := p.window(lines, s, rows)
	s.top = lines[first+from-to].key
	return p.standing(s)
}

// Unlifted is p as the card that lifted it closes: the table shown, paused,
// its view in rows rows lifted past where scrolling reaches, ending at its
// paused foot again, the line chosen still in it; as it stands otherwise. A
// view of no rows is placed as placedRows has it.
func (p LogPage) Unlifted(rows int) LogPage {
	lines, s, rows := p.lines(), p.stand(), placedRows(rows)
	if !s.paused || len(lines) == 0 {
		return p
	}
	lowest := max(p.footOf(lines, s)-rows+1, 0)
	if first, _ := p.window(lines, s, rows); first <= lowest {
		return p
	}
	s.top = lines[lowest].key
	return p.standing(s)
}

// ChosenRequest is the Requests' line chosen, while Requests show and one is
// chosen: false otherwise.
func (p LogPage) ChosenRequest() (views.LogLine, bool) {
	if p.Shown != LogRequests {
		return views.LogLine{}, false
	}
	i, ok := p.lit(p.lines(), p.stand())
	if !ok || i >= len(p.Requests.Lines) {
		return views.LogLine{}, false
	}
	return p.Requests.Lines[i], true
}
