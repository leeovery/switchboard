package watch

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/status"
)

func TestNotifications(t *testing.T) {
	// Room: the session and the week apply to every model, so an account with
	// either used up has no room.
	full := func(weekUsed float64) status.Document {
		return document(account("work", "Work", refused(session(1, 30*time.Minute)), week(weekUsed)))
	}
	room := func(weekUsed float64) status.Document {
		return document(account("work", "Work", session(0.02, 5*time.Hour), week(weekUsed)))
	}
	unread := document(unreadable("work", "Work"))
	tests := []struct {
		name string
		// reads are read in turn, five minutes apart.
		reads []status.Document
		want  []string
	}{
		{
			name:  "an account with room again",
			reads: []status.Document{full(0.5), room(0.5)},
			want:  []string{"work · Work has room again"},
		},
		{
			name: "an account with room again once its window reset, each reading judged when it was read",
			reads: []status.Document{
				document(account("work", "Work", refused(session(1, 3*time.Minute)), week(0.5))),
				room(0.5),
			},
			want: []string{"work · Work has room again"},
		},
		{
			name: "an account with room again once the window at its reserve resets",
			reads: []status.Document{
				document(reserving(account("work", "Work", session(0.95, 3*time.Minute), week(0.5)), 0.1)),
				document(reserving(account("work", "Work", session(0.02, 5*time.Hour), week(0.5)), 0.1)),
			},
			want: []string{"work · Work has room again"},
		},
		{
			name: "an account still at its reserve",
			reads: []status.Document{
				document(reserving(account("work", "Work", session(0.2, 5*time.Hour), week(0.93)), 0.1)),
				document(reserving(account("work", "Work", session(0.3, 5*time.Hour), week(0.94)), 0.1)),
			},
		},
		{
			name:  "an account still without room",
			reads: []status.Document{full(0.5), full(0.5)},
		},
		{
			name:  "an account with room all along",
			reads: []status.Document{room(0.2), room(0.5)},
		},
		{
			name: "a window only some models count",
			reads: []status.Document{
				document(account("work", "Work", session(0.2, 5*time.Hour), week(0.5), refused(fableWeek(1)))),
				document(account("work", "Work", session(0.2, 5*time.Hour), week(0.5), fableWeek(0.1))),
			},
		},
		{
			name:  "a window passing 90%",
			reads: []status.Document{room(0.85), room(0.91)},
			want:  []string{"work · Work: Week at 91%"},
		},
		{
			name:  "a window reaching 90%",
			reads: []status.Document{room(0.89), room(0.9)},
			want:  []string{"work · Work: Week at 90%"},
		},
		{
			name:  "a window staying over 90%",
			reads: []status.Document{room(0.85), room(0.91), room(0.95)},
			want:  []string{"work · Work: Week at 91%"},
		},
		{
			name:  "a window falling below 90%",
			reads: []status.Document{room(0.95), room(0.5)},
		},
		{
			name:  "a window passing 90% again after it reset",
			reads: []status.Document{room(0.85), room(0.91), room(0.1), room(0.92)},
			want:  []string{"work · Work: Week at 91%", "work · Work: Week at 92%"},
		},
		{
			name: "a window new to the account",
			reads: []status.Document{
				room(0.5),
				document(account("work", "Work", session(0.02, 5*time.Hour), week(0.5), fableWeek(0.95))),
			},
		},
		{
			name:  "both at once, in order",
			reads: []status.Document{full(0.85), room(0.93)},
			want:  []string{"work · Work has room again", "work · Work: Week at 93%"},
		},
		{
			name: "each account for itself",
			reads: []status.Document{
				document(account("work", "Work", session(0.2, 5*time.Hour), week(0.85)), account("personal", "Personal", session(0.2, 5*time.Hour), week(0.5))),
				document(account("work", "Work", session(0.2, 5*time.Hour), week(0.85)), account("personal", "Personal", session(0.2, 5*time.Hour), week(0.92))),
			},
			want: []string{"personal · Personal: Week at 92%"},
		},
		{
			name:  "nothing for the first read",
			reads: []status.Document{full(0.95)},
		},
		{
			name:  "across a read that failed",
			reads: []status.Document{full(0.85), unread, room(0.91)},
			want:  []string{"work · Work has room again", "work · Work: Week at 91%"},
		},
		{
			name:  "nothing for an account's first reading after failing",
			reads: []status.Document{unread, room(0.95)},
		},
		{
			name:  "nothing when a read fails",
			reads: []status.Document{room(0.5), unread},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, tt.reads[0])
			h.start()
			for _, doc := range tt.reads[1:] {
				h.clock.now = h.clock.now.Add(5 * time.Minute)
				h.read(doc)
			}

			if !slices.Equal(h.notifier.posted, tt.want) {
				t.Errorf("posted %q, want %q", h.notifier.posted, tt.want)
			}
		})
	}
}

