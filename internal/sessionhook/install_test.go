package sessionhook_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/sessionhook"
)

// switchboard is the path setup runs switchboard by, and oldSwitchboard one
// it ran by before.
const (
	switchboard    = "/opt/homebrew/bin/switchboard"
	oldSwitchboard = "/Users/test/go/bin/switchboard"
)

// command is the hook's command for the switchboard at path, as JSON writes
// it.
func command(path string) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(sessionhook.Command(path)); err != nil {
		panic(err)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// ours is switchboard's hook object, running the switchboard at path, as
// Claude Code writes it, each line indented by indent.
func ours(indent, path string) string {
	return indent + "{\n" +
		indent + `  "type": "command",` + "\n" +
		indent + `  "command": ` + command(path) + ",\n" +
		indent + `  "timeout": 5` + "\n" +
		indent + "}"
}

// ourGroup is a SessionStart group holding switchboard's hook alone, as
// Install adds it, at SessionStart's indent.
func ourGroup(path string) string {
	return "      {\n        \"hooks\": [\n" + ours("          ", path) + "\n        ]\n      }"
}

// onlyOurs is the settings Install leaves where there were none.
var onlyOurs = "{\n  \"hooks\": {\n    \"SessionStart\": [\n" + ourGroup(switchboard) + "\n    ]\n  }\n}\n"

// othersBefore and othersAfter are settings with others of the user's,
// written as Claude Code writes them, keys in no sorted order, before and
// after Install adds switchboard's hook: only that is new.
var (
	othersHead = `{
  "model": "opus",
  "env": {
    "ZED": "1",
    "ALPHA": "<b> & é é"
  },
  "permissions": {
    "allow": [],
    "deny": [
      "Bash(rm:*)"
    ]
  },
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "Bash",
        "hooks": [
          {
            "type": "command",
            "command": "switchboard status --session \"$SESSION\" && true"
          }
        ]
      }
    ],
    "SessionStart": [
      {
        "matcher": "startup",
        "hooks": [
          {
            "type": "command",
            "command": "echo switchboard hook session-start"
          }
        ]
      }`
	othersTail = `
    ],
    "Stop": []
  },
  "statusLine": {
    "type": "command",
    "command": "~/bin/statusline"
  }
}
`
	othersBefore = othersHead + othersTail
	othersAfter  = othersHead + ",\n" + ourGroup(switchboard) + othersTail
)

// session is settings whose SessionStart holds groups.
func session(groups ...string) string {
	return "{\n  \"hooks\": {\n    \"SessionStart\": [\n" + strings.Join(groups, ",\n") + "\n    ]\n  }\n}\n"
}

// group is a SessionStart group holding hooks, each written at its indent.
func group(hooks ...string) string {
	return "      {\n        \"hooks\": [\n" + strings.Join(hooks, ",\n") + "\n        ]\n      }"
}

// matched is a SessionStart group under matcher, as JSON writes it,
// holding hooks.
func matched(matcher string, hooks ...string) string {
	return "      {\n        \"matcher\": " + matcher + ",\n        \"hooks\": [\n" + strings.Join(hooks, ",\n") + "\n        ]\n      }"
}

// oursCompact is switchboard's hook object, written without spaces.
var oursCompact = `{"type":"command","command":` + command(switchboard) + `,"timeout":5}`

// usersHook is a SessionStart hook of the user's.
const usersHook = "          {\n            \"type\": \"command\",\n            \"command\": \"~/bin/notes\"\n          }"

func TestInstall(t *testing.T) {
	tests := []struct {
		name string
		// before is the settings file before Install, or missing where nil.
		before *string
		want   string
		// wantOutcome is what Install says it did, InPlace leaving the file
		// as it was.
		wantOutcome sessionhook.Outcome
	}{
		{name: "missing", want: onlyOurs, wantOutcome: sessionhook.Installed},
		{name: "empty", before: new(""), want: onlyOurs, wantOutcome: sessionhook.Installed},
		{name: "only spaces", before: new(" \n\t\n"), want: onlyOurs, wantOutcome: sessionhook.Installed},
		{name: "an empty object", before: new("{}\n"), want: onlyOurs, wantOutcome: sessionhook.Installed},
		{
			name:        "other settings and hooks, a user's naming switchboard among them: ours added, theirs as they were",
			before:      new(othersBefore),
			want:        othersAfter,
			wantOutcome: sessionhook.Installed,
		},
		{
			name:        "hooks without SessionStart: SessionStart added after the others",
			before:      new("{\n  \"hooks\": {\n    \"Stop\": []\n  }\n}\n"),
			want:        "{\n  \"hooks\": {\n    \"Stop\": [],\n    \"SessionStart\": [\n" + ourGroup(switchboard) + "\n    ]\n  }\n}\n",
			wantOutcome: sessionhook.Installed,
		},
		{
			name:        "hooks given twice: the last, which Claude Code reads, taken, the first left as it is",
			before:      new(`{"hooks": [], "hooks": {}}`),
			want:        "{\n  \"hooks\": [],\n  \"hooks\": {\n    \"SessionStart\": [\n" + ourGroup(switchboard) + "\n    ]\n  }\n}\n",
			wantOutcome: sessionhook.Installed,
		},
		{
			name:        "ours, right: nothing written",
			before:      new(onlyOurs),
			want:        onlyOurs,
			wantOutcome: sessionhook.InPlace,
		},
		{
			name:        "ours, right, written another way: nothing written",
			before:      new(`{"hooks":{"SessionStart":[{"hooks":[{"timeout":5.0,"command":` + command(switchboard) + `,"type":"command"}]}]}}`),
			want:        `{"hooks":{"SessionStart":[{"hooks":[{"timeout":5.0,"command":` + command(switchboard) + `,"type":"command"}]}]}}`,
			wantOutcome: sessionhook.InPlace,
		},
		{
			name:        "ours, by another path: put right",
			before:      new(session(ourGroup(oldSwitchboard))),
			want:        onlyOurs,
			wantOutcome: sessionhook.PutRight,
		},
		{
			name: "ours, by another type and timeout, a key of its own kept: put right in place",
			before: new("{\n  \"hooks\": {\n    \"SessionStart\": [\n      {\n        \"hooks\": [\n          {\n            \"type\": \"prompt\",\n" +
				"            \"command\": " + command(switchboard) + ",\n            \"statusMessage\": \"checking\",\n            \"timeout\": 60\n" +
				"          }\n        ]\n      }\n    ]\n  }\n}\n"),
			want: "{\n  \"hooks\": {\n    \"SessionStart\": [\n      {\n        \"hooks\": [\n          {\n            \"type\": \"command\",\n" +
				"            \"command\": " + command(switchboard) + ",\n            \"statusMessage\": \"checking\",\n            \"timeout\": 5\n" +
				"          }\n        ]\n      }\n    ]\n  }\n}\n",
			wantOutcome: sessionhook.PutRight,
		},
		{
			name:        "ours, in a group with a user's hook, by another path: put right there",
			before:      new(session(group(usersHook, ours("          ", oldSwitchboard)))),
			want:        session(group(usersHook, ours("          ", switchboard))),
			wantOutcome: sessionhook.PutRight,
		},
		{
			name:        "ours, three times: the first put right, the others removed, with the group they leave empty",
			before:      new(session(ourGroup(oldSwitchboard), group(usersHook, ours("          ", switchboard)), ourGroup(switchboard))),
			want:        session(ourGroup(switchboard), group(usersHook)),
			wantOutcome: sessionhook.PutRight,
		},
		{
			name:        "ours, right, then again: the second removed",
			before:      new(session(ourGroup(switchboard), ourGroup(oldSwitchboard))),
			want:        onlyOurs,
			wantOutcome: sessionhook.PutRight,
		},
		{
			name:        `ours, right, under "*", which matches every session: in place`,
			before:      new(session(matched(`"*"`, ours("          ", switchboard)))),
			want:        session(matched(`"*"`, ours("          ", switchboard))),
			wantOutcome: sessionhook.InPlace,
		},
		{
			name:        "ours, right, under an empty matcher: in place",
			before:      new(session(matched(`""`, ours("          ", switchboard)))),
			want:        session(matched(`""`, ours("          ", switchboard))),
			wantOutcome: sessionhook.InPlace,
		},
		{
			name:        "ours, under a matcher alone: taken out, with its group, and ours put in a group of its own",
			before:      new(session(matched(`"startup"`, ours("          ", switchboard)))),
			want:        onlyOurs,
			wantOutcome: sessionhook.PutRight,
		},
		{
			name:        "ours, under compact with a user's hook: taken out, theirs left, and ours put in a group of its own",
			before:      new(session(matched(`"compact"`, usersHook, ours("          ", switchboard)))),
			want:        session(matched(`"compact"`, usersHook), ourGroup(switchboard)),
			wantOutcome: sessionhook.PutRight,
		},
		{
			name:        "ours, under a matcher before ours right: the one under the matcher taken out",
			before:      new(session(matched(`"resume"`, ours("          ", switchboard)), ourGroup(switchboard))),
			want:        onlyOurs,
			wantOutcome: sessionhook.PutRight,
		},
		{
			name:        "groups not shaped as Claude Code reads them: left as the user's",
			before:      new(session(`      "a group that isn't an object"`, `      {"hooks": "not an array"}`, `      {"matcher": "x"}`, group(`          "a hook that isn't an object"`))),
			want:        session(`      "a group that isn't an object"`, "      {\n        \"hooks\": \"not an array\"\n      }", "      {\n        \"matcher\": \"x\"\n      }", group(`          "a hook that isn't an object"`), ourGroup(switchboard)),
			wantOutcome: sessionhook.Installed,
		},
		{
			name:        "SessionStart given twice: the last read, where ours is right",
			before:      new("{\n  \"hooks\": {\n    \"SessionStart\": [],\n    \"SessionStart\": [\n" + ourGroup(switchboard) + "\n    ]\n  }\n}\n"),
			want:        "{\n  \"hooks\": {\n    \"SessionStart\": [],\n    \"SessionStart\": [\n" + ourGroup(switchboard) + "\n    ]\n  }\n}\n",
			wantOutcome: sessionhook.InPlace,
		},
		{
			name:        "a group's hooks given twice, the first ours: the last read, which isn't, so ours added",
			before:      new(`{"hooks":{"SessionStart":[{"hooks":[` + oursCompact + `],"hooks":[]}]}}`),
			want:        session("      {\n        \"hooks\": [\n"+ours("          ", switchboard)+"\n        ],\n        \"hooks\": []\n      }", ourGroup(switchboard)),
			wantOutcome: sessionhook.Installed,
		},
		{
			name:        "a hook's command given twice, the first ours: the last read, which isn't, so ours added",
			before:      new(`{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":` + command(switchboard) + `,"command":"~/bin/notes"}]}]}}`),
			want:        session(group("          {\n            \"type\": \"command\",\n            \"command\": "+command(switchboard)+",\n            \"command\": \"~/bin/notes\"\n          }"), ourGroup(switchboard)),
			wantOutcome: sessionhook.Installed,
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

			got, err := sessionhook.Install(path, switchboard)
			if err != nil || got != tt.wantOutcome {
				t.Fatalf("Install() = %v, %v, want %v", got, err, tt.wantOutcome)
			}
			if written := readFile(t, path); written != tt.want {
				t.Errorf("Install() wrote\n%s\nwant\n%s", written, tt.want)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			wantMode := fs.FileMode(0o600)
			if tt.before == nil {
				wantMode = 0o644
			}
			if info.Mode().Perm() != wantMode {
				t.Errorf("the settings file's mode is %v, want %v", info.Mode().Perm(), wantMode)
			}
			if tt.wantOutcome == sessionhook.InPlace && !info.ModTime().Equal(long) {
				t.Errorf("the settings file was modified at %v, want it left as it was, at %v", info.ModTime(), long)
			}
		})
	}
}

func TestInstallRefuses(t *testing.T) {
	tests := []struct {
		name    string
		before  string
		wantErr string
	}{
		{name: "junk", before: "not json", wantErr: "line 1, column 2: invalid character 'o'"},
		{name: "junk on its third line", before: "{\n  \"model\": \"opus\",\n  x\n}\n", wantErr: "line 3, column 3: invalid character 'x'"},
		{name: "junk after letters of more than a byte", before: "{\"é\": 1 x}", wantErr: "line 1, column 9: invalid character 'x'"},
		{name: "cut short in a string", before: `{"model": "op`, wantErr: "it ends before its JSON does"},
		{name: "cut short after its brace", before: "{\n", wantErr: "it ends before its JSON does"},
		{name: "cut short after a member", before: `{"a": 1`, wantErr: "it ends before its JSON does"},
		{name: "cut short after a comma", before: `{"a": 1,`, wantErr: "it ends before its JSON does"},
		{name: "cut short after an object in it", before: `{"a": {"b": 1}`, wantErr: "it ends before its JSON does"},
		{name: "a bad last byte, not cut short", before: `{"a": 1 x`, wantErr: "line 1, column 9: invalid character 'x'"},
		{name: "a byte order mark", before: "\xef\xbb\xbf{}\n", wantErr: "it starts with a byte order mark, which JSON doesn't allow"},
		{name: "an array", before: "[]\n", wantErr: "not a JSON object"},
		{name: "a string", before: `"settings"`, wantErr: "not a JSON object"},
		{name: "an object, then more", before: "{} {}", wantErr: "not a JSON object: more follows it"},
		{name: "hooks an array", before: `{"hooks": []}`, wantErr: "hooks: not a JSON object"},
		{name: "hooks null", before: `{"hooks": null}`, wantErr: "hooks: not a JSON object"},
		{name: "SessionStart an object", before: `{"hooks": {"SessionStart": {}}}`, wantErr: "hooks.SessionStart: not a JSON array"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			writeFile(t, path, tt.before, 0o644)

			got, err := sessionhook.Install(path, switchboard)
			if err == nil || !strings.HasPrefix(err.Error(), path+": ") || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Install() = %v, %v, want an error naming %s and saying %q", got, err, path, tt.wantErr)
			}
			if written := readFile(t, path); written != tt.before {
				t.Errorf("Install() left\n%s\nwant it as it was\n%s", written, tt.before)
			}
		})
	}
}

