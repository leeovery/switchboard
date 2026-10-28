package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strings"

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

// Situation is where the user's login shell, asked which claude it runs,
// leaves the claude link step.
type Situation int

const (
	// Unreached is a step that stopped before it asked the shell: the link
	// blocked, or no Claude Code for claude to start, as its Said says.
	Unreached Situation = iota
	// Through is a shell where a claude typed in a new terminal goes through
	// switchboard.
	Through
	// UnknownShell is a shell setup doesn't ask, or none: switchboard's bin
	// directory is to go first on PATH by hand, which setup can't check.
	UnknownShell
	// InFront is a shell whose PATH finds the link, but where an alias in
	// front of it runs something else.
	InFront
	// CantLook is a startup file setup can't look in for its line.
	CantLook
	// WhichFile is a shell that couldn't be asked, with no line where it
	// usually reads its startup file, so setup can't tell which file to add
	// one to.
	WhichFile
	// RivalFile is a startup file the shell says it reads that isn't there,
	// while another a new terminal may read is, so setup adds no line.
	RivalFile
	// OfferLine is a startup file without switchboard's line, which setup
	// offers to add, as AddLine does.
	OfferLine
	// OtherLine is switchboard's line in the startup file reading another
	// switchboard shell directory's file than this one's.
	OtherLine
	// NotTaking is switchboard's line in the startup file, not taking
	// effect, or in a shell that couldn't be asked whether it does.
	NotTaking
	// LineAdded is switchboard's line just added, or found there already as
	// it was to be added, the shell to ask again.
	LineAdded
	// CantAdd is switchboard's line offered, which setup couldn't add.
	CantAdd
)

// ShellFound is the situation the user's login shell, asked which claude it
// runs, leaves the claude link step in.
type ShellFound struct {
	Situation Situation
	// Said is the shell's row, labelled "Your shell", and what the line walk
	// says of it.
	Said Said
	// File is the startup file switchboard's line goes in, and Line the line,
	// where the shell is one setup asks, and the file one it can name.
	File, Line string
	// Last is set where switchboard's line, not taking effect or just added,
	// is the last thing in File that changes PATH.
	Last bool
	// ByHand is how to have claude go through switchboard by hand, as the
	// line walk shows it after Said: the bin directory last, but for a shell
	// setup doesn't ask, after which it says setup can't check it.
	ByHand []string
	// answer is the login shell's that left it.
	answer shell.Answer
}

// errNoLine is what AddLine fails with where setup offers no line to add.
var errNoLine = errors.New("setup offers no line to add")

// AddLine adds switchboard's line, which found offers, to the end of the
// startup file it offers it for, unless a line of switchboard's is there
// already, as when the user has added it meanwhile, so it's added once. It
// returns what that leaves: the line added, or found there, the login shell
// to ask again, as AskAgain does; a line there reading another switchboard
// shell directory's file, as Link says; or, where it can't be added, how to
// add it by hand. It fails, changing nothing, where found offers no line.
func (s *Setup) AddLine(found LinkFound) (LinkFound, error) {
	at := &found.Shell
	if at.Situation != OfferLine {
		return found, errNoLine
	}
	where := s.fromHome(at.File)
	sh, _ := shell.Of(s.Getenv("SHELL"))
	there, err := shell.FindLine(sh, at.File, s.Home)
	switch {
	case err != nil:
	case there.Sources == "":
		err = shell.AddLine(at.File, at.Line)
	case filepath.Clean(there.Sources) == shell.InitFile(s.Shells.Dir, sh):
		at.Situation, at.Last, at.ByHand = LineAdded, there.Last, nil
		at.Said = yourShell(Done, "switchboard's line is in "+where+" already", "switchboard's line is in "+where+" already.")
		return found, nil
	default:
		found.Shell = (&run{Setup: s, claudeCode: found.claudeCode}).onPath(at.answer)
		return found, nil
	}
	if err != nil {
		at.Situation = CantAdd
		at.Said = yourShell(Warning, fmt.Sprintf("setup can't add its line to %s: %v", where, err), fmt.Sprintf("Setup can't add it: %v.", err))
		return found, nil
	}
	logger.Info("added switchboard's line to the shell's startup file", "path", at.File)
	at.Situation, at.Last, at.ByHand = LineAdded, true, nil
	at.Said = yourShell(Done, "switchboard's line added to the end of "+where, "Added it to "+where+".")
	return found, nil
}

