package cli_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/launch"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// A config naming no pools once the move is done, the router down, has the
// verbs that probe, count or list pools take them as the record keeps them,
// as those reading the files do: the probe narrowed to the pool asked, one
// not among them refused, and the pool named once there's a second.
func TestTheVerbsProbingCountingOrListingPoolsTakeThemAsTheyStand(t *testing.T) {
	deps := unpooledDeps(t, unpooledConfig, true)
	probed := []struct {
		args []string
		want []string
		pool string
	}{
		{args: nil, want: []string{"side"}, pool: "b2n8"},
		{args: []string{"--pool", "q7mt"}, want: []string{"work"}, pool: "q7mt"},
	}
	for _, tt := range probed {
		got := run(t, deps, append([]string{"status", "--probe", "--json"}, tt.args...)...)
		var doc status.Document
		if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil {
			t.Fatalf("switchboard status --probe --json %s = %+v: %v", strings.Join(tt.args, " "), got, err)
		}
		var ids []string
		for _, a := range doc.Accounts {
			ids = append(ids, a.ID)
		}
		if !slices.Equal(ids, tt.want) || doc.PoolID != tt.pool {
			t.Errorf("switchboard status --probe --json %s probed %q in pool %q, want %q in %q", strings.Join(tt.args, " "), ids, doc.PoolID, tt.want, tt.pool)
		}
	}
	if got := run(t, deps, "status", "--probe", "--json", "--pool", "z5k2"); got.code == 0 || !strings.Contains(got.stderr, `there's no pool "z5k2"`) {
		t.Errorf("switchboard status --probe --pool z5k2 = %+v, want it refused, naming the pools", got)
	}
	headed := []struct {
		args []string
		want string
	}{
		{args: []string{"status", "--probe", "--pretty", "--pool", "q7mt"}, want: "pool Acme\n\n"},
		{args: []string{"usage", "--probe", "--pretty", "--pool", "q7mt"}, want: "\npool Acme\n\n"},
		{args: []string{"accounts"}, want: "POOL"},
		{args: []string{"pool", "--pretty"}, want: "Acme"},
	}
	for _, tt := range headed {
		if got := run(t, deps, tt.args...); got.code != 0 || !strings.Contains(got.stdout, tt.want) {
			t.Errorf("switchboard %s = %+v, want it to print %q", strings.Join(tt.args, " "), got, tt.want)
		}
	}
	none := unpooledDeps(t, unpooledConfig, false)
	if got := run(t, none, "status", "--probe", "--json"); got.code == 0 || !strings.Contains(got.stderr, "can't tell which pool to read: "+launch.NoPoolsNamed) {
		t.Errorf("switchboard status --probe, nothing naming the pools, = %+v, want it refused, probing none", got)
	}
	if got := run(t, none, "pool", "--pretty"); got.stdout != "no pools: "+launch.NoPoolsNamed+"\n" {
		t.Errorf("switchboard pool --pretty, nothing naming the pools, = %+v, want it to say so", got)
	}
}

// A config naming no pools once the move is done labels a pool as the record
// keeps it, as this Mac's config last wrote it, in every verb's JSON alike.
func TestThePoolsOfAConfigNamingNoneAreLabelledAsTheRecordKeepsThem(t *testing.T) {
	deps := unpooledDeps(t, unpooledConfig, true)
	for _, args := range [][]string{{"requests"}, {"events"}} {
		got := run(t, deps, append(args, "--json", "--pool", "q7mt")...)
		labels := fieldsOf(t, got, "pool_label")
		if got.code != 0 || !slices.Contains(labels, "Acme") || slices.Contains(labels, "q7mt") {
			t.Errorf("switchboard %s --json --pool q7mt labelled the pools %q (%+v), want Acme, as the record keeps it", strings.Join(args, " "), labels, got)
		}
	}
	got := run(t, deps, "history", "--json", "--pool", "q7mt")
	if accounts := historyAccounts(t, got); !slices.Equal(accounts, []string{"work q7mt Acme s"}) {
		t.Errorf("switchboard history --json --pool q7mt gave the accounts %q, want work's in Acme, labelled as the record keeps it", accounts)
	}
}

