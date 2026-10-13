package watch

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/notify"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/status/statustest"
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
	// pastTheWarning has the week read at 95%, resetting three minutes after
	// the clock starts: read again, it's still that reading, of the week
	// before its reset, as the router keeps it until the account is read again.
	pastTheWarning := document(account("work", "Work", session(0.02, 5*time.Hour), window("7d", "Week", 0.95, 3*time.Minute)))
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
			name: "a window passing 90% in the period after it reset, read before at more, which counts from empty",
			reads: []status.Document{
				document(account("work", "Work", session(0.02, 5*time.Hour), window("7d", "Week", 0.95, 3*time.Minute))),
				room(0.92),
			},
			want: []string{"work · Work: Week at 92%"},
		},
		{
			name:  "a window read past 90% whose reset has passed, fetched again and again before it's read again: nothing, as it stands empty",
			reads: []status.Document{pastTheWarning, pastTheWarning, pastTheWarning, pastTheWarning},
		},
		{
			name:  "a window read past 90% whose reset has passed, then read again passing 90%: once",
			reads: []status.Document{pastTheWarning, pastTheWarning, pastTheWarning, room(0.92), room(0.93)},
			want:  []string{"work · Work: Week at 92%"},
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

func TestRoomAgainIsAnAccountsQuotaBack(t *testing.T) {
	withRoom := account("work", "Work", session(0.2, 5*time.Hour), week(0.5))
	probedWithRoom := builtBy(status.SourceProbe, withRoom)
	spentSession := account("work", "Work", refused(session(1, 3*time.Minute)), week(0.5))
	pinned := builtBy(status.SourceRouter, reserving(account("work", "Work", session(0.95, 3*time.Minute), week(0.5)), 0.1))
	pinned.Pin = status.Pin{Accounts: []string{"work"}, Since: start.UTC()}
	tests := []struct {
		name string
		// last is the document the account was last read in, as the clock
		// starts, and this the one it's read in five minutes later.
		last, this status.Document
		want       bool
	}{
		{
			name: "after a limit holding back every request",
			last: builtBy(status.SourceRouter, limitedTo(withRoom, 3*time.Minute)), this: probedWithRoom, want: true,
		},
		{
			name: "after a limit holding back a model's requests alone",
			last: builtBy(status.SourceRouter, limitedTo(account("work", "Work", session(0.2, 5*time.Hour), week(0.5), fableWeek(0.5)), 3*time.Minute, "7d_oi")),
			this: probedWithRoom,
		},
		{
			name: "after its token refused, which isn't quota",
			last: builtBy(status.SourceRouter, tokenRefused(withRoom, 3*time.Minute)), this: probedWithRoom,
		},
		{
			name: "after its token refused and a window every model shares spent, both past",
			last: builtBy(status.SourceRouter, tokenRefused(spentSession, 3*time.Minute)), this: probedWithRoom, want: true,
		},
		{
			name: "after a window every model shares spent, its token refused since",
			last: builtBy(status.SourceProbe, spentSession), this: builtBy(status.SourceRouter, tokenRefused(withRoom, time.Hour)),
		},
		{name: "after its reserve, which the global pin spent", last: pinned, this: probedWithRoom},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := lastReads{}.with(tt.last.At(start, policy)).alerts(tt.this.At(start.Add(5*time.Minute), policy), notifications)
			if posted := slices.Contains(messagesOf(got), "work · Work has room again"); posted != tt.want {
				t.Errorf("posted %q, want room again %v", messagesOf(got), tt.want)
			}
		})
	}
}