// AskAgain asks the user's login shell again which claude it runs, found
// being what setup found before, and returns the situation its answer
// leaves, sorted afresh, as Link sorts it; but where switchboard's line was
// just added, and the shell's PATH doesn't find the link, NotTaking, in the
// file it was added to. Elsewhere its sentence is the line walk's as it asks
// again: where claude doesn't go through switchboard yet, what keeps it from
// it, and no more. Unreached, there's nothing to ask: it returns found as it
// is. It fails with accounts.ErrInterrupted where the user interrupts the
// asking.
func (s *Setup) AskAgain(ctx context.Context, found LinkFound) (LinkFound, error) {
	return (&run{Setup: s, claudeCode: found.claudeCode}).askAgain(ctx, found)
}

// shellFiles writes switchboard's files for the shells, telling the line
// walk alone which it wrote or brought up to date, or that they were up to
// date already.
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
	var sentences []string
	if wrote := names[shell.Created]; len(wrote) > 0 {
		sentences = append(sentences, fmt.Sprintf("Wrote switchboard's files for the shells, in %s: %s.", dir, prose.List(wrote)))
	}
	if updated := names[shell.Updated]; len(updated) > 0 {
		sentences = append(sentences, fmt.Sprintf("Brought switchboard's files for the shells up to date, in %s: %s.", dir, prose.List(updated)))
	}
	if len(names[shell.Current]) == len(files) {
		sentences = append(sentences, fmt.Sprintf("switchboard's files for the shells are up to date, in %s.", dir))
	}
	r.tell(Said{Mark: Done, Sentence: strings.Join(sentences, "\n")})
	return nil
}

// onPath is the situation answer, the login shell's, leaves: through
// switchboard; an alias in front of the link that runs something else; or,
// for a shell switchboard knows, what keeps switchboard's line from having
// claude go through it, as startupLine says. Another shell setup can't ask,
// so it's left with the bin directory, to put on PATH by hand.
func (r *run) onPath(answer shell.Answer) ShellFound {
	at := r.sorted(answer)
	at.answer = answer
	return at
}

// sorted is the situation answer leaves, as onPath says.
func (r *run) sorted(answer shell.Answer) ShellFound {
	sh, known := shell.Of(r.Getenv("SHELL"))
	switch {
	case answer.Through():
		return r.through(answer)
	case !known:
		const cantCheck = "Setup can't check it for this shell: once it's done, claude in a new terminal goes through switchboard."
		return ShellFound{
			Situation: UnknownShell,
			Said:      yourShell(Warning, r.stands(answer)+" · setup can't check it for this shell: put switchboard's bin directory first on PATH by hand", r.notYet(answer)),
			ByHand:    append(r.byHand("To have claude go through switchboard, put this directory first on PATH, wherever your shell sets it:"), cantCheck),
		}
	case answer.Kind == shell.Link:
		return ShellFound{Situation: InFront, Said: r.notYetSaid(answer)}
	}
	return r.startupLine(sh, answer)
}