// requests --account without --pool reads the account's requests in the
// pool it's in now, as a verb naming accounts takes their pool from them,
// none asked for where every pool is fenced: so one moved from Default into
// Acme has its requests in Acme listed, never Default's with them; with
// --pool, those in that pool alone.
func TestRequestsOfAnAccountAreReadInThePoolItsInNow(t *testing.T) {
	for _, fenced := range []bool{false, true} {
		deps := pooledDeps(t, fenced)
		appendRequests(t, deps, ledger.Line{At: october(7, 8, 0, 0), Request: "p0", Session: sessionC, Account: "work", Pool: "b2n8"})
		tests := []struct {
			args []string
			want []string
		}{
			{args: []string{"--account", "work"}, want: []string{"p2"}},
			{args: []string{"--account", "work", "--pool", "b2n8"}, want: []string{"p0"}},
			{args: []string{"--account", "side"}, want: []string{"p1", "p3"}},
			{args: []string{"--account", "side", "--pool", "q7mt"}},
		}
		for _, tt := range tests {
			got := run(t, deps, append([]string{"requests", "--json"}, tt.args...)...)
			if requests := fieldsOf(t, got, "request"); got.code != 0 || !slices.Equal(requests, tt.want) {
				t.Errorf("every pool fenced %v: switchboard requests --json %s printed %q (%+v), want %q", fenced, strings.Join(tt.args, " "), requests, got, tt.want)
			}
		}
	}
}

// Once the first pool the accounts record notes is no longer configured, as
// after pool remove of an emptied Default, the work recorded in no pool is
// the config's first pool's, as the router routes it: never in no pool.
func TestWorkInNoPoolIsTheConfigsFirstOnceTheOneNotedIsRemoved(t *testing.T) {
	deps := pooledDeps(t, false)
	path, err := config.Path(deps.Getenv, deps.HomeDir)
	if err != nil {
		t.Fatal(err)
	}
	writeStateFile(t, filepath.Dir(path), filepath.Base(path), []byte("[[account]]\nid = \"work\"\nlabel = \"Work\"\npool = \"q7mt\"\n\n"+
		"[[account]]\nid = \"side\"\nlabel = \"Side\"\npool = \"q7mt\"\n\n[[pool]]\nid = \"q7mt\"\nlabel = \"Acme\"\n"))
	tests := []struct {
		args  []string
		field string
		want  []string
	}{
		{args: []string{"requests"}, field: "request", want: []string{"p1", "p2"}},
		{args: []string{"events"}, field: "kind", want: []string{"started", "started", "limit", "pin", "restart"}},
	}
	for _, tt := range tests {
		got := run(t, deps, append(tt.args, "--json")...)
		if values := fieldsOf(t, got, tt.field); got.code != 0 || !slices.Equal(values, tt.want) {
			t.Errorf("switchboard %s --json printed %s %q (%+v), want %q: those from before pools Acme's, the first now", strings.Join(tt.args, " "), tt.field, values, got, tt.want)
		}
	}
	// The ledger's days give side's request from before pools to Acme too.
	got := run(t, deps, "history", "--json", "--pool", "q7mt", "--since", "2026-10-07")
	if accounts := historyAccounts(t, got); !slices.Contains(accounts, "side q7mt Acme s") {
		t.Errorf("switchboard history --json --pool q7mt gave the accounts %q, want side's session from before pools in Acme", accounts)
	}
}

// A damaged accounts record is read for the first pool once a verb, and
// warned of once, by the pool's read and the ledger's alike.
func TestTheFirstPoolIsReadOnceAVerb(t *testing.T) {
	deps := pooledDeps(t, false)
	writeStateFile(t, stateDir(t, deps), "accounts.json", []byte("{\n"))
	if got := run(t, deps, "history", "--json", "--since", "2026-10-07"); got.code != 0 {
		t.Fatalf("switchboard history --json = %+v, want its days", got)
	}
	if log := readLog(t, deps, "cli.log"); strings.Count(log, "first pool") != 1 {
		t.Errorf("cli.log reads\n%s\nwant the first pool unread warned of once", log)
	}
}

// The dashboard's ledger reads the first pool afresh at each ask, as it may
// stay open across the move to pools: opened before there's a history to
// wait on it, its work in no pool is in no pool until the move notes the
// first pool, then the first's.
func TestTheDashboardsLedgerReadsTheFirstPoolAfresh(t *testing.T) {
	cfg, state := dashboardBeforePools(t)
	if n := sideInDefault(cfg.Ledger); n != 1 {
		t.Fatalf("side's day has %d sessions in Default before the move to pools, want p3's alone, 1", n)
	}
	markMoved(t, state)
	if n := sideInDefault(cfg.Ledger); n != 2 {
		t.Errorf("side's day has %d sessions in Default once the move notes it the first, want p1's and p3's, 2", n)
	}
}

