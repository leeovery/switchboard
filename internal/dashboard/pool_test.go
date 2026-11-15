package dashboard

import (
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/status"
)

// acme is pool Acme shown, fenced, with three accounts, beside Default.
var acme = status.ShownPool{ID: "q7mt", Label: "Acme", Dirs: []string{"~/Code/acme"}, Accounts: 3, Pools: 2}

// emptyAcme is acme with no accounts.
var emptyAcme = status.ShownPool{ID: "q7mt", Label: "Acme", Dirs: []string{"~/Code/acme"}, Pools: 2}

// The title row names the pool shown after the badge, the tabs moved along,
// once there's a second pool, at every width, the phone's included; never
// with one pool, or none shown.
func TestTheTitleRowNamesThePoolShownOnceThereIsASecond(t *testing.T) {
	tests := []struct {
		name  string
		width int
		pool  status.ShownPool
		want  []string
	}{
		{name: "one of two, as overview-acme-160 draws it", width: 160, pool: acme, want: []string{
			" SWITCHBOARD   Acme    Overview   Accounts   History   Sessions   Runway   Graphs   Log " + blanks(52) + "Mon 28 Sep  13:12:00",
		}},
		{name: "the only pool: unnamed", width: 160, pool: status.ShownPool{ID: "b2n8", Label: "Default", Accounts: 3, Pools: 1}, want: []string{
			" SWITCHBOARD    Overview   Accounts   History   Sessions   Runway   Graphs   Log " + blanks(59) + "Mon 28 Sep  13:12:00",
		}},
		{name: "none shown, every pool fenced: unnamed", width: 160, pool: status.ShownPool{Pools: 2}, want: []string{
			" SWITCHBOARD    Overview   Accounts   History   Sessions   Runway   Graphs   Log " + blanks(59) + "Mon 28 Sep  13:12:00",
		}},
		{name: "on a phone, as overview-acme-73x61 draws it", width: 73, pool: acme, want: []string{
			" SWITCHBOARD   Acme" + blanks(34) + "Mon 28 Sep  13:12:00",
			" Overview  Accounts  History  Sessions  Runway  Graphs  Log",
		}},
		{name: "at the narrowest width off a phone, a long label cut to leave the tabs whole", width: 100, pool: status.ShownPool{ID: "q7mt", Label: "Acme Corporation Research", Accounts: 1, Pools: 2}, want: []string{
			" SWITCHBOARD   Acme Corporatio…    Overview   Accounts   History   Sessions   Runway   Graphs   Log",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := Frame{Width: tt.width, Height: 5, View: Overview, Pool: tt.pool}
			c := newCanvas(f.Width, f.Height)
			f.title(c, now)
			if rows := c.rows(Look{})[:len(tt.want)]; !slices.Equal(rows, tt.want) {
				t.Errorf("the title reads\n%s\nwant\n%s", strings.Join(rows, "\n"), strings.Join(tt.want, "\n"))
			}
		})
	}
}

// The pool's name is drawn as setup's is, bold text.primary, and a click on
// a tab lands where the tab is drawn, moved along after it.
func TestThePoolsNameIsSetupsInkAndTheTabsTargetsFollowIt(t *testing.T) {
	f := Frame{Width: 160, Height: 5, View: Overview, Pool: acme}
	c := newCanvas(f.Width, f.Height)
	f.title(c, now)
	if got := c.at(15, 0).ink; got != titleInk {
		t.Errorf("the pool's name is in %+v, want %+v, as setup's", got, titleInk)
	}
	row := c.rows(Look{})[0]
	for _, target := range f.tabTargets() {
		if got := strings.TrimSpace(string([]rune(row)[target.X : target.X+target.Width])); got != target.Tab.Title() {
			t.Errorf("the target of the %s tab covers %q, want the tab", target.Tab.Title(), got)
		}
	}
}

