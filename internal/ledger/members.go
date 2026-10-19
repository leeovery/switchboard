package ledger

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"io/fs"
	"os"

	"github.com/leeovery/switchboard/internal/atomicfile"
)

// memberMax is the most a member of one of the ledger's files of gzip
// members runs to, and inflates to, as it's read: a busy session's part of a
// day runs to a few hundred kilobytes, and a member pointed at by hand to
// run, or inflate, past it mustn't be read whole into memory.
const memberMax = 64 << 20

// Member is where a gzip member lies in one of the ledger's files of them, as
// a digest points at it: Offset, where its bytes begin in the file; Length,
// how many there are; and CRC, the CRC-32 (IEEE) of those bytes as they were
// written, so the bytes read there are known to be the member pointed at,
// whole, before a byte of them is inflated.
type Member struct {
	Offset, Length int64
	CRC            uint32
}

// storedMember is a member as a digest's file holds it, zeros left out.
type storedMember struct {
	Offset int64  `json:"offset,omitzero"`
	Length int64  `json:"length,omitzero"`
	CRC    uint32 `json:"crc,omitzero"`
}

// stored is m as a digest's file holds it.
func (m Member) stored() storedMember {
	return storedMember(m)
}

// member is the member m points at.
func (m storedMember) member() Member {
	return Member(m)
}

// errMemberDamaged is the error of a member whose file doesn't hold it as it
// was written where it's pointed at: cut short, damaged, or written again
// since, of other members.
var errMemberDamaged = errors.New("not the member written there")

// memberOf is content as one gzip member.
func memberOf(content []byte) []byte {
	var member bytes.Buffer
	w := gzip.NewWriter(&member)
	// A bytes.Buffer's writes never fail, so neither can the member's.
	_, _ = w.Write(content)
	_ = w.Close()
	return member.Bytes()
}

// membersOf returns a file of the gzip members given, each as it's given,
// one after another in the order given, so the same members make the same
// file, byte for byte; and where each lies in it, in the same order: an
// empty file of none.
func membersOf(members [][]byte) ([]byte, []Member) {
	var file bytes.Buffer
	at := make([]Member, len(members))
	for i, m := range members {
		at[i] = Member{Offset: int64(file.Len()), Length: int64(len(m)), CRC: crc32.ChecksumIEEE(m)}
		file.Write(m)
	}
	return file.Bytes(), at
}

// writeMembers writes file, of gzip members as membersOf makes one, at path,
// whole, the user's alone, over any there: or, where it's empty, removes any
// there, so none is held, as membersHeld judges it.
func writeMembers(path string, file []byte) error {
	if len(file) == 0 {
		return removed(path)
	}
	return atomicfile.Write(path, file, 0o600)
}

// membersHeld reports whether the file of gzip members at path is as long as
// size, as the head pointing at its members records it, by a look at the
// file alone: where size is 0, that there's none.
func membersHeld(path string, size int64) bool {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return size == 0
	}
	return err == nil && info.Size() == size
}

// memberIn returns what the member m points at in the file at path holds,
// inflated: failing as memberBytes fails, and with errMemberDamaged where
// its bytes don't inflate as one gzip member, whole, of no more than
// memberMax bytes.
func memberIn(path string, m Member) ([]byte, error) {
	data, err := memberBytes(path, m)
	if err != nil {
		return nil, err
	}
	content, err := inflatedMember(data)
	if err != nil {
		return nil, fmt.Errorf("%d bytes at %d of %s: %w: %w", m.Length, m.Offset, path, errMemberDamaged, err)
	}
	return content, nil
}

// memberBytes returns the bytes of the member m points at in the file at
// path, as they were written, none of them inflated: failing as os.Open
// fails where the file can't be opened, with fs.ErrNotExist where it's
// gone, or as File.Stat fails where it can't be looked at; and with
// errMemberDamaged where the file holds no bytes where m says, as one
// pointed past its end, told before room is made for them, or more than
// memberMax of them, or not those written there, as their CRC tells.
func memberBytes(path string, m Member) ([]byte, error) {
	if m.Offset < 0 || m.Length <= 0 || m.Length > memberMax {
		return nil, fmt.Errorf("%d bytes at %d of %s: %w", m.Length, m.Offset, path, errMemberDamaged)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if m.Length > info.Size()-m.Offset {
		return nil, fmt.Errorf("%d bytes at %d of %s, of %d: %w", m.Length, m.Offset, path, info.Size(), errMemberDamaged)
	}
	data := make([]byte, m.Length)
	if _, err := file.ReadAt(data, m.Offset); err != nil {
		return nil, fmt.Errorf("%d bytes at %d of %s: %w: %w", m.Length, m.Offset, path, errMemberDamaged, err)
	}
	if crc32.ChecksumIEEE(data) != m.CRC {
		return nil, fmt.Errorf("%d bytes at %d of %s, by their CRC: %w", m.Length, m.Offset, path, errMemberDamaged)
	}
	return data, nil
}

// errMemberTooLarge is the error of a member that inflates past memberMax.
var errMemberTooLarge = fmt.Errorf("inflates past %d bytes", memberMax)

// inflatedMember returns what data, one gzip member, whole, holds, inflated:
// failing where it isn't one, as one ending early, or before data does, or
// of a trailer that doesn't check; and with errMemberTooLarge, having
// inflated no more than memberMax and a byte, where it inflates past that.
func inflatedMember(data []byte) ([]byte, error) {
	r := bytes.NewReader(data)
	member, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	member.Multistream(false)
	content, err := io.ReadAll(io.LimitReader(member, memberMax+1))
	switch {
	case err != nil:
		return nil, err
	case len(content) > memberMax:
		return nil, errMemberTooLarge
	case r.Len() > 0:
		return nil, fmt.Errorf("%d bytes past its end", r.Len())
	}
	return content, nil
}
