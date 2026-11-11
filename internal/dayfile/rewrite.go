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

	"github.com/leeovery/switchboard/internal/atomicfile"
)

// rewriteTries is how many times Rewrite starts a day's files afresh, where
// they change otherwise than by lines appended as it rewrites them, as a
// compress or a prune changes them, before it gives up.
const rewriteTries = 5

// Rewritten is what rewriting a day's files did: whether any of their lines
// changed, and a look at them just before they were renamed into place and
// just after, as Stat gives one: both zero where the day has neither file,
// and alike where nothing changed.
type Rewritten struct {
	Changed       bool
	Before, After DayStat
}

// Rewrite writes the files of the local day with the given date anew, each
// line as rewrite makes it, given without its line ending, beside each file,
// and renamed into place: a plain file as a plain file, a compressed one as a
// compressed file of one gzip member. A file none of whose lines changes is
// left as it is. A line cut short at a plain file's end is handed rewrite
// too, and written without its line ending, so a writer ends it before it
// appends, as it ends any.
//
// The files are read and written beside without holding them, so appends,
// prunes and compresses go on meanwhile, and held only for the moment each is
// checked and renamed: lines appended to a plain file meanwhile are rewritten
// too, under the hold, and written after those before them, so none is lost.
// Where a file changed otherwise meanwhile, replaced, compressed or pruned,
// it starts afresh, rewriteTries times at most, failing after. A day with
// both files, as a compress cut short leaves one, is compressed first, under
// the hold, as a prune would compress it, so a stop between the two files'
// renames can never leave one rewritten beside the other unrewritten, which
// the next compress would take for lines it doesn't hold. A day with neither
// file is left alone. A compressed file that's damaged, as one cut short, is
// repaired first, as repairHeld repairs it: its whole lines before the
// damage, which readers read, kept in its place, and the file itself set
// aside; so the day's every line is rewritten, and none left as it was to be
// read beside those rewritten.
func (f *Files) Rewrite(date string, rewrite func(line []byte) []byte) (Rewritten, error) {
	for range rewriteTries {
		r, settled, err := f.rewriteOnce(date, rewrite)
		if errors.Is(err, ErrDamaged) {
			r, settled, err = Rewritten{}, false, f.repairHeld(date)
		}
		if err != nil {
			return Rewritten{}, fmt.Errorf("rewrite the %s of %s: %w", f.Name, date, err)
		}
		if settled {
			return r, nil
		}
	}
	return Rewritten{}, fmt.Errorf("rewrite the %s of %s: its files kept changing as they were rewritten", f.Name, date)
}

// rewriteOnce rewrites the day's files once, as Rewrite says, reporting
// whether it settled them: not where they changed otherwise than by lines
// appended meanwhile, or held both files, which it compresses into one.
func (f *Files) rewriteOnce(date string, rewrite func([]byte) []byte) (Rewritten, bool, error) {
	look, err := f.Stat(date)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return Rewritten{}, true, nil
	case err != nil:
		return Rewritten{}, false, err
	case !look.PlainModified.IsZero() && !look.CompressedModified.IsZero():
		return Rewritten{}, false, f.compressHeld(date)
	}
	file := plainFile(date)
	if look.PlainModified.IsZero() {
		file = compressedFile(date)
	}
	staged, err := f.stageRewrite(file, rewrite)
	if errors.Is(err, fs.ErrNotExist) {
		return Rewritten{}, false, nil
	}
	if err != nil {
		return Rewritten{}, false, err
	}
	defer staged.discard()
	if f.rewriting != nil {
		f.rewriting(file)
	}
	return f.finishRewrite(date, file, staged, rewrite)
}

