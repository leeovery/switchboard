package dashboard

import (
	"cmp"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/theme"
)

// SetupEditCard is an account's edit card, open over the Accounts page, as
// it stands: the account's name; its fields, Label and Reserve as typed, Plan
// the plan chosen of Plans, those it offers, its Pool, where it has one, and
// Primary whether yes is chosen, PrimaryHint its hint, "" for the card's
// own; the field chosen; and where setup refused a value, why, as Why says,
// under the field Refused names.
type SetupEditCard struct {
	Name           string
	Label, Reserve Field
	Plans          []string
	Plan           string
	Pool           *SetupPool
	Primary        bool
	PrimaryHint    string
	Chosen         setup.Field
	Refused        setup.Field
	Why            string
}

// SetupPool is a card's Pool, as it stands: its options, each pool by its
// label, then new pool; the one chosen, by its place among them; the hint
// under it, as setup words it; and whether new pool is chosen, where enter
// opens the new pool card, whichever field is chosen.
type SetupPool struct {
	Options []string
	Chosen  int
	Hint    string
	New     bool
}

// SetupNewPoolCard is the new pool card, open over the add card or the edit
// card that opened it, as it stands: its Label and its Dirs, as typed; the
// row chosen, its label's, a directory's, or past the last, + another
// directory; and where setup refused a value, why, as Why says, under the
// field Refused names, of the directories, the one at Dir.
type SetupNewPoolCard struct {
	Label   Field
	Dirs    []Field
	Chosen  int
	Refused setup.Field
	Dir     int
	Why     string
}

// SetupReserveCard is the primary's reserve card, open over the Accounts
// page on a first run, as it stands: the primary's name; its Reserve, typed
// in as the screen has it; and where setup refused it, why.
type SetupReserveCard struct {
	Name    string
	Reserve Field
	Why     string
}

// SetupAddCard is the card adding an account, open over the Accounts page,
// as it stands: its fields, Label as typed, Plan the plan chosen of Plans,
// those it offers, "" for none, and its Pool, where it has one; the field chosen; and
// where setup refused a value, why, as Why says, under the field Refused
// names.
type SetupAddCard struct {
	Label   Field
	Plans   []string
	Plan    string
	Pool    *SetupPool
	Chosen  setup.Field
	Refused setup.Field
	Why     string
}

// SetupWhichCard is the card asking which account a token made, or pasted,
// to add an account is for, open over the Accounts page, where its
// organization doesn't settle it, as it stands: the label the account's
// added with; what setup asks, as it words it; and the choice chosen, by
// its place among Ask's choices.
type SetupWhichCard struct {
	Label  string
	Ask    setup.WhichAsk
	Chosen int
}

// SetupRemoveCard is the card asking before an account is removed, open over
// the Accounts page: the account's name, and why setup refused to remove it,
// where it did.
type SetupRemoveCard struct {
	Name, Refused string
}

// setupCardWidth is the natural width of setup's cards, frame 6's, their
// edges included.
const setupCardWidth = 80

// What the edit card and the add card say: their fields' names and their
// hints.
const (
	labelName   = "Label"
	labelHint   = "what every screen calls it"
	planName    = "Plan"
	planHint    = "used by History to value usage"
	primaryName = "Primary"
	primaryHint = "the account the Claude apps are signed in to"
	reserveName = "Reserve"
	reserveHint = "usage kept back for the Claude apps"
	poolName    = "Pool"
	dirName     = "Directory"
	dirHint     = "a session started here, or in any folder inside, is in the pool"
	anotherDir  = "another directory"
)

// newPoolSays is what the new pool card says of a pool, over its fields,
// muted, in the lines the card breaks it into.
var newPoolSays = []line{
	{{"A pool's accounts serve only the sessions started in its directories,", mutedInk}},
	{{"and those sessions go out on its accounts alone.", mutedInk}},
}

// What the add card says: what an account is, over its fields.
const addCardIntro = "Each account is one Claude subscription: a Pro, Max or Ultra plan."

// addTokenSays is what the add card says of the token setup makes next, in
// text.secondary, in the lines the card breaks it into.
var addTokenSays = []line{
	{{"Setup then creates a token with claude setup-token. You approve it in", secondaryInk}},
	{{"the browser while signed in to this subscription. Tokens last a year.", secondaryInk}},
}