// startupLine is the situation switchboard's line for sh leaves, in the
// startup file the login shell, having answered answer, says it reads: none
// there, to offer, unless the shell couldn't be asked or a new terminal may
// read another; or what keeps the line there from taking effect.
func (r *run) startupLine(sh shell.Shell, answer shell.Answer) ShellFound {
	initFile := shell.InitFile(r.Shells.Dir, sh)
	line := shell.Line(sh, initFile, r.Home)
	file := shell.StartupFile(sh, r.Home, answer.ConfigDir)
	found, err := shell.FindLine(sh, file, r.Home)
	rival := shell.RivalStartupFile(sh, r.Home, file)
	where, notYet, stands := r.fromHome(file), r.notYet(answer), r.stands(answer)
	at := ShellFound{File: file, Line: line, ByHand: r.byHand(addByHand, line, orElsewhere)}
	switch {
	case err != nil:
		at.Situation = CantLook
		at.Said = yourShell(Warning,
			fmt.Sprintf("%s · setup can't look for its line in %s: %v", stands, where, err),
			fmt.Sprintf("%s Setup can't look for its line in %s: %v.", notYet, where, err))
	case found.Sources == "" && answer.Kind == shell.Unasked:
		at.Situation = WhichFile
		at.Said = yourShell(Warning,
			stands+" · setup can't tell which startup file it reads, so it adds no line",
			fmt.Sprintf("%s Setup can't tell which startup file %s reads, so it adds no line.", notYet, sh))
		at.ByHand = r.byHand(fmt.Sprintf("To have claude go through switchboard by hand, add this line to the end of %s's startup file, usually %s, after anything there that changes PATH:", sh, where), line, orElsewhere)
	case found.Sources == "" && rival != "":
		other := r.fromHome(rival)
		at.Situation = RivalFile
		at.Said = yourShell(Warning,
			fmt.Sprintf("%s · it says it reads %s, which isn't there, but %s is: setup can't tell which a new terminal reads, so it adds no line", stands, where, other),
			fmt.Sprintf("%s %s says it reads %s, which isn't there, but %s is: setup can't tell which a new terminal reads, so it adds no line.", notYet, sh, where, other))
		at.ByHand = r.byHand(fmt.Sprintf("To have claude go through switchboard by hand, add this line to the end of whichever of %s and %s a new terminal reads, after anything there that changes PATH:", where, other), line, orElsewhere)
	case found.Sources == "":
		at.Situation = OfferLine
		at.Said = yourShell(Warning, stands,
			fmt.Sprintf("%s This line, at the end of %s, has claude go through switchboard: it reads switchboard's file for %s, which puts %s first on PATH, and loads switchboard's completions.\n%s", notYet, where, sh, r.fromHome(r.Bin), line))
	case filepath.Clean(found.Sources) != initFile:
		reads := fmt.Sprintf("switchboard's line in %s reads %s, not this switchboard's file for %s, %s", where, r.fromHome(found.Sources), sh, r.fromHome(initFile))
		at.Situation = OtherLine
		at.Said = yourShell(Warning, stands+" · "+reads, notYet+" "+reads+".")
		at.ByHand = r.byHand("Put this line in its place:", line, orElsewhere)
	default:
		return r.notTakingEffect(file, line, found.Last, answer)
	}
	return at
}

// addByHand and orElsewhere say how to have claude go through switchboard by
// hand: by switchboard's line, shown after addByHand, or by the bin
// directory, shown after orElsewhere.
const (
	addByHand   = "To have claude go through switchboard by hand, add this line to the end of your shell's startup file, after anything there that changes PATH:"
	orElsewhere = "Or, where you keep your PATH another way, put this directory first on it:"
)

// notTakingEffect is the situation of switchboard's line, line, in file, the
// shell's startup file, where the login shell's PATH doesn't find the link,
// as answer, the shell's, says, or setup can't tell, as the shell couldn't
// be asked: something after the line changes PATH, or, where it's last,
// something read after the file.
func (r *run) notTakingEffect(file, line string, last bool, answer shell.Answer) ShellFound {
	where := r.fromHome(file)
	cause := "something after the line changes PATH"
	fix := fmt.Sprintf("move switchboard's line to the end of %s, or what changes PATH after it to before it.", where)
	if last {
		cause = fmt.Sprintf("something read after %s changes PATH", where)
		fix = "put switchboard's line after what changes PATH, wherever that's read."
	}
	at := ShellFound{Situation: NotTaking, File: file, Line: line, Last: last}
	if answer.Kind == shell.Unasked {
		at.Said = yourShell(Warning,
			fmt.Sprintf("switchboard's line is in %s, but setup can't ask it whether it takes effect: %s", where, answer.Why),
			fmt.Sprintf("switchboard's line is in %s, but setup can't ask %s whether it takes effect: %s.", where, r.shellName(), answer.Why))
		at.ByHand = r.byHand(fmt.Sprintf("Should it not, %s: %s %s", cause, fix, orElsewhere))
		return at
	}
	at.Said = yourShell(Warning,
		fmt.Sprintf("%s · switchboard's line is in %s, but %s", r.stands(answer), where, cause),
		fmt.Sprintf("switchboard's line is in %s, but %s, so %s.", where, r.runs(answer), cause))
	at.ByHand = r.byHand(fmt.Sprintf("To have claude go through switchboard, %s %s", fix, orElsewhere))
	return at
}

