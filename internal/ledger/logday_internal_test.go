package ledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
)

// sideVersion is the version of the days' sides the day heads' tests keep,
// as their builder would give it.
const sideVersion = 7

// dayHeadOf is the head of the local day with the given date, of a side of
// sideVersion, as it was built against a summary of as many lines as given,
// every one of them given by the day's read, of the side given: none of "".
func dayHeadOf(date string, lines int, side string) DayHead {
	start, end, _ := dayfile.Day(date)
	head := DayHead{Body: sideVersion, Day: date, Start: start, End: end, SummaryLines: lines, Lines: lines}
	if side != "" {
		head.Side = json.RawMessage(side)
	}
	return head
}

// chunksGiven are the chunks of a day's rows the day heads' tests write: two,
// the first's meta holding characters JSON may escape for HTML, and the
// second of no meta.
func chunksGiven() []BuiltChunk {
	return []BuiltChunk{
		{Rows: []byte("r1\nr2\n"), Count: 2, Meta: json.RawMessage(`{"sessions":{"<one>":2}}`)},
		{Rows: []byte("r3\n"), Count: 1},
	}
}

// spacedChunks are chunksGiven, the first's meta given with spaces.
func spacedChunks() []BuiltChunk {
	chunks := chunksGiven()
	chunks[0].Meta = json.RawMessage(`{ "sessions": {"<one>": 2} }`)
	return chunks
}

// A day head reads back as it was written, every field of it, its side byte
// for byte but for its insignificant spaces, its characters unescaped, and
// its rows as written beside it, each chunk's meta as its side is; and one
// of no side, or no rows, reads back of none.
func TestADayHeadReadsBackAsItWasWritten(t *testing.T) {
	every := dayHeadOf(digested, 42, `[{"id":"one","dir":"~/Code/<api> & co"},{"id":"two","named":"café"}]`)
	every.Lines, every.RestsOn = 41, map[string]int{before: untold, after: 30}
	tests := []struct {
		name   string
		head   DayHead
		chunks []BuiltChunk
	}{
		{name: "every field held", head: every, chunks: chunksGiven()},
		{name: "a side of no sessions", head: dayHeadOf(digested, 0, `[]`), chunks: chunksGiven()},
		{name: "no side", head: dayHeadOf(digested, 3, ""), chunks: chunksGiven()},
		{name: "no rows", head: every},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := digestDays(t)
			written, err := d.writeDay(tt.head, tt.chunks)
			if err != nil {
				t.Fatalf("writeDay(%+v): %v", tt.head, err)
			}

			read, err := d.dayHead(digested, sideVersion)

			if err != nil || !reflect.DeepEqual(read, written) {
				t.Errorf("dayHead(%q) = %+v, %v; want %+v, as it was written", digested, read, err, written)
			}
			if len(read.Rows.Chunks) != len(tt.chunks) {
				t.Fatalf("dayHead(%q) points at %d chunks, want %d, as many as were written", digested, len(read.Rows.Chunks), len(tt.chunks))
			}
			for i, c := range read.Rows.Chunks {
				rows, err := memberIn(rowsFile(d.files.Dir, digested), c.Member)
				if want := tt.chunks[i]; err != nil || !bytes.Equal(rows, want.Rows) || c.Count != want.Count {
					t.Errorf("chunk %d of %s holds %q, %v, of %d rows; want %q, of %d, as written", i, digested, rows, err, c.Count, want.Rows, want.Count)
				}
			}
		})
	}
}

// A day head's side is stored as it was given but for its insignificant
// spaces, as a JSON value of its builder's; and so is each chunk's meta.
func TestADayHeadsSideIsStoredAsItWasGiven(t *testing.T) {
	d := digestDays(t)
	if _, err := d.writeDay(dayHeadOf(digested, 1, `{ "sessions": [ "<one>" ] }`), spacedChunks()); err != nil {
		t.Fatal(err)
	}

	read, err := d.dayHead(digested, sideVersion)

	if want := `{"sessions":["<one>"]}`; err != nil || string(read.Side) != want {
		t.Errorf("dayHead(%q) has its side read as %s, %v; want %s, as it was given, its spaces left out", digested, read.Side, err, want)
	}
	if want := `{"sessions":{"<one>":2}}`; len(read.Rows.Chunks) == 0 || string(read.Rows.Chunks[0].Meta) != want {
		t.Errorf("dayHead(%q) has its rows read as %+v; want its first chunk's meta %s, as it was given, its spaces left out", digested, read.Rows, want)
	}
}

