package cli_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/cli"
)

// Every object a data verb prints as JSON carries the schema its fields keep
// to, first, so a reader knows the meaning of what follows before reading it.
func TestEachDataVerbsJSONCarriesItsSchemaFirst(t *testing.T) {
	routed := func(t *testing.T) cli.Deps {
		srv := newServeSetup(t, fakeClaudeAPI(t), nil)
		srv.start(t)
		srv.route(t, sessionOne, "claude-haiku-4-5-20251001")
		return srv.deps
	}
	probed := func(t *testing.T) cli.Deps { return statusDeps(t, fakeClaudeAPI(t), nil) }
	listed := func(t *testing.T) cli.Deps {
		deps := sessionsDeps(t)
		serveSessions(t, deps, routedSessions)
		return deps
	}
	tests := []struct {
		args []string
		deps func(t *testing.T) cli.Deps
	}{
		{args: []string{"status", "--json"}, deps: probed},
		{args: []string{"status", "--session", sessionOne, "--json"}, deps: routed},
		{args: []string{"usage", "--json"}, deps: probed},
		{args: []string{"sessions", "--json"}, deps: listed},
		{args: []string{"sessions", moving, "--json"}, deps: listed},
		{args: []string{"history", "--json"}, deps: weeksDeps},
		{args: []string{"history", "--windows", "--json"}, deps: weeksDeps},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			got := run(t, tt.deps(t), tt.args...)
			if got.code != 0 {
				t.Fatalf("switchboard %s = %+v, want it to succeed", strings.Join(tt.args, " "), got)
			}
			if key, value := firstField(t, got.stdout); key != "schema" || value != "1" {
				t.Errorf("switchboard %s printed %q: %s first, want \"schema\": 1", strings.Join(tt.args, " "), got.stdout, key)
			}
		})
	}
}

// firstField is the key of the first field of the JSON object printed, and
// its value as JSON.
func firstField(t *testing.T, printed string) (key, value string) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(printed))
	if open, err := dec.Token(); err != nil || open != json.Delim('{') {
		t.Fatalf("printed %q, want a JSON object (%v)", printed, err)
	}
	name, err := dec.Token()
	if err != nil {
		t.Fatalf("printed %q, want a field (%v)", printed, err)
	}
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		t.Fatalf("printed %q, want its first field's value (%v)", printed, err)
	}
	key, _ = name.(string)
	return key, string(bytes.TrimSpace(raw))
}