func TestAlertsAtEachHardMomentAgreeWithTheMoment(t *testing.T) {
	// roomAgain is what's posted at each moment of its document, read as
	// built, then fetched again with nothing read since; warned is what's
	// posted of warnings at each moment of its document, its windows read
	// unused before.
	roomAgain := map[string][]string{
		"a week at its reserve when built, then reset": {"work · work has room again"},
		"a limit holding, then lifted":                 {"work · work has room again"},
	}
	warned := map[string][]string{
		"a week at its reserve when built":                           {"work · work: Week at 92%"},
		"a week at its reserve when built, the global pin naming it": {"work · work: Week at 92%"},
		"a limit holding":                      {"work · work: Session at 100%"},
		"probed, of one account, with no room": {"work · work: Week at 100%"},
	}
	for _, hm := range statustest.HardMoments(policy) {
		t.Run(hm.Name, func(t *testing.T) {
			read, look := hm.Doc.At(statustest.Built, policy), hm.Doc.At(hm.Now, policy)
			var back, warnings []string
			for _, a := range look.Accounts {
				if was, _ := read.Account(a.ID); was.OutOfQuota && a.CanRise {
					back = append(back, a.Title()+" has room again")
				}
				for _, w := range a.Windows {
					if w.Used >= notifications.Warning {
						warnings = append(warnings, a.Title()+": "+w.Label+" at "+status.Percent(w.Used))
					}
				}
			}

			fetched := messagesOf(lastReads{}.with(read).alerts(look, notifications))
			if !slices.Equal(fetched, back) || !slices.Equal(back, roomAgain[hm.Name]) {
				t.Errorf("fetched again with nothing read since, posted %q; want %q, as the moments say, which is %q",
					fetched, back, roomAgain[hm.Name])
			}
			warnOnly := config.Notifications{Warning: notifications.Warning}
			fresh := messagesOf(lastReads{}.with(unused(hm.Doc).At(statustest.Built, policy)).alerts(look, warnOnly))
			if !slices.Equal(fresh, warnings) || !slices.Equal(warnings, warned[hm.Name]) {
				t.Errorf("read from unused, posted %q; want %q, as the moment says, which is %q", fresh, warnings, warned[hm.Name])
			}
		})
	}
}

// builtBy is the document source builds as the clock starts of the accounts
// given, each as it stands then, as AsOf has it.
func builtBy(source string, accounts ...status.Account) status.Document {
	doc := status.Document{GeneratedAt: start.UTC(), Source: source}
	if source == status.SourceRouter {
		doc.Router = status.Health{Healthy: true}
	}
	for _, a := range accounts {
		doc.Accounts = append(doc.Accounts, a.AsOf(policy, start))
	}
	return doc
}

// limitedTo is a held back by the router's limit in the windows given, until
// lifts after the clock starts.
func limitedTo(a status.Account, lifts time.Duration, windows ...string) status.Account {
	a.Limit = status.Limit{ID: 4, Windows: windows, Until: start.Add(lifts).UTC()}
	return a
}

// tokenRefused is a whose token the upstream refuses, until lifts after the
// clock starts.
func tokenRefused(a status.Account, lifts time.Duration) status.Account {
	a.Refused = status.Refusal{Until: start.Add(lifts).UTC(), Status: 401}
	return a
}

// unused is doc with each of its accounts' windows read unused.
func unused(doc status.Document) status.Document {
	doc.Accounts = slices.Clone(doc.Accounts)
	for i, a := range doc.Accounts {
		a.Windows = slices.Clone(a.Windows)
		for j := range a.Windows {
			a.Windows[j].Utilization, a.Windows[j].Status = 0, ""
		}
		doc.Accounts[i] = a
	}
	return doc
}

// messagesOf are what the notices say, in turn.
func messagesOf(notices []notify.Notice) []string {
	var messages []string
	for _, n := range notices {
		messages = append(messages, n.Message)
	}
	return messages
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
	if got, want := h.keyLine(), oneKeys; got != want {
		t.Errorf("the key line reads %q, want %q: a failed notification changes nothing on screen", got, want)
	}
	want := []string{"level=WARN", `msg="notification failed" component=watch`, "account=work", `news="Week at 91%"`, `error="post notification: exit status 1"`}
	if !log.Has(want...) {
		t.Errorf("log reads\n%s\nwant a line with %q", log, want)
	}
	if log.Has("msg=notification ") {
		t.Errorf("log reads\n%s\nwant no word of the notification going", log)
	}
}