// A day head's fields but its side and its rows' chunks are read by stopping
// at the first of those: so they're its stored form's last fields, written
// after every other.
func TestADayHeadsSideAndChunksAreItsStoredFormsLastFields(t *testing.T) {
	fields := reflect.VisibleFields(reflect.TypeFor[storedDayHead]())
	var last []string
	for _, f := range fields[len(fields)-2:] {
		name, _, _ := strings.Cut(f.Tag.Get("json"), ",")
		last = append(last, name)
	}
	if want := []string{logdaySide, logdayChunks}; !slices.Equal(last, want) {
		t.Errorf("storedDayHead's last fields are stored as %q, want %q: its other fields come first, as dayHeadIn reads them", last, want)
	}
}

// A day head's file is its stored form, gzipped, the user's alone: its keys
// short, its zeros left out, its day's bounds as nanoseconds since the Unix
// epoch, the days it rests on, its rows file's size and how many rows its
// chunks hold in all; then its side, and its rows' chunks, each where it
// lies in the rows file, how many rows it holds, and its meta, as they were
// given but for their insignificant spaces, their characters unescaped. Its
// rows file is the user's alone too.
func TestADayHeadIsWrittenGzippedAsItsStoredFormTheUsersAlone(t *testing.T) {
	d := digestDays(t)
	head := DayHead{Body: sideVersion, Day: digested, Start: time.Date(2026, 10, 4, 23, 0, 0, 0, time.UTC),
		End: time.Date(2026, 10, 5, 23, 0, 0, 0, time.UTC), SummaryLines: 40, Lines: 39, RestsOn: map[string]int{after: 30, before: untold},
		Side: json.RawMessage(`[ {"dir": "~/<api> & co"} ]`)}
	written, err := d.writeDay(head, spacedChunks())
	if err != nil {
		t.Fatal(err)
	}

	path := logdayFile(d.files.Dir, digested)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	c := written.Rows.Chunks
	want := fmt.Sprintf(`{"version":%d,"body":7,"day":"2026-10-05","from":1791154800000000000,"to":1791241200000000000,"summary_lines":40,"lines":39,`+
		`"rests_on":{"2026-10-04":-1,"2026-10-06":30},"rows_size":%d,"rows":3,"side":[{"dir":"~/<api> & co"}],`+
		`"chunks":[{"length":%d,"crc":%d,"count":2,"meta":{"sessions":{"<one>":2}}},{"offset":%d,"length":%d,"crc":%d,"count":1}]}`+"\n",
		logdayVersion, written.Rows.Size, c[0].Length, c[0].CRC, c[1].Offset, c[1].Length, c[1].CRC)
	if got := string(gunzipped(t, data)); got != want {
		t.Errorf("the day head's file holds\n%s\nwant\n%s", got, want)
	}
	for _, path := range []string{path, rowsFile(d.files.Dir, digested)} {
		if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
			t.Errorf("%s is %v, %v; want it the user's alone, 0600", filepath.Base(path), info.Mode().Perm(), err)
		}
	}
}