// Primary's choices, and the edit card's keys, the add card's, the remove
// card's and the reserve card's.
var (
	primaryChoices = []string{"no", "yes"}
	editKeys       = []Key{{Key: "tab", Does: "next"}, {Key: "←→", Does: "select"}, {Key: "enter", Does: "save"}, {Key: "esc", Does: "cancel"}}
	addKeys        = []Key{{Key: "tab", Does: "next"}, {Key: "enter", Does: "create token"}, {Key: "esc", Does: "cancel"}}
	newPoolKeys    = []Key{{Key: "tab", Does: "next"}, {Key: "enter", Does: "create pool"}, {Key: "esc", Does: "back"}}
	makingPoolKey  = Key{Key: "enter", Does: "new pool"}
	addingDirKey   = Key{Key: "enter", Does: "add a directory"}
	whichKeys      = []Key{{Key: "↑↓", Does: "select"}, {Key: "enter", Does: "save"}, {Key: "esc", Does: "cancel"}}
	removeKeys     = []Key{{Key: "enter", Does: "remove"}, {Key: "esc", Does: "keep it"}}
	reserveKeys    = []Key{{Key: "enter", Does: "save"}, {Key: "esc", Does: "no reserve"}}
)

// What the remove card says of the account it would remove, after its name.
const (
	removeSays  = " is taken out of the config, and its token file deleted, so no requests are routed to it."
	removeKeeps = "Its Claude subscription is left as it is."
)

// setupCard is a card of setup's, as the dashboard's Card draws it, but over
// the page as it is, unfaded; and its fields, each drawn over the row of its
// body it's on.
type setupCard struct {
	card   Card
	fields []cardField
}

// cardField is a field of a card's, on the row of its body given, from the
// cell of its content given.
type cardField struct {
	row, at int
	field   textField
}

// card is the card open over the page of the step showing, at the screen's
// time: the Accounts page's, as accountsCard has it, or the claude link
// page's offer card; nil where none is.
func (s Setup) card(width int) *setupCard {
	switch s.At {
	case SetupAccounts:
		return s.accountsCard(width)
	case SetupLink:
		return s.offerCard(width)
	}
	return nil
}

// accountsCard is the card open over the Accounts page: the new pool card,
// over the add card or the edit card that opened it; its add card, its
// edit card, its remove card, its token card, its paste card, the card
// asking which account a token is for, or the primary's reserve card, at the
// screen's time; nil where none is.
func (s Setup) accountsCard(width int) *setupCard {
	if s.Accounts == nil {
		return nil
	}
	cardWidth, _ := cardAcross(width, setupCardWidth)
	switch a := s.Accounts; {
	case a.NewPool != nil:
		return a.NewPool.card(cardWidth)
	case a.Add != nil:
		return a.Add.card(cardWidth)
	case a.Edit != nil:
		return a.Edit.card(cardWidth)
	case a.Remove != nil:
		return a.Remove.card(cardWidth)
	case a.Token != nil:
		return a.Token.card(cardWidth, s.Now)
	case a.Paste != nil:
		return a.Paste.card(cardWidth, s.Now)
	case a.Which != nil:
		return a.Which.card(cardWidth)
	case a.Reserve != nil:
		return a.Reserve.card(cardWidth)
	}
	return nil
}

// cardSpins reports whether the card open over the page of the step
// showing has its spinner moving: the token card, but at its making's end,
// the paste card while it checks the token, and the offer card while it adds
// the line.
func (s Setup) cardSpins() bool {
	switch {
	case s.At == SetupLink:
		return s.Linking.Offer != nil && s.Linking.Offer.Adding
	case s.At != SetupAccounts || s.Accounts == nil:
		return false
	}
	switch a := s.Accounts; {
	case a.Token != nil:
		return a.Token.Stage.spins()
	case a.Paste != nil:
		return a.Paste.Checking
	}
	return false
}

// tokenCard is the token card open over the page of the step showing: nil
// where none is.
func (s Setup) tokenCard() *SetupTokenCard {
	if s.At != SetupAccounts || s.Accounts == nil {
		return nil
	}
	return s.Accounts.Token
}

// draw draws the card over what's drawn on c, as Card.draw draws it, then
// each of its fields its body shows, over its row.
func (k setupCard) draw(c *canvas) {
	k.card.draw(c)
	x, y, _, _ := k.card.box(c.width, c.height())
	shown := len(k.card.shown(c.height()))
	for _, f := range k.fields {
		if f.row < shown {
			f.field.draw(c, x+cardInset+f.at, y+2+f.row, k.card.width-2*cardInset-f.at)
		}
	}
}

