package dayfile

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"

	"github.com/leeovery/switchboard/internal/atomicfile"
)

// compareBuffer is how much of each of the two things compared at once a
// comparison holds.
const compareBuffer = 32 << 10

// compress moves the lines of the plain file of the local day with the given
// date into the day's compressed file, as addPlain adds them, and then
// removes the plain file. One that fails leaves the plain file.
func (f *Files) compress(date string) error {
	if err := f.addPlain(date); err != nil {
		return err
	}
	return os.Remove(f.path(plainFile(date)))
}

// addPlain adds the lines of the plain file of the local day with the given
// date to the day's compressed file: it writes that afresh, whole, as the one
// there, if any, followed by the plain file's lines it doesn't hold yet, as
// compressedDay.holds says, as adding gives them, a gzip member of their own;
// or leaves it as it is, where it holds them all. It streams the files, and
// holds neither whole.
func (f *Files) addPlain(date string) error {
	plain, err := os.Open(f.path(plainFile(date)))
	if err != nil {
		return err
	}
	defer func() { _ = plain.Close() }()
	info, err := plain.Stat()
	if err != nil {
		return err
	}
	lines := io.NewSectionReader(plain, 0, info.Size())
	path := f.path(compressedFile(date))
	compressed, err := openCompressed(path)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	defer compressed.close()
	held, err := compressed.holds(lines)
	if err != nil {
		return fmt.Errorf("read %s: %w", path, err)
	}
	if held == lines.Size() {
		return nil
	}
	return atomicfile.WriteFrom(path, 0o600, func(w io.Writer) error {
		if _, err := io.Copy(w, compressed.written()); err != nil {
			return err
		}
		return gzipTo(w, compressed.adding(lines, held))
	})
}

// compressedDay is a day's compressed file, open to read, and what's known
// of the lines it gives, its members' one after another: how long they run
// to, where those each member adds start, and whether they end in a line cut
// short. Its file is nil where it isn't there, which gives none.
type compressedDay struct {
	file *os.File
	// size is how long the file runs to, as it's written.
	size int64
	// length is how long its lines run to.
	length int64
	// starts are where the lines each member adds start, in order: after
	// the line ending a member begins with, where that's the one adding put
	// after the lines before it, cut short.
	starts   []int64
	cutShort bool
}

// openCompressed opens the compressed file at path, and reads its members
// through, one at a time, to learn what compressedDay says of its lines.
func openCompressed(path string) (compressedDay, error) {
	file, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return compressedDay{}, nil
	}
	if err != nil {
		return compressedDay{}, err
	}
	c := compressedDay{file: file}
	info, err := file.Stat()
	if err == nil {
		c.size = info.Size()
		err = c.measure()
	}
	if err != nil {
		c.close()
		return compressedDay{}, err
	}
	return c, nil
}

// measure reads the file's members through, one at a time, as the lines they
// give are counted.
func (c *compressedDay) measure() error {
	src := bufio.NewReader(c.written())
	members, err := gzip.NewReader(src)
	if err != nil {
		return err
	}
	var lines span
	for {
		// Reset turns reading the members as one stream back on.
		members.Multistream(false)
		var member span
		if _, err := io.Copy(&member, members); err != nil {
			return err
		}
		start := lines.n
		if lines.cutShort() && member.first == '\n' {
			start++
		}
		c.starts = append(c.starts, start)
		lines.add(member)
		switch err := members.Reset(src); {
		case errors.Is(err, io.EOF):
			c.length, c.cutShort = lines.n, lines.cutShort()
			return nil
		case err != nil:
			return err
		}
	}
}

// holds returns how much of the plain file's lines, in bytes from its start,
// the compressed file's lines hold already, from the first of the places
// heldFrom gives where its lines run on to their end as the plain file's
// start: all of them, or those of its last members; and none where it finds
// no such place.
func (c compressedDay) holds(plain *io.SectionReader) (int64, error) {
	for _, from := range c.heldFrom(plain.Size()) {
		n := c.length - from
		same, err := c.endsWith(from, io.NewSectionReader(plain, 0, n))
		if err != nil {
			return 0, err
		}
		if same {
			return n, nil
		}
	}
	return 0, nil
}

