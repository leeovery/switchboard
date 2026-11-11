package dayfile

import (
	"bufio"
	"compress/flate"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/leeovery/switchboard/internal/atomicfile"
)

const (
	// mostAside is how many damaged files of one day repairing one sets
	// aside, each under a name of its own, before it gives up.
	mostAside = 100
	// asideSuffix ends the name of a damaged file set aside, as asideFor
	// names one, but for a number after it.
	asideSuffix = ".damaged"
)

// ErrDamaged is what reading a compressed day's file fails with, wrapped,
// where it's damaged, as corrupt says: not gzip at all, as one empty, or,
// rewriting it, cut short.
var ErrDamaged = errors.New("the file is damaged")

// damage is a failure to read a compressed day's file that's damaged, as
// ErrDamaged says, said as it failed.
type damage struct {
	error
}

func (d damage) Unwrap() error {
	return d.error
}

func (damage) Is(target error) bool {
	return target == ErrDamaged
}

// corrupt reports whether err, reading a compressed file through gzip, is
// the file's own damage: not gzip, as one empty, cut short, failing its
// checksum, or holding what doesn't inflate. A failure of the file system to
// read it, as an I/O error, isn't, and is to be tried again.
func corrupt(err error) bool {
	var inflate flate.CorruptInputError
	return errors.Is(err, gzip.ErrHeader) || errors.Is(err, gzip.ErrChecksum) || errors.As(err, &inflate) ||
		errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF)
}

// damaged is err, reading a compressed file through gzip, told as damage
// where it's the file's own, as corrupt says.
func damaged(err error) error {
	if err != nil && corrupt(err) {
		return damage{err}
	}
	return err
}

// damageTold reads a compressed file's lines through gzip, what it fails with
// before their end told as damage, as damaged tells it.
type damageTold struct {
	r io.Reader
}

func (d damageTold) Read(p []byte) (int, error) {
	n, err := d.r.Read(p)
	if err != nil && !errors.Is(err, io.EOF) {
		err = damaged(err)
	}
	return n, err
}

// repairHeld puts in place of the damaged compressed file of the local day
// with the given date one of the lines it gives before its damage, as
// wholeLinesOf writes them, holding the day's files, and keeps the damaged
// one aside, under a name the files don't list, as asideFor gives it, for
// whoever would recover more of it, till its day's past keeping: so the day
// reads as it did, and is rewritten, compressed and pruned as any is. It's
// warned of.
func (f *Files) repairHeld(date string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := f.path(compressedFile(date))
	tmp, err := atomicfile.Stage(path, 0o600, func(w io.Writer) error { return f.wholeLinesOf(path, w) })
	if err != nil {
		return fmt.Errorf("repair %s: %w", compressedFile(date).name(f.Prefix), err)
	}
	aside, err := f.asideFor(path)
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		return fmt.Errorf("repair %s: %w", compressedFile(date).name(f.Prefix), errors.Join(err, os.Remove(tmp)))
	}
	f.Logger.Warn("a day's compressed file was damaged; it's set aside, and its lines before the damage kept", "file", compressedFile(date).name(f.Prefix), "aside", filepath.Base(aside))
	return nil
}

// wholeLinesOf writes the lines the compressed file at path gives before its
// damage to w, compressed, each member's in a gzip member of its own, as
// copyMember copies them, so the lines each adds start where they did, as a
// compress stopped part way is found by: up to the member that's damaged,
// and none after it, as there's no telling where the next starts. One that
// isn't gzip at all gives a member of none. It fails where the file can't be
// read for another reason than its damage, as corrupt tells.
func (f *Files) wholeLinesOf(path string, w io.Writer) error {
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	var r io.Reader = src
	if f.repairing != nil {
		r = f.repairing(src)
	}
	in := bufio.NewReader(r)
	members, err := gzip.NewReader(in)
	if err != nil {
		if !corrupt(err) {
			return err
		}
		return gzipTo(w, func(io.Writer) error { return nil })
	}
	for {
		members.Multistream(false)
		var read error
		err := gzipTo(w, func(member io.Writer) error {
			read = copyMember(member, members)
			return ignoreCorrupt(read)
		})
		if err != nil || read != nil {
			return err
		}
		switch err := members.Reset(in); {
		case errors.Is(err, io.EOF):
			return nil
		case err != nil:
			return ignoreCorrupt(err)
		}
	}
}

