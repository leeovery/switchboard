package shell

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/leeovery/switchboard/internal/atomicfile"
)

// mark ends switchboard's line in a startup file, so it's found again.
const mark = "# switchboard"

// homeVariable begins a path written from the home.
const homeVariable = "$HOME/"

// Line returns switchboard's line for sh's startup file, which sources
// initFile, sh's file of switchboard's, when it's there: initFile is written
// from $HOME when it's in home. Where the file isn't there, the line ends
// with a status of 0, not the failed test's, as the last line of .zshrc sets
// the first prompt's.
func Line(sh Shell, initFile, home string) string {
	quote := doubleQuoted(sh)
	written := quote(initFile)
	if rel, err := filepath.Rel(home, initFile); home != "" && err == nil && filepath.IsLocal(rel) {
		written = homeVariable + quote(filepath.ToSlash(rel))
	}
	if sh == Fish {
		return fmt.Sprintf(`if test -r "%[1]s"; source "%[1]s"; end  %[2]s`, written, mark)
	}
	return fmt.Sprintf(`if [ -r "%[1]s" ]; then . "%[1]s"; fi  %[2]s`, written, mark)
}

// A line of switchboard's shape, the file it reads and the one it sources
// each caught, between double quotes.
var (
	posixLine = regexp.MustCompile(`^\s*if \[ -r "((?:[^"\\]|\\.)*)" \]; then \. "((?:[^"\\]|\\.)*)"; fi\s+` + mark + `\s*$`)
	fishLine  = regexp.MustCompile(`^\s*if test -r "((?:[^"\\]|\\.)*)"; source "((?:[^"\\]|\\.)*)"; end\s+` + mark + `\s*$`)
)

// escaped is a character escaped between double quotes.
var escaped = regexp.MustCompile(`\\(.)`)

// sources returns the file line sources, $HOME in it standing for home,
// where it's switchboard's line for sh: of its shape, reading and sourcing
// the one file, sh's of switchboard's, from any path, and ending with its
// mark. A comment of the user's that's the mark alone, or a line of their
// own that puts switchboard's bin directory on PATH, isn't, and sources "".
func sources(sh Shell, line, home string) string {
	shape := posixLine
	if sh == Fish {
		shape = fishLine
	}
	m := shape.FindStringSubmatch(line)
	if m == nil || m[1] != m[2] || !strings.HasSuffix(m[1], "switchboard/shell/init."+string(sh)) {
		return ""
	}
	written, fromHome := strings.CutPrefix(m[1], homeVariable)
	path := escaped.ReplaceAllString(written, "$1")
	if fromHome {
		return filepath.Join(home, path)
	}
	return path
}

// StartupFile returns the startup file of sh's that switchboard's line goes
// at the end of, the user's home being home: for zsh, .zshrc in configDir,
// where the shell said it reads it from, else the home; for bash, the first
// there of ~/.bash_profile, ~/.bash_login and ~/.profile, as a login bash
// reads only the first it finds, else ~/.bash_profile; and for fish,
// config.fish in configDir, else ~/.config/fish. A configDir that isn't
// absolute is passed over. Setup's own environment never says where: its
// ZDOTDIR can be a terminal's own, as an editor's terminal sets it to a
// directory of its own.
func StartupFile(sh Shell, home, configDir string) string {
	switch sh {
	case Zsh:
		return filepath.Join(absolute(configDir, home), ".zshrc")
	case Fish:
		return filepath.Join(absolute(configDir, filepath.Join(home, ".config", "fish")), "config.fish")
	}
	for _, name := range []string{".bash_profile", ".bash_login", ".profile"} {
		if path := filepath.Join(home, name); there(path) {
			return path
		}
	}
	return filepath.Join(home, ".bash_profile")
}

