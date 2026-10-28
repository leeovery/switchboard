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

// LinkFound is what the claude link step found, with no question asked.
type LinkFound struct {
	// Said is what the step told before it asked the login shell, in turn:
	// the claude link, then, the line walk's alone, switchboard's files for
	// the shells.
	Said []Said
	// Shell is the situation the login shell's answer leaves.
	Shell ShellFound
	// claudeCode is Claude Code's own claude, as claude.Find found it.
	claudeCode string
}

// Link makes switchboard's claude link, or repairs it, writes switchboard's
// files for the shells, and asks the user's login shell which claude it
// runs, as the line walk's claude link step does, and returns what it found,
// asking the user nothing. It fails as the step does, and with
// accounts.ErrInterrupted where the user interrupts the asking, returning
// what it said, and the zero ShellFound.
func (s *Setup) Link(ctx context.Context) (LinkFound, error) {
	switchboard, err := s.LaunchAgent.Binary(s.Install.Executable)
	if err != nil {
		return LinkFound{}, err
	}
	var found LinkFound
	r := &run{Setup: s, switchboard: switchboard, tell: func(said Said) { found.Said = append(found.Said, said) }}
	found.Shell, err = r.linkClaude(ctx)
	found.claudeCode = r.claudeCode
	return found, err
}

// link has every claude a new terminal starts go through switchboard. It
// makes switchboard's claude link, in switchboard's own directory, leading
// to switchboard by the path it was run by, so an upgrade moves it on, or
// repairs one there leading anywhere else: the directory is switchboard's.
// Anything in the link's place that isn't a link is left alone, and said to
// be. Then it writes switchboard's files for the shells, and has the user's
// login shell run the link (see settle). Then, whatever it found, unless it
// failed or was interrupted, it installs the session hook, which tells a
// session that didn't start through switchboard so.
func (r *run) link(ctx context.Context) error {
	found, err := r.linkClaude(ctx)
	if err != nil {
		return err
	}
	if err := r.settle(ctx, LinkFound{Shell: found, claudeCode: r.claudeCode}); err != nil {
		return err
	}
	r.sessionHook(ctx)
	return nil
}

// linkClaude makes switchboard's claude link, writes switchboard's files for
// the shells, telling each, and asks the user's login shell which claude it
// runs, as Link says, returning the situation its answer leaves: Unreached,
// the shell's row saying why it wasn't asked, where the link is blocked, or
// there's no Claude Code for it to start.
func (r *run) linkClaude(ctx context.Context) (ShellFound, error) {
	linked, err := r.makeLink()
	switch {
	case err != nil:
		return ShellFound{}, err
	case !linked:
		return ShellFound{Situation: Unreached, Said: yourShell(Warning, "not asked, as the claude link isn't made", "")}, nil
	}
	if r.claudeCode, err = claude.Find(r.Path, r.InstallPaths, func() (string, error) { return r.switchboard, nil }); err != nil {
		return ShellFound{Situation: Unreached, Said: yourShell(Warning,
			fmt.Sprintf("not asked, as there's no Claude Code for claude to start: %v: install it, then run setup again", err),
			fmt.Sprintf("There's no Claude Code for claude to start: %v. Install it, then run setup again.", err))}, nil
	}
	if err := r.shellFiles(); err != nil {
		return ShellFound{}, err
	}
	answer, err := r.ask(ctx)
	if err != nil {
		return ShellFound{}, err
	}
	return r.onPath(answer), nil
}

// makeLink makes switchboard's claude link, or repairs one leading anywhere
// else, reporting whether it's there, as it should be. Anything else in its
// place is left alone, and said to be.
func (r *run) makeLink() (bool, error) {
	link := r.claudeLink()
	leads := r.fromHome(link) + " → switchboard"
	info, err := os.Lstat(link)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return true, r.putLink(link, Said{Mark: Done, Label: "Link", Detail: leads, Sentence: fmt.Sprintf("Linked %s to %s.", link, r.switchboard)})
	case err != nil:
		return false, fmt.Errorf("look at switchboard's claude link: %w", err)
	case info.Mode()&fs.ModeSymlink == 0:
		r.tell(Said{
			Mark:     Warning,
			Label:    "Link",
			Detail:   fmt.Sprintf("not made: %s is there already, and isn't a link, so it's left alone: move it aside, then run setup again", r.fromHome(link)),
			Sentence: fmt.Sprintf("%s is there already, and isn't a link, so it's left alone, and claude isn't linked: move it aside, then run setup again.", link),
		})
		return false, nil
	}
	target, err := os.Readlink(link)
	switch {
	case err != nil:
		return false, fmt.Errorf("look at switchboard's claude link: %w", err)
	case target == r.switchboard:
		r.tell(Said{Mark: Done, Label: "Link", Detail: leads, Sentence: fmt.Sprintf("%s leads to switchboard, at %s.", link, r.switchboard)})
		return true, nil
	}
	return true, r.putLink(link, Said{
		Mark:     Done,
		Label:    "Link",
		Detail:   leads + " · repaired: it led to " + r.fromHome(target),
		Sentence: fmt.Sprintf("%s led to %s: it leads to switchboard, at %s, now.", link, target, r.switchboard),
	})
}

// putLink puts the link at link to switchboard, making its directory, and
// tells done. It's made beside where it goes, then renamed there, so a
// claude run meanwhile finds the one there before, or this one.
func (r *run) putLink(link string, done Said) error {
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
	r.tell(done)
	return nil
}
