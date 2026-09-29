// Package skill is the Claude Code skill switchboard carries, which tells
// Claude what switchboard does under claude, so no session needs it
// explained: its text, embedded with its version, and the copy installed
// where Claude Code reads skills. Switchboard owns that copy, and overwrites
// edits to it.
package skill

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/leeovery/switchboard/internal/atomicfile"
)

// text is the skill as it's installed, carrying its version, which is bumped
// whenever the text changes.
//
//go:embed SKILL.md
var text []byte

// versionLine is the line of the skill that carries its version: an HTML
// comment in its body, as Claude Code takes only the frontmatter keys it
// knows, and version isn't one.
var versionLine = regexp.MustCompile(`(?m)^<!-- switchboard skill version: (\d+) -->$`)

// Version returns the version of the skill this switchboard carries.
func Version() int {
	return versionOf(text)
}

// versionOf returns the version a copy of the skill carries, or 0, older than
// any, when it carries none that can be read.
func versionOf(skill []byte) int {
	match := versionLine.FindSubmatch(skill)
	if match == nil {
		return 0
	}
	version, err := strconv.Atoi(string(match[1]))
	if err != nil {
		return 0
	}
	return version
}

// Path returns where Claude Code reads the skill: skills/switchboard/SKILL.md
// in its config directory, which is $CLAUDE_CONFIG_DIR, else ~/.claude.
func Path(getenv func(string) string, homeDir func() (string, error)) (string, error) {
	dir := getenv("CLAUDE_CONFIG_DIR")
	if dir == "" {
		home, err := homeDir()
		if err != nil {
			return "", fmt.Errorf("locate Claude Code's config directory: %w", err)
		}
		dir = filepath.Join(home, ".claude")
	}
	return filepath.Join(dir, "skills", "switchboard", "SKILL.md"), nil
}

// Install writes the skill to path, in place of any file there, making its
// directory: whole, or not at all.
func Install(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create the skill's directory: %w", err)
	}
	if err := atomicfile.Write(path, text, 0o644); err != nil {
		return fmt.Errorf("write the skill: %w", err)
	}
	return nil
}

// Refreshed is what Refresh found installed, and what it did.
type Refreshed struct {
	// Installed is whether a copy of the skill is installed.
	Installed bool
	// Was is the installed copy's version, 0 when it can't be read.
	Was int
	// Rewritten is whether Refresh rewrote the copy with this switchboard's.
	Rewritten bool
}

// Refresh brings the copy of the skill installed at path up to date: it
// rewrites one whose version is older than this switchboard's, or can't be
// read, and leaves any other as it is. It never installs one that isn't
// there, as setup didn't, or the user removed it. It reports what it did, for
// its caller to log.
func Refresh(path string) (Refreshed, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Refreshed{}, nil
	}
	// Any other error leaves data nil: a copy that can't be read carries no
	// version that can be, so it's rewritten.
	found := Refreshed{Installed: true, Was: versionOf(data)}
	if found.Was >= Version() {
		return found, nil
	}
	if err := Install(path); err != nil {
		return Refreshed{}, err
	}
	found.Rewritten = true
	return found, nil
}
