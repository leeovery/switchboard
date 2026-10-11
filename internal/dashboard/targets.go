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

// Targets are the zones of the frame's page a click acts on, as it's drawn.
// Until the rest are built, they're its tabs.
func (f Frame) Targets() []Target {
	x, y, gap := f.tabsAt()
	targets := make([]Target, 0, len(Views()))
	for _, v := range Views() {
		width := f.tab(v).width()
		targets = append(targets, Target{X: x, Y: y, Width: width, On: OnTab, Tab: v})
		x += width + gap
	}
	return targets
}
