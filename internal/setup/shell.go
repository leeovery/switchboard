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
// once the user's login shell, asked, says a claude typed there goes through
// switchboard's claude link. Until then, for a shell switchboard knows, it
// sees to switchboard's line, as startupLine does, or says what's in front
// of PATH's claude that keeps it from the link; then shows how to put it
// right by hand, and asks the shell again until it runs the link, or the
// user leaves it. Another shell it can't ask, so it shows the bin directory,
// to put on PATH by hand. Leaving it isn't a failure: setup goes on.
func (r *run) onPath(ctx context.Context) error {
	answer, err := r.ask(ctx)
	if err != nil || answer.Through() {
		return err
	}
	sh, known := shell.Of(r.Getenv("SHELL"))
	switch {
	case !known:
		r.Terminal.sayf("%s", r.notYet(answer))
		r.show("To have claude go through switchboard, put this directory first on PATH, wherever your shell sets it:")
		r.Terminal.sayf("Setup can't check it for this shell: once it's done, claude in a new terminal goes through switchboard.")
		return nil
	case answer.Kind == shell.Link:
		r.Terminal.sayf("%s", r.notYet(answer))
		return r.checkAgain(ctx)
	}
	return r.startupLine(ctx, sh, answer)
}

// startupLine offers to add switchboard's line for sh to the end of the
// startup file the login shell, having answered answer, says it reads,
// unless the shell couldn't be asked or a new terminal may read another, or
// says what keeps the line there from taking effect; then shows how to put
// it right by hand, as onPath says.
func (r *run) startupLine(ctx context.Context, sh shell.Shell, answer shell.Answer) error {
	initFile := shell.InitFile(r.Shells.Dir, sh)
	line := shell.Line(sh, initFile, r.Home)
	file := shell.StartupFile(sh, r.Home, answer.ConfigDir)
	found, err := shell.FindLine(sh, file, r.Home)
	rival := shell.RivalStartupFile(sh, r.Home, file)
	switch {
	case err != nil:
		r.Terminal.sayf("%s Setup can't look for its line in %s: %v.", r.notYet(answer), r.fromHome(file), err)
		return r.byHand(ctx, addByHand, line, orElsewhere)
	case found.Sources == "" && answer.Kind == shell.Unasked:
		r.Terminal.sayf("%s Setup can't tell which startup file %s reads, so it adds no line.", r.notYet(answer), sh)
		return r.byHand(ctx, fmt.Sprintf("To have claude go through switchboard by hand, add this line to the end of %s's startup file, usually %s, after anything there that changes PATH:", sh, r.fromHome(file)), line, orElsewhere)
	case found.Sources == "" && rival != "":
		r.Terminal.sayf("%s %s says it reads %s, which isn't there, but %s is: setup can't tell which a new terminal reads, so it adds no line.", r.notYet(answer), sh, r.fromHome(file), r.fromHome(rival))
		return r.byHand(ctx, fmt.Sprintf("To have claude go through switchboard by hand, add this line to the end of whichever of %s and %s a new terminal reads, after anything there that changes PATH:", r.fromHome(file), r.fromHome(rival)), line, orElsewhere)
	case found.Sources == "":
		return r.offerLine(ctx, sh, file, line, answer)
	case filepath.Clean(found.Sources) != initFile:
		r.Terminal.sayf("%s switchboard's line in %s reads %s, not this switchboard's file for %s, %s.", r.notYet(answer), r.fromHome(file), r.fromHome(found.Sources), sh, r.fromHome(initFile))
		return r.byHand(ctx, "Put this line in its place:", line, orElsewhere)
	}
	return r.notTakingEffect(ctx, file, found.Last, answer)
}

// addByHand and orElsewhere say how to have claude go through switchboard by
// hand: by switchboard's line, shown after addByHand, or by the bin
// directory, shown after orElsewhere.
const (
	addByHand   = "To have claude go through switchboard by hand, add this line to the end of your shell's startup file, after anything there that changes PATH:"
	orElsewhere = "Or, where you keep your PATH another way, put this directory first on it:"
)

// offerLine offers to add line, switchboard's line for sh, to the end of
// file, the startup file the login shell, having answered answer, says it
// reads, and asks the shell again once it's added; then, until a claude
// typed there goes through switchboard, shows how to put it right by hand.
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
	if answer, err = r.ask(ctx); err != nil || answer.Through() {
		return err
	}
	if answer.Kind == shell.Link {
		r.Terminal.sayf("%s", r.notYet(answer))
		return r.checkAgain(ctx)
	}
	return r.notTakingEffect(ctx, file, true, answer)
}

// notTakingEffect says switchboard's line is in file, the shell's startup
// file, but the login shell's PATH doesn't find the link, as answer, the
// shell's, says, or that setup can't tell, as the shell couldn't be asked:
// something after the line changes PATH, or, where it's last, something
// read after the file. Then it shows how to have it take effect by hand.
func (r *run) notTakingEffect(ctx context.Context, file string, last bool, answer shell.Answer) error {
	where := r.fromHome(file)
	cause := "something after the line changes PATH"
	fix := fmt.Sprintf("move switchboard's line to the end of %s, or what changes PATH after it to before it.", where)
	if last {
		cause = fmt.Sprintf("something read after %s changes PATH", where)
		fix = "put switchboard's line after what changes PATH, wherever that's read."
	}
	if answer.Kind == shell.Unasked {
		r.Terminal.sayf("switchboard's line is in %s, but setup can't ask %s whether it takes effect: %s.", where, r.shellName(), answer.Why)
		return r.byHand(ctx, fmt.Sprintf("Should it not, %s: %s %s", cause, fix, orElsewhere))
	}
	r.Terminal.sayf("switchboard's line is in %s, but %s, so %s.", where, r.runs(answer), cause)
	return r.byHand(ctx, fmt.Sprintf("To have claude go through switchboard, %s %s", fix, orElsewhere))
}