// A turn's ledger reads the first pool afresh at each ask too, as the one
// the dashboard opens with does: made before the move to pools, its work in
// no pool is the first's once the move notes it.
func TestATurnsLedgerReadsTheFirstPoolAfresh(t *testing.T) {
	cfg, state := dashboardBeforePools(t)
	turned, err := cfg.Turn(t.Context(), "", watch.SamePool)
	if err != nil {
		t.Fatalf("the turn to the pool shown = %v, want it", err)
	}
	l := turned.NewLedger()
	if n := sideInDefault(l); n != 1 {
		t.Fatalf("side's day has %d sessions in Default before the move to pools, want p3's alone, 1", n)
	}
	markMoved(t, state)
	if n := sideInDefault(l); n != 2 {
		t.Errorf("side's day has %d sessions in Default once the move notes it the first, want p1's and p3's, 2", n)
	}
}

// dashboardBeforePools is the dashboard opened on a config of two accounts
// and no pools, its accounts moved onto their ids but its history not yet
// moved to pools, and its state directory, whose ledger holds today
// pooledLines: side's p3 in Default, and its p1 in no pool.
func dashboardBeforePools(t *testing.T) (*watch.Config, string) {
	t.Helper()
	deps := testDeps(map[string]string{"XDG_STATE_HOME": shortTempDir(t)}, t.TempDir())
	deps.Now = func() time.Time { return ledgerNow }
	path, err := config.Path(deps.Getenv, deps.HomeDir)
	if err != nil {
		t.Fatal(err)
	}
	writeStateFile(t, filepath.Dir(path), filepath.Base(path), []byte("[[account]]\nid = \"work\"\nlabel = \"Work\"\n\n[[account]]\nid = \"side\"\nlabel = \"Side\"\n"))
	state := stateDir(t, deps)
	markDone(t, state, idsMark)
	cfg := recordWatch(t, &deps)
	if got := run(t, deps, "dashboard"); got != (result{}) {
		t.Fatalf("switchboard dashboard = %+v, want exit status 0 and nothing printed", got)
	}
	filePooledLines(t, state)
	return cfg, state
}

// sideInDefault is how many of side's sessions l reads in Default today.
func sideInDefault(l views.Ledger) int {
	days := l.Days(ledgerNow)
	for _, a := range days[len(days)-1].Accounts {
		if a.Account == "side" {
			return a.InPool("b2n8", "").Sessions
		}
	}
	return 0
}

// The dashboard reads the router's files of events of the pool it shows
// alone, the one the verbs read with none named, so the Log's Events and
// Runway's LATELY, which both read them through it, tell nothing of
// another pool's.
func TestTheDashboardReadsTheEventsOfThePoolItShowsAlone(t *testing.T) {
	deps := pooledDeps(t, false)
	fileEvent(t, deps, "2026-10-07", eventLine(runOne, 6, status.Event{At: october(7, 12, 10, 0), Kind: status.EventCap, Account: "side", Windows: []string{"5h"}}))
	fileEvent(t, deps, "2026-10-07", eventLine(runOne, 7, status.Event{At: october(7, 12, 20, 0), Kind: status.EventCap, Account: "work", Windows: []string{"5h"}}))
	cfg := recordWatch(t, &deps)
	if got := run(t, deps, "dashboard"); got != (result{}) {
		t.Fatalf("switchboard dashboard = %+v, want exit status 0 and nothing printed", got)
	}

	read := views.ReadEventsFrom(cfg.Events, views.RunwayLatelyFrom(ledgerNow), ledgerNow)
	var ids []int
	for _, e := range read.Events() {
		ids = append(ids, e.Event.ID)
	}
	if want := []int{1, 4, 5, 6}; !slices.Equal(ids, want) {
		t.Errorf("the dashboard's events are those of the ids %v, want %v: Default's start, pin and cap, and the router's own", ids, want)
	}
	var told []string
	for _, s := range views.RunwayLately(status.Moment{Now: ledgerNow}, read.Events()) {
		told = append(told, s.Full.String())
	}
	if len(told) != 1 || !strings.HasPrefix(told[0], "side ") {
		t.Errorf("Runway's LATELY tells %q, want side's cap alone", told)
	}
}

