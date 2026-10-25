package dashboard

import "slices"

// toggledSection is a section of a page whose keys show and hide its
// sections, as the Overview's and Graphs' do: its place among the page's,
// in the order the page draws them, whose words the page's list holds.
type toggledSection interface {
	~int
	list() sectionList
}

// sectionList are a page's sections' words, each at its section's place, in
// the order the page draws them, and those shown at first, a bit each.
type sectionList struct {
	words []sectionWording
	first uint8
}

// sectionWording are a section's key; its word, as the key line words it
// after its key; its heading's name; and its name in the preferences file.
type sectionWording struct {
	key, word, heading, name string
}

// wordingOf are the section's words, as its page's list has them.
func wordingOf[S toggledSection](s S) sectionWording {
	return s.list().words[s]
}

// everyOf are the page's sections, in the order the page draws them.
func everyOf[S toggledSection]() []S {
	var s S
	every := make([]S, len(s.list().words))
	for i := range every {
		every[i] = S(i)
	}
	return every
}

// keyedOf is the page's section the key given shows and hides, reporting
// false for a key that's none of its sections'.
func keyedOf[S toggledSection](key string) (S, bool) {
	for _, s := range everyOf[S]() {
		if wordingOf(s).key == key {
			return s, true
		}
	}
	return 0, false
}

// shown are which of a page's sections show. The zero value shows those
// shown at first.
type shown[S toggledSection] struct {
	// toggled are the sections shown otherwise than at first, a bit each,
	// so the zero value shows those shown at first.
	toggled uint8
}

// first are the sections shown at first, a bit each.
func (s shown[S]) first() uint8 {
	var section S
	return section.list().first
}

// Has reports whether the section shows.
func (s shown[S]) Has(section S) bool {
	return (s.first()^s.toggled)&(1<<section) != 0
}

// Toggled is s with the section hidden where it shows, and shown where it's
// hidden.
func (s shown[S]) Toggled(section S) shown[S] {
	return shown[S]{toggled: s.toggled ^ 1<<section}
}

// Names are the names of the sections that show, in the order the page draws
// them, as the preferences file keeps them: never nil, so none showing is
// kept as none.
func (s shown[S]) Names() []string {
	names := []string{}
	for _, section := range everyOf[S]() {
		if s.Has(section) {
			names = append(names, wordingOf(section).name)
		}
	}
	return names
}

// Toggles are the sections' keys, as the key line lists them, in the order
// the page draws them: each key and its word, its word faint where its
// section is hidden.
func (s shown[S]) Toggles() []Key {
	every := everyOf[S]()
	toggles := make([]Key, 0, len(every))
	for _, section := range every {
		words := wordingOf(section)
		toggles = append(toggles, Key{Key: words.key, Does: words.word, Hidden: !s.Has(section)})
	}
	return toggles
}

// none reports whether every section is hidden.
func (s shown[S]) none() bool {
	return !slices.ContainsFunc(everyOf[S](), s.Has)
}

// shownNamed are the sections the names given show, as the preferences file
// keeps them, a name of none of them left out, as a later switchboard's: or,
// given nil, as a file that keeps none has it, those shown at first.
func shownNamed[S toggledSection](names []string) shown[S] {
	var s shown[S]
	if names == nil {
		return s
	}
	var named uint8
	for _, section := range everyOf[S]() {
		if slices.Contains(names, wordingOf(section).name) {
			named |= 1 << section
		}
	}
	return shown[S]{toggled: named ^ s.first()}
}
