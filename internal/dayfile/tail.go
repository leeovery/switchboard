package dayfile

import (
	"bytes"
	"io"
	"io/fs"
	"os"
)

// lookBack is how much of a plain file a Tail reads back through at a time,
// from its end, for the end of its last line.
const lookBack = 4096

// Tail reads the lines of a local day's files as a tail reads a file: whole,
// then, at each read after, only what the day's plain file gained since the
// last read ended, from the end of the last line it read whole, as lines are
// only ever appended to it. Of a plain file read on as it grows, a last line
// cut short, without its line ending, as one still being written, is counted
// as ReadDay counts it, and given by Cut rather than handed on, as it's read
// again at each read until it's whole. Where the day's files changed
// otherwise since the last read, the plain file replaced, or shorter than
// where the last read ended, the day compressed, or pruned, or a compressed
// file beside its plain one, as after the clock was set back to it, which is
// read whole again at any change, the files are read afresh, from their
// start, as ReadDay reads them. A Tail isn't safe for concurrent use.
type Tail[T any] struct {
	files  *Files
	date   string
	decode func(line []byte) (T, bool)
	// read is set once a read ended well, the day's files then as plain and
	// compressed looked, nil where one wasn't there, so the next read goes on
	// from it. following is set where the plain file was the day's only one,
	// read as far as end, the end of its last line read whole.
	read              bool
	plain, compressed fs.FileInfo
	following         bool
	end               int64
	// whole counts the lines read whole, and cut the last line cut short, held
	// in last where it read as a T.
	whole, cut counted
	last       T
	hasLast    bool
}

// counted is how many lines were read, and how many of those were unread, as
// ReadDay counts them.
type counted struct {
	lines, unread int
}

// NewTail returns a Tail of the files of the local day with the given date,
// which decode makes a T of each line of, as ReadDay says, and none read yet.
func NewTail[T any](f *Files, date string, decode func(line []byte) (T, bool)) *Tail[T] {
	return &Tail[T]{files: f, date: date, decode: decode}
}

// Read hands take each line the day's files gained since the last read ended
// that decode makes a T of, in the order they came: where it reads the files
// afresh, from their start, as it does the first time, it calls afresh first,
// for what take was handed before to be let go of. It reports false where one
// of the files couldn't be looked at or opened, which is warned of as Read
// says: what could be read is handed on all the same, and the next read reads
// them afresh. A damaged file is read up to the damage, warned of as Read
// says.
func (t *Tail[T]) Read(afresh func(), take func(T)) bool {
	t.files.mu.RLock()
	defer t.files.mu.RUnlock()
	plain, compressed, ok := t.look()
	switch {
	case !ok:
		t.read = false
		return false
	case t.unchanged(plain, compressed):
		return true
	case t.grew(plain, compressed) && t.readOn(take):
		return true
	}
	afresh()
	return t.readAfresh(take)
}

// Lines returns how many lines the day's files held as the last read ended,
// and how many of those were unread, as ReadDay counts them: the last line,
// cut short, as Cut gives it, among them.
func (t *Tail[T]) Lines() (lines, unread int) {
	return t.whole.lines + t.cut.lines, t.whole.unread + t.cut.unread
}

// Cut returns the line the plain file ended in as the last read ended, cut
// short, where it read as a T, reporting false where there's none: one that
// isn't handed on until it's whole.
func (t *Tail[T]) Cut() (T, bool) {
	return t.last, t.hasLast
}

// look looks at the day's plain file and compressed file, reading neither:
// nil where one isn't there. It reports false where one can't be looked at,
// which is warned of.
func (t *Tail[T]) look() (plain, compressed fs.FileInfo, ok bool) {
	for _, file := range []dayFile{plainFile(t.date), compressedFile(t.date)} {
		info, err := t.files.stat(file)
		if err != nil {
			t.files.warn("can't read the "+t.files.Name, file, err)
			return nil, nil, false
		}
		if file.compressed {
			compressed = info
		} else {
			plain = info
		}
	}
	return plain, compressed, true
}

// unchanged reports whether the day's files, as plain and compressed look,
// are as the last read, which ended well, left them, as sameAs says.
func (t *Tail[T]) unchanged(plain, compressed fs.FileInfo) bool {
	return t.read && sameAs(t.plain, plain) && sameAs(t.compressed, compressed)
}

// grew reports whether the day's plain file, as plain looks, is the one the
// last read followed, the day's only file still, and longer than it was.
func (t *Tail[T]) grew(plain, compressed fs.FileInfo) bool {
	return t.read && t.following && compressed == nil && plain != nil && os.SameFile(t.plain, plain) && plain.Size() > t.plain.Size()
}

