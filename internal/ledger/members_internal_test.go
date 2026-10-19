package ledger

import (
	"bytes"
	"compress/gzip"
	"errors"
	"hash/crc32"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
)

// membersPath is where the members' tests keep their file of members: none
// written there yet.
func membersPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "members.gz")
}

// held writes data as the file at path, failing the test where it can't be.
func held(t *testing.T, path string, data []byte) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

// A file of members holds each member given, one after another in the order
// given, from its start to its end, each read back from where it lies as its
// content was given, and its bytes as they were given; and the same members
// make the same file, byte for byte.
func TestAFileOfMembersHoldsEachInTheOrderGiven(t *testing.T) {
	contents := [][]byte{[]byte(`{"turns":[1]}`), []byte(`[2]`), bytes.Repeat([]byte("3"), 4096)}
	given := make([][]byte, len(contents))
	for i, c := range contents {
		given[i] = memberOf(c)
	}
	file, members := membersOf(given)
	path := membersPath(t)
	held(t, path, file)

	var at int64
	for i, m := range members {
		if m.Offset != at {
			t.Errorf("member %d is at %d, want %d, after those before it", i, m.Offset, at)
		}
		at += m.Length
		if got, err := memberIn(path, m); err != nil || !bytes.Equal(got, contents[i]) {
			t.Errorf("memberIn(%+v) = %.40q, %v; want %.40q, the content of member %d", m, got, err, contents[i], i)
		}
		if got, err := memberBytes(path, m); err != nil || !bytes.Equal(got, given[i]) {
			t.Errorf("memberBytes(%+v) = %d bytes, %v; want the %d of member %d, as given", m, len(got), err, len(given[i]), i)
		}
	}
	if at != int64(len(file)) || len(members) != len(contents) {
		t.Errorf("the file is %d bytes of %d members, want %d of %d, its members' and none besides", len(file), len(members), at, len(contents))
	}
	if got, want := gunzipped(t, file), slices.Concat(contents...); !bytes.Equal(got, want) {
		t.Errorf("the file, inflated whole, holds %.60q, want %.60q, each member's in turn", got, want)
	}
	if again, _ := membersOf(slices.Clone(given)); !bytes.Equal(again, file) {
		t.Errorf("the same members make a file of %d bytes, want it byte for byte the first's, of %d", len(again), len(file))
	}
}

// A file of no members is empty.
func TestAFileOfNoMembersIsEmpty(t *testing.T) {
	if file, members := membersOf(nil); len(file) > 0 || len(members) > 0 {
		t.Errorf("membersOf(nil) = %d bytes, %v; want none", len(file), members)
	}
}

// oneMember is a file of one member, of content.
func oneMember(content string) []byte {
	return memberOf([]byte(content))
}

// whole is a member pointed at as the whole of data, by data's CRC.
func whole(data []byte) Member {
	return Member{Length: int64(len(data)), CRC: crc32.ChecksumIEEE(data)}
}

