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
	// Family is its family, as FamilyOf reads it from its ids, as opus.
	Family string
	// Shade is its shade among the versions' six, viz.version.1 to
	// viz.version.6, as the table assigns them, a family's newest first: 0
	// for none.
	Shade int
	// Older is whether the table holds a newer version of its family: where
	// it has no shade, it's drawn in its family's hue mixed toward the canvas.
	Older bool
}

// families are the families Claude's models come in, in the order the
// version table lists them: the one list the router reads a model's family
// by, and the version table's rows are of.
var families = []string{"opus", "sonnet", "fable", "haiku"}

// Families returns the families Claude's models come in, in the order the
// version table lists them.
func Families() []string {
	return slices.Clone(families)
}

// FamilyOf returns the family a model belongs to, such as "opus" for
// claude-opus-5-5, or the model's own id when it names none of the
// families. A model's id names its family as one of its hyphenated parts,
// wherever it falls: both claude-opus-5-5 and claude-3-5-haiku-20241022 do.
// Models of a family are counted against the same windows.
func FamilyOf(model string) string {
	for part := range strings.SplitSeq(model, "-") {
		if slices.Contains(families, part) {
			return part
		}
	}
	return model
}

// familyShades are the shades the table assigns each family's versions,
// newest first: two each to Opus and Sonnet, and one each to Fable and
// Haiku.
var familyShades = map[string][]int{"opus": {1, 2}, "sonnet": {3, 4}, "fable": {5}, "haiku": {6}}

// shaded is the table's rows, by family and within a family newest first,
// each given its shade and whether it's older than its family's newest, so a
// version added to the table takes them afresh.
func shaded(rows []Version) []Version {
	seen := make(map[string]int)
	for i, v := range rows {
		nth := seen[v.Family]
		seen[v.Family]++
		rows[i].Older = nth > 0
		if shades := familyShades[v.Family]; nth < len(shades) {
			rows[i].Shade = shades[nth]
		}
	}
	return rows
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
