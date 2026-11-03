package export_test

import (
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"

	"github.com/leeovery/switchboard/internal/export"
)

// idShape is the shape of an install's id.
var idShape = regexp.MustCompile(`^[0-9a-f]{16}$`)

func TestTheInstallsIDIsMadeOnceTheUsersAlone(t *testing.T) {
	state := filepath.Join(t.TempDir(), "switchboard")

	id, err := export.InstallID(state)
	if err != nil || !idShape.MatchString(id) {
		t.Fatalf("InstallID() = %q, %v; want 16 lowercase hex characters", id, err)
	}
	path := filepath.Join(state, "install-id")
	data, err := os.ReadFile(path)
	if err != nil || string(data) != id+"\n" {
		t.Errorf("install-id holds %q (%v), want %q and a line ending", data, err, id)
	}
	if info := look(t, path); info.Mode().Perm() != 0o600 {
		t.Errorf("install-id is %v, want 0600", info.Mode().Perm())
	}
	if again, err := export.InstallID(state); err != nil || again != id {
		t.Errorf("InstallID() again = %q, %v; want %q, as it was made", again, err, id)
	}
	if entries, err := os.ReadDir(state); err != nil || len(entries) != 1 {
		t.Errorf("the state directory holds %v (%v), want install-id alone", entries, err)
	}
}

func TestTheInstallsIDIsTheOneFoundThere(t *testing.T) {
	state := t.TempDir()
	writeFile(t, filepath.Join(state, "install-id"), []byte("0123456789abcdef\n"), exportNow)

	if id, err := export.InstallID(state); err != nil || id != "0123456789abcdef" {
		t.Errorf("InstallID() = %q, %v; want the one there", id, err)
	}
}

func TestMakersOfTheInstallsIDAtOnceAgreeOnOne(t *testing.T) {
	const makers = 16
	state := t.TempDir()
	ids := make([]string, makers)
	errs := make([]error, makers)
	var wg sync.WaitGroup
	for i := range makers {
		wg.Go(func() { ids[i], errs[i] = export.InstallID(state) })
	}
	wg.Wait()

	for i := range makers {
		if errs[i] != nil || ids[i] != ids[0] || !idShape.MatchString(ids[i]) {
			t.Errorf("maker %d: InstallID() = %q, %v; want %q, the one every maker gets", i, ids[i], errs[i], ids[0])
		}
	}
	if entries, err := os.ReadDir(state); err != nil || len(entries) != 1 {
		t.Errorf("the state directory holds %v (%v), want install-id alone", entries, err)
	}
}

func TestAnInstallIDFileThatDoesntHoldAnIDFails(t *testing.T) {
	tests := []struct {
		name, data string
	}{
		{name: "empty", data: ""},
		{name: "too short", data: "0123456789abcde\n"},
		{name: "too long", data: "0123456789abcdef0\n"},
		{name: "upper case", data: "0123456789ABCDEF\n"},
		{name: "not hex", data: "0123456789abcdeg\n"},
		{name: "without its line ending", data: "0123456789abcdef"},
		{name: "two lines", data: "0123456789abcdef\n0123456789abcdef\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := t.TempDir()
			path := filepath.Join(state, "install-id")
			writeFile(t, path, []byte(tt.data), exportNow)

			if id, err := export.InstallID(state); err == nil {
				t.Errorf("InstallID() of %q = %q; want it to fail", tt.data, id)
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != tt.data {
				t.Errorf("install-id holds %q (%v), want it left as it was, %q", data, err, tt.data)
			}
		})
	}
}

func TestExportingWithACorruptInstallIDFails(t *testing.T) {
	state, dir := t.TempDir(), t.TempDir()
	writeFile(t, filepath.Join(state, "install-id"), []byte("work\n"), exportNow)
	writeFile(t, requests.plain(state, day2), []byte(linesOf(`{"n":1}`)), appendedAt)

	if r, err := exporter(state).Export(dir); err == nil {
		t.Errorf("Export() = %+v; want it to fail", r)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Errorf("the folder exported into holds %v (%v), want nothing", entries, err)
	}
}

func TestAMacsFolderIsNamedByItsInstallsID(t *testing.T) {
	if got, want := export.Folder("/backups", "0123456789abcdef"), "/backups/mac-0123456789abcdef"; got != want {
		t.Errorf("Folder() = %q, want %q", got, want)
	}
}
