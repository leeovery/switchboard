package ledger

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"

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

// builtName returns the date of the local day a file of the given name is
// built of the lines of, its digest, its parts file or its head, reporting
// false where it's none of them.
func builtName(name string) (string, bool) {
	built := []struct{ prefix, suffix string }{{digestPrefix, digestSuffix}, {partsPrefix, partsSuffix}, {logdayPrefix, logdaySuffix}}
	for _, b := range built {
		if date, ok := datedName(name, b.prefix, b.suffix); ok {
			return date, true
		}
	}
	return "", false
}

// builtPart is a session's part of a day as it's built, to be written to the
// day's parts file: on, the date of the state it was folded on, as PartRef's
// On has it; and part and times, the gzip members of the part and of its
// times, as builtPartOf makes them of what its Digester gives, or as a parts
// file written before holds them, copied, as keptPart reads them: none of
// its times where it has none.
type builtPart struct {
	on          string
	part, times []byte
}

// builtPartOf returns the part of a session folded on the state of the day
// with the date on, "" for none, as builtPart has it: part and times, JSON
// values of its Digester's, each a member of its own, its insignificant
// spaces left out; none of its times where they're empty, and none at all
// where part is. It fails where either isn't a JSON value.
func builtPartOf(on string, part, times json.RawMessage) (builtPart, error) {
	if len(part) == 0 {
		return builtPart{}, nil
	}
	built := builtPart{on: on}
	var err error
	if built.part, err = compactMember(part); err != nil {
		return builtPart{}, fmt.Errorf("the part: %w", err)
	}
	if len(times) > 0 {
		if built.times, err = compactMember(times); err != nil {
			return builtPart{}, fmt.Errorf("the part's times: %w", err)
		}
	}
	return built, nil
}

// compactMember is data, a JSON value, its insignificant spaces left out, as
// one gzip member. It fails where data isn't a JSON value.
func compactMember(data json.RawMessage) ([]byte, error) {
	var compact bytes.Buffer
	if err := json.Compact(&compact, data); err != nil {
		return nil, err
	}
	return memberOf(compact.Bytes()), nil
}

// partsFileOf returns the parts file of parts, the parts of sessions by their
// ids: each session's part's member, then its times', in the order of the
// sessions' ids, as membersOf lays them out, so the same parts make the same
// file, byte for byte; and where each session's part lies in it, by its id,
// on the date its part gives.
func partsFileOf(parts map[string]builtPart) ([]byte, map[string]PartRef) {
	ids := slices.Sorted(maps.Keys(parts))
	var members [][]byte
	for _, id := range ids {
		members = append(members, parts[id].part)
		if parts[id].times != nil {
			members = append(members, parts[id].times)
		}
	}
	file, at := membersOf(members)
	refs := make(map[string]PartRef, len(parts))
	for _, id := range ids {
		ref := PartRef{On: parts[id].on, Member: at[0]}
		at = at[1:]
		if parts[id].times != nil {
			times := at[0]
			ref.Times, at = &times, at[1:]
		}
		refs[id] = ref
	}
	return file, refs
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

// readMember returns what the member m points at in the parts file of the
// local day with the given date holds, a part or its times, as it was
// written: failing as memberIn fails, with fs.ErrNotExist where the day holds
// no parts file.
func (d days) readMember(date string, m Member) (json.RawMessage, error) {
	data, err := memberIn(partsFile(d.files.Dir, date), m)
	if err != nil {
		return nil, fmt.Errorf("a part of %s: %w", date, err)
	}
	return data, nil
}

// keptPart returns the part of a session ref points at in the parts file of
// the local day with the given date, as builtPart has it, on the date ref
// names: its members' bytes as they were written there, none of them
// inflated, to be written again as they are. It fails as memberBytes fails,
// with fs.ErrNotExist where the day holds no parts file.
func (d days) keptPart(date string, ref PartRef) (builtPart, error) {
	path := partsFile(d.files.Dir, date)
	kept := builtPart{on: ref.On}
	var err error
	if kept.part, err = memberBytes(path, ref.Member); err == nil && ref.Times != nil {
		kept.times, err = memberBytes(path, *ref.Times)
	}
	if err != nil {
		return builtPart{}, fmt.Errorf("a part of %s: %w", date, err)
	}
	return kept, nil
}

// removed removes the file at path, failing as os.Remove does, but where
// it's gone already.
func removed(path string) error {
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
