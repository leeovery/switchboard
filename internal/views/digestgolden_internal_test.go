package views

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/ledger"
)

// update has the golden of the digests' encoding written, where none is
// kept of the versions the views and the ledger write, as -update asks.
var update = flag.Bool("update", false, "write the golden of a day's digest, where none is kept of the versions written")

// goldenDay is the day of the golden world whose digest the golden holds.
const goldenDay = "2026-10-08"

// goldenWorld is a fixed world of two days, written out in full so no
// change to a generator moves it, so the golden holds a value of every key
// of a digest: a session s, its own requests, a subagent's on both days, one
// moved off an account that hit its limit, with the accounts it tried,
// minutes after another of its own, settling its move as it writes the
// session's context to the cache again, while it would still be held there,
// and one running across midnight; and another session, o, beside it: one
// of its requests without an id; one moved, settling its move while the
// session's context is held, its answer giving no usage; and its last that
// gave usage a subagent's, in a directory of its own, stopping to call a
// tool, arriving and ending within a second, its usage writing to the cache
// for five minutes and beyond what it breaks down, searching the web, and
// counting its input under a name in capitals no price knows. And zoned,
// the requests of a session z that a change of time zone filed under the
// second day, beside the days they arrived on: one the evening before, one
// past its midnight. Its times are those of the zone the test sets.
func goldenWorld() (lines, zoned []ledger.Line) {
	at := func(day, hour, minute int) time.Time {
		return time.Date(2026, 10, day, hour, minute, 0, 0, time.Local).UTC()
	}
	usage := func(write, read int) json.RawMessage {
		return fmt.Appendf(nil, `{"input_tokens":10,"cache_creation_input_tokens":%d,"cache_read_input_tokens":%d,`+
			`"cache_creation":{"ephemeral_5m_input_tokens":0,"ephemeral_1h_input_tokens":%[1]d},"output_tokens":300}`, write, read)
	}
	line := func(request, session string, at time.Time, model, account, reason string) ledger.Line {
		return ledger.Line{At: at, Request: request, Kind: ledger.KindMessage, Session: session, Model: model, Account: account, Reason: reason,
			Status: 200, Attempts: 1, TotalMS: 4000, Usage: usage(0, 90000)}
	}
	s1 := line("r1", "s", at(7, 10, 0), foldOpus, "work", "new")
	s1.Dir, s1.Class, s1.Prompt, s1.Usage = "/code/app", ledger.ClassMain, "p1", usage(3000, 0)
	s1.Answer.Stop, s1.Answer.Tools = "tool_use", []string{"Read", "Edit"}
	s2 := line("r2", "s", at(7, 10, 5), foldHaiku, "work", "new")
	s2.Class, s2.AgentID, s2.Prompt = "subagent", "a1", "p1"
	o1 := line("o1", "o", at(7, 11, 0), foldSonnet, "side", "new")
	o1.Class, o1.Shape.InferenceGeo = ledger.ClassMain, "us"
	o3 := line("o3", "o", at(7, 11, 30), foldSonnet, "work", "moved: side hit its limit")
	o3.From, o3.Tried, o3.Class, o3.Usage = "side", []ledger.Tried{{Account: "side", Why: "hit its limit"}}, ledger.ClassMain, nil
	s3 := line("r3", "s", at(8, 9, 0), foldHaiku, "work", "sticky")
	s3.Class, s3.AgentID, s3.Prompt = "subagent", "a1", "p2"
	warm := line("r3w", "s", at(8, 9, 10), foldOpus, "work", "sticky")
	warm.Class, warm.Prompt = ledger.ClassMain, "p2"
	s4 := line("r4", "s", at(8, 9, 30), foldOpus, "side", "moved: work hit its limit")
	s4.From, s4.Tried, s4.Class, s4.Prompt, s4.Usage = "work", []ledger.Tried{{Account: "work", Why: "hit its limit"}}, ledger.ClassMain, "p2", usage(95000, 0)
	s5 := line("r5", "s", at(8, 9, 31), foldOpus, "side", "sticky")
	s5.Class, s5.Prompt = ledger.ClassMain, "p2"
	s6 := line("r6", "s", at(8, 23, 50), foldOpus, "side", "sticky")
	s6.Class, s6.Prompt, s6.TotalMS = ledger.ClassMain, "p3", (20 * time.Minute).Milliseconds()
	o2 := line("", "o", at(8, 12, 0), foldSonnet, "side", "sticky")
	o4 := line("o4", "o", at(8, 13, 0).Add(250*time.Millisecond), foldHaiku, "work", "sticky")
	o4.Class, o4.AgentID, o4.Dir, o4.TotalMS = "subagent", "a2", "/code/site", 1500
	o4.Answer.Stop, o4.Answer.Tools = "tool_use", []string{"Bash"}
	o4.Usage = json.RawMessage(`{"Input_Tokens":7,"cache_creation_input_tokens":1200,"cache_read_input_tokens":500,` +
		`"cache_creation":{"ephemeral_5m_input_tokens":1000},"output_tokens":120,"server_tool_use":{"web_search_requests":2}}`)
	early := line("z1", "z", at(7, 23, 40), foldSonnet, "side", "new")
	late := line("z2", "z", at(9, 0, 20), foldSonnet, "side", "sticky")
	return []ledger.Line{s1, s2, o1, o3, s3, warm, s4, s5, o2, o4, s6}, []ledger.Line{early, late}
}