// copyMember copies the lines a member of a compressed file gives, read from
// r, to w: every one, as it is, where the member's whole, the last without
// its ending where it has none, as another member may give that; else those
// before the damage, the last, cut short by it, left out, unless it's whole
// JSON, as the readers take a last line without its ending, which is given
// one. It returns what reading the member failed with.
func copyMember(w io.Writer, r io.Reader) error {
	in := bufio.NewReader(r)
	for {
		line, err := in.ReadBytes('\n')
		ended := errors.Is(err, io.EOF)
		switch {
		case err == nil || ended:
		case corrupt(err) && json.Valid(line):
			line = append(line, '\n')
		default:
			return err
		}
		if _, werr := w.Write(line); werr != nil || ended {
			return werr
		}
		if err != nil {
			return err
		}
	}
}

// ignoreCorrupt is err but where it's a compressed file's damage, as corrupt
// says, as reading up to it is all there is to it.
func ignoreCorrupt(err error) error {
	if corrupt(err) {
		return nil
	}
	return err
}

// asideFor links the damaged file at path under a name of its own beside it
// that no day's file has, its name ending in asideSuffix, then a number from
// 2 where that's taken by another, and returns the name: the one it has
// already where it's linked so, as by a repair stopped before it was done.
// On a file system without links, as FAT's, it's copied there instead.
func (f *Files) asideFor(path string) (string, error) {
	for n := 1; n <= mostAside; n++ {
		aside := path + asideSuffix
		if n > 1 {
			aside = fmt.Sprintf("%s-%d", aside, n)
		}
		err := f.setAside(path, aside)
		switch {
		case err == nil:
			return aside, nil
		case !errors.Is(err, fs.ErrExist):
			return "", err
		case sameFile(path, aside):
			return aside, nil
		}
	}
	return "", fmt.Errorf("%d damaged files of the day are set aside already", mostAside)
}

// setAside gives the file at path the name aside too, as a link, else, on a
// file system without links, as a copy, failing, matching fs.ErrExist, where
// something has that name already.
func (f *Files) setAside(path, aside string) error {
	link := f.link
	if link == nil {
		link = os.Link
	}
	err := link(path, aside)
	if !errors.Is(err, errors.ErrUnsupported) && !errors.Is(err, fs.ErrPermission) {
		return err
	}
	return copyAside(path, aside)
}

// copyAside copies the file at path to a file of its own named aside, failing,
// matching fs.ErrExist, where something has that name already, and leaving
// none where it fails otherwise.
func copyAside(path, aside string) error {
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	dst, err := os.OpenFile(aside, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, src)
	if err = errors.Join(err, dst.Close()); err != nil {
		return errors.Join(err, os.Remove(aside))
	}
	return nil
}

// asideOf returns the date and the local day of the damaged compressed file
// set aside under the given name, as asideFor names one, reporting false for
// a name that isn't one.
func (f *Files) asideOf(name string) (string, time.Time, bool) {
	held, after, ok := strings.CutLast(name, asideSuffix)
	if !ok || !asideNumbered(after) {
		return "", time.Time{}, false
	}
	file, day, ok := f.named(held)
	return file.date, day, ok && file.compressed
}

// asideNumbered reports whether after, what follows asideSuffix in a name, is
// what asideFor puts there: nothing, or a dash and a number.
func asideNumbered(after string) bool {
	digits, dashed := strings.CutPrefix(after, "-")
	return after == "" || dashed && digits != "" && strings.Trim(digits, "0123456789") == ""
}

// sameFile reports whether the paths given name one file.
func sameFile(a, b string) bool {
	infoA, errA := os.Lstat(a)
	infoB, errB := os.Lstat(b)
	return errA == nil && errB == nil && os.SameFile(infoA, infoB)
}