// A day head that can't be written whole, of a day that isn't one, of a side
// that isn't JSON, or of a chunk whose meta isn't, isn't written at all, nor
// its rows, failing with an error that names the day.
func TestADayHeadThatCantBeWrittenWholeIsntWritten(t *testing.T) {
	unmeant := chunksGiven()
	unmeant[1].Meta = json.RawMessage(`{"first":"r`)
	tests := []struct {
		name   string
		head   DayHead
		chunks []BuiltChunk
	}{
		{name: "of a day that isn't one", head: DayHead{Body: sideVersion, Day: "someday"}, chunks: chunksGiven()},
		{name: "a side that isn't JSON", head: dayHeadOf(digested, 1, `{"dir":"~/a`), chunks: chunksGiven()},
		{name: "a chunk's meta that isn't JSON", head: dayHeadOf(digested, 1, `[]`), chunks: unmeant},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := digestDays(t)

			_, err := d.writeDay(tt.head, tt.chunks)

			entries, _ := os.ReadDir(d.files.Dir)
			if err == nil || len(entries) > 0 {
				t.Fatalf("writeDay(%+v) = %v, leaving %d files; want it failing, writing none", tt.head, err, len(entries))
			}
			if !strings.Contains(err.Error(), tt.head.Day) {
				t.Errorf("writeDay(%+v) failed with %q, want it naming the day, %s", tt.head, err, tt.head.Day)
			}
		})
	}
}

// A day head held that doesn't read as the day's head, or reads as one of
// another version, fails as its reader can tell from there being none.
func TestADayHeadThatDoesntReadAsTheDaysFailsSoItsReaderCanTell(t *testing.T) {
	stored := dayHeadOf(digested, 40, `[{"id":"one"}]`).stored()
	file := func(s storedDayHead) []byte {
		data, err := gzippedJSON(s)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	whole := file(stored)
	envelope := stored
	envelope.Version = logdayVersion + 1
	damaged := bytes.Clone(whole)
	damaged[len(damaged)/2] ^= 0xff
	tests := []struct {
		name string
		// file is what the ledger holds as the day's head: nil for none.
		file []byte
		body int
		want error
	}{
		{name: "none held", file: nil, body: sideVersion, want: fs.ErrNotExist},
		{name: "of another version of the envelope", file: file(envelope), body: sideVersion, want: errDayHeadOutdated},
		{name: "of another version of the side", file: whole, body: sideVersion + 1, want: errDayHeadOutdated},
		{name: "another day's", file: file(dayHeadOf(before, 7, `[]`).stored()), body: sideVersion, want: errNotTheDaysHead},
		{name: "cut short within its JSON", file: whole[:len(whole)/2], body: sideVersion, want: errNotTheDaysHead},
		{name: "cut short past its JSON, in its gzip trailer", file: whole[:len(whole)-4], body: sideVersion, want: errNotTheDaysHead},
		{name: "damaged", file: damaged, body: sideVersion, want: errNotTheDaysHead},
		{name: "not gzipped", file: gunzipped(t, whole), body: sideVersion, want: errNotTheDaysHead},
		{name: "gzipped, but not JSON", file: gzippedText(t, "not a head"), body: sideVersion, want: errNotTheDaysHead},
		{name: "gzipped JSON, but not an object", file: gzippedText(t, `["2026-10-05"]`), body: sideVersion, want: errNotTheDaysHead},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := digestDays(t)
			if tt.file != nil {
				if err := os.WriteFile(logdayFile(d.files.Dir, digested), tt.file, 0o600); err != nil {
					t.Fatal(err)
				}
			}

			_, err := d.dayHead(digested, tt.body)

			if !errors.Is(err, tt.want) {
				t.Errorf("dayHead(%q, %d) failed with %v, want %v", digested, tt.body, err, tt.want)
			}
			if !errors.Is(tt.want, fs.ErrNotExist) && errors.Is(err, fs.ErrNotExist) {
				t.Errorf("dayHead(%q, %d) failed with %v, as though none were held", digested, tt.body, err)
			}
		})
	}
}