func TestInstallWritesThroughALink(t *testing.T) {
	dir := t.TempDir()
	dotfiles := filepath.Join(dir, "dotfiles", "claude-settings.json")
	writeFile(t, dotfiles, "{}\n", 0o600)
	path := filepath.Join(dir, "claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("../dotfiles/claude-settings.json", path); err != nil {
		t.Fatal(err)
	}

	if got, err := sessionhook.Install(path, switchboard); err != nil || got != sessionhook.Installed {
		t.Fatalf("Install() = %v, %v, want %v", got, err, sessionhook.Installed)
	}
	if target, err := os.Readlink(path); err != nil || target != "../dotfiles/claude-settings.json" {
		t.Errorf("%s leads to %q (%v), want the link left as it was", path, target, err)
	}
	if written := readFile(t, dotfiles); written != onlyOurs {
		t.Errorf("the linked file holds\n%s\nwant\n%s", written, onlyOurs)
	}
	if info, err := os.Stat(dotfiles); err != nil || info.Mode().Perm() != 0o600 {
		t.Errorf("the linked file: %v, %v, want its mode, 0600, kept", info, err)
	}
}

func TestInstallByAPathWithAQuote(t *testing.T) {
	const quoted = "/Users/test/it's/switchboard"
	path := filepath.Join(t.TempDir(), "settings.json")

	if got, err := sessionhook.Install(path, quoted); err != nil || got != sessionhook.Installed {
		t.Fatalf("Install() = %v, %v, want %v", got, err, sessionhook.Installed)
	}
	want := `[ -x '/Users/test/it'\''s/switchboard' ] && '/Users/test/it'\''s/switchboard' hook session-start || true`
	if got := sessionhook.Command(quoted); got != want {
		t.Errorf("Command(%q) = %s, want %s", quoted, got, want)
	}
	if !strings.Contains(readFile(t, path), command(quoted)) {
		t.Errorf("the settings hold\n%s\nwant the command %s", readFile(t, path), command(quoted))
	}
	// Found again, by that path or another.
	for _, again := range []string{quoted, switchboard} {
		wantOutcome := sessionhook.InPlace
		if again != quoted {
			wantOutcome = sessionhook.PutRight
		}
		if got, err := sessionhook.Install(path, again); err != nil || got != wantOutcome {
			t.Errorf("Install(%q) again = %v, %v, want %v", again, got, err, wantOutcome)
		}
	}
	if got := readFile(t, path); got != onlyOurs {
		t.Errorf("the settings hold\n%s\nwant\n%s", got, onlyOurs)
	}
}

