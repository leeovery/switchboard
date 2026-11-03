package export

import (
	"bufio"
	"cmp"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/leeovery/switchboard/internal/dayfile"
)

// headerRead is how much of a folder's snapshot is read at a time to read
// its header: more than a header holds.
const headerRead = 256

// lineEnding ends each line a compressed day's file holds.
var lineEnding = []byte{'\n'}

// snapshot writes the lines of the day with the given date, whose files
// looked as stat says before they were read, as the day's compressed file in
// the folder, a snapshot of them: not where the folder's is a snapshot of the
// plain file as it is still, as snapshotted says, nor where the day has gone.
// The snapshot's header holds the plain file's size and modification time,
// as markOf marks them, and the file takes the time too. Its lines are those
// snapshotTo writes. The plain file is looked at before its lines are read,
// so a line appended as they're read changes it, and the next export writes
// the day again. Where the day's files can't be read, it fails with an
// unreadable, but where writing into the folder failed too, with why that
// did, as a write that fails stops the export.
func (d dayLines) snapshot(date string, stat dayfile.DayStat) (outcome, error) {
	dst, mark := d.dst.CompressedPath(date), markOf(stat)
	if snapshotted(dst, mark) {
		return asItWas, nil
	}
	var read, written error
	err := writeFile(dst, stat.PlainModified, func(w io.Writer) error {
		if d.wrap != nil {
			w = d.wrap(w)
		}
		read, written = d.snapshotTo(w, date, mark)
		return cmp.Or(written, read)
	})
	switch {
	case written != nil || err != nil && read == nil:
		return asItWas, err
	case errors.Is(read, fs.ErrNotExist):
		return gone, nil
	case read != nil:
		return asItWas, unreadable{read}
	}
	return wrote, nil
}

// snapshotTo writes to w, as a gzip member whose header's comment is mark,
// the lines ReadDay reads of the day with the given date, each once,
// whichever of the day's files holds it, but for a line that doesn't read as
// JSON, as one cut short at the plain file's end, still being written, which
// the next export takes up whole. It returns why reading the lines failed,
// and why writing them did, apart.
func (d dayLines) snapshotTo(w io.Writer, date, mark string) (read, written error) {
	member := gzip.NewWriter(w)
	member.Comment = mark
	_, _, read = dayfile.ReadDay(d.src, date, jsonLine, func(line []byte) bool {
		if _, written = member.Write(line); written == nil {
			_, written = member.Write(lineEnding)
		}
		return written == nil
	})
	if written == nil {
		written = member.Close()
	}
	return read, written
}

// markOf is what a snapshot's header says of the plain file it was taken
// of, as stat looks at it: its size and its modification time, to the
// nanosecond, which the folder's file system needn't keep.
func markOf(stat dayfile.DayStat) string {
	return fmt.Sprintf("switchboard plain %d %d", stat.PlainSize, stat.PlainModified.UnixNano())
}

// snapshotted reports whether the file at path is a snapshot its header
// marks as mark says, reading headerRead bytes of it at a time until its
// header's read: a file in a folder synced from elsewhere may be fetched as
// it's read.
func snapshotted(path, mark string) bool {
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = file.Close() }()
	header, err := gzip.NewReader(bufio.NewReaderSize(file, headerRead))
	return err == nil && header.Comment == mark
}

// jsonLine takes the line, where it reads as JSON, as every line written
// does, but for one cut short.
func jsonLine(line []byte) ([]byte, bool) {
	return line, json.Valid(line)
}