// A day head is judged by its other fields before its side is read: one of
// another version, of the envelope or of the side, as every one is after a
// release raising either, fails as outdated whatever its side holds, as it
// isn't read; and one of the version asked for then has it read, failing
// where it doesn't.
func TestADayHeadIsJudgedBeforeItsSideIsRead(t *testing.T) {
	headed := func(version, body int) string {
		return fmt.Sprintf(`{"version":%d,"body":%d,"day":"2026-10-05","from":1,"to":2,"summary_lines":40,"lines":40,`, version, body)
	}
	tests := []struct {
		name string
		head string
		want error
	}{
		{name: "of another version of the envelope", head: headed(logdayVersion+1, sideVersion), want: errDayHeadOutdated},
		{name: "of another version of the side", head: headed(logdayVersion, sideVersion+1), want: errDayHeadOutdated},
		{name: "of the version asked for", head: headed(logdayVersion, sideVersion), want: errNotTheDaysHead},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := digestDays(t)
			if err := os.WriteFile(logdayFile(d.files.Dir, digested), gzippedText(t, tt.head+`"side":not a side`), 0o600); err != nil {
				t.Fatal(err)
			}

			_, err := d.dayHead(digested, sideVersion)

			if !errors.Is(err, tt.want) {
				t.Errorf("dayHead(%q, %d) of a side that doesn't read failed with %v, want %v", digested, sideVersion, err, tt.want)
			}
		})
	}
}

// A day head's fields but its side and its rows' chunks are read without
// either, which no damage to them or its gzip trailer fails, as the whole
// read does, as none of them is read: so judging a head inflates none of its
// side, nor of its chunks, whichever comes first.
func TestADayHeadsFieldsAreReadWithoutItsSide(t *testing.T) {
	fields := fmt.Sprintf(`{"version":%d,"body":%d,"day":"2026-10-05","from":1,"to":2,"summary_lines":40,"lines":39,"rests_on":{"2026-10-04":9},"rows_size":90,`,
		logdayVersion, sideVersion)
	whole := gzippedText(t, fields+`"side":[{"id":"one"}],"chunks":[{"length":90,"crc":7,"count":3}]}`)
	tests := []struct {
		name string
		file []byte
	}{
		{name: "its side cut short", file: gzippedText(t, fields+`"side":[{"id":"o`)},
		{name: "its side not JSON", file: gzippedText(t, fields+`"side":not a side`)},
		{name: "its chunks not JSON", file: gzippedText(t, fields+`"side":[],"chunks":not chunks`)},
		{name: "its chunks not JSON, of no side", file: gzippedText(t, fields+`"chunks":not chunks`)},
		{name: "cut short in its gzip trailer", file: whole[:len(whole)-4]},
	}
	want := DayHead{Body: sideVersion, Day: digested, Start: time.Unix(0, 1), End: time.Unix(0, 2), SummaryLines: 40, Lines: 39,
		RestsOn: map[string]int{before: 9}, Rows: DayRows{Size: 90}}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := digestDays(t)
			if err := os.WriteFile(logdayFile(d.files.Dir, digested), tt.file, 0o600); err != nil {
				t.Fatal(err)
			}

			got, err := d.sidelessHead(digested, sideVersion)

			if err != nil || !reflect.DeepEqual(got, want) {
				t.Errorf("sidelessHead(%q) = %+v, %v; want %+v, its side unread", digested, got, err, want)
			}
			if _, err := d.dayHead(digested, sideVersion); !errors.Is(err, errNotTheDaysHead) {
				t.Errorf("dayHead(%q) failed with %v, want %v: the whole read reads its side", digested, err, errNotTheDaysHead)
			}
		})
	}
}

