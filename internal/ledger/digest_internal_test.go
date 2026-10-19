package ledger

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/logs"
)

// The dates of the local days the digests' tests digest: digested, a
// Monday; the days either side of it; and rested, a day before them its
// sessions rest on.
const (
	digested      = "2026-10-05"
	before, after = "2026-10-04", "2026-10-06"
	rested        = "2026-10-02"
)

// statesVersion is the version of the sessions' states the digests' tests
// digest, as their builder would give it.
const statesVersion = 3

// digestDays are the ledger's days in a directory of the test's own.
func digestDays(t *testing.T) days {
	t.Helper()
	logger := logs.For("router")
	return days{files: filesIn(t.TempDir(), logger), logger: logger}
}

// headOf is the head of a digest of the local day with the given date, of
// states of statesVersion, as it was built against a summary of as many
// lines as given, every one of them read, none arriving on another day.
func headOf(date string, lines int) DigestHead {
	start, end, _ := dayfile.Day(date)
	return DigestHead{Body: statesVersion, Day: date, Start: start, End: end, SummaryLines: lines, Lines: lines}
}

// arriving is h holding as many lines that arrived before its day started,
// early, and after it ended, late, as given.
func arriving(h DigestHead, early, late int) DigestHead {
	h.Early, h.Late = early, late
	return h
}

// gunzipped returns what data, gzipped, holds.
func gunzipped(t *testing.T, data []byte) []byte {
	t.Helper()
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	return plain
}

// A digest reads back as it was written, every field of it, a session's
// state byte for byte, its characters unescaped; each session's entry is the
// one written, whichever is asked for, pointing at its part, which reads back
// as it was given, but for its insignificant spaces, on the date it was
// given, none pointing at one of a session given none; and its head, read
// alone, is the head written, recording its parts file's size.
func TestADigestReadsBackAsItWasWritten(t *testing.T) {
	every := arriving(headOf(digested, 42), 1, 2)
	every.Lines = 41
	tests := []struct {
		name   string
		digest Digest
		parts  map[string]builtPart
	}{
		{
			name: "every field held",
			digest: Digest{Head: every, Sessions: map[string]DigestEntry{
				"one": {RestsOn: map[string]int{rested: 12, digested: 42},
					State: json.RawMessage(`{"story":{"dir":"~/Code/<api> & co","named":"café"},"counts":{"2026-10-05":7}}`)},
				"two":   {RestsOn: map[string]int{digested: 42}, State: json.RawMessage(`[1,"\u00e9",null]`)},
				"three": {RestsOn: map[string]int{digested: 42}, State: json.RawMessage(`{}`)},
			}},
			parts: map[string]builtPart{
				"one": {on: rested, data: json.RawMessage(`{"turns":[{"prompt":"<api> & café"}], "times": [3, 1, 4]}`)},
				"two": {data: json.RawMessage(`[1, "\u00e9"]`)},
			},
		},
		{
			name:   "none of its counts, and no sessions",
			digest: Digest{Head: headOf(digested, 0)},
		},
		{
			name:   "a session of no state, resting on no day, of no part",
			digest: Digest{Head: headOf(digested, 3), Sessions: map[string]DigestEntry{"one": {}}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := digestDays(t)
			written, err := d.writeDigest(tt.digest, tt.parts)
			if err != nil {
				t.Fatalf("writeDigest(%+v): %v", tt.digest, err)
			}

			read, err := d.digest(digested, statesVersion)

			if err != nil || !reflect.DeepEqual(read, written) {
				t.Fatalf("digest(%q) = %+v, %v; want %+v, as it was written", digested, read, err, written)
			}
			for id, want := range tt.digest.Sessions {
				got := read.Sessions[id]
				if !reflect.DeepEqual(got.RestsOn, want.RestsOn) || !bytes.Equal(got.State, want.State) {
					t.Errorf("the entry of %q reads %+v, want %+v", id, got, want)
				}
				isPart(t, d, id, got.Part, tt.parts)
			}
			head := tt.digest.Head
			head.PartsSize = partsSize(t, d)
			if !reflect.DeepEqual(read.Head, head) {
				t.Errorf("the digest's head reads %+v, want %+v, as it was written, recording its parts file's size", read.Head, head)
			}
			if head, err := d.digestHead(digested, statesVersion); err != nil || !reflect.DeepEqual(head, written.Head) {
				t.Errorf("digestHead(%q) = %+v, %v; want %+v, as it was written", digested, head, err, written.Head)
			}
		})
	}
}

