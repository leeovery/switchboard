package router

import (
	"fmt"
	"slices"
	"sync"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

const (
	// healthWindow is how far back the router's health looks.
	healthWindow = 5 * time.Minute
	// minFailures is the fewest failures in the window that make the router
	// unhealthy, when they're half its requests at least: a failure or two
	// says little on its own.
	minFailures = 5
)

// result is how a routed request answered ended: when, and whether the router
// failed it itself.
type result struct {
	at     time.Time
	failed bool
}

// health is how the router fares with the requests it routes, judged over the
// last healthWindow: only the failures it makes itself count against it, not
// the upstream's answers it passes on. It's safe for concurrent use.
type health struct {
	now func() time.Time
	// emit hears each change, as it's logged.
	emit func(Event)

	mu sync.Mutex
	// results are those of the requests answered in the window.
	results []result
	// healthy is the router's health as last judged.
	healthy bool
	// awoke is when the router last noticed the Mac wake from sleep.
	awoke time.Time
}

func newHealth(now func() time.Time, emit func(Event)) *health {
	return &health{now: now, emit: emit, healthy: true}
}

// record notes a routed request that arrived at arrived, answered now, and
// whether the router failed it itself. A failure of one that arrived before
// the router last noticed the Mac wake isn't counted: it may have gone out on
// a connection the sleep left dead, which is the sleep's doing, not the
// router's.
func (h *health) record(arrived time.Time, failed bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := h.now()
	if !failed || !arrived.Before(h.awoke) {
		h.results = append(h.results, result{at: now, failed: failed})
	}
	h.judge(now)
}

// wake notes that the router has noticed the Mac wake from sleep, now.
func (h *health) wake() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.awoke = h.now()
}

// report is the router's health now.
func (h *health) report() status.Health {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.judge(h.now())
}

// judge forgets the requests that have left the window at now, and judges the
// router's health on the rest, logging and announcing any change. A change
// is announced as it's judged, under h.mu, so announcements can't cross.
func (h *health) judge(now time.Time) status.Health {
	h.results = slices.DeleteFunc(h.results, func(r result) bool { return !inWindow(r, now) })
	judged := assess(h.results, now)
	if judged.Healthy == h.healthy {
		return judged
	}
	h.healthy = judged.Healthy
	if judged.Healthy {
		logger.Info("healthy again", "requests", judged.Requests, "failures", judged.Failures)
	} else {
		logger.Warn("unhealthy", "reason", judged.Reason)
	}
	h.emit(HealthChanged{Healthy: judged.Healthy, Reason: judged.Reason})
	return judged
}

// assess judges the router's health at now on the requests answered in the
// window before it: unhealthy when it failed minFailures of them at least,
// and half of them at least.
func assess(results []result, now time.Time) status.Health {
	var judged status.Health
	for _, r := range results {
		if !inWindow(r, now) {
			continue
		}
		judged.Requests++
		if r.failed {
			judged.Failures++
		}
	}
	judged.Healthy = judged.Failures < minFailures || 2*judged.Failures < judged.Requests
	if !judged.Healthy {
		judged.Reason = fmt.Sprintf("%d of the %d requests in the last %d minutes failed",
			judged.Failures, judged.Requests, int(healthWindow/time.Minute))
	}
	return judged
}

// inWindow reports whether a request's result counts at now.
func inWindow(r result, now time.Time) bool {
	return now.Sub(r.at) < healthWindow
}
