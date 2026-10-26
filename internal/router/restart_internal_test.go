package router

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/status"
)

// Configs the tests' config file holds: two valid, and one that isn't.
const (
	oneAccount  = "[[account]]\nid = \"work\"\n"
	twoAccounts = "[[account]]\nid = \"work\"\n\n[[account]]\nid = \"side\"\n"
	notAConfig  = "[[account]]\nid = \"work\"\ncolour = \"red\"\n"
)

func TestTheConfigFileIsLookedAtForAChange(t *testing.T) {
	tests := []struct {
		name string
		// change changes what the router was started from.
		change func(t *testing.T, s startedFrom)
		// wantDue is why a restart is due, if one is, and wantLog the line
		// the change is logged with, if any.
		wantDue string
		wantLog []string
	}{
		{
			name:   "left alone",
			change: func(*testing.T, startedFrom) {},
		},
		{
			name:    "edited through its link, where it's kept, into another valid config",
			change:  func(t *testing.T, s startedFrom) { writeFile(t, s.kept, twoAccounts) },
			wantDue: "config changed",
			wantLog: []string{"level=INFO", `msg="config changed"`},
		},
		{
			name: "replaced whole where it's kept, as an editor saves it",
			change: func(t *testing.T, s startedFrom) {
				saved := filepath.Join(filepath.Dir(s.kept), ".config.toml.new")
				writeFile(t, saved, twoAccounts)
				rename(t, saved, s.kept)
			},
			wantDue: "config changed",
			wantLog: []string{"level=INFO", `msg="config changed"`},
		},
		{
			name: "its link leading to another valid config",
			change: func(t *testing.T, s startedFrom) {
				other := filepath.Join(filepath.Dir(s.kept), "other.toml")
				writeFile(t, other, twoAccounts)
				relink(t, s.config, other)
			},
			wantDue: "config changed",
			wantLog: []string{"level=INFO", `msg="config changed"`},
		},
		{
			name:    "made invalid",
			change:  func(t *testing.T, s startedFrom) { writeFile(t, s.kept, notAConfig) },
			wantLog: []string{"level=WARN", `msg="config change refused; carrying on with the config as it was"`, `error="invalid config `},
		},
		{
			name:    "removed",
			change:  func(t *testing.T, s startedFrom) { remove(t, s.kept) },
			wantLog: []string{"level=WARN", `msg="config change refused; carrying on with the config as it was"`, "no such file or directory"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			s := newStartedFrom(t)
			r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, newInFlight(), at(start), unheeded)

			tt.change(t, s)
			// Two looks, as a config file that's invalid at one may only be
			// caught while it's saved.
			r.look()
			r.look()
			if got := r.due(); got != tt.wantDue {
				t.Errorf("due() = %q, want %q", got, tt.wantDue)
			}
			if tt.wantLog != nil && !log.Has(append(tt.wantLog, "path="+s.config)...) {
				t.Errorf("log reads\n%s\nwant a line with %q", log, tt.wantLog)
			}
			if tt.wantLog == nil && log.Has("component=router") {
				t.Errorf("log reads\n%s\nwant nothing of the router's", log)
			}
		})
	}
}

func TestTheBinaryIsUpgradedOnceItLeadsToAnotherFile(t *testing.T) {
	tests := []struct {
		name string
		// change changes where the binary's path leads.
		change      func(t *testing.T, s startedFrom)
		wantUpgrade bool
	}{
		{
			name:   "left alone",
			change: func(*testing.T, startedFrom) {},
		},
		{
			name:        "moved on to another version",
			change:      func(t *testing.T, s startedFrom) { relink(t, s.binary, s.version("1.1")) },
			wantUpgrade: true,
		},
		{
			name:        "rewritten where it is",
			change:      func(t *testing.T, s startedFrom) { writeFile(t, s.version("1.0"), "switchboard 1.0, rebuilt") },
			wantUpgrade: true,
		},
		{
			name:   "leading nowhere for a moment, as an upgrade moves it on",
			change: func(t *testing.T, s startedFrom) { remove(t, s.binary) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			s := newStartedFrom(t)
			r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, newInFlight(), at(start), unheeded)

			tt.change(t, s)
			r.look()
			want := ""
			if tt.wantUpgrade {
				want = "upgraded"
			}
			if got := r.due(); got != want {
				t.Errorf("due() = %q, want %q", got, want)
			}
			upgraded := log.Has("level=INFO", `msg="the binary leads to another file, as after an upgrade"`, "path="+s.binary)
			if upgraded != tt.wantUpgrade {
				t.Errorf("log reads\n%s\nwant the upgrade logged: %v", log, tt.wantUpgrade)
			}
		})
	}
}

func TestTheTimeZoneChangesOnceItsFileLeadsToAnother(t *testing.T) {
	tests := []struct {
		name string
		// change changes where the time zone's file leads.
		change   func(t *testing.T, s startedFrom)
		wantDue  string
		wantNews bool
	}{
		{
			name:   "left alone",
			change: func(*testing.T, startedFrom) {},
		},
		{
			name:     "leading to another time zone's, as the Mac is taken there",
			change:   func(t *testing.T, s startedFrom) { relink(t, s.zone, s.zoneInfo("Etc/GMT-9")) },
			wantDue:  "time zone changed",
			wantNews: true,
		},
		{
			name:   "leading nowhere for a moment, as it's set",
			change: func(t *testing.T, s startedFrom) { remove(t, s.zone) },
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			s := newStartedFrom(t)
			r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, newInFlight(), at(start), unheeded)

			tt.change(t, s)
			r.look()
			if got := r.due(); got != tt.wantDue {
				t.Errorf("due() = %q, want %q", got, tt.wantDue)
			}
			news := log.Has("level=INFO", `msg="the time zone's file leads to another, as in another time zone"`, "path="+s.zone)
			if news != tt.wantNews {
				t.Errorf("log reads\n%s\nwant the change logged: %v", log, tt.wantNews)
			}
		})
	}
}

