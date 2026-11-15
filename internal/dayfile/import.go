package dayfile

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"encoding/json/jsontext"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"slices"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/linescan"
)

// lineEnding ends each line added to a day's files.
var lineEnding = []byte{'\n'}

// Taken is how a day staged to import was taken in.
type Taken int

const (
	// Added is a day the files held none of: its file copied in whole, or
	// its lines appended to its plain file, as take says.
	Added Taken = iota + 1
	// Merged is a day the files held: the lines it didn't hold added.
	Merged
	// PastKeep is a day past keeping, left out, as the next prune would
	// remove it.
	PastKeep
)

// Imported is what taking a day staged to import in did: how the day was
// taken, and, of a day Merged, how many lines it gained. Its zero value is a
// day the files held none of whose staged file held no line to take in.
type Imported struct {
	Taken Taken
	Lines int
}

// Import takes in the file at staged, which holds the lines of the local day
// with the given date as export writes a day's, a gzip member or more of
// lines, as ImportThen does, with nothing to do after.
func (w *Writer[T]) Import(ctx context.Context, date, staged string) (Imported, error) {
	return w.ImportThen(ctx, date, staged, func(Imported) error { return nil })
}

// ImportThen takes in the file at staged, which holds the lines of the local
// day with the given date as export writes a day's, a gzip member or more of
// lines, by the Writer's own keep and clock, as take says, then does then,
// given what it took in, the two as one piece of work on Run's goroutine, as
// Do does work: so the Writer does nothing else between them, and then sees
// its files as the take left them. Only the Writer is held off: then may see
// what other goroutines did meanwhile, as to files beside its own. Work
// taken up runs to its end, which leaves both done, or neither, though a
// caller whose context ends first returns before it does, as Do says. It
// returns what it took in, failing where the day couldn't be, doing none of
// then, and with then's error; or, where it didn't wait for that, why, as Do
// says: ctx's error, or ErrStopped.
func (w *Writer[T]) ImportThen(ctx context.Context, date, staged string, then func(Imported) error) (Imported, error) {
	day, _, ok := Day(date)
	if !ok {
		return Imported{}, fmt.Errorf("can't import %s: %q isn't a date", staged, date)
	}
	var imported Imported
	var took, did error
	if done := w.Do(ctx, func() {
		if imported, took = w.files.take(stagedDay{date: date, day: day, path: staged}, w.now(), w.keep); took == nil {
			did = then(imported)
		}
	}); done != nil {
		return Imported{}, done
	}
	switch {
	case took != nil:
		return Imported{}, fmt.Errorf("import the %s of %s: %w", w.files.Name, date, took)
	case did != nil:
		return Imported{}, did
	}
	return imported, nil
}

// stagedDay is a day's file staged to import: the date of its local day,
// when that starts, and where the file is.
type stagedDay struct {
	date string
	day  time.Time
	path string
}

// take takes the staged day in at now, the files kept as long as keep says.
// A day past keeping, as a prune tells one, is left out. A day the files
// hold none of has the staged file copied in whole, as copyIn copies it,
// where it's done with, as a compress tells one, and the file reads to its
// end; else its lines appended to its plain file, as addLines adds them,
// which a prune compresses as it does any. A day the files hold has the
// staged lines it doesn't hold, as ReadDay reads them, added, as addLines
// adds them: appended to its plain file where it has one, else as a gzip
// member of their own after its compressed file's.
func (f *Files) take(s stagedDay, now time.Time, keep time.Duration) (Imported, error) {
	if pastKeepingAt(s.day, now, keep) {
		return Imported{Taken: PastKeep}, nil
	}
	held, err := f.heldLines(s.date)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return f.add(s, now)
	case err != nil:
		return Imported{}, err
	}
	return f.merge(s, held)
}

// merge merges the staged day into the files, which hold the lines held of
// it, as take says. Where the day's own compressed file doesn't read, as one
// cut short, so no member can be added after it, the day is passed over,
// taking nothing in, and warned of, as a compress passes it over, so the
// rest of an import goes on.
func (f *Files) merge(s stagedDay, held lineCounts) (Imported, error) {
	stat, err := f.Stat(s.date)
	if err != nil {
		return Imported{}, err
	}
	to := f.addingMember(s.date)
	if !stat.PlainModified.IsZero() {
		to = f.appending(s.date)
	}
	added, err := f.addLines(s.path, held, to)
	if u, ok := errors.AsType[heldUnread](err); ok {
		f.Logger.Warn("can't import into a day of the "+f.Name+" whose own compressed file doesn't read; it's passed over", "day", s.date, "error", u.error)
		return Imported{}, nil
	}
	if err != nil {
		return Imported{}, err
	}
	return Imported{Taken: Merged, Lines: added}, nil
}

