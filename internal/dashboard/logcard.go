package dashboard

// LogCard is the Log's details card of the line chosen, as enter opens it
// over the page: none yet, the card being built after the page.
type LogCard struct{}

// card is the card f draws of it.
func (LogCard) card(Frame) Card {
	return Card{}
}