func TestAChangeMadeOnceWhatTheRouterStartsFromWasFoundIsSeen(t *testing.T) {
	tests := []struct {
		name string
		// change changes what the router is started from, once it's found,
		// as the router starts.
		change  func(t *testing.T, s startedFrom)
		wantDue string
	}{
		{
			name:    "its config file making another valid config",
			change:  func(t *testing.T, s startedFrom) { writeFile(t, s.kept, twoAccounts) },
			wantDue: "config changed",
		},
		{
			name:    "its binary leading to another file",
			change:  func(t *testing.T, s startedFrom) { relink(t, s.binary, s.version("1.1")) },
			wantDue: "upgraded",
		},
		{
			name:    "the time zone's file leading to another",
			change:  func(t *testing.T, s startedFrom) { relink(t, s.zone, s.zoneInfo("Etc/GMT-9")) },
			wantDue: "time zone changed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStartedFrom(t)
			config, binary, zone := Watch(s.config), Watch(s.binary), Watch(s.zone)

			tt.change(t, s)
			r := newRestarts(config, binary, zone, true, newInFlight(), at(start), unheeded)
			r.look()
			if got := r.due(); got != tt.wantDue {
				t.Errorf("due() = %q, want %q", got, tt.wantDue)
			}
		})
	}
}

func TestABinaryThatLedNowhereAsTheRouterStartedIsNeverUpgraded(t *testing.T) {
	s := newStartedFrom(t)
	missing := filepath.Join(t.TempDir(), "switchboard")
	r := newRestarts(Watch(s.config), Watch(missing), Watch(s.zone), true, newInFlight(), at(start), unheeded)

	link(t, s.version("1.1"), missing)
	r.look()
	if got := r.due(); got != "" {
		t.Errorf("due() = %q, want none: nothing's known of the binary running", got)
	}
}

func TestARestartIsHeldBackWhileTheConfigFileIsInvalid(t *testing.T) {
	s := newStartedFrom(t)
	r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, newInFlight(), at(start), unheeded)

	writeFile(t, s.kept, notAConfig)
	relink(t, s.binary, s.version("1.1"))
	r.look()
	if ready := r.ready(); ready != nil {
		t.Errorf("upgraded, with the config file invalid at a look, ready() = %v, want none until the next look", ready)
	}
	r.look()
	if got, ready := r.due(), r.ready(); got != "" || ready != nil {
		t.Errorf("upgraded, with the config file invalid at two looks, due() = %q, and ready() = %v, want none: a router started again couldn't start", got, ready)
	}

	writeFile(t, s.kept, twoAccounts)
	r.look()
	if got := r.due(); got != "config changed" {
		t.Errorf("with the config file put right, due() = %q, want config changed", got)
	}
}

func TestARestartDueIsSaidOnceByHand(t *testing.T) {
	log := logstest.Capture(t)
	s := newStartedFrom(t)
	r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), false, newInFlight(), at(start), unheeded)

	writeFile(t, s.kept, twoAccounts)
	r.look()
	relink(t, s.binary, s.version("1.1"))
	writeFile(t, s.kept, oneAccount)
	r.look()
	if ready := r.ready(); ready != nil {
		t.Errorf("ready() = %v, want nil: a router run by hand doesn't restart", ready)
	}
	due := []string{"level=INFO", `msg="restart due; run switchboard serve again to take it up"`, `reason="config changed"`}
	if !log.Has(due...) {
		t.Errorf("log reads\n%s\nwant a line with %q", log, due)
	}
	if n := strings.Count(log.String(), `msg="restart due`); n != 1 {
		t.Errorf("log reads\n%s\nwant the restart due said once, not %d times", log, n)
	}
}

func TestASupervisedRouterRestartsOnceNoRequestIsInFlight(t *testing.T) {
	log := logstest.Capture(t)
	s := newStartedFrom(t)
	inFlight := newInFlight()
	r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, inFlight, at(start), unheeded)
	if r.ready() != nil {
		t.Fatal("ready() isn't nil, with no restart due")
	}

	inFlight.begin()
	relink(t, s.binary, s.version("1.1"))
	r.look()
	if !log.Has("level=INFO", `msg="restart due; restarting once no request is in flight"`, "reason=upgraded") {
		t.Errorf("log reads\n%s\nwant the restart due", log)
	}
	ready := r.ready()
	select {
	case <-ready:
		t.Fatal("ready() is closed with a request in flight")
	default:
	}
	inFlight.end()
	select {
	case <-ready:
	default:
		t.Fatal("ready() isn't closed once no request is in flight")
	}

	if !r.restart(t.Context()) {
		t.Fatal("restart() = false, want the router restarted")
	}
	select {
	case <-r.restarted:
	default:
		t.Error("restarted isn't closed once the router restarts")
	}
	if !log.Has("level=INFO", "msg=restarting", "reason=upgraded") {
		t.Errorf("log reads\n%s\nwant the restart", log)
	}
}

func TestARestartAskedForIsTheOneTheRouterMakes(t *testing.T) {
	log := logstest.Capture(t)
	s := newStartedFrom(t)
	r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, newInFlight(), at(start), unheeded)
	relink(t, s.binary, s.version("1.1"))
	r.look()

	r.atOnce()
	if !r.restart(t.Context()) {
		t.Error("restart() = false, want true: a restart is due")
	}
	select {
	case <-r.restarted:
	default:
		t.Fatal("restarted isn't closed once the router is asked to restart")
	}
	if r.reason != "asked to" {
		t.Errorf("the router restarts because %q, want asked to", r.reason)
	}
	if n := strings.Count(log.String(), "msg=restarting"); n != 1 {
		t.Errorf("log reads\n%s\nwant the restart said once, not %d times", log, n)
	}
}

