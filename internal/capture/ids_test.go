package capture

import (
	"regexp"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/dashboard/watch"
)

// unlikeLabels are the labels the guard against ids gives the worlds'
// accounts, by their ids, each unlike any id and any word the dashboard
// says, so an id shown never passes for a label: the worlds keep the ids
// their records are keyed by, words as work and side, and take labels in
// their place, as a config labelling hashed ids does the other way round.
var unlikeLabels = map[string]string{
	"work": "a1b2", "personal": "c3d4", "side": "e5f6", "client": "g7h8", "spare": "i9j0",
	"lab": "k1l2", "team": "m3n4", "extra": "o5p6", "ops": "q7r8", "studio": "s9t0", teamID: "u1v2",
}

// idSizes are the sizes the guard draws every fixture at: the owner's phone
// upright, iPad and desktop, and capture's 160 × 37.
var idSizes = []watch.Size{{Width: 73, Height: 61}, {Width: 108, Height: 44}, {Width: 173, Height: 53}, wide(37)}

// unlikeLabelled is the world given, each account, and each removed, labelled
// as unlikeLabel has it.
func unlikeLabelled(w accountWorld) accountWorld {
	w.accounts = slices.Clone(w.accounts)
	for i, a := range w.accounts {
		w.accounts[i].Label = unlikeLabel(a.ID)
	}
	w.gone = slices.Clone(w.gone)
	for i, g := range w.gone {
		w.gone[i].Label = unlikeLabel(g.ID)
	}
	return w
}

// unlikeLabel is the label unlikeLabels gives the account with the given
// id, or, where it gives none, its id inside a word, which never reads as
// the id said.
func unlikeLabel(id string) string {
	if label, ok := unlikeLabels[id]; ok {
		return label
	}
	return "z" + id + "z"
}

// wordsLikeIDs are the dashboard's own phrases that read as an id of the
// worlds', each whole: Claude's extra usage, as Compare names it; History's
// "plenty to spare"; Graphs' WHEN YOU WORK; and a card's key, ←→ moving side
// to side.
var wordsLikeIDs = regexp.MustCompile(`(?i)extra usage|plenty to spare|when you work|←→ side   ↑↓ move   space tick   enter apply   esc cancel`)

// idsShown are the ids of the world's accounts, and of those removed, that
// the rows show as words, as id said: a word of its own, never a part of
// one, in any case, as a heading upper-cases a name, the dashboard's
// phrases like them, as wordsLikeIDs has them, passed over.
func idsShown(rows []string, w accountWorld) []string {
	var ids []string
	for _, a := range w.accounts {
		ids = append(ids, a.ID)
	}
	for _, g := range w.gone {
		ids = append(ids, g.ID)
	}
	var shown []string
	for _, id := range ids {
		said := regexp.MustCompile(`(?i)(^|[^\pL\pN_-])` + regexp.QuoteMeta(id) + `($|[^\pL\pN_-])`)
		for _, row := range rows {
			if said.MatchString(wordsLikeIDs.ReplaceAllString(row, "")) {
				shown = append(shown, id+" in: "+row)
			}
		}
	}
	return shown
}

// No fixture the watch draws shows an account's id where its label differs:
// every page, over its world with each account labelled unlike its id, as
// unlikeLabels has it, settled at its own size, whatever's open over it
// kept, drawn at idSizes, names every account by its label alone.
func TestNoViewShowsAnAccountsID(t *testing.T) {
	for _, p := range pages {
		labelled := p
		labelled.world = func(a accountWorlds) accountWorld { return unlikeLabelled(p.world(a)) }
		t.Run(p.name, func(t *testing.T) {
			t.Parallel()
			worlds := worldsNow()
			w := labelled.world(worlds)
			f := labelled.fixture(worlds)
			m := f.settle(f.Size)
			for _, size := range idSizes {
				for _, shown := range idsShown(fluidAt(m, size).rows, w) {
					t.Errorf("at %s, an account's id is shown, %s", sizeName(size), shown)
				}
			}
		})
	}
}