// sameAs reports whether a file that looked as was, nil where it wasn't
// there, looks as it did as now does: the same file, as long, and last
// modified at the same time; or not there still.
func sameAs(was, now fs.FileInfo) bool {
	if was == nil || now == nil {
		return was == nil && now == nil
	}
	return os.SameFile(was, now) && was.Size() == now.Size() && was.ModTime().Equal(now.ModTime())
}

// readOn reads the plain file on from where the last read ended, as follow
// does, where it's still the file the last read followed, no shorter, as when
// it opens it: it reports false, having read none of it, where it's not, as
// when it was compressed since it was looked at.
func (t *Tail[T]) readOn(take func(T)) bool {
	file, err := os.Open(t.files.path(plainFile(t.date)))
	if err != nil {
		return false
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil || !os.SameFile(info, t.plain) || info.Size() < t.plain.Size() {
		return false
	}
	return t.follow(file, info, take)
}

// readAfresh reads the day's files from their start, as ReadDay does,
// handing take each line: the plain file, where it's the day's only one, as
// follow reads it, so the next read goes on from where it ended. It reports
// false where one of the files couldn't be opened, or looked at once opened,
// which is warned of.
func (t *Tail[T]) readAfresh(take func(T)) bool {
	*t = Tail[T]{files: t.files, date: t.date, decode: t.decode}
	plain, failed := t.files.openOne(plainFile(t.date), nil)
	compressed, failed := t.files.openOne(compressedFile(t.date), failed)
	plainInfo, failed := openedInfo(plain, failed)
	compressedInfo, failed := openedInfo(compressed, failed)
	if len(failed) == 0 && plain != nil && compressed == nil {
		defer func() { _ = plain.Close() }()
		return t.follow(plain.src, plainInfo, take)
	}
	for _, e := range failed {
		t.files.warn("can't read the "+t.files.Name, e.file, e.err)
	}
	lines, unread := readOpened(t.files, dayOpened(plain, compressed), t.decode, handing(take))
	t.whole = counted{lines: lines, unread: unread}
	t.read, t.plain, t.compressed = len(failed) == 0, plainInfo, compressedInfo
	return t.read
}

// openedInfo looks at file, once opened, adding why it can't be to failed:
// nil where it wasn't there.
func openedInfo(file *openFile, failed []fileError) (fs.FileInfo, []fileError) {
	if file == nil {
		return nil, failed
	}
	info, err := file.src.Stat()
	if err != nil {
		return nil, append(failed, fileError{file: file.dayFile, err: err})
	}
	return info, failed
}

// follow hands take each line the plain file, the day's only one, opened as
// file and looked at as info, holds whole from end on, and holds the line it
// ends in, cut short, apart, as Cut gives it: so the next read goes on from
// the end of its last line read whole. It reports false where it can't find
// that end, which is warned of.
func (t *Tail[T]) follow(file *os.File, info fs.FileInfo, take func(T)) bool {
	size := info.Size()
	end, err := lastLineEnd(file, t.end, size)
	if err != nil {
		t.files.warn(t.files.Name+" read short", plainFile(t.date), err)
		t.read = false
		return false
	}
	lines, unread := readOpened(t.files, []openFile{t.section(file, t.end, end)}, t.decode, handing(take))
	t.whole.lines, t.whole.unread = t.whole.lines+lines, t.whole.unread+unread
	t.last, t.hasLast = *new(T), false
	lines, unread = readOpened(t.files, []openFile{t.section(file, end, size)}, t.decode, func(v T) bool {
		t.last, t.hasLast = v, true
		return true
	})
	t.cut = counted{lines: lines, unread: unread}
	t.read, t.following, t.plain, t.compressed, t.end = true, true, info, nil, end
	return true
}

// section is the plain file, opened as file, from from up to to, as one of
// the day's files opened to read the lines it holds.
func (t *Tail[T]) section(file *os.File, from, to int64) openFile {
	return openFile{dayFile: plainFile(t.date), ReadCloser: io.NopCloser(io.NewSectionReader(file, from, to-from))}
}

// handing has take, which wants every line, take each as Read's do.
func handing[T any](take func(T)) func(T) bool {
	return func(v T) bool {
		take(v)
		return true
	}
}

// lastLineEnd returns where the last line of what r holds from from up to to
// that ends in a line ending ends, just after its line feed: from, where
// none does.
func lastLineEnd(r io.ReaderAt, from, to int64) (int64, error) {
	var buf [lookBack]byte
	for to > from {
		chunk := buf[:min(to-from, lookBack)]
		start := to - int64(len(chunk))
		if _, err := r.ReadAt(chunk, start); err != nil {
			return from, err
		}
		if i := bytes.LastIndexByte(chunk, '\n'); i >= 0 {
			return start + int64(i) + 1, nil
		}
		to = start
	}
	return from, nil
}