// card is the edit card, width cells wide: EDIT and the account's name in its
// top edge; then a blank row apart, its fields, Label, Plan and its hint,
// Pool and its hint, where it has one, Primary and its hint, and Reserve,
// its hint after it, each refusal under the field it's of; and its keys,
// enter opening the new pool card where Pool is on new pool.
func (e SetupEditCard) card(width int) *setupCard {
	content := width - 2*cardInset
	focus := cardFocus{chosen: e.Chosen, refused: e.Refused, why: e.Why}
	k := &setupCard{card: Card{title: "EDIT " + strings.ToUpper(e.Name), width: width, keys: poolKeys(editKeys, e.Pool)}}
	k.text(focus.textField(setup.LabelField, labelName, "", e.Label), 0, content)
	primary := 0
	if e.Primary {
		primary = 1
	}
	k.card.body = append(k.card.body, nil, focus.planRow(content, e.Plans, e.Plan), hintUnder(content, planHint), nil)
	k.card.body = append(k.card.body, focus.poolRows(content, e.Pool)...)
	k.card.body = append(k.card.body,
		optionsRow(content, primaryName, primaryChoices, primary, e.Chosen == setup.PrimaryField),
		hintUnder(content, cmp.Or(e.PrimaryHint, primaryHint)),
		nil,
	)
	k.text(focus.textField(setup.ReserveField, reserveName, reserveHint, e.Reserve), 0, content)
	return k
}

// poolRows are a card's Pool, width cells wide, and the blank row after it,
// where it has one: its options, the one chosen bold where Pool is the field
// chosen; its hint under it; and why setup refused the pool chosen, where it
// did, under that. None where it has no Pool.
func (f cardFocus) poolRows(width int, pool *SetupPool) []line {
	if pool == nil {
		return nil
	}
	rows := []line{optionsRow(width, poolName, pool.Options, pool.Chosen, f.chosen == setup.PoolField), hintUnder(width, pool.Hint)}
	return append(append(rows, f.whyUnder(setup.PoolField, width)...), nil)
}

// whyUnder is why setup refused the value of the choice of the field given,
// where it did, on the rows under it, width cells wide, from its value's
// column: none where it didn't.
func (f cardFocus) whyUnder(of setup.Field, width int) []line {
	if f.refused != of {
		return nil
	}
	var rows []line
	for _, l := range (line{{f.why, errorInk}}).wrap(width - valueAt(width)) {
		rows = append(rows, append(line{spaces(valueAt(width))}, l...))
	}
	return rows
}

// poolKeys are a card's keys, given, but where Pool is on new pool, for
// enter, which opens the new pool card.
func poolKeys(keys []Key, pool *SetupPool) []Key {
	if pool == nil || !pool.New {
		return keys
	}
	keys = slices.Clone(keys)
	keys[slices.IndexFunc(keys, func(k Key) bool { return k.Key == makingPoolKey.Key })] = makingPoolKey
	return keys
}

// card is the primary's reserve card, width cells wide, its words setup's:
// RESERVE and the primary's name in its top edge; what the primary is and
// what its reserve is, in text.secondary; then a blank row apart, its
// Reserve, as it's given, its hint after it, as the edit card's, why setup
// refused it under it, and what it takes under that, faint; a blank row on,
// where the other accounts' reserves are set, muted; and its keys.
func (r SetupReserveCard) card(width int) *setupCard {
	content := width - 2*cardInset
	asked := setup.AskingReserve(r.Name)
	k := &setupCard{card: Card{title: "RESERVE · " + strings.ToUpper(r.Name), width: width, keys: reserveKeys}}
	for _, says := range asked.Says {
		k.card.body = append(k.card.body, line{{says, secondaryInk}}.wrap(content)...)
	}
	k.card.body = append(k.card.body, nil)
	k.text(textField{name: reserveName, hint: reserveHint, wrong: r.Why, field: r.Reserve}, 0, content)
	k.card.body = append(k.card.body, hintUnder(content, asked.Takes), nil)
	k.card.body = append(k.card.body, line{{asked.Elsewhere, mutedInk}}.wrap(content)...)
	return k
}

