package skill_test

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/skill"
)

// The skill's version, and the SHA-256 of its text at that version: a change
// to SKILL.md fails TestTheVersionIsBumpedWhenTheTextChanges until its version
// is bumped, and both are recorded here afresh.
const (
	recordedVersion = 4
	recordedDigest  = "143d1549e3b88cfda5c5fa0050c5fc9e02021dabfdd7da5ec778f903ffa3ccdf"
)

func TestTheVersionIsBumpedWhenTheTextChanges(t *testing.T) {
	sum := sha256.Sum256(skillText(t))
	digest := hex.EncodeToString(sum[:])
	switch version := skill.Version(); {
	case version == recordedVersion && digest != recordedDigest:
		t.Errorf("SKILL.md changed, but not its version: bump the version it carries, so the router rewrites the copies installed, then record it here with the text's digest, %s", digest)
	case version != recordedVersion || digest != recordedDigest:
		t.Errorf("record the skill's version, %d, here, with its text's digest, %s", version, digest)
	}
}

func TestTheSkillIsOneClaudeCodeReads(t *testing.T) {
	text := string(skillText(t))
	frontmatter, body, closed := strings.Cut(strings.TrimPrefix(text, "---\n"), "\n---\n")
	if !strings.HasPrefix(text, "---\n") || !closed {
		t.Fatalf("SKILL.md holds\n%s\nwant it to open with frontmatter between two --- lines", text)
	}
	fields := make(map[string]string)
	for line := range strings.Lines(frontmatter) {
		key, value, _ := strings.Cut(strings.TrimSuffix(line, "\n"), ": ")
		fields[key] = value
	}
	if keys := slices.Sorted(maps.Keys(fields)); !slices.Equal(keys, []string{"description", "name"}) {
		t.Errorf("the frontmatter's keys are %q, want name and description alone: the skill needs no other, and one Claude Code doesn't know could trip it", keys)
	}
	path, err := skill.Path(envFrom(map[string]string{"CLAUDE_CONFIG_DIR": "/claude"}), noHome)
	if dir := filepath.Base(filepath.Dir(path)); err != nil || fields["name"] != dir {
		t.Errorf("the skill's name is %q, want %q, the directory it's installed in (%v)", fields["name"], dir, err)
	}
	if description := fields["description"]; description == "" || strings.Contains(description, ": ") || strings.Contains(description, " #") {
		t.Errorf("the description is %q, want one that reads as a plain YAML string: not empty, and with no %q or %q", description, ": ", " #")
	}
	if version := skill.Version(); version < 1 || !strings.Contains(body, versionLine(version)) {
		t.Errorf("the skill's version is %d, want one of at least 1, carried in its body as %q", version, versionLine(version))
	}
}

func TestPath(t *testing.T) {
	home := func() (string, error) { return "/home/tester", nil }
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "CLAUDE_CONFIG_DIR",
			env:  map[string]string{"CLAUDE_CONFIG_DIR": "/claude"},
			want: "/claude/skills/switchboard/SKILL.md",
		},
		{
			name: "the home directory",
			want: "/home/tester/.claude/skills/switchboard/SKILL.md",
		},
		{
			name: "an empty CLAUDE_CONFIG_DIR counts as unset",
			env:  map[string]string{"CLAUDE_CONFIG_DIR": ""},
			want: "/home/tester/.claude/skills/switchboard/SKILL.md",
		},
		{
			name: "a relative CLAUDE_CONFIG_DIR taken as it is, as Claude Code takes it",
			env:  map[string]string{"CLAUDE_CONFIG_DIR": "claude"},
			want: "claude/skills/switchboard/SKILL.md",
		},
		{
			name: "XDG's directories don't move it, as they don't move Claude Code's",
			env:  map[string]string{"XDG_CONFIG_HOME": "/xdg/config", "XDG_DATA_HOME": "/xdg/data"},
			want: "/home/tester/.claude/skills/switchboard/SKILL.md",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := skill.Path(envFrom(tt.env), home)
			if err != nil || got != tt.want {
				t.Errorf("Path() = %q, %v, want %q", got, err, tt.want)
			}
		})
	}
}

