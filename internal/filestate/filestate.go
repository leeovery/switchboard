// Package filestate tells whether a file has changed: how one stands, its
// links followed, as which file it is and when it last changed, compared
// with how it stood before. The one rule serves the router, looking after
// the config file and the binary it was started from, and the dashboard,
// following the config it reads.
package filestate

import (
	"io/fs"
	"os"
)

// State is how a file stands: which file it is, and when it last changed.
// The zero State is a file that isn't there.
type State struct {
	info fs.FileInfo
}

// Of returns how the file at path stands now, its links followed.
func Of(path string) State {
	info, err := os.Stat(path)
	if err != nil {
		return State{}
	}
	return State{info: info}
}

// Info is how a file stood as info was read of it: a file that isn't there
// for none.
func Info(info fs.FileInfo) State {
	return State{info: info}
}

// There reports whether the file is there.
func (s State) There() bool {
	return s.info != nil
}

// MovedTo reports whether the path s was found at has moved on to t: t is
// there, and another file than s, or s changed. A path that leads nowhere, as
// it may for a moment while an upgrade moves a link on, or an editor saves a
// file, hasn't.
func (s State) MovedTo(t State) bool {
	return t.There() && !t.Same(s)
}

// Same reports whether s and t are the same file, unchanged, or both not
// there.
func (s State) Same(t State) bool {
	if !s.There() || !t.There() {
		return s.There() == t.There()
	}
	return os.SameFile(s.info, t.info) && s.info.ModTime().Equal(t.info.ModTime())
}