// A member its file doesn't hold as it was written where it's pointed at
// doesn't read, failing as its file being gone does, or as damaged: its
// bytes there not those written, by their CRC; no bytes there; or bytes
// whose CRC is right, but that aren't one gzip member, whole, which its
// bytes alone, uninflated, read as, being as written.
func TestAMemberNotHeldAsWrittenDoesntRead(t *testing.T) {
	file, members := membersOf([][]byte{oneMember(`{"turns":[1,2,3]}`), oneMember(`[4]`)})
	one, two := members[0], members[1]
	damaged := bytes.Clone(file)
	damaged[two.Offset+two.Length/2] ^= 0xff
	single := oneMember(`[1]`)
	cut, twice, trailing := single[:len(single)-4], slices.Concat(single, oneMember(`[2]`)), append(slices.Clone(single), 0)
	notGzipped := []byte("not a member")
	tests := []struct {
		name string
		// file is what the file pointed into holds: nil for none.
		file   []byte
		member Member
		want   error
		// asWritten is whether the bytes pointed at are those written, as
		// memberBytes reads them.
		asWritten bool
	}{
		{name: "its file gone", member: two, want: fs.ErrNotExist},
		{name: "a byte of it damaged, its file as long", file: damaged, member: two, want: errMemberDamaged},
		{name: "its file cut short within it", file: file[:two.Offset+two.Length/2], member: two, want: errMemberDamaged},
		{name: "its file written again, of other members", file: oneMember(`[4,5]`), member: two, want: errMemberDamaged},
		{name: "pointed at past its file's end", file: file, member: Member{Offset: int64(len(file)), Length: two.Length, CRC: two.CRC}, want: errMemberDamaged},
		{name: "pointed at by another member's CRC", file: file, member: Member{Offset: two.Offset, Length: two.Length, CRC: one.CRC}, want: errMemberDamaged},
		{name: "pointed at before its file's start", file: file, member: Member{Offset: -1, Length: one.Length, CRC: one.CRC}, want: errMemberDamaged},
		{name: "pointed at as no bytes", file: file, member: whole(nil), want: errMemberDamaged},
		{name: "pointed at as longer than a member can be", file: file, member: Member{Length: memberMax + 1, CRC: one.CRC}, want: errMemberDamaged},
		{name: "its bytes, by their CRC, not gzipped", file: notGzipped, member: whole(notGzipped), want: errMemberDamaged, asWritten: true},
		{name: "its bytes, by their CRC, a member cut short", file: cut, member: whole(cut), want: errMemberDamaged, asWritten: true},
		{name: "its bytes, by their CRC, two members", file: twice, member: whole(twice), want: errMemberDamaged, asWritten: true},
		{name: "its bytes, by their CRC, a member and a byte past its end", file: trailing, member: whole(trailing), want: errMemberDamaged, asWritten: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := membersPath(t)
			if tt.file != nil {
				held(t, path, tt.file)
			}

			got, err := memberIn(path, tt.member)

			if !errors.Is(err, tt.want) {
				t.Errorf("memberIn(%+v) = %q, %v; want it failing with %v", tt.member, got, err, tt.want)
			}
			if data, err := memberBytes(path, tt.member); tt.asWritten != (err == nil) || tt.asWritten && !bytes.Equal(data, tt.file) || !tt.asWritten && !errors.Is(err, tt.want) {
				t.Errorf("memberBytes(%+v) = %d bytes, %v; want them read as written %t, else failing with %v", tt.member, len(data), err, tt.asWritten, tt.want)
			}
		})
	}
}

// A member pointed past its file's end, as long as a member can be, fails as
// damaged before room is made for its bytes.
func TestAMemberPointedPastItsFilesEndIsRefusedBeforeItsRead(t *testing.T) {
	path := membersPath(t)
	held(t, path, oneMember(`[1]`))
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)

	_, err := memberIn(path, Member{Offset: 1, Length: memberMax, CRC: 1})

	runtime.ReadMemStats(&after)
	if allocated := after.TotalAlloc - before.TotalAlloc; !errors.Is(err, errMemberDamaged) || allocated >= memberMax {
		t.Errorf("memberIn() of %d bytes past its file's end failed with %v, allocating %d bytes; want it failing with %v, allocating fewer than it points at",
			memberMax, err, allocated, errMemberDamaged)
	}
}

// A member, its bytes as written, that inflates past memberMax, as one
// pointed at by hand may, fails as damaged, inflated no further.
func TestAMemberInflatingPastItsMostDoesntRead(t *testing.T) {
	var data bytes.Buffer
	w, err := gzip.NewWriterLevel(&data, gzip.BestSpeed)
	if err != nil {
		t.Fatal(err)
	}
	zeros := make([]byte, 1<<20)
	for range memberMax / len(zeros) {
		if _, err := w.Write(zeros); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := w.Write([]byte{0}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	path := membersPath(t)
	held(t, path, data.Bytes())

	_, err = memberIn(path, whole(data.Bytes()))

	if !errors.Is(err, errMemberDamaged) || !errors.Is(err, errMemberTooLarge) {
		t.Errorf("memberIn() of a member inflating past %d bytes failed with %v, want %v, as %v", memberMax, err, errMemberDamaged, errMemberTooLarge)
	}
}