// card is the add card, width cells wide: ADD AN ACCOUNT in its top edge;
// what an account is, muted; then a blank row apart, its fields, Label, its
// hint after it, Plan, and Pool and its hint, where it has one, each refusal
// under the field it's of; what setup does next, making the account's
// token; and its keys, enter opening the new pool card where Pool is on new
// pool.
func (a SetupAddCard) card(width int) *setupCard {
	content := width - 2*cardInset
	focus := cardFocus{chosen: a.Chosen, refused: a.Refused, why: a.Why}
	k := &setupCard{card: Card{title: "ADD AN ACCOUNT", width: width, keys: poolKeys(addKeys, a.Pool)}}
	k.card.body = append(line{{addCardIntro, mutedInk}}.wrap(content), nil)
	k.text(focus.textField(setup.LabelField, labelName, labelHint, a.Label), 0, content)
	k.card.body = append(k.card.body, nil, focus.planRow(content, a.Plans, a.Plan))
	k.card.body = append(append(k.card.body, focus.whyUnder(setup.PlanField, content)...), nil)
	k.card.body = append(k.card.body, focus.poolRows(content, a.Pool)...)
	for _, l := range addTokenSays {
		k.card.body = append(k.card.body, l.wrap(content)...)
	}
	return k
}

// card is the card asking which account a token is for, width cells wide,
// its words setup's: WHICH ACCOUNT and the label the account's added with in
// its top edge; why it asks, in text.secondary; then a blank row apart, its
// choices, a row each, laid out as choiceRows has them; and its keys.
func (w SetupWhichCard) card(width int) *setupCard {
	content := width - 2*cardInset
	k := &setupCard{card: Card{title: "WHICH ACCOUNT · " + strings.ToUpper(w.Label), width: width, keys: whichKeys}}
	for _, says := range w.Ask.Says {
		k.card.body = append(k.card.body, line{{says, secondaryInk}}.wrap(content)...)
	}
	k.card.body = append(append(k.card.body, nil), choiceRows(content, w.Ask.Choices, w.Chosen)...)
	return k
}

// choiceRows are a list of choices, width cells wide, a row each: ■ before
// the one chosen, its label in text.primary, bold, and □ before the rest,
// their labels muted, each mark in accent.key, as optionsRow marks a
// choice's options; and when each was last seen, where it says, faint, in a
// column choicesApart cells past the widest label, a label cut short to
// leave it room.
func choiceRows(width int, choices []setup.WhichChoice, chosen int) []line {
	const markWidth = 2
	labels, seen := 0, 0
	for _, c := range choices {
		labels = max(labels, line{{c.Label, mutedInk}}.width())
		seen = max(seen, line{{c.Seen, faintInk}}.width())
	}
	labels = min(labels, width-markWidth-choicesApart-seen)
	rows := make([]line, len(choices))
	for i, c := range choices {
		mark, k := "□ ", mutedInk
		if i == chosen {
			mark, k = "■ ", ink{token: theme.TextPrimary, bold: true}
		}
		label := line{{c.Label, k}}.fit(labels)
		row := append(line{{mark, choiceMarkInk}}, label...)
		if c.Seen != "" {
			row = append(row, spaces(labels-label.width()+choicesApart), span{c.Seen, faintInk})
		}
		rows[i] = row
	}
	return rows
}

// cardFocus is where a card of setup's stands among its fields: the field
// chosen; and where setup refused a value, why, under the field refused
// names.
type cardFocus struct {
	chosen, refused setup.Field
	why             string
}

// textField is the card's text field of the field given, named and hinted as
// given, typed in where it's the one chosen, saying why setup refused its
// value, where it did.
func (f cardFocus) textField(of setup.Field, name, hint string, value Field) textField {
	field := textField{name: name, hint: hint, field: value}
	field.field.Typing = f.chosen == of
	if f.refused == of {
		field.wrong = f.why
	}
	return field
}

// planRow is the card's Plan, width cells wide, its options plans, plan the
// one chosen, bold where Plan is the field chosen.
func (f cardFocus) planRow(width int, plans []string, plan string) line {
	return optionsRow(width, planName, planNames(plans), slices.Index(plans, plan), f.chosen == setup.PlanField)
}

