package screen

import (
	"context"
	"errors"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/dashboard"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/setup"
)

// newPoolCard is the new pool card, open over the add card or the edit card
// that opened it, as the screen holds it: its label and its directories, as
// typed, one at least; the row chosen, its label's, a directory's, or past
// the last, + another directory; where setup refused a value, which, of the
// directories the one at dir among them, and why; and whether it's being
// checked, when it takes no key.
type newPoolCard struct {
	label    entry
	dirs     []entry
	chosen   int
	refused  setup.Field
	dir      int
	why      string
	checking bool
}

// newPoolOpened is the new pool card as it opens: holding the new pool held,
// where there's one, as it was given, else empty, with a directory to type;
// its label chosen.
func newPoolOpened(held *accounts.NewPool) *newPoolCard {
	c := &newPoolCard{}
	if held != nil {
		c.label = entryOf(held.Label)
		for _, d := range held.Dirs {
			c.dirs = append(c.dirs, entryOf(d))
		}
	}
	if len(c.dirs) == 0 {
		c.dirs = []entry{{}}
	}
	return c
}

// another is the row of + another directory, past the directories.
func (c newPoolCard) another() int {
	return len(c.dirs) + 1
}

// typedIn is the text field chosen, the label's or a directory's: nil where
// + another directory is.
func (c *newPoolCard) typedIn() (*entry, setup.Field, int) {
	switch {
	case c.chosen == 0:
		return &c.label, setup.PoolLabelField, 0
	case c.chosen < c.another():
		return &c.dirs[c.chosen-1], setup.DirsField, c.chosen - 1
	}
	return nil, setup.NoField, 0
}

// retyped has the text field chosen hold now, why setup refused its value
// unsaid once that's changed.
func (c newPoolCard) retyped(now entry) newPoolCard {
	field, of, dir := c.typedIn()
	if field == nil {
		return c
	}
	if c.refused == of && (of != setup.DirsField || c.dir == dir) && now.value() != field.value() {
		c.refused, c.why = setup.NoField, ""
	}
	*field = now
	return c
}

// pressed is the card once key is pressed in its row chosen, reporting false
// for a key it doesn't take: a text field's, as entry has them.
func (c newPoolCard) pressed(key tea.KeyPressMsg) (newPoolCard, bool) {
	field, _, _ := c.typedIn()
	if field == nil {
		return c, false
	}
	now, ok := field.pressed(key)
	return c.retyped(now), ok
}

// pasted is the card with text pasted into its row chosen, where it's a
// text field.
func (c newPoolCard) pasted(text string) newPoolCard {
	if field, _, _ := c.typedIn(); field != nil {
		return c.retyped(field.pasted(text))
	}
	return c
}

// pool is the pool the card gives, its label as typed, and each directory
// typed, trimmed, those left empty passed over; and the place among the
// card's directories of each it gives.
func (c newPoolCard) pool() (accounts.NewPool, []int) {
	pool := accounts.NewPool{Label: c.label.value()}
	var at []int
	for i, d := range c.dirs {
		if dir := strings.TrimSpace(d.value()); dir != "" {
			pool.Dirs, at = append(pool.Dirs, dir), append(at, i)
		}
	}
	return pool, at
}

// drawn is the card as dashboard.Setup draws it.
func (c newPoolCard) drawn() *dashboard.SetupNewPoolCard {
	dirs := make([]dashboard.Field, len(c.dirs))
	for i, d := range c.dirs {
		dirs[i] = d.field(c.chosen == i+1)
	}
	return &dashboard.SetupNewPoolCard{
		Label: c.label.field(c.chosen == 0), Dirs: dirs, Chosen: c.chosen,
		Refused: c.refused, Dir: c.dir, Why: c.why,
	}
}

// newPoolPressed acts on a key at the new pool card: esc closes it, back to
// the card that opened it, holding what it held; tab chooses the next row,
// the first after the last; enter on + another directory adds a directory
// to type, chosen, and elsewhere has setup check the pool, with CheckPool,
// the card taking no key till it's checked; and the row chosen takes the
// keys it takes, as newPoolCard.pressed has them.
func (m Model) newPoolPressed(key tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	card := *m.newPool
	switch key.String() {
	case "esc":
		m.newPool = nil
		return m, nil
	case "tab":
		card.chosen = (card.chosen + 1) % (card.another() + 1)
	case "enter":
		if card.chosen == card.another() {
			card.dirs = append(card.dirs, entry{})
			card.chosen = len(card.dirs)
			break
		}
		card.checking = true
		m.newPool = &card
		pool, _ := card.pool()
		return m, m.jobs.read(checkPool(m.steps, pool))
	default:
		if now, ok := card.pressed(key); ok {
			card = now
		}
	}
	m.newPool = &card
	return m, nil
}

// poolCheckedMsg is what CheckPool answered, checking the new pool card's
// pool.
type poolCheckedMsg struct {
	pool accounts.NewPool
	err  error
}

// checkPool is the job that checks the pool given, with CheckPool.
func checkPool(steps Steps, pool accounts.NewPool) job {
	return func(context.Context) tea.Msg {
		return poolCheckedMsg{pool: pool, err: steps.CheckPool(pool)}
	}
}

// poolChecked takes in what CheckPool answered, while the new pool card is
// open: where setup refused a value, why, said under its row, the card
// staying open; where it failed otherwise, the step failed, not checked,
// every card closed; else the card closed, and the card that opened it
// holding the pool, chosen.
func (m Model) poolChecked(msg poolCheckedMsg) (Model, tea.Cmd) {
	if m.newPool == nil {
		return m, nil
	}
	card := *m.newPool
	card.checking = false
	if refused, ok := errors.AsType[*setup.Refused](msg.err); ok {
		_, at := card.pool()
		card.refused, card.why = refused.Field, prose.Clean(refused.Why)
		if refused.Field == setup.DirsField && refused.Dir < len(at) {
			card.dir = at[refused.Dir]
		}
		m.newPool = &card
		return m, nil
	}
	m.newPool = nil
	if msg.err != nil {
		m.add, m.edit = nil, nil
		m.walk, m.accountsFailed = m.walk.finished(msg.err), dashboard.AccountsNotChecked
		return m, nil
	}
	switch {
	case m.add != nil:
		add := *m.add
		held := add.pool.holding(msg.pool)
		add.pool = &held
		add.form = add.unrefused(setup.PoolField)
		m.add = &add
	case m.edit != nil:
		edit := *m.edit
		held := edit.pool.holding(msg.pool)
		edit.pool = &held
		edit.form = edit.unrefused(setup.PoolField)
		m.edit = &edit
	}
	return m, nil
}
