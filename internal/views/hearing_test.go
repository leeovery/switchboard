package views_test

import (
	"fmt"
	"maps"
	"runtime"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/views"
)

// anHoursStream is an hour of a busy day's request stream, as the watch
// keeps it, heard through a Hearing of its own: of each of the sessions
// given, as many requests, one every 6 seconds back from now, each ended;
// and those sessions, as the router lists them.
func anHoursStream(sessions, requests int) ([]status.Session, views.Stream) {
	var listed []status.Session
	s := views.Stream{Live: true, Requests: make(map[string]views.Told)}
	for i := range sessions {
		id := fmt.Sprintf("%08d-0000-4000-8000-000000000000", i)
		listed = append(listed, status.Session{ID: id, Assignments: []status.Assignment{seatOn(opus, "opus", "work", status.ReasonNew, time.Hour, 0, "")}})
		for k := range requests {
			sent := now.Add(-time.Duration(k) * 6 * time.Second)
			r := fmt.Sprintf("r%d-%d", i, k)
			s.Requests[r] = views.Told{
				Request: r, Session: id, Model: opus, Account: "work", Class: "main",
				Sent: sent, First: sent.Add(time.Second), Ended: sent.Add(3 * time.Second), Heard: sent.Add(3 * time.Second),
			}
		}
	}
	return listed, s.HeardBy(new(views.Hearing))
}

// BenchmarkRunningOfAnHoursStream is SESSIONS and ROUTING at a frame, as
// each reads RunningOf: of ten sessions with 600 requests each in the hour,
// a heavy run of several subscriptions, the stream heard as it changed.
func BenchmarkRunningOfAnHoursStream(b *testing.B) {
	sessions, s := anHoursStream(10, 600)
	m := runningDoc().At(now, claude.Policy)
	b.ReportAllocs()
	for b.Loop() {
		views.RunningOf(m, sessions, s, views.HeldList{})
	}
}

// RunningOf, as SESSIONS and ROUTING read it at each frame drawn while
// anything moves, hears the request stream once as what it told changes,
// not at each frame: a frame of an hour's stream, 600 requests of each of
// ten sessions, costs what a frame of one request each does, in the times
// it allocates and the bytes.
func TestAFrameOfAnHoursStreamCostsWhatOneOfAMinutesDoes(t *testing.T) {
	m := runningDoc().At(now, claude.Policy)
	frame := func(requests int) (allocs float64, bytes uint64) {
		sessions, s := anHoursStream(10, requests)
		drawn := func() { views.RunningOf(m, sessions, s, views.HeldList{}) }
		return testing.AllocsPerRun(20, drawn), bytesPerRun(20, drawn)
	}
	hour, hourBytes := frame(600)
	minute, minuteBytes := frame(1)
	if hour > minute || hourBytes > minuteBytes+minuteBytes/10 {
		t.Errorf("a frame of an hour's stream allocates %.0f times, %d bytes; one of a request each %.0f, %d: the stream is heard again at each frame",
			hour, hourBytes, minute, minuteBytes)
	}
}

// bytesPerRun is how many bytes f allocates a run, on average, over the
// runs given, after one to warm it.
func bytesPerRun(runs int, f func()) uint64 {
	f()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	for range runs {
		f()
	}
	runtime.ReadMemStats(&after)
	return (after.TotalAlloc - before.TotalAlloc) / uint64(runs)
}

// A Stream given a Hearing is heard once for every look at it, and again as
// what it told changes: its requests, which are another map once any
// changes, as a Stream's map is its own, copied before it changes; and
// whether it's live.
func TestAStreamHeardOnceIsHeardAgainAsWhatItToldChanges(t *testing.T) {
	sessions := []status.Session{{ID: asking, Assignments: []status.Assignment{seatOn(opus, "opus", "side", status.ReasonNew, 30*time.Minute, time.Minute, "")}}}
	m := runningDoc().At(now, claude.Policy)
	doing := func(s views.Stream) views.Doing {
		rows := views.RunningOf(m, sessions, s, views.HeldList{})
		if len(rows) != 1 {
			t.Fatalf("RunningOf gave %d rows, want 1", len(rows))
		}
		return rows[0].Activity.Doing
	}
	sent := views.Told{Request: "r1", Session: asking, Model: opus, Account: "side", Class: "main", Sent: now.Add(-time.Second), Heard: now.Add(-time.Second)}
	h := new(views.Hearing)
	s := views.Stream{Live: true, Requests: map[string]views.Told{"r1": sent}}.HeardBy(h)
	if got := doing(s); got != views.Asking {
		t.Fatalf("heard first, it's doing %d, want asking", got)
	}
	ended := sent
	ended.Ended, ended.Heard = now, now
	// What a Stream's own map holds never changes where it lies: changed so
	// here, the hearing held stands, as the stream was heard the once.
	s.Requests["r1"] = ended
	if got := doing(s); got != views.Asking {
		t.Errorf("its map changed where it lies, it's doing %d, want asking still, as heard the once", got)
	}
	s.Requests = maps.Clone(s.Requests)
	if got := doing(s); got != views.Idle {
		t.Errorf("another map, its request ended, it's doing %d, want idle, heard again", got)
	}
	s.Requests["r1"], s.Live = sent, false
	if got := doing(s); got != views.Idle {
		t.Errorf("no longer live, it's doing %d, want idle, heard again, nothing known in flight", got)
	}
	if got := doing(views.Stream{Live: true, Requests: s.Requests}.HeardBy(h)); got != views.Asking {
		t.Errorf("live again, given the same Hearing, it's doing %d, want asking, heard again", got)
	}
}
