package ledger

import (
	"bytes"
	"hash/crc32"
	"reflect"
	"testing"
	"time"
)

// A day's rows file is each chunk's rows as a gzip member of its own, one
// after another, in the chunks' order: so the same chunks always make the
// same file, byte for byte, each chunk lying where the one before ends, of
// the CRC of its member's bytes, the file as long as its members; and no
// chunks make none.
func TestARowsFileIsItsChunksMembersOneAfterAnother(t *testing.T) {
	chunks := chunksGiven()
	file, rows := rowsFileOf(chunks)

	again, rowsAgain := rowsFileOf(chunksGiven())
	if !bytes.Equal(file, again) || !reflect.DeepEqual(rows, rowsAgain) {
		t.Errorf("rowsFileOf made\n%x, %+v\nthen\n%x, %+v\nof the same chunks; want the same file each time", file, rows, again, rowsAgain)
	}
	var at int64
	for i, c := range rows.Chunks {
		member := memberOf(chunks[i].Rows)
		want := RowsChunk{Offset: at, Length: int64(len(member)), CRC: crc32.ChecksumIEEE(member), Count: chunks[i].Count, Meta: chunks[i].Meta}
		if !reflect.DeepEqual(c, want) || !bytes.Equal(file[at:at+c.Length], member) {
			t.Errorf("chunk %d lies as %+v, holding %x; want %+v, holding its rows' member %x", i, c, file[at:at+c.Length], want, member)
		}
		at += c.Length
	}
	if rows.Size != at || int64(len(file)) != at {
		t.Errorf("the rows file is %d bytes, recorded as %d; want %d, its members' alone", len(file), rows.Size, at)
	}
	if none, noRows := rowsFileOf(nil); len(none) > 0 || !reflect.DeepEqual(noRows, DayRows{}) {
		t.Errorf("rowsFileOf(nil) = %x, %+v; want an empty file, pointed at as none", none, noRows)
	}
}

// A day's rows stand only while its head does, it points at a row of each of
// the lines it was built of, and the day's rows file is as long as the head
// records, as a look at it tells: one gone, or written again since of other
// rows, as by a build that stopped before its head, doesn't; nor does a head
// of more lines than rows, or fewer, as of a day of lines but no rows; and a
// head of no lines has none standing only where the day holds no rows file.
func TestADaysRowsStandOnlyWhileItsHeadDoesAndItsFileIsAsLong(t *testing.T) {
	built := dayHeadOf(digested, 40, `[]`)
	built.Lines = 3
	elsewhere := built
	elsewhere.Start, elsewhere.End = built.Start.Add(time.Hour), built.End.Add(time.Hour)
	more, fewer, none := built, built, built
	more.Lines, fewer.Lines, none.Lines = 4, 2, 0
	tests := []struct {
		name string
		head DayHead
		// chunks are the rows written as the day's, its head pointing at
		// them; file, what's written as its rows file after, nil for what
		// was, empty for none.
		chunks []BuiltChunk
		file   []byte
		want   bool
	}{
		{name: "as written", head: built, chunks: chunksGiven(), want: true},
		{name: "its rows file gone", head: built, chunks: chunksGiven(), file: []byte{}},
		{name: "its rows file written again, of other rows", head: built, chunks: chunksGiven(), file: memberOf([]byte("r1\nr2\nr3\n"))},
		{name: "its head not standing", head: elsewhere, chunks: chunksGiven()},
		{name: "of more lines than rows", head: more, chunks: chunksGiven()},
		{name: "of fewer lines than rows", head: fewer, chunks: chunksGiven()},
		{name: "of lines but no rows", head: built},
		{name: "of no lines, no rows held", head: none, want: true},
		{name: "of no lines, a rows file held", head: none, file: memberOf([]byte("r1\n"))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := digestDays(t)
			head, err := d.writeDay(tt.head, tt.chunks)
			if err != nil {
				t.Fatal(err)
			}
			if tt.file != nil {
				if err := d.writeRows(digested, tt.file); err != nil {
					t.Fatal(err)
				}
			}

			stands := d.rowsStand(groundsOf(keeping(nil), nil), head)

			if stands != tt.want {
				t.Errorf("rowsStand(%+v) = %t, want %t", head, stands, tt.want)
			}
		})
	}
}
