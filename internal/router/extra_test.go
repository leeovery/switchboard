package router_test

import (
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/quota"
	"github.com/leeovery/switchboard/internal/router"
)

func TestExtraUsageIsReadOffAnswers(t *testing.T) {
	on := quota.ExtraUsage{Status: quota.StatusAllowed, Utilization: new(0.4), ResetsAt: now.Add(72 * time.Hour)}
	warned := quota.ExtraUsage{Status: quota.StatusAllowedWarning, Utilization: new(0.85), ResetsAt: on.ResetsAt}
	steps := []struct {
		name string
		// gives is the extra usage the answer's headers give: none where
		// it's zero.
		gives, want quota.ExtraUsage
	}{
		{name: "an answer's headers reach the document", gives: on, want: on},
		{name: "a later answer without them leaves the last given", want: on},
		{name: "a later answer's replace them", gives: warned, want: warned},
	}
	var mu sync.Mutex
	var gives quota.ExtraUsage
	up := newUpstream(t, func(w http.ResponseWriter, _ *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		reportWindows(w.Header(), []quota.Window{session, week})
		reportExtra(w.Header(), gives)
		_, _ = io.WriteString(w, `{"type":"message"}`)
	})
	rt := newRouter(t, up.URL)
	rt.SetChooser(&fixedChooser{account: "side"})
	proxy := serveProxy(t, rt)

	for _, step := range steps {
		mu.Lock()
		gives = step.gives
		mu.Unlock()
		readAll(t, send(t, http.MethodPost, proxy+"/v1/messages", claudeCode(workToken), strings.NewReader(messages)))
		side, _ := rt.Status().Account("side")
		if !reflect.DeepEqual(side.Extra, step.want) {
			t.Errorf("%s: side reads extra usage %s, want %s", step.name, extraText(side.Extra), extraText(step.want))
		}
		if want := []quota.Window{session, week}; !reflect.DeepEqual(side.Windows, want) {
			t.Errorf("%s: side reads windows %+v, want %+v: extra usage is never a window", step.name, side.Windows, want)
		}
	}
	if work, _ := rt.Status().Account("work"); work.Extra.Given() {
		t.Errorf("work, whose token the client sent, reads extra usage %s, want none", extraText(work.Extra))
	}
}

func TestExtraUsageIsReadOffProbes(t *testing.T) {
	on := quota.ExtraUsage{Status: quota.StatusAllowed, Utilization: new(0.4), ResetsAt: now.Add(72 * time.Hour)}
	r := newRouted(t)
	r.readsAs(workToken, session, week)
	r.readsAs(sideToken, session, week)
	r.prober.answer(sideToken, probeResult{usage: quota.Usage{Windows: []quota.Window{session, week}, Extra: on}})
	want := map[string]quota.ExtraUsage{"work": {}, "personal": {}, "side": on}

	probed, err := router.NewClient(serveControl(t, r.rt)).Refresh(t.Context(), time.Minute)
	if err != nil {
		t.Fatalf("Refresh() error = %v", err)
	}
	for id, wantExtra := range want {
		a, _ := probed.Account(id)
		if !reflect.DeepEqual(a.Extra, wantExtra) {
			t.Errorf("once probed, %s reads extra usage %s, want %s", id, extraText(a.Extra), extraText(wantExtra))
		}
		if wantWindows := []quota.Window{session, week}; a.TokenSet && !reflect.DeepEqual(a.Windows, wantWindows) {
			t.Errorf("once probed, %s reads windows %+v, want %+v: extra usage is never a window", id, a.Windows, wantWindows)
		}
	}

	if got := r.ask(t, sessionID, opus, "side"); got != "side" {
		t.Fatalf("the request went out on %s, want side, as pinned", got)
	}
	if side, _ := r.rt.Status().Account("side"); !reflect.DeepEqual(side.Extra, on) {
		t.Errorf("after an answer giving none, side reads extra usage %s, want its probe's %s", extraText(side.Extra), extraText(on))
	}
}

func TestTheRoutersDocumentLeavesOutExtraUsageWhereNoneWasGiven(t *testing.T) {
	up := newUpstream(t, answerWith(http.StatusOK, session, week))
	rt := newRouter(t, up.URL)
	rt.SetChooser(&fixedChooser{account: "side"})
	proxy := serveProxy(t, rt)
	readAll(t, send(t, http.MethodPost, proxy+"/v1/messages", claudeCode(workToken), strings.NewReader(messages)))

	data, err := json.Marshal(rt.Status())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "extra_usage") {
		t.Errorf("the document reads\n%s\nwant no extra_usage, as no answer gave it", data)
	}
}

// reportExtra sets the headers that give extra usage, as the API's do: each
// of its fields given, and none where it's zero.
func reportExtra(h http.Header, e quota.ExtraUsage) {
	const prefix = "anthropic-ratelimit-unified-overage-"
	if e.Status != "" {
		h.Set(prefix+"status", string(e.Status))
	}
	if e.Utilization != nil {
		h.Set(prefix+"utilization", strconv.FormatFloat(*e.Utilization, 'f', -1, 64))
	}
	if !e.ResetsAt.IsZero() {
		h.Set(prefix+"reset", strconv.FormatInt(e.ResetsAt.Unix(), 10))
	}
}

// extraText gives extra usage as JSON, its utilization's value rather than
// its address.
func extraText(e quota.ExtraUsage) string {
	data, err := json.Marshal(e)
	if err != nil {
		return err.Error()
	}
	return string(data)
}
