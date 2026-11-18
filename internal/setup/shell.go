package setup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/leeovery/switchboard/internal/accounts"
	"github.com/leeovery/switchboard/internal/claude"
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
	// OfferCompletions is a shell that runs switchboard's claude already,
	// as through a PATH kept in another file, but whose startup file is
	// without switchboard's line, which setup offers to add for its
	// completions alone, as AddLine does.
	OfferCompletions
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
	// Said is the shell's row, labelled "Your shell".
	Said Said
	// File is the startup file switchboard's line goes in, and Line the line,
	// where the shell is one setup asks, and the file one it can name; Where
	// is File written from ~, as the user reads it.
	File, Line, Where string
	// Rival is the other startup file a new terminal may read, from ~, where
	// setup can't tell which it reads: "" elsewhere.
	Rival string
	// Front is what an alias in front of the claude the shell's PATH finds
	// runs, where that's not switchboard's claude, and how to put it right:
	// "" where nothing else stands in front.
	Front string
	// Unasked is set where the shell couldn't be asked which claude it runs.
	Unasked bool
	// Dir is switchboard's bin directory, written from $HOME, to put first on
	// PATH by hand, once the shell's been asked.
	Dir string
	// Last is set where switchboard's line, not taking effect or just added,
	// is the last thing in File that changes PATH.
	Last bool
	// answer is the login shell's that left it.
	answer shell.Answer
}

// Remedy is what a situation leaves to put right by hand, as the claude
// link's page offers it, and Done's line under it: Parts, what to do, in
// turn; Line and Dir, switchboard's line and the bin directory, each to
// copy where a part puts it in place, "" elsewhere; and Checkable, whether
// asking the shell again can tell it's put right.
type Remedy struct {
	Parts     []RemedyPart
	Line, Dir string
	Checkable bool
}

// RemedyPart is a part of what to do to put the situation right by hand:
// what to do, and what to put in place, where there's anything, the line or
// the directory.
type RemedyPart struct {
	Says, Put string
}

// Remedy is what the situation leaves to put right by hand: nothing where a
// new terminal's claude goes through switchboard already, or setup stopped
// before it asked the shell; the bin directory alone for a shell setup
// doesn't ask, which it can't check; asking again alone where an alias in
// front is to go, or switchboard's line is in already; where the line is
// there but not taking effect, moving it, or what changes PATH after it,
// else the directory; where it reads another switchboard's file, the line
// put in its place, else the directory; and anywhere else, the line added,
// else the directory.
func (s ShellFound) Remedy() Remedy {
	switch s.Situation {
	case Unreached, Through, OfferCompletions:
		return Remedy{}
	case UnknownShell:
		return Remedy{Parts: []RemedyPart{{Says: capital(dirFirst), Put: s.Dir}}, Dir: s.Dir}
	case InFront, LineAdded:
		return Remedy{Checkable: true}
	case NotTaking:
		cause, fix := notTakingFix(s.Where, s.Last)
		says := capital(fix)
		if s.Unasked {
			says = shouldItNot(cause, fix)
		}
		return Remedy{Parts: []RemedyPart{{Says: says}, {Says: orElsewhere, Put: s.Dir}}, Dir: s.Dir, Checkable: true}
	case OtherLine:
		return s.byLine(inItsPlace)
	}
	return s.byLine(addYourself)
}

// Gains says what switchboard's line, offered, gains the shell, as the card
// offering it says: switchboard's completions, where the shell runs its
// claude already; else its claude first.
func (s ShellFound) Gains() string {
	if s.Situation == OfferCompletions {
		return "so your shell loads switchboard's completions"
	}
	return "so your shell finds switchboard's claude first"
}

// Final reports whether the situation ends the claude link step as it's
// found, leaving nothing to put right by hand: a new terminal's claude goes
// through switchboard, or setup stopped before it asked the shell.
func (s ShellFound) Final() bool {
	switch s.Situation {
	case Unreached, Through, OfferCompletions:
		return true
	}
	return false
}

// byLine is the remedy of switchboard's line put in place, as says says,
// else the bin directory put first on PATH, either checked by asking again.
func (s ShellFound) byLine(says string) Remedy {
	return Remedy{
		Parts: []RemedyPart{{Says: says, Put: s.Line}, {Says: orElsewhere, Put: s.Dir}},
		Line:  s.Line, Dir: s.Dir, Checkable: true,
	}
}

// first is what the remedy's first part puts in place, which Done offers to
// copy, and whether that's the bin directory: "" where it puts nothing.
func (r Remedy) first() (put string, directory bool) {
	if len(r.Parts) == 0 {
		return "", false
	}
	put = r.Parts[0].Put
	return put, put != "" && put == r.Dir
}

