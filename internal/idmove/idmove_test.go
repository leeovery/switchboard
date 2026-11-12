package idmove_test

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/idmove"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/tokens"
)

// movedTokens are what the fixture's tokens directory holds moved: each
// account's token file and record by its new id alone.
var movedTokens = []string{workID, workID + ".taken", personalID, personalID + ".taken"}

// movedConfig is the fixture's config moved: each account's id the one its
// organization derives, and personal, which had no label, labelled by the id
// it had.
const movedConfig = `[[account]]
id      = "b6s7"
label   = "Work"
primary = true

[[account]]
id = "qgr2"
label = "personal"
`

func TestAWholeMoveMovesEveryAccountAndItsHistoryOntoItsID(t *testing.T) {
	f := newFixture(t)
	f.moved(t, answering())

	if got := readFile(t, f.config); got != movedConfig {
		t.Errorf("the config reads\n%s\nwant\n%s", got, movedConfig)
	}
	record := f.record(t)
	for _, want := range []identity.Entry{
		{ID: workID, Organization: workOrganization, Was: work},
		{ID: personalID, Organization: personalOrganization, Was: personal},
	} {
		got, ok := record.ByID(want.ID)
		if !ok || got.Organization != want.Organization || got.Was != want.Was || !got.Removed.IsZero() {
			t.Errorf("the record has %s as %+v, want %+v, configured", want.ID, got, want)
		}
	}
	removed, ok := record.ByWas(gone)
	if !ok || removed.Label != gone || removed.Organization != "" || removed.Removed.IsZero() || !identity.WellFormed(removed.ID) {
		t.Errorf("the record has the account removed before as %+v, want it removed, labelled %q, its organization unknown, under an id switchboard makes", removed, gone)
	}
	if got, want := tokenNames(t, f), movedTokens; !slices.Equal(got, want) {
		t.Errorf("the tokens directory holds %q, want %q", got, want)
	}
	for id, want := range map[string]string{workID: workToken, personalID: personalToken} {
		if got, err := f.tokens.Read(id); err != nil || got.Reveal() != want {
			t.Errorf("%s's token reads %v, want its own", id, err)
		}
	}
	if !idmove.Moved(f.state) || idmove.Pending(f.state) {
		t.Error("the move isn't marked done")
	}
	if _, err := os.Stat(filepath.Join(ledger.Dir(f.state), "digest-"+longAgo+".json.gz")); err == nil {
		t.Error("what the ledger built of a day under the old ids is still there, want it deleted to be built again")
	}
}

func TestEveryFileOfTheHistoryNamesTheNewIDsAlone(t *testing.T) {
	f := newFixture(t)
	f.moved(t, answering())

	removed, _ := f.record(t).ByWas(gone)
	for path, text := range f.snapshot(t) {
		if !strings.HasPrefix(path, "ledger") && !strings.HasPrefix(path, "history") && path != "state.json" {
			continue
		}
		for _, old := range []string{
			`"` + work + `"`, `"` + personal + `"`, `"` + gone + `"`,
			": " + work + " ", ": " + personal + " ",
			", " + personal + " under pressure", ", " + gone + " under pressure",
		} {
			if strings.Contains(text, old) {
				t.Errorf("%s still names %s:\n%s", path, old, text)
			}
		}
		if strings.HasPrefix(path, "history") && !strings.Contains(text, `"`+workID+`"`) {
			t.Errorf("%s doesn't name %s:\n%s", path, workID, text)
		}
	}
	state := readFile(t, filepath.Join(f.state, "state.json"))
	for _, want := range []string{`"accounts": ["b6s7"]`, `"account": "qgr2"`, `"pin": "b6s7"`, `"reason": "moved: b6s7 hit its limit"`,
		`"b6s7": {"sha256": "aa", "former": [{"sha256": "cc", "replaced_at": "2026-11-07T09:00:00Z", "from": "` + removed.ID + `"}]}`,
		`"` + removed.ID + `": {"read_at"`,
	} {
		if !strings.Contains(state, want) {
			t.Errorf("the state file doesn't hold %s, every other byte as it was:\n%s", want, state)
		}
	}
}