// byHand shows how to have claude go through switchboard by hand, as show
// does; then asks the login shell again, as checkAgain does.
func (r *run) byHand(ctx context.Context, how ...string) error {
	r.show(how...)
	return r.checkAgain(ctx)
}

// show shows how to have claude go through switchboard by hand, the lines
// how says it, then the bin directory, to put first on PATH.
func (r *run) show(how ...string) {
	for _, line := range how {
		r.Terminal.sayf("%s", line)
	}
	r.Terminal.sayf("%s", r.fromHomeVariable(r.Bin))
}

// checkAgain asks the login shell again, as the user says to, until a claude
// typed in a new terminal goes through switchboard, or the user leaves it.
func (r *run) checkAgain(ctx context.Context) error {
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
		if err != nil || answer.Through() {
			return err
		}
		r.Terminal.sayf("%s", r.notYet(answer))
	}
}

// ask asks the user's login shell which claude it runs, saying so first, as
// its startup files can take a while, where it's a shell setup asks; and
// says so where a claude typed there goes through switchboard. An interrupt
// meanwhile, which the shell, in a session of its own, never sees, ends the
// asking, and setup with it.
func (r *run) ask(ctx context.Context) (shell.Answer, error) {
	if _, known := shell.Of(r.Getenv("SHELL")); known {
		r.Terminal.sayf("Asking %s, as a new terminal starts it, which claude it runs.", r.shellName())
	}
	asking, stop := r.interruptible(ctx)
	defer stop()
	answer := r.Login.Which(asking, r.claudeLink())
	if asking.Err() != nil && ctx.Err() == nil {
		return answer, accounts.ErrInterrupted
	}
	if answer.Through() {
		r.sayLinked(answer)
	}
	return answer, nil
}

// interruptible returns a context that ends with ctx, or at an interrupt,
// as setup's Interrupt says.
func (s *Setup) interruptible(ctx context.Context) (context.Context, context.CancelFunc) {
	if s.Interrupt != nil {
		return s.Interrupt(ctx)
	}
	return signal.NotifyContext(ctx, os.Interrupt)
}

// sayLinked says claude goes through switchboard in a new terminal, as
// answer, the login shell's, says, and what's in front of PATH's claude
// there: an alias that runs claude by name, or the link by its path, goes
// through switchboard; a function, or an alias setup doesn't read, does
// where it runs claude by name.
func (r *run) sayLinked(answer shell.Answer) {
	r.Terminal.sayf("claude goes through switchboard in a new terminal: %s runs %s.", r.shellName(), r.fromHome(r.claudeLink()))
	front := answer.Front
	switch {
	case front.Text == "":
	case front.Runs == shell.ByName:
		r.Terminal.sayf("%s's claude is %s, which runs claude by name, so goes through switchboard too.", r.shellName(), front.Text)
	case front.Runs == shell.TheLink:
		r.Terminal.sayf("%s's claude is %s, which runs switchboard's claude link by its path.", r.shellName(), front.Text)
	default:
		r.Terminal.sayf("%s's claude is %s, which setup doesn't read: it goes through switchboard where it runs claude by name.", r.shellName(), front.Text)
	}
}

// notYet says claude doesn't go through switchboard in a new terminal yet,
// as answer, the login shell's, says: what its PATH finds, and an alias in
// front of it that runs something else; or that setup can't tell, as the
// shell couldn't be asked.
func (r *run) notYet(answer shell.Answer) string {
	if answer.Kind == shell.Unasked {
		return fmt.Sprintf("Setup can't ask %s which claude it runs: %s.", r.shellName(), answer.Why)
	}
	const not = "claude doesn't go through switchboard in a new terminal yet: "
	front := r.inFront(answer.Front)
	switch {
	case answer.Kind == shell.Link:
		return not + front + "."
	case front != "":
		return not + r.runs(answer) + ". " + front + "."
	}
	return not + r.runs(answer) + "."
}

// inFront says what an alias in front of PATH's claude runs, where that's
// not switchboard's claude, and how to put it right: "" where it's nothing
// else.
func (r *run) inFront(front shell.Front) string {
	const fix = "removing the alias, or pointing it at claude, puts it right"
	switch front.Runs {
	case shell.APath:
		return fmt.Sprintf("%s's claude is %s, which runs %s, not switchboard's claude: %s", r.shellName(), front.Text, front.Path, fix)
	case shell.Else:
		return fmt.Sprintf("%s's claude is %s, which runs something other than switchboard's claude: %s", r.shellName(), front.Text, fix)
	case shell.SetsPath:
		return fmt.Sprintf("%s's claude is %s, which sets PATH for claude, so setup can't tell which claude it runs: taking that PATH setting out of the alias puts it right", r.shellName(), front.Text)
	}
	return ""
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