// The dashboard's Log names the pool a pin set or cleared with force cleared
// the own pins of where the files hold more than one pool's work: with two
// pools, and with one once the other is removed; but none where there never
// was another.
func TestTheDashboardNamesThePoolsOfItsEventsWhereThereAreSeveral(t *testing.T) {
	deps := pooledDeps(t, false)
	cfg := recordWatch(t, &deps)
	dashboard := func() {
		t.Helper()
		if got := run(t, deps, "dashboard"); got != (result{}) {
			t.Fatalf("switchboard dashboard = %+v, want exit status 0 and nothing printed", got)
		}
	}
	dashboard()
	if cfg.PoolNamed == nil || cfg.PoolNamed("q7mt") != "Acme" || cfg.PoolNamed("b2n8") != "Default" {
		t.Errorf("the dashboard names the pools q7mt and b2n8 %q, want Acme and Default", poolsNamedBy(cfg.PoolNamed, "q7mt", "b2n8"))
	}

	onePoolLeft(t, deps, acmeRemovedEntry)
	dashboard()
	if cfg.PoolNamed == nil || cfg.PoolNamed("q7mt") != "Acme" || cfg.PoolNamed("b2n8") != "Default" {
		t.Errorf("once Acme is removed, the dashboard names the pools q7mt and b2n8 %q, want Acme and Default", poolsNamedBy(cfg.PoolNamed, "q7mt", "b2n8"))
	}

	onePoolLeft(t, deps)
	dashboard()
	if cfg.PoolNamed != nil {
		t.Errorf("with one pool and never another, the dashboard names the pools q7mt and b2n8 %q, want none named", poolsNamedBy(cfg.PoolNamed, "q7mt", "b2n8"))
	}
}

// poolsNamedBy are the labels named gives the pools with the ids given, none
// where named is nil.
func poolsNamedBy(named func(id string) string, ids ...string) []string {
	if named == nil {
		return nil
	}
	var labels []string
	for _, id := range ids {
		labels = append(labels, named(id))
	}
	return labels
}

// Where nothing names the pools, the dashboard reads none of the router's
// files of events, warning why, never every pool's together.
func TestTheDashboardReadsNoEventsWhereNothingNamesThePools(t *testing.T) {
	deps := unpooledDeps(t, unpooledConfig, false)
	cfg := recordWatch(t, &deps)
	if got := run(t, deps, "dashboard"); got != (result{}) {
		t.Fatalf("switchboard dashboard = %+v, want exit status 0 and nothing printed", got)
	}
	if read := views.ReadEventsFrom(cfg.Events, views.RunwayLatelyFrom(ledgerNow), ledgerNow); len(read.Events()) != 0 {
		t.Errorf("the dashboard read %d events, want none", len(read.Events()))
	}
	if log := readLog(t, deps, "cli.log"); !strings.Contains(log, "can't tell which pool the dashboard shows") {
		t.Errorf("cli.log reads\n%s\nwant it to say it can't tell the dashboard's pool", log)
	}
}

