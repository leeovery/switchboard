package setup

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/leeovery/switchboard/internal/claude"
	"github.com/leeovery/switchboard/internal/prose"
	"github.com/leeovery/switchboard/internal/shell"
)

// LoginShell is the user's login shell, asked which claude it runs, link
// being switchboard's claude link, as shell.Login asks it.
type LoginShell interface {
	Which(ctx context.Context, link string) shell.Answer
}

// shellFiles writes switchboard's files for the shells, saying which it
// wrote or brought up to date, or that they were up to date already.
func (r *run) shellFiles() error {
	files, err := r.Shells.Write()
	if err != nil {
		return err
	}
	names := make(map[shell.State][]string)
	for _, f := range files {
		names[f.State] = append(names[f.State], filepath.Base(f.Path))
		switch f.State {
		case shell.Created:
			logger.Info("wrote switchboard's file for a shell", "path", f.Path)
		case shell.Updated:
			logger.Info("brought switchboard's file for a shell up to date", "path", f.Path)
		}
	}
	dir := r.fromHome(r.Shells.Dir)
	if wrote := names[shell.Created]; len(wrote) > 0 {
		r.Terminal.sayf("Wrote switchboard's files for the shells, in %s: %s.", dir, prose.List(wrote))
	}
	if updated := names[shell.Updated]; len(updated) > 0 {
		r.Terminal.sayf("Brought switchboard's files for the shells up to date, in %s: %s.", dir, prose.List(updated))
	}
	if len(names[shell.Current]) == len(files) {
		r.Terminal.sayf("switchboard's files for the shells are up to date, in %s.", dir)
	}
	return nil
}

// onPath has claude go through switchboard in a new terminal, which it does
// once the user's login shell, asked, runs switchboard's claude link. Until
// then, for a shell switchboard knows, it offers to add switchboard's line
// to the end of the shell's startup file, or says what keeps the line there
// from taking effect; then, or for any other shell, it shows how to have it
// do so by hand, and asks the shell again until it runs the link, or the
// user leaves it. Leaving it isn't a failure: setup goes on.
func (r *run) onPath(ctx context.Context) error {
	answer := r.Login.Which(ctx, r.claudeLink())
	if answer.Kind == shell.Link {
		r.sayLinked()
		return nil
	}
	sh, known := shell.Of(r.Getenv("SHELL"))
	if !known {
		r.Terminal.sayf("%s", r.notYet(answer))
		return r.byHand(ctx, "")
	}
	line := shell.Line(sh, shell.InitFile(r.Shells.Dir, sh), r.Home)
	done, err := r.offerLine(ctx, sh, line, answer)
	if err != nil || done {
		return err
	}
	return r.byHand(ctx, line)
}

// offerLine offers to add line, switchboard's line for sh, to the end of
// sh's startup file, the login shell having answered answer, and asks the
// shell again once it's added, reporting whether the shell runs the link.
// Where the line's there already, it says something after it changes PATH.
func (r *run) offerLine(ctx context.Context, sh shell.Shell, line string, answer shell.Answer) (bool, error) {
	file := shell.StartupFile(sh, r.Getenv, r.Home)
	has, err := shell.HasLine(sh, file)
	switch {
	case err != nil:
		r.Terminal.sayf("%s Setup can't look for its line in %s: %v.", r.notYet(answer), r.fromHome(file), err)
		return false, nil
	case has:
		r.sayNotTakingEffect(file, answer)
		return false, nil
	}
	r.Terminal.sayf("%s This line, at the end of %s, has claude go through switchboard: it reads switchboard's file for %s, which puts %s first on PATH, and loads switchboard's completions.", r.notYet(answer), r.fromHome(file), sh, r.fromHome(r.Bin))
	r.Terminal.sayf("%s", line)
	add, err := r.Terminal.confirm("Add it?", true)
	if err != nil || !add {
		return false, err
	}
	if err := shell.AddLine(file, line); err != nil {
		r.Terminal.sayf("Setup can't add it: %v.", err)
		return false, nil
	}
	logger.Info("added switchboard's line to the shell's startup file", "path", file)
	r.Terminal.sayf("Added it to %s.", r.fromHome(file))
	if answer = r.Login.Which(ctx, r.claudeLink()); answer.Kind == shell.Link {
		r.sayLinked()
		return true, nil
	}
	r.sayNotTakingEffect(file, answer)
	return false, nil
}