// isPart fails t where ref, the part the entry of the session with the given
// id points at in the digests' day's parts file, isn't of the part parts
// gives it, as it was given, but for its insignificant spaces, on its date:
// or isn't none, where it gives none.
func isPart(t *testing.T, d days, id string, ref *PartRef, parts map[string]builtPart) {
	t.Helper()
	want, parted := parts[id]
	if !parted || ref == nil {
		if parted || ref != nil {
			t.Errorf("the entry of %q points at part %+v, want one %t", id, ref, parted)
		}
		return
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, want.data); err != nil {
		t.Fatal(err)
	}
	if got, err := d.readPart(digested, *ref); err != nil || ref.On != want.on || !bytes.Equal(got, compact.Bytes()) {
		t.Errorf("the part of %q, on %q, reads %s, %v; want %s, on %q", id, ref.On, got, err, compact.Bytes(), want.on)
	}
}

// partsSize is the size of the parts file of the digests' day, failing the
// test where it can't be looked at: none where there's none.
func partsSize(t *testing.T, d days) int64 {
	t.Helper()
	info, err := os.Stat(partsFile(d.files.Dir, digested))
	if errors.Is(err, fs.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}

// A digest's head is read by stopping at its sessions' entries: so they're
// its stored form's last field, written after every field of its head.
func TestADigestsSessionsAreItsStoredFormsLastField(t *testing.T) {
	fields := reflect.VisibleFields(reflect.TypeFor[storedDigest]())
	if name, _, _ := strings.Cut(fields[len(fields)-1].Tag.Get("json"), ","); name != digestSessions {
		t.Errorf("storedDigest's last field is stored as %q, want %q: its head's fields come first, as storedHeadIn reads them", name, digestSessions)
	}
}

// A digest's head is read without its sessions' entries, which no damage to
// them or its gzip trailer fails, as the whole read does, as none of them is
// read.
func TestADigestsHeadIsReadWithoutItsSessions(t *testing.T) {
	head := fmt.Sprintf(`{"version":%d,"body":3,"day":"2026-10-05","from":1,"to":2,"summary_lines":40,"lines":40,"late":1,"parts_size":300,`, digestVersion)
	whole := gzippedText(t, head+`"sessions":{"one":{"state":[1]}}}`)
	tests := []struct {
		name string
		file []byte
	}{
		{name: "its sessions cut short", file: gzippedText(t, head+`"sessions":{"one":{"sta`)},
		{name: "its sessions not JSON", file: gzippedText(t, head+`"sessions":not a digest`)},
		{name: "cut short in its gzip trailer", file: whole[:len(whole)-4]},
	}
	want := DigestHead{Body: statesVersion, Day: digested, Start: time.Unix(0, 1), End: time.Unix(0, 2), SummaryLines: 40, Lines: 40, Late: 1, PartsSize: 300}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := digestDays(t)
			if err := os.WriteFile(digestFile(d.files.Dir, digested), tt.file, 0o600); err != nil {
				t.Fatal(err)
			}

			got, err := d.digestHead(digested, statesVersion)

			if err != nil || !reflect.DeepEqual(got, want) {
				t.Errorf("digestHead(%q) = %+v, %v; want %+v, its sessions unread", digested, got, err, want)
			}
			if _, err := d.digest(digested, statesVersion); !errors.Is(err, errNotTheDaysDigest) {
				t.Errorf("digest(%q) failed with %v, want %v: the whole read reads its sessions", digested, err, errNotTheDaysDigest)
			}
		})
	}
}