func TestEveryDaysTotalsByAccountAreTheSameMoved(t *testing.T) {
	f := newFixture(t)
	before := f.tallies(t)
	f.moved(t, answering())

	rename := f.renamer(t)
	want := make(map[string]int, len(before))
	for key, n := range before {
		kind, date, account := splitKey(key)
		want[kind+"|"+date+"|"+rename(account)] += n
	}
	if got := f.tallies(t); !maps.Equal(got, want) {
		t.Errorf("the days' totals by account are\n%v\nwant\n%v", got, want)
	}
}

func TestAMoveStoppedAfterAnyStepCarriesOnToTheSameEnd(t *testing.T) {
	clean := newFixture(t)
	var steps []string
	m := clean.mover(answering())
	m.StopAfter(func(step string) error {
		steps = append(steps, step)
		return nil
	})
	if err := m.Run(t.Context()); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	want := clean.snapshot(t)
	if len(steps) < 10 {
		t.Fatalf("the move told of %d steps, %q, want each, and each day's files", len(steps), steps)
	}
	for _, stop := range steps {
		t.Run(stop, func(t *testing.T) {
			f := newFixture(t)
			stopped := f.mover(answering())
			stopped.StopAfter(func(step string) error {
				if step == stop {
					return errors.New("stopped")
				}
				return nil
			})
			if err := stopped.Run(t.Context()); err == nil {
				t.Fatal("Run() stopped nowhere")
			}
			f.moved(t, answering())
			if got := f.snapshot(t); !maps.Equal(got, want) {
				t.Errorf("stopped after %s and run again, the install differs from one never stopped:\n%s", stop, differences(want, got))
			}
		})
	}
}

func TestTheMoveWaitsOnAnAPIThatDoesntAnswerChangingNothing(t *testing.T) {
	f := newFixture(t)
	before := f.snapshot(t)
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	silent := answering()
	silent.unanswered = 1 << 30
	if err := f.mover(silent).Run(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() = %v, want it waiting for the API till its context ended", err)
	}
	if silent.asked < 2 {
		t.Errorf("the API was asked %d times, want it asked again while it didn't answer", silent.asked)
	}
	if got := f.snapshot(t); !maps.Equal(got, before) {
		t.Errorf("waiting on the API, the install changed:\n%s", differences(before, got))
	}

	late := answering()
	late.unanswered = 3
	f.moved(t, late)
	if got := readFile(t, f.config); got != movedConfig {
		t.Errorf("once the API answered, the config reads\n%s\nwant\n%s", got, movedConfig)
	}
}

func TestAnAnswerNamingNoOrganizationMovesAnAccountOntoARandomID(t *testing.T) {
	for _, tc := range []struct {
		name string
		orgs func() *organizations
	}{
		{"naming none", func() *organizations {
			o := answering()
			delete(o.of, personalToken)
			return o
		}},
		{"refusing the token, asked again", func() *organizations {
			o := answering()
			o.refused = map[string]int{personalToken: always}
			return o
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			f.moved(t, tc.orgs())
			e, ok := f.record(t).ByWas(personal)
			if !ok || e.ID == personalID || e.Organization != "" || !identity.WellFormed(e.ID) {
				t.Errorf("personal moved onto %+v, want a random id, its organization unknown", e)
			}
			if _, err := f.tokens.Read(e.ID); err != nil {
				t.Errorf("personal's token by its random id = %v, want it read", err)
			}
		})
	}
}