func TestPathWithoutHomeDirectory(t *testing.T) {
	if _, err := skill.Path(envFrom(map[string]string{"CLAUDE_CONFIG_DIR": "/claude"}), noHome); err != nil {
		t.Errorf("Path() with CLAUDE_CONFIG_DIR set: error = %v, want none", err)
	}
	if _, err := skill.Path(envFrom(nil), noHome); !errors.Is(err, errNoHome) {
		t.Errorf("Path() with nothing set: error = %v, want %v", err, errNoHome)
	}
}

func TestInstall(t *testing.T) {
	current := string(skillText(t))
	tests := []struct {
		name string
		// lay lays out Claude Code's config directory, at config, before the
		// skill is installed.
		lay func(t *testing.T, config string)
	}{
		{name: "with no config directory yet", lay: func(*testing.T, string) {}},
		{name: "with no skills yet", lay: func(t *testing.T, config string) { mkdir(t, config) }},
		{name: "in place of a copy edited by hand", lay: installed(current + "\nEdited by hand.\n")},
		{name: "in place of an older copy", lay: installed(asVersion(t, skill.Version()-1))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := filepath.Join(t.TempDir(), ".claude")
			tt.lay(t, config)
			path := skillIn(config)

			if err := skill.Install(path); err != nil {
				t.Fatalf("Install() error = %v", err)
			}
			checkInstalled(t, path)
		})
	}
}

func TestRefresh(t *testing.T) {
	current := string(skillText(t))
	version := skill.Version()
	withVersionLine := func(line string) string { return strings.Replace(current, versionLine(version), line, 1) }
	tests := []struct {
		name string
		// installed is what the copy installed holds, and mode its mode,
		// 0644 when it's zero.
		installed string
		mode      fs.FileMode
		want      skill.Refreshed
	}{
		{
			name:      "an older copy",
			installed: asVersion(t, version-1),
			want:      skill.Refreshed{Installed: true, Was: version - 1, Rewritten: true},
		},
		{
			name:      "a copy carrying no version",
			installed: withVersionLine(""),
			want:      skill.Refreshed{Installed: true, Rewritten: true},
		},
		{
			name:      "a copy whose version isn't a number",
			installed: withVersionLine("<!-- switchboard skill version: two -->"),
			want:      skill.Refreshed{Installed: true, Rewritten: true},
		},
		{
			name:      "a copy whose version is too large to read",
			installed: withVersionLine("<!-- switchboard skill version: 99999999999999999999 -->"),
			want:      skill.Refreshed{Installed: true, Rewritten: true},
		},
		{
			name:      "a copy whose version isn't on a line of its own",
			installed: withVersionLine("Edited by hand. " + versionLine(version)),
			want:      skill.Refreshed{Installed: true, Rewritten: true},
		},
		{
			name:      "a current copy that can't be read",
			installed: current,
			mode:      0o200,
			want:      skill.Refreshed{Installed: true, Rewritten: true},
		},
		{
			name:      "a current copy",
			installed: current,
			want:      skill.Refreshed{Installed: true, Was: version},
		},
		{
			name:      "a current copy, edited by hand",
			installed: current + "\nEdited by hand.\n",
			want:      skill.Refreshed{Installed: true, Was: version},
		},
		{
			name:      "a newer copy",
			installed: asVersion(t, version+1),
			want:      skill.Refreshed{Installed: true, Was: version + 1},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.mode != 0 && tt.mode&0o400 == 0 && os.Getuid() == 0 {
				t.Skip("root reads any file")
			}
			path := skillIn(t.TempDir())
			write(t, path, tt.installed, cmp.Or(tt.mode, 0o644))

			got, err := skill.Refresh(path)
			if err != nil || got != tt.want {
				t.Fatalf("Refresh() = %+v, %v, want %+v", got, err, tt.want)
			}
			if tt.want.Rewritten {
				checkInstalled(t, path)
				return
			}
			if data, err := os.ReadFile(path); err != nil || string(data) != tt.installed {
				t.Errorf("the copy holds\n%s\n(%v), want it left as it was", data, err)
			}
		})
	}
}