// errNoLine is what AddLine fails with where setup offers no line to add.
var errNoLine = errors.New("setup offers no line to add")

// AddLine adds switchboard's line, which found offers, to the end of the
// startup file it offers it for, unless a line of switchboard's is there
// already, as when the user has added it meanwhile, so it's added once. It
// returns what that leaves: the line added, or found there, the login shell
// to ask again, as AskAgain does; a line there reading another switchboard
// shell directory's file, as Link says; or, where it can't be added, how to
// add it by hand, but of a line offered for its completions alone, the
// situation as it was, saying why. It fails, changing nothing, where found
// offers no line.
func (s *Setup) AddLine(found LinkFound) (LinkFound, error) {
	at := &found.Shell
	if at.Situation != OfferLine && at.Situation != OfferCompletions {
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
		at.Situation, at.Last = LineAdded, there.Last
		at.Said = yourShell(Done, "switchboard's line is in "+where+" already")
		return found, nil
	default:
		found.Shell = (&run{Setup: s, claudeCode: found.claudeCode}).onPath(at.answer)
		return found, nil
	}
	if err != nil {
		cant := fmt.Sprintf("setup can't add its line to %s: %v", where, err)
		if at.Situation == OfferCompletions {
			at.Said = yourShell(Warning, cant+", so switchboard's completions don't load")
			return found, nil
		}
		at.Situation, at.Said = CantAdd, yourShell(Warning, cant)
		return found, nil
	}
	logger.Info("added switchboard's line to the shell's startup file", "path", at.File)
	at.Situation, at.Last = LineAdded, true
	at.Said = yourShell(Done, "switchboard's line added to the end of "+where)
	return found, nil
}

// AskAgain asks the user's login shell again which claude it runs, found
// being what setup found before, and returns the situation its answer
// leaves, sorted afresh, as Link sorts it; but where switchboard's line was
// just added, and the shell's PATH doesn't find the link, NotTaking, in the
// file it was added to. Unreached, there's nothing to ask: it returns found
// as it is. It fails with accounts.ErrInterrupted where the user interrupts
// the asking.
func (s *Setup) AskAgain(ctx context.Context, found LinkFound) (LinkFound, error) {
	return (&run{Setup: s, claudeCode: found.claudeCode}).askAgain(ctx, found)
}

// shellFiles writes switchboard's files for the shells, logging each it
// wrote or brought up to date.
func (r *run) shellFiles() error {
	files, err := r.Shells.Write()
	if err != nil {
		return err
	}
	for _, f := range files {
		switch f.State {
		case shell.Created:
			logger.Info("wrote switchboard's file for a shell", "path", f.Path)
		case shell.Updated:
			logger.Info("brought switchboard's file for a shell up to date", "path", f.Path)
		}
	}
	return nil
}

// onPath is the situation answer, the login shell's, leaves: through
// switchboard; an alias in front of the link that runs something else; or,
// for a shell switchboard knows, what keeps switchboard's line from having
// claude go through it, as startupLine says. Another shell setup can't ask,
// so it's left with the bin directory, to put on PATH by hand.
func (r *run) onPath(answer shell.Answer) ShellFound {
	return r.answered(r.sorted(answer), answer)
}

// answered is at, the situation answer, the login shell's, leaves, with
// what the answer says beside it: whether the shell couldn't be asked, the
// alias in front of PATH's claude, where it runs something else, and the
// bin directory to put first on PATH by hand.
func (r *run) answered(at ShellFound, answer shell.Answer) ShellFound {
	at.answer, at.Unasked, at.Dir = answer, answer.Kind == shell.Unasked, r.binDir()
	at.Front = r.inFront(answer.Front, r.fromHome(answer.Front.Path))
	return at
}

// sorted is the situation answer leaves, as onPath says.
func (r *run) sorted(answer shell.Answer) ShellFound {
	sh, known := shell.Of(r.Getenv("SHELL"))
	switch {
	case answer.Through():
		return r.through(answer, sh, known)
	case !known:
		return ShellFound{
			Situation: UnknownShell,
			Said:      yourShell(Warning, r.stands(answer)+" · setup can't check it for this shell: put switchboard's bin directory first on PATH by hand"),
		}
	case answer.Kind == shell.Link:
		return ShellFound{Situation: InFront, Said: yourShell(Warning, r.stands(answer))}
	}
	return r.startupLine(sh, answer)
}

