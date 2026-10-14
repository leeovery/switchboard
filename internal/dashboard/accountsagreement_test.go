package dashboard

import (
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/status/statustest"
)

// At each hard moment, Detail says of each account what the Overview's views
// say of it: its head how it stands, in ROUTING's glyph and first words, with
// no time it's back; its list the state ROUTING's kind is; and its USAGE each
// window as USAGE does, bar and words. All accounts' room of a window is what
// USAGE's rows leave of it, to each limit, summed, of the accounts it counts:
// not the pool's, which is to each cap, so the two differ by design wherever
// an account holds back a reserve, as the moment of a week at its reserve
// shows.
func TestDetailAgreesWithTheViewsAtEachHardMoment(t *testing.T) {
	for _, hm := range statustest.HardMoments(claudeLike) {
		t.Run(hm.Name, func(t *testing.T) {
			d := drawnAt(t, hm.Doc, hm.Now)
			for _, a := range d.moment.Accounts {
				detail := detailAt(hm.Doc, hm.Now, a.ID)
				agree(t, a.ID+" stands", []viewSays{{"ROUTING", d.routingSays(a)}, {"Detail's head", headSays(detail)}})
				agree(t, a.ID+"'s state", []viewSays{{"ROUTING", d.routing.accounts[d.place(a)].standing.Kind.State().Glyph()}, {"Detail's list", listSays(detail, a)}})
				for _, w := range a.Windows {
					if g, ok := d.gauge(a, w); ok {
						agree(t, a.ID+"'s "+w.Key+" in USAGE", []viewSays{{"USAGE", gaugeSays(g)}, {"Detail", gaugeSays(detail.gauge(w.Key))}})
					}
				}
			}
			all := detailAt(hm.Doc, hm.Now, "")
			for _, w := range d.usage.windows {
				if room, ok := d.roomLeft(w.key); ok {
					agree(t, "all accounts' room of "+w.key, []viewSays{{"USAGE's rows, each to its limit", room}, {"Detail", roomSays(all.gauge(w.key))}})
				}
			}
		})
	}
}

// At a week at its reserve, all accounts' room of the week is to each
// account's limit, and the pool's to each cap: the reserve held back apart.
func TestAllAccountsRoomIsNotThePools(t *testing.T) {
	i := hardMoment(t, "a week at its reserve when built")
	d := drawnAt(t, i.Doc, i.Now)
	pool, ok := d.usage.poolGauge("7d")
	if !ok {
		t.Fatal("USAGE draws no pool's week")
	}
	all := roomSays(detailAt(i.Doc, i.Now, "").gauge("7d"))
	if pooled := roomSays(pool); all == pooled {
		t.Errorf("all accounts' room of the week is the pool's, %q, want it to each limit, apart from the reserve the pool holds back", all)
	}
}

// hardMoment is the hard moment of the name given.
func hardMoment(t *testing.T, name string) statustest.HardMoment {
	t.Helper()
	for _, hm := range statustest.HardMoments(claudeLike) {
		if hm.Name == name {
			return hm
		}
	}
	t.Fatalf("no hard moment %q", name)
	return statustest.HardMoment{}
}

// detailAt is Detail of doc at now, wide, its accounts configured as doc's,
// the account with the given id chosen, "" for all accounts.
func detailAt(doc status.Document, now time.Time, id string) detailDrawn {
	var configured config.Accounts
	for _, a := range doc.Accounts {
		configured = append(configured, config.Account{ID: a.ID})
	}
	f := frameOf(160, 40).At(doc, now)
	f.View, f.Accounts = Accounts, AccountsShown{Shown: AccountsDetail, Accounts: configured, Chosen: id}
	return f.detailDrawn(doc, now, f.detailLayout())
}

// clauseApart is what sets a standing's clauses apart.
const clauseApart = " · "

// routingSays is how ROUTING says account a stands: its glyph and first
// words.
func (d drawn) routingSays(a status.AccountNow) string {
	said, _, _ := strings.Cut(standingWords(d.routing.accounts[d.place(a)].standing, false, 200).plain(), clauseApart)
	return said
}

// headSays is how Detail's head says its account stands, as ROUTING would:
// its glyph and first words; or the head whole, where it says when the
// account's back, which it never does.
func headSays(d detailDrawn) string {
	head := strings.TrimSpace(textOf(d.panel().rows(Look{}))[0])
	words := strings.TrimSpace(strings.TrimPrefix(head, listName(d.account, 1).fitted(d.f.edge()).plain()))
	if strings.Contains(words, "back ") {
		return head
	}
	said, _, _ := strings.Cut(words, clauseApart)
	return said
}

// listSays is the glyph Detail's list draws beside account a.
func listSays(d detailDrawn, a status.AccountNow) string {
	c := growing(d.f.Width)
	d.list(c, 0, d.f.viewRows())
	return string([]rune(textOf(c.rows(Look{}))[a.Place*listApart])[listDotAt])
}

// gaugeSays is what a gauge says of its window: the share its bar fills,
// "none" where it draws it empty, whether it's faded as stale, and its words.
func gaugeSays(g gauge) string {
	used := "none"
	if !g.bar.empty {
		used = prose.Percent(g.bar.used)
	}
	return used + " " + staleness(g.bar.stale) + " " + g.words.full.plain()
}

// roomSays is the room a gauge's words lead with.
func roomSays(g gauge) string {
	room, _, _ := strings.Cut(g.words.full.plain(), " ")
	return room
}

// roomLeft is the room USAGE's rows leave of the window with the given key,
// to each limit, summed, of the accounts whose use of it they read, as
// poolWords says it, reporting false where fewer than two accounts are
// counted, where there's no all accounts.
func (d drawn) roomLeft(key string) (string, bool) {
	room, of := 0.0, 0
	for _, a := range d.moment.Accounts {
		w, ok := a.Window(key)
		if !ok {
			continue
		}
		if g, ok := d.gauge(a, w); ok && !g.bar.empty {
			room, of = room+max(0, 1-g.bar.used), of+1
		}
	}
	if len(d.moment.Accounts) < 2 || of == 0 {
		return "", false
	}
	return roomSays(gauge{words: poolWords(room, of, false)}), true
}

// poolGauge is the pool's gauge of the window with the given key, reporting
// false where USAGE draws no pool's.
func (b usageBlock) poolGauge(key string) (gauge, bool) {
	for _, r := range b.rows {
		if !r.pool {
			continue
		}
		for j, w := range b.windows {
			if w.key == key {
				return r.gauges[j], true
			}
		}
	}
	return gauge{}, false
}