func TestRefreshInstallsNoneWhereThereIsNone(t *testing.T) {
	tests := []struct {
		name string
		// lay lays out Claude Code's config directory, at config.
		lay func(t *testing.T, config string)
	}{
		{name: "no config directory", lay: func(*testing.T, string) {}},
		{name: "no skills", lay: func(t *testing.T, config string) { mkdir(t, config) }},
		{name: "other skills alone", lay: func(t *testing.T, config string) {
			write(t, filepath.Join(config, "skills", "other", "SKILL.md"), "---\nname: other\n---\n", 0o644)
		}},
		{name: "the skill's directory, empty", lay: func(t *testing.T, config string) { mkdir(t, filepath.Dir(skillIn(config))) }},
		{name: "a link to nowhere", lay: func(t *testing.T, config string) {
			mkdir(t, filepath.Dir(skillIn(config)))
			if err := os.Symlink(filepath.Join(config, "nowhere"), skillIn(config)); err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			config := filepath.Join(root, ".claude")
			tt.lay(t, config)
			before := tree(t, root)

			got, err := skill.Refresh(skillIn(config))
			if err != nil || got != (skill.Refreshed{}) {
				t.Errorf("Refresh() = %+v, %v, want nothing found, and no error", got, err)
			}
			if after := tree(t, root); !slices.Equal(after, before) {
				t.Errorf("Refresh() left %q, want what was there alone, %q", after, before)
			}
		})
	}
}

func TestADirectoryWhereTheSkillGoes(t *testing.T) {
	path := skillIn(t.TempDir())
	mkdir(t, filepath.Join(path, "kept"))

	if err := skill.Install(path); err == nil {
		t.Error("Install() succeeded, want it to fail to replace a directory")
	}
	if got, err := skill.Refresh(path); err == nil || got != (skill.Refreshed{}) {
		t.Errorf("Refresh() = %+v, %v, want it to fail to replace a directory", got, err)
	}
	for _, dir := range []string{path, filepath.Dir(path)} {
		if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
			t.Errorf("%s holds %v (%v), want what was there alone", dir, entries, err)
		}
	}
}

var errNoHome = errors.New("no home directory")

// noHome is a home directory that can't be found.
func noHome() (string, error) { return "", errNoHome }

// envFrom returns a getenv backed by vars, so tests never read the real
// environment.
func envFrom(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

// skillText is the skill switchboard carries: SKILL.md, which it embeds.
func skillText(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile("SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// versionLine is the line of the skill that carries version.
func versionLine(version int) string {
	return fmt.Sprintf("<!-- switchboard skill version: %d -->", version)
}

// asVersion is the skill's text, carrying version in place of its own.
func asVersion(t *testing.T, version int) string {
	t.Helper()
	text, line := string(skillText(t)), versionLine(skill.Version())
	if !strings.Contains(text, line) {
		t.Fatalf("SKILL.md doesn't carry its version as %q", line)
	}
	return strings.Replace(text, line, versionLine(version), 1)
}

// skillIn is where the skill goes in Claude Code's config directory, config.
func skillIn(config string) string {
	return filepath.Join(config, "skills", "switchboard", "SKILL.md")
}

// installed lays out a config directory with a copy of the skill holding
// content.
func installed(content string) func(t *testing.T, config string) {
	return func(t *testing.T, config string) { write(t, skillIn(config), content, 0o644) }
}

// checkInstalled checks that the skill at path is the one switchboard
// carries, written whole, readable by all, and alone in its directory.
func checkInstalled(t *testing.T, path string) {
	t.Helper()
	if data, err := os.ReadFile(path); err != nil || string(data) != string(skillText(t)) {
		t.Errorf("the skill holds\n%s\n(%v), want the one switchboard carries", data, err)
	}
	if info, err := os.Stat(path); err != nil || info.Mode() != 0o644 {
		t.Errorf("the skill's mode = %v (%v), want %v", info.Mode(), err, fs.FileMode(0o644))
	}
	if entries, err := os.ReadDir(filepath.Dir(path)); err != nil || len(entries) != 1 {
		t.Errorf("the skill's directory holds %v (%v), want the skill alone", entries, err)
	}
}

// tree lists everything under root, links unfollowed.
func tree(t *testing.T, root string) []string {
	t.Helper()
	var paths []string
	err := filepath.WalkDir(root, func(path string, _ fs.DirEntry, err error) error {
		paths = append(paths, path)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return paths
}

// write writes content to a file at path, with the mode given, creating its
// directories.
func write(t *testing.T, path, content string, mode fs.FileMode) {
	t.Helper()
	mkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, mode); err != nil {
		t.Fatal(err)
	}
}

func mkdir(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
}
