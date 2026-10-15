package dashboard

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// stepFiveBoard is a Switchboard that takes step 5 at 160 columns in a
// short terminal: its world, and the calls the watch built of it.
type stepFiveBoard struct {
	name  string
	doc   status.Document
	calls []views.Call
}

// stepFiveBoards are the scene, personal's calls starting above its line;
// the frame of ten accounts; and twenty calls on side, more than a short
// terminal's view holds.
func stepFiveBoards() []stepFiveBoard {
	crowd := make([]views.Call, 20)
	for i := range crowd {
		crowd[i] = boardCall(fmt.Sprintf("%04x0000-0000-4000-8000-000000000000", i), "side", "~/Code/crowd", idleSince(routedAt.Add(-time.Minute)), "")
	}
	return []stepFiveBoard{
		{name: "the scene", doc: usageWorld(), calls: sceneCalls()},
		{name: "ten accounts", doc: boardOfTen(), calls: tenCalls()},
		{name: "twenty calls on side", doc: usageWorld(), calls: crowd},
	}
}

// eachAtStep5 calls each with the frame of each board at each height from
// 10 to 30 rows at which it takes step 5, and draws its view, scrolled to
// its top, and its layout.
func eachAtStep5(t *testing.T, each func(t *testing.T, b stepFiveBoard, f Frame, laid boardLayout)) {
	t.Helper()
	for _, b := range stepFiveBoards() {
		for height := 10; height <= 30; height++ {
			f := boardFrame(160, height, Look{}, boardOf(boardLook{calls: b.calls}), Traffic{})
			if laid := f.switchboardLaid(b.doc, routedAt); laid.step == compactedStep && !laid.lanes && !f.cramped() {
				t.Run(fmt.Sprintf("%s at %d rows", b.name, height), func(t *testing.T) { each(t, b, f, laid) })
			}
		}
	}
}

// At step 5, each account's rows run from its first, its group's first
// call's or its line's, whichever is higher, to its line's last, its calls
// among them; and start below the account above's, its group moving down to
// clear them: the one rule an account's rows are laid out and scrolled by.
func TestAtStep5EachAccountsRowsStartBelowTheAccountAbovesLast(t *testing.T) {
	eachAtStep5(t, func(t *testing.T, _ stepFiveBoard, _ Frame, laid boardLayout) {
		for i, ln := range laid.lines {
			if i > 0 && ln.first < laid.lines[i-1].end() {
				t.Errorf("%s's rows start on %d, want below %s's last, %d", ln.account, ln.first, laid.lines[i-1].account, laid.lines[i-1].end()-1)
			}
			if want := min(ln.top, ln.groupStart); len(ln.patched.Calls) > 0 && ln.first != want {
				t.Errorf("%s's first row is %d, want its group's or its line's, whichever is higher, %d", ln.account, ln.first, want)
			}
		}
		for _, c := range laid.calls {
			if ln, _ := laid.line(c.account); c.cord.from < ln.first || c.cord.from >= ln.end() {
				t.Errorf("%s's call is on row %d, want it among %s's rows, %d to %d", c.session, c.cord.from, ln.account, ln.first, ln.end()-1)
			}
		}
	})
}

// At step 5, every call shows at some scroll, its row whole. Where every
// account fits the view, under the row saying how many are above, it
// scrolls an account at a time, as Accounts does: each scroll shows the
// accounts from the one scrolled to, each whole, its every call with it,
// and none of the next, so paging on never cuts a call; the view's first
// row says how many accounts are above, and the row over the key line how
// many are below, those it doesn't show. Where one doesn't fit, it scrolls
// a row at a time, the row over the key line saying how many rows are above
// and below.
func TestAtStep5EveryCallShowsAtSomeScrollAndWhatsOutOfViewIsCounted(t *testing.T) {
	eachAtStep5(t, func(t *testing.T, b stepFiveBoard, f Frame, laid boardLayout) {
		byRows := f.sessionsForm(b.doc, routedAt).byRows
		reached := make(map[Plug]bool)
		most := f.Scrolling(b.doc, routedAt).Most
		for offset := range most + 1 {
			f.Scroll = offset
			rows := textOf(f.Draw(b.doc, routedAt))
			targeted := targetedCalls(t, f, b.doc, rows)
			for plug := range targeted {
				reached[plug] = true
			}
			if byRows {
				rowsCounted(t, offset, most, rows[f.Height-underView])
				continue
			}
			accountsCounted(t, f, b.doc, laid, rows, targeted)
		}
		for _, c := range laid.calls {
			if !reached[Plug{Session: c.session, Account: c.account}] {
				t.Errorf("%s's call shows at no scroll", status.ShownID(c.session))
			}
		}
	})
}

