package router_test

import (
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/router"
)

func TestARunningRouterPostsNotifications(t *testing.T) {
	cfg := runConfig(t, newAccountsAPI(t).URL)
	notifier := &notes{}
	cfg.Notifier, cfg.Notifications = notifier, config.Notifications{Moves: true}
	stop := runRouter(t, cfg)

	for _, pin := range []string{"work", "side"} {
		if status, err := askPinned("http://"+cfg.Listen, pin); err != nil || status != http.StatusOK {
			t.Fatalf("a request pinned to %s was answered %d (%v), want 200", pin, status, err)
		}
	}
	want := []string{"session 0b5c6f2e moved from work · Work to side · Side (pinned)"}
	waitUntil(t, "the move is told of", func() bool { return slices.Equal(notifier.posted(), want) })
	if err := stop(); err != nil {
		t.Errorf("Run() = %v, want nil", err)
	}
}

func TestAHangingNotifierNeverHoldsUpRouting(t *testing.T) {
	cfg := runConfig(t, newAccountsAPI(t).URL)
	notifier := &notes{hold: make(chan struct{})}
	cfg.Notifier, cfg.Notifications = notifier, config.Notifications{Moves: true}
	stop := runRouter(t, cfg)

	routed := make(chan error, 1)
	go func() {
		// Every request but the first moves the session, and more move it
		// than the notifications' queue holds.
		for i := range router.NotificationQueue + 20 {
			status, err := askPinned("http://"+cfg.Listen, []string{"work", "side"}[i%2])
			if err == nil && status != http.StatusOK {
				err = fmt.Errorf("answered %d", status)
			}
			if err != nil {
				routed <- fmt.Errorf("request %d: %w", i+1, err)
				return
			}
		}
		routed <- nil
	}()
	select {
	case err := <-routed:
		if err != nil {
			t.Errorf("with the notifier hanging, %v, want every request answered 200", err)
		}
	case <-time.After(5 * time.Second):
		t.Error("with the notifier hanging, the requests were held up")
	}
	if got := notifier.posted(); len(got) != 1 {
		t.Errorf("posted %q, want the one notification the notifier hangs on", got)
	}
	close(notifier.hold)
	if err := stop(); err != nil {
		t.Errorf("Run() = %v, want nil", err)
	}
}

// askPinned sends a messages request of the tests' session through the proxy
// at url, pinned to pin, and returns the status it's answered with.
func askPinned(url, pin string) (int, error) {
	req, err := http.NewRequest(http.MethodPost, url+"/v1/messages", strings.NewReader(messages))
	if err != nil {
		return 0, err
	}
	req.Header = with(with(claudeCode(workToken), claude.SessionHeader, sessionID), "X-Switchboard-Account", pin)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	_, err = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode, err
}

// notes is a notifier that notes each message it's given. While hold is open,
// each post waits for it to close.
type notes struct {
	hold chan struct{}

	mu       sync.Mutex
	messages []string
}

func (n *notes) Notify(message string) error {
	n.mu.Lock()
	n.messages = append(n.messages, message)
	n.mu.Unlock()
	if n.hold != nil {
		<-n.hold
	}
	return nil
}

// posted returns the messages given so far.
func (n *notes) posted() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return slices.Clone(n.messages)
}