// A day head's fields read without its side fail as the whole read fails,
// where they don't read as the day's head's, or read as one of another
// version.
func TestADayHeadsFieldsThatDontReadFailAsTheWholeReadDoes(t *testing.T) {
	stored := dayHeadOf(digested, 40, `[]`).stored()
	envelope := stored
	envelope.Version = logdayVersion + 1
	file := func(s storedDayHead) []byte {
		data, err := gzippedJSON(s)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	tests := []struct {
		name string
		// file is what the ledger holds as the day's head: nil for none.
		file []byte
		body int
		want error
	}{
		{name: "none held", file: nil, body: sideVersion, want: fs.ErrNotExist},
		{name: "of another version of the envelope", file: file(envelope), body: sideVersion, want: errDayHeadOutdated},
		{name: "of another version of the side", file: file(stored), body: sideVersion + 1, want: errDayHeadOutdated},
		{name: "another day's", file: file(dayHeadOf(before, 7, `[]`).stored()), body: sideVersion, want: errNotTheDaysHead},
		{name: "cut short before its side", file: gzippedText(t, `{"version":1,"body":7,"day":"2026-10-05","from":`), body: sideVersion, want: errNotTheDaysHead},
		{name: "not gzipped", file: gunzipped(t, file(stored)), body: sideVersion, want: errNotTheDaysHead},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := digestDays(t)
			if tt.file != nil {
				if err := os.WriteFile(logdayFile(d.files.Dir, digested), tt.file, 0o600); err != nil {
					t.Fatal(err)
				}
			}

			_, err := d.sidelessHead(digested, tt.body)

			if !errors.Is(err, tt.want) {
				t.Errorf("sidelessHead(%q, %d) failed with %v, want %v", digested, tt.body, err, tt.want)
			}
			if _, whole := d.dayHead(digested, tt.body); !errors.Is(whole, tt.want) {
				t.Errorf("dayHead(%q, %d) failed with %v, want %v, as the read without its side fails", digested, tt.body, whole, tt.want)
			}
		})
	}
}

// A day head stands, and its side with it, while what it was built from
// does: its day in this time zone, keeping the lines its summary counted as
// it was built; none of the lines of the days either side of it arriving on
// it, as far as their digests tell, as a digest's day must be alone; and
// each day beside it whose files gave it a line keeping the lines it did as
// it was built, as a digest's entry rests on its days, which no day whose
// lines couldn't be told then does. A day it rests on pruned since, which
// alone passes, gives its day's read none of its lines.
func TestADayHeadStandsOnlyWhileWhatItWasBuiltFromDoes(t *testing.T) {
	built := dayHeadOf(digested, 40, `[]`)
	_, offset := built.Start.Zone()
	elsewhere := time.FixedZone("elsewhere", offset+3*60*60)
	inZone, started, ended := built, built, built
	inZone.Start, inZone.End = time.Date(2026, 10, 5, 0, 0, 0, 0, elsewhere), time.Date(2026, 10, 6, 0, 0, 0, 0, elsewhere)
	started.Start, ended.End = built.Start.Add(time.Hour), built.End.Add(time.Hour)
	onBefore, onAfter, onUntold := built, built, built
	onBefore.RestsOn, onAfter.RestsOn, onUntold.RestsOn = map[string]int{before: 9}, map[string]int{after: 30}, map[string]int{before: untold}
	tests := []struct {
		name string
		head DayHead
		// kept are the lines the ledger keeps of each day, as keeping gives
		// them, and heads the heads of the digests it holds, by their dates.
		kept  map[string]keptDay
		heads map[string]DigestHead
		want  bool
	}{
		{name: "as it was built", head: built, kept: keeping(nil), want: true},
		{
			name: "the day after's digest current, none of its lines arriving before it", head: built,
			kept: keeping(map[string]int{after: 30}), heads: map[string]DigestHead{after: headOf(after, 30)}, want: true,
		},
		{
			name: "the day after's digest not current, as its summary was made again since, though a line of it arrived before it", head: built,
			kept: keeping(map[string]int{after: 31}), heads: map[string]DigestHead{after: arriving(headOf(after, 30), 1, 0)}, want: true,
		},
		{
			name: "the day before's digest current, none of its lines arriving after it", head: built,
			kept: keeping(map[string]int{before: 9}), heads: map[string]DigestHead{before: headOf(before, 9)}, want: true,
		},
		{
			name: "the day after's digest current, a line of it arriving before it", head: built,
			kept: keeping(map[string]int{after: 30}), heads: map[string]DigestHead{after: arriving(headOf(after, 30), 1, 0)}, want: false,
		},
		{
			name: "the day before's digest current, a line of it arriving after it", head: built,
			kept: keeping(map[string]int{before: 9}), heads: map[string]DigestHead{before: arriving(headOf(before, 9), 0, 1)}, want: false,
		},
		{name: "the day before keeping lines, of no digest", head: built, kept: keeping(map[string]int{before: 9}), want: false},
		{name: "the day before keeping lines of no summary yet, of no digest", head: built, kept: keeping(nil, before), want: false},
		{name: "built in another time zone", head: inZone, kept: keeping(nil), want: false},
		{name: "built where its day started otherwise", head: started, kept: keeping(nil), want: false},
		{name: "built where its day ended otherwise", head: ended, kept: keeping(nil), want: false},
		{name: "its day's summary missing", head: built, kept: keeping(map[string]int{digested: 0}, digested), want: false},
		{name: "its day's summary not standing as held, of the lines it was built against", head: built, kept: keeping(nil, digested), want: false},
		{name: "its day's summary made again since, of more lines", head: built, kept: keeping(map[string]int{digested: 41}), want: false},
		{name: "its day's lines pruned", head: built, kept: keeping(map[string]int{digested: 0}), want: false},
		{
			name: "resting on the day before, keeping the lines it did, its digest current", head: onBefore,
			kept: keeping(map[string]int{before: 9}), heads: map[string]DigestHead{before: headOf(before, 9)}, want: true,
		},
		{name: "resting on the day before, its lines pruned since", head: onBefore, kept: keeping(nil), want: false},
		{
			name: "resting on the day before, summarised again since, of a line more, its digest current", head: onBefore,
			kept: keeping(map[string]int{before: 10}), heads: map[string]DigestHead{before: headOf(before, 10)}, want: false,
		},
		{name: "resting on the day after, keeping the lines it did, of no digest", head: onAfter, kept: keeping(map[string]int{after: 30}), want: true},
		{name: "resting on the day after, its lines pruned since", head: onAfter, kept: keeping(map[string]int{after: 0}), want: false},
		{name: "resting on the day after, summarised again since, of no digest", head: onAfter, kept: keeping(map[string]int{after: 31}), want: false},
		{name: "resting on the day after, its lines not told since", head: onAfter, kept: keeping(map[string]int{after: 30}, after), want: false},
		{
			name: "resting on the day before, untold as it was built, told since, its digest current", head: onUntold,
			kept: keeping(map[string]int{before: 9}), heads: map[string]DigestHead{before: headOf(before, 9)}, want: false,
		},
		{name: "resting on the day before, untold as it was built, its lines pruned since", head: onUntold, kept: keeping(nil), want: false},
		{name: "resting on the day before, untold as it was built, nor since", head: onUntold, kept: keeping(map[string]int{before: 9}, before), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := groundsOf(tt.kept, tt.heads)

			if got := g.dayHeadStands(tt.head); got != tt.want {
				t.Errorf("dayHeadStands(%+v) = %t, want %t, the ledger keeping %v, holding digests %+v", tt.head, got, tt.want, tt.kept, tt.heads)
			}
		})
	}
}

