package ledger_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/quota"
)

// The pools the tests' requests are routed in, by their ids.
const (
	acme  = "q7mt"
	first = "b2n8"
)

// inPool is l, routed in the pool with the given id.
func inPool(l ledger.Line, pool string) ledger.Line {
	l.Pool = pool
	return l
}

func TestALineIsReadBackNamingThePoolItWasRoutedInOrNone(t *testing.T) {
	state, dir, _ := stateDirs(t)
	pooled, unpooled := inPool(asked("1", on(0, 9, 0)), acme), asked("2", on(0, 10, 0))

	writeAt(t, dir, on(0, 11, 0), noReadings, &pooled, &unpooled)
	var got []string
	for h := range readerAt(state, on(0, 11, 0)).Lines(on(0, 0, 0)) {
		got = append(got, h.Pool)
	}
	if want := []string{acme, ""}; !slices.Equal(got, want) {
		t.Errorf("the lines read back name the pools %q, want %q: the one it was routed in, and none of one routed in none", got, want)
	}
}

// A request is of the pool it recorded, ids compared ignoring case; one
// naming none is the first pool's once the move to pools is done, and
// before, of no pool but none.
func TestARequestIsOfThePoolItRecordedAndOneNamingNoneTheFirsts(t *testing.T) {
	tests := []struct {
		name                  string
		recorded, pool, first string
		want                  bool
	}{
		{name: "the pool it recorded", recorded: acme, pool: acme, first: first, want: true},
		{name: "the pool it recorded, in capitals", recorded: "Q7MT", pool: acme, first: first, want: true},
		{name: "another pool than it recorded", recorded: acme, pool: first, first: first},
		{name: "naming none, the first pool", pool: first, first: first, want: true},
		{name: "naming none, the first pool in capitals", pool: "B2N8", first: first, want: true},
		{name: "naming none, another pool", pool: acme, first: first},
		{name: "naming none, no pool, the first known", pool: "", first: first},
		{name: "naming none, no pool, before the move", pool: "", want: true},
		{name: "naming none, a pool, before the move", pool: acme},
		{name: "a pool, no pool asked", recorded: acme, pool: "", first: first},
	}
	for _, tt := range tests {
		if got := ledger.RecordedIn(tt.recorded, tt.pool, tt.first); got != tt.want {
			t.Errorf("%s: RecordedIn(%q, %q, %q) = %v, want %v", tt.name, tt.recorded, tt.pool, tt.first, got, tt.want)
		}
	}
}

// A follower finds a session's lines on a day it hasn't summarised by the
// sessions its lines name, whatever pools they were routed in.
func TestAFollowerFindsTheSessionsOfADaysLinesInEveryPoolTheyNamed(t *testing.T) {
	tests := []struct {
		name string
		// pools are the pools of session one's request and session two's.
		pools [2]string
	}{
		{name: "in one pool", pools: [2]string{acme, acme}},
		{name: "in two pools", pools: [2]string{first, acme}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state, dir, _ := stateDirs(t)
			two := asked("2", on(0, 10, 0))
			two.Session = "two"
			holdLines(t, dir, date, inPool(asked("1", on(0, 9, 0)), tt.pools[0]), inPool(two, tt.pools[1]))
			f := followerOf(state, &testClock{at: on(1, 10, 0)}, caps)

			for _, session := range []string{"one", "two"} {
				if got := slices.Collect(f.Session(session)); len(got) != 1 || got[0].Session != session {
					t.Errorf("Session(%q) = %+v, want its one line, its day naming it", session, got)
				}
			}
		})
	}
}