// compressHeld compresses the day's files, holding them, as a prune does.
func (f *Files) compressHeld(date string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.compress(date); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// stagedRewrite is one of a day's files read and rewritten beside, as
// stageRewrite stages it: the file, still open, and how it looked as it was
// opened; where it was read to, the end of its last line read whole, of a
// plain file, or its size; the file written beside it; and whether any line
// changed.
type stagedRewrite struct {
	src     *os.File
	info    fs.FileInfo
	end     int64
	tmp     string
	changed bool
}

// stageRewrite reads file and writes it anew beside, as Rewrite says, holding
// nothing: a plain file up to the end of its last line read whole, a
// compressed one whole.
func (f *Files) stageRewrite(file dayFile, rewrite func([]byte) []byte) (*stagedRewrite, error) {
	src, err := os.Open(f.path(file))
	if err != nil {
		return nil, err
	}
	info, err := src.Stat()
	if err != nil {
		_ = src.Close()
		return nil, err
	}
	s := &stagedRewrite{src: src, info: info}
	if s.tmp, err = atomicfile.Stage(f.path(file), 0o600, func(w io.Writer) error { return s.write(file, w, rewrite) }); err != nil {
		_ = src.Close()
		return nil, err
	}
	return s, nil
}

// write writes the file's lines, as rewrite makes them, to w: a compressed
// file's through gzip, whole, a plain file's up to the end of its last line
// read whole, which it notes as where it was read to.
func (s *stagedRewrite) write(file dayFile, w io.Writer, rewrite func([]byte) []byte) error {
	if !file.compressed {
		var err error
		s.end, s.changed, err = rewriteLines(bufio.NewReader(s.src), w, rewrite, false)
		return err
	}
	s.end = s.info.Size()
	members, err := gzip.NewReader(bufio.NewReader(s.src))
	if err != nil {
		return damaged(err)
	}
	return gzipTo(w, func(member io.Writer) error {
		var err error
		_, s.changed, err = rewriteLines(damageTold{members}, member, rewrite, true)
		return err
	})
}

// finishRewrite checks, holding the day's files, that the file staged is the
// one there still, as its look tells, but for lines appended to a plain file
// since, which it rewrites onto the file beside; and renames that into place
// where any line changed. It reports false, renaming nothing, where the
// file's gone, was replaced or changed otherwise, or the day came to have
// both files.
func (f *Files) finishRewrite(date string, file dayFile, s *stagedRewrite, rewrite func([]byte) []byte) (Rewritten, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	before, err := f.statHeld(date)
	if err != nil {
		return Rewritten{}, false, ignoreGone(err)
	}
	if !s.current(file, before) {
		return Rewritten{}, false, nil
	}
	if !file.compressed {
		if err := s.catchUp(before.PlainSize, rewrite); err != nil {
			return Rewritten{}, false, err
		}
	}
	if !s.changed {
		return Rewritten{Before: before, After: before}, true, nil
	}
	if err := os.Rename(s.tmp, f.path(file)); err != nil {
		return Rewritten{}, false, err
	}
	s.tmp = ""
	after, err := f.statHeld(date)
	if err != nil {
		return Rewritten{}, false, err
	}
	return Rewritten{Changed: true, Before: before, After: after}, true, nil
}

// current reports whether the day, as look looks at it, still has the file
// staged alone, as it was opened, but for a plain file grown since.
func (s *stagedRewrite) current(file dayFile, look DayStat) bool {
	now, err := os.Stat(s.src.Name())
	if err != nil || !os.SameFile(s.info, now) {
		return false
	}
	if file.compressed {
		return look.PlainModified.IsZero() && now.Size() == s.info.Size() && now.ModTime().Equal(s.info.ModTime())
	}
	return look.CompressedModified.IsZero() && now.Size() >= s.end
}

// catchUp rewrites what the plain file gained since it was read to, up to
// size, onto the file beside it, synced.
func (s *stagedRewrite) catchUp(size int64, rewrite func([]byte) []byte) error {
	if size == s.end {
		return nil
	}
	tmp, err := os.OpenFile(s.tmp, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	buffered := bufio.NewWriter(tmp)
	_, changed, err := rewriteLines(io.NewSectionReader(s.src, s.end, size-s.end), buffered, rewrite, true)
	if err == nil {
		err = buffered.Flush()
	}
	if err == nil {
		err = tmp.Sync()
	}
	s.changed = s.changed || changed
	return errors.Join(err, tmp.Close())
}

// discard closes the file staged, and removes the file written beside it,
// where it wasn't renamed into place.
func (s *stagedRewrite) discard() {
	_ = s.src.Close()
	if s.tmp != "" {
		_ = os.Remove(s.tmp)
	}
}

// statHeld looks at the day's files as Stat does, for one holding them.
func (f *Files) statHeld(date string) (DayStat, error) {
	plain, err := f.stat(plainFile(date))
	if err != nil {
		return DayStat{}, err
	}
	compressed, err := f.stat(compressedFile(date))
	switch {
	case err != nil:
		return DayStat{}, err
	case plain == nil && compressed == nil:
		return DayStat{}, f.noLines(date)
	}
	var s DayStat
	if plain != nil {
		s.PlainSize, s.PlainModified = plain.Size(), plain.ModTime()
	}
	if compressed != nil {
		s.CompressedSize, s.CompressedModified = compressed.Size(), compressed.ModTime()
	}
	return s, nil
}

// ignoreGone is err, but nil where it's the day having no files.
func ignoreGone(err error) error {
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// rewriteLines writes each line src gives to dst, as rewrite makes it, and
// returns how many bytes of src it took, and whether any line changed. A
// line without its line ending, at src's end, is taken where whole is set,
// written without one, and otherwise left for a later read.
func rewriteLines(src io.Reader, dst io.Writer, rewrite func([]byte) []byte, whole bool) (took int64, changed bool, err error) {
	r := bufio.NewReader(src)
	for {
		line, readErr := r.ReadBytes('\n')
		ended := len(line) > 0 && line[len(line)-1] == '\n'
		if len(line) > 0 && (ended || whole) {
			content := bytes.TrimSuffix(line, lineEnding)
			out := rewrite(content)
			changed = changed || !bytes.Equal(out, content)
			if _, err := dst.Write(out); err != nil {
				return took, changed, err
			}
			if ended {
				if _, err := dst.Write(lineEnding); err != nil {
					return took, changed, err
				}
			}
			took += int64(len(line))
		}
		switch {
		case errors.Is(readErr, io.EOF):
			return took, changed, nil
		case readErr != nil:
			return took, changed, readErr
		}
	}
}