// byHand is how to have claude go through switchboard by hand, the lines
// how says it, then the bin directory, to put first on PATH.
func (r *run) byHand(how ...string) []string {
	return append(how, r.fromHomeVariable(r.Bin))
}

// settle has claude go through switchboard in a new terminal, which it does
// once the user's login shell, asked, says a claude typed there goes through
// switchboard's claude link. Until then, it tells the situation found
// leaves, and, for a shell switchboard knows, offers switchboard's line,
// where found does, or shows how to put it right by hand, and asks the
// shell again until it runs the link, or the user leaves it. Another shell
// it can't ask, so it shows the bin directory, to put on PATH by hand.
// Leaving it isn't a failure: setup goes on.
func (r *run) settle(ctx context.Context, found LinkFound) error {
	at := found.Shell
	r.tell(at.Said)
	switch at.Situation {
	case Unreached, Through:
		return nil
	case UnknownShell:
		r.show(at.ByHand)
		return nil
	case OfferLine:
		return r.offer(ctx, found)
	case LineAdded:
		again, err := r.askAgain(ctx, found)
		if err != nil {
			return err
		}
		return r.settle(ctx, again)
	}
	r.show(at.ByHand)
	return r.checkAgain(ctx, found)
}

// offer asks whether to add switchboard's line, which found offers, and adds
// it, as AddLine does, where the user says to, settling what that leaves;
// else it shows how to add it by hand, and asks the shell again, as
// checkAgain does.
func (r *run) offer(ctx context.Context, found LinkFound) error {
	add, err := r.Terminal.confirm("Add it?", true)
	switch {
	case err != nil:
		return err
	case !add:
		r.show(found.Shell.ByHand)
		return r.checkAgain(ctx, found)
	}
	added, err := r.AddLine(found)
	if err != nil {
		return err
	}
	return r.settle(ctx, added)
}

// show shows lines, how to have claude go through switchboard by hand.
func (r *run) show(lines []string) {
	for _, line := range lines {
		r.Terminal.say(line)
	}
}

// checkAgain asks the login shell again, as AskAgain does, found being what
// setup found before, as the user says to, until a claude typed in a new
// terminal goes through switchboard, or the user leaves it.
func (r *run) checkAgain(ctx context.Context, found LinkFound) error {
	for {
		again, err := r.Terminal.confirm("Check again?", true)
		if err != nil {
			return err
		}
		if !again {
			r.Terminal.sayf("Left as it is: until it's done, claude in a new terminal doesn't go through switchboard. Running setup again checks it.")
			return nil
		}
		if found, err = r.askAgain(ctx, found); err != nil {
			return err
		}
		r.tell(found.Shell.Said)
		if found.Shell.Situation == Through {
			return nil
		}
	}
}

// askAgain asks the login shell again, as AskAgain says.
func (r *run) askAgain(ctx context.Context, found LinkFound) (LinkFound, error) {
	was := found.Shell
	if was.Situation == Unreached {
		return found, nil
	}
	answer, err := r.ask(ctx)
	if err != nil {
		return found, err
	}
	switch {
	case was.Situation == LineAdded && !answer.Through() && answer.Kind != shell.Link:
		found.Shell = r.notTakingEffect(was.File, was.Line, was.Last, answer)
		found.Shell.answer = answer
	default:
		found.Shell = r.onPath(answer)
		if found.Shell.Situation != Through {
			found.Shell.Said.Sentence = r.notYet(answer)
		}
	}
	return found, nil
}

// ask asks the user's login shell which claude it runs, noting so first, as
// its startup files can take a while, where it's a shell setup asks. An
// interrupt meanwhile, which the shell, in a session of its own, never sees,
// ends the asking, and setup with it.
func (r *run) ask(ctx context.Context) (shell.Answer, error) {
	if _, known := shell.Of(r.Getenv("SHELL")); known && r.note != nil {
		r.note(fmt.Sprintf("Asking %s, as a new terminal starts it, which claude it runs.", r.shellName()))
	}
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
	return answer, nil
}

// interruptOnSIGINT ends the context it returns at an interrupt.
func interruptOnSIGINT(ctx context.Context) (context.Context, context.CancelFunc) {
	return signal.NotifyContext(ctx, os.Interrupt)
}