// A digest's head that doesn't read as the day's digest's, or reads as one of
// another version, fails as the whole read fails.
func TestADigestsHeadThatDoesntReadFailsAsTheWholeReadDoes(t *testing.T) {
	stored := storedOf(Digest{Head: headOf(digested, 40)})
	file := func(s storedDigest) []byte {
		data, err := gzippedJSON(s)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	envelope := stored
	envelope.Version = digestVersion + 1
	tests := []struct {
		name string
		// file is what the ledger holds as the day's digest: nil for none.
		file []byte
		body int
		want error
	}{
		{name: "none held", file: nil, body: statesVersion, want: fs.ErrNotExist},
		{name: "of another version of the envelope", file: file(envelope), body: statesVersion, want: errDigestOutdated},
		{name: "of another version of the states", file: file(stored), body: statesVersion + 1, want: errDigestOutdated},
		{name: "another day's", file: file(storedOf(Digest{Head: headOf(before, 7)})), body: statesVersion, want: errNotTheDaysDigest},
		{name: "cut short within its head", file: gzippedText(t, `{"version":1,"body":3,"day":"2026-10-05","from":`), body: statesVersion, want: errNotTheDaysDigest},
		{name: "not gzipped", file: gunzipped(t, file(stored)), body: statesVersion, want: errNotTheDaysDigest},
		{name: "gzipped, but not JSON", file: gzippedText(t, "not a digest"), body: statesVersion, want: errNotTheDaysDigest},
		{name: "gzipped JSON, but not an object", file: gzippedText(t, `["2026-10-05"]`), body: statesVersion, want: errNotTheDaysDigest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := digestDays(t)
			if tt.file != nil {
				if err := os.WriteFile(digestFile(d.files.Dir, digested), tt.file, 0o600); err != nil {
					t.Fatal(err)
				}
			}

			_, err := d.digestHead(digested, tt.body)

			if !errors.Is(err, tt.want) {
				t.Errorf("digestHead(%q, %d) failed with %v, want %v", digested, tt.body, err, tt.want)
			}
			if _, whole := d.digest(digested, tt.body); !errors.Is(whole, tt.want) {
				t.Errorf("digest(%q, %d) failed with %v, want %v, as the head read fails", digested, tt.body, whole, tt.want)
			}
		})
	}
}

// A digest's file is its stored form, gzipped, the user's alone: its keys
// short, its zeros left out, its day's bounds as nanoseconds since the Unix
// epoch, its parts file's size, its sessions in the order of their ids, each
// state as it was given, but for its insignificant spaces, its characters
// unescaped, and where each part lies, a first at no offset, a whole one on
// no day.
func TestADigestIsWrittenGzippedAsItsStoredFormTheUsersAlone(t *testing.T) {
	d := digestDays(t)
	digest := Digest{
		Head: DigestHead{Body: statesVersion, Day: digested, Start: time.Date(2026, 10, 4, 23, 0, 0, 0, time.UTC),
			End: time.Date(2026, 10, 5, 23, 0, 0, 0, time.UTC), SummaryLines: 40, Lines: 40},
		Sessions: map[string]DigestEntry{
			"two": {RestsOn: map[string]int{digested: 40}, State: json.RawMessage(`{ "dir": "~/<api> & co" }`)},
			"one": {RestsOn: map[string]int{rested: 12, digested: 40}, State: json.RawMessage(`[1]`)},
		},
	}
	parts := map[string]builtPart{"one": {on: rested, data: json.RawMessage(`[2]`)}, "two": {data: json.RawMessage(`[3]`)}}
	written, err := d.writeDigest(digest, parts)
	if err != nil {
		t.Fatal(err)
	}

	path := digestFile(d.files.Dir, digested)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	one, two := written.Sessions["one"].Part, written.Sessions["two"].Part
	want := fmt.Sprintf(`{"version":%d,"body":3,"day":"2026-10-05","from":1791154800000000000,"to":1791241200000000000,"summary_lines":40,"lines":40,"parts_size":%d,`+
		`"sessions":{"one":{"rests_on":{"2026-10-02":12,"2026-10-05":40},"state":[1],"part":{"on":"2026-10-02","length":%d,"crc":%d}},`+
		`"two":{"rests_on":{"2026-10-05":40},"state":{"dir":"~/<api> & co"},"part":{"offset":%d,"length":%d,"crc":%d}}}}`+"\n",
		digestVersion, written.Head.PartsSize, one.Length, one.CRC, two.Offset, two.Length, two.CRC)
	if got := string(gunzipped(t, data)); got != want {
		t.Errorf("the digest's file holds\n%s\nwant\n%s", got, want)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the digest's file is %v, %v; want it the user's alone, 0600", info.Mode().Perm(), err)
	}
}

