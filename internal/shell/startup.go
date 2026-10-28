package shell

import (
	"cmp"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/leeovery/switchboard/internal/atomicfile"
)

// mark ends switchboard's line in a startup file, so it's found again.
const mark = "# switchboard"

// Line returns switchboard's line for sh's startup file, which sources
// initFile, sh's file of switchboard's, when it's there: initFile is written
// from $HOME when it's in home. The line leaves the status as it was when the
// file isn't there, as the last line of .zshrc sets the first prompt's.
func Line(sh Shell, initFile, home string) string {
	quote := doubleQuoted(sh)
	written := quote(initFile)
	if rel, err := filepath.Rel(home, initFile); home != "" && err == nil && filepath.IsLocal(rel) {
		written = "$HOME/" + quote(filepath.ToSlash(rel))
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

// ours reports whether line is switchboard's line for sh: of its shape,
// reading and sourcing the one file, sh's of switchboard's, from any path,
// and ending with its mark. A comment of the user's that's the mark alone,
// or a line of their own that puts switchboard's bin directory on PATH,
// isn't.
func ours(sh Shell, line string) bool {
	shape := posixLine
	if sh == Fish {
		shape = fishLine
	}
	m := shape.FindStringSubmatch(line)
	return m != nil && m[1] == m[2] && strings.HasSuffix(m[1], "switchboard/shell/init."+string(sh))
}

// StartupFile returns the startup file of sh's that switchboard's line goes
// at the end of, the user's home being home: for zsh, .zshrc in $ZDOTDIR,
// else the home; for bash, the first there of ~/.bash_profile,
// ~/.bash_login and ~/.profile, as a login bash reads only the first it
// finds, else ~/.bash_profile; and for fish, fish/config.fish in
// $XDG_CONFIG_HOME, else ~/.config.
func StartupFile(sh Shell, getenv func(string) string, home string) string {
	switch sh {
	case Zsh:
		return filepath.Join(cmp.Or(getenv("ZDOTDIR"), home), ".zshrc")
	case Fish:
		return filepath.Join(cmp.Or(getenv("XDG_CONFIG_HOME"), filepath.Join(home, ".config")), "fish", "config.fish")
	}
	for _, name := range []string{".bash_profile", ".bash_login", ".profile"} {
		if _, err := os.Stat(filepath.Join(home, name)); err == nil {
			return filepath.Join(home, name)
		}
	}
	return filepath.Join(home, ".bash_profile")
}

// HasLine reports whether the startup file at file holds switchboard's line
// for sh: one that isn't there holds none.
func HasLine(sh Shell, file string) (bool, error) {
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", file, err)
	}
	for line := range strings.Lines(string(data)) {
		if ours(sh, line) {
			return true, nil
		}
	}
	return false, nil
}

// AddLine adds line to the end of the startup file at file, after a newline
// when the file doesn't end in one. It appends to the file at the end of
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
	text := line + "\n"
	if ends, err := endsInNewline(f); err != nil {
		_ = f.Close()
		return fmt.Errorf("read %s: %w", file, err)
	} else if !ends {
		text = "\n" + text
	}
	if _, err := f.WriteString(text); err != nil {
		_ = f.Close()
		return fmt.Errorf("add to %s: %w", file, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("add to %s: %w", file, err)
	}
	return nil
}

// endsInNewline reports whether f is empty or ends in a newline.
func endsInNewline(f *os.File) (bool, error) {
	info, err := f.Stat()
	if err != nil {
		return false, err
	}
	if info.Size() == 0 {
		return true, nil
	}
	last := make([]byte, 1)
	if _, err := f.ReadAt(last, info.Size()-1); err != nil {
		return false, err
	}
	return last[0] == '\n', nil
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
