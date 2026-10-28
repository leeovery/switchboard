package setup

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/leeovery/switchboard/internal/claude"
)

// link has every claude a new terminal starts go through switchboard. It
// makes switchboard's claude link, in switchboard's own directory, leading
// to switchboard by the path it was run by, so an upgrade moves it on, or
// repairs one there leading anywhere else: the directory is switchboard's.
// Anything in the link's place that isn't a link is left alone, and said to
// be. Then it writes switchboard's files for the shells, and has the user's
// login shell run the link (see onPath).
func (r *run) link(ctx context.Context) error {
	linked, err := r.makeLink()
	if err != nil || !linked {
		return err
	}
	if _, err := claude.Find(r.Path, r.InstallPaths, func() (string, error) { return r.switchboard, nil }); err != nil {
		r.Terminal.sayf("There's no Claude Code for claude to start: %v. Install it, then run setup again.", err)
		return nil
	}
	if err := r.shellFiles(); err != nil {
		return err
	}
	return r.onPath(ctx)
}

// makeLink makes switchboard's claude link, or repairs one leading anywhere
// else, reporting whether it's there, as it should be. Anything else in its
// place is left alone, and said to be.
func (r *run) makeLink() (bool, error) {
	link := r.claudeLink()
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