// heldUnread is why a day's own compressed file couldn't be read, so no
// member can be added after it.
type heldUnread struct{ error }

// add adds the staged day, which the files hold none of, at now, as take
// says.
func (f *Files) add(s stagedDay, now time.Time) (Imported, error) {
	if doneWithAt(s.day, now) {
		copied, err := f.copyIn(s)
		if err != nil {
			return Imported{}, err
		}
		if copied {
			return Imported{Taken: Added}, nil
		}
	}
	added, err := f.addLines(s.path, make(lineCounts), f.appending(s.date))
	if err != nil || added == 0 {
		return Imported{}, err
	}
	return Imported{Taken: Added}, nil
}

// copyIn copies the staged file in whole, byte for byte, as the compressed
// file of its day, last modified when it was, beside where it goes and
// renamed into place, as compressing a day writes one, reporting whether it
// did. It copies none that doesn't read to its end, as measure reads a
// compressed file through, as one cut short, which the day would read short
// for good, a member never to be added after the damage; nor one that holds
// no lines, as one empty: either is left to addLines, which takes in the lines
// that read, warning of the rest, and fails where the file can't be opened.
func (f *Files) copyIn(s stagedDay) (bool, error) {
	staged, err := openCompressed(s.path)
	if err != nil {
		return false, nil
	}
	defer staged.close()
	if staged.length == 0 {
		return false, nil
	}
	info, err := staged.file.Stat()
	if err != nil {
		return false, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := os.MkdirAll(f.Dir, 0o700); err != nil {
		return false, err
	}
	err = atomicfile.WriteModified(f.path(compressedFile(s.date)), 0o600, info.ModTime(), func(w io.Writer) error {
		_, err := io.Copy(w, staged.written())
		return err
	})
	return err == nil, err
}

// lineCounts counts lines, held by the hashes of their keys, as keyOf gives
// them: none of the lines themselves, as a day's can run to tens of
// megabytes.
type lineCounts map[[sha256.Size]byte]int

// add counts the line of the given key once more.
func (c lineCounts) add(key [sha256.Size]byte) {
	c[key]++
}

// take counts the line of the given key once less, reporting whether it was
// counted at all.
func (c lineCounts) take(key [sha256.Size]byte) bool {
	if c[key] == 0 {
		return false
	}
	c[key]--
	return true
}

// keyOf returns the hash of line's key, as the files' Key gives it: of the
// line itself where they have none.
func (f *Files) keyOf(line []byte) [sha256.Size]byte {
	if f.Key != nil {
		line = f.Key(line)
	}
	return sha256.Sum256(line)
}

// Without returns a Key, as Files have one, that's a line without its member
// of the given name, one of the JSON object's own, the rest's bytes as they
// were: so two lines alike but for that member, or for whether they have it,
// are one. A line without one, or that doesn't read as one JSON object, is
// its own.
func Without(name string) func(line []byte) []byte {
	return func(line []byte) []byte {
		from, to, ok := memberAt(line, name)
		if !ok {
			return line
		}
		return slices.Concat(line[:from], line[to:])
	}
}

// memberAt returns where line, a JSON object, holds its first member of the
// given name, with the comma that parts it from the one before, or, where
// it's the object's first, from the one after, so the rest is the object
// without it: reporting false where it has none, or doesn't read as one JSON
// object.
func memberAt(line []byte, name string) (from, to int64, ok bool) {
	dec := jsontext.NewDecoder(bytes.NewReader(line), jsontext.AllowDuplicateNames(true), jsontext.AllowInvalidUTF8(true))
	if open, err := dec.ReadToken(); err != nil || open.Kind() != '{' {
		return 0, 0, false
	}
	for i, prev := 0, dec.InputOffset(); dec.PeekKind() != '}'; i, prev = i+1, dec.InputOffset() {
		token, err := dec.ReadToken()
		if err != nil {
			return 0, 0, false
		}
		key := token.String()
		if _, err := dec.ReadValue(); err != nil {
			return 0, 0, false
		}
		if !ok && key == name {
			from, to, ok = prev, dec.InputOffset(), true
			if i == 0 {
				to = pastComma(line, to)
			}
		}
	}
	if _, err := dec.ReadToken(); err != nil {
		return 0, 0, false
	}
	if _, err := dec.ReadToken(); !errors.Is(err, io.EOF) {
		return 0, 0, false
	}
	return from, to, ok
}

// pastComma returns the offset in line, JSON, past the comma that follows at,
// whitespace aside, where one does: else at.
func pastComma(line []byte, at int64) int64 {
	rest := bytes.TrimLeft(line[at:], " \t\r\n")
	if len(rest) > 0 && rest[0] == ',' {
		return int64(len(line)-len(rest)) + 1
	}
	return at
}

// heldLines returns the lines the files of the local day with the given date
// hold, by their keys, as keyOf gives them, each as many times as they hold
// it, as ReadDay reads them, failing as it does: with fs.ErrNotExist where
// the day has none.
func (f *Files) heldLines(date string) (lineCounts, error) {
	held := make(lineCounts)
	_, _, err := ReadDay(f, date, func(line []byte) (struct{}, bool) {
		held.add(f.keyOf(line))
		return struct{}{}, true
	}, func(struct{}) bool { return true })
	return held, err
}

// sink adds lines to a day's files: what write writes, to the writer it's
// handed.
type sink func(write func(w io.Writer) error) error

// appending is the sink that appends to the plain file of the local day with
// the given date, as Append does, making the file's directory, private,
// should it have gone.
func (f *Files) appending(date string) sink {
	return func(write func(w io.Writer) error) error {
		f.mu.RLock()
		defer f.mu.RUnlock()
		if err := os.MkdirAll(f.Dir, 0o700); err != nil {
			return err
		}
		return appendWith(f.path(plainFile(date)), func(file io.Writer) error {
			buffered := bufio.NewWriter(file)
			if err := write(buffered); err != nil {
				return err
			}
			return buffered.Flush()
		})
	}
}

// addingMember is the sink that adds a gzip member of its own after the
// members of the compressed file of the local day with the given date, as
// compressing the day adds one, its first line starting a line of its own.
// It fails with heldUnread where the compressed file doesn't read.
func (f *Files) addingMember(date string) sink {
	return func(write func(w io.Writer) error) error {
		f.mu.Lock()
		defer f.mu.Unlock()
		path := f.path(compressedFile(date))
		compressed, err := openCompressed(path)
		if err != nil {
			return heldUnread{fmt.Errorf("read %s: %w", path, err)}
		}
		defer compressed.close()
		return compressed.addMember(path, func(member io.Writer) error {
			if _, err := io.Copy(member, compressed.lineStart()); err != nil {
				return err
			}
			return write(member)
		})
	}
}

// addLines adds, through to, each line of the staged file at path, as
// readStaged hands them, but as many of each as held counts of its key, as
// keyOf gives it, which it takes from held, and returns how many it added: a
// line staged more times than the day held it is added the times more, as an
// event that changed back to how it stood has that version written whole
// again. A line held stands, a staged one of its key passed over. Where
// there's none to add, it leaves the files as they are.
func (f *Files) addLines(path string, held lineCounts, to sink) (int, error) {
	if more, err := f.anyNew(path, maps.Clone(held)); err != nil || !more {
		return 0, err
	}
	added := 0
	err := to(func(w io.Writer) error {
		var written error
		read := f.readStaged(path, func(line []byte) bool {
			if held.take(f.keyOf(line)) {
				return true
			}
			if _, written = w.Write(line); written == nil {
				_, written = w.Write(lineEnding)
			}
			if written == nil {
				added++
			}
			return written == nil
		})
		return errors.Join(read, written)
	})
	return added, err
}

// anyNew reports whether the staged file at path has a line to add, as
// addLines adds them, taking those it passes over from held.
func (f *Files) anyNew(path string, held lineCounts) (bool, error) {
	found := false
	err := f.readStaged(path, func(line []byte) bool {
		found = !held.take(f.keyOf(line))
		return !found
	})
	return found, err
}

// readStaged hands take each line of the staged file at path that reads as
// JSON, as every line written whole does, without its line ending, until take
// reports false, passing over each that doesn't, which appended to a plain
// file would run into the next, and those too long to hold, as ReadDay does.
// It reads the file's members as one stream, and one damaged, as a file cut
// short, up to the damage, which is warned of. It fails where the file can't
// be opened.
func (f *Files) readStaged(path string, take func(line []byte) bool) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	if err := eachJSON(file, f.LineMax, take); err != nil {
		f.Logger.Warn(f.Name+" staged to import read short", "file", path, "error", err)
	}
	return nil
}

// eachJSON hands take each line of what src gives through gzip that reads as
// JSON, without its line ending, until take reports false, passing over each
// that runs to more than most with its line ending, and returns why the read
// ended short, if it did.
func eachJSON(src io.Reader, most int, take func(line []byte) bool) error {
	members, err := gzip.NewReader(src)
	if err != nil {
		return err
	}
	lines := linescan.New(members, most)
	for lines.Scan() {
		if line := lines.Bytes(); json.Valid(line) && !take(line) {
			return nil
		}
	}
	return lines.Err()
}