// startup is switchboard's line for a shell in the startup file the login
// shell says it reads: the file, and where, the file from ~; switchboard's
// file for the shell, which the line reads, and the line; what's there of
// switchboard's, or why setup can't look; and the other file a new terminal
// may read, from ~, "" where there's none.
type startup struct {
	file, where, initFile, line string
	found                       shell.Found
	err                         error
	rival                       string
}

// startupOf is switchboard's line for sh in the startup file the login
// shell, having answered answer, says it reads.
func (r *run) startupOf(sh shell.Shell, answer shell.Answer) startup {
	s := startup{initFile: shell.InitFile(r.Shells.Dir, sh), file: shell.StartupFile(sh, r.Home, answer.ConfigDir)}
	s.line, s.where = shell.Line(sh, s.initFile, r.Home), r.fromHome(s.file)
	s.found, s.err = shell.FindLine(sh, s.file, r.Home)
	if rival := shell.RivalStartupFile(sh, r.Home, s.file); rival != "" {
		s.rival = r.fromHome(rival)
	}
	return s
}

// offerable reports whether setup can offer to add switchboard's line to
// the file: it can look in it, the line isn't there, the shell said where
// it reads it, and a new terminal reads no other.
func (s startup) offerable(answer shell.Answer) bool {
	return s.err == nil && s.found.Sources == "" && answer.Kind != shell.Unasked && s.rival == ""
}

// startupLine is the situation switchboard's line for sh leaves, in the
// startup file the login shell, having answered answer, says it reads: none
// there, to offer, unless the shell couldn't be asked or a new terminal may
// read another; or what keeps the line there from taking effect.
func (r *run) startupLine(sh shell.Shell, answer shell.Answer) ShellFound {
	s := r.startupOf(sh, answer)
	stands := r.stands(answer)
	at := ShellFound{File: s.file, Line: s.line, Where: s.where}
	switch {
	case s.err != nil:
		at.Situation = CantLook
		at.Said = yourShell(Warning, fmt.Sprintf("%s · setup can't look for its line in %s: %v", stands, s.where, s.err))
	case s.offerable(answer):
		at.Situation = OfferLine
		at.Said = yourShell(Warning, stands)
	case s.found.Sources == "" && answer.Kind == shell.Unasked:
		at.Situation = WhichFile
		at.Said = yourShell(Warning, stands+" · setup can't tell which startup file it reads, so it adds no line")
	case s.found.Sources == "":
		at.Situation, at.Rival = RivalFile, s.rival
		at.Said = yourShell(Warning,
			fmt.Sprintf("%s · it says it reads %s, which isn't there, but %s is: setup can't tell which a new terminal reads, so it adds no line", stands, s.where, s.rival))
	case filepath.Clean(s.found.Sources) != s.initFile:
		at.Situation = OtherLine
		at.Said = yourShell(Warning,
			fmt.Sprintf("%s · switchboard's line in %s reads %s, not this switchboard's file for %s, %s", stands, s.where, r.fromHome(s.found.Sources), sh, r.fromHome(s.initFile)))
	default:
		return r.notTakingEffect(s.file, s.line, s.found.Last, answer)
	}
	return at
}

// What setup says to have claude go through switchboard by hand: by
// switchboard's line, after addYourself, or after inItsPlace, in place of
// another switchboard's; or by the bin directory, after orElsewhere, or for
// a shell setup doesn't ask, after dirFirst.
const (
	addYourself = "Add this line to your shell's startup file yourself, after anything else there that changes PATH:"
	inItsPlace  = "Put this line in its place:"
	orElsewhere = "Or, where you keep your PATH another way, put this directory first on it:"
	dirFirst    = "put this directory first on PATH, wherever your shell sets it:"
)

// notTakingEffect is the situation of switchboard's line, line, in file, the
// shell's startup file, where the login shell's PATH doesn't find the link,
// as answer, the shell's, says, or setup can't tell, as the shell couldn't
// be asked: something after the line changes PATH, or, where it's last,
// something read after the file.
func (r *run) notTakingEffect(file, line string, last bool, answer shell.Answer) ShellFound {
	where := r.fromHome(file)
	at := ShellFound{Situation: NotTaking, File: file, Line: line, Where: where, Last: last}
	if answer.Kind == shell.Unasked {
		at.Said = yourShell(Warning, fmt.Sprintf("switchboard's line is in %s, but setup can't ask it whether it takes effect: %s", where, answer.Why))
		return at
	}
	cause, _ := notTakingFix(where, last)
	at.Said = yourShell(Warning, fmt.Sprintf("%s · switchboard's line is in %s, but %s", r.stands(answer), where, cause))
	return at
}