// An event naming a pool since removed is no pool's; and where the accounts
// record notes no first pool, work naming none is the config's first pool's,
// as the router routes it.
func TestWorkNamingAPoolRemovedIsNoPoolsAndNoneWithTheFirstUnknownTheConfigsFirsts(t *testing.T) {
	deps := pooledDeps(t, false)
	fileEvent(t, deps, "2026-10-07", eventLine(runOne, 6, status.Event{At: october(7, 12, 30, 0), Kind: status.EventStarted, Session: sessionC, Account: "work", Pool: "x9z9", Reason: status.ReasonNew}))
	for _, args := range [][]string{{"events"}, {"events", "--pool", "q7mt"}} {
		got := run(t, deps, append(args, "--json")...)
		if pools := fieldsOf(t, got, "pool"); got.code != 0 || slices.Contains(pools, "x9z9") {
			t.Errorf("switchboard %s --json printed events of the pools %q (%+v), want none of x9z9, removed", strings.Join(args, " "), pools, got)
		}
	}

	err := identity.NewFile(stateDir(t, deps)).Update(func(r *identity.Record) error {
		r.Pools = []identity.PoolEntry{{ID: "b2n8", Added: testNow, Here: true}, {ID: "q7mt", Added: testNow, Here: true}}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		args  []string
		field string
		want  []string
	}{
		{args: []string{"requests"}, field: "request", want: []string{"p1", "p3"}},
		{args: []string{"requests", "--pool", "q7mt"}, field: "request", want: []string{"p2"}},
		{args: []string{"events"}, field: "kind", want: []string{"started", "pin", "restart"}},
	}
	for _, tt := range tests {
		got := run(t, deps, append(tt.args, "--json")...)
		if values := fieldsOf(t, got, tt.field); got.code != 0 || !slices.Equal(values, tt.want) {
			t.Errorf("switchboard %s --json, the first pool unknown, printed %s %q (%+v), want %q", strings.Join(tt.args, " "), tt.field, values, got, tt.want)
		}
	}
}

// An account that worked in two pools on one day is read in each with its
// work there alone, its requests and its day's sessions alike.
func TestAnAccountsDayInTwoPoolsIsReadInEachByItsOwnWork(t *testing.T) {
	deps := pooledDeps(t, false)
	var lines bytes.Buffer
	for _, l := range []ledger.Line{
		{At: october(6, 9, 0, 0), Request: "p7", Session: sessionA, Account: "work", Pool: "b2n8"},
		{At: october(6, 10, 0, 0), Request: "p8", Session: sessionB, Account: "work", Pool: "q7mt"},
		{At: october(6, 11, 0, 0), Request: "p9", Session: sessionB, Account: "work", Pool: "q7mt"},
	} {
		l.At, l.Kind, l.Model, l.Reason, l.Status, l.Attempts, l.TotalMS, l.Usage = l.At.UTC(), ledger.KindMessage, "claude-opus-5-5", "new", 200, 1, 100, json.RawMessage(unknownUsage)
		data, err := json.Marshal(l)
		if err != nil {
			t.Fatal(err)
		}
		lines.Write(append(data, '\n'))
	}
	writeStateFile(t, ledger.Dir(stateDir(t, deps)), "requests-2026-10-06.jsonl", lines.Bytes())

	requests := []struct {
		args []string
		want []string
	}{
		{args: []string{"requests", "--since", "2026-10-06"}, want: []string{"p7", "p1", "p3"}},
		{args: []string{"requests", "--since", "2026-10-06", "--pool", "q7mt"}, want: []string{"p8", "p9", "p2"}},
	}
	for _, tt := range requests {
		got := run(t, deps, append(tt.args, "--json")...)
		if values := fieldsOf(t, got, "request"); got.code != 0 || !slices.Equal(values, tt.want) {
			t.Errorf("switchboard %s --json printed the requests %q (%+v), want %q", strings.Join(tt.args, " "), values, got, tt.want)
		}
	}
	history := []struct {
		args []string
		want []string
	}{
		{args: nil, want: []string{"work b2n8 Default s", "side b2n8 Default ss"}},
		{args: []string{"--pool", "q7mt"}, want: []string{"work q7mt Acme s", "work q7mt Acme s"}},
	}
	for _, tt := range history {
		got := run(t, deps, append([]string{"history", "--json", "--since", "2026-10-06"}, tt.args...)...)
		if accounts := historyAccounts(t, got); !slices.Equal(accounts, tt.want) {
			t.Errorf("switchboard history --json --since 2026-10-06 %s gave the accounts %q, want %q", strings.Join(tt.args, " "), accounts, tt.want)
		}
	}
}

// With two pools, the text of the verbs reading the files names the pool it
// reads first, as status and usage do, and tells of its accounts alone.
func TestTheVerbsTextNamesThePoolOnceThereIsASecond(t *testing.T) {
	deps := pooledDeps(t, false)
	writeReadings(t, readings.Dir(stateDir(t, deps)),
		readings.Reading{At: october(7, 9, 30, 0), Account: "work", Key: "5h", Utilization: 0.4, ResetsAt: october(7, 14, 0, 0), Status: quota.StatusAllowed},
		readings.Reading{At: october(7, 10, 30, 0), Account: "side", Key: "5h", Utilization: 0.2, ResetsAt: october(7, 15, 0, 0), Status: quota.StatusAllowed})
	for _, args := range [][]string{{"requests"}, {"events"}, {"sessions"}, {"history"}, {"history", "--windows"}} {
		got := run(t, deps, append(args, "--pretty", "--pool", "q7mt")...)
		if got.code != 0 || !strings.HasPrefix(got.stdout, "pool Acme\n\n") || strings.Contains(got.stdout, "Side") {
			t.Errorf("switchboard %s --pretty --pool q7mt = %+v, want it to start naming pool Acme, and tell nothing of Side, in Default", strings.Join(args, " "), got)
		}
	}
	tests := []struct {
		args []string
		want []string
	}{
		{args: nil, want: []string{"side"}},
		{args: []string{"--pool", "q7mt"}, want: []string{"work"}},
	}
	for _, tt := range tests {
		got := run(t, deps, append([]string{"history", "--windows", "--json"}, tt.args...)...)
		var doc struct {
			Windows []struct {
				Account string `json:"account"`
			} `json:"windows"`
		}
		if err := json.Unmarshal([]byte(got.stdout), &doc); err != nil {
			t.Fatalf("switchboard history --windows --json %s = %+v: %v", strings.Join(tt.args, " "), got, err)
		}
		var accounts []string
		for _, w := range doc.Windows {
			accounts = append(accounts, w.Account)
		}
		if !slices.Equal(accounts, tt.want) {
			t.Errorf("switchboard history --windows --json %s gave the windows of %q, want %q: the pool's accounts' alone", strings.Join(tt.args, " "), accounts, tt.want)
		}
	}
}
