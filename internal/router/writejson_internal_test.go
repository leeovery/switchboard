package router

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/logs/logstest"
	"github.com/leeovery/switchboard/internal/status"
)

func TestAnAnswerThatCantBeJSONIsA500(t *testing.T) {
	log := logstest.Capture(t)
	// A time RFC 3339 can't write, as JSON gives times.
	doc := status.Document{GeneratedAt: time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)}
	w := httptest.NewRecorder()

	writeJSON(w, doc)
	var got problem
	if err := json.Unmarshal(w.Body.Bytes(), &got); w.Code != http.StatusInternalServerError || err != nil || got.Error == "" {
		t.Errorf("answered %d, %q, want a 500 saying why", w.Code, w.Body.String())
	}
	if !log.Has("level=ERROR", `msg="can't give the answer as JSON"`) {
		t.Errorf("the log reads\n%s\nwant why the answer couldn't be given", log)
	}
}

func TestAnAnswerIsJSON(t *testing.T) {
	w := httptest.NewRecorder()

	writeJSON(w, problem{Error: "none"})
	if w.Code != http.StatusOK || w.Header().Get("Content-Type") != "application/json" || w.Body.String() != "{\"error\":\"none\"}\n" {
		t.Errorf("answered %d, %q, %q; want the answer as JSON", w.Code, w.Header().Get("Content-Type"), w.Body.String())
	}
}
