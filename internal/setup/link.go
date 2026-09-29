package setup

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/leeovery/switchboard/internal/claude"
)

// link has every claude started from PATH go through switchboard. It makes
// switchboard's claude link, in switchboard's own directory, leading to
// switchboard by the path it was run by, so an upgrade moves it on, or
// repairs one there leading anywhere else: the directory is switchboard's.
// Then it checks the directory is on PATH ahead of the real claude, and says
// the line to add to the shell's startup file when it isn't: it never edits
// one. Anything in the link's place that isn't a link is left alone, and
// said to be.
func (r *run) link(context.Context) error {
	linked, err := r.makeLink()
	if err != nil || !linked {
		return err
	}
	realClaude, err := claude.Find(r.Path, r.InstallPaths, func() (string, error) { return r.switchboard, nil })
	if err != nil {
		r.Terminal.sayf("There's no Claude Code for claude to start: %v. Install it, then run setup again.", err)
		return nil
	}
	ahead, onPath := r.placed(realClaude)
	if ahead {
		r.Terminal.sayf("claude on PATH goes through switchboard, from %s.", r.Bin)
		return nil
	}
	r.sayPathLine(onPath, filepath.Dir(realClaude))
	return nil
}

// makeLink makes switchboard's claude link, or repairs one leading anywhere
// else, reporting whether it's there, as it should be. Anything else in its
// place is left alone, and said to be.
func (r *run) makeLink() (bool, error) {
	link := filepath.Join(r.Bin, claude.Command)
	info, err := os.Lstat(link)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return true, r.putLink(link, fmt.Sprintf("Linked %s to %s.", link, r.switchboard))
	case err != nil:
		return false, fmt.Errorf("look at switchboard's claude link: %w", err)
	case info.Mode()&fs.ModeSymlink == 0:
		r.Terminal.sayf("%s is there already, and isn't a link, so it's left alone, and claude isn't linked: move it aside, then run setup again.", link)
		return false, nil
	}
	target, err := os.Readlink(link)
	switch {
	case err != nil:
		return false, fmt.Errorf("look at switchboard's claude link: %w", err)
	case target == r.switchboard:
		r.Terminal.sayf("%s leads to switchboard, at %s.", link, r.switchboard)
		return true, nil
	}
	return true, r.putLink(link, fmt.Sprintf("%s led to %s: it leads to switchboard, at %s, now.", link, target, r.switchboard))
}

// putLink puts the link at link to switchboard, making its directory, and
// says so as done says. It's made beside where it goes, then renamed there,
// so a claude run meanwhile finds the one there before, or this one.
func (r *run) putLink(link, done string) error {
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		return fmt.Errorf("make switchboard's bin directory: %w", err)
	}
	made := fmt.Sprintf("%s.%d", link, os.Getpid())
	if err := os.Symlink(r.switchboard, made); err != nil {
		return fmt.Errorf("link claude to switchboard: %w", err)
	}
	if err := os.Rename(made, link); err != nil {
		_ = os.Remove(made)
		return fmt.Errorf("link claude to switchboard: %w", err)
	}
	logger.Info("linked claude to switchboard", "link", link, "switchboard", r.switchboard)
	r.Terminal.sayf("%s", done)
	return nil
}

// placed reports whether switchboard's directory is on PATH ahead of the
// real claude, at realClaude, and whether it's on PATH at all. A directory on
// PATH that's the same directory, links followed, counts; a relative one
// doesn't, as claude.Find passes it over.
func (r *run) placed(realClaude string) (ahead, onPath bool) {
	mine, err := os.Stat(r.Bin)
	if err != nil {
		return false, false
	}
	behind := false
	for _, dir := range filepath.SplitList(r.Path) {
		if !filepath.IsAbs(dir) {
			continue
		}
		if info, err := os.Stat(dir); err == nil && os.SameFile(info, mine) {
			return !behind, true
		}
		behind = behind || filepath.Clean(dir) == filepath.Dir(realClaude)
	}
	return false, false
}

// sayPathLine says why claude on PATH doesn't go through switchboard yet,
// the real claude's directory being claudeDir, and the line to add to the
// shell's startup file that has it do so.
func (r *run) sayPathLine(onPath bool, claudeDir string) {
	why := r.Bin + " isn't on PATH"
	if onPath {
		why = fmt.Sprintf("%s is on PATH, but after %s, where Claude Code is", r.Bin, claudeDir)
	}
	r.Terminal.sayf("claude doesn't go through switchboard yet: %s. Add this line to your shell's startup file, such as ~/.zshrc, after anything else there that changes PATH, so it stays ahead of %s:", why, claudeDir)
	r.Terminal.sayf("%s", exportLine(r.Bin, r.Home))
	r.Terminal.sayf("Running setup again, in a new terminal, checks it.")
}

// exportLine is the line of a shell's startup file that puts dir first on
// PATH, dir written from $HOME when it's in home.
func exportLine(dir, home string) string {
	written := doubleQuoted(dir)
	if rel, err := filepath.Rel(home, dir); home != "" && err == nil && filepath.IsLocal(rel) {
		written = "$HOME/" + doubleQuoted(filepath.ToSlash(rel))
	}
	return `export PATH="` + written + `:$PATH"`
}

// doubleQuoted escapes what a shell reads specially between double quotes.
var doubleQuoted = strings.NewReplacer(`\`, `\\`, `"`, `\"`, "$", `\$`, "`", "\\`").Replace
