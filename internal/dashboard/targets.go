package dashboard

// Target is a zone of the page a click acts on, from X along row Y, Width
// cells wide, and what's there, as On says: a tab, Tab; a sub-tab, by its
// place among them, SubTab; < or > by a period or a window, Step, -1 or 1; a
// key of the key line, Key, as the line names it; or a row of the view,
// Row, by its index.
type Target struct {
	X, Y, Width int
	On          Clicked
	Tab         View
	SubTab      int
	Step        int
	Key         string
	Row         int
}

// Clicked is what a click lands on.
type Clicked int

const (
	// OnTab is a tab in the title row.
	OnTab Clicked = iota + 1
	// OnSubTab is a sub-tab in the heading row.
	OnSubTab
	// OnStep is < or > by a period or a window.
	OnStep
	// OnKey is a key of the key line.
	OnKey
	// OnRow is a row of the view.
	OnRow
)

// Targets are the zones of the frame's page a click acts on, each over the
// cells it's drawn on, laid out as the page draws them: its tabs; the
// heading row's sub-tabs, and < and > by its period or window; and the key
// line's keys, as keyTargets has them.
func (f Frame) Targets() []Target {
	targets := f.tabTargets()
	for _, p := range f.headingPieces() {
		if p.click.On != 0 {
			targets = append(targets, p.target())
		}
	}
	return append(targets, f.keyTargets()...)
}

// tabTargets are the tabs, as tabs draws them.
func (f Frame) tabTargets() []Target {
	x, y, gap := f.tabsAt()
	targets := make([]Target, 0, len(Views()))
	for _, v := range Views() {
		width := f.tab(v).width()
		targets = append(targets, Target{X: x, Y: y, Width: width, On: OnTab, Tab: v})
		x += width + gap
	}
	return targets
}

// keyTargets are the key line's keys, as keyLine draws them, but for p pin,
// as no click opens the routing card, and q quit: none while a note takes
// the line's place, or where it has no row of its own.
func (f Frame) keyTargets() []Target {
	y := f.Height - 1
	if y < f.titleRows() || f.KeyLine.Note != "" {
		return nil
	}
	var targets []Target
	for _, k := range f.keysAt() {
		if k.key.Key == "p" || k.key.Key == "q" {
			continue
		}
		targets = append(targets, Target{X: k.x, Y: y, Width: k.key.words().width(), On: OnKey, Key: k.key.Key})
	}
	return targets
}