// notTakingFix is what keeps switchboard's line, in the startup file written
// where, from taking effect, and how to put it right: something after it
// changes PATH, so it's to move to the end; or, where it's last, something
// read after the file does, so it's to go after that.
func notTakingFix(where string, last bool) (cause, fix string) {
	if last {
		return fmt.Sprintf("something read after %s changes PATH", where), "put switchboard's line after what changes PATH, wherever that's read."
	}
	return "something after the line changes PATH", fmt.Sprintf("move switchboard's line to the end of %s, or what changes PATH after it to before it.", where)
}

// shouldItNot says how to put switchboard's line right, should it not take
// effect for cause, as fix says, where the shell couldn't be asked whether
// it does.
func shouldItNot(cause, fix string) string {
	return fmt.Sprintf("Should it not, %s: %s", cause, fix)
}

// binDir is switchboard's bin directory, written from $HOME, to put first on
// PATH by hand.
func (r *run) binDir() string {
	return r.fromHomeVariable(r.Bin)
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
		found.Shell = r.answered(r.notTakingEffect(was.File, was.Line, was.Last, answer), answer)
	default:
		found.Shell = r.onPath(answer)
	}
	return found, nil
}

// ask asks the user's login shell which claude it runs, as Ask does, and
// setup ends with the asking where it's interrupted.
func (r *run) ask(ctx context.Context) (shell.Answer, error) {
	return Ask(ctx, r.Login, r.claudeLink(), r.Interrupt)
}

// Ask asks login, the user's login shell, which claude it runs, link being
// switchboard's claude link, as setup asks it. An interrupt meanwhile, taken
// as interrupt takes it, or through os/signal where it's nil, which the
// shell, in a session of its own, never sees, ends the asking, which fails
// with accounts.ErrInterrupted.
func Ask(ctx context.Context, login LoginShell, link string, interrupt func(context.Context) (context.Context, context.CancelFunc)) (shell.Answer, error) {
	asking, stop := interruptible(ctx, interrupt)
	defer stop()
	answer := login.Which(asking, link)
	if asking.Err() != nil && ctx.Err() == nil {
		return answer, accounts.ErrInterrupted
	}
	return answer, nil
}

// interruptible returns a context that ends with ctx, or at an interrupt,
// as setup's Interrupt says.
func (s *Setup) interruptible(ctx context.Context) (context.Context, context.CancelFunc) {
	return interruptible(ctx, s.Interrupt)
}

// interruptible returns a context that ends with ctx, or at an interrupt,
// taken as interrupt takes it, or through os/signal where it's nil.
func interruptible(ctx context.Context, interrupt func(context.Context) (context.Context, context.CancelFunc)) (context.Context, context.CancelFunc) {
	if interrupt != nil {
		return interrupt(ctx)
	}
	return signal.NotifyContext(ctx, os.Interrupt)
}

// yourShell is the login shell's row.
func yourShell(mark Mark, detail string) Said {
	return Said{Mark: mark, Label: "Your shell", Detail: detail}
}

// through is the situation of claude going through switchboard in a new
// terminal, as answer, the login shell's, says, with what's in front of
// PATH's claude there, as linkedFront says it; but where the shell is one
// setup knows, sh, and its startup file is one setup can offer switchboard's
// line to, as startup's offerable has it, OfferCompletions, the line
// offered for its completions, as a PATH kept in another file finds
// switchboard's claude without loading them.
func (r *run) through(answer shell.Answer, sh shell.Shell, known bool) ShellFound {
	detail := "runs switchboard's claude: asked just now, as a new terminal starts it"
	if front := r.linkedFront(answer.Front); front != "" {
		detail += " · " + front
	}
	at := ShellFound{Situation: Through, Said: yourShell(Done, detail)}
	if !known {
		return at
	}
	if s := r.startupOf(sh, answer); s.offerable(answer) {
		at.Situation, at.File, at.Line, at.Where = OfferCompletions, s.file, s.line, s.where
		at.Said.Detail += " · switchboard's line isn't in " + s.where + ", so its completions don't load"
	}
	return at
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

// stands says, as the login shell's row does, what keeps claude from going
// through switchboard in a new terminal yet: what its PATH finds, and an
// alias in front of it that runs something else; or that setup can't tell,
// as the shell couldn't be asked.
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

// shellName names the user's login shell, by its program's name.
func (r *run) shellName() string {
	if path := r.Getenv("SHELL"); path != "" {
		return filepath.Base(path)
	}
	return "your login shell"
}

// claudeLink is switchboard's claude link.
func (r *run) claudeLink() string {
	return ClaudeLink(r.Bin)
}

// ClaudeLink is switchboard's claude link in bin, switchboard's bin
// directory.
func ClaudeLink(bin string) string {
	return filepath.Join(bin, claude.Command)
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
