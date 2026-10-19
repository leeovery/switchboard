package ledger

import (
	"encoding/json"
	"fmt"
	"path/filepath"
)

// rowsPrefix and rowsSuffix begin and end the name of a day's rows file,
// beside its head: rows-<date>.gz.
const rowsPrefix, rowsSuffix = "rows-", ".gz"

// rowsFile is where the ledger in dir keeps the rows of the local day with
// the given date, as its head points at them.
func rowsFile(dir, date string) string {
	return filepath.Join(dir, rowsPrefix+date+rowsSuffix)
}

// DayRows are a day's rows as its head points at them in the day's rows
// file: Size, how long the file was written beside the head, in bytes, none
// where none was; Count, how many rows its chunks hold in all, as their
// builder counted them, read with the head's other fields, its chunks
// unread, so a head can be told to hold a row of each of its lines without
// them; and Chunks, each chunk of them, in the order of the lines they're
// built of, as RowsChunk has it, none of a day of no rows.
type DayRows struct {
	Size   int64
	Count  int
	Chunks []RowsChunk
}

// counted reports whether r's chunks count as many rows as r does in all,
// as a head written whole points at them: not where one's count was changed
// since, as by hand.
func (r DayRows) counted() bool {
	n := 0
	for _, c := range r.Chunks {
		n += c.Count
	}
	return n == r.Count
}

// RowsChunk is a chunk of a day's rows as the day's head points at it: the
// Member, its gzip member in the day's rows file; Count, how many rows it
// holds; and Meta, what it holds, as its builder told it, as BuiltChunk has
// it, none where it's nil.
type RowsChunk struct {
	Member
	Count int
	Meta  json.RawMessage
}

// storedChunk is a chunk of a day's rows as its head's file holds it, zeros
// left out.
type storedChunk struct {
	storedMember
	Count int             `json:"count,omitzero"`
	Meta  json.RawMessage `json:"meta,omitempty"`
}

// stored is c as its head's file holds it.
func (c RowsChunk) stored() storedChunk {
	return storedChunk{storedMember: c.Member.stored(), Count: c.Count, Meta: c.Meta}
}

// chunk is the chunk s holds.
func (s storedChunk) chunk() RowsChunk {
	return RowsChunk{Member: s.member(), Count: s.Count, Meta: s.Meta}
}

// rowsFileOf returns the rows file of chunks, a day's rows in chunks, in
// their order: each chunk's rows as a gzip member of its own, one after
// another, as membersOf lays them out, so the same chunks make the same
// file, byte for byte; and the rows as the day's head points at them there,
// the file's size recorded: none of none, as an empty file is none.
func rowsFileOf(chunks []BuiltChunk) ([]byte, DayRows) {
	members := make([][]byte, len(chunks))
	for i, c := range chunks {
		members[i] = memberOf(c.Rows)
	}
	file, at := membersOf(members)
	rows := DayRows{Size: int64(len(file)), Count: rowsIn(chunks)}
	for i, c := range chunks {
		rows.Chunks = append(rows.Chunks, RowsChunk{Member: at[i], Count: c.Count, Meta: c.Meta})
	}
	return file, rows
}

// rowsIn is how many rows chunks hold in all, as their builder counted them.
func rowsIn(chunks []BuiltChunk) int {
	n := 0
	for _, c := range chunks {
		n += c.Count
	}
	return n
}

// writeRows writes file as the rows file of the local day with the given
// date, as writeMembers writes one: removing any there where it's empty, as
// of a day of no rows, so the day holds none, as rowsHeld judges it.
func (d days) writeRows(date string, file []byte) error {
	return writeMembers(rowsFile(d.files.Dir, date), file)
}

// rowsHeld reports whether the rows file of the local day with the given
// date is as long as size, as the day's head records it, as membersHeld
// judges it.
func (d days) rowsHeld(date string, size int64) bool {
	return membersHeld(rowsFile(d.files.Dir, date), size)
}

// rowsStand reports whether the rows head points at stand, as g says, and
// the ledger's files: so a reader resting on them reads what a read of its
// day's lines reads. The head stands, as dayHeadStands says; it points at a
// row of each of the lines it was built of, as their count tells, so a head
// of lines but no rows doesn't; and the day's rows file is as long as the
// head records, as rowsHeld judges it by a look at it alone, as one gone
// isn't, nor one written again since by a build that stopped before its
// head, of other lines. Each chunk is judged as it's read, by its CRC, as
// RowChunk reads it, so one damaged in a file as long reads as none.
func (d days) rowsStand(g digestGrounds, head DayHead) bool {
	return g.dayHeadStands(head) && head.Rows.Count == head.Lines && d.rowsHeld(head.Day, head.Rows.Size)
}

// DayRows returns the rows of the head the ledger holds of the local day
// with the given date, of the version body, its builder's, as the head
// points at them in the day's rows file: reporting false where it holds
// none whose rows stand, as rowsStand says by the follower's own look at the
// day, or whose chunks count otherwise than its rows in all, as counted
// says, so its reader reads the day's lines instead. Each chunk is read as
// RowChunk reads it, and one that doesn't read has its reader read the
// day's lines instead too.
func (f *Follower) DayRows(date string, body int) (DayRows, bool) {
	head, err := f.days.dayHead(date, body)
	if err != nil || !f.days.rowsStand(f.grounds(), head) || !head.Rows.counted() {
		return DayRows{}, false
	}
	return head.Rows, true
}

// RowChunk returns the rows chunk holds, a chunk of the rows of the local
// day with the given date as DayRows gives them, as their builder gave them:
// failing as memberIn fails, as where they don't read as written there, by
// their CRC, as of a rows file damaged, or written again since; and with
// fs.ErrNotExist where the day holds no rows file.
func (f *Follower) RowChunk(date string, chunk RowsChunk) ([]byte, error) {
	rows, err := memberIn(rowsFile(f.days.files.Dir, date), chunk.Member)
	if err != nil {
		return nil, fmt.Errorf("a chunk of the rows of %s: %w", date, err)
	}
	return rows, nil
}
