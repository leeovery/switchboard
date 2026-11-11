package cli_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/cli"
	"github.com/leeovery/switchboard/internal/status"
)

// Every object a data verb prints as JSON carries the schema its fields keep
// to, first, so a reader knows the meaning of what follows before reading it:
// each line of those that print a line a stored line too.
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
	ledgered := func(t *testing.T) cli.Deps {
		deps, _ := ledgerDeps(t)
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
		{args: []string{"requests", "--json"}, deps: ledgered},
		{args: []string{"events", "--json"}, deps: eventsDeps},
	}
	for _, tt := range tests {
		t.Run(strings.Join(tt.args, " "), func(t *testing.T) {
			got := run(t, tt.deps(t), tt.args...)
			if got.code != 0 {
				t.Fatalf("switchboard %s = %+v, want it to succeed", strings.Join(tt.args, " "), got)
			}
			holdsSchemaFirst(t, got.stdout)
		})
	}

	t.Run("events -f", func(t *testing.T) {
		deps := eventsDeps(t)
		stdout, interrupt := follow(t, deps, offATerminal, "events", "--since", "12:00", "-f")
		listed := printedJSON(t, restartDue, routerUnwell)
		stdout.waitFor(t, listed)
		followed := eventLine(runTwo, 2, status.Event{At: october(7, 13, 10, 0), Kind: status.EventHealth})
		fileEvent(t, deps, "2026-10-07", followed)
		stdout.waitFor(t, listed+printedJSON(t, followed))
		if code, stderr := interrupt(); code != 0 || stderr != "" {
			t.Fatalf("switchboard events -f exited %d, printing %q on stderr, once interrupted; want 0 and nothing", code, stderr)
		}
		holdsSchemaFirst(t, stdout.String())
	})
}

// holdsSchemaFirst fails the test unless printed is one JSON object or more,
// each with "schema": 1 its first field.
func holdsSchemaFirst(t *testing.T, printed string) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(printed))
	objects := 0
	for dec.More() {
		var object json.RawMessage
		if err := dec.Decode(&object); err != nil {
			t.Fatalf("printed %q, want JSON objects (%v)", printed, err)
		}
		if key, value := firstField(t, object); key != "schema" || value != "1" {
			t.Errorf("printed %s: %s first, want \"schema\": 1", object, key)
		}
		objects++
	}
	if objects == 0 {
		t.Errorf("printed %q, want a JSON object at least", printed)
	}
}

// firstField is the key of the first field of the JSON object given, and its
// value as JSON.
func firstField(t *testing.T, object json.RawMessage) (key, value string) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(string(object)))
	if open, err := dec.Token(); err != nil || open != json.Delim('{') {
		t.Fatalf("printed %s, want a JSON object (%v)", object, err)
	}
	name, err := dec.Token()
	if err != nil {
		t.Fatalf("printed %s, want a field (%v)", object, err)
	}
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		t.Fatalf("printed %s, want its first field's value (%v)", object, err)
	}
	key, _ = name.(string)
	return key, string(raw)
}

// withSchema is lines, each a JSON object on a line of its own, as a data
// verb prints them: each with the schema put first.
func withSchema(lines string) string {
	return strings.ReplaceAll("\n"+lines, "\n{", "\n{\"schema\":1,")[1:]
}