func TestARestartDueIsReported(t *testing.T) {
	tests := []struct {
		name       string
		supervised bool
		want       status.Restart
	}{
		{name: "by the service's router", supervised: true, want: status.Restart{Reason: "upgraded", Since: start, InFlight: 2}},
		{name: "by a router run by hand", want: status.Restart{Reason: "upgraded", Since: start, InFlight: 2, ByHand: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStartedFrom(t)
			inFlight := newInFlight()
			clock := &testClock{now: start}
			r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), tt.supervised, inFlight, clock.read, unheeded)
			r.look()
			if got := r.report(); got != (status.Restart{}) {
				t.Fatalf("with nothing changed, report() = %+v, want none due", got)
			}

			inFlight.begin()
			inFlight.begin()
			relink(t, s.binary, s.version("1.1"))
			r.look()
			clock.now = start.Add(time.Hour)
			r.look()
			if got := r.report(); got != tt.want {
				t.Errorf("upgraded an hour ago, with two requests in flight, report() = %+v, want %+v", got, tt.want)
			}

			writeFile(t, s.kept, notAConfig)
			r.look()
			r.look()
			if got := r.report(); got != (status.Restart{}) {
				t.Errorf("with the config file invalid at two looks, report() = %+v, want none due: a router started again couldn't start", got)
			}
		})
	}
}

func TestARestartIsToldOfAsItFallsDue(t *testing.T) {
	s := newStartedFrom(t)
	var heard []Event
	r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, newInFlight(), at(start), func(e Event) { heard = append(heard, e) })

	r.look()
	relink(t, s.binary, s.version("1.1"))
	r.look()
	r.look()
	// Its reason changes while it stays due, then it's due no longer while
	// the config file is invalid, at two looks, then falls due again once
	// it's put right.
	writeFile(t, s.kept, twoAccounts)
	r.look()
	writeFile(t, s.kept, notAConfig)
	r.look()
	r.look()
	writeFile(t, s.kept, oneAccount)
	r.look()
	want := []Event{RestartDue{Reason: "upgraded"}, RestartDue{Reason: "config changed"}}
	if !reflect.DeepEqual(heard, want) {
		t.Errorf("heard %+v, want %+v: each time it fell due", heard, want)
	}
}

func TestAConfigFileCaughtWhileItsSavedLeavesARestartDue(t *testing.T) {
	tests := []struct {
		name string
		// looks is how many looks find the config file invalid before it's
		// put right.
		looks int
		// wantDue is whether a restart stays due while it's invalid, and
		// wantHeard the restarts told of as they fall due.
		wantDue   bool
		wantHeard []Event
		wantWarn  bool
	}{
		{
			name:      "caught at a look, as an editor saving it leaves it",
			looks:     1,
			wantDue:   true,
			wantHeard: []Event{RestartDue{Reason: "upgraded"}},
		},
		{
			name:      "invalid at two looks in a row",
			looks:     2,
			wantHeard: []Event{RestartDue{Reason: "upgraded"}, RestartDue{Reason: "config changed"}},
			wantWarn:  true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			s := newStartedFrom(t)
			var heard []Event
			r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, newInFlight(), at(start), func(e Event) { heard = append(heard, e) })
			relink(t, s.binary, s.version("1.1"))
			r.look()

			writeFile(t, s.kept, notAConfig)
			for range tt.looks {
				r.look()
			}
			if due := r.due() != ""; due != tt.wantDue {
				t.Errorf("with the config file invalid at %d looks, due() = %q, want due: %v", tt.looks, r.due(), tt.wantDue)
			}
			writeFile(t, s.kept, twoAccounts)
			r.look()
			if got := r.due(); got != "config changed" {
				t.Errorf("once it's saved, due() = %q, want config changed", got)
			}
			if !reflect.DeepEqual(heard, tt.wantHeard) {
				t.Errorf("heard %+v, want %+v", heard, tt.wantHeard)
			}
			if warned := log.Has("level=WARN", `msg="config change refused`); warned != tt.wantWarn {
				t.Errorf("log reads\n%s\nwant the config refused at warn: %v", log, tt.wantWarn)
			}
		})
	}
}

func TestARestartLooksAgainBeforeItGoes(t *testing.T) {
	s := newStartedFrom(t)
	r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, newInFlight(), at(start), unheeded)
	relink(t, s.binary, s.version("1.1"))
	r.look()

	writeFile(t, s.kept, notAConfig)
	if r.restart(t.Context()) {
		t.Error("restart() = true, want the router kept running: its config file is invalid since it last looked")
	}
	select {
	case <-r.restarted:
		t.Error("restarted is closed, want the router kept running")
	default:
	}
}

func TestARestartGoesAheadUnlessItsBinaryRefusesToStart(t *testing.T) {
	const refusal = "invalid config /test/config.toml:\naccount work: its label is longer than 100 characters"
	tests := []struct {
		name string
		// answer is how the binary answers whether it could start.
		answer      binaryAnswer
		wantRestart bool
		wantLog     []string
	}{
		{name: "able to start", answer: binaryAnswer{}, wantRestart: true},
		{
			name:        "unable to answer, as one from before the check",
			answer:      binaryAnswer{err: errors.New("exit status 1")},
			wantRestart: true,
			wantLog:     []string{"level=INFO", `msg="its binary couldn't answer whether it could start; restarting all the same"`, `error="exit status 1"`},
		},
		{
			name:    "refusing",
			answer:  binaryAnswer{refusal: refusal},
			wantLog: []string{"level=WARN", `msg="restart held, as its binary refuses to start; carrying on as it is"`, "reason=upgraded", `refusal="invalid config /test/config.toml:\naccount work: its label is longer than 100 characters"`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			s := newStartedFrom(t)
			var heard []Event
			r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, newInFlight(), at(start), func(e Event) { heard = append(heard, e) })
			asked := askedOf(r, tt.answer)
			relink(t, s.binary, s.version("1.1"))
			r.look()

			if got := r.restart(t.Context()); got != tt.wantRestart {
				t.Errorf("restart() = %v, want %v", got, tt.wantRestart)
			}
			if *asked != 1 {
				t.Errorf("the binary was asked %d times, want once", *asked)
			}
			if restarted := r.restarting(); restarted != tt.wantRestart {
				t.Errorf("restarted is closed: %v, want %v", restarted, tt.wantRestart)
			}
			if tt.wantLog != nil && !log.Has(append(tt.wantLog, "path="+s.binary)...) {
				t.Errorf("log reads\n%s\nwant a line with %q", log, tt.wantLog)
			}
			wantHeard := []Event{RestartDue{Reason: "upgraded"}}
			wantReport := status.Restart{Reason: "upgraded", Since: start}
			if !tt.wantRestart {
				wantHeard = append(wantHeard, RestartHeld{Reason: "upgraded", Refusal: refusal})
				wantReport.Held = refusal
			}
			if !reflect.DeepEqual(heard, wantHeard) {
				t.Errorf("heard %+v, want %+v", heard, wantHeard)
			}
			if got := r.report(); got != wantReport {
				t.Errorf("report() = %+v, want %+v", got, wantReport)
			}
		})
	}
}