// card is the new pool card, width cells wide: NEW POOL in its top edge;
// what a pool is, muted; then a blank row apart, its Label, its hint after
// it; a blank row on, its directories, the first named Directory, its hint
// under it; then + another directory, chosen as the Accounts page's + add an
// account is; each refusal under the field it's of; and its keys, enter
// adding a directory where + another directory is chosen.
func (n SetupNewPoolCard) card(width int) *setupCard {
	content := width - 2*cardInset
	focus := cardFocus{chosen: setup.NoField, refused: n.Refused, why: n.Why}
	if n.Chosen == 0 {
		focus.chosen = setup.PoolLabelField
	}
	keys := newPoolKeys
	if n.Chosen > len(n.Dirs) {
		keys = slices.Clone(keys)
		keys[1] = addingDirKey
	}
	k := &setupCard{card: Card{title: "NEW POOL", width: width, keys: keys}}
	for _, l := range newPoolSays {
		k.card.body = append(k.card.body, l.wrap(content)...)
	}
	k.card.body = append(k.card.body, nil)
	k.text(focus.textField(setup.PoolLabelField, labelName, labelHint, n.Label), 0, content)
	k.card.body = append(k.card.body, nil)
	for i, dir := range n.Dirs {
		f := textField{field: dir}
		f.field.Typing = n.Chosen == i+1
		if i == 0 {
			f.name = dirName
		}
		if n.Refused == setup.DirsField && n.Dir == i {
			f.wrong = n.Why
		}
		k.text(f, 0, content)
		if i == 0 {
			k.card.body = append(k.card.body, hintUnder(content, dirHint))
		}
	}
	k.card.body = append(k.card.body, anotherDirRow(content, n.Chosen > len(n.Dirs)))
	return k
}

// anotherDirRow is the new pool card's + another directory, width cells
// wide, from its value's column: faint, as a hint; or chosen, + bold
// accent.key and its words text.primary, as the Accounts page's + add an
// account is chosen.
func anotherDirRow(width int, chosen bool) line {
	if !chosen {
		return line{spaces(valueAt(width)), {"+ " + anotherDir, faintInk}}
	}
	return line{spaces(valueAt(width)), {"+", ink{token: theme.AccentKey, bold: true}}, {" " + anotherDir, ink{token: theme.TextPrimary}}}
}

// text puts f on the card's next row of its body, content cells wide, from
// the cell at, and the rows under it saying what's wrong with its value,
// where anything is.
func (k *setupCard) text(f textField, at, content int) {
	k.fields = append(k.fields, cardField{row: len(k.card.body), at: at, field: f})
	k.card.body = append(k.card.body, make([]line, 1+len(f.wrongLines(content-at)))...)
}

// card is the remove card, width cells wide: REMOVE and the account's name in
// its top edge; what removing it does, and what it leaves; why setup
// refused to remove it, where it did; and its keys.
func (r SetupRemoveCard) card(width int) *setupCard {
	k := &setupCard{card: Card{title: "REMOVE " + strings.ToUpper(r.Name), width: width, keys: removeKeys}}
	content := width - 2*cardInset
	says := line{{r.Name, titleInk}, {removeSays, secondaryInk}}
	k.card.body = append(says.wrap(content), line{{removeKeeps, mutedInk}})
	if r.Refused != "" {
		k.card.body = append(append(k.card.body, nil), line{{r.Refused, errorInk}}.wrap(content)...)
	}
	return k
}

// choiceMarkInk is the ink of the mark before each of a choice's options.
var choiceMarkInk = ink{token: theme.AccentKey}

// optionsRow is a form's row of a choice, width cells wide: its name, in
// text.subtle, then from its value's column, laid out as a text field's,
// its options, four cells apart, ■ before the one chosen, in text.primary,
// bold where the choice is the field chosen, and □ before the rest, muted,
// each mark in accent.key.
func optionsRow(width int, name string, options []string, chosen int, active bool) line {
	l := line{{name, dimInk}}.fit(fieldParts(width)[fieldName].Size)
	l = append(l, spaces(valueAt(width)-l.width()))
	for i, option := range options {
		if i > 0 {
			l = append(l, spaces(choicesApart))
		}
		mark, k := "□ ", mutedInk
		if i == chosen {
			mark, k = "■ ", ink{token: theme.TextPrimary, bold: active}
		}
		l = append(l, span{mark, choiceMarkInk}, span{option, k})
	}
	return l
}

// choicesApart is the cells between a choice's options.
const choicesApart = 4

// hintUnder is a field's hint, on the row under it, width cells wide, from
// its value's column, faint.
func hintUnder(width int, hint string) line {
	return line{spaces(valueAt(width)), {hint, faintInk}}
}

// planNames are the plans as an edit offers them: each by its name, as
// setup.PlanName has it, or for none, not set.
func planNames(plans []string) []string {
	names := make([]string, len(plans))
	for i, p := range plans {
		names[i] = setup.PlanName(p)
		if names[i] == "" {
			names[i] = notSet
		}
	}
	return names
}
