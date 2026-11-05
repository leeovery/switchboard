package export

import (
	"io/fs"
	"syscall"
	"testing"
	"time"
)

func TestAStagedFileIsALeftoverAnHourAfterItLastChangedOnAnyVolume(t *testing.T) {
	now := time.Date(2026, 10, 9, 14, 42, 7, 0, time.UTC)
	aMonthAgo := now.AddDate(0, -1, 0)
	tests := []struct {
		name         string
		ctime, birth time.Time
		want         bool
	}{
		{name: "staged just now on APFS, which sets its birthtime back with its modified time", ctime: now, birth: aMonthAgo},
		{name: "staged just now on FAT or exFAT, which give its modified time as its ctime", ctime: aMonthAgo, birth: now},
		{name: "left a month ago", ctime: aMonthAgo, birth: aMonthAgo, want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			staged := stagedFile{st: syscall.Stat_t{
				Mtimespec:     timespec(aMonthAgo),
				Ctimespec:     timespec(tt.ctime),
				Birthtimespec: timespec(tt.birth),
			}}
			e := Exporter{Now: func() time.Time { return now }}

			if got := e.leftover(fs.FileInfoToDirEntry(staged)); got != tt.want {
				t.Errorf("leftover() = %v, want %v", got, tt.want)
			}
		})
	}
}

func timespec(t time.Time) syscall.Timespec {
	return syscall.NsecToTimespec(t.UnixNano())
}

// stagedFile is a file staged beside export.json, as its stat st has it.
type stagedFile struct {
	st syscall.Stat_t
}

func (stagedFile) Name() string         { return ".export.json.123" }
func (stagedFile) Size() int64          { return 0 }
func (stagedFile) Mode() fs.FileMode    { return 0o600 }
func (f stagedFile) ModTime() time.Time { return time.Unix(f.st.Mtimespec.Unix()) }
func (stagedFile) IsDir() bool          { return false }
func (f stagedFile) Sys() any           { return &f.st }