func TestARestartHeldIsSaidOnceAndAskedAboutNoMore(t *testing.T) {
	log := logstest.Capture(t)
	s := newStartedFrom(t)
	var heard []Event
	r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, newInFlight(), at(start), func(e Event) { heard = append(heard, e) })
	asked := askedOf(r, binaryAnswer{refusal: "invalid config"})
	relink(t, s.binary, s.version("1.1"))
	r.look()
	r.restart(t.Context())

	for range 5 {
		r.look()
		if ready := r.ready(); ready != nil {
			t.Fatalf("ready() = %v while the binary refuses to start, with nothing changed, want none", ready)
		}
	}
	if *asked != 1 {
		t.Errorf("the binary was asked %d times, want once: nothing changed since it refused", *asked)
	}
	if n := strings.Count(log.String(), `msg="restart held`); n != 1 {
		t.Errorf("log reads\n%s\nwant the restart held said once, not %d times", log, n)
	}
	if want := []Event{RestartDue{Reason: "upgraded"}, RestartHeld{Reason: "upgraded", Refusal: "invalid config"}}; !reflect.DeepEqual(heard, want) {
		t.Errorf("heard %+v, want %+v: the hold told of once", heard, want)
	}
	if got := r.report(); got.Held != "invalid config" {
		t.Errorf("report() = %+v, want the restart held, saying why", got)
	}
}

func TestARestartHeldIsAskedAboutAgainOnceTheConfigFileOrTheBinaryChanges(t *testing.T) {
	tests := []struct {
		name string
		// change changes what the router was started from, once the binary
		// has refused to start.
		change  func(t *testing.T, s startedFrom)
		wantDue string
	}{
		{
			name:    "the config file put right",
			change:  func(t *testing.T, s startedFrom) { writeFile(t, s.kept, twoAccounts) },
			wantDue: "config changed",
		},
		{
			name: "the binary moved on to another version",
			change: func(t *testing.T, s startedFrom) {
				writeFile(t, s.version("1.2"), "switchboard 1.2")
				relink(t, s.binary, s.version("1.2"))
			},
			wantDue: "upgraded",
		},
		{
			name:    "the binary rewritten where it is",
			change:  func(t *testing.T, s startedFrom) { writeFile(t, s.version("1.1"), "switchboard 1.1, rebuilt") },
			wantDue: "upgraded",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			s := newStartedFrom(t)
			r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, newInFlight(), at(start), unheeded)
			asked := askedOf(r, binaryAnswer{refusal: "invalid config"}, binaryAnswer{})
			relink(t, s.binary, s.version("1.1"))
			r.look()
			r.restart(t.Context())

			tt.change(t, s)
			r.look()
			if got := r.report(); got.Held != "" || got.Reason != tt.wantDue {
				t.Errorf("once changed, report() = %+v, want a restart due for %s, held no longer", got, tt.wantDue)
			}
			if r.ready() == nil {
				t.Fatal("once changed, ready() = nil, want the restart to go ahead once no request is in flight")
			}
			if !r.restart(t.Context()) {
				t.Error("restart() = false, want the router restarted: the binary asked again starts")
			}
			if *asked != 2 {
				t.Errorf("the binary was asked %d times, want twice: again once changed", *asked)
			}
			if !log.Has("level=INFO", `msg="the config file or the binary has changed since the restart was held; asking the binary again"`) {
				t.Errorf("log reads\n%s\nwant the hold lifted", log)
			}
		})
	}
}

func TestARestartHeldStaysHeldThroughAFileGoneForAMoment(t *testing.T) {
	const refusal = "invalid config"
	saving := func(s startedFrom) string { return s.kept + ".saving" }
	heldOnce := []Event{RestartDue{Reason: "upgraded"}, RestartHeld{Reason: "upgraded", Refusal: refusal}}
	tests := []struct {
		name string
		// gone takes a file the router was started from away, for looks
		// looks, and back puts it back as it was.
		gone, back func(t *testing.T, s startedFrom)
		looks      int
		// wantHeard are the events told of, from the restart first falling
		// due on.
		wantHeard []Event
	}{
		{
			name:      "the binary's link, as an upgrade moves it on",
			gone:      func(t *testing.T, s startedFrom) { remove(t, s.binary) },
			back:      func(t *testing.T, s startedFrom) { link(t, s.version("1.1"), s.binary) },
			looks:     1,
			wantHeard: heldOnce,
		},
		{
			name:      "the binary's link, for a while",
			gone:      func(t *testing.T, s startedFrom) { remove(t, s.binary) },
			back:      func(t *testing.T, s startedFrom) { link(t, s.version("1.1"), s.binary) },
			looks:     3,
			wantHeard: heldOnce,
		},
		{
			name:      "the config file, as an editor saves it",
			gone:      func(t *testing.T, s startedFrom) { rename(t, s.kept, saving(s)) },
			back:      func(t *testing.T, s startedFrom) { rename(t, saving(s), s.kept) },
			looks:     1,
			wantHeard: heldOnce,
		},
		{
			name:  "the config file, long enough for its change to be refused, the restart falling due again held",
			gone:  func(t *testing.T, s startedFrom) { rename(t, s.kept, saving(s)) },
			back:  func(t *testing.T, s startedFrom) { rename(t, saving(s), s.kept) },
			looks: 2,
			wantHeard: append(slices.Clone(heldOnce),
				RestartHeld{Reason: "config changed", Refusal: refusal}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			s := newStartedFrom(t)
			var heard []Event
			r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, newInFlight(), at(start), func(e Event) { heard = append(heard, e) })
			asked := askedOf(r, binaryAnswer{refusal: refusal}, binaryAnswer{err: fs.ErrNotExist}, binaryAnswer{refusal: refusal})
			relink(t, s.binary, s.version("1.1"))
			r.look()
			r.restart(t.Context())

			tt.gone(t, s)
			for i := range tt.looks {
				r.look()
				if ready := r.ready(); r.held == nil || ready != nil {
					t.Fatalf("at look %d with the file gone, held: %v, and ready() = %v, want the restart held still", i+1, r.held, ready)
				}
			}
			tt.back(t, s)
			r.look()
			if got, ready := r.report(), r.ready(); got.Held != refusal || ready != nil {
				t.Errorf("with the file back, report() = %+v, and ready() = %v, want the restart held still, saying why", got, ready)
			}
			if *asked != 1 {
				t.Errorf("the binary was asked %d times, want once: nothing moved on since it refused", *asked)
			}
			if n := strings.Count(log.String(), `msg="restart held`); n != 1 {
				t.Errorf("log reads\n%s\nwant the restart held said once, not %d times", log, n)
			}
			if !reflect.DeepEqual(heard, tt.wantHeard) {
				t.Errorf("heard %+v, want %+v: the events saying what the status document does", heard, tt.wantHeard)
			}
		})
	}
}

