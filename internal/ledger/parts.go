package ledger

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
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
// built of the lines of, its digest, its parts file, its head or its rows
// file, reporting false where it's none of them.
func builtName(name string) (string, bool) {
	built := []struct{ prefix, suffix string }{{digestPrefix, digestSuffix}, {partsPrefix, partsSuffix}, {logdayPrefix, logdaySuffix}, {rowsPrefix, rowsSuffix}}
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

// stateKey is one of the states a digest's entry holds: of the session with
// the id id, apart 0 for its own, i for the ith of its states apart, as
// DigestEntry's Apart orders them.
type stateKey struct {
	id    string
	apart int
}

// compare orders k and o by their sessions' ids, then each session's own
// state before its states apart, in their order.
func (k stateKey) compare(o stateKey) int {
	return cmp.Or(strings.Compare(k.id, o.id), cmp.Compare(k.apart, o.apart))
}

// partsFileOf returns the parts file of parts, the parts of the states of
// sessions, as stateKey keys them: each state's part's member, then its
// times', in the order compare gives, as membersOf lays them out, so the
// same parts make the same file, byte for byte, and one of sessions of one
// pool each is laid out as it was before pools; and where each state's part
// lies in it, as its key keys it, on the date its part gives.
func partsFileOf(parts map[stateKey]builtPart) ([]byte, map[stateKey]PartRef) {
	keys := slices.SortedFunc(maps.Keys(parts), stateKey.compare)
	var members [][]byte
	for _, k := range keys {
		members = append(members, parts[k].part)
		if parts[k].times != nil {
			members = append(members, parts[k].times)
		}
	}
	file, at := membersOf(members)
	refs := make(map[stateKey]PartRef, len(parts))
	for _, k := range keys {
		ref := PartRef{On: parts[k].on, Member: at[0]}
		at = at[1:]
		if parts[k].times != nil {
			times := at[0]
			ref.Times, at = &times, at[1:]
		}
		refs[k] = ref
	}
	return file, refs
}

// writeParts writes file as the parts file of the local day with the given
// date, as writeMembers writes one: removing any there where it's empty, as
// of a day whose sessions have no parts, so the day holds none, as
// partsHeld judges it.
func (d days) writeParts(date string, file []byte) error {
	return writeMembers(partsFile(d.files.Dir, date), file)
}

// partsHeld reports whether the parts file of the local day with the given
// date is as long as size, as the head of the day's digest records it, as
// membersHeld judges it.
func (d days) partsHeld(date string, size int64) bool {
	return membersHeld(partsFile(d.files.Dir, date), size)
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