// yourShell is the login shell's row.
func yourShell(mark Mark, detail, sentence string) Said {
	return Said{Mark: mark, Label: "Your shell", Detail: detail, Sentence: sentence}
}

// through is the situation of claude going through switchboard in a new
// terminal, as answer, the login shell's, says, with what's in front of
// PATH's claude there, as linkedFront says it.
func (r *run) through(answer shell.Answer) ShellFound {
	detail := "runs switchboard's claude: asked just now, as a new terminal starts it"
	sentence := fmt.Sprintf("claude goes through switchboard in a new terminal: %s runs %s.", r.shellName(), r.fromHome(r.claudeLink()))
	if front := r.linkedFront(answer.Front); front != "" {
		detail += " · " + front
		sentence += "\n" + front + "."
	}
	return ShellFound{Situation: Through, Said: yourShell(Done, detail, sentence)}
}

// linkedFront says what's in front of PATH's claude where claude goes
// through switchboard: an alias that runs claude by name, or the link by its
// path, goes through switchboard; a function, or an alias setup doesn't
// read, does where it runs claude by name. It's "" where nothing is.
func (r *run) linkedFront(front shell.Front) string {
	switch {
	case front.Text == "":
		return ""
	case front.Runs == shell.ByName:
		return fmt.Sprintf("%s's claude is %s, which runs claude by name, so goes through switchboard too", r.shellName(), front.Text)
	case front.Runs == shell.TheLink:
		return fmt.Sprintf("%s's claude is %s, which runs switchboard's claude link by its path", r.shellName(), front.Text)
	}
	return fmt.Sprintf("%s's claude is %s, which setup doesn't read: it goes through switchboard where it runs claude by name", r.shellName(), front.Text)
}

// notYetSaid is the login shell's row where claude doesn't go through
// switchboard in a new terminal yet, as answer, the shell's, says.
func (r *run) notYetSaid(answer shell.Answer) Said {
	return yourShell(Warning, r.stands(answer), r.notYet(answer))
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
	front := r.inFront(answer.Front, answer.Front.Path)
	switch {
	case answer.Kind == shell.Link:
		return not + front + "."
	case front != "":
		return not + r.runs(answer) + ". " + front + "."
	}
	return not + r.runs(answer) + "."
}

// stands says, as the login shell's row does, what notYet says keeps claude
// from going through switchboard in a new terminal yet.
func (r *run) stands(answer shell.Answer) string {
	if answer.Kind == shell.Unasked {
		return "setup can't ask it which claude it runs: " + answer.Why
	}
	whose := "another claude"
	if r.claudeCode != "" && filepath.Clean(answer.Claude) == filepath.Clean(r.claudeCode) {
		whose = "Claude Code's own claude"
	}
	finds := fmt.Sprintf("runs %s, %s, so its sessions aren't routed", whose, r.fromHome(answer.Claude))
	if answer.Kind == shell.None {
		finds = "finds no claude, so claude doesn't go through switchboard"
	}
	front := r.inFront(answer.Front, r.fromHome(answer.Front.Path))
	switch {
	case answer.Kind == shell.Link:
		return front
	case front != "":
		return finds + " · " + front
	}
	return finds
}

// inFront says what an alias in front of PATH's claude runs, where that's
// not switchboard's claude, a path it runs written as path, and how to put
// it right: "" where it's nothing else.
func (r *run) inFront(front shell.Front, path string) string {
	const fix = "removing the alias, or pointing it at claude, puts it right"
	switch front.Runs {
	case shell.APath:
		return fmt.Sprintf("%s's claude is %s, which runs %s, not switchboard's claude: %s", r.shellName(), front.Text, path, fix)
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
func (s *Setup) fromHome(path string) string {
	return s.relative(path, "~/")
}

// fromHomeVariable writes path from $HOME where it's in the home, as a shell
// reads it between double quotes, where it reads no ~.
func (s *Setup) fromHomeVariable(path string) string {
	return s.relative(path, "$HOME/")
}

// relative writes path from home, written as home says, where it's in the
// home.
func (s *Setup) relative(path, home string) string {
	rel, err := filepath.Rel(s.Home, path)
	if s.Home == "" || err != nil || !filepath.IsLocal(rel) {
		return path
	}
	return home + filepath.ToSlash(rel)
}