func TestARequestBegunAsTheBinaryIsAskedPutsTheRestartOffToTheNextQuietMoment(t *testing.T) {
	s := newStartedFrom(t)
	inFlight := newInFlight()
	r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, inFlight, at(start), unheeded)
	asked := askedOf(r, binaryAnswer{})
	ask := r.ask
	r.ask = func(ctx context.Context) (string, error) {
		if *asked == 0 {
			inFlight.begin()
		}
		return ask(ctx)
	}
	relink(t, s.binary, s.version("1.1"))
	r.look()

	if r.restart(t.Context()) || r.restarting() {
		t.Fatal("restart() = true, want the restart put off: a request began as the binary was asked")
	}
	if quiet(r.ready()) {
		t.Fatal("ready() is closed with a request in flight, want it once there's none")
	}
	inFlight.end()
	if !quiet(r.ready()) {
		t.Fatal("ready() isn't closed with no request in flight, want the restart to go ahead")
	}
	if !r.restart(t.Context()) {
		t.Error("restart() = false at the next quiet moment, want the router restarted: the binary's answer still clears it")
	}
	if *asked != 1 {
		t.Errorf("the binary was asked %d times, want once: nothing moved on since it answered", *asked)
	}
}

func TestARestartPutOffIsAskedAboutAgainOnceTheConfigFileOrTheBinaryChanges(t *testing.T) {
	tests := []struct {
		name string
		// change changes what the router was started from, once the binary
		// has answered that it could start, and a request has begun.
		change func(t *testing.T, s startedFrom)
		// again is how the binary answers, asked again.
		again       binaryAnswer
		wantRestart bool
	}{
		{
			name:        "the config file edited",
			change:      func(t *testing.T, s startedFrom) { writeFile(t, s.kept, twoAccounts) },
			wantRestart: true,
		},
		{
			name: "the binary moved on to another version",
			change: func(t *testing.T, s startedFrom) {
				writeFile(t, s.version("1.2"), "switchboard 1.2")
				relink(t, s.binary, s.version("1.2"))
			},
			wantRestart: true,
		},
		{
			name: "the binary moved on to one that refuses to start",
			change: func(t *testing.T, s startedFrom) {
				writeFile(t, s.version("1.2"), "switchboard 1.2")
				relink(t, s.binary, s.version("1.2"))
			},
			again: binaryAnswer{refusal: "invalid config"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStartedFrom(t)
			inFlight := newInFlight()
			r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, inFlight, at(start), unheeded)
			asked := askedOf(r, binaryAnswer{}, tt.again)
			relink(t, s.binary, s.version("1.1"))
			r.look()
			inFlight.begin()
			r.restart(t.Context())

			tt.change(t, s)
			inFlight.end()
			if got := r.restart(t.Context()); got != tt.wantRestart {
				t.Errorf("restart() = %v at the next quiet moment, want %v, as the binary asked again answers", got, tt.wantRestart)
			}
			if *asked != 2 {
				t.Errorf("the binary was asked %d times, want twice: again once changed", *asked)
			}
			if held := r.report().Held != ""; held == tt.wantRestart {
				t.Errorf("report() = %+v, want it held: %v", r.report(), !tt.wantRestart)
			}
		})
	}
}