// heldFrom returns the places in the file's lines from which they may run on
// to their end as the plain file's lines, size long, start, in the order
// holds looks at them, the longest first: where they'd end with the whole
// plain file, as when a compress stopped once it had written the compressed
// file, before it removed the plain one; then the start of each member's
// lines from which they'd give less than the whole, the first first, as when
// a line was appended to the plain file after that, once or more, each time
// after another compress stopped so. The earliest comes first: it's the first
// such compresses wrote, and a later one's lines can start the plain file
// too, as a line cut short to a bare start of one, such as {"n":, does, which
// a crash or a full disk leaves.
func (c compressedDay) heldFrom(size int64) []int64 {
	if c.file == nil || size == 0 {
		return nil
	}
	var from []int64
	if size <= c.length {
		from = append(from, c.length-size)
	}
	for _, start := range c.starts {
		if n := c.length - start; n > 0 && n < size {
			from = append(from, start)
		}
	}
	return from
}

// holdsAll reports whether a day's compressed file's lines, compressed, hold
// every one of its plain file's, plain: they end with them, the first place
// holds looks.
func holdsAll(compressed, plain []byte) bool {
	return bytes.HasSuffix(compressed, plain)
}

// endsWith reports whether the file's lines from from on are those want
// gives, as long as they run to.
func (c compressedDay) endsWith(from int64, want *io.SectionReader) (bool, error) {
	members, err := gzip.NewReader(bufio.NewReader(c.written()))
	if err != nil {
		return false, err
	}
	if _, err := io.CopyN(io.Discard, members, from); err != nil {
		return false, err
	}
	return sameFor(members, want, want.Size())
}

// adding returns the plain file's lines compressing the day adds to the
// compressed file, those after the first held, which it holds already: those
// run on from them, as the plain file has them, and where none are held, the
// whole plain file's, after a line ending where the compressed file's lines
// end in one cut short, so the first of them starts a line of its own. The
// compressed file's lines still end with the plain file's then, as holds
// looks for.
func (c compressedDay) adding(plain *io.SectionReader, held int64) io.Reader {
	rest := io.NewSectionReader(plain, held, plain.Size()-held)
	if held == 0 && c.cutShort {
		return io.MultiReader(strings.NewReader("\n"), rest)
	}
	return rest
}

// written reads the file as it's written, from its start.
func (c compressedDay) written() io.Reader {
	if c.file == nil {
		return bytes.NewReader(nil)
	}
	return io.NewSectionReader(c.file, 0, c.size)
}

// close closes the file, where it's open.
func (c compressedDay) close() {
	if c.file != nil {
		_ = c.file.Close()
	}
}

// span is the lines written to it, as how many bytes they run to, and the
// first of them and the last.
type span struct {
	n           int64
	first, last byte
}

func (s *span) Write(p []byte) (int, error) {
	if len(p) > 0 {
		s.add(span{n: int64(len(p)), first: p[0], last: p[len(p)-1]})
	}
	return len(p), nil
}

// add takes in the lines of o, after its own.
func (s *span) add(o span) {
	if o.n == 0 {
		return
	}
	if s.n == 0 {
		s.first = o.first
	}
	s.n, s.last = s.n+o.n, o.last
}

// cutShort reports whether the lines counted end in a line cut short, as
// cutShort says of lines.
func (s span) cutShort() bool {
	return s.n > 0 && s.last != '\n'
}

// sameFor reports whether a and b give the same n bytes, holding no more
// than compareBuffer of each at once.
func sameFor(a, b io.Reader, n int64) (bool, error) {
	bufA, bufB := make([]byte, compareBuffer), make([]byte, compareBuffer)
	for n > 0 {
		chunk := min(n, compareBuffer)
		if _, err := io.ReadFull(a, bufA[:chunk]); err != nil {
			return false, err
		}
		if _, err := io.ReadFull(b, bufB[:chunk]); err != nil {
			return false, err
		}
		if !bytes.Equal(bufA[:chunk], bufB[:chunk]) {
			return false, nil
		}
		n -= chunk
	}
	return true, nil
}

// gzipTo writes what lines gives to w, compressed, as a gzip member of its
// own.
func gzipTo(w io.Writer, lines io.Reader) error {
	member := gzip.NewWriter(w)
	_, err := io.Copy(member, lines)
	return errors.Join(err, member.Close())
}