// A day head is built to rest on each day beside its own whose files give it
// a line, of the lines the ledger keeps of it: the day before untold where
// they can't be told, so the head doesn't stand once that day is pruned, as
// alone would have it; the day after not at all, as while it's today, so the
// head stands meanwhile, as alone has it while that day's digest isn't
// current.
func TestAHeadRestsOnTheDaysBesideItsOwnAsTheirLinesAreTold(t *testing.T) {
	tests := []struct {
		name string
		// kept are the lines the ledger keeps of each day, as keeping gives
		// them.
		kept map[string]keptDay
		want map[string]int
	}{
		{name: "both told", kept: keeping(map[string]int{before: 9, after: 30}), want: map[string]int{before: 9, after: 30}},
		{name: "the day before pruned", kept: keeping(map[string]int{after: 30}), want: map[string]int{before: 0, after: 30}},
		{name: "the day before untold", kept: keeping(map[string]int{before: 9, after: 30}, before), want: map[string]int{before: untold, after: 30}},
		{name: "the day after untold, as today", kept: keeping(map[string]int{before: 9}), want: map[string]int{before: 9}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := keptBeside(digested, groundsOf(tt.kept, nil).kept); !maps.Equal(got, tt.want) {
				t.Errorf("keptBeside(%q) = %v, want %v, the ledger keeping %v", digested, got, tt.want, tt.kept)
			}
		})
	}
}
