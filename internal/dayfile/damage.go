package dayfile

import (
	"bufio"
	"compress/flate"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/leeovery/switchboard/internal/atomicfile"
)

// mostAside is how many damaged files of one day repairing one sets aside,
// each under a name of its own, before it gives up.
const mostAside = 100

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
// with the given date one of the whole lines it gives before its damage,
// holding the day's files, and keeps the damaged one aside, under a name the
// files don't list, as asideFor gives it, for whoever would recover more of
// it: so the day reads as it did, and is rewritten, compressed and pruned as
// any is. It's warned of.
func (f *Files) repairHeld(date string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	path := f.path(compressedFile(date))
	tmp, err := atomicfile.Stage(path, 0o600, func(w io.Writer) error { return wholeLinesOf(path, w) })
	if err != nil {
		return fmt.Errorf("repair %s: %w", compressedFile(date).name(f.Prefix), err)
	}
	aside, err := asideFor(path)
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		return fmt.Errorf("repair %s: %w", compressedFile(date).name(f.Prefix), errors.Join(err, os.Remove(tmp)))
	}
	f.Logger.Warn("a day's compressed file was damaged; it's set aside, and its lines before the damage kept", "file", compressedFile(date).name(f.Prefix), "aside", filepath.Base(aside))
	return nil
}

// wholeLinesOf writes the whole lines the compressed file at path gives
// before its damage to w, compressed, a gzip member of their own: none of
// one that isn't gzip at all. It fails where the file can't be read for
// another reason than its damage, as corrupt tells.
func wholeLinesOf(path string, w io.Writer) error {
	src, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() { _ = src.Close() }()
	return gzipTo(w, func(member io.Writer) error {
		lines, err := gzip.NewReader(bufio.NewReader(src))
		if err != nil {
			return ignoreCorrupt(err)
		}
		in := bufio.NewReader(lines)
		for {
			line, err := in.ReadBytes('\n')
			if err != nil {
				// A line cut short by the damage, or the file's end, is
				// left out.
				return ignoreCorrupt(err)
			}
			if _, err := member.Write(line); err != nil {
				return err
			}
		}
	})
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
// that no day's file has, its name ending ".damaged", then a number from 2
// where that's taken by another, and returns the name: the one it has
// already where it's linked so, as by a repair stopped before it was done.
func asideFor(path string) (string, error) {
	for n := 1; n <= mostAside; n++ {
		aside := path + ".damaged"
		if n > 1 {
			aside = fmt.Sprintf("%s-%d", aside, n)
		}
		err := os.Link(path, aside)
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

// sameFile reports whether the paths given name one file.
func sameFile(a, b string) bool {
	infoA, errA := os.Lstat(a)
	infoB, errB := os.Lstat(b)
	return errA == nil && errB == nil && os.SameFile(infoA, infoB)
}