func TestNotificationsAsTheConfigSays(t *testing.T) {
	const roomAgain = "work · Work has room again"
	tests := []struct {
		name     string
		settings config.Notifications
		// before and after are how much of the week is used at each read:
		// the session has no room at the first, and has at the second.
		before, after float64
		want          []string
	}{
		{name: "as it says when it doesn't", settings: notifications, before: 0.85, after: 0.91, want: []string{roomAgain, "work · Work: Week at 91%"}},
		{name: "a warning at the share given", settings: config.Notifications{Room: true, Warning: 0.8}, before: 0.75, after: 0.85, want: []string{roomAgain, "work · Work: Week at 85%"}},
		{name: "none short of the share given", settings: config.Notifications{Room: true, Warning: 0.95}, before: 0.85, after: 0.91, want: []string{roomAgain}},
		{name: "no warning at 0", settings: config.Notifications{Room: true}, before: 0.85, after: 0.91, want: []string{roomAgain}},
		{name: "no room again when it's off", settings: config.Notifications{Warning: 0.9}, before: 0.85, after: 0.91, want: []string{"work · Work: Week at 91%"}},
		{name: "none at all", settings: config.Notifications{Limits: true, Moves: true}, before: 0.85, after: 0.91},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t, document(account("work", "Work", refused(session(1, 30*time.Minute)), week(tt.before))))
			h.model.cfg.Notifications = tt.settings
			h.start()

			h.clock.now = h.clock.now.Add(5 * time.Minute)
			h.read(document(account("work", "Work", session(0.02, 5*time.Hour), week(tt.after))))
			if !slices.Equal(h.notifier.posted, tt.want) {
				t.Errorf("posted %q, want %q", h.notifier.posted, tt.want)
			}
		})
	}
}

func TestNotificationsAreLogged(t *testing.T) {
	log := logstest.Capture(t)
	h := newHarness(t, document(account("work", "Work", refused(session(1, 30*time.Minute)), week(0.85))))
	h.start()

	h.clock.now = at(13, 20, 0)
	h.read(document(account("work", "Work", session(0.02, 5*time.Hour), week(0.91))))
	for _, want := range [][]string{
		{"level=INFO", "msg=notification component=watch", "account=work", `news="room again"`},
		{"level=INFO", "msg=notification component=watch", "account=work", `news="Week at 91%"`},
	} {
		if !log.Has(want...) {
			t.Errorf("log reads\n%s\nwant a line with %q", log, want)
		}
	}
	if strings.Contains(log.String(), "Work") {
		t.Errorf("log reads\n%s\nwant the account named by its id alone, not its label", log)
	}
}

func TestFailedNotificationsGoOnlyToTheLog(t *testing.T) {
	log := logstest.Capture(t)
	h := newHarness(t, sessionAndWeek(0.2, 0.85))
	h.notifier.err = errors.New("post notification: exit status 1")
	h.start()

	h.clock.now = at(13, 20, 0)
	h.read(sessionAndWeek(0.2, 0.91))
	if want := []string{"work · Work: Week at 91%"}; !slices.Equal(h.notifier.posted, want) {
		t.Errorf("posted %q, want %q", h.notifier.posted, want)
	}
	if got, want := h.footer(), "updated 13:20 · next 13:50 · r refresh · q quit"; got != want {
		t.Errorf("footer = %q, want %q: a failed notification changes nothing on screen", got, want)
	}
	want := []string{"level=WARN", `msg="notification failed" component=watch`, "account=work", `news="Week at 91%"`, `error="post notification: exit status 1"`}
	if !log.Has(want...) {
		t.Errorf("log reads\n%s\nwant a line with %q", log, want)
	}
	if log.Has("msg=notification ") {
		t.Errorf("log reads\n%s\nwant no word of the notification going", log)
	}
}