// A day's parts file is a gzip member of each session's part, as it was
// given, but for its insignificant spaces, its characters unescaped, one
// after another in the order of the sessions' ids, from its start to its
// end, the user's alone; and the same parts write the same file, byte for
// byte, whatever order they're given in.
func TestAPartsFileIsAMemberOfEachPartInTheOrderOfTheirIdsTheUsersAlone(t *testing.T) {
	d := digestDays(t)
	sessions := map[string]DigestEntry{"c": {}, "a": {}, "b": {}, "d": {}}
	parts := map[string]builtPart{
		"c": {on: before, data: json.RawMessage(`{"turns": [3]}`)},
		"a": {on: rested, data: json.RawMessage(`{"prompt":"<api> & café"}`)},
		"b": {data: json.RawMessage(`[ 2 ]`)},
	}
	written, err := d.writeDigest(Digest{Head: headOf(digested, 4), Sessions: sessions}, parts)
	if err != nil {
		t.Fatal(err)
	}

	path := partsFile(d.files.Dir, digested)
	file, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(gunzipped(t, file)), `{"prompt":"<api> & café"}[2]{"turns":[3]}`; got != want {
		t.Errorf("the parts file's members hold %s, want %s, of a, b and c, in that order", got, want)
	}
	var at int64
	for _, id := range []string{"a", "b", "c"} {
		ref := written.Sessions[id].Part
		if ref == nil || ref.Offset != at {
			t.Fatalf("the part of %q is at %+v, want it at %d, after those of the ids before it", id, ref, at)
		}
		at += ref.Length
	}
	if at != int64(len(file)) || written.Head.PartsSize != at {
		t.Errorf("the parts file is %d bytes, its head records %d; want both %d, the members' bytes, and none besides", len(file), written.Head.PartsSize, at)
	}
	if info, err := os.Stat(path); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the parts file is %v, %v; want it the user's alone, 0600", info.Mode().Perm(), err)
	}
	again := digestDays(t)
	if _, err := again.writeDigest(Digest{Head: headOf(digested, 4), Sessions: maps.Clone(sessions)}, maps.Clone(parts)); err != nil {
		t.Fatal(err)
	}
	if same, err := os.ReadFile(partsFile(again.files.Dir, digested)); err != nil || !bytes.Equal(same, file) {
		t.Errorf("the same parts written again make a parts file of %d bytes, %v; want it byte for byte the first's", len(same), err)
	}
}

// A digest written of no parts leaves its day no parts file, the one there
// before removed, its head recording none.
func TestADigestOfNoPartsLeavesItsDayNoPartsFile(t *testing.T) {
	d := digestDays(t)
	if _, err := d.writeDigest(Digest{Head: headOf(digested, 1), Sessions: map[string]DigestEntry{"one": {}}},
		map[string]builtPart{"one": {data: json.RawMessage(`[1]`)}}); err != nil {
		t.Fatal(err)
	}

	written, err := d.writeDigest(Digest{Head: headOf(digested, 1), Sessions: map[string]DigestEntry{"one": {}}}, nil)

	if _, statted := os.Stat(partsFile(d.files.Dir, digested)); err != nil || !errors.Is(statted, fs.ErrNotExist) {
		t.Errorf("writeDigest() = %v, the parts file looked at: %v; want it written, and none there", err, statted)
	}
	if written.Head.PartsSize != 0 || written.Sessions["one"].Part != nil {
		t.Errorf("the digest written is %+v; want its head recording no parts, its entry pointing at none", written)
	}
}