func TestAFileChangedAsTheBinaryIsAskedIsLookedAtAgainBeforeTheRestart(t *testing.T) {
	const refusal = "invalid config"
	due := RestartDue{Reason: "upgraded"}
	tests := []struct {
		name string
		// change changes what the router was started from as the binary is
		// asked whether it could start, and answer is how it answers.
		change func(t *testing.T, s startedFrom)
		answer binaryAnswer
		// wantRestart is whether the next quiet moment restarts the router,
		// the binary asked wantAsked times in all: not where the config file
		// makes no valid config, which the binary isn't asked about.
		wantRestart bool
		wantAsked   int
		// wantHeard are the events told of, from the restart falling due.
		wantHeard []Event
	}{
		{
			name:      "the config file saved broken",
			change:    func(t *testing.T, s startedFrom) { writeFile(t, s.kept, "this is not toml [[[") },
			wantAsked: 1,
			wantHeard: []Event{due},
		},
		{
			name:        "the config file saved valid",
			change:      func(t *testing.T, s startedFrom) { writeFile(t, s.kept, twoAccounts) },
			wantRestart: true,
			wantAsked:   2,
			wantHeard:   []Event{due},
		},
		{
			name: "the binary moved on to another version",
			change: func(t *testing.T, s startedFrom) {
				writeFile(t, s.version("1.2"), "switchboard 1.2")
				relink(t, s.binary, s.version("1.2"))
			},
			wantRestart: true,
			wantAsked:   2,
			wantHeard:   []Event{due},
		},
		{
			name:      "the config file saved valid, the binary refusing to start before and after",
			change:    func(t *testing.T, s startedFrom) { writeFile(t, s.kept, twoAccounts) },
			answer:    binaryAnswer{refusal: refusal},
			wantAsked: 2,
			wantHeard: []Event{due, RestartHeld{Reason: "config changed", Refusal: refusal}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStartedFrom(t)
			var heard []Event
			r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, newInFlight(), at(start), func(e Event) { heard = append(heard, e) })
			asked := askedOf(r, tt.answer)
			ask := r.ask
			r.ask = func(ctx context.Context) (string, error) {
				if *asked == 0 {
					tt.change(t, s)
				}
				return ask(ctx)
			}
			relink(t, s.binary, s.version("1.1"))
			r.look()

			if r.restart(t.Context()) || r.restarting() || r.held != nil {
				t.Fatalf("restart() = true, or held: %v, want neither: the binary answered for files that have changed since", r.held)
			}
			if got := r.restart(t.Context()); got != tt.wantRestart {
				t.Errorf("at the next quiet moment, restart() = %v, want %v", got, tt.wantRestart)
			}
			if *asked != tt.wantAsked {
				t.Errorf("the binary was asked %d times, want %d", *asked, tt.wantAsked)
			}
			if !reflect.DeepEqual(heard, tt.wantHeard) {
				t.Errorf("heard %+v, want %+v: a refusal told of once, of the files it was given", heard, tt.wantHeard)
			}
		})
	}
}

func TestABinaryNotThereIsntRestartedIntoTillItsBack(t *testing.T) {
	s := newStartedFrom(t)
	r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, newInFlight(), at(start), unheeded)
	asked := askedOf(r, binaryAnswer{})
	relink(t, s.binary, s.version("1.1"))
	r.look()

	remove(t, s.binary)
	if r.restart(t.Context()) || r.restarting() {
		t.Fatal("restart() = true, want the router kept running: its binary isn't there")
	}
	if ready := r.ready(); ready != nil {
		t.Errorf("ready() = %v with the binary not there at the last look, want none, for the router not to look again and again", ready)
	}
	if err := r.restartable(t.Context()); err == nil || !strings.Contains(err.Error(), "isn't there") {
		t.Errorf("restartable() = %v, want it refused, the binary not there", err)
	}
	if *asked != 0 {
		t.Errorf("the binary was asked %d times while it wasn't there, want none", *asked)
	}

	link(t, s.version("1.1"), s.binary)
	r.look()
	if !quiet(r.ready()) {
		t.Fatal("ready() isn't closed with the binary back, want the restart to go ahead")
	}
	if !r.restart(t.Context()) {
		t.Error("restart() = false with the binary back, want the router restarted")
	}
	if *asked != 1 {
		t.Errorf("the binary was asked %d times, want once, back", *asked)
	}
}

// quiet reports whether ready is closed.
func quiet(ready <-chan struct{}) bool {
	select {
	case <-ready:
		return true
	default:
		return false
	}
}

func TestABinaryThatHasntAnsweredInTimeIsRestartedInto(t *testing.T) {
	tests := []struct {
		name string
		// restart restarts r as the router would, and reports whether it can.
		restart func(t *testing.T, r *restarts) bool
	}{
		{name: "a restart due", restart: func(t *testing.T, r *restarts) bool { return r.restart(t.Context()) }},
		{name: "a restart asked for", restart: func(t *testing.T, r *restarts) bool { return r.restartable(t.Context()) == nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				log := logstest.Capture(t)
				s := newStartedFrom(t)
				r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, newInFlight(), at(start), unheeded)
				r.ask = asking(Config{Check: unanswering, Binary: Watch(s.binary)})
				relink(t, s.binary, s.version("1.1"))
				r.look()

				began := time.Now()
				if !tt.restart(t, r) {
					t.Error("the restart didn't go ahead, want it to once the binary hasn't answered in time")
				}
				if waited := time.Since(began); waited != checkTimeout {
					t.Errorf("waited %v for the binary's answer, want %v", waited, checkTimeout)
				}
				if got := r.report(); r.held != nil || got.Held != "" {
					t.Errorf("held: %v, and report() = %+v, want nothing held", r.held, got)
				}
				if !log.Has("level=INFO", `msg="its binary couldn't answer whether it could start; restarting all the same"`, `error="context deadline exceeded"`) {
					t.Errorf("log reads\n%s\nwant the binary's silence logged", log)
				}
			})
		})
	}
}

// unanswering runs a program that never answers, until ctx ends.
func unanswering(ctx context.Context, _ string, _, _ []string) ([]byte, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestARouterToldToStopAsItAsksItsBinaryNeitherRestartsNorHolds(t *testing.T) {
	log := logstest.Capture(t)
	s := newStartedFrom(t)
	var heard []Event
	r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, newInFlight(), at(start), func(e Event) { heard = append(heard, e) })
	stopped, stop := context.WithCancel(t.Context())
	r.ask = func(ctx context.Context) (string, error) {
		stop()
		return "", ctx.Err()
	}
	relink(t, s.binary, s.version("1.1"))
	r.look()

	if r.restart(stopped) {
		t.Error("restart() = true, want false: the router was told to stop")
	}
	if r.restarting() || r.held != nil {
		t.Errorf("restarting: %v, and held: %v, want neither", r.restarting(), r.held)
	}
	if log.Has(`msg="its binary couldn't answer`) || len(heard) != 1 {
		t.Errorf("log reads\n%s\nand heard %+v, want nothing said of the binary's answer", log, heard)
	}
}

