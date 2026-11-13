package screen

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/setup"
)

// pooledSteps are frame 3's accounts, in Default, unfenced, with a second
// pool, Acme, fenced to ~/Code/acme, holding Acme 1 and Acme 2, Acme 1
// marked its primary: the pools' frames 5 to 7.
func pooledSteps() *fakeSteps {
	steps := frame3Steps()
	steps.review.Pools = config.Pools{
		{ID: "b2n8", Label: "Default"},
		{ID: "q7mt", Label: "Acme", Dirs: []string{"/home/user/Code/acme"}, WrittenDirs: []string{"~/Code/acme"}},
	}
	accounts := slices.Clone(steps.review.Accounts)
	for i := range accounts {
		accounts[i].Pool = "b2n8"
	}
	steps.review.Accounts = append(accounts,
		setup.Reviewed{ID: "acme1", Label: "Acme 1", Plan: "ultra", Primary: true, Pool: "q7mt", Token: readToken},
		setup.Reviewed{ID: "acme2", Label: "Acme 2", Plan: "ultra", Pool: "q7mt", Token: readToken},
	)
	steps.checked["acme1"] = setup.TokenStanding{State: setup.TokenWorking}
	steps.checked["acme2"] = setup.TokenStanding{State: setup.TokenWorking}
	return steps
}

// addingPooled is a harness of pooledSteps, its add card open on + add an
// account.
func addingPooled(t *testing.T, steps *fakeSteps) *harness {
	t.Helper()
	h := accountsHarness(t, steps)
	h.start()
	for range len(steps.review.Accounts) {
		h.pressAndDeliver("down")
	}
	h.pressAndDeliver("enter")
	if h.model.add == nil {
		t.Fatal("enter on + add an account opened no add card")
	}
	return h
}

