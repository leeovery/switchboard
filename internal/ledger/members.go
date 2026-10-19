package ledger

import (
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"maps"
	"os"
	"slices"
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

// membersOf returns a file of gzip members, one of each of contents, in the
// order of their keys, so the same contents make the same file, byte for
// byte; and where each lies in it, by its key: an empty file of none.
func membersOf(contents map[string][]byte) ([]byte, map[string]Member) {
	var file bytes.Buffer
	members := make(map[string]Member, len(contents))
	for _, key := range slices.Sorted(maps.Keys(contents)) {
		offset := file.Len()
		w := gzip.NewWriter(&file)
		// A bytes.Buffer's writes never fail, so neither can the member's.
		_, _ = w.Write(contents[key])
		_ = w.Close()
		written := file.Bytes()[offset:]
		members[key] = Member{Offset: int64(offset), Length: int64(len(written)), CRC: crc32.ChecksumIEEE(written)}
	}
	return file.Bytes(), members
}

// memberIn returns what the member m points at in the file at path holds,
// inflated: failing as os.Open fails where the file can't be opened, with
// fs.ErrNotExist where it's gone, or as File.Stat fails where it can't be
// looked at; and with errMemberDamaged where the file holds no bytes where m
// says, as one pointed past its end, told before room is made for them, or
// not those written there, as their CRC tells, or where they don't inflate
// as one gzip member, whole, of no more than memberMax bytes either way.
func memberIn(path string, m Member) ([]byte, error) {
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
	content, err := inflatedMember(data)
	if err != nil {
		return nil, fmt.Errorf("%d bytes at %d of %s: %w: %w", m.Length, m.Offset, path, errMemberDamaged, err)
	}
	return content, nil
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