// A pool with no accounts says so on its status line, after how the router
// stands and the restart it has due, which takes up any the config gives it,
// in place of how it routes, whichever way the document was read. A pool the
// router refuses to read, not knowing it, says how the router stands as it
// answered, as one it knows does, whatever document is on screen.
func TestThePoolWithNoAccountsSaysSoOnItsStatusLine(t *testing.T) {
	probed := func(d *status.Document) {
		d.Source, d.Fallback, d.Accounts = status.SourceProbe, status.Fallback{Router: status.RouterNotRunning}, nil
	}
	none := func(d *status.Document) { d.Accounts, d.Best, d.Sessions = nil, "", 0 }
	due := status.Restart{Reason: status.RestartForConfig, InFlight: 1}
	waiting := emptyAcme
	waiting.Accounts = 1
	refused := func(health status.Health) status.ShownPool {
		return waiting.Refused(status.RouterAnswer{Health: health, Restart: due})
	}
	tests := []struct {
		name  string
		pool  status.ShownPool
		doc   status.Document
		width int
		want  string
	}{
		{name: "routed, a restart due", pool: waiting, doc: routed(none, func(d *status.Document) { d.PoolID, d.Restart = "q7mt", due }), width: 160, want: "● healthy  ·  restart due (config changed)  ·  no accounts routed yet"},
		{name: "refused by a healthy router, a restart due", pool: refused(status.Health{Healthy: true}), width: 160, want: "● healthy  ·  restart due (config changed)  ·  no accounts routed yet"},
		{name: "refused by an unhealthy router", pool: refused(status.Health{Reason: "every account refused"}), doc: routed(), width: 160, want: "● unhealthy  ·  restart due (config changed)  ·  no accounts routed yet"},
		{name: "routed, as empty-pool-160 draws it", doc: routed(func(d *status.Document) { d.Accounts, d.Best, d.Sessions = nil, "", 0 }), width: 160, want: "● healthy  ·  no accounts"},
		{name: "probed", doc: routed(probed), width: 160, want: "○ probed · router not running  ·  no accounts"},
		{name: "on a phone", doc: routed(func(d *status.Document) { d.Accounts, d.Best, d.Sessions = nil, "", 0 }), width: 52, want: "● healthy · no accounts"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool := tt.pool
			if pool.ID == "" {
				pool = emptyAcme
			}
			f := Frame{Width: tt.width, Height: 8, Pool: pool.AsRead(tt.doc.At(printNow, printPolicy))}
			c := newCanvas(f.Width, f.Height)
			f.routerStatus(c, tt.doc, printNow)
			if got := strings.TrimRight(c.rows(Look{})[f.statusRow()], " "); got != tt.want {
				t.Errorf("the status line reads %q, want %q", got, tt.want)
			}
		})
	}
}

// A pool with no accounts draws, on every tab, the page empty-pool-160
// draws: what doesn't start, how to add an account or move one in, and how
// long its directories stay fenced; no heading row, no view, nothing to
// click on but its tabs and its keys, and nothing that scrolls.
func TestAPoolWithNoAccountsDrawsItsPageOnEveryTab(t *testing.T) {
	doc := routed(func(d *status.Document) { d.Accounts, d.Best, d.Sessions = nil, "", 0 })
	want := []string{
		"  Acme has no accounts, so a session started in ~/Code/acme doesn't start.",
		"",
		"  Add one with switchboard accounts add <label> --pool q7mt, or in switchboard setup.",
		"  Move one in with switchboard accounts edit <id> --pool q7mt.",
		"",
		"  Its directories stay fenced until the pool is removed with switchboard pool remove q7mt.",
	}
	for _, v := range Views() {
		t.Run(v.Title(), func(t *testing.T) {
			f := Frame{Width: 160, Height: 37, View: v, Pool: emptyAcme, Policy: printPolicy}
			rows := f.Draw(doc, printNow)
			for i, w := range want {
				if got := strings.TrimRight(rows[4+i], " "); got != w {
					t.Errorf("row %d reads %q, want %q", 4+i, got, w)
				}
			}
			for y := 4 + len(want); y < f.Height-1; y++ {
				if got := strings.TrimSpace(rows[y]); got != "" {
					t.Errorf("row %d reads %q, want it blank, drawing no view", y, got)
				}
			}
			for _, target := range f.Targets(doc, printNow) {
				if target.On != OnTab && target.On != OnKey {
					t.Errorf("a click on %+v acts, want only the tabs and the key line's keys to", target)
				}
			}
			if under := f.Under(doc, printNow, 10, 10); under != UnderNothing {
				t.Errorf("under the page is %v, want nothing that scrolls", under)
			}
		})
	}
}

// On a phone, the page of a pool with no accounts wraps its lines at the
// frame's edge, under the tabs' rows.
func TestAPhonesPageOfAPoolWithNoAccountsWraps(t *testing.T) {
	doc := routed(func(d *status.Document) { d.Accounts, d.Best, d.Sessions = nil, "", 0 })
	f := Frame{Width: 52, Height: 30, View: Overview, Pool: emptyAcme, Policy: printPolicy}
	rows := f.Draw(doc, printNow)
	top := f.statusRow() + 2
	if got, want := strings.TrimRight(rows[top], " "), "  Acme has no accounts, so a session started in"; got != want {
		t.Errorf("the page's first row reads %q, want %q", got, want)
	}
	for y, row := range rows[top : f.Height-1] {
		if strings.Contains(row, "…") {
			t.Errorf("row %d reads %q, want it wrapped, never cut", top+y, row)
		}
	}
}
