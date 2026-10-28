package setup

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/leeovery/switchboard/internal/accounts"
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
// once the user's login shell, asked, finds switchboard's claude link on
// PATH. Until then, for a shell switchboard knows, it offers to add
// switchboard's line to the end of the shell's startup file, or says what
// keeps the line there from taking effect; then, or for any other shell, it
// shows how to have it do so by hand, and asks the shell again until it
// runs the link, or the user leaves it. Leaving it isn't a failure: setup
// goes on.
func (r *run) onPath(ctx context.Context) error {
	answer, err := r.ask(ctx)
	if err != nil || answer.Kind == shell.Link {
		return err
	}
	sh, known := shell.Of(r.Getenv("SHELL"))
	if !known {
		r.Terminal.sayf("%s", r.notYet(answer))
		return r.byHand(ctx, "To have claude go through switchboard, put this directory first on PATH, wherever your shell sets it:")
	}
	initFile := shell.InitFile(r.Shells.Dir, sh)
	line := shell.Line(sh, initFile, r.Home)
	file := shell.StartupFile(sh, r.Getenv, r.Home, answer.ConfigDir)
	reads, err := shell.FindLine(sh, file, r.Home)
	switch {
	case err != nil:
		r.Terminal.sayf("%s Setup can't look for its line in %s: %v.", r.notYet(answer), r.fromHome(file), err)
		return r.byHand(ctx, addByHand, line, orElsewhere)
	case reads == "":
		return r.offerLine(ctx, sh, file, line, answer)
	case filepath.Clean(reads) != initFile:
		r.Terminal.sayf("%s switchboard's line in %s reads %s, not this switchboard's file for %s, %s.", r.notYet(answer), r.fromHome(file), r.fromHome(reads), sh, r.fromHome(initFile))
		return r.byHand(ctx, "Put this line in its place:", line, orElsewhere)
	}
	return r.notTakingEffect(ctx, file, answer)
}

// addByHand and orElsewhere say how to have claude go through switchboard by
// hand: by switchboard's line, shown after addByHand, or by the bin
// directory, shown after orElsewhere.
const (
	addByHand   = "To have claude go through switchboard by hand, add this line to the end of your shell's startup file, after anything there that changes PATH:"
	orElsewhere = "Or, where you keep your PATH another way, put this directory first on it:"
)

// offerLine offers to add line, switchboard's line for sh, to the end of
// file, sh's startup file, the login shell having answered answer, and asks
// the shell again once it's added; then, until the shell runs the link,
// shows how to have it do so by hand.
func (r *run) offerLine(ctx context.Context, sh shell.Shell, file, line string, answer shell.Answer) error {
	r.Terminal.sayf("%s This line, at the end of %s, has claude go through switchboard: it reads switchboard's file for %s, which puts %s first on PATH, and loads switchboard's completions.", r.notYet(answer), r.fromHome(file), sh, r.fromHome(r.Bin))
	r.Terminal.sayf("%s", line)
	add, err := r.Terminal.confirm("Add it?", true)
	switch {
	case err != nil:
		return err
	case !add:
		return r.byHand(ctx, addByHand, line, orElsewhere)
	}
	if err := shell.AddLine(file, line); err != nil {
		r.Terminal.sayf("Setup can't add it: %v.", err)
		return r.byHand(ctx, addByHand, line, orElsewhere)
	}
	logger.Info("added switchboard's line to the shell's startup file", "path", file)
	r.Terminal.sayf("Added it to %s.", r.fromHome(file))
	if answer, err = r.ask(ctx); err != nil || answer.Kind == shell.Link {
		return err
	}
	return r.notTakingEffect(ctx, file, answer)
}