// RivalStartupFile returns the startup file a new terminal may read in place
// of file, sh's as the shell said it reads it, the user's home being home:
// for zsh, ~/.zshrc, where file, .zshrc in the ZDOTDIR the shell reported,
// isn't there and the home's is. The shell's asked once its startup files
// have run, so the ZDOTDIR it reports may not be where a new terminal reads
// .zshrc: one exported in .zshrc or after it isn't, though one exported in
// .zshenv is, as is one exported in .zprofile for a login shell, as macOS
// terminals start. It returns "" where there's no rival.
func RivalStartupFile(sh Shell, home, file string) string {
	usual := filepath.Join(home, ".zshrc")
	if sh != Zsh || file == usual || there(file) || !there(usual) {
		return ""
	}
	return usual
}

// there reports whether there's a file at path, links followed.
func there(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// absolute returns dir where it's absolute, else otherwise.
func absolute(dir, otherwise string) string {
	if filepath.IsAbs(dir) {
		return dir
	}
	return otherwise
}

// Found is switchboard's line in a startup file, as FindLine finds it.
type Found struct {
	// Sources is the file the line sources: "" where there's no line.
	Sources string
	// Last is whether nothing but blank lines and comments follows it.
	Last bool
}

// FindLine returns switchboard's line for sh in the startup file at file,
// $HOME in it standing for home: none where there's no such line, or no
// such file.
func FindLine(sh Shell, file, home string) (Found, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return Found{}, nil
	}
	if err != nil {
		return Found{}, fmt.Errorf("read %s: %w", file, err)
	}
	var found Found
	for line := range strings.Lines(string(data)) {
		if path := sources(sh, line, home); path != "" && found.Sources == "" {
			found = Found{Sources: path, Last: true}
			continue
		}
		if text := strings.TrimSpace(line); text != "" && !strings.HasPrefix(text, "#") {
			found.Last = false
		}
	}
	return found, nil
}

// AddLine adds line to the end of the startup file at file, on a line of its
// own, as before says. It appends to the file at the end of
// file's links, as a startup file kept with the user's dotfiles is linked to,
// so the file keeps its inode, mode and links; and makes a file that isn't
// there, and its directory. It fails, saying why, at a link that leads
// nowhere, or a file it can't write.
func AddLine(file, line string) error {
	target, err := atomicfile.Target(file)
	if err != nil {
		return fmt.Errorf("follow %s: %w", file, err)
	}
	f, err := os.OpenFile(target, os.O_RDWR|os.O_APPEND, 0)
	switch {
	case errors.Is(err, fs.ErrNotExist) && target != file:
		return fmt.Errorf("%s is a link to %s, which isn't there", file, target)
	case errors.Is(err, fs.ErrNotExist):
		return create(file, line)
	case err != nil:
		return fmt.Errorf("open %s: %w", file, err)
	}
	have, err := io.ReadAll(f)
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("read %s: %w", file, err)
	}
	if _, err := f.WriteString(before(have) + line + "\n"); err != nil {
		_ = f.Close()
		return fmt.Errorf("add to %s: %w", file, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("add to %s: %w", file, err)
	}
	return nil
}

// before is what goes between have, a startup file's text, and the line
// added after it: a newline where have doesn't end in one; and an empty line
// where its last line ends in a backslash that escapes the newline after it,
// which would join the added line to it.
func before(have []byte) string {
	if len(have) == 0 {
		return ""
	}
	sep := ""
	last, ended := bytes.CutSuffix(have, []byte("\n"))
	if !ended {
		sep = "\n"
	}
	if backslashes := len(last) - len(bytes.TrimRight(last, `\`)); backslashes%2 == 1 {
		sep += "\n"
	}
	return sep
}

// create makes the startup file at file, holding line alone, and its
// directory.
func create(file, line string) error {
	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		return fmt.Errorf("make %s's directory: %w", file, err)
	}
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("make %s: %w", file, err)
	}
	if _, err := f.WriteString(line + "\n"); err != nil {
		_ = f.Close()
		return fmt.Errorf("add to %s: %w", file, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("add to %s: %w", file, err)
	}
	return nil
}