// The digest of a day of the golden world, its file's JSON as the ledger
// writes it, inflated and indented, is what the golden of the versions it's
// written in holds: testdata/digest-v<DigestVersion>.json. A change to how a
// session's state is stored, or to the working behind it, or to the ledger's
// envelope, that changes it without a new version fails, as digests already
// written would be read as they no longer are. A new version has its golden
// written with -update, the ones before kept; one already kept of the
// envelope written isn't written again, and -update fails where it differs. Its world's times are UTC's, so it
// encodes alike in every zone the tests run in.
func TestADigestIsEncodedAsTheGoldenOfItsVersionsHoldsIt(t *testing.T) {
	zone := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = zone })
	got := goldenDigest(t)
	path := filepath.Join("testdata", fmt.Sprintf("digest-v%d.json", DigestVersion))
	kept, err := os.ReadFile(path)
	missing := errors.Is(err, fs.ErrNotExist)
	if err != nil && !missing {
		t.Fatal(err)
	}
	const write = "go test ./internal/views -run TestADigestIsEncodedAsTheGoldenOfItsVersionsHoldsIt -update"
	switch {
	case bytes.Equal(got, kept):
	case *update && (missing || envelopeOf(t, kept) != envelopeOf(t, got)):
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
	case *update:
		t.Fatalf("%s holds a digest of version %d of the envelope written: bump DigestVersion, or the ledger's digestVersion, rather than write it again", path, DigestVersion)
	case missing:
		t.Fatalf("no golden of a digest of version %d is kept at %s: write it with %s", DigestVersion, path, write)
	default:
		t.Errorf("the digest of %s is written\n%s\nwant, as %s holds it\n%s\n"+
			"bump DigestVersion where how a session's state is stored, or the working behind it, changed, or the ledger's digestVersion where its envelope did, "+
			"then write the new version's golden with %s", goldenDay, got, path, kept, write)
	}
}

// goldenDigest is the digest of goldenDay of the golden world, written as a
// real ledger and digested day by day, as digestChain digests it: its file's
// JSON, inflated and indented, ending its last line.
func goldenDigest(t *testing.T) []byte {
	t.Helper()
	lines, zoned := goldenWorld()
	c := newDigestChain(t, "the golden world", 0, lines)
	c.file(goldenDay, zoned...)
	c.digestDayByDay(func(LedgerDay) {})
	data, err := os.ReadFile(c.digestPath(goldenDay))
	if err != nil {
		t.Fatal(err)
	}
	var indented bytes.Buffer
	if err := json.Indent(&indented, gunzipped(t, data), "", "  "); err != nil {
		t.Fatal(err)
	}
	return append(indented.Bytes(), '\n')
}

// envelopeOf is the version of the ledger's envelope a digest's JSON names.
func envelopeOf(t *testing.T, digest []byte) int {
	t.Helper()
	var head struct {
		Version int `json:"version"`
	}
	if err := json.Unmarshal(digest, &head); err != nil {
		t.Fatal(err)
	}
	return head.Version
}
