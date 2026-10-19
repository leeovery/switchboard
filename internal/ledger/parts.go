package ledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/leeovery/switchboard/internal/atomicfile"
)

// partsPrefix and partsSuffix begin and end the name of a day's parts file,
// beside its digest: parts-<date>.gz.
const partsPrefix, partsSuffix = "parts-", ".gz"

// partsFile is where the ledger in dir keeps the parts of the sessions of the
// local day with the given date, as its digest points at them.
func partsFile(dir, date string) string {
	return filepath.Join(dir, partsPrefix+date+partsSuffix)
}

// digestedName returns the date of the local day a file of the given name is
// the digest of, or the parts file of, reporting false where it's neither.
func digestedName(name string) (string, bool) {
	if date, ok := datedName(name, digestPrefix, digestSuffix); ok {
		return date, true
	}
	return datedName(name, partsPrefix, partsSuffix)
}

// builtPart is a session's part of a day as it's built, to be written to the
// day's parts file: on, the date of the state it was folded on, as PartRef's
// On has it; and data, the part, a JSON value of its Digester's.
type builtPart struct {
	on   string
	data json.RawMessage
}

// partsFileOf returns the parts file of parts, the parts of sessions by their
// ids, as membersOf makes it of each part's JSON, its insignificant spaces
// left out, so it's of the sessions in the order of their ids; and where each
// session's part lies in it, by its id. It fails where a part isn't a JSON
// value.
func partsFileOf(parts map[string]builtPart) ([]byte, map[string]Member, error) {
	contents := make(map[string][]byte, len(parts))
	for id, p := range parts {
		var compact bytes.Buffer
		if err := json.Compact(&compact, p.data); err != nil {
			return nil, nil, fmt.Errorf("the part of %s: %w", id, err)
		}
		contents[id] = compact.Bytes()
	}
	file, members := membersOf(contents)
	return file, members, nil
}

// writeParts writes file as the parts file of the local day with the given
// date, whole, the user's alone, over any there: or, where it's empty, as of
// a day whose sessions have no parts, removes any there, so the day holds
// none, as partsHeld judges it.
func (d days) writeParts(date string, file []byte) error {
	path := partsFile(d.files.Dir, date)
	if len(file) == 0 {
		return removed(path)
	}
	return atomicfile.Write(path, file, 0o600)
}

// partsHeld reports whether the parts file of the local day with the given
// date is as long as size, as the head of the day's digest records it, by a
// look at the file alone: where size is 0, that there's none.
func (d days) partsHeld(date string, size int64) bool {
	info, err := os.Stat(partsFile(d.files.Dir, date))
	if errors.Is(err, fs.ErrNotExist) {
		return size == 0
	}
	return err == nil && info.Size() == size
}

// readPart returns the part of a session ref points at in the parts file of
// the local day with the given date, as it was written: failing as memberIn
// fails, with fs.ErrNotExist where the day holds no parts file.
func (d days) readPart(date string, ref PartRef) (json.RawMessage, error) {
	part, err := memberIn(partsFile(d.files.Dir, date), ref.Member)
	if err != nil {
		return nil, fmt.Errorf("a part of %s: %w", date, err)
	}
	return part, nil
}

// removed removes the file at path, failing as os.Remove does, but where
// it's gone already.
func removed(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
