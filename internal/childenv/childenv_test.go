package childenv_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/childenv"
)

func TestMinimal(t *testing.T) {
	env := map[string]string{
		"PATH":                    "/usr/bin:/bin",
		"HOME":                    "/home/tester",
		"TMPDIR":                  "/tmp/tester",
		"LANG":                    "en_GB.UTF-8",
		"CLAUDE_TOKEN_WORK":       "test-token-work",
		"CLAUDE_CODE_OAUTH_TOKEN": "test-token-oauth",
		"ANTHROPIC_API_KEY":       "test-key",
		"TERM":                    "xterm-256color",
		// The listeners a router that replaced itself was handed, which
		// nothing it runs may take for its own.
		"SWITCHBOARD_LISTENERS": "control=9,proxy=8",
	}

	got := childenv.Minimal(func(key string) string { return env[key] })
	if want := []string{"PATH=/usr/bin:/bin", "HOME=/home/tester", "TMPDIR=/tmp/tester", "LANG=en_GB.UTF-8"}; !slices.Equal(got, want) {
		t.Errorf("Minimal() = %q, want %q", got, want)
	}
}

func TestMinimalOfNothingSetIsAnEmptyEnvironment(t *testing.T) {
	if got := childenv.Minimal(func(string) string { return "" }); got == nil || len(got) > 0 {
		t.Errorf("Minimal() = %#v, want an empty environment, not nil, which exec.Cmd takes for the whole of this one", got)
	}
}

func TestBeside(t *testing.T) {
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	at := func(parts ...string) string { return filepath.Join(append([]string{root}, parts...)...) }
	program := writeProgram(t, at("bin", "claude"))
	// npm's claude, a link into its package; a link to a program in another
	// directory; and one to a program in the same.
	writeProgram(t, at("npm", "lib", "claude-code", "cli.js"))
	link(t, filepath.Join("..", "lib", "claude-code", "cli.js"), at("npm", "bin", "claude"))
	link(t, program, at("links", "claude"))
	writeProgram(t, at("versions", "claude-2.1.300"))
	link(t, "claude-2.1.300", at("versions", "claude"))
	tests := []struct {
		name string
		env  []string
		path string
		want []string
	}{
		{
			name: "a program, its directory first",
			env:  []string{"PATH=/usr/bin:/bin", "HOME=/home/tester"},
			path: program,
			want: []string{"PATH=" + at("bin") + ":/usr/bin:/bin", "HOME=/home/tester"},
		},
		{
			name: "through a link, the link's directory, then where it leads",
			env:  []string{"HOME=/home/tester", "PATH=/usr/bin:/bin"},
			path: at("npm", "bin", "claude"),
			want: []string{"HOME=/home/tester", "PATH=" + at("npm", "bin") + ":" + at("npm", "lib", "claude-code") + ":/usr/bin:/bin"},
		},
		{
			name: "through a link to another directory",
			env:  []string{"PATH=/usr/bin:/bin"},
			path: at("links", "claude"),
			want: []string{"PATH=" + at("links") + ":" + at("bin") + ":/usr/bin:/bin"},
		},
		{
			name: "through a link in the same directory, it once",
			env:  []string{"PATH=/usr/bin:/bin"},
			path: at("versions", "claude"),
			want: []string{"PATH=" + at("versions") + ":/usr/bin:/bin"},
		},
		{
			name: "without a PATH, those directories alone",
			env:  []string{"HOME=/home/tester"},
			path: program,
			want: []string{"HOME=/home/tester", "PATH=" + at("bin")},
		},
		{
			name: "a program that isn't there, its directory",
			env:  []string{"PATH=/usr/bin:/bin"},
			path: at("missing", "claude"),
			want: []string{"PATH=" + at("missing") + ":/usr/bin:/bin"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			env := slices.Clone(tt.env)

			if got := childenv.Beside(env, tt.path); !slices.Equal(got, tt.want) {
				t.Errorf("Beside() = %q, want %q", got, tt.want)
			}
			if !slices.Equal(env, tt.env) {
				t.Errorf("Beside() changed the environment it was given to %q", env)
			}
		})
	}
}

// writeProgram writes a program at path, making its directory, and returns
// path.
func writeProgram(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

// link makes a symbolic link at path leading to target, making its
// directory.
func link(t *testing.T, target, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
}
