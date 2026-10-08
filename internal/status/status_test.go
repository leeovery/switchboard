package status_test

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/score"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens/tokenstest"
)

// policy scores the windows as Claude's are: the session and the week apply
// to every model, the week is perishable, and the session's reset decides
// between accounts scoring near enough equal.
var policy = score.Policy{Shared: []string{"5h", "7d"}, Perishable: "7d", Tiebreak: "5h", Started: "5h", Pressure: "5h"}

func TestCollect(t *testing.T) {
	now := time.Date(2026, 9, 28, 14, 12, 0, 0, time.FixedZone("UTC+1", 60*60))
	workUsage := quota.Usage{
		Windows: []quota.Window{
			{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: time.Date(2026, 9, 28, 18, 10, 0, 0, time.UTC)},
			{Key: "7d", Label: "Week", Utilization: 0.5, ResetsAt: time.Date(2026, 10, 2, 21, 0, 0, 0, time.UTC)},
		},
		Failures: []quota.Failure{{Label: "Fable", Window: "7d_oi", Error: "HTTP 529 · Overloaded"}},
	}
	prober := &fakeProber{results: map[string]probeResult{
		"test-token-work": {usage: workUsage},
		"test-token-side": {err: errors.New("HTTP 401 · Invalid bearer token")},
	}}
	collector := status.Collector{
		Prober: prober,
		Policy: policy,
		Token:  tokenstest.Files{"work": "test-token-work", "side": " test-token-side\n"}.Read,
		Now:    func() time.Time { return now },
	}

	got := collector.Collect(t.Context(), accounts)
	want := status.Document{
		GeneratedAt: now.UTC(),
		Source:      "probe",
		Best:        "work",
		Accounts: []status.Account{
			{ID: "work", Label: "Work", TokenSet: true, FetchedAt: now.UTC(), Usage: workUsage},
			{ID: "personal", Label: "Personal", TokenSet: false, Error: tokenstest.Missing("personal").Error()},
			{ID: "side", Label: "Side", TokenSet: true, Error: "HTTP 401 · Invalid bearer token"},
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Collect() =\n%+v\nwant\n%+v", got, want)
	}
	if got, want := prober.probed(), []string{"test-token-side", "test-token-work"}; !slices.Equal(got, want) {
		t.Errorf("probed tokens %q, want %q", got, want)
	}
}

func TestCollectGivesThePrimaryAndTheWindowsAtEachReserve(t *testing.T) {
	now := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	session := quota.Window{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: now.Add(5 * time.Hour)}
	week := func(utilization float64) quota.Window {
		return quota.Window{Key: "7d", Label: "Week", Utilization: utilization, ResetsAt: now.Add(24 * time.Hour)}
	}
	collector := status.Collector{
		Prober: &fakeProber{results: map[string]probeResult{
			"test-token-work": withWindows(session, week(0.93)),
			"test-token-side": withWindows(session, week(0.97)),
		}},
		Policy: policy,
		Token:  tokenstest.Files{"work": "test-token-work", "side": "test-token-side"}.Read,
		Now:    func() time.Time { return now },
	}
	accounts := []config.Account{
		{ID: "work", Label: "Work", Primary: true, Reserve: 0.1},
		{ID: "personal", Label: "Personal", Reserve: 0.05},
		{ID: "side", Label: "Side"},
	}

	doc := collector.Collect(t.Context(), accounts)
	if doc.Primary != "work" {
		t.Errorf("Collect().Primary = %q, want work, the account marked primary", doc.Primary)
	}
	want := map[string]status.Account{
		"work":     {ID: "work", Label: "Work", Primary: true, Reserve: 0.1, AtReserve: []string{"7d"}},
		"personal": {ID: "personal", Label: "Personal", Reserve: 0.05},
		"side":     {ID: "side", Label: "Side"},
	}
	for _, got := range doc.Accounts {
		w := want[got.ID]
		if got.Primary != w.Primary || got.Reserve != w.Reserve || !slices.Equal(got.AtReserve, w.AtReserve) {
			t.Errorf("account %s is primary %v, reserve %v, at its reserve in %q; want %v, %v, %q",
				got.ID, got.Primary, got.Reserve, got.AtReserve, w.Primary, w.Reserve, w.AtReserve)
		}
	}
	if doc.Best != "side" {
		t.Errorf("Collect().Best = %q, want side: work's quota would need using first, but its week has reached its reserve", doc.Best)
	}
}

func TestAnAccountsWindowsProjected(t *testing.T) {
	now := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	// The session began two hours ago, 30% used: its use since it started
	// ends it at 75%. The week began five days ago, 50% used: its use since
	// it started ends it at 70%.
	session := quota.Window{Key: "5h", Label: "Session", Utilization: 0.3, ResetsAt: now.Add(3 * time.Hour)}
	week := quota.Window{Key: "7d", Label: "Week", Utilization: 0.5, ResetsAt: now.Add(2 * 24 * time.Hour)}
	watched := status.Pressure{Window: "5h"}
	sinceStarted := func(atReset float64) status.Heading {
		return status.Heading{Kind: score.OnPace, AtReset: atReset}
	}
	tests := []struct {
		name     string
		pressure status.Pressure
		rates    []status.Rate
		window   quota.Window
		want     status.Heading
	}{
		{
			name:     "the session, at its recent rate, running out",
			pressure: watched,
			rates:    []status.Rate{{Window: "5h", Rate: 0.3}},
			window:   session,
			want:     status.Heading{Kind: score.RunsOut, At: now.Add(2*time.Hour + 20*time.Minute), Recent: true},
		},
		{
			name:     "the session, at its recent rate, though its use since it started ends it more used",
			pressure: watched,
			rates:    []status.Rate{{Window: "5h", Rate: 0.05}},
			window:   session,
			want:     status.Heading{Kind: score.OnPace, AtReset: 0.45, Recent: true},
		},
		{
			name:     "the session, without a recent rate, at its use since it started",
			pressure: watched,
			window:   session,
			want:     sinceStarted(0.75),
		},
		{
			name:   "the session, probed, at its use since it started",
			window: session,
			want:   sinceStarted(0.75),
		},
		{
			name:     "the week, at its recent rate, which has it run out sooner",
			pressure: watched,
			rates:    []status.Rate{{Window: "5h", Rate: 0.3}, {Window: "7d", Rate: 0.03}},
			window:   week,
			want:     status.Heading{Kind: score.RunsOut, At: now.Add(16*time.Hour + 40*time.Minute), Recent: true},
		},
		{
			name:     "the week, at its recent rate, which ends it more used",
			pressure: watched,
			rates:    []status.Rate{{Window: "7d", Rate: 0.01}},
			window:   week,
			want:     status.Heading{Kind: score.OnPace, AtReset: 0.98, Recent: true},
		},
		{
			name:     "the week, at its use since it started, its recent rate slower",
			pressure: watched,
			rates:    []status.Rate{{Window: "7d", Rate: 0.001}},
			window:   week,
			want:     sinceStarted(0.7),
		},
		{
			name:     "the week, without a recent rate, at its use since it started",
			pressure: watched,
			rates:    []status.Rate{{Window: "5h", Rate: 0.3}},
			window:   week,
			want:     sinceStarted(0.7),
		},
		{
			name:   "the week, probed, at its use since it started",
			window: week,
			want:   sinceStarted(0.7),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := status.Account{ID: "work", Windows: []quota.Window{session, week}, Pressure: tt.pressure, Rates: tt.rates}
			got := a.Project(tt.window, now)
			if got.Kind != tt.want.Kind || math.Abs(got.AtReset-tt.want.AtReset) > 1e-9 || !got.At.Equal(tt.want.At) || got.Recent != tt.want.Recent {
				t.Errorf("Project() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestWhereAnAccountsWindowRunsOut(t *testing.T) {
	now := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	// The session, 58% used, at 30% an hour lately, reaches its limit in 1h
	// 24m and a 10% reserve in 1h 4m, before it resets in three.
	session := quota.Window{Key: "5h", Label: "Session", Utilization: 0.58, ResetsAt: now.Add(3 * time.Hour)}
	pressed := status.Pressure{Window: "5h", Rate: 0.3, Recent: true}
	rates := []status.Rate{{Window: "5h", Rate: 0.3, Since: now.Add(-30 * time.Minute)}}
	tests := []struct {
		name        string
		reserve     float64
		pin         []string
		window      quota.Window
		want        time.Time
		wantReserve bool
		wantOK      bool
	}{
		{name: "at its limit, without a reserve", window: session, want: now.Add(84 * time.Minute), wantOK: true},
		{name: "where its reserve starts, which holds it back", reserve: 0.1, window: session, want: now.Add(64 * time.Minute), wantReserve: true, wantOK: true},
		{name: "at its limit, the pin spending its reserve", reserve: 0.1, pin: []string{"work"}, window: session, want: now.Add(84 * time.Minute), wantOK: true},
		{name: "not before it resets", reserve: 0.1, window: quota.Window{Key: "5h", Label: "Session", Utilization: 0.1, ResetsAt: now.Add(time.Hour)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := status.Account{ID: "work", Reserve: tt.reserve, Windows: []quota.Window{tt.window}, Pressure: pressed, Rates: rates}
			doc := status.Document{Pin: status.Pin{Accounts: tt.pin}, Accounts: []status.Account{a}}
			got, ok := doc.RunsOut(a, tt.window, now)
			if ok != tt.wantOK || !got.At.Round(time.Second).Equal(tt.want) || (ok && got.Reserve != tt.wantReserve) {
				t.Errorf("RunsOut() = %+v, %v, want at %s, at the reserve %v, %v", got, ok, tt.want, tt.wantReserve, tt.wantOK)
			}
			if ok && (!got.Recent || !got.Since.Equal(now.Add(-30*time.Minute))) {
				t.Errorf("RunsOut() goes by %+v, want the recent rate the words say", got.Heading)
			}
			c := score.Candidate{ID: a.ID, Windows: a.Windows, Reserve: tt.reserve, Rate: pressed.Rate}
			if doc.Pin.Has(a.ID) {
				c.Reserve = 0
			}
			if p := (score.Policy{Pressure: "5h"}).PressureOf(c, now); ok && !got.At.Round(time.Second).Equal(p.RunsOut.Round(time.Second)) {
				t.Errorf("RunsOut() is at %s, want %s, where the router's pressure has it run out", got.At, p.RunsOut)
			}
		})
	}
}

func TestAProjectionStaysPutWhileTheDocumentDoes(t *testing.T) {
	read := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	// The session, 58% used two hours in, heads at its pace for 97% by its
	// reset in three, and at 30% an hour lately reaches its limit in 1h 24m;
	// the week, used at its pace, runs out in a day.
	session := quota.Window{Key: "5h", Label: "Session", Utilization: 0.58, ResetsAt: read.Add(3 * time.Hour)}
	week := quota.Window{Key: "7d", Label: "Week", Utilization: 0.8, ResetsAt: read.Add(4 * 24 * time.Hour)}
	paced := status.Account{ID: "work", FetchedAt: read.Add(-time.Minute), Windows: []quota.Window{session, week}}
	rated := paced
	rated.Rates = []status.Rate{{Window: "5h", Rate: 0.3, Since: read.Add(-30 * time.Minute)}}
	for _, a := range []status.Account{paced, rated} {
		doc := status.Document{GeneratedAt: read, Accounts: []status.Account{a}}
		for _, w := range a.Windows {
			first, ok := doc.RunsOut(a, w, read)
			heading := doc.Project(a, w, read)
			for _, later := range []time.Duration{time.Second, 10 * time.Minute} {
				again, stillOK := doc.RunsOut(a, w, read.Add(later))
				if ok != stillOK || !again.At.Equal(first.At) {
					t.Errorf("%s's %s runs out at %v, %v as read, and at %v, %v %s later, want it put", a.ID, w.Key, first.At, ok, again.At, stillOK, later)
				}
				if got := doc.Project(a, w, read.Add(later)); got != heading {
					t.Errorf("%s's %s heads for %+v as read, and %+v %s later, want it put", a.ID, w.Key, heading, got, later)
				}
			}
		}
	}
	doc := status.Document{GeneratedAt: read, Accounts: []status.Account{paced}}
	if got := doc.Project(paced, session, session.ResetsAt); got != (status.Heading{}) {
		t.Errorf("once its session has reset, it heads for %+v, want nowhere", got)
	}
}

func TestAnAccountAsItStands(t *testing.T) {
	now := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	running := quota.Window{Key: "5h", Label: "Session", Utilization: 0.95, ResetsAt: now.Add(time.Hour), Status: quota.StatusAllowedWarning}
	lapsed := quota.Window{Key: "5h", Label: "Session", Utilization: 0.95, ResetsAt: now.Add(-time.Minute), Status: quota.StatusAllowedWarning}
	empty := quota.Window{Key: "5h", Label: "Session"}
	week := quota.Window{Key: "7d", Label: "Week", Utilization: 0.93, ResetsAt: now.Add(24 * time.Hour), Status: quota.StatusAllowedWarning}
	tests := []struct {
		name    string
		read    []quota.Window
		reserve float64
		// want are its windows as they stand, wantLapsed those that have
		// lapsed, and wantAtReserve those at its reserve.
		want          []quota.Window
		wantLapsed    []string
		wantAtReserve []string
	}{
		{name: "as read, while its session runs", read: []quota.Window{running, week}, want: []quota.Window{running, week}},
		{
			name:       "its session empty once it has lapsed, its week as read",
			read:       []quota.Window{lapsed, week},
			want:       []quota.Window{empty, week},
			wantLapsed: []string{"5h"},
		},
		{
			name:          "at its reserve in its week, and not in its session, which has lapsed",
			read:          []quota.Window{lapsed, week},
			reserve:       0.1,
			want:          []quota.Window{empty, week},
			wantLapsed:    []string{"5h"},
			wantAtReserve: []string{"7d"},
		},
		{
			name:          "at its reserve in both while its session runs",
			read:          []quota.Window{running, week},
			reserve:       0.1,
			want:          []quota.Window{running, week},
			wantAtReserve: []string{"5h", "7d"},
		},
		{name: "nothing, with nothing read"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			read := slices.Clone(tt.read)
			a := status.Account{ID: "work", Label: "Work", Reserve: tt.reserve, TokenSet: true, Windows: read}

			got := a.AsOf(policy, now)
			if !slices.Equal(got.Windows, tt.want) || !slices.Equal(got.Lapsed, tt.wantLapsed) || !slices.Equal(got.AtReserve, tt.wantAtReserve) {
				t.Errorf("AsOf() reads %+v, lapsed %q, at its reserve %q; want %+v, %q, %q",
					got.Windows, got.Lapsed, got.AtReserve, tt.want, tt.wantLapsed, tt.wantAtReserve)
			}
			for _, w := range got.Windows {
				if got.HasLapsed(w) != slices.Contains(tt.wantLapsed, w.Key) {
					t.Errorf("HasLapsed(%s) = %v, want %v", w.Key, got.HasLapsed(w), !got.HasLapsed(w))
				}
			}
			if !slices.Equal(read, tt.read) {
				t.Errorf("the account's windows as read became %+v, want them as they were", read)
			}
		})
	}
}

func TestAnAccountsWindowByItsKey(t *testing.T) {
	session := quota.Window{Key: "5h", Label: "Session", Utilization: 0.23}
	week := quota.Window{Key: "7d", Label: "Week", Utilization: 0.93}
	a := status.Account{ID: "work", Windows: []quota.Window{session, week}}
	tests := []struct {
		key    string
		want   quota.Window
		wantOK bool
	}{
		{key: "5h", want: session, wantOK: true},
		{key: "7d", want: week, wantOK: true},
		{key: "7d_oi"},
	}
	for _, tt := range tests {
		if got, ok := a.Window(tt.key); got != tt.want || ok != tt.wantOK {
			t.Errorf("Window(%q) = %+v, %v; want %+v, %v", tt.key, got, ok, tt.want, tt.wantOK)
		}
	}
}

func TestCollectLogsEachAccount(t *testing.T) {
	log := logstest.Capture(t)
	now := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	collector := status.Collector{
		Prober: &fakeProber{results: map[string]probeResult{
			"test-token-work": {usage: quota.Usage{
				Windows: []quota.Window{
					{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: now.Add(5 * time.Hour)},
					{Key: "7d", Label: "Week", Utilization: 0.5, ResetsAt: now.Add(72 * time.Hour)},
				},
				Failures: []quota.Failure{{Label: "Fable", Window: "7d_oi", Error: "HTTP 529 · Overloaded"}},
			}},
			"test-token-side": {err: errors.New("HTTP 401 · Invalid bearer token")},
		}},
		Policy: policy,
		Token:  tokenstest.Files{"work": "test-token-work", "side": "test-token-side"}.Read,
		Now:    func() time.Time { return now },
	}

	collector.Collect(t.Context(), accounts)
	for _, want := range [][]string{
		{"level=DEBUG", `msg="probed account" component=status`, "account=work", "duration=", "windows=2"},
		{"level=WARN", `msg="window unread" component=status`, "account=work", "window=7d_oi", `error="HTTP 529 · Overloaded"`},
		{"level=DEBUG", `msg="not probed: no usable token" component=status`, "account=personal", `error="` + tokenstest.Missing("personal").Error() + `"`},
		{"level=WARN", `msg="probe failed" component=status`, "account=side", "duration=", `error="HTTP 401 · Invalid bearer token"`},
	} {
		if !log.Has(want...) {
			t.Errorf("log reads\n%s\nwant a line with %q", log, want)
		}
	}
	for _, private := range []string{"test-token-work", "test-token-side", "Work", "Personal", "Side"} {
		if strings.Contains(log.String(), private) {
			t.Errorf("log reads\n%s\nwant no tokens or labels, only ids, but it has %q", log, private)
		}
	}
}

func TestCollectProbesAccountsAtOnce(t *testing.T) {
	collector := status.Collector{
		Prober: &gatheringProber{waitFor: len(accounts), gathered: make(chan struct{})},
		Token:  tokenstest.Files{"work": "test-token-work", "personal": "test-token-personal", "side": "test-token-side"}.Read,
		Now:    time.Now,
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	for _, account := range collector.Collect(ctx, accounts).Accounts {
		if account.Error != "" {
			t.Errorf("account %s: %s", account.ID, account.Error)
		}
	}
}

func TestCollectBest(t *testing.T) {
	now := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	session := quota.Window{Key: "5h", Label: "Session", Utilization: 0.1, ResetsAt: now.Add(3 * time.Hour)}
	sessionEndingSooner := session
	sessionEndingSooner.ResetsAt = now.Add(time.Hour)
	week := func(utilization float64, resetsIn time.Duration) quota.Window {
		return quota.Window{Key: "7d", Label: "Week", Utilization: utilization, ResetsAt: now.Add(resetsIn)}
	}
	weekRefused := week(1, -time.Minute)
	weekRefused.Status = quota.StatusRejected
	fableRefused := quota.Window{Key: "7d_oi", Label: "Fable week", Utilization: 1, ResetsAt: now.Add(48 * time.Hour), Status: quota.StatusRejected}
	unreadable := probeResult{err: errors.New("HTTP 401 · Invalid bearer token")}
	tests := []struct {
		name       string
		work, side probeResult
		// workReserve is the share of work's every window left unused.
		workReserve float64
		want        string
	}{
		{
			name: "the account whose week resets soonest",
			work: withWindows(session, week(0.5, 5*24*time.Hour)),
			side: withWindows(session, week(0.5, 24*time.Hour)),
			want: "side",
		},
		{
			name: "of two scoring near enough equal, the one whose session resets soonest",
			work: withWindows(session, week(0.5, 50*time.Hour)),
			side: withWindows(sessionEndingSooner, week(0.55, 50*time.Hour)),
			want: "side",
		},
		{
			name:        "passing over an account at its reserve",
			work:        withWindows(session, week(0.9, 24*time.Hour)),
			side:        withWindows(session, week(0.5, 5*24*time.Hour)),
			workReserve: 0.1,
			want:        "side",
		},
		{
			name:        "judging an account by the room before its reserve",
			work:        withWindows(session, week(0.5, 24*time.Hour)),
			side:        withWindows(session, week(0.5, 30*time.Hour)),
			workReserve: 0.2,
			want:        "side",
		},
		{
			name: "judged on the windows every model shares",
			work: withWindows(session, week(0.5, 24*time.Hour), fableRefused),
			side: withWindows(session, week(0.5, 5*24*time.Hour)),
			want: "work",
		},
		{
			name: "a refused week that has reset by the clock",
			work: withWindows(session, weekRefused),
			side: unreadable,
			want: "work",
		},
		{
			name: "none when no account can take a request",
			work: withWindows(session, week(1, 24*time.Hour)),
			side: unreadable,
			want: "",
		},
		{
			name: "none when no account could be read",
			work: unreadable,
			side: unreadable,
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := status.Collector{
				Prober: &fakeProber{results: map[string]probeResult{"test-token-work": tt.work, "test-token-side": tt.side}},
				Policy: policy,
				Token:  tokenstest.Files{"work": "test-token-work", "side": "test-token-side"}.Read,
				Now:    func() time.Time { return now },
			}
			accounts := []config.Account{{ID: "work", Label: "Work", Reserve: tt.workReserve}, {ID: "side", Label: "Side"}}

			if got := collector.Collect(t.Context(), accounts).Best; got != tt.want {
				t.Errorf("Collect().Best = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBestOfThePinnedWhileOneHasRoomAndThoseItsChosenAmong(t *testing.T) {
	now := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	session := quota.Window{Key: "5h", Label: "Session", Utilization: 0.1, ResetsAt: now.Add(3 * time.Hour)}
	week := func(utilization float64, resetsIn time.Duration) []quota.Window {
		return []quota.Window{session, {Key: "7d", Label: "Week", Utilization: utilization, ResetsAt: now.Add(resetsIn)}}
	}
	account := func(id string, windows []quota.Window) status.Account {
		return status.Account{ID: id, Label: id, TokenSet: true, Windows: windows}
	}
	// Spare's week needs using first, then side's, then work's. Each
	// session resets in 3 hours: used at pressing an hour, it runs out before
	// then, and at atReserve, it reaches a reserve of a tenth before then,
	// but its limit after.
	const pressing, atReserve = 0.5, 0.28
	accounts := []status.Account{
		account("work", week(0.5, 5*24*time.Hour)),
		account("side", week(0.5, 3*24*time.Hour)),
		account("spare", week(0.5, 24*time.Hour)),
	}
	tests := []struct {
		name   string
		pinned []string
		// change changes the accounts before the best is chosen of them.
		change func(accounts []status.Account)
		want   string
		// wantAmong are the accounts it's chosen among.
		wantAmong []string
	}{
		{name: "unpinned, the best of every account", want: "spare", wantAmong: []string{"work", "side", "spare"}},
		{name: "the one pinned", pinned: []string{"work"}, want: "work", wantAmong: []string{"work"}},
		{name: "the best of those pinned", pinned: []string{"work", "side"}, want: "side", wantAmong: []string{"work", "side"}},
		{
			name:   "one pinned at its reserve, which the pin spends",
			pinned: []string{"work"},
			change: func(accounts []status.Account) {
				accounts[0].Reserve, accounts[0].Windows = 0.1, week(0.95, 5*24*time.Hour)
			},
			want: "work", wantAmong: []string{"work"},
		},
		{
			name:   "the best of every account once none pinned has room",
			pinned: []string{"work", "side"},
			change: func(accounts []status.Account) {
				accounts[0].Windows, accounts[1].Windows = week(1, 5*24*time.Hour), week(1, 3*24*time.Hour)
			},
			want: "spare", wantAmong: []string{"spare"},
		},
		{
			name:   "the first pinned with room when none pinned can be scored",
			pinned: []string{"work", "side"},
			change: func(accounts []status.Account) {
				accounts[0].Windows, accounts[1].Windows = week(1, 5*24*time.Hour), nil
			},
			want: "side", wantAmong: []string{"side"},
		},
		{
			name:   "none pinned without a usable token",
			pinned: []string{"work"},
			change: func(accounts []status.Account) { accounts[0] = status.Account{ID: "work", Label: "work"} },
			want:   "spare", wantAmong: []string{"side", "spare"},
		},
		{
			name:   "unpinned, passing over the best under pressure",
			change: func(accounts []status.Account) { accounts[2].Pressure.Rate = pressing },
			want:   "side", wantAmong: []string{"work", "side"},
		},
		{
			name:   "unpinned, passing over the best under pressure at its reserve",
			change: func(accounts []status.Account) { accounts[2].Reserve, accounts[2].Pressure.Rate = 0.1, atReserve },
			want:   "side", wantAmong: []string{"work", "side"},
		},
		{
			name:   "the best of those pinned, passing over one under pressure for another pinned",
			pinned: []string{"work", "side"},
			change: func(accounts []status.Account) { accounts[1].Pressure.Rate = pressing },
			want:   "work", wantAmong: []string{"work"},
		},
		{
			name:   "the best of those pinned, every one under pressure",
			pinned: []string{"work", "side"},
			change: func(accounts []status.Account) {
				accounts[0].Pressure.Rate, accounts[1].Pressure.Rate = pressing, pressing
			},
			want: "side", wantAmong: []string{"work", "side"},
		},
		{
			name:   "the best of those pinned, under pressure only at its reserve, which the pin spends",
			pinned: []string{"work", "side"},
			change: func(accounts []status.Account) { accounts[1].Reserve, accounts[1].Pressure.Rate = 0.1, atReserve },
			want:   "side", wantAmong: []string{"work", "side"},
		},
		{
			name: "unpinned, passing over one held back by a limit naming no window, its windows with room",
			change: func(accounts []status.Account) {
				accounts[2].Limit = status.Limit{Until: now.Add(time.Hour)}
			},
			want: "side", wantAmong: []string{"work", "side"},
		},
		{
			name:   "of those pinned, passing over one whose token is refused",
			pinned: []string{"work", "side"},
			change: func(accounts []status.Account) {
				accounts[1].Refused = status.Refusal{Until: now.Add(time.Hour), Status: 401}
			},
			want: "work", wantAmong: []string{"work"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			accounts := slices.Clone(accounts)
			if tt.change != nil {
				tt.change(accounts)
			}
			if got := status.Best(policy, accounts, tt.pinned, now); got != tt.want {
				t.Errorf("Best() = %q, want %q", got, tt.want)
			}
			if got := status.Choosable(policy, accounts, tt.pinned, now); !slices.Equal(got, tt.wantAmong) {
				t.Errorf("Choosable() = %q, want %q", got, tt.wantAmong)
			}
		})
	}
}

func TestDocumentJSON(t *testing.T) {
	generated := time.Date(2026, 9, 28, 13, 12, 0, 0, time.UTC)
	tests := []struct {
		name string
		doc  status.Document
		want string
	}{
		{
			name: "with an account to use next",
			doc: status.Document{
				GeneratedAt: generated,
				Source:      status.SourceProbe,
				Best:        "work",
				Accounts: []status.Account{
					{
						ID: "work", Label: "Work", TokenSet: true, FetchedAt: generated,
						Windows: []quota.Window{
							{Key: "5h", Label: "Session", Utilization: 0.23, ResetsAt: time.Date(2026, 9, 28, 18, 10, 0, 0, time.UTC), Status: quota.StatusAllowed},
							{Key: "7d", Label: "Week", Utilization: 0.93},
						},
						Failures: []quota.Failure{{Label: "Fable", Window: "7d_oi", Error: "HTTP 529 · Overloaded"}},
					},
					{ID: "personal", Label: "Personal", Error: "token missing: write it to /Users/tester/.local/state/switchboard/tokens/personal"},
					{ID: "side", Label: "Side", TokenSet: true, Error: "HTTP 401 · Invalid bearer token"},
				},
			},
			want: `{
  "generated_at": "2026-09-28T13:12:00Z",
  "source": "probe",
  "best": "work",
  "accounts": [
    {
      "id": "work",
      "label": "Work",
      "token_set": true,
      "fetched_at": "2026-09-28T13:12:00Z",
      "windows": [
        {
          "key": "5h",
          "label": "Session",
          "utilization": 0.23,
          "resets_at": "2026-09-28T18:10:00Z",
          "status": "allowed"
        },
        {
          "key": "7d",
          "label": "Week",
          "utilization": 0.93
        }
      ],
      "failures": [
        {
          "label": "Fable",
          "window": "7d_oi",
          "error": "HTTP 529 · Overloaded"
        }
      ]
    },
    {
      "id": "personal",
      "label": "Personal",
      "token_set": false,
      "error": "token missing: write it to /Users/tester/.local/state/switchboard/tokens/personal"
    },
    {
      "id": "side",
      "label": "Side",
      "token_set": true,
      "error": "HTTP 401 · Invalid bearer token"
    }
  ]
}`,
		},
		{
			name: "without one",
			doc: status.Document{
				GeneratedAt: generated,
				Source:      status.SourceProbe,
				Accounts:    []status.Account{{ID: "personal", Label: "Personal", Error: "token missing: write it to /Users/tester/.local/state/switchboard/tokens/personal"}},
			},
			want: `{
  "generated_at": "2026-09-28T13:12:00Z",
  "source": "probe",
  "accounts": [
    {
      "id": "personal",
      "label": "Personal",
      "token_set": false,
      "error": "token missing: write it to /Users/tester/.local/state/switchboard/tokens/personal"
    }
  ]
}`,
		},
		{
			name: "probed, as the router isn't running",
			doc: status.Document{
				GeneratedAt: generated,
				Source:      status.SourceProbe,
				Fallback:    status.Fallback{Router: status.RouterNotRunning},
				Accounts:    []status.Account{{ID: "work", Label: "Work", TokenSet: true}},
			},
			want: `{
  "generated_at": "2026-09-28T13:12:00Z",
  "source": "probe",
  "fallback": {
    "router": "not running"
  },
  "accounts": [
    {
      "id": "work",
      "label": "Work",
      "token_set": true
    }
  ]
}`,
		},
		{
			name: "probed, as the router didn't answer as it should",
			doc: status.Document{
				GeneratedAt: generated,
				Source:      status.SourceProbe,
				Fallback:    status.Fallback{Router: status.RouterUnhealthy, Reason: "no answer within 500ms"},
				Accounts:    []status.Account{{ID: "work", Label: "Work", TokenSet: true}},
			},
			want: `{
  "generated_at": "2026-09-28T13:12:00Z",
  "source": "probe",
  "fallback": {
    "router": "unhealthy",
    "reason": "no answer within 500ms"
  },
  "accounts": [
    {
      "id": "work",
      "label": "Work",
      "token_set": true
    }
  ]
}`,
		},
		{
			name: "the router's, with a week started again",
			doc: status.Document{
				GeneratedAt: generated,
				Source:      status.SourceRouter,
				Router:      status.Health{Healthy: true},
				Accounts: []status.Account{{
					ID: "work", Label: "Work", TokenSet: true, FetchedAt: generated,
					Windows: []quota.Window{{
						Key: "7d", Label: "Week", Utilization: 0.01, ResetsAt: time.Date(2026, 10, 2, 21, 0, 0, 0, time.UTC),
						Status: quota.StatusAllowed, RestartedAt: generated.Add(-time.Hour),
					}},
				}},
			},
			want: `{
  "generated_at": "2026-09-28T13:12:00Z",
  "source": "router",
  "router": {
    "healthy": true,
    "requests": 0,
    "failures": 0
  },
  "accounts": [
    {
      "id": "work",
      "label": "Work",
      "token_set": true,
      "fetched_at": "2026-09-28T13:12:00Z",
      "windows": [
        {
          "key": "7d",
          "label": "Week",
          "utilization": 0.01,
          "resets_at": "2026-10-02T21:00:00Z",
          "status": "allowed",
          "restarted_at": "2026-09-28T12:12:00Z"
        }
      ]
    }
  ]
}`,
		},
		{
			name: "the router's, with each account's pressure and recent rates",
			doc: status.Document{
				GeneratedAt: generated,
				Source:      status.SourceRouter,
				Router:      status.Health{Healthy: true},
				Accounts: []status.Account{
					{
						ID: "work", Label: "Work", TokenSet: true,
						Pressure: status.Pressure{Window: "5h", Rate: 0.3, Recent: true, Since: generated.Add(-30 * time.Minute), RunsOut: generated.Add(80 * time.Minute), Under: true},
						Rates:    []status.Rate{{Window: "5h", Rate: 0.3, Since: generated.Add(-30 * time.Minute)}, {Window: "7d", Rate: 0.03, Since: generated.Add(-2 * time.Hour)}},
					},
					{ID: "side", Label: "Side", TokenSet: true, Pressure: status.Pressure{Window: "5h", Rate: 0.1, RunsOut: generated.Add(9 * time.Hour)}},
					{ID: "spare", Label: "Spare", TokenSet: true, Pressure: status.Pressure{Window: "5h"}},
				},
			},
			want: `{
  "generated_at": "2026-09-28T13:12:00Z",
  "source": "router",
  "router": {
    "healthy": true,
    "requests": 0,
    "failures": 0
  },
  "accounts": [
    {
      "id": "work",
      "label": "Work",
      "token_set": true,
      "pressure": {
        "window": "5h",
        "rate": 0.3,
        "recent": true,
        "since": "2026-09-28T12:42:00Z",
        "runs_out": "2026-09-28T14:32:00Z",
        "under": true
      },
      "rates": [
        {
          "window": "5h",
          "rate": 0.3,
          "since": "2026-09-28T12:42:00Z"
        },
        {
          "window": "7d",
          "rate": 0.03,
          "since": "2026-09-28T11:12:00Z"
        }
      ]
    },
    {
      "id": "side",
      "label": "Side",
      "token_set": true,
      "pressure": {
        "window": "5h",
        "rate": 0.1,
        "runs_out": "2026-09-28T22:12:00Z"
      }
    },
    {
      "id": "spare",
      "label": "Spare",
      "token_set": true,
      "pressure": {
        "window": "5h",
        "rate": 0
      }
    }
  ]
}`,
		},
		{
			name: "the router's, with its pin, its health and the sessions",
			doc: status.Document{
				GeneratedAt: generated,
				Source:      status.SourceRouter,
				Pin:         status.Pin{Accounts: []string{"work", "side"}, Since: generated.Add(-time.Hour), Move: true},
				Router:      status.Health{Requests: 8, Failures: 6, Reason: "6 of the 8 requests in the last 5 minutes failed"},
				Sessions:    2,
				Accounts: []status.Account{
					{ID: "work", Label: "Work", TokenSet: true, Sessions: 2},
					{ID: "side", Label: "Side", TokenSet: true, Limit: status.Limit{Windows: []string{"5h"}, Until: generated.Add(2 * time.Hour)}},
				},
			},
			want: `{
  "generated_at": "2026-09-28T13:12:00Z",
  "source": "router",
  "pin": {
    "accounts": [
      "work",
      "side"
    ],
    "account": "work",
    "since": "2026-09-28T12:12:00Z",
    "move": true
  },
  "router": {
    "healthy": false,
    "requests": 8,
    "failures": 6,
    "reason": "6 of the 8 requests in the last 5 minutes failed"
  },
  "sessions": 2,
  "accounts": [
    {
      "id": "work",
      "label": "Work",
      "token_set": true,
      "sessions": 2
    },
    {
      "id": "side",
      "label": "Side",
      "token_set": true,
      "limit": {
        "windows": [
          "5h"
        ],
        "until": "2026-09-28T15:12:00Z"
      }
    }
  ]
}`,
		},
		{
			name: "the router's, with refusals",
			doc: status.Document{
				GeneratedAt: generated,
				Source:      status.SourceRouter,
				Router:      status.Health{Healthy: true, Requests: 2},
				Accounts: []status.Account{
					{ID: "work", Label: "Work", TokenSet: true, Refused: status.Refusal{Until: generated.Add(10 * time.Minute), Status: 401}},
					{ID: "side", Label: "Side", TokenSet: true, Refused: status.Refusal{Until: generated.Add(9 * time.Minute), Status: 403, Family: "opus"}},
				},
			},
			want: `{
  "generated_at": "2026-09-28T13:12:00Z",
  "source": "router",
  "router": {
    "healthy": true,
    "requests": 2,
    "failures": 0
  },
  "accounts": [
    {
      "id": "work",
      "label": "Work",
      "token_set": true,
      "refused": {
        "until": "2026-09-28T13:22:00Z",
        "status": 401
      }
    },
    {
      "id": "side",
      "label": "Side",
      "token_set": true,
      "refused": {
        "until": "2026-09-28T13:21:00Z",
        "status": 403,
        "family": "opus"
      }
    }
  ]
}`,
		},
		{
			name: "with the primary, its reserve, and the windows that have reached it",
			doc: status.Document{
				GeneratedAt: generated,
				Source:      status.SourceProbe,
				Primary:     "work",
				Accounts: []status.Account{
					{
						ID: "work", Label: "Work", Primary: true, Reserve: 0.1, TokenSet: true, FetchedAt: generated,
						Windows:   []quota.Window{{Key: "5h", Label: "Session", Utilization: 0.95, ResetsAt: time.Date(2026, 9, 28, 18, 10, 0, 0, time.UTC)}},
						AtReserve: []string{"5h"},
					},
					{ID: "side", Label: "Side", TokenSet: true},
				},
			},
			want: `{
  "generated_at": "2026-09-28T13:12:00Z",
  "source": "probe",
  "primary": "work",
  "accounts": [
    {
      "id": "work",
      "label": "Work",
      "primary": true,
      "reserve": 0.1,
      "token_set": true,
      "fetched_at": "2026-09-28T13:12:00Z",
      "windows": [
        {
          "key": "5h",
          "label": "Session",
          "utilization": 0.95,
          "resets_at": "2026-09-28T18:10:00Z"
        }
      ],
      "at_reserve": [
        "5h"
      ]
    },
    {
      "id": "side",
      "label": "Side",
      "token_set": true
    }
  ]
}`,
		},
		{
			name: "the router's, priming",
			doc: status.Document{
				GeneratedAt: generated,
				Source:      status.SourceRouter,
				Router:      status.Health{Healthy: true},
				Prime: status.Prime{Day: "08:00-23:00", Window: "5h", Slots: []status.Slot{
					{Account: "work", At: "04:10", Next: time.Date(2026, 9, 28, 17, 10, 0, 0, time.UTC)},
					{Account: "side", At: "06:40", Next: time.Date(2026, 9, 29, 5, 45, 0, 0, time.UTC)},
				}},
				Accounts: []status.Account{{ID: "work", Label: "Work", TokenSet: true}, {ID: "side", Label: "Side", TokenSet: true}},
			},
			want: `{
  "generated_at": "2026-09-28T13:12:00Z",
  "source": "router",
  "prime": {
    "day": "08:00-23:00",
    "window": "5h",
    "slots": [
      {
        "account": "work",
        "at": "04:10",
        "next": "2026-09-28T17:10:00Z"
      },
      {
        "account": "side",
        "at": "06:40",
        "next": "2026-09-29T05:45:00Z"
      }
    ]
  },
  "router": {
    "healthy": true,
    "requests": 0,
    "failures": 0
  },
  "accounts": [
    {
      "id": "work",
      "label": "Work",
      "token_set": true
    },
    {
      "id": "side",
      "label": "Side",
      "token_set": true
    }
  ]
}`,
		},
		{
			name: "probed, priming, which says nothing of when each account is next primed",
			doc: status.Document{
				GeneratedAt: generated,
				Source:      status.SourceProbe,
				Prime:       status.Prime{Day: "22:00-06:00", Window: "5h", Slots: []status.Slot{{Account: "work", At: "19:30"}}},
				Accounts:    []status.Account{{ID: "work", Label: "Work", TokenSet: true}},
			},
			want: `{
  "generated_at": "2026-09-28T13:12:00Z",
  "source": "probe",
  "prime": {
    "day": "22:00-06:00",
    "window": "5h",
    "slots": [
      {
        "account": "work",
        "at": "19:30"
      }
    ]
  },
  "accounts": [
    {
      "id": "work",
      "label": "Work",
      "token_set": true
    }
  ]
}`,
		},
		{
			name: "the router's, with a session that has lapsed",
			doc: status.Document{
				GeneratedAt: generated,
				Source:      status.SourceRouter,
				Router:      status.Health{Healthy: true},
				Accounts: []status.Account{
					{
						ID: "work", Label: "Work", TokenSet: true, FetchedAt: generated.Add(-6 * time.Hour),
						Windows: []quota.Window{
							{Key: "5h", Label: "Session"},
							{Key: "7d", Label: "Week", Utilization: 0.93, ResetsAt: time.Date(2026, 10, 2, 21, 0, 0, 0, time.UTC), Status: quota.StatusAllowedWarning},
						},
						Lapsed: []string{"5h"},
					},
				},
			},
			want: `{
  "generated_at": "2026-09-28T13:12:00Z",
  "source": "router",
  "router": {
    "healthy": true,
    "requests": 0,
    "failures": 0
  },
  "accounts": [
    {
      "id": "work",
      "label": "Work",
      "token_set": true,
      "fetched_at": "2026-09-28T07:12:00Z",
      "windows": [
        {
          "key": "5h",
          "label": "Session",
          "utilization": 0
        },
        {
          "key": "7d",
          "label": "Week",
          "utilization": 0.93,
          "resets_at": "2026-10-02T21:00:00Z",
          "status": "allowed_warning"
        }
      ],
      "lapsed": [
        "5h"
      ]
    }
  ]
}`,
		},
		{
			name: "the router's, healthy",
			doc: status.Document{
				GeneratedAt: generated,
				Source:      status.SourceRouter,
				Router:      status.Health{Healthy: true, Requests: 12, Failures: 1},
				Accounts:    []status.Account{{ID: "work", Label: "Work", TokenSet: true}},
			},
			want: `{
  "generated_at": "2026-09-28T13:12:00Z",
  "source": "router",
  "router": {
    "healthy": true,
    "requests": 12,
    "failures": 1
  },
  "accounts": [
    {
      "id": "work",
      "label": "Work",
      "token_set": true
    }
  ]
}`,
		},
		{
			name: "the router's, with what has happened lately",
			doc: status.Document{
				GeneratedAt: generated,
				Source:      status.SourceRouter,
				Router:      status.Health{Healthy: true},
				Events: []status.Event{
					{ID: 7, At: generated, Kind: status.EventPin, Account: "work", Accounts: []string{"work", "side"}, Move: true, Force: true, By: "cli"},
					{ID: 6, At: generated, Kind: status.EventAuto, Account: "side", Session: "0b5c6f2e-7d41-4a3b-9c8e-1f2a3b4c5d6e", By: "dashboard"},
					{ID: 5, At: generated, Kind: status.EventMoved, Session: "0b5c6f2e-7d41-4a3b-9c8e-1f2a3b4c5d6e", Model: "claude-opus-5-5", From: "work", To: "side", Reason: "moved: work hit its limit", Limit: 3},
					{ID: 4, At: generated, Kind: status.EventPressure, Account: "side", Windows: []string{"5h"}, Until: generated.Add(2 * time.Hour), Since: generated.Add(-30 * time.Minute)},
					{ID: 3, At: generated.Add(-time.Minute), Kind: status.EventLimit, Account: "work", To: "side", Windows: []string{"5h"}, Until: generated.Add(time.Hour), Count: 2},
					{ID: 2, At: generated.Add(-2 * time.Minute), Kind: status.EventRefused, Account: "work", Until: generated.Add(8 * time.Minute), Status: 403, Family: "opus"},
					{ID: 1, At: generated.Add(-3 * time.Minute), Kind: status.EventStarted, Account: "work", Session: "0b5c6f2e-7d41-4a3b-9c8e-1f2a3b4c5d6e", Model: "claude-opus-5-5", Reason: "new"},
				},
				Accounts: []status.Account{{ID: "work", Label: "Work", TokenSet: true}},
			},
			want: `{
  "generated_at": "2026-09-28T13:12:00Z",
  "source": "router",
  "router": {
    "healthy": true,
    "requests": 0,
    "failures": 0
  },
  "events": [
    {
      "id": 7,
      "at": "2026-09-28T13:12:00Z",
      "kind": "pin",
      "account": "work",
      "accounts": [
        "work",
        "side"
      ],
      "move": true,
      "force": true,
      "by": "cli"
    },
    {
      "id": 6,
      "at": "2026-09-28T13:12:00Z",
      "kind": "auto",
      "account": "side",
      "session": "0b5c6f2e-7d41-4a3b-9c8e-1f2a3b4c5d6e",
      "by": "dashboard"
    },
    {
      "id": 5,
      "at": "2026-09-28T13:12:00Z",
      "kind": "moved",
      "session": "0b5c6f2e-7d41-4a3b-9c8e-1f2a3b4c5d6e",
      "model": "claude-opus-5-5",
      "from": "work",
      "to": "side",
      "reason": "moved: work hit its limit",
      "limit": 3
    },
    {
      "id": 4,
      "at": "2026-09-28T13:12:00Z",
      "kind": "pressure",
      "account": "side",
      "windows": [
        "5h"
      ],
      "until": "2026-09-28T15:12:00Z",
      "since": "2026-09-28T12:42:00Z"
    },
    {
      "id": 3,
      "at": "2026-09-28T13:11:00Z",
      "kind": "limit",
      "account": "work",
      "to": "side",
      "windows": [
        "5h"
      ],
      "until": "2026-09-28T14:12:00Z",
      "count": 2
    },
    {
      "id": 2,
      "at": "2026-09-28T13:10:00Z",
      "kind": "refused",
      "account": "work",
      "until": "2026-09-28T13:20:00Z",
      "status": 403,
      "family": "opus"
    },
    {
      "id": 1,
      "at": "2026-09-28T13:09:00Z",
      "kind": "started",
      "account": "work",
      "session": "0b5c6f2e-7d41-4a3b-9c8e-1f2a3b4c5d6e",
      "model": "claude-opus-5-5",
      "reason": "new"
    }
  ],
  "accounts": [
    {
      "id": "work",
      "label": "Work",
      "token_set": true
    }
  ]
}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.MarshalIndent(tt.doc, "", "  ")
			if err != nil {
				t.Fatalf("MarshalIndent() error = %v", err)
			}
			if string(got) != tt.want {
				t.Errorf("MarshalIndent() =\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestPinJSON(t *testing.T) {
	since := time.Date(2026, 9, 28, 12, 12, 0, 0, time.UTC)
	tests := []struct {
		name string
		json string
		want status.Pin
	}{
		{
			name: "one account",
			json: `{"accounts":["side"],"account":"side","since":"2026-09-28T12:12:00Z","move":false}`,
			want: status.Pin{Accounts: []string{"side"}, Since: since},
		},
		{
			name: "several, the first its account too",
			json: `{"accounts":["work","side"],"account":"work","since":"2026-09-28T12:12:00Z","move":true}`,
			want: status.Pin{Accounts: []string{"work", "side"}, Since: since, Move: true},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := json.Marshal(tt.want)
			if err != nil || string(got) != tt.json {
				t.Errorf("Marshal() = %s, %v, want %s", got, err, tt.json)
			}
			var read status.Pin
			if err := json.Unmarshal(got, &read); err != nil || !reflect.DeepEqual(read, tt.want) {
				t.Errorf("Unmarshal() = %+v, %v, want %+v", read, err, tt.want)
			}
		})
	}
}

func TestAPinThatNamesItsAccountAloneReadsAsAPinToThatOne(t *testing.T) {
	var read status.Pin
	// As a switchboard from before pins named several wrote it.
	err := json.Unmarshal([]byte(`{"account":"side","since":"2026-09-28T12:12:00Z","move":true}`), &read)
	want := status.Pin{Accounts: []string{"side"}, Since: time.Date(2026, 9, 28, 12, 12, 0, 0, time.UTC), Move: true}
	if err != nil || !reflect.DeepEqual(read, want) {
		t.Errorf("Unmarshal() = %+v, %v, want %+v", read, err, want)
	}
}

func TestADocumentWithoutEventsSaysNothingOfThem(t *testing.T) {
	for _, events := range [][]status.Event{nil, {}} {
		got, err := json.Marshal(status.Document{Source: status.SourceRouter, Events: events})
		if err != nil || strings.Contains(string(got), `"events"`) {
			t.Errorf("Marshal() with events %#v = %s, %v, want no events", events, got, err)
		}
	}
}

func TestADocumentWithoutAPinSaysNothingOfOne(t *testing.T) {
	got, err := json.Marshal(status.Document{Source: status.SourceRouter})
	if err != nil || strings.Contains(string(got), `"pin"`) {
		t.Errorf("Marshal() = %s, %v, want no pin", got, err)
	}
	var doc status.Document
	if err := json.Unmarshal(got, &doc); err != nil || !doc.Pin.IsZero() {
		t.Errorf("Unmarshal() = pin %+v, %v, want none", doc.Pin, err)
	}
}

type probeResult struct {
	usage quota.Usage
	err   error
}

// withWindows is a probe that reads windows.
func withWindows(windows ...quota.Window) probeResult {
	return probeResult{usage: quota.Usage{Windows: windows}}
}

// fakeProber answers each token with its result, and records the tokens it's given.
type fakeProber struct {
	results map[string]probeResult

	mu     sync.Mutex
	tokens []string
}

func (p *fakeProber) Probe(_ context.Context, token string) (quota.Probe, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tokens = append(p.tokens, token)
	result := p.results[token]
	return quota.Probe{Usage: result.usage}, result.err
}

// probed returns the tokens the prober was given, sorted.
func (p *fakeProber) probed() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Sorted(slices.Values(p.tokens))
}

// gatheringProber holds each probe until waitFor of them have started, which
// they only can when they run at the same time.
type gatheringProber struct {
	waitFor  int
	gathered chan struct{}

	mu      sync.Mutex
	started int
}

func (p *gatheringProber) Probe(ctx context.Context, _ string) (quota.Probe, error) {
	p.mu.Lock()
	if p.started++; p.started == p.waitFor {
		close(p.gathered)
	}
	p.mu.Unlock()
	select {
	case <-p.gathered:
		return quota.Probe{}, nil
	case <-ctx.Done():
		return quota.Probe{}, errors.New("probed alone: the probes ran one at a time")
	}
}

// accounts are three accounts, which the tests give tokens as they need.
var accounts = []config.Account{
	{ID: "work", Label: "Work"},
	{ID: "personal", Label: "Personal"},
	{ID: "side", Label: "Side"},
}
