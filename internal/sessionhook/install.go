package sessionhook

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"unicode/utf8"

	"github.com/leeovery/switchboard/internal/atomicfile"
)

// Outcome is what Install did.
type Outcome int

const (
	// InPlace is switchboard's entry found as it should be: nothing is
	// written.
	InPlace Outcome = iota
	// Installed is the entry added, where there was none.
	Installed
	// PutRight is the entry corrected, moved into a group of its own, or
	// found more than once and the others removed.
	PutRight
)

// newFileMode is the mode of a settings file Install makes.
const newFileMode fs.FileMode = 0o644

// restarts is how many times Install starts again from a settings file that
// changed as it was writing it, as Claude Code writes it as it likes.
const restarts = 3

// errChanged is what a write fails with when the settings file changed since
// it was read.
var errChanged = errors.New("changed since it was read")

// readFile reads a file, as os.ReadFile does: a seam, so tests can change
// the settings file between Install's reads of it.
var readFile = os.ReadFile

// bom is the byte order mark, which JSON doesn't allow, nor Claude Code.
var bom = []byte("\xef\xbb\xbf")

// Install puts switchboard's entry, running switchboard at the path given,
// in Claude Code's settings file at path, or puts it right, and reports
// which, writing nothing where it's in place already. Every other setting and
// hook is left as it was, in its order, and the file is written whole, by
// its mode, through a link to it as to the file. A file that isn't missing,
// empty or a JSON object, or whose hooks aren't shaped as Claude Code reads
// them, is left as it is, and Install fails saying what's wrong. A file that
// changes as it's written, as by Claude Code, is read again and the entry
// put in what it holds then. A switchboard run by a name other than its own,
// as through a link of the user's, writes nothing and fails: the entry is
// known by that name, and one by another would be added again at every run.
func Install(path, switchboard string) (Outcome, error) {
	if name := filepath.Base(switchboard); name != binaryName {
		return InPlace, fmt.Errorf("switchboard is running as %s here, so the session hook isn't installed: run setup as %s", name, binaryName)
	}
	target, err := atomicfile.Target(path)
	if err != nil {
		return InPlace, fmt.Errorf("follow %s: %w", path, err)
	}
	for range restarts + 1 {
		outcome, err := installAt(target, Command(switchboard))
		if !errors.Is(err, errChanged) {
			if err != nil {
				return InPlace, fmt.Errorf("%s: %w", path, err)
			}
			return outcome, nil
		}
	}
	return InPlace, fmt.Errorf("%s kept changing as the session hook was written into it, so it's left as it is: run setup again", path)
}

// installAt puts the entry running command in the settings file at target,
// failing with errChanged, having written nothing, where the file changed
// meanwhile.
func installAt(target, command string) (Outcome, error) {
	read, mode, err := readSettings(target)
	if err != nil {
		return InPlace, err
	}
	settings, err := parseSettings(read)
	if err != nil {
		return InPlace, err
	}
	outcome, err := place(&settings, command)
	if err != nil || outcome == InPlace {
		return outcome, err
	}
	if err := writeSettings(target, settings, mode, read); err != nil {
		return InPlace, err
	}
	return outcome, nil
}

// readSettings reads the settings file at path as it is, and its mode: nil,
// and newFileMode, where it's missing.
func readSettings(path string) ([]byte, fs.FileMode, error) {
	data, err := readFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, newFileMode, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("read Claude Code's settings: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, fmt.Errorf("read Claude Code's settings: %w", err)
	}
	return data, info.Mode().Perm(), nil
}

// parseSettings reads data, the settings file's, as an object: one with
// nothing in it where the file's missing or holds only spaces. Where it
// can't, it says where.
func parseSettings(data []byte) (object, error) {
	if bytes.HasPrefix(data, bom) {
		return nil, errors.New("it starts with a byte order mark, which JSON doesn't allow")
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return object{}, nil
	}
	settings, err := parseObject(data)
	if err == nil {
		return settings, nil
	}
	if cutShort(data) {
		return nil, errors.New("it ends before its JSON does")
	}
	if syntax, ok := errors.AsType[*json.SyntaxError](err); ok {
		line, column := position(data, syntax.Offset)
		return nil, fmt.Errorf("line %d, column %d: %w", line, column, err)
	}
	return nil, err
}

// cutShort reports whether data, which doesn't parse as an object, ends
// before its JSON does. Its error can't say: a bad last byte and the end
// coming too soon both fail at data's end. So it's read again with a byte
// no JSON takes after it, which fails past data's end only where data was
// all well formed, as far as it went.
func cutShort(data []byte) bool {
	_, err := parseObject(append(slices.Clip(data), 0))
	syntax, ok := errors.AsType[*json.SyntaxError](err)
	return ok && syntax.Offset > int64(len(data))
}

// position is the line and column, each from 1, of the byte data's reader
// failed at, having read offset bytes.
func position(data []byte, offset int64) (line, column int) {
	before := data[:max(0, min(offset-1, int64(len(data))))]
	start := bytes.LastIndexByte(before, '\n') + 1
	return bytes.Count(before, []byte{'\n'}) + 1, utf8.RuneCount(before[start:]) + 1
}

// writeSettings writes settings to path, indented as Claude Code writes them,
// making its directory, unless the file no longer holds read, what it held
// when it was read: then it fails with errChanged, writing nothing.
func writeSettings(path string, settings object, mode fs.FileMode, read []byte) error {
	var out bytes.Buffer
	if err := json.Indent(&out, settings.encode(), "", "  "); err != nil {
		return fmt.Errorf("write Claude Code's settings: %w", err)
	}
	out.WriteByte('\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create Claude Code's config directory: %w", err)
	}
	var reread error
	err := atomicfile.WriteChecked(path, mode, func(w io.Writer) error {
		_, err := w.Write(out.Bytes())
		return err
	}, func() error {
		now, _, err := readSettings(path)
		if err != nil {
			reread = err
			return err
		}
		if !bytes.Equal(now, read) {
			return errChanged
		}
		return nil
	})
	if reread != nil {
		return reread
	}
	if err != nil && !errors.Is(err, errChanged) {
		return fmt.Errorf("write Claude Code's settings: %w", err)
	}
	return err
}