// The table groups the accounts by pool, once there's a second: each pool's
// label over its accounts, with its directories, or everywhere else for the
// unfenced; each pool's primary marked.
func TestTheTableGroupsTheAccountsByPool(t *testing.T) {
	h := accountsHarness(t, pooledSteps())
	h.start()
	var got []string
	for _, row := range h.rows()[firstRow : firstRow+16] {
		got = append(got, strings.TrimRight(ansiCut(row, 30), " "))
	}
	want := []string{
		"    Default  everywhere else", "",
		"▌   Work ◆          Ultra", "",
		"    Personal        not set", "",
		"    Side            Pro", "",
		"    Acme  ~/Code/acme", "",
		"    Acme 1 ◆        Ultra", "",
		"    Acme 2          Ultra", "",
		"  + add an account", "",
	}
	if !slices.Equal(got, want) {
		t.Errorf("the table reads\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// ansiCut is row's first cells, as many as given.
func ansiCut(row string, cells int) string {
	runes := []rune(row)
	return string(runes[:min(cells, len(runes))])
}

// The add card's Pool, the third field, offers every pool by its label, in
// the config's order, then new pool, the pool accounts add adds one to
// without --pool chosen as it opens; its hint says which sessions use the
// pool chosen; and the account checked goes in it.
func TestTheAddCardsPool(t *testing.T) {
	h := addingPooled(t, pooledSteps())
	if got := h.cardRow("Pool"); got != "  Pool      ■ Default    □ Acme    □ new pool" {
		t.Errorf("Pool reads %q, want Default, the unfenced pool, chosen", got)
	}
	if !strings.Contains(strings.Join(cardRows(h), "\n"), "sessions started everywhere else use it") {
		t.Errorf("the card reads\n%s\nwant Default's hint", strings.Join(cardRows(h), "\n"))
	}
	h.typeIn("Acme 3")
	for _, k := range []string{"tab", "tab", "right"} {
		h.pressAndDeliver(k)
	}
	if got := h.model.add.chosen; got != setup.PoolField {
		t.Fatalf("tab twice chose field %d, want Pool", got)
	}
	if !strings.Contains(strings.Join(cardRows(h), "\n"), "only sessions started in ~/Code/acme use it") {
		t.Errorf("Acme chosen, the card reads\n%s\nwant Acme's hint", strings.Join(cardRows(h), "\n"))
	}
	h.pressAndDeliver("enter")
	if want := []setup.Adding{{Label: "Acme 3", Plan: "pro", Pool: "q7mt"}}; !reflect.DeepEqual(h.steps.adds, want) {
		t.Errorf("CheckAdding was asked %+v, want %+v", h.steps.adds, want)
	}
}

// cardRows are the rows of the card open.
func cardRows(h *harness) []string {
	_, rows := h.card()
	return rows
}

// Where every pool is fenced, the add card's Pool opens on the first; with
// one pool, it offers it, then new pool, as it's where a second is made.
func TestTheAddCardsPoolAsItOpens(t *testing.T) {
	fenced := pooledSteps()
	fenced.review.Pools[0].Dirs, fenced.review.Pools[0].WrittenDirs = []string{"/home/user/Code/work"}, []string{"~/Code/work"}
	one := frame3Steps()
	one.review.Pools = config.Pools{{ID: "b2n8", Label: "Default"}}
	for name, tt := range map[string]struct {
		steps *fakeSteps
		want  string
	}{
		"every pool fenced": {steps: fenced, want: "  Pool      ■ Default    □ Acme    □ new pool"},
		"one pool":          {steps: one, want: "  Pool      ■ Default    □ new pool"},
	} {
		t.Run(name, func(t *testing.T) {
			h := addingPooled(t, tt.steps)
			if got := h.cardRow("Pool"); got != tt.want {
				t.Errorf("Pool reads %q, want %q", got, tt.want)
			}
		})
	}
}

// new pool, chosen on the add card, has enter open the new pool card, the
// key line saying so; esc on it goes back to the add card, holding what it
// held; + another directory adds one to type; and a pool CheckPool passes
// closes it, the add card holding it among its pools, chosen, said as its
// hint, and the account checked with it.
func TestTheNewPoolCardFromTheAddCard(t *testing.T) {
	h := addingPooled(t, pooledSteps())
	h.typeIn("Client 1")
	for _, k := range []string{"tab", "tab", "right", "right"} {
		h.pressAndDeliver(k)
	}
	if rows := cardRows(h); !strings.Contains(rows[len(rows)-1], "enter new pool") {
		t.Errorf("new pool chosen, the card's keys read %q, want enter making the pool", rows[len(rows)-1])
	}
	h.pressAndDeliver("enter")
	if title, _ := h.card(); title != "NEW POOL" {
		t.Fatalf("enter on new pool opened %q, want NEW POOL", title)
	}
	h.pressAndDeliver("esc")
	if title, _ := h.card(); title != "ADD AN ACCOUNT" || h.model.add.label.value() != "Client 1" {
		t.Fatalf("esc left %q open, want the add card back as it was", title)
	}

	h.pressAndDeliver("enter")
	h.typeIn("Client")
	h.pressAndDeliver("tab")
	h.typeIn("~/Code/client")
	h.pressAndDeliver("tab")
	if rows := cardRows(h); !strings.Contains(rows[len(rows)-1], "enter add a directory") {
		t.Errorf("+ another directory chosen, the card's keys read %q, want enter adding one", rows[len(rows)-1])
	}
	h.pressAndDeliver("enter")
	h.typeIn("/Volumes/Client")
	h.pressAndDeliver("enter")
	want := accounts.NewPool{Label: "Client", Dirs: []string{"~/Code/client", "/Volumes/Client"}}
	if !reflect.DeepEqual(h.steps.pools, []accounts.NewPool{want}) {
		t.Fatalf("CheckPool was asked %+v, want %+v", h.steps.pools, want)
	}
	if title, _ := h.card(); title != "ADD AN ACCOUNT" {
		t.Fatalf("the pool passing, %q is open, want the add card back", title)
	}
	if got := h.cardRow("Pool"); got != "  Pool      □ Default    □ Acme    ■ Client    □ new pool" {
		t.Errorf("Pool reads %q, want Client held, and chosen", got)
	}
	if !strings.Contains(strings.Join(cardRows(h), "\n"), "only sessions started in ~/Code/client or /Volumes/Client use it") {
		t.Errorf("the card reads\n%s\nwant Client's hint", strings.Join(cardRows(h), "\n"))
	}
	h.pressAndDeliver("enter")
	if len(h.steps.adds) != 1 || !reflect.DeepEqual(h.steps.adds[0].NewPool, &want) {
		t.Fatalf("CheckAdding was asked %+v, want Client 1 with Client to make", h.steps.adds)
	}
	if got := h.steps.nextMake(t); !reflect.DeepEqual(got.NewPool, &want) {
		t.Errorf("MakeToken was asked %+v, want Client made with the account", got)
	}
}

// A pool setup refuses is said under the new pool card's field it's of, the
// directory refused among them, the card staying open; unsaid once the
// value changes.
func TestTheNewPoolCardSaysWhyAPoolIsRefused(t *testing.T) {
	steps := pooledSteps()
	steps.poolErr = &setup.Refused{Field: setup.DirsField, Dir: 1, Why: "is inside Acme's ~/Code/acme"}
	h := addingPooled(t, steps)
	for _, k := range []string{"tab", "tab", "right", "right", "enter"} {
		h.pressAndDeliver(k)
	}
	h.typeIn("Client")
	for _, k := range []string{"tab", "tab", "enter"} {
		h.pressAndDeliver(k)
	}
	h.typeIn("~/Code/client")
	h.pressAndDeliver("tab")
	h.pressAndDeliver("enter")
	h.typeIn("~/Code/acme/x")
	h.pressAndDeliver("enter")
	rows := cardRows(h)
	at := slices.IndexFunc(rows, func(r string) bool { return strings.Contains(r, "~/Code/acme/x") })
	if at < 0 || !strings.Contains(rows[at+1], "is inside Acme's ~/Code/acme") {
		t.Fatalf("the card reads\n%s\nwant why under the second directory, ~/Code/acme/x, the first left empty passed over", strings.Join(rows, "\n"))
	}
	h.typeIn("y")
	if strings.Contains(strings.Join(cardRows(h), "\n"), "is inside Acme's") {
		t.Errorf("changed, the directory's refusal is still said")
	}
}

// The edit card has a Pool once there's a second pool, its hint saying a
// move takes the account's sessions off; Primary's hint names the pool
// chosen. Moved, the edit asks for the pool, and the primary where yes is
// chosen; moved to a pool with no account, or a new one, Primary shows yes,
// held, as it's the pool's first.
func TestTheEditCardsPoolAndPrimary(t *testing.T) {
	steps := pooledSteps()
	steps.review.Pools = append(steps.review.Pools, config.Pool{ID: "r2d4", Label: "Spare", Dirs: []string{"/x"}, WrittenDirs: []string{"/x"}})
	h := accountsHarness(t, steps)
	h.start()
	for range 4 {
		h.pressAndDeliver("down")
	}
	h.pressAndDeliver("e")
	rows := strings.Join(cardRows(h), "\n")
	for _, want := range []string{
		"  Pool      □ Default    ■ Acme    □ Spare    □ new pool",
		"            moving it takes its sessions off at once",
		"            Acme's sessions start on it; the Claude apps sign in to it",
	} {
		if !strings.Contains(rows, want) {
			t.Errorf("the card reads\n%s\nwant %q", rows, want)
		}
	}
	h.pressAndDeliver("tab")
	h.pressAndDeliver("tab")
	h.pressAndDeliver("left")
	if got := h.cardRow("Primary"); got != "  Primary   ■ no    □ yes" || !strings.Contains(strings.Join(cardRows(h), "\n"), "Default's sessions start on it") {
		t.Errorf("Default chosen, Primary reads %q, want no, its hint Default's", got)
	}
	h.pressAndDeliver("right")
	h.pressAndDeliver("right")
	if got := h.cardRow("Primary"); got != "  Primary   □ no    ■ yes" {
		t.Errorf("Spare, with no account, chosen, Primary reads %q, want yes, held", got)
	}
	h.pressAndDeliver("enter")
	if edits := h.steps.edits; len(edits) != 1 || edits[0].id != "acme2" || edits[0].edit.Pool == nil || *edits[0].edit.Pool != "r2d4" || !edits[0].edit.Primary {
		t.Errorf("EditAccount was given %+v, want acme2 moved to Spare, its primary", edits)
	}
}

// With one pool, the edit card has no Pool, as there's no other to move an
// account to, and Primary's hint is as before pools.
func TestTheEditCardWithOnePoolHasNoPool(t *testing.T) {
	steps := frame3Steps()
	steps.review.Pools = config.Pools{{ID: "b2n8", Label: "Default"}}
	h := accountsHarness(t, steps)
	h.start()
	h.pressAndDeliver("e")
	if got := h.cardRow("Pool"); got != "" {
		t.Errorf("with one pool, the edit card has Pool %q, want none", got)
	}
	if !strings.Contains(strings.Join(cardRows(h), "\n"), "the account the Claude apps are signed in to") {
		t.Errorf("the card reads\n%s\nwant Primary's hint as before pools", strings.Join(cardRows(h), "\n"))
	}
}

// With Pool on new pool, enter opens the new pool card from whichever field
// is chosen, checking and saving nothing, as the account has no pool to go
// in till one's made: on the add card, and on the edit card.
func TestEnterWithNewPoolChosenOpensTheNewPoolCard(t *testing.T) {
	h := addingPooled(t, pooledSteps())
	h.typeIn("Client 1")
	for _, k := range []string{"tab", "tab", "right", "right", "tab", "enter"} {
		h.pressAndDeliver(k)
	}
	if title, _ := h.card(); title != "NEW POOL" || len(h.steps.adds) != 0 {
		t.Errorf("enter on Label, Pool on new pool, opened %q and checked %+v; want NEW POOL, nothing checked", title, h.steps.adds)
	}

	h = accountsHarness(t, pooledSteps())
	h.start()
	h.pressAndDeliver("e")
	for _, k := range []string{"tab", "tab", "right", "right", "tab", "enter"} {
		h.pressAndDeliver(k)
	}
	if title, _ := h.card(); title != "NEW POOL" || len(h.steps.edits) != 0 {
		t.Errorf("enter on Primary, Pool on new pool, opened %q and saved %+v; want NEW POOL, nothing saved", title, h.steps.edits)
	}
}
