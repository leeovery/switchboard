package idmove_test

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/idmove"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/readings"
	"github.com/leeovery/switchboard/internal/status"
	"github.com/leeovery/switchboard/internal/tokens"
)

// The fixture's accounts: two configured, by their old ids, each with its
// token and its token's organization, and one the history alone tells of,
// removed before ids were made.
const (
	work     = "work"
	personal = "personal"
	gone     = "old"

	workToken     = "test-token-work"
	personalToken = "test-token-personal"

	workOrganization     = "test-org-work"
	personalOrganization = "test-org-personal"

	// workID and personalID are the ids the two organizations derive, as
	// identity's tests pin them.
	workID     = "b6s7"
	personalID = "qgr2"
)

// The fixture's days: one long done, compressed, and yesterday and today,
// plain.
const (
	longAgo   = "2026-11-03"
	yesterday = "2026-11-07"
	today     = "2026-11-08"
)

// now is when the fixture's move is made, by the local clock.
var now = time.Date(2026, 11, 8, 12, 0, 0, 0, time.Local)

// clock reads now.
func clock() time.Time { return now }

// fixtureConfig is the fixture's config: work labelled and the primary,
// personal without a label.
const fixtureConfig = `[[account]]
id      = "work"
label   = "Work"
primary = true

[[account]]
id = "personal"
`

// fixture is a switchboard install to move: its config file, its state
// directory and its token files.
type fixture struct {
	config, state string
	tokens        tokens.Store
}

// newFixture returns an install of the fixture's config, its accounts'
// tokens, several days of their history, every kind of it, and a state
// file naming them.
func newFixture(t *testing.T) fixture {
	t.Helper()
	f := fixture{config: filepath.Join(t.TempDir(), "config.toml"), state: t.TempDir()}
	f.tokens = tokens.NewStore(f.state, os.Getuid())
	writeFile(t, f.config, fixtureConfig)
	f.token(t, work, workToken)
	f.token(t, personal, personalToken)
	f.history(t)
	return f
}