// notTakingEffect says switchboard's line is in file, the shell's startup
// file, but the login shell's PATH doesn't find the link, as answer, the
// shell's, says, or that setup can't tell, as the shell couldn't be asked;
// then shows how to have it take effect by hand.
func (r *run) notTakingEffect(ctx context.Context, file string, answer shell.Answer) error {
	move := fmt.Sprintf("move switchboard's line to the end of %s, or what changes PATH after it to before it. %s", r.fromHome(file), orElsewhere)
	if answer.Kind == shell.Unasked {
		r.Terminal.sayf("switchboard's line is in %s, but setup can't ask %s whether it takes effect: %s.", r.fromHome(file), r.shellName(), answer.Why)
		return r.byHand(ctx, "Should it not, "+move)
	}
	r.Terminal.sayf("switchboard's line is in %s, but %s, so something after the line changes PATH.", r.fromHome(file), r.runs(answer))
	return r.byHand(ctx, "To have claude go through switchboard, "+move)
}

// byHand shows how to have claude go through switchboard by hand, the lines
// how says it, then the bin directory, to put first on PATH; then asks the
// login shell again, as the user says to, until it runs the link, or the
// user leaves it.
func (r *run) byHand(ctx context.Context, how ...string) error {
	for _, line := range how {
		r.Terminal.sayf("%s", line)
	}
	r.Terminal.sayf("%s", r.fromHomeVariable(r.Bin))
	for {
		again, err := r.Terminal.confirm("Check again?", true)
		if err != nil {
			return err
		}
		if !again {
			r.Terminal.sayf("Left as it is: until it's done, claude in a new terminal doesn't go through switchboard. Running setup again checks it.")
			return nil
		}
		answer, err := r.ask(ctx)
		if err != nil || answer.Kind == shell.Link {
			return err
		}
		r.Terminal.sayf("%s", r.notYet(answer))
	}
}

// ask asks the user's login shell which claude it runs, saying so first, as
// its startup files can take a while, and says so where its PATH finds the
// link. An interrupt meanwhile, which the shell, in a session of its own,
// never sees, ends the asking, and setup with it.
func (r *run) ask(ctx context.Context) (shell.Answer, error) {
	r.Terminal.sayf("Asking %s, as a new terminal starts it, which claude it runs.", r.shellName())
	interrupt := r.Interrupt
	if interrupt == nil {
		interrupt = interruptOnSIGINT
	}
	asking, stop := interrupt(ctx)
	defer stop()
	answer := r.Login.Which(asking, r.claudeLink())
	if asking.Err() != nil && ctx.Err() == nil {
		return answer, accounts.ErrInterrupted
	}
	if answer.Kind == shell.Link {
		r.sayLinked(answer)
	}
	return answer, nil
}

// interruptOnSIGINT ends the context it returns at an interrupt.
func interruptOnSIGINT(ctx context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(ctx, os.Interrupt)
}

// sayLinked says claude goes through switchboard in a new terminal, as
// answer, the login shell's, says: and where an alias or function stands in
// front of PATH's claude, that it does so only where it runs claude by name.
func (r *run) sayLinked(answer shell.Answer) {
	r.Terminal.sayf("claude goes through switchboard in a new terminal: %s runs %s.", r.shellName(), r.fromHome(r.claudeLink()))
	if answer.Front != "" {
		r.Terminal.sayf("%s's claude is %s, which goes through switchboard where it runs claude by name.", r.shellName(), answer.Front)
	}
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

// runs says what the login shell's PATH finds for claude, as it answered,
// other than the link.
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
	return r.relative(path, "~/")
}

// fromHomeVariable writes path from $HOME where it's in the home, as a shell
// reads it between double quotes, where it reads no ~.
func (r *run) fromHomeVariable(path string) string {
	return r.relative(path, "$HOME/")
}

// relative writes path from home, written as home says, where it's in the
// home.
func (r *run) relative(path, home string) string {
	rel, err := filepath.Rel(r.Home, path)
	if r.Home == "" || err != nil || !filepath.IsLocal(rel) {
		return path
	}
	return home + filepath.ToSlash(rel)
}