// A digest that can't be written whole, of a day that isn't one, or holding
// a state, or a part, that isn't JSON, isn't written at all, nor its parts
// file, failing with an error that names the day.
func TestADigestThatCantBeWrittenWholeIsntWritten(t *testing.T) {
	cutShort := map[string]DigestEntry{"one": {State: json.RawMessage(`{"dir":"~/a`)}}
	tests := []struct {
		name   string
		digest Digest
		parts  map[string]builtPart
	}{
		{name: "of a day that isn't one", digest: Digest{Head: DigestHead{Body: statesVersion, Day: "someday"}}},
		{
			name: "a session's state that isn't JSON", digest: Digest{Head: headOf(digested, 1), Sessions: cutShort},
			parts: map[string]builtPart{"one": {data: json.RawMessage(`[1]`)}},
		},
		{
			name: "a session's part that isn't JSON", digest: Digest{Head: headOf(digested, 1), Sessions: map[string]DigestEntry{"one": {}}},
			parts: map[string]builtPart{"one": {data: json.RawMessage(`{"turns":[1`)}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := digestDays(t)

			_, err := d.writeDigest(tt.digest, tt.parts)

			entries, _ := os.ReadDir(d.files.Dir)
			if err == nil || len(entries) > 0 {
				t.Fatalf("writeDigest(%+v) = %v, leaving %d files; want it failing, writing none", tt.digest, err, len(entries))
			}
			if !strings.Contains(err.Error(), tt.digest.Head.Day) {
				t.Errorf("writeDigest(%+v) failed with %q, want it naming the day, %s", tt.digest, err, tt.digest.Head.Day)
			}
		})
	}
}