// token writes the account's token, and its record.
func (f fixture) token(t *testing.T, id, token string) {
	t.Helper()
	parsed, err := tokens.Parse(token)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.tokens.Write(id, parsed, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
}

// history writes the fixture's history: the readings history's lines, the
// request ledger's and the router's events', on the long-done day,
// compressed, and on yesterday and today, plain; the ledger's summaries of
// the days that have ended; a file the ledger built of a day's lines; and
// the router's state file.
func (f fixture) history(t *testing.T) {
	t.Helper()
	read := readingsFiles(f.state)
	putDay(t, read, longAgo, true, reading(longAgo, work, 0.2), reading(longAgo, gone, 0.4), reading(longAgo, personal, 0.1))
	putDay(t, read, yesterday, false, reading(yesterday, work, 0.5), reading(yesterday, personal, 0.3))
	putDay(t, read, today, false, reading(today, work, 0.6), reading(today, personal, 0.35))

	lines := ledger.Files(ledger.Dir(f.state), logs.For("test"))
	putDay(t, lines, longAgo, true,
		line(longAgo, "r1", work, status.PassingOver(status.ReasonNew, personal), "", nil),
		line(longAgo, "r2", gone, "new", "", nil),
		line(longAgo, "r3", personal, status.PassingOver(status.ReasonMovedOff+work+" "+status.WhyLimit, gone), work, []ledger.Tried{{Account: work, Why: status.WhyLimit}}),
	)
	putDay(t, lines, yesterday, false,
		line(yesterday, "r4", personal, status.ReasonSticky, "", nil),
		line(yesterday, "r5", work, status.ReasonPinYields+personal+" "+status.WhyReserve, "", nil),
	)
	putDay(t, lines, today, false,
		line(today, "r6", work, status.Compacted(work, 0.35), personal, nil),
		line(today, "r7", "", status.ReasonNoRoom, "", nil),
	)
	ledger.Open(ledger.Dir(f.state), dayfile.Forever, clock, ledger.HistoryIn(read), ledger.Caps{Shared: []string{"five_hour"}}, logs.For("test")).SummariseEnded(now)
	writeFile(t, filepath.Join(ledger.Dir(f.state), "digest-"+longAgo+".json.gz"), "built of the old ids")

	evts := events.Files(ledger.Dir(f.state), logs.For("test"))
	putDay(t, evts, longAgo, true,
		event(longAgo, 1, status.Event{Kind: status.EventLimit, Account: work}),
		event(longAgo, 2, status.Event{Kind: status.EventMoved, From: work, To: personal, Reason: status.ReasonMovedOff + work + " " + status.WhyLimit}),
	)
	putDay(t, evts, today, false, event(today, 1, status.Event{Kind: status.EventPin, Account: work, Accounts: []string{work, personal}}))

	writeFile(t, filepath.Join(f.state, "state.json"), `{
  "version": 1,
  "pin": {"accounts": ["work"], "since": "2026-11-08T09:00:00Z", "move": false},
  "sessions": [{"session": "s1", "model": "claude-opus-5-5", "account": "personal", "pin": "work", "reason": "moved: work hit its limit", "assigned_at": "2026-11-08T09:00:00Z", "last_seen": "2026-11-08T11:00:00Z"}],
  "session_pins": {"s1": {"account": "work", "since": "2026-11-08T09:00:00Z"}},
  "tokens": {"work": {"sha256": "aa", "former": [{"sha256": "cc", "replaced_at": "2026-11-07T09:00:00Z", "from": "old"}]}, "personal": {"sha256": "bb"}},
  "readings": {"work": {"read_at": "2026-11-08T11:00:00Z"}, "old": {"read_at": "2026-11-03T11:00:00Z"}},
  "token_years": {"work": {"sha256": "aa", "within": 30}}
}
`)
}

// readingsFiles are the readings history's files in the state directory.
func readingsFiles(state string) *dayfile.Files {
	return readings.Files(readings.Dir(state), logs.For("test"))
}

// at is noon on the local day with the given date.
func at(date string) time.Time {
	start, _, _ := dayfile.Day(date)
	return start.Add(12 * time.Hour)
}

// reading is a line of the readings history of the account's 5-hour window
// on the day with the given date.
func reading(date, account string, use float64) string {
	return jsonOf(readings.Reading{At: at(date), Account: account, Key: "five_hour", Utilization: use, Source: readings.FromAnswer})
}

// line is a line of the request ledger of a request answered on the day with
// the given date.
func line(date, request, account, reason, from string, tried []ledger.Tried) string {
	status := 200
	if account == "" {
		status = 529
	}
	return jsonOf(ledger.Line{At: at(date), Request: request, Kind: ledger.KindMessage, Session: "s-" + request, Model: "claude-opus-5-5",
		Account: account, Reason: reason, From: from, Tried: tried, Status: status, Attempts: 1})
}

// event is a line of the router's events, told on the day with the given
// date.
func event(date string, id int, e status.Event) string {
	e.ID, e.At = id, at(date)
	data, err := events.Marshal(events.Line{Event: e, Run: at(date)})
	if err != nil {
		panic(err)
	}
	return string(data)
}

// jsonOf is v as JSON.
func jsonOf(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}

// putDay writes the lines given as the files of the day with the given date:
// compressed, or plain.
func putDay(t *testing.T, files *dayfile.Files, date string, compressed bool, lines ...string) {
	t.Helper()
	text := strings.Join(lines, "\n") + "\n"
	if !compressed {
		writeFile(t, filepath.Join(files.Dir, files.Prefix+"-"+date+".jsonl"), text)
		return
	}
	var data bytes.Buffer
	w := gzip.NewWriter(&data)
	if _, err := w.Write([]byte(text)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	writeFile(t, files.CompressedPath(date), data.String())
}

// writeFile writes content to a file at path, the user's alone, making its
// directory.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// organizations answers each token's organization, as the API does, unless
// told otherwise: it fails as though the API didn't answer as many times as
// unanswered says, whatever the token, and always for a token silent holds;
// for one it's to refuse, it fails as the API refusing it does, as many
// times as refused says; and one it has no organization of, it answers
// naming none. It counts the times it's asked, by token too.
type organizations struct {
	mu         sync.Mutex
	of         map[string]string
	unanswered int
	silent     map[string]bool
	refused    map[string]int
	asked      int
	askedFor   map[string]int
}

// always is as many times as a test could ask.
const always = 1 << 30

// answering are the organizations of the fixture's tokens, answered.
func answering() *organizations {
	return &organizations{of: map[string]string{workToken: workOrganization, personalToken: personalOrganization}}
}

func (o *organizations) Organization(ctx context.Context, token tokens.Token) (string, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.asked++
	if o.askedFor == nil {
		o.askedFor = make(map[string]int)
	}
	o.askedFor[token.Reveal()]++
	switch {
	case o.unanswered > 0:
		o.unanswered--
		return "", errors.New("timed out after 5s")
	case o.silent[token.Reveal()]:
		return "", errors.New("timed out after 5s")
	case o.refused[token.Reveal()] > 0:
		o.refused[token.Reveal()]--
		return "", refusedError{}
	}
	return o.of[token.Reveal()], nil
}

// refusedError is the API refusing a token, as a probe's failure says.
type refusedError struct{}

func (refusedError) Error() string { return "401: invalid token" }
func (refusedError) Refused() bool { return true }

// mover is the move of the fixture's install, asking o for its tokens'
// organizations, by the fixture's clock, drawing random ids from a source
// that draws the same each time, and asking again, and looking again at a
// token, at once.
func (f fixture) mover(o idmove.Organizations) *idmove.Mover {
	return &idmove.Mover{ConfigPath: f.config, StateDir: f.state, Tokens: f.tokens, Organizations: o,
		Now: clock, Random: rand.NewChaCha8([32]byte{7}), Retry: time.Millisecond, Again: time.Millisecond}
}

// moved runs the move of the fixture's install to its end, failing the test
// where it fails.
func (f fixture) moved(t *testing.T, o idmove.Organizations) {
	t.Helper()
	if err := f.mover(o).Run(t.Context()); err != nil {
		t.Fatalf("Run() = %v", err)
	}
}

// record is the fixture's accounts record.
func (f fixture) record(t *testing.T) identity.Record {
	t.Helper()
	r, err := identity.NewFile(f.state).Read()
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// renamer renames ids as the fixture's accounts record has moved them.
func (f fixture) renamer(t *testing.T) func(string) string {
	t.Helper()
	record := f.record(t)
	return func(id string) string {
		if e, ok := record.ByWas(id); ok {
			return e.ID
		}
		return id
	}
}

// snapshot is every file of the fixture's install, by its path, as it holds
// it: a compressed file's lines, a link where it leads; but for the
// accounts record's lock, which holds nothing.
func (f fixture) snapshot(t *testing.T) map[string]string {
	t.Helper()
	files := map[string]string{"config": readFile(t, f.config)}
	err := filepath.WalkDir(f.state, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || d.Name() == "accounts.lock" {
			return err
		}
		rel, _ := filepath.Rel(f.state, path)
		if d.Type()&fs.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			files[rel] = "-> " + target
			return err
		}
		files[rel] = readFile(t, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// readFile returns what the file at path holds: through gzip, where it's
// compressed.
func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, ".gz") {
		return string(data)
	}
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return string(data)
	}
	text, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return string(text)
}