func TestARestartAskedForIsRefusedWhileItsBinaryRefusesToStart(t *testing.T) {
	tests := []struct {
		name string
		// answer is how the binary answers whether it could start.
		answer  binaryAnswer
		wantErr string
	}{
		{name: "able to start"},
		{name: "unable to answer, as one from before the check", answer: binaryAnswer{err: errors.New("exit status 1")}},
		{
			name:    "refusing",
			answer:  binaryAnswer{refusal: "invalid config /test/config.toml:\naccount work: its label is longer than 100 characters"},
			wantErr: "the router's binary refuses to start, so the router started again would stop: invalid config /test/config.toml:\naccount work: its label is longer than 100 characters",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStartedFrom(t)
			r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, newInFlight(), at(start), unheeded)
			asked := askedOf(r, tt.answer)

			err := r.restartable(t.Context())
			switch {
			case tt.wantErr == "" && err != nil:
				t.Errorf("restartable() = %v, want nil", err)
			case tt.wantErr != "" && (err == nil || err.Error() != tt.wantErr):
				t.Errorf("restartable() = %v, want %q", err, tt.wantErr)
			}
			if *asked != 1 {
				t.Errorf("the binary was asked %d times, want once", *asked)
			}
		})
	}
}

func TestARestartAskedForIsRefusedWhenAFileChangesAsItsChecked(t *testing.T) {
	saved := func(t *testing.T, s startedFrom) { writeFile(t, s.kept, twoAccounts) }
	upgraded := func(t *testing.T, s startedFrom) {
		writeFile(t, s.version("1.2"), "switchboard 1.2")
		relink(t, s.binary, s.version("1.2"))
	}
	configFile := func(s startedFrom) string { return "config file, " + s.config + "," }
	binary := func(s startedFrom) string { return "binary, " + s.binary + "," }
	tests := []struct {
		name string
		// change changes what the router was started from as the binary is
		// asked whether it could start, answering as answer says; or, with
		// asRead, as the config file is read, the router given no binary to
		// ask.
		change func(t *testing.T, s startedFrom)
		answer binaryAnswer
		asRead bool
		// named names what changed, as the refusal does.
		named func(s startedFrom) string
	}{
		{name: "the config file saved as the binary is asked", change: saved, named: configFile},
		{
			name:   "the config file saved as the binary is asked, the binary refusing",
			change: saved,
			answer: binaryAnswer{refusal: "invalid config"},
			named:  configFile,
		},
		{name: "the binary moved on to another version as it's asked", change: upgraded, named: binary},
		{
			name:   "both as the binary is asked",
			change: func(t *testing.T, s startedFrom) { saved(t, s); upgraded(t, s) },
			named:  func(s startedFrom) string { return configFile(s) + " and its " + binary(s) },
		},
		{
			name:   "the binary's link gone as it's asked, which can't answer",
			change: func(t *testing.T, s startedFrom) { remove(t, s.binary) },
			answer: binaryAnswer{err: fs.ErrNotExist},
			named:  binary,
		},
		{name: "the config file saved as it's read, with no binary to ask", change: saved, asRead: true, named: configFile},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			s := newStartedFrom(t)
			r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, newInFlight(), at(start), unheeded)
			r.ask = func(context.Context) (string, error) {
				tt.change(t, s)
				return tt.answer.refusal, tt.answer.err
			}
			if tt.asRead {
				r.ask = nil
				r.load = func(path string) (*config.Config, error) {
					tt.change(t, s)
					return config.Load(path)
				}
			}

			want := "the router's " + tt.named(s) + " changed as it checked whether it could restart: try again"
			if err := r.restartable(t.Context()); err == nil || err.Error() != want {
				t.Errorf("restartable() = %v, want %q: the check was of files no longer there", err, want)
			}
			if log.Has(`msg="its binary couldn't answer whether it could start; restarting all the same"`) {
				t.Errorf("log reads\n%s\nwant nothing said of restarting, as it doesn't", log)
			}
		})
	}
}

func TestABinarysSilenceIsLoggedOnceAsTheRestartItClearedBegins(t *testing.T) {
	said := `msg="its binary couldn't answer whether it could start; restarting all the same"`
	tests := []struct {
		name string
		// meanwhile happens before the next quiet moment, once a request has
		// put the restart off.
		meanwhile func(t *testing.T, s startedFrom)
		// again is how the binary answers, asked again.
		again       binaryAnswer
		wantRestart bool
		wantSaid    int
	}{
		{name: "nothing", meanwhile: func(*testing.T, startedFrom) {}, wantRestart: true, wantSaid: 1},
		{
			name: "the binary moved on to one that refuses to start",
			meanwhile: func(t *testing.T, s startedFrom) {
				writeFile(t, s.version("1.2"), "switchboard 1.2")
				relink(t, s.binary, s.version("1.2"))
			},
			again: binaryAnswer{refusal: "invalid config"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log := logstest.Capture(t)
			s := newStartedFrom(t)
			inFlight := newInFlight()
			r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, inFlight, at(start), unheeded)
			askedOf(r, binaryAnswer{err: errors.New("exit status 1")}, tt.again)
			relink(t, s.binary, s.version("1.1"))
			r.look()
			inFlight.begin()

			if r.restart(t.Context()) || log.Has(said) {
				t.Fatalf("restart() = true, or the log reads\n%s\nwant neither: a request put the restart off", log)
			}
			tt.meanwhile(t, s)
			inFlight.end()
			if got := r.restart(t.Context()); got != tt.wantRestart {
				t.Errorf("at the next quiet moment, restart() = %v, want %v", got, tt.wantRestart)
			}
			if n := strings.Count(log.String(), said); n != tt.wantSaid {
				t.Errorf("the log reads\n%s\nwant the binary's silence said %d times, not %d: once a restart it cleared, as it begins", log, tt.wantSaid, n)
			}
		})
	}
}

func TestABinaryUnableToAnswerIsLoggedOnlyAsItsRestartedInto(t *testing.T) {
	log := logstest.Capture(t)
	s := newStartedFrom(t)
	r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, newInFlight(), at(start), unheeded)
	asked := askedOf(r, binaryAnswer{err: errors.New("exit status 1")})
	ask := r.ask
	r.ask = func(ctx context.Context) (string, error) {
		if *asked == 0 {
			writeFile(t, s.kept, twoAccounts)
		}
		return ask(ctx)
	}
	relink(t, s.binary, s.version("1.1"))
	r.look()

	said := `msg="its binary couldn't answer whether it could start; restarting all the same"`
	if r.restart(t.Context()) || log.Has(said) {
		t.Fatalf("restart() = true, or the log reads\n%s\nwant neither: the binary's silence was of files that changed since", log)
	}
	if !r.restart(t.Context()) || strings.Count(log.String(), said) != 1 {
		t.Errorf("the log reads\n%s\nwant the router restarted, its binary's silence said once", log)
	}
}

