package tokens_test

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/tokens"
)

const workToken = "test-token-work"

func TestRead(t *testing.T) {
	tests := []struct {
		name string
		// content is what the token file holds, and mode its mode, when put
		// doesn't put something else in its place; zero leaves no file.
		content string
		mode    fs.FileMode
		// put puts something at the token file's path, where it's given.
		put func(t *testing.T, path string)
		// otherUser has the file be another user's than the store's.
		otherUser bool
		want      string
		// wantErr is the error Read fails with, %s standing for the token
		// file's path, and wantMissing whether it's ErrMissing.
		wantErr     string
		wantMissing bool
	}{
		{name: "a token", content: workToken, mode: 0o600, want: workToken},
		{name: "the whitespace around it ignored", content: "\n " + workToken + "\r\n", mode: 0o600, want: workToken},
		{name: "readable by its owner alone", content: workToken, mode: 0o400, want: workToken},
		{name: "runnable, by its owner alone", content: workToken, mode: 0o700, want: workToken},
		{name: "as large as a token file can be", content: strings.Repeat("x", 4<<10), mode: 0o600, want: strings.Repeat("x", 4<<10)},
		{name: "missing", wantErr: "token missing: write it to %s", wantMissing: true},
		{name: "empty", content: "", mode: 0o600, wantErr: "token missing: write it to %s, which is empty", wantMissing: true},
		{name: "whitespace alone", content: " \n\t\n", mode: 0o600, wantErr: "token missing: write it to %s, which is empty", wantMissing: true},
		{name: "two tokens", content: workToken + " test-token-side", mode: 0o600, wantErr: "the token file %s holds more than a token: write the token alone to it"},
		{name: "a shell's export of it", content: "export CLAUDE_CODE_OAUTH_TOKEN=" + workToken + "\n", mode: 0o600, wantErr: "the token file %s holds more than a token: write the token alone to it"},
		{name: "larger than a token file can be", content: strings.Repeat("x", 4<<10+1), mode: 0o600, wantErr: "the token file %s holds more than a token: write the token alone to it"},
		{name: "readable by its group", content: workToken, mode: 0o640, wantErr: "other users can read the token file (mode 0640): chmod 600 %s"},
		{name: "readable by anyone", content: workToken, mode: 0o604, wantErr: "other users can read the token file (mode 0604): chmod 600 %s"},
		{name: "writable by its group", content: workToken, mode: 0o620, wantErr: "other users can write the token file (mode 0620): chmod 600 %s"},
		{name: "writable by anyone", content: workToken, mode: 0o602, wantErr: "other users can write the token file (mode 0602): chmod 600 %s"},
		{name: "readable and writable by anyone", content: workToken, mode: 0o666, wantErr: "other users can read and write the token file (mode 0666): chmod 600 %s"},
		{name: "another user's", content: workToken, mode: 0o600, otherUser: true, wantErr: "the token file %s is another user's: replace it with one of your own"},
		{name: "a directory", put: mkdir, wantErr: "the token file %s isn't a file: replace it with one holding the token"},
		{name: "a link to a file its owner alone can read", put: linkTo(0o600), want: workToken},
		{name: "a link to a file others can read", put: linkTo(0o644), wantErr: "other users can read the token file (mode 0644): chmod 600 %s"},
		{name: "unreadable", content: workToken, mode: 0o200, wantErr: "read the token file: open %s: permission denied"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mode != 0 && tt.mode&0o400 == 0 && os.Getuid() == 0 {
				t.Skip("root reads any file")
			}
			uid := os.Getuid()
			if tt.otherUser {
				uid++
			}
			store := tokens.NewStore(t.TempDir(), uid)
			path := store.Path("work")
			switch {
			case tt.put != nil:
				mkdirPrivate(t, filepath.Dir(path))
				tt.put(t, path)
			case tt.mode != 0:
				writeFile(t, path, tt.content, tt.mode)
			}

			token, err := store.Read("work")
			if tt.wantErr == "" {
				if err != nil || token.Reveal() != tt.want {
					t.Errorf("Read() = %q, %v, want %q", token.Reveal(), err, tt.want)
				}
				return
			}
			if want := fmt.Sprintf(tt.wantErr, path); err == nil || err.Error() != want {
				t.Errorf("Read() error = %v, want %q", err, want)
			}
			if missing := errors.Is(err, tokens.ErrMissing); missing != tt.wantMissing {
				t.Errorf("Read() error is ErrMissing: %v, want %v", missing, tt.wantMissing)
			}
			if err != nil && strings.Contains(err.Error(), workToken) {
				t.Errorf("Read() error = %q, which shows the token", err)
			}
		})
	}
}

func TestReadIsTheStateDirectorys(t *testing.T) {
	state := t.TempDir()
	writeFile(t, filepath.Join(state, "tokens", "work"), workToken, 0o600)

	token, err := tokens.NewStore(state, os.Getuid()).Read("work")
	if err != nil || token.Reveal() != workToken {
		t.Errorf("Read() = %q, %v, want the token in <state dir>/tokens/work", token.Reveal(), err)
	}
}