func TestInstallRefusesASwitchboardByAnotherName(t *testing.T) {
	for _, before := range []*string{nil, new("{}\n"), new(onlyOurs)} {
		path := filepath.Join(t.TempDir(), "settings.json")
		if before != nil {
			writeFile(t, path, *before, 0o644)
		}

		got, err := sessionhook.Install(path, "/Users/test/bin/sb")
		if want := "switchboard is running as sb here, so the session hook isn't installed: run setup as switchboard"; err == nil || err.Error() != want {
			t.Errorf("Install() = %v, %v, want the error %q", got, err, want)
		}
		if before == nil {
			if _, err := os.Stat(path); !errors.Is(err, fs.ErrNotExist) {
				t.Errorf("%s: %v, want nothing written", path, err)
			}
		} else if written := readFile(t, path); written != *before {
			t.Errorf("Install() left\n%s\nwant it as it was\n%s", written, *before)
		}
	}
}

func TestOursIsKnownByItsShape(t *testing.T) {
	tests := []struct {
		name    string
		command string
		ours    bool
	}{
		{name: "ours", command: sessionhook.Command(switchboard), ours: true},
		{name: "ours, by a path with quotes", command: sessionhook.Command("/x/'a'/''/switchboard"), ours: true},
		{name: "ours, by a path with spaces", command: sessionhook.Command("/Users/test/My Apps/switchboard"), ours: true},
		{name: "two paths", command: "[ -x '" + switchboard + "' ] && '" + oldSwitchboard + "' hook session-start || true"},
		{name: "a binary named otherwise", command: sessionhook.Command("/opt/homebrew/bin/switchboard-notes")},
		{name: "a binary in a directory named switchboard", command: sessionhook.Command("/opt/switchboard/bin/notes")},
		{name: "unguarded", command: "'" + switchboard + "' hook session-start"},
		{name: "unquoted", command: "[ -x " + switchboard + " ] && " + switchboard + " hook session-start || true"},
		{name: "another subcommand", command: "[ -x '/x' ] && '/x' status --session || true"},
		{name: "something after", command: sessionhook.Command(switchboard) + "; echo hi"},
		{name: "an unended quote", command: "[ -x '/x ] && '/x' hook session-start || true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			settings := `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":` + jsonString(tt.command) + `}]}]}}`
			path := filepath.Join(t.TempDir(), "settings.json")
			writeFile(t, path, settings, 0o644)

			want := sessionhook.Installed
			if tt.ours {
				want = sessionhook.PutRight
			}
			if got, err := sessionhook.Install(path, switchboard); err != nil || got != want {
				t.Errorf("Install() over %s = %v, %v, want %v", tt.command, got, err, want)
			}
		})
	}
}

func jsonString(s string) string {
	b, err := json.Marshal(s)
	if err != nil {
		panic(err)
	}
	return string(b)
}

func TestInstallFailsWhereItCantWrite(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("root can write to any directory")
	}
	dir := filepath.Join(t.TempDir(), "claude")
	if err := os.MkdirAll(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })

	path := filepath.Join(dir, "settings.json")
	got, err := sessionhook.Install(path, switchboard)
	if err == nil || !strings.HasPrefix(err.Error(), path+": write Claude Code's settings: ") {
		t.Errorf("Install() = %v, %v, want it to fail writing the settings", got, err)
	}
	if !errors.Is(err, fs.ErrPermission) {
		t.Errorf("Install() error = %v, want %v within it", err, fs.ErrPermission)
	}
}

func writeFile(t *testing.T, path, content string, mode fs.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestInstallStartsAgainFromAFileChangedMeanwhile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	writeFile(t, path, "{}\n", 0o644)
	reads := 0
	sessionhook.SetReadFile(t, func(name string) ([]byte, error) {
		reads++
		if reads == 2 {
			// Claude Code writes the file between Install's read and its write.
			writeFile(t, path, "{\n  \"model\": \"sonnet\"\n}\n", 0o644)
		}
		return os.ReadFile(name)
	})

	if got, err := sessionhook.Install(path, switchboard); err != nil || got != sessionhook.Installed {
		t.Fatalf("Install() = %v, %v, want %v", got, err, sessionhook.Installed)
	}
	want := "{\n  \"model\": \"sonnet\",\n  \"hooks\": {\n    \"SessionStart\": [\n" + ourGroup(switchboard) + "\n    ]\n  }\n}\n"
	if written := readFile(t, path); written != want {
		t.Errorf("Install() wrote\n%s\nwant\n%s", written, want)
	}
	checkNothingLeftBeside(t, path)
}

// checkNothingLeftBeside checks no file Install wrote beside the settings
// file at path, to rename into place, is left there.
func checkNothingLeftBeside(t *testing.T, path string) {
	t.Helper()
	left, err := filepath.Glob(filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".*"))
	if err != nil || len(left) > 0 {
		t.Errorf("beside %s: %v (%v), want nothing left", path, left, err)
	}
}

func TestInstallGivesUpOnAFileThatKeepsChanging(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	writeFile(t, path, "{}\n", 0o644)
	reads := 0
	sessionhook.SetReadFile(t, func(name string) ([]byte, error) {
		reads++
		if reads%2 == 0 {
			writeFile(t, path, fmt.Sprintf("{\"changes\": %d}\n", reads), 0o644)
		}
		return os.ReadFile(name)
	})

	got, err := sessionhook.Install(path, switchboard)
	if want := path + " kept changing as the session hook was written into it, so it's left as it is: run setup again"; err == nil || err.Error() != want {
		t.Errorf("Install() = %v, %v, want the error %q", got, err, want)
	}
	if reads != 8 {
		t.Errorf("Install() read the file %d times, want 8: twice a try, starting again 3 times", reads)
	}
	if written := readFile(t, path); written != "{\"changes\": 8}\n" {
		t.Errorf("the settings file holds\n%s\nwant the last change, nothing of Install's", written)
	}
	checkNothingLeftBeside(t, path)
}

func TestTheCommandRunsUnderSh(t *testing.T) {
	ran := filepath.Join(t.TempDir(), "ran")
	switchboard := filepath.Join(t.TempDir(), `it's a $HOME \n dir`, "switchboard")
	writeFile(t, switchboard, "#!/bin/sh\nprintf '%s\\n' \"$*\" > '"+ran+"'\n", 0o700)
	sh := func() error {
		return exec.Command("/bin/sh", "-c", sessionhook.Command(switchboard)).Run()
	}

	if err := sh(); err != nil {
		t.Fatalf("sh -c %s: %v", sessionhook.Command(switchboard), err)
	}
	if got := readFile(t, ran); got != "hook session-start\n" {
		t.Errorf("switchboard ran with %q, want %q", got, "hook session-start\n")
	}

	// One that fails fails no session.
	writeFile(t, switchboard, "#!/bin/sh\nexit 3\n", 0o700)
	if err := sh(); err != nil {
		t.Errorf("sh -c %s, switchboard failing: %v, want it to succeed all the same", sessionhook.Command(switchboard), err)
	}

	if err := os.Remove(switchboard); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(ran); err != nil {
		t.Fatal(err)
	}
	if err := sh(); err != nil {
		t.Errorf("sh -c %s, switchboard gone: %v, want it to succeed doing nothing", sessionhook.Command(switchboard), err)
	}
	if _, err := os.Stat(ran); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("%s: %v, want nothing run", ran, err)
	}
}