func TestARouterRestartingAlreadyCanRestartWithoutAskingItsBinary(t *testing.T) {
	s := newStartedFrom(t)
	r := newRestarts(Watch(s.config), Watch(s.binary), Watch(s.zone), true, newInFlight(), at(start), unheeded)
	asked := askedOf(r, binaryAnswer{refusal: "invalid config"})
	r.atOnce()

	if err := r.restartable(t.Context()); err != nil {
		t.Errorf("restartable() = %v, want nil: the router is restarting already", err)
	}
	if *asked != 0 {
		t.Errorf("the binary was asked %d times, want none", *asked)
	}
}

// binaryAnswer is how a router's binary answers whether it could start: why
// it refuses, or why it couldn't answer.
type binaryAnswer struct {
	refusal string
	err     error
}

// askedOf has r ask its binary whether it could start, answered as answers
// says, in turn, and past its end as its last does, and returns how many
// times it's asked.
func askedOf(r *restarts, answers ...binaryAnswer) *int {
	asked := new(int)
	r.ask = func(context.Context) (string, error) {
		a := answers[min(*asked, len(answers)-1)]
		*asked++
		return a.refusal, a.err
	}
	return asked
}

func TestRequestsInFlight(t *testing.T) {
	inFlight := newInFlight()
	quiet := func() bool {
		select {
		case <-inFlight.quiet():
			return true
		default:
			return false
		}
	}
	if !quiet() {
		t.Fatal("quiet() isn't closed before any request")
	}
	inFlight.begin()
	inFlight.begin()
	inFlight.end()
	if quiet() {
		t.Error("quiet() is closed with a request still in flight")
	}
	inFlight.end()
	if !quiet() {
		t.Error("quiet() isn't closed once every request has ended")
	}
}

func TestTheRequestsCountedAreThoseInFlightButForUpgradedConnections(t *testing.T) {
	tests := []struct {
		name string
		// header is the request's.
		header    http.Header
		wantQuiet bool
	}{
		{name: "a request", header: http.Header{"Authorization": {"Bearer " + workToken}}},
		{name: "a connection upgraded, which can stay open as long as its session runs", header: http.Header{"Connection": {"Upgrade"}, "Upgrade": {"websocket"}}, wantQuiet: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			inFlight := newInFlight()
			arrived, release := make(chan struct{}), make(chan struct{})
			h := inFlight.count(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				close(arrived)
				<-release
			}))
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/sessions", nil)
			req.Header = tt.header
			served := make(chan struct{})
			go func() {
				h.ServeHTTP(httptest.NewRecorder(), req)
				close(served)
			}()
			<-arrived

			select {
			case <-inFlight.quiet():
				if !tt.wantQuiet {
					t.Error("quiet() is closed while the request is in flight")
				}
			default:
				if tt.wantQuiet {
					t.Error("quiet() isn't closed, want the connection passed over")
				}
			}
			close(release)
			<-served
			select {
			case <-inFlight.quiet():
			default:
				t.Error("quiet() isn't closed once the request has been served")
			}
		})
	}
}

// startedFrom is what a router was started from, in a directory of the
// test's own: its config file, a link to one kept in a dotfiles directory;
// its binary, a link to one of two versions, as Homebrew's is; and the time
// zone's file, a link to one of two zones' files, as macOS's is.
type startedFrom struct {
	root string
	// config is the config file's path, and kept where it's kept.
	config, kept string
	binary       string
	zone         string
}

// newStartedFrom returns what a router was started from: a valid config, its
// binary at version 1.0, and the time zone Etc/GMT, each last changed an
// hour ago.
func newStartedFrom(t *testing.T) startedFrom {
	t.Helper()
	root := t.TempDir()
	s := startedFrom{
		root:   root,
		config: filepath.Join(root, "config", "config.toml"),
		kept:   filepath.Join(root, "dotfiles", "config.toml"),
		binary: filepath.Join(root, "bin", "switchboard"),
		zone:   filepath.Join(root, "etc", "localtime"),
	}
	writeFile(t, s.kept, oneAccount)
	link(t, s.kept, s.config)
	for _, v := range []string{"1.0", "1.1"} {
		writeFile(t, s.version(v), "switchboard "+v)
	}
	link(t, filepath.Join("..", "Cellar", "switchboard", "1.0", "bin", "switchboard"), s.binary)
	for _, name := range []string{"Etc/GMT", "Etc/GMT-9"} {
		writeFile(t, s.zoneInfo(name), "TZif "+name)
	}
	link(t, s.zoneInfo("Etc/GMT"), s.zone)
	anHourAgo := time.Now().Add(-time.Hour)
	for _, path := range []string{s.kept, s.version("1.0"), s.version("1.1"), s.zoneInfo("Etc/GMT"), s.zoneInfo("Etc/GMT-9")} {
		if err := os.Chtimes(path, anHourAgo, anHourAgo); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

// version is where the binary of the version given is.
func (s startedFrom) version(v string) string {
	return filepath.Join(s.root, "Cellar", "switchboard", v, "bin", "switchboard")
}

// zoneInfo is where the file of the time zone named is.
func (s startedFrom) zoneInfo(name string) string {
	return filepath.Join(s.root, "zoneinfo", filepath.FromSlash(name))
}

// writeFile writes content to the file at path, in place of what it held,
// making its directory first.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// link puts a link at path leading to target, making its directory first.
func link(t *testing.T, target, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}

// relink has the link at path lead to target from now on, as ln -sf does.
func relink(t *testing.T, path, target string) {
	t.Helper()
	remove(t, path)
	link(t, target, path)
}

func rename(t *testing.T, from, to string) {
	t.Helper()
	if err := os.Rename(from, to); err != nil {
		t.Fatal(err)
	}
}

func remove(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
}
