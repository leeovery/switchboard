package dashboard

// sessionStory is the session's STORY as the page l lays out draws it, a
// line a row from its first: blank until its drawer lands, which takes this
// stub's place.
func (f Frame) sessionStory(_ *SessionShown, l sessionLayout) []line {
	return make([]line, l.storyRows())
}
