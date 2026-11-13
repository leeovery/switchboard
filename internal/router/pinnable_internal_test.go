package router

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// An account there's none of, pinned in a pool, is refused naming the
// accounts that can be pinned in that pool alone, as run --account names a
// launch's pool's: a pin of Acme's, named or by the accounts beside it, or
// a session's own pin in Acme, never offers Default's. A pin whose pool
// isn't known, as of no account the router has, names every pool's.
func TestAnAccountThereIsNoneOfIsRefusedNamingThePinsPoolsAccounts(t *testing.T) {
	r := newPoolsRouter(t, func() time.Time { return start }, true)
	if w := routeIn(t, r, personalToken, "one", acmePool, nil); w.Code != http.StatusOK {
		t.Fatalf("a request in Acme was answered %d, want 200", w.Code)
	}
	tests := []struct {
		name, path, body, want string
	}{
		{name: "a pin in Acme, named", path: "/pin?pool=" + acmePool, body: `{"accounts": ["nope"]}`, want: `there's no account \"nope\": pin personal`},
		{name: "a pin beside Acme's account", path: "/pin", body: `{"accounts": ["personal", "nope"]}`, want: `there's no account \"nope\": pin personal`},
		{name: "a pin beside Default's account", path: "/pin", body: `{"accounts": ["nope", "side"]}`, want: `there's no account \"nope\": pin work or side`},
		{name: "a pin of no pool known", path: "/pin", body: `{"accounts": ["nope"]}`, want: `there's no account \"nope\": pin work, personal or side`},
		{name: "a session's own pin in Acme", path: "/sessions/one/pin", body: `{"account": "nope"}`, want: `there's no account \"nope\": pin personal`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, tt.path, strings.NewReader(tt.body))
			w := httptest.NewRecorder()
			r.Control().ServeHTTP(w, req)
			if w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), tt.want) {
				t.Errorf("POST %s %s answered %d, %s; want 400, saying %s", tt.path, tt.body, w.Code, w.Body.String(), tt.want)
			}
		})
	}
}
