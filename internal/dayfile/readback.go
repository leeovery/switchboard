package dayfile

import (
	"io"
	"slices"

	"github.com/leeovery/switchboard/internal/linescan"
)

// backStretch is how far back from where it ends, at least, each stretch of
// a plain file's lines ReadBack reads reaches: to the start of the line
// there, so a stretch holds whole lines.
const backStretch = 64 << 10

// ReadBack hands take each line the plain file of the local day with the
// given date holds that decode makes a T of, the last first, as ReadDay
// would hand them turned about, until take reports false, and returns how
// many lines the file holds, as far back as it read, and how many of those
// were unread, as ReadDay counts them: decode is handed each line as ReadDay
// hands it, and mustn't keep it. It reads the file as it was as it opened it,
// so a line appended after is never handed, and one it ended in then, cut
// short, is handed as ReadDay would hand it, as the file's last. It reads the
// file back a stretch of whole lines at a time, of 64 KiB or so, passing over
// a line too long to hold without holding it, as ReadDay does. It reports
// false, reading nothing, where the day has a compressed file, beside its
// plain one or alone, or one that can't be looked at: the day is read
// forward, as ReadDay reads it, then. Where the plain file can't be opened,
// it fails as ReadDay does, and with fs.ErrNotExist where the day has no
// files. A file damaged is read back as far as the stretch of lines holding
// the damage, warned of as Read says.
func ReadBack[T any](f *Files, date string, decode func(line []byte) (T, bool), take func(T) bool) (lines, unread int, ok bool, err error) {
	f.mu.RLock()
	defer f.mu.RUnlock()
	plain, failed := f.openOne(plainFile(date), nil)
	if plain != nil {
		defer func() { _ = plain.Close() }()
	}
	size, ok, err := f.plainAlone(date, plain, failed)
	if !ok || err != nil {
		return 0, 0, ok, err
	}
	lines, unread = readBackOpened(f, plain.dayFile, plain.src, size, decode, take)
	return lines, unread, true, nil
}

// plainAlone returns how long the plain file of the local day with the given
// date was as it was opened, as plain, nil where it isn't there, failed
// saying why where it couldn't be. It reports false where the day has a
// compressed file too, or one that can't be looked at, or the plain file
// can't be looked at once opened, which ReadDay has no need to: the day is
// read forward then. It fails where the plain file couldn't be opened, as
// ReadDay does, and with fs.ErrNotExist where the day has no files. It looks
// for the compressed file once the plain file is opened, as openDay opens
// them, so a day another process compresses meanwhile is never taken for one
// without files.
func (f *Files) plainAlone(date string, plain *openFile, failed []fileError) (size int64, ok bool, err error) {
	compressed, err := f.stat(compressedFile(date))
	switch {
	case compressed != nil || err != nil:
		return 0, false, nil
	case len(failed) > 0:
		return 0, true, f.failure(failed)
	case plain == nil:
		return 0, true, f.noLines(date)
	}
	info, err := plain.src.Stat()
	if err != nil {
		return 0, false, nil
	}
	return info.Size(), true, nil
}

// readBackOpened hands take each line the file r reads holds up to size that
// decode makes a T of, the last first, until take reports false, and returns
// how many lines it holds, as far back as it read, and how many of those were
// unread, as readOpened counts them.
func readBackOpened[T any](f *Files, file dayFile, r io.ReaderAt, size int64, decode func(line []byte) (T, bool), take func(T) bool) (lines, unread int) {
	var read tally
	read.add(f.readBack(file, r, size, decoding(&read, decode, take)))
	return read.counts()
}

// readBack hands take each line the file r reads holds up to size, the last
// first, without its line ending, until take reports false, and returns how
// many of the lines it handed take, and how many it skipped as too long to
// hold, as LineMax says, without holding them, after the last of those. A
// file damaged is read back as far as the stretch of lines holding the
// damage, warned of as Read says.
func (f *Files) readBack(file dayFile, r io.ReaderAt, size int64, take func(line []byte) bool) (handed, long int) {
	var held stretch
	var err error
	for end := size; end > 0; {
		if end, err = held.readBefore(r, end, f.LineMax); err != nil {
			break
		}
		h, l, more := held.handBack(take)
		handed, long = handed+h, long+l
		if !more {
			return handed, long
		}
	}
	f.readEnded(file, err)
	return handed, long
}

// stretch is the lines of a stretch of a file, from the start of a line to
// the start of another, or the file's end, held as ReadDay reads them, to be
// handed back the last first. text holds them one after another, without
// their line endings, each ending where ends says, after as many lines
// skipped as too long to hold as passed says; and long counts those skipped
// in all.
type stretch struct {
	text   []byte
	ends   []int
	passed []int
	long   int
}

// readBefore holds the lines of what r holds before end, the start of a line
// or r's end, from the start of the line backStretch before it, or r's
// start, in place of those it held, and returns where they start, and why
// they couldn't be read, if they couldn't. A line is held only as far as
// most, as ReadDay holds it.
func (s *stretch) readBefore(r io.ReaderAt, end int64, most int) (start int64, err error) {
	start, err = lastLineEnd(r, 0, max(end-backStretch, 0))
	if err != nil {
		return start, err
	}
	lines := measuredSection(r, start, end)
	if err := s.read(lines, most); err != nil {
		return start, err
	}
	if lines.short() {
		return start, io.ErrUnexpectedEOF
	}
	return start, nil
}

// read holds the lines r gives, as linescan reads them, holding at most most
// of a line, in place of those it held, and returns why reading them failed,
// if it did.
func (s *stretch) read(r io.Reader, most int) error {
	s.text, s.ends, s.passed = s.text[:0], s.ends[:0], s.passed[:0]
	lines := linescan.New(r, most)
	for lines.Scan() {
		s.text = append(s.text, lines.Bytes()...)
		s.ends = append(s.ends, len(s.text))
		s.passed = append(s.passed, lines.Long())
	}
	s.long = lines.Long()
	return lines.Err()
}

// handBack hands take the lines held, the last first, until it reports
// false, and returns how many it handed, how many lines it skipped as too
// long to hold after the last of those, and whether take wanted more.
func (s *stretch) handBack(take func(line []byte) bool) (handed, long int, more bool) {
	after := s.long
	for i, passed := range slices.Backward(s.passed) {
		long, after = long+after-passed, passed
		handed++
		if !take(s.line(i)) {
			return handed, long, false
		}
	}
	return handed, long + after, true
}

// line returns the i-th line held.
func (s *stretch) line(i int) []byte {
	start := 0
	if i > 0 {
		start = s.ends[i-1]
	}
	return s.text[start:s.ends[i]:s.ends[i]]
}
