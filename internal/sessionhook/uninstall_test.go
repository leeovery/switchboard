package sessionhook_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/sessionhook"
)

func TestUninstall(t *testing.T) {
	tests := []struct {
		name string
		// before is the settings file before Uninstall, or missing where nil.
		before *string
		// want is the file Uninstall leaves, or missing where nil.
		want        *string
		wantRemoved bool
	}{
		{name: "missing: nothing written", wantRemoved: false},
		{name: "empty: nothing written", before: new(""), want: new(""), wantRemoved: false},
		{name: "others alone: nothing written", before: new(othersBefore), want: new(othersBefore), wantRemoved: false},
		{name: "ours alone: hooks taken out with it", before: new(onlyOurs), want: new("{}\n"), wantRemoved: true},
		{
			name:        "ours among others, a user's naming switchboard among them: ours taken out, theirs as they were",
			before:      new(othersAfter),
			want:        new(othersBefore),
			wantRemoved: true,
		},
		{
			name:        "ours alone in hooks with others: SessionStart taken out, the others kept",
			before:      new("{\n  \"hooks\": {\n    \"Stop\": [],\n    \"SessionStart\": [\n" + ourGroup(switchboard) + "\n    ]\n  }\n}\n"),
			want:        new("{\n  \"hooks\": {\n    \"Stop\": []\n  }\n}\n"),
			wantRemoved: true,
		},
		{
			name:        "ours in a group with a user's hook: theirs kept in its group",
			before:      new(session(group(usersHook, ours("          ", switchboard)))),
			want:        new(session(group(usersHook))),
			wantRemoved: true,
		},
		{
			name:        "ours three times, by other paths and under a matcher: every one taken out",
			before:      new(session(ourGroup(oldSwitchboard), matched(`"compact"`, usersHook, ours("          ", switchboard)), ourGroup(switchboard))),
			want:        new(session(matched(`"compact"`, usersHook))),
			wantRemoved: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "claude", "settings.json")
			long := time.Now().Add(-time.Hour).Truncate(time.Second)
			if tt.before != nil {
				writeFile(t, path, *tt.before, 0o600)
				if err := os.Chtimes(path, long, long); err != nil {
					t.Fatal(err)
				}
			}

			got, err := sessionhook.Uninstall(path)
			if err != nil || got != tt.wantRemoved {
				t.Fatalf("Uninstall() = %v, %v, want %v", got, err, tt.wantRemoved)
			}
			if tt.want == nil {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("the settings file: %v, want none made", err)
				}
				return
			}
			if written := readFile(t, path); written != *tt.want {
				t.Errorf("Uninstall() left\n%s\nwant\n%s", written, *tt.want)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != 0o600 {
				t.Errorf("the settings file's mode is %v, want its own, 0600, kept", info.Mode().Perm())
			}
			if !tt.wantRemoved && !info.ModTime().Equal(long) {
				t.Errorf("the settings file was modified at %v, want it left as it was, at %v", info.ModTime(), long)
			}
			checkNothingLeftBeside(t, path)
		})
	}
}

func TestUninstallThenInstallPutsTheEntryBack(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	writeFile(t, path, othersAfter, 0o644)

	if removed, err := sessionhook.Uninstall(path); err != nil || !removed {
		t.Fatalf("Uninstall() = %v, %v, want it removed", removed, err)
	}
	if removed, err := sessionhook.Uninstall(path); err != nil || removed {
		t.Errorf("Uninstall(), again = %v, %v, want nothing there to remove", removed, err)
	}
	if got, err := sessionhook.Install(path, switchboard); err != nil || got != sessionhook.Installed {
		t.Fatalf("Install() = %v, %v, want %v", got, err, sessionhook.Installed)
	}
	if written := readFile(t, path); written != othersAfter {
		t.Errorf("Install() after Uninstall() wrote\n%s\nwant\n%s", written, othersAfter)
	}
}

func TestUninstallLeavesASettingsFileItCantReadAlone(t *testing.T) {
	tests := []struct {
		name    string
		before  string
		wantErr string
	}{
		{name: "junk", before: "not json", wantErr: "line 1, column 2: invalid character 'o'"},
		{name: "cut short", before: `{"hooks": {"SessionStart": [` + oursCompact, wantErr: "it ends before its JSON does"},
		{name: "hooks an array", before: `{"hooks": []}`, wantErr: "hooks: not a JSON object"},
		{name: "SessionStart an object", before: `{"hooks": {"SessionStart": {}}}`, wantErr: "hooks.SessionStart: not a JSON array"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			writeFile(t, path, tt.before, 0o644)

			got, err := sessionhook.Uninstall(path)
			if err == nil || !strings.HasPrefix(err.Error(), path+": ") || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Uninstall() = %v, %v, want an error naming %s and saying %q", got, err, path, tt.wantErr)
			}
			if written := readFile(t, path); written != tt.before {
				t.Errorf("Uninstall() left\n%s\nwant it as it was\n%s", written, tt.before)
			}
		})
	}
}

func TestUninstallWritesThroughALink(t *testing.T) {
	dir := t.TempDir()
	dotfiles := filepath.Join(dir, "dotfiles", "claude-settings.json")
	writeFile(t, dotfiles, othersAfter, 0o600)
	path := filepath.Join(dir, "claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../dotfiles/claude-settings.json", path); err != nil {
		t.Fatal(err)
	}

	if removed, err := sessionhook.Uninstall(path); err != nil || !removed {
		t.Fatalf("Uninstall() = %v, %v, want it removed", removed, err)
	}
	if target, err := os.Readlink(path); err != nil || target != "../dotfiles/claude-settings.json" {
		t.Errorf("%s leads to %q (%v), want the link left as it was", path, target, err)
	}
	if written := readFile(t, dotfiles); written != othersBefore {
		t.Errorf("the linked file holds\n%s\nwant\n%s", written, othersBefore)
	}
}

func TestUninstallStartsAgainFromAFileChangedMeanwhile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	writeFile(t, path, onlyOurs, 0o644)
	reads := 0
	sessionhook.SetReadFile(t, func(name string) ([]byte, error) {
		reads++
		if reads == 2 {
			// Claude Code writes the file between Uninstall's read and its write.
			writeFile(t, path, "{\n  \"model\": \"sonnet\",\n"+strings.TrimPrefix(onlyOurs, "{\n"), 0o644)
		}
		return os.ReadFile(name)
	})

	if removed, err := sessionhook.Uninstall(path); err != nil || !removed {
		t.Fatalf("Uninstall() = %v, %v, want it removed", removed, err)
	}
	if want, written := "{\n  \"model\": \"sonnet\"\n}\n", readFile(t, path); written != want {
		t.Errorf("Uninstall() wrote\n%s\nwant\n%s", written, want)
	}
	checkNothingLeftBeside(t, path)
}