// byHand shows how to have claude go through switchboard by hand, by line,
// switchboard's line for the user's shell, where there is one, or by its
// bin directory, first on PATH; then asks the login shell again, as the user
// says to, until it runs the link, or the user leaves it.
func (r *run) byHand(ctx context.Context, line string) error {
	if line != "" {
		r.Terminal.sayf("To have claude go through switchboard by hand, add this line to the end of your shell's startup file, after anything there that changes PATH:")
		r.Terminal.sayf("%s", line)
		r.Terminal.sayf("Or, where you keep your PATH another way, put this directory first on it:")
	} else {
		r.Terminal.sayf("To have claude go through switchboard, put this directory first on PATH, wherever your shell sets it:")
	}
	r.Terminal.sayf("%s", r.fromHome(r.Bin))
	for {
		again, err := r.Terminal.confirm("Check again?", true)
		if err != nil {
			return err
		}
		if !again {
			r.Terminal.sayf("Left as it is: until it's done, claude in a new terminal doesn't go through switchboard. Running setup again checks it.")
			return nil
		}
		answer := r.Login.Which(ctx, r.claudeLink())
		if answer.Kind == shell.Link {
			r.sayLinked()
			return nil
		}
		r.Terminal.sayf("%s", r.notYet(answer))
	}
}

// sayLinked says claude goes through switchboard in a new terminal.
func (r *run) sayLinked() {
	r.Terminal.sayf("claude goes through switchboard in a new terminal: %s runs %s.", r.shellName(), r.fromHome(r.claudeLink()))
}

// notYet says claude doesn't go through switchboard in a new terminal yet,
// as answer, the login shell's, says: or that setup can't tell, as the shell
// couldn't be asked.
func (r *run) notYet(answer shell.Answer) string {
	if answer.Kind == shell.Unasked {
		return fmt.Sprintf("Setup can't ask %s which claude it runs: %s.", r.shellName(), answer.Why)
	}
	return fmt.Sprintf("claude doesn't go through switchboard in a new terminal yet: %s.", r.runs(answer))
}

// sayNotTakingEffect says switchboard's line is in the startup file at
// file, but doesn't take effect, as answer, the login shell's, says: or that
// setup can't tell, as the shell couldn't be asked.
func (r *run) sayNotTakingEffect(file string, answer shell.Answer) {
	if answer.Kind == shell.Unasked {
		r.Terminal.sayf("switchboard's line is in %s, but setup can't ask %s whether it takes effect: %s.", r.fromHome(file), r.shellName(), answer.Why)
		return
	}
	r.Terminal.sayf("switchboard's line is in %s, but %s, so something after the line changes PATH.", r.fromHome(file), r.runs(answer))
}

// runs says what the login shell runs for claude, as it answered, other than
// the link.
func (r *run) runs(answer shell.Answer) string {
	if answer.Kind == shell.None {
		return r.shellName() + " finds no claude"
	}
	return fmt.Sprintf("%s runs another claude, %s", r.shellName(), answer.Claude)
}

// shellName names the user's login shell, by its program's name.
func (r *run) shellName() string {
	if path := r.Getenv("SHELL"); path != "" {
		return filepath.Base(path)
	}
	return "your login shell"
}

// claudeLink is switchboard's claude link.
func (r *run) claudeLink() string {
	return filepath.Join(r.Bin, claude.Command)
}

// fromHome writes path from ~ where it's in the home, as the user reads it.
func (r *run) fromHome(path string) string {
	rel, err := filepath.Rel(r.Home, path)
	if r.Home == "" || err != nil || !filepath.IsLocal(rel) {
		return path
	}
	return filepath.Join("~", rel)
}
