package ledger

import "testing"

// A version added to the table takes the shades afresh: a newer Opus takes
// viz.version.1, and Opus 5.5 and Opus 5 move down one, Opus 5 to none; and
// a family with one version has none older.
func TestAVersionAddedToTheTableTakesTheShadesAfresh(t *testing.T) {
	rows := shaded([]Version{
		{Name: "Opus 6", Family: "opus"},
		{Name: "Opus 5.5", Family: "opus"},
		{Name: "Opus 5", Family: "opus"},
		{Name: "Sonnet 5.5", Family: "sonnet"},
		{Name: "Fable 5.1", Family: "fable"},
		{Name: "Haiku 5.5", Family: "haiku"},
		{Name: "Haiku 4.5", Family: "haiku"},
		{Name: "Mythos 5.1", Family: "mythos"},
	})
	want := []struct {
		shade int
		older bool
	}{{shade: 1}, {shade: 2, older: true}, {older: true}, {shade: 3}, {shade: 5}, {shade: 6}, {older: true}, {}}
	for i, w := range want {
		if v := rows[i]; v.Shade != w.shade || v.Older != w.older {
			t.Errorf("%s has shade %d, older %v; want shade %d, older %v", v.Name, v.Shade, v.Older, w.shade, w.older)
		}
	}
}