// A digest whose parts file can't be written isn't written, as it would
// point at parts that aren't there: the digest held before is left as it
// was.
func TestADigestWhosePartsCantBeWrittenIsntWritten(t *testing.T) {
	d := digestDays(t)
	was, err := d.writeDigest(Digest{Head: headOf(digested, 1), Sessions: map[string]DigestEntry{"one": {State: json.RawMessage(`[1]`)}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// A parts file can't be written over a directory standing at its path.
	if err := os.Mkdir(partsFile(d.files.Dir, digested), 0o700); err != nil {
		t.Fatal(err)
	}
	digest := Digest{Head: headOf(digested, 2), Sessions: map[string]DigestEntry{"one": {State: json.RawMessage(`[1,2]`)}}}

	_, err = d.writeDigest(digest, map[string]builtPart{"one": {data: json.RawMessage(`[2]`)}})

	if read, readErr := d.digest(digested, statesVersion); err == nil || readErr != nil || !reflect.DeepEqual(read, was) {
		t.Errorf("writeDigest() = %v, the digest then reading %+v, %v; want it failing, the digest held before, %+v, left as it was", err, read, readErr, was)
	}
}

// A digest held that doesn't read as the day's digest, or reads as one of
// another version, fails as its reader can tell from there being none.
func TestADigestThatDoesntReadAsTheDaysFailsSoItsReaderCanTell(t *testing.T) {
	stored := storedOf(Digest{Head: headOf(digested, 40), Sessions: map[string]DigestEntry{"one": {State: json.RawMessage(`{"dir":"~/api"}`)}}})
	file := func(s storedDigest) []byte {
		data, err := gzippedJSON(s)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	whole := file(stored)
	envelope := stored
	envelope.Version = digestVersion + 1
	damaged := bytes.Clone(whole)
	damaged[len(damaged)/2] ^= 0xff
	plain := gunzipped(t, whole)
	tests := []struct {
		name string
		// file is what the ledger holds as the day's digest: nil for none.
		file []byte
		body int
		want error
	}{
		{name: "none held", file: nil, body: statesVersion, want: fs.ErrNotExist},
		{name: "of another version of the envelope", file: file(envelope), body: statesVersion, want: errDigestOutdated},
		{name: "of another version of the states", file: whole, body: statesVersion + 1, want: errDigestOutdated},
		{name: "another day's", file: file(storedOf(Digest{Head: headOf(before, 7)})), body: statesVersion, want: errNotTheDaysDigest},
		{name: "cut short within its JSON", file: whole[:len(whole)/2], body: statesVersion, want: errNotTheDaysDigest},
		{name: "cut short past its JSON, in its gzip trailer", file: whole[:len(whole)-4], body: statesVersion, want: errNotTheDaysDigest},
		{name: "damaged", file: damaged, body: statesVersion, want: errNotTheDaysDigest},
		{name: "not gzipped", file: plain, body: statesVersion, want: errNotTheDaysDigest},
		{name: "gzipped, but not JSON", file: gzippedText(t, "not a digest"), body: statesVersion, want: errNotTheDaysDigest},
		{name: "gzipped JSON, but not an object", file: gzippedText(t, `["2026-10-05"]`), body: statesVersion, want: errNotTheDaysDigest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := digestDays(t)
			if tt.file != nil {
				if err := os.WriteFile(digestFile(d.files.Dir, digested), tt.file, 0o600); err != nil {
					t.Fatal(err)
				}
			}

			_, err := d.digest(digested, tt.body)

			if !errors.Is(err, tt.want) {
				t.Errorf("digest(%q, %d) failed with %v, want %v", digested, tt.body, err, tt.want)
			}
			if !errors.Is(tt.want, fs.ErrNotExist) && errors.Is(err, fs.ErrNotExist) {
				t.Errorf("digest(%q, %d) failed with %v, as though none were held", digested, tt.body, err)
			}
		})
	}
}

// A digest is judged by its head before its sessions' entries are read: one
// of another version, of the envelope or of the states, as every one is
// after a release raising either, fails as outdated whatever follows its
// head, as none of its entries is read; and one of the version asked for
// then has them read, failing where they don't.
func TestADigestIsJudgedByItsHeadBeforeItsSessionsAreRead(t *testing.T) {
	headed := func(version, body int) string {
		return fmt.Sprintf(`{"version":%d,"body":%d,"day":"2026-10-05","from":1,"to":2,"summary_lines":40,"lines":40,`, version, body)
	}
	tests := []struct {
		name string
		head string
		want error
	}{
		{name: "of another version of the envelope", head: headed(digestVersion+1, statesVersion), want: errDigestOutdated},
		{name: "of another version of the states", head: headed(digestVersion, statesVersion+1), want: errDigestOutdated},
		{name: "of the version asked for", head: headed(digestVersion, statesVersion), want: errNotTheDaysDigest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := digestDays(t)
			if err := os.WriteFile(digestFile(d.files.Dir, digested), gzippedText(t, tt.head+`"sessions":not a digest`), 0o600); err != nil {
				t.Fatal(err)
			}

			_, err := d.digest(digested, statesVersion)

			if !errors.Is(err, tt.want) {
				t.Errorf("digest(%q, %d) of sessions that don't read failed with %v, want %v", digested, statesVersion, err, tt.want)
			}
		})
	}
}

// A digest's file that inflates past digestMax, as one made by hand may,
// fails as one that doesn't read as the day's, inflated no further: its
// gzip trailer, cut short two megabytes past the most, is never reached.
func TestADigestInflatingPastItsMostFailsUnread(t *testing.T) {
	d := digestDays(t)
	var data bytes.Buffer
	w, err := gzip.NewWriterLevel(&data, gzip.BestSpeed)
	if err != nil {
		t.Fatal(err)
	}
	padding := bytes.Repeat([]byte{'a'}, 1<<20)
	parts := [][]byte{fmt.Appendf(nil, `{"version":%d,"body":3,"day":"2026-10-05","from":0,"to":0,"sessions":{"one":{"state":"`, digestVersion)}
	for range digestMax/len(padding) + 2 {
		parts = append(parts, padding)
	}
	for _, part := range append(parts, []byte(`"}}}`)) {
		if _, err := w.Write(part); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(digestFile(d.files.Dir, digested), data.Bytes()[:data.Len()-4], 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = d.digest(digested, statesVersion)

	if !errors.Is(err, errNotTheDaysDigest) || !errors.Is(err, errDigestTooLarge) {
		t.Errorf("digest(%q, %d) of a file inflating past %d bytes failed with %v, want %v, as %v", digested, statesVersion, digestMax, err,
			errNotTheDaysDigest, errDigestTooLarge)
	}
}

// gzippedText is text, gzipped.
func gzippedText(t *testing.T, text string) []byte {
	t.Helper()
	var data bytes.Buffer
	w := gzip.NewWriter(&data)
	if _, err := w.Write([]byte(text)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return data.Bytes()
}

// keptDay is what digestGrounds' kept gives of a day: its lines, as its
// summary counts them, and whether that summary stands as held.
type keptDay struct {
	lines int
	known bool
}

// keeping is the lines the ledger keeps of each day, as digestGrounds' kept
// gives them, of the digests' day as it was built: its own, against a
// summary of 40 lines; those of the day its sessions rest on; none of the
// day before; and none summarised yet of the day after, as of today. over
// gives other lines of the days it names, and unsettled the days whose
// summaries don't stand as held, of the lines they count all the same.
func keeping(over map[string]int, unsettled ...string) map[string]keptDay {
	lines := map[string]int{rested: 12, before: 0, digested: 40}
	maps.Copy(lines, over)
	kept := make(map[string]keptDay, len(lines))
	for date, n := range lines {
		kept[date] = keptDay{lines: n, known: true}
	}
	for _, date := range unsettled {
		kept[date] = keptDay{lines: lines[date]}
	}
	return kept
}

// groundsOf are the grounds of a ledger that keeps the lines of each day
// kept gives, by its date, none summarised of a day it doesn't give, and
// holds the digests whose heads are given, by their dates.
func groundsOf(kept map[string]keptDay, heads map[string]DigestHead) digestGrounds {
	return digestGrounds{
		kept: func(date string) (int, bool) {
			return kept[date].lines, kept[date].known
		},
		heads: func(date string) (DigestHead, bool) {
			head, held := heads[date]
			return head, held
		},
	}
}

// A digest stands, and a session's entry in it, while what it was built from
// does: its day in this time zone, keeping the lines its summary counted as
// it was built, none of them of another day, and none of another's of its
// own, as far as the days beside it tell; and each day the entry rests on
// keeping the lines it did.
func TestADigestStandsOnlyWhileWhatItWasBuiltFromDoes(t *testing.T) {
	built := headOf(digested, 40)
	entry := DigestEntry{RestsOn: map[string]int{rested: 12, digested: 40}, State: json.RawMessage(`{}`)}
	_, offset := built.Start.Zone()
	elsewhere := time.FixedZone("elsewhere", offset+3*60*60)
	inZone, started, ended := built, built, built
	inZone.Start, inZone.End = time.Date(2026, 10, 5, 0, 0, 0, 0, elsewhere), time.Date(2026, 10, 6, 0, 0, 0, 0, elsewhere)
	started.Start, ended.End = built.Start.Add(time.Hour), built.End.Add(time.Hour)
	tests := []struct {
		name  string
		head  DigestHead
		entry DigestEntry
		// kept are the lines the ledger keeps of each day, as keeping gives
		// them, and heads the heads of the digests it holds, by their dates.
		kept  map[string]keptDay
		heads map[string]DigestHead
		want  bool
	}{
		{name: "as it was built", head: built, entry: entry, kept: keeping(nil), want: true},
		{name: "its head alone, as it was built", head: built, kept: keeping(nil), want: true},
		{
			name: "the day after's digest current, none of its lines arriving before it", head: built, entry: entry,
			kept: keeping(map[string]int{after: 30}), heads: map[string]DigestHead{after: headOf(after, 30)}, want: true,
		},
		{
			name: "the day after's digest not current, as its summary was made again since, though a line of it arrived before it",
			head: built, entry: entry,
			kept: keeping(map[string]int{after: 31}), heads: map[string]DigestHead{after: arriving(headOf(after, 30), 1, 0)}, want: true,
		},
		{
			name: "the day before's digest current, none of its lines arriving after it", head: built, entry: entry,
			kept: keeping(map[string]int{before: 9}), heads: map[string]DigestHead{before: headOf(before, 9)}, want: true,
		},
		{name: "built in another time zone", head: inZone, entry: entry, kept: keeping(nil), want: false},
		{name: "built where its day started otherwise", head: started, entry: entry, kept: keeping(nil), want: false},
		{name: "built where its day ended otherwise", head: ended, entry: entry, kept: keeping(nil), want: false},
		{name: "a line it read arriving before its day", head: arriving(built, 1, 0), entry: entry, kept: keeping(nil), want: false},
		{name: "a line it read arriving after its day", head: arriving(built, 0, 1), entry: entry, kept: keeping(nil), want: false},
		{name: "its day's summary missing", head: built, kept: keeping(map[string]int{digested: 0}, digested), want: false},
		{name: "its day's summary not standing as held, of the lines it was built against", head: built, kept: keeping(nil, digested), want: false},
		{name: "its day's summary made again since, of more lines", head: built, kept: keeping(map[string]int{digested: 41}), want: false},
		{name: "its day's lines pruned", head: built, kept: keeping(map[string]int{digested: 0}), want: false},
		{
			name: "a day its entry rests on summarised again since, of more lines", head: built, entry: entry,
			kept: keeping(map[string]int{rested: 13}), want: false,
		},
		{name: "a day its entry rests on pruned", head: built, entry: entry, kept: keeping(map[string]int{rested: 0}), want: false},
		{
			name: "a day its entry rests on, its summary not standing as held, of the lines it was built from", head: built, entry: entry,
			kept: keeping(nil, rested), want: false,
		},
		{
			name: "the day after's digest current, a line of it arriving before it", head: built, entry: entry,
			kept: keeping(map[string]int{after: 30}), heads: map[string]DigestHead{after: arriving(headOf(after, 30), 1, 0)}, want: false,
		},
		{
			name: "the day before's digest current, a line of it arriving after it", head: built, entry: entry,
			kept: keeping(map[string]int{before: 9}), heads: map[string]DigestHead{before: arriving(headOf(before, 9), 0, 1)}, want: false,
		},
		{name: "the day before keeping lines, of no digest", head: built, entry: entry, kept: keeping(map[string]int{before: 9}), want: false},
		{name: "the day before keeping lines of no summary yet, of no digest", head: built, entry: entry, kept: keeping(nil, before), want: false},
		{
			name: "the day before keeping lines, its digest not current, as its summary was made again since", head: built, entry: entry,
			kept: keeping(map[string]int{before: 10}), heads: map[string]DigestHead{before: headOf(before, 9)}, want: false,
		},
		{
			name: "the day before keeping lines, its digest not current, as its summary doesn't stand as held", head: built, entry: entry,
			kept: keeping(map[string]int{before: 9}, before), heads: map[string]DigestHead{before: headOf(before, 9)}, want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := groundsOf(tt.kept, tt.heads)

			if got := g.stands(tt.head, tt.entry); got != tt.want {
				t.Errorf("stands(%+v, %+v) = %t, want %t, the ledger keeping %v, holding digests %+v", tt.head, tt.entry, got, tt.want, tt.kept, tt.heads)
			}
		})
	}
}