func TestAnAccountHoldingADerivedIDKeepsIt(t *testing.T) {
	f := newFixture(t)
	writeFile(t, f.config, strings.Replace(fixtureConfig, `id      = "work"`, `id      = "`+workID+`"`, 1))
	if err := os.Rename(f.tokens.Path(work), f.tokens.Path(workID)); err != nil {
		t.Fatal(err)
	}
	if err := identity.NewFile(f.state).Update(func(r *identity.Record) error {
		r.Add(workID, workOrganization, now.Add(-time.Hour))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	f.moved(t, answering())

	e, _ := f.record(t).ByID(workID)
	if e.Was != "" || e.Organization != workOrganization {
		t.Errorf("the record has %s as %+v, want it as it was added", workID, e)
	}
	if p, ok := f.record(t).ByWas(personal); !ok || p.ID != personalID {
		t.Errorf("personal moved onto %+v, want %s", p, personalID)
	}
	if got := readFile(t, f.config); !strings.Contains(got, `id      = "`+workID+`"`) || !strings.Contains(got, `id = "`+personalID+`"`) {
		t.Errorf("the config reads\n%s\nwant %s kept and personal moved onto %s", got, workID, personalID)
	}
}

func TestAnOldIDAnOrganizationWouldDeriveLengthensTheIDItDerives(t *testing.T) {
	f := newFixture(t)
	writeFile(t, f.config, strings.Replace(fixtureConfig, `id      = "work"`, `id      = "`+personalID+`"`, 1))
	for _, name := range []string{"", ".taken"} {
		if err := os.Rename(f.tokens.Path(work)+name, f.tokens.Path(personalID)+name); err != nil {
			t.Fatal(err)
		}
	}
	f.moved(t, answering())

	record := f.record(t)
	w, _ := record.ByWas(personalID)
	p, _ := record.ByWas(personal)
	if w.ID != workID {
		t.Errorf("the account that was %s moved onto %+v, want %s, its organization's", personalID, w, workID)
	}
	if !strings.HasPrefix(p.ID, personalID) || len(p.ID) <= len(personalID) || p.Organization != personalOrganization {
		t.Errorf("personal moved onto %+v, want its organization's id lengthened past %s, an old id", p, personalID)
	}
}

func TestAFreshInstallIsMarkedMovedChangingNothing(t *testing.T) {
	f := fixture{config: filepath.Join(t.TempDir(), "config.toml"), state: t.TempDir()}
	f.tokens = tokens.NewStore(f.state, os.Getuid())
	writeFile(t, f.config, "[[account]]\nid = \""+workID+"\"\nlabel = \"Work\"\n")
	if err := identity.NewFile(f.state).Update(func(r *identity.Record) error {
		r.Add(workID, workOrganization, now)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	before := f.snapshot(t)
	if idmove.Pending(f.state) {
		t.Error("a fresh install's history waits on the move, want it to have none to wait on")
	}
	f.moved(t, answering())

	after := f.snapshot(t)
	delete(after, "accounts.moved")
	if !maps.Equal(after, before) || !idmove.Moved(f.state) {
		t.Errorf("a fresh install changed, or isn't marked moved:\n%s", differences(before, after))
	}
}

func TestAccountsWithoutATokenFile(t *testing.T) {
	f := newFixture(t)
	writeFile(t, f.config, fixtureConfig+"\n[[account]]\nid = \"spare\"\n\n[[account]]\nid = \"side-1\"\n")
	f.moved(t, answering())

	record := f.record(t)
	if e, ok := record.ByID("spare"); !ok || e.Was != "" {
		t.Errorf("spare, which switchboard could have made and the history doesn't name, is %+v, want it kept", e)
	}
	if e, ok := record.ByWas("side-1"); !ok || !identity.WellFormed(e.ID) {
		t.Errorf("side-1, an id switchboard doesn't make, moved onto %+v, want a random id", e)
	}
}

func TestAnAccountAnotherMacMovedTakesItsTokenFileAndHistoryHere(t *testing.T) {
	f := newFixture(t)
	writeFile(t, f.config, movedConfig)
	f.moved(t, answering())

	if got := readFile(t, f.config); got != movedConfig {
		t.Errorf("the config reads\n%s\nwant it as the other Mac moved it", got)
	}
	for was, id := range map[string]string{work: workID, personal: personalID} {
		if e, ok := f.record(t).ByWas(was); !ok || e.ID != id {
			t.Errorf("%s moved onto %+v, want %s", was, e, id)
		}
	}
	if got, want := tokenNames(t, f), movedTokens; !slices.Equal(got, want) {
		t.Errorf("the tokens directory holds %q, want %q", got, want)
	}
	if got := readFile(t, filepath.Join(readings.Dir(f.state), "readings-"+today+".jsonl")); strings.Contains(got, `"work"`) {
		t.Errorf("today's readings still name work:\n%s", got)
	}
}

func TestARouterRestartingOntoTheNewIDsFinishesTheMove(t *testing.T) {
	f := newFixture(t)
	history := f.snapshotOf(t, "ledger", "history", "state.json")
	m := f.mover(answering())
	m.Routing = []string{work, personal}
	if err := m.Run(t.Context()); !errors.Is(err, idmove.ErrAwaitingRestart) {
		t.Fatalf("Run() beside a router on the old ids = %v, want ErrAwaitingRestart", err)
	}
	if got := readFile(t, f.config); got != movedConfig {
		t.Errorf("the config reads\n%s\nwant it moved, for the router to restart onto", got)
	}
	for _, id := range []string{work, personal, workID, personalID} {
		if _, err := f.tokens.Read(id); err != nil {
			t.Errorf("before the restart, %s's token = %v, want each read by its old id and its new", id, err)
		}
	}
	if got := f.snapshotOf(t, "ledger", "history", "state.json"); !maps.Equal(got, history) {
		t.Errorf("before the restart, the history changed:\n%s", differences(history, got))
	}

	restarted := f.mover(answering())
	restarted.Routing = []string{workID, personalID}
	if err := restarted.Settle(); err != nil {
		t.Fatalf("Settle() = %v", err)
	}
	state := readFile(t, filepath.Join(f.state, "state.json"))
	readingsToday := readFile(t, filepath.Join(readings.Dir(f.state), "readings-"+today+".jsonl"))
	if strings.Contains(state, `"work"`) || strings.Contains(readingsToday, `"work"`) {
		t.Errorf("as the router restarted, the state file and today's readings still name work:\n%s\n%s", state, readingsToday)
	}
	if err := restarted.Run(t.Context()); err != nil {
		t.Fatalf("Run() beside the router restarted = %v", err)
	}
	if got, want := tokenNames(t, f), movedTokens; !slices.Equal(got, want) {
		t.Errorf("the tokens directory holds %q, want %q", got, want)
	}
	if got := readFile(t, filepath.Join(f.state, "state.json")); got != state {
		t.Errorf("the move rewrote the state file the router keeps, want it left to the router")
	}
	if !idmove.Moved(f.state) {
		t.Error("the move isn't marked done")
	}
}

// A router keeps appending to today's files as the move rewrites them: here,
// lines under the old id, so each run of the move has lines to rewrite, while
// they're appended. However the two fall, each line appended is kept, and
// the last run renames every one.
func TestLinesAppendedAsTheMoveRunsAreKept(t *testing.T) {
	f := newFixture(t)
	h := idmove.History{
		Ledger:   ledger.Open(ledger.Dir(f.state), dayfile.Forever, clock, ledger.HistoryIn(readingsFiles(f.state)), ledger.Caps{}, logs.For("test")),
		Readings: readingsFiles(f.state),
		Events:   events.Files(ledger.Dir(f.state), logs.For("test")),
	}
	before := f.lineCount(t)
	const mostAppended = 5000
	done := make(chan struct{})
	var appended atomic.Int64
	var appending sync.WaitGroup
	appending.Go(func() {
		for n := range mostAppended {
			select {
			case <-done:
				return
			default:
			}
			appendLine(t, h.Readings, reading(today, work, float64(n)/mostAppended))
			appendLine(t, h.Ledger.Files(), line(today, "late", work, "new", "", nil))
			appended.Add(2)
		}
	})
	m := f.mover(answering())
	m.History, m.Routing = &h, []string{workID, personalID}
	for range 5 {
		f.unmark(t)
		if err := m.Run(t.Context()); err != nil {
			t.Fatalf("Run() = %v", err)
		}
	}
	close(done)
	appending.Wait()
	f.unmark(t)
	if err := m.Run(t.Context()); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if got, want := f.lineCount(t), before+int(appended.Load()); got != want {
		t.Errorf("the history holds %d lines, want the %d it held and the %d appended as it was moved", got, before, appended.Load())
	}
	for _, path := range []string{filepath.Join(readings.Dir(f.state), "readings-"+today+".jsonl"), filepath.Join(ledger.Dir(f.state), "requests-"+today+".jsonl")} {
		if strings.Contains(readFile(t, path), `"`+work+`"`) {
			t.Errorf("%s still names work once the move's last run is done", filepath.Base(path))
		}
	}
}

// appendLine appends line to files, as their writer appends one.
func appendLine(t *testing.T, files *dayfile.Files, line string) {
	lines := make(dayfile.Lines)
	lines.Add(at(today), []byte(line))
	if err := files.Append(lines); err != nil {
		t.Error(err)
	}
}

// unmark takes away the fixture's mark that the move is done, for it to be
// run again.
func (f fixture) unmark(t *testing.T) {
	t.Helper()
	if err := os.Remove(filepath.Join(f.state, "accounts.moved")); err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
}

// tokenNames are the names of what the fixture's tokens directory holds, in
// order.
func tokenNames(t *testing.T, f fixture) []string {
	t.Helper()
	entries, err := os.ReadDir(f.tokens.Dir())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// snapshotOf is the fixture's snapshot of the paths under the state
// directory that begin as given.
func (f fixture) snapshotOf(t *testing.T, prefixes ...string) map[string]string {
	t.Helper()
	of := make(map[string]string)
	for path, text := range f.snapshot(t) {
		if slices.ContainsFunc(prefixes, func(p string) bool { return strings.HasPrefix(path, p) }) {
			of[path] = text
		}
	}
	return of
}

// differences says how two snapshots differ, path by path.
func differences(want, got map[string]string) string {
	var said []string
	for _, path := range slices.Sorted(maps.Keys(want)) {
		if g, ok := got[path]; !ok {
			said = append(said, path+": gone")
		} else if g != want[path] {
			said = append(said, path+":\n  want "+want[path]+"\n  got  "+g)
		}
	}
	for _, path := range slices.Sorted(maps.Keys(got)) {
		if _, ok := want[path]; !ok {
			said = append(said, path+": new\n  "+got[path])
		}
	}
	return strings.Join(said, "\n")
}

// tallies counts what the fixture's history holds, by kind, day and account,
// as "kind|date|account": each day's lines of the readings history, the
// request ledger and the router's events, by each field that names an
// account, as "requests.from" and "events.accounts[]"; the readings' use, in
// thousandths, by account; and each day's requests upstream, input tokens,
// and each window's highest use and rise, in thousandths, by account, as its
// summary has them, and its requests upstream added up over the pools it
// names, each pool's as InPool gives it, so a day whose pools don't add up to
// its whole tallies other than its whole. A day whose compressed file isn't
// gzip at all holds no lines to count.
func (f fixture) tallies(t *testing.T) map[string]int {
	t.Helper()
	counts := make(map[string]int)
	kinds := []struct {
		name   string
		files  *dayfile.Files
		fields []string
	}{
		{"readings", readingsFiles(f.state), []string{"account"}},
		{"requests", ledger.Files(ledger.Dir(f.state), logs.For("test")), []string{"account", "from", "tried[].account"}},
		{"events", events.Files(ledger.Dir(f.state), logs.For("test")), []string{"account", "from", "to", "accounts[]"}},
	}
	for _, kind := range kinds {
		dates, err := kind.files.All()
		if err != nil {
			t.Fatal(err)
		}
		for _, date := range dates {
			if _, _, err := dayfile.ReadDay(kind.files, date, func(l []byte) ([]byte, bool) { return l, true }, func(l []byte) bool {
				for field, accounts := range namedIn(t, l, kind.fields...) {
					for _, account := range accounts {
						counts[kind.name+"."+field+"|"+date+"|"+account]++
					}
				}
				if r, ok := readings.In(l); ok && kind.name == "readings" {
					counts["readings.use|"+date+"|"+r.Account] += int(math.Round(r.Utilization * 1000))
				}
				return true
			}); err != nil && !errors.Is(err, dayfile.ErrDamaged) {
				t.Fatal(err)
			}
		}
	}
	dir := ledger.Dir(f.state)
	dates, err := ledger.SummaryDates(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, date := range dates {
		var s ledger.Summary
		if err := json.Unmarshal([]byte(readFile(t, ledger.SummaryFile(dir, date))), &s); err != nil {
			t.Fatal(err)
		}
		for _, a := range s.Accounts {
			for _, m := range a.Models {
				counts["summary|"+date+"|"+a.Account] += m.Upstream + m.Unsent
				var usage struct {
					Input int `json:"input_tokens"`
				}
				if len(m.Usage) > 0 {
					if err := json.Unmarshal(m.Usage, &usage); err != nil {
						t.Fatal(err)
					}
				}
				counts["summary.input|"+date+"|"+a.Account] += usage.Input
			}
			for window, use := range a.Highest {
				counts["summary.highest."+window+"|"+date+"|"+a.Account] += int(math.Round(use * 1000))
			}
			for window, rise := range a.Rise {
				counts["summary.rise."+window+"|"+date+"|"+a.Account] += int(math.Round(rise * 1000))
			}
			for _, pool := range poolsOf(a) {
				for _, m := range a.InPool(pool).Models {
					counts["summary.pools|"+date+"|"+a.Account] += m.Upstream + m.Unsent
				}
			}
		}
	}
	return counts
}

// poolsOf are the pools an account's day has work in: each its Pools list,
// else its one Pool, "" where it names none.
func poolsOf(a ledger.AccountDay) []string {
	if a.Pools == nil {
		return []string{a.Pool}
	}
	pools := make([]string, len(a.Pools))
	for i, p := range a.Pools {
		pools[i] = p.Pool
	}
	return pools
}

// lineCount counts the lines of the readings history and the request ledger,
// as each names an account, or none.
func (f fixture) lineCount(t *testing.T) int {
	t.Helper()
	n := 0
	for key, count := range f.tallies(t) {
		if kind, _, _ := splitKey(key); kind == "readings.account" || kind == "requests.account" {
			n += count
		}
	}
	return n
}

// namedIn are the accounts a line of JSON names in each of the fields given,
// by field: a field by its name, which names one, "" for none; "name[]" for
// each string of the array it holds; and "name[].key" for each string the
// objects of the array it holds hold by key.
func namedIn(t *testing.T, line []byte, fields ...string) map[string][]string {
	t.Helper()
	var values map[string]any
	if err := json.Unmarshal(line, &values); err != nil {
		t.Fatalf("%s: %v", line, err)
	}
	named := make(map[string][]string, len(fields))
	for _, field := range fields {
		name, each, many := strings.Cut(field, "[]")
		if !many {
			s, _ := values[name].(string)
			named[field] = []string{s}
			continue
		}
		items, _ := values[name].([]any)
		for _, item := range items {
			if key, ok := strings.CutPrefix(each, "."); ok {
				object, _ := item.(map[string]any)
				item = object[key]
			}
			if s, ok := item.(string); ok {
				named[field] = append(named[field], s)
			}
		}
	}
	return named
}

// splitKey splits a tally's key into its kind, date and account.
func splitKey(key string) (kind, date, account string) {
	parts := strings.SplitN(key, "|", 3)
	return parts[0], parts[1], parts[2]
}