func TestAnAccountsDayNamesThePoolItsWorkWasInOrListsItsPoolsWhereItWasInMore(t *testing.T) {
	read := windowsRead{read: true, highest: `{"5h":0.25}`, rise: `{"5h":0.25}`, resets: `[` + resetJSON("5h", on(0, 13, 0), 0.25) + `]`}
	history := readingsOf(workRead(on(0, 9, 0), "5h", 0.25, on(0, 13, 0), quota.StatusAllowed))
	tests := []struct {
		name  string
		lines []ledger.Line
		// want is work's day, as its JSON gives it.
		want string
	}{
		{
			name:  "every request in no pool, as before pools: as before them",
			lines: []ledger.Line{asked("1", on(0, 9, 0)), asked("2", on(0, 10, 0))},
			want:  workDayJSON(read, on(0, 9, 0), on(0, 10, 0)),
		},
		{
			name:  "every request in one pool: named by it, once",
			lines: []ledger.Line{inPool(asked("1", on(0, 9, 0)), acme), inPool(asked("2", on(0, 10, 0)), acme)},
			want:  `{"account":"work","pool":"` + acme + `",` + workJSON(on(0, 9, 0), on(0, 10, 0)) + read.json() + `}`,
		},
		{
			name:  "moved from one pool to another partway: every pool's work together, then each pool's, and its windows once",
			lines: []ledger.Line{inPool(asked("1", on(0, 9, 0)), first), inPool(asked("2", on(0, 10, 0)), acme)},
			want: `{"account":"work",` + workJSON(on(0, 9, 0), on(0, 10, 0)) + `,"pools":[{"pool":"` + first + `",` + workJSON(on(0, 9, 0)) + `},` +
				`{"pool":"` + acme + `",` + workJSON(on(0, 10, 0)) + `}]` + read.json() + `}`,
		},
		{
			name:  "some requests in no pool, and some in one: no pool its own, never mixed with the pool's, and first",
			lines: []ledger.Line{inPool(asked("1", on(0, 9, 0)), acme), asked("2", on(0, 10, 0))},
			want: `{"account":"work",` + workJSON(on(0, 9, 0), on(0, 10, 0)) + `,"pools":[{` + workJSON(on(0, 10, 0)) + `},` +
				`{"pool":"` + acme + `",` + workJSON(on(0, 9, 0)) + `}]` + read.json() + `}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := string(summaryJSON(t, history, tt.lines...))
			if want := `{"version":7,"day":"2026-10-05","lines":2,"accounts":[` + tt.want + `]}`; got != want {
				t.Errorf("the summary is\n%s\nwant\n%s", got, want)
			}
		})
	}
}

// busyDay is a day of every kind of line a summary counts: requests of two
// models and an inference geo, a quota check and a count of tokens, one the
// router answered itself and one whose body couldn't be read, of no account,
// one whose answer gave no usage, and sessions moved from one account to
// another, and from an account that answered nothing that day; with work's
// window read through it, and reset in it.
func busyDay() ([]ledger.Line, heldReadings) {
	answered := func(request, session, account, model string, at time.Time) ledger.Line {
		return ledger.Line{At: at, Request: request, Kind: ledger.KindMessage, Session: session, Model: model, Account: account, Reason: "sticky",
			Status: 200, Attempts: 1, Usage: usage(`{"input_tokens":10,"cache_creation":{"ephemeral_1h_input_tokens":4},"output_tokens":20,` +
				`"iterations":[{"type":"message","input_tokens":10}],"service_tier":"standard"}`)}
	}
	moved := func(l ledger.Line, from string) ledger.Line {
		l.From, l.Reason = from, "moved: "+from+" hit its limit"
		return l
	}
	geo := answered("4", "two", "work", opus, on(0, 11, 0))
	geo.Shape.InferenceGeo = "us"
	check := answered("5", "resuming", "work", haiku, on(0, 11, 5))
	check.Kind = ledger.KindCheck
	count := answered("6", "one", "work", opus, on(0, 11, 6))
	count.Kind, count.Usage = ledger.KindCount, nil
	unmetered := answered("7", "one", "work", opus, on(0, 11, 7))
	unmetered.Status, unmetered.Usage = 529, nil
	lines := []ledger.Line{
		answered("1", "one", "work", opus, on(0, 9, 0)),
		answered("2", "one", "work", haiku, on(0, 9, 30)),
		answered("3", "two", "side", opus, on(0, 10, 0)),
		geo, check, count, unmetered,
		moved(answered("8", "two", "work", opus, on(0, 14, 0)), "side"),
		moved(answered("9", "three", "side", opus, on(0, 15, 0)), "gone"),
		moved(answered("10", "one", "side", haiku, on(0, 16, 0)), "work"),
		{At: on(0, 17, 0), Request: "11", Kind: ledger.KindMessage, Session: "four", Model: opus, Reason: "no account has room", Status: 429},
		{At: on(0, 17, 1), Request: "12", Kind: ledger.KindMessage, Session: "five", Status: 413},
	}
	history := readingsOf(workRead(on(-1, 20, 0), "5h", 0.1, on(0, 1, 0), quota.StatusAllowed),
		workRead(on(0, 9, 0), "5h", 0.2, on(0, 13, 0), quota.StatusAllowed), workRead(on(0, 14, 0), "5h", 0.5, on(0, 18, 0), quota.StatusAllowed))
	return lines, history
}

// pooled is lines, each routed in the pool pool gives it, by its place.
func pooled(lines []ledger.Line, pool func(i int) string) []ledger.Line {
	routed := make([]ledger.Line, len(lines))
	for i, l := range lines {
		routed[i] = inPool(l, pool(i))
	}
	return routed
}

// unsplit is s, its accounts' days without their pools, as a summary before
// pools gave them.
func unsplit(s ledger.Summary) ledger.Summary {
	accounts := make([]ledger.AccountDay, len(s.Accounts))
	for i, a := range s.Accounts {
		a.Pool, a.Pools = "", nil
		accounts[i] = a
	}
	s.Accounts = accounts
	return s
}

// splits are ways the lines of busyDay can be routed in pools, each line's
// pool by its place.
var splits = []struct {
	name string
	pool func(i int) string
}{
	{name: "every line in one pool", pool: func(int) string { return acme }},
	{name: "the first half in no pool and the rest in one, as an account moved partway", pool: func(i int) string {
		if i < 6 {
			return ""
		}
		return acme
	}},
	{name: "every other line in another pool", pool: func(i int) string { return []string{first, acme}[i%2] }},
	{name: "every line in a pool of its own", pool: func(i int) string { return string(rune('a' + i)) }},
}

func TestEveryAccountsDayIsTheSameWhicheverWayItsWorkSplitsByPool(t *testing.T) {
	lines, history := busyDay()
	whole := summarised(t, history, lines...)
	for _, tt := range splits {
		t.Run(tt.name, func(t *testing.T) {
			if got := summarised(t, history, pooled(lines, tt.pool)...); !reflect.DeepEqual(unsplit(got), whole) {
				t.Errorf("the accounts' days split by pool are\n%+v\nwant, but for their pools,\n%+v", got, whole)
			}
		})
	}
}

func TestEachPoolsWorkIsThatOfTheLinesThatNamedItAloneAndItsWindowsItsAccounts(t *testing.T) {
	lines, history := busyDay()
	whole := summarised(t, history, lines...)
	for _, tt := range splits {
		t.Run(tt.name, func(t *testing.T) {
			routed := pooled(lines, tt.pool)
			got := summarised(t, history, routed...)
			for _, pool := range poolsOf(routed) {
				alone := summarised(t, history, slices.DeleteFunc(slices.Clone(routed), func(l ledger.Line) bool { return l.Pool != pool })...)
				for _, a := range got.InPool(pool, "").Accounts {
					if want := workOf(alone, whole, a.Account); !reflect.DeepEqual(a.Work, want) {
						t.Errorf("%q's work in pool %q is\n%+v\nwant that of the pool's lines alone\n%+v", a.Account, pool, a.Work, want)
					}
					if w := accountOf(whole, a.Account); !sameWindows(a, w) {
						t.Errorf("%q's windows in pool %q are\n%+v\nwant its own, whatever pool it's in\n%+v", a.Account, pool, a, w)
					}
				}
			}
		})
	}
}

// poolsOf are the pools lines were routed in, by their ids, each once, in
// order.
func poolsOf(lines []ledger.Line) []string {
	var pools []string
	for _, l := range lines {
		pools = append(pools, l.Pool)
	}
	slices.Sort(pools)
	return slices.Compact(pools)
}

// workOf is the work of the account with the given id in alone, a day's
// summary of some of its lines: none where it has none there, its sessions
// named as whole, the day's summary of every line, names its.
func workOf(alone, whole ledger.Summary, account string) ledger.Work {
	if a, ok := find(alone, account); ok {
		return a.Work
	}
	if w, _ := find(whole, account); w.SessionIDs != nil {
		return ledger.Work{SessionIDs: []string{}}
	}
	return ledger.Work{}
}

// accountOf is the day of the account with the given id in s.
func accountOf(s ledger.Summary, account string) ledger.AccountDay {
	a, _ := find(s, account)
	return a
}

// find returns the day of the account with the given id in s, reporting
// whether it holds one.
func find(s ledger.Summary, account string) (ledger.AccountDay, bool) {
	i := slices.IndexFunc(s.Accounts, func(a ledger.AccountDay) bool { return a.Account == account })
	if i < 0 {
		return ledger.AccountDay{}, false
	}
	return s.Accounts[i], true
}

// sameWindows reports whether a and b hold the same of their accounts'
// windows.
func sameWindows(a, b ledger.AccountDay) bool {
	a.Work, a.Pool, a.Pools = ledger.Work{}, "", nil
	b.Work, b.Pool, b.Pools = ledger.Work{}, "", nil
	return reflect.DeepEqual(a, b)
}

// noWork is the work of an account's day in a pool it did none in, its
// sessions named, as a summary names them since version 2.
var noWork = ledger.Work{SessionIDs: []string{}}

func TestTheDayOfTheRequestsNoAccountAnsweredSplitsByPoolToo(t *testing.T) {
	refused := func(request, session string, at time.Time) ledger.Line {
		return ledger.Line{At: at, Request: request, Kind: ledger.KindMessage, Session: session, Model: opus, Reason: "no account has room", Status: 429}
	}
	got := summarised(t, noReadings, refused("1", "one", on(0, 9, 0)), inPool(refused("2", "two", on(0, 10, 0)), acme),
		inPool(refused("3", "three", on(0, 11, 0)), acme), inPool(asked("4", on(0, 12, 0)), acme))

	none, ok := find(got, "")
	if !ok || len(none.Pools) != 2 || none.Pools[0].Pool != "" || none.Pools[1].Pool != acme {
		t.Fatalf("the day of no account is %+v, want it split between no pool and %q", none, acme)
	}
	for pool, want := range map[string][]string{"": {"one"}, acme: {"three", "two"}} {
		a, ok := find(got.InPool(pool, ""), "")
		if !ok || !slices.Equal(a.SessionIDs, want) || len(a.Models) != 1 || a.Models[0].Unsent != len(want) {
			t.Errorf("the day of no account in pool %q is %+v, want the %d requests of %q the router answered itself there", pool, a, len(want), want)
		}
	}
	if a, ok := find(got.InPool(first, ""), ""); ok {
		t.Errorf("the day in a pool the router answered nothing itself in holds a day of no account, %+v; want none", a)
	}
	if a, _ := find(got.InPool("", ""), "work"); !reflect.DeepEqual(a.Work, noWork) {
		t.Errorf("work's day in no pool is %+v, want no work, as it served none outside %q", a, acme)
	}
}

func TestASummaryBeforePoolsReadsAsItDidAndAsOneOfNoPool(t *testing.T) {
	held := ofVersion(summaryOf(date, 2, windowsRead{highest: `{"5h":0.9}`}, theTwo...), 6)
	state, dir, _ := stateDirs(t)
	writeFile(t, dir, "day-"+date+".json", []byte(held+"\n"))

	days := readerAt(state, on(3, 10, 0)).Days(on(0, 0, 0))
	if got := summariesJSON(t, days[:1])[0]; got != held {
		t.Fatalf("the day's summary, its lines gone, reads as\n%s\nwant it as it's written\n%s", got, held)
	}
	day := days[0]
	if got := day.InPool("", ""); !reflect.DeepEqual(got, day) {
		t.Errorf("the day in no pool is\n%+v\nwant the whole of it\n%+v", got, day)
	}
	if a, ok := find(day.InPool(acme, ""), "work"); !ok || !reflect.DeepEqual(a.Work, noWork) || !sameWindows(a, accountOf(day, "work")) {
		t.Errorf("work's day in pool %q is %+v, want no work, and its windows as they are", acme, a)
	}
}

// Once the move to pools is done, work in no pool left in a summary, as one
// of a day whose lines are gone, is the first pool's: counted in it,
// alongside the first's own, and in no pool of its own.
func TestASummarysWorkInNoPoolIsTheFirstPoolsOnceTheMoveIsDone(t *testing.T) {
	day := summarised(t, noReadings, asked("1", on(0, 9, 0)), inPool(asked("2", on(0, 10, 0)), first), inPool(asked("3", on(0, 11, 0)), acme))

	if a, _ := find(day.InPool(first, first), "work"); len(a.Models) != 1 || a.Models[0].Upstream != 2 {
		t.Errorf("work's day in the first pool is %+v, want its request in no pool counted with the first's own, two", a)
	}
	if a, _ := find(day.InPool("", first), "work"); len(a.Models) != 0 {
		t.Errorf("work's day in no pool is %+v, want none, its work there the first pool's", a)
	}
	if a, _ := find(day.InPool(acme, first), "work"); len(a.Models) != 1 || a.Models[0].Upstream != 1 {
		t.Errorf("work's day in %q is %+v, want its one request there alone", acme, a)
	}
}

// A pool's work in a summary is found by its id in any case, as the config
// compares pool ids: lines recording one pool's id in two cases, as before
// and after its id was written again in capitals, are its work together, in
// the first pool too, as work naming no pool is the first's.
func TestAPoolsWorkInASummaryIsFoundByItsIDInAnyCase(t *testing.T) {
	day := summarised(t, noReadings, inPool(asked("1", on(0, 9, 0)), acme), inPool(asked("2", on(0, 10, 0)), "Q7MT"),
		asked("3", on(0, 11, 0)), inPool(asked("4", on(0, 12, 0)), "B2N8"))
	tests := []struct {
		pool string
		want int
	}{
		{pool: acme, want: 2},
		{pool: "Q7mt", want: 2},
		{pool: first, want: 2},
	}
	for _, tt := range tests {
		if a, _ := find(day.InPool(tt.pool, first), "work"); len(a.Models) != 1 || a.Models[0].Upstream != tt.want {
			t.Errorf("work's day in pool %q is %+v, want its %d requests there, whatever the case of the id they recorded", tt.pool, a, tt.want)
		}
	}
}

func TestAPoolsWorkKnowsNoLessWhenItsDayIsSummarisedAgain(t *testing.T) {
	// The day's summary before, of work's three requests in acme.
	held := `{"version":7,"day":"2026-10-05","lines":3,"accounts":[{"account":"work","pool":"` + acme + `",` +
		workJSON(on(0, 9, 0), on(0, 10, 0), on(0, 11, 0)) + `}]}`
	inFirst := func(request string, minute int) ledger.Line { return inPool(asked(request, on(0, 14, minute)), first) }
	checkInFirst := func(request string, minute int) ledger.Line {
		l := inFirst(request, minute)
		l.Kind = ledger.KindCheck
		return l
	}
	tests := []struct {
		name  string
		lines []ledger.Line
		// want are work's requests upstream in every pool together, and in
		// each.
		want  int
		pools map[string]int
	}{
		{
			name:  "two of the pool's requests lost since, and three in another filed since: each pool's the more of the two, the whole their sum",
			lines: []ledger.Line{inPool(asked("1", on(0, 9, 0)), acme), inFirst("4", 0), inFirst("5", 1), inFirst("6", 2)},
			want:  6, pools: map[string]int{acme: 3, first: 3},
		},
		{
			name:  "every one of the pool's requests lost since, and four in another filed since: the pool kept as it was, the whole their sum",
			lines: []ledger.Line{inFirst("4", 0), inFirst("5", 1), inFirst("6", 2), inFirst("7", 3)},
			want:  7, pools: map[string]int{acme: 3, first: 4},
		},
		{
			// The checks have its lines number more than held's: fewer, the
			// summary stands, as to a damaged file.
			name:  "two of the pool's requests lost since, and one in another filed since, with two checks: each pool's the more of the two, the whole their sum",
			lines: []ledger.Line{inPool(asked("1", on(0, 9, 0)), acme), inFirst("4", 0), checkInFirst("5", 1), checkInFirst("6", 2)},
			want:  4, pools: map[string]int{acme: 3, first: 1},
		},
	}
	for _, tt := range tests {
		for _, by := range []string{"the router", "a reader"} {
			t.Run(tt.name+", by "+by, func(t *testing.T) {
				state, dir, _ := stateDirs(t)
				writeFile(t, dir, "day-"+date+".json", []byte(held+"\n"))
				holdLines(t, dir, date, tt.lines...)

				var got ledger.Summary
				if err := json.Unmarshal([]byte(summaryAgain(t, by, state, dir, on(3, 10, 0), nil)), &got); err != nil {
					t.Fatal(err)
				}
				work := accountOf(got, "work")
				pools := upstreamByPool(work)
				if len(work.Models) != 1 || work.Models[0].Upstream != tt.want || !reflect.DeepEqual(pools, tt.pools) {
					t.Fatalf("work's day is %+v, its requests upstream by pool %v; want %d in all, %v", work, pools, tt.want, tt.pools)
				}
			})
		}
	}
}
