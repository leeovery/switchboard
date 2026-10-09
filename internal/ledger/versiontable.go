package ledger

import (
	"slices"
	"strings"
)

// Version is a model version as the views name it: a row of the version
// table.
type Version struct {
	// IDs are the ids requests ask for it by, as the price table holds them.
	IDs []string
	// Name is its name as the views show it, as Opus 5.5.
	Name string
	// Family is its family, as internal/claude reads it from its ids, as
	// opus.
	Family string
}

// Version returns the version the table names with the id given, reporting
// false for one it doesn't name.
func (t Table) Version(id string) (Version, bool) {
	i := slices.IndexFunc(t.Versions, func(v Version) bool { return slices.Contains(v.IDs, id) })
	if i < 0 {
		return Version{}, false
	}
	return t.Versions[i], true
}

// VersionName returns the name of the version with the id given as the views
// show it: the table's, as Opus 5.5, else its id, claude- taken off, as
// opus-6.
func (t Table) VersionName(id string) string {
	if v, ok := t.Version(id); ok {
		return v.Name
	}
	return ShortModelID(id)
}

// ShortModelID returns a model's id as the views show it where they name a
// model by its id: claude- taken off, as opus-5-5.
func ShortModelID(id string) string {
	return strings.TrimPrefix(id, "claude-")
}