// targetedCalls are the calls the frame f of doc draws as rows has them
// that a click chooses, each on its row, which starts with its id.
func targetedCalls(t *testing.T, f Frame, doc status.Document, rows []string) map[Plug]bool {
	t.Helper()
	targeted := make(map[Plug]bool)
	for _, target := range f.Targets(doc, routedAt) {
		if target.On != OnSession {
			continue
		}
		targeted[Plug{Session: target.Session, Account: target.Account}] = true
		if !strings.HasPrefix(rows[target.Y], status.ShownID(target.Session)) {
			t.Errorf("scrolled %d, %s's target is on row %d, which reads %q", f.Scroll, status.ShownID(target.Session), target.Y+1, rows[target.Y])
		}
	}
	return targeted
}

// rowsCounted holds the row over the key line, under, of a Switchboard
// scrolled offset rows of most, to saying how many rows are above and
// below.
func rowsCounted(t *testing.T, offset, most int, under string) {
	t.Helper()
	if said := fmt.Sprintf("▲ %d more above", offset); offset > 0 && !strings.Contains(under, said) {
		t.Errorf("scrolled %d rows, the row over the key line reads %q, want it to say %q", offset, under, said)
	}
	if offset < most && !strings.Contains(under, "more below") {
		t.Errorf("scrolled %d rows of %d, the row over the key line reads %q, want it to say how many are below", offset, most, under)
	}
}

// accountsCounted holds the frame f of doc, drawn as rows, its layout laid,
// its calls targeted, to showing a page of accounts whole from the one it's
// scrolled to, each with every call of its, and to saying how many are
// above, on the view's first row, and below, on the row over the key line.
func accountsCounted(t *testing.T, f Frame, doc status.Document, laid boardLayout, rows []string, targeted map[Plug]bool) {
	t.Helper()
	var shown []int
	for i, ln := range laid.lines {
		name := fmt.Sprintf("%d %s", i+1, ln.account)
		if !slices.ContainsFunc(rows[f.top():f.Height-underView], func(row string) bool { return strings.Contains(row, name) }) {
			continue
		}
		shown = append(shown, i)
		for _, p := range ln.patched.Calls {
			if !targeted[Plug{Session: p.Session, Account: ln.account}] {
				t.Errorf("scrolled %d, %s shows, but not its call %s", f.Scroll, ln.account, status.ShownID(p.Session))
			}
		}
	}
	offset, page := f.Scroll, f.Scrolling(doc, routedAt).Page
	var want []int
	for i := offset; i < offset+page; i++ {
		want = append(want, i)
	}
	if !slices.Equal(shown, want) {
		t.Errorf("scrolled %d, the accounts shown are %v, want the page from it, %v", offset, shown, want)
	}
	if over, said := rows[f.top()], "▲ "+moreAccounts(offset); offset > 0 && !strings.HasSuffix(over, said) || offset == 0 && strings.Contains(over, "▲") {
		t.Errorf("scrolled %d, the view's first row reads %q, want it to say %d above", offset, over, offset)
	}
	below := len(laid.lines) - offset - page
	if under, said := rows[f.Height-underView], "▼ "+moreAccounts(below); below > 0 && !strings.HasSuffix(under, said) || below == 0 && strings.Contains(under, "▼") {
		t.Errorf("scrolled %d, the row over the key line reads %q, want it to say %d below", offset, under, below)
	}
}
