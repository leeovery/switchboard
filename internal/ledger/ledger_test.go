package ledger_test

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/logs/logstest"
)

// now is the time by the clock in tests: a Tuesday, 13:12 UTC.
var now = time.Date(2026, 10, 6, 13, 12, 0, 0, time.UTC)

// tokenShaped is shaped like a Claude token, though it's none.
const tokenShaped = "sk-ant-oat01-fake_token-shaped"

func TestALineIsWrittenAsJSON(t *testing.T) {
	// arrived is when the request arrived, in a time zone of its own, to the
	// nanosecond.
	arrived := time.Date(2026, 10, 6, 14, 12, 0, 123456789, time.FixedZone("UTC+1", 3600))
	tests := []struct {
		name string
		line ledger.Line
		want string
	}{
		{
			name: "a request moved by a limit, answered on the account it went to",
			line: ledger.Line{At: arrived, Request: "3f2a91c4", Kind: ledger.KindMessage, Session: "5b0e7c1a-1f2a-4b3c-9d8e-7f6a5b4c3d2e",
				Model: "claude-opus-5-5", Account: "side", Reason: "moved: work hit its limit", From: "work",
				Tried: []ledger.Tried{{Account: "work", Why: "hit its limit"}}, Status: 200, Attempts: 2, FirstMS: ms(812), TotalMS: 14230,
				Agent: "claude-cli/2.1.0 (external, cli)", Betas: []string{"oauth-2025-04-20", "context-1m-2025-08-07"}},
			want: `{"at":"2026-10-06T13:12:00.123Z","request":"3f2a91c4","kind":"message","session":"5b0e7c1a-1f2a-4b3c-9d8e-7f6a5b4c3d2e",` +
				`"model":"claude-opus-5-5","account":"side","reason":"moved: work hit its limit","from":"work",` +
				`"tried":[{"account":"work","why":"hit its limit"}],"status":200,"attempts":2,"first_ms":812,"total_ms":14230,` +
				`"agent":"claude-cli/2.1.0 (external, cli)","betas":["oauth-2025-04-20","context-1m-2025-08-07"]}`,
		},
		{
			name: "a request the router answered itself, of no session, its client saying nothing of itself",
			line: ledger.Line{At: arrived, Request: "3f2a91c5", Kind: ledger.KindMessage, Reason: "no account has room", Status: 429, TotalMS: 3},
			want: `{"at":"2026-10-06T13:12:00.123Z","request":"3f2a91c5","kind":"message","reason":"no account has room","status":429,"attempts":0,"total_ms":3}`,
		},
		{
			name: "a quota check whose client went before an answer came",
			line: ledger.Line{At: arrived, Request: "3f2a91c6", Kind: ledger.KindCheck, Session: "check", Model: "claude-opus-5-5", Account: "work",
				Reason: "new", Canceled: true, Attempts: 1, TotalMS: 2000},
			want: `{"at":"2026-10-06T13:12:00.123Z","request":"3f2a91c6","kind":"check","session":"check","model":"claude-opus-5-5","account":"work",` +
				`"reason":"new","status":0,"canceled":true,"attempts":1,"total_ms":2000}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()

			write(t, dir, &tt.line)
			if got := readDay(t, dir, arrived); got != tt.want+"\n" {
				t.Errorf("the ledger holds\n%s\nwant\n%s", got, tt.want)
			}
		})
	}
}

func TestTheLedgerFilesALineUnderTheLocalDayItsRequestArrivedOn(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ledger")
	// A directory others can read, which the ledger makes private.
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	yesterday := now.AddDate(0, 0, -1)

	write(t, dir, &ledger.Line{At: yesterday, Request: "1", Kind: ledger.KindMessage}, &ledger.Line{At: now, Request: "2", Kind: ledger.KindMessage},
		&ledger.Line{At: now, Request: "3", Kind: ledger.KindCount})
	for at, want := range map[time.Time][]string{yesterday: {`"request":"1"`}, now: {`"request":"2"`, `"request":"3"`}} {
		got := strings.Split(strings.TrimSuffix(readDay(t, dir, at), "\n"), "\n")
		if len(got) != len(want) {
			t.Fatalf("the file of %s holds %q, want a line for each of %q", at.Local().Format(time.DateOnly), got, want)
		}
		for i := range want {
			if !strings.Contains(got[i], want[i]) {
				t.Errorf("line %d of %s is %s, want the request %s's", i+1, at.Local().Format(time.DateOnly), got[i], want[i])
			}
		}
	}
	for path, want := range map[string]fs.FileMode{dir: fs.ModeDir | 0o700, dayFile(dir, now): 0o600, dayFile(dir, yesterday): 0o600} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode() != want {
			t.Errorf("%s is %v, want %v", filepath.Base(path), info.Mode(), want)
		}
	}
}

func TestALineHoldsNothingShapedLikeAToken(t *testing.T) {
	dir := t.TempDir()

	write(t, dir, &ledger.Line{At: now, Request: "1", Kind: ledger.KindMessage, Session: tokenShaped, Model: "model-" + tokenShaped,
		Reason: "pinned " + tokenShaped, Tried: []ledger.Tried{{Account: "work", Why: tokenShaped}}, Agent: tokenShaped + " (cli)",
		Betas: []string{tokenShaped, "oauth-2025-04-20"}})
	got := readDay(t, dir, now)
	want := `{"at":"2026-10-06T13:12:00Z","request":"1","kind":"message","session":"[redacted]","model":"model-[redacted]",` +
		`"reason":"pinned [redacted]","tried":[{"account":"work","why":"[redacted]"}],"status":0,"attempts":0,"total_ms":0,` +
		`"agent":"[redacted] (cli)","betas":["[redacted]","oauth-2025-04-20"]}` + "\n"
	if got != want {
		t.Errorf("the ledger holds\n%s\nwant\n%s", got, want)
	}
}

func TestALineThatCantBeWrittenIsLoggedOnce(t *testing.T) {
	log := logstest.Capture(t)
	dir := t.TempDir()
	// A time past the year 9999 can't be put as JSON.
	unwritable := now.AddDate(8000, 0, 0)

	write(t, dir, &ledger.Line{At: unwritable, Request: "1"}, &ledger.Line{At: unwritable, Request: "2"}, &ledger.Line{At: now, Request: "3"})
	if got := readDay(t, dir, now); !strings.Contains(got, `"request":"3"`) || strings.Count(got, "\n") != 1 {
		t.Errorf("the ledger holds\n%s\nwant the line that could be written, alone", got)
	}
	if n := strings.Count(log.String(), `msg="request ledger can't hold a line; it goes unwritten"`); n != 1 || !log.Has("level=WARN", "request=1") {
		t.Errorf("log reads\n%s\nwant the first line that couldn't be written warned of, once", log)
	}
}

// write has a ledger kept in dir, by now's clock, note lines, and returns once
// it has written every one it can.
func write(t *testing.T, dir string, lines ...*ledger.Line) {
	t.Helper()
	l := ledger.Open(dir, 90*24*time.Hour, func() time.Time { return now }, logs.For("router"))
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		l.Run(ctx)
		close(done)
	}()
	for _, line := range lines {
		l.Note(line)
	}
	cancel()
	<-done
}

// dayFile is the ledger's plain file in dir of the local day at falls on.
func dayFile(dir string, at time.Time) string {
	return filepath.Join(dir, "requests-"+at.Local().Format(time.DateOnly)+".jsonl")
}

// readDay returns what the ledger's plain file in dir of the local day at
// falls on holds.
func readDay(t *testing.T, dir string, at time.Time) string {
	t.Helper()
	data, err := os.ReadFile(dayFile(dir, at))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// ms is n milliseconds, as a line gives them where they may not be given.
func ms(n int64) *int64 {
	return new(n)
}