func TestWrite(t *testing.T) {
	tests := []struct {
		name string
		// before puts what's there before the token is written, under the
		// state directory, where it's given.
		before func(t *testing.T, state string)
	}{
		{name: "with no tokens directory yet", before: func(*testing.T, string) {}},
		{name: "with no state directory yet", before: func(t *testing.T, state string) {
			if err := os.Remove(state); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "into a tokens directory others can use", before: func(t *testing.T, state string) {
			if err := os.Mkdir(filepath.Join(state, "tokens"), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(filepath.Join(state, "tokens"), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "over a token file others could read", before: func(t *testing.T, state string) {
			writeFile(t, filepath.Join(state, "tokens", "work"), "test-token-stale", 0o644)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := filepath.Join(t.TempDir(), "switchboard")
			if err := os.Mkdir(state, 0o700); err != nil {
				t.Fatal(err)
			}
			tt.before(t, state)
			store := tokens.NewStore(state, os.Getuid())

			if err := store.Write("work", parse(t, workToken)); err != nil {
				t.Fatalf("Write() error = %v", err)
			}
			checkMode(t, filepath.Join(state, "tokens"), fs.ModeDir|0o700)
			checkMode(t, store.Path("work"), 0o600)
			if data, err := os.ReadFile(store.Path("work")); err != nil || string(data) != workToken+"\n" {
				t.Errorf("the token file holds %q (%v), want the token and a newline", data, err)
			}
			if entries, err := os.ReadDir(filepath.Join(state, "tokens")); err != nil || len(entries) != 1 {
				t.Errorf("the tokens directory holds %v (%v), want the token file alone", entries, err)
			}
			if token, err := store.Read("work"); err != nil || token.Reveal() != workToken {
				t.Errorf("Read() = %q, %v, want the token written", token.Reveal(), err)
			}
		})
	}
}

func TestWriteLeavesNothingBehindWhenItFails(t *testing.T) {
	store := tokens.NewStore(t.TempDir(), os.Getuid())
	// A directory, with something in it, where the token file goes.
	writeFile(t, filepath.Join(store.Path("work"), "kept"), "kept", 0o600)

	err := store.Write("work", parse(t, workToken))
	if err == nil || !strings.HasPrefix(err.Error(), "write the token file: ") {
		t.Fatalf("Write() error = %v, want it to fail writing the token file", err)
	}
	if strings.Contains(err.Error(), workToken) {
		t.Errorf("Write() error = %q, which shows the token", err)
	}
	entries, err := os.ReadDir(filepath.Dir(store.Path("work")))
	if err != nil || len(entries) != 1 || entries[0].Name() != "work" {
		t.Errorf("the tokens directory holds %v (%v), want what was there alone", entries, err)
	}
}

func TestRemove(t *testing.T) {
	store := tokens.NewStore(t.TempDir(), os.Getuid())
	if err := store.Write("work", parse(t, workToken)); err != nil {
		t.Fatal(err)
	}

	if err := store.Remove("work"); err != nil {
		t.Fatalf("Remove() error = %v", err)
	}
	if _, err := store.Read("work"); !errors.Is(err, tokens.ErrMissing) {
		t.Errorf("once removed, Read() error = %v, want %v", err, tokens.ErrMissing)
	}
	if err := store.Remove("work"); err != nil {
		t.Errorf("Remove() of a token file that's gone: %v, want nothing to do", err)
	}
}

func TestRemoveFails(t *testing.T) {
	store := tokens.NewStore(t.TempDir(), os.Getuid())
	// A directory, with something in it, where the token file goes.
	writeFile(t, filepath.Join(store.Path("work"), "kept"), "kept", 0o600)

	err := store.Remove("work")
	if err == nil || !strings.HasPrefix(err.Error(), "remove the token file: ") {
		t.Errorf("Remove() error = %v, want it to fail removing the token file", err)
	}
}

func TestHas(t *testing.T) {
	tests := []struct {
		name string
		// put puts what's at the token file's path, when it's given.
		put  func(t *testing.T, path string)
		want bool
	}{
		{name: "no token file"},
		{name: "a token file", put: func(t *testing.T, path string) { writeFile(t, path, workToken, 0o600) }, want: true},
		{name: "a token file it can't use", put: func(t *testing.T, path string) { writeFile(t, path, workToken, 0o644) }, want: true},
		{name: "a link to one", put: linkTo(0o600), want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := tokens.NewStore(t.TempDir(), os.Getuid())
			if tt.put != nil {
				mkdirPrivate(t, filepath.Dir(store.Path("work")))
				tt.put(t, store.Path("work"))
			}

			if got := store.Has("work"); got != tt.want {
				t.Errorf("Has() = %v, want %v", got, tt.want)
			}
		})
	}
}

// parse returns the token text holds.
func parse(t *testing.T, text string) tokens.Token {
	t.Helper()
	token, err := tokens.Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

// writeFile writes content to a file at path with mode, making its directory
// first.
func writeFile(t *testing.T, path, content string, mode fs.FileMode) {
	t.Helper()
	mkdirPrivate(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func mkdirPrivate(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
}

// mkdir puts a directory at path.
func mkdir(t *testing.T, path string) {
	t.Helper()
	mkdirPrivate(t, path)
}

// linkTo puts a link at path to a file elsewhere, holding the token, with
// mode.
func linkTo(mode fs.FileMode) func(t *testing.T, path string) {
	return func(t *testing.T, path string) {
		t.Helper()
		target := filepath.Join(t.TempDir(), "secrets", "work")
		writeFile(t, target, workToken+"\n", mode)
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
	}
}

// checkMode checks what's at path has mode.
func checkMode(t *testing.T, path string, mode fs.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode() != mode {
		t.Errorf("%s has mode %v, want %v", path, info.Mode(), mode)
	}
}
