package dashboard

// LogPanel is the Log's panel of what its table shows, as f opens it over
// the page: none yet, the panel being built after the page.
type LogPanel struct{}

// card is the card f draws of it.
func (LogPanel) card(Frame) Card {
	return Card{}
}
