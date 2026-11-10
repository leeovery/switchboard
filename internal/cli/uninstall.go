package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/leeovery/switchboard/internal/atomicfile"
	"github.com/leeovery/switchboard/internal/config"
	"github.com/leeovery/switchboard/internal/redact"
	"github.com/leeovery/switchboard/internal/service"
	"github.com/leeovery/switchboard/internal/sessionhook"
	"github.com/leeovery/switchboard/internal/setup"
	"github.com/leeovery/switchboard/internal/shell"
	"github.com/leeovery/switchboard/internal/skill"
	"github.com/leeovery/switchboard/internal/theme"
)

func newUninstallCommand(a *app) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   "uninstall",
		Short: "Take away what setup put on this Mac, before brew uninstall",
		Long: `Take away what setup put on this Mac, a line each: the service, as service
uninstall takes it away; the line setup added to the shell's startup file,
and switchboard's own files for the shells; switchboard's entry in Claude
Code's SessionStart hooks, every other setting and hook left as it is; the
skill; and the claude link. The config, the accounts' token files and the
recorded history stay, so a reinstall carries on where it left off, unless
--all takes them too. The export's folder is never touched, nor switchboard's
binary, which brew uninstall switchboard removes.

Nothing asks first. A piece that fails doesn't stop the rest, and uninstall
fails after the last. Run again, it says each piece isn't there.`,
		Args:        noArgs,
		Annotations: map[string]string{unmadeAnnotation: ""},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.uninstall(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), all)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "take the config, the accounts' token files and the recorded history too")
	return cmd
}

// errNotAllRemoved is what uninstall fails with where a piece failed.
var errNotAllRemoved = errors.New("some of switchboard couldn't be taken away, as said above: put it right, then run switchboard uninstall again")

// The labels of uninstall's lines, a piece each.
const (
	serviceLabel = "Service"
	lineLabel    = "Startup line"
	shellLabel   = "Shell files"
	hookLabel    = "Session hook"
	skillLabel   = "Skill"
	linkLabel    = "claude link"
	dataLabel    = "Data"
	keptLabel    = "Kept"
	configLabel  = "Config"
	stateLabel   = "State"
	exportLabel  = "Export"
)

// The last lines uninstall says.
const (
	tokensStay   = "The accounts' setup tokens stay valid on Anthropic's side until their year is out, their files removed here or not."
	brewLastLine = "Last, brew uninstall switchboard removes switchboard itself."
)

// uninstall takes away what setup put in place, in turn, saying on out what
// became of each piece, then, with all, the config and the state directory,
// keeping the export's folder wherever it is; and fails, once it's said the
// last, where any piece failed.
func (a *app) uninstall(ctx context.Context, out, errOut io.Writer, all bool) error {
	u := &uninstalling{out: out, widths: widths(nil, labelRows()...)}
	a.uninstallPieces(ctx, u, errOut)
	if all {
		a.uninstallAll(u)
	} else {
		u.done(keptLabel, "the config, the accounts' token files and the history: switchboard uninstall --all removes them too")
	}
	_, _ = fmt.Fprintln(out)
	_, _ = fmt.Fprintln(out, tokensStay)
	_, _ = fmt.Fprintln(out, brewLastLine)
	if u.failed {
		return errNotAllRemoved
	}
	return nil
}

// labelRows are rows of every label uninstall's lines may have, after
// their marks, which its columns are as wide as.
func labelRows() [][]string {
	var rows [][]string
	for _, label := range []string{serviceLabel, lineLabel, shellLabel, hookLabel, skillLabel, linkLabel, dataLabel, keptLabel, configLabel, stateLabel, exportLabel} {
		rows = append(rows, []string{doneMark, label, ""})
	}
	return rows
}

// uninstallPieces takes away, in turn, what setup put in place: the
// service, the startup line and the shells' files, the session hook, the
// skill and the claude link, then switchboard's data directory, once
// nothing's left in it.
func (a *app) uninstallPieces(ctx context.Context, u *uninstalling, errOut io.Writer) {
	a.uninstallTheService(ctx, u, errOut)
	a.uninstallTheShell(ctx, u)
	path, removed, err := a.uninstallTheHook()
	u.outcome(hookLabel, "removed from "+path, removed, err)
	path, removed, err = a.uninstallTheSkill()
	u.outcome(skillLabel, "removed "+path, removed, err)
	a.uninstallTheLink(u)
	a.uninstallTheData(u)
}

// uninstallTheData removes switchboard's data directory, once nothing's
// left in it, saying so, or that it's kept, as something's still in it:
// nothing where it isn't there.
func (a *app) uninstallTheData(u *uninstalling) {
	dir, err := config.DataDir(a.Getenv, a.HomeDir)
	if err != nil {
		u.fail(dataLabel, err)
		return
	}
	removed, err := atomicfile.RemoveIfEmpty(dir)
	switch {
	case err != nil:
		u.fail(dataLabel, err)
	case removed:
		u.done(dataLabel, "removed "+dir)
	case there(dir):
		u.warn(dataLabel, "kept "+dir+": something's still in it")
	}
}

// there reports whether there's anything at path.
func there(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// uninstallTheService takes the service away, as service uninstall does.
// Off macOS, there's none.
func (a *app) uninstallTheService(ctx context.Context, u *uninstalling, errOut io.Writer) {
	removal, err := a.removeService(ctx, errOut)
	switch {
	case errors.Is(err, service.ErrUnsupported):
		u.done(serviceLabel, notThere)
	case err != nil:
		u.fail(serviceLabel, err)
	case removal.app != nil:
		did := notThere
		if removal.removed {
			did = "removed " + removal.plist
		}
		u.broke(serviceLabel, did+", but the notifications' app: "+oneLine(removal.app))
	case removal.removed:
		u.done(serviceLabel, "removed "+removal.plist)
	default:
		u.done(serviceLabel, notThere)
	}
}

// uninstallTheShell takes switchboard's line out of every startup file it's
// in, then removes switchboard's files for the shells.
func (a *app) uninstallTheShell(ctx context.Context, u *uninstalling) {
	dir, err := config.ShellDir(a.Getenv, a.HomeDir)
	if err != nil {
		u.fail(lineLabel, err)
		u.fail(shellLabel, err)
		return
	}
	a.uninstallTheLine(ctx, u, dir)
	removed, err := shell.RemoveFiles(dir)
	u.outcome(shellLabel, "removed from "+dir, anyRemoved(removed), err)
}

// uninstallTheLine takes switchboard's line, sourcing a file in dir,
// switchboard's shell directory, out of every startup file it's in. Where
// the user's login shell reads its startup file is the shell's to say, so
// it's asked, as setup asks it: where it can't be, uninstall says it couldn't
// look there, never that the line's not there.
func (a *app) uninstallTheLine(ctx context.Context, u *uninstalling, dir string) {
	home, err := a.HomeDir()
	if err != nil {
		u.fail(lineLabel, fmt.Errorf("locate the home directory: %w", err))
		return
	}
	bin, err := config.BinDir(a.Getenv, a.HomeDir)
	if err != nil {
		u.fail(lineLabel, err)
		return
	}
	login, _ := shell.Of(a.Getenv("SHELL"))
	answer := shell.Login{Environ: a.Environ(), Run: a.LoginShell}.Which(ctx, setup.ClaudeLink(bin))
	lines, err := shell.RemoveLines(home, dir, login, answer.ConfigDir)
	for _, file := range lines.Removed {
		u.done(lineLabel, "removed from "+file)
	}
	for _, file := range lines.Indented {
		u.warn(lineLabel, "left alone: an indented copy in "+file+", which may be all of a block of yours, so it's yours to take out: with switchboard's files gone it does nothing")
	}
	unlooked := shell.SaysWhere(login) && answer.Kind == shell.Unasked
	if unlooked {
		u.warn(lineLabel, fmt.Sprintf("couldn't look where %s reads its startup file, as it couldn't be asked: %s", login, answer.Why))
	}
	switch {
	case err != nil:
		u.fail(lineLabel, err)
	case len(lines.Removed) == 0 && len(lines.Indented) == 0 && !unlooked:
		u.done(lineLabel, notThere)
	}
}

// anyRemoved reports whether any of files was removed.
func anyRemoved(files []shell.File) bool {
	for _, f := range files {
		if f.State == shell.Removed {
			return true
		}
	}
	return false
}

// uninstallTheHook takes switchboard's entry out of Claude Code's settings,
// returning where they are, and whether it was there.
func (a *app) uninstallTheHook() (string, bool, error) {
	path, err := sessionhook.Path(a.Getenv, a.HomeDir)
	if err != nil {
		return "", false, err
	}
	removed, err := sessionhook.Uninstall(path)
	return path, removed, err
}

// uninstallTheSkill removes the skill, returning where it was, and whether
// it was there.
func (a *app) uninstallTheSkill() (string, bool, error) {
	path, err := skill.Path(a.Getenv, a.HomeDir)
	if err != nil {
		return "", false, err
	}
	removed, err := skill.Uninstall(path)
	return path, removed, err
}

// uninstallTheLink removes switchboard's claude link, and its bin directory
// once nothing's left in it, leaving a claude of anyone else's alone.
func (a *app) uninstallTheLink(u *uninstalling) {
	bin, err := config.BinDir(a.Getenv, a.HomeDir)
	if err != nil {
		u.fail(linkLabel, err)
		return
	}
	link := setup.ClaudeLink(bin)
	removal, err := setup.RemoveLink(bin)
	switch {
	case err != nil:
		u.fail(linkLabel, err)
	case removal == setup.LinkRemoved:
		u.done(linkLabel, "removed "+link)
	case removal == setup.LinkNotOurs:
		u.warn(linkLabel, "left alone: "+link+" isn't switchboard's link, so it's someone else's")
	default:
		u.done(linkLabel, notThere)
	}
}

// uninstallAll removes the config and the state directory, keeping the
// export's folder wherever it is: both, where the config can't say where
// that is. A config file the environment puts outside the config directory
// goes alone, and of a themes directory it puts there, the theme files.
func (a *app) uninstallAll(u *uninstalling) {
	file, err := a.configFile()
	if err != nil {
		u.fail(configLabel, err)
		return
	}
	export, err := config.ExportFolderOf(file, a.HomeDir)
	if err != nil {
		why := fmt.Errorf("kept, with the state directory, as where the export goes can't be told: %w", err)
		u.fail(configLabel, why)
		u.fail(stateLabel, why)
		return
	}
	kept := false
	if places, err := config.PlacesOf(a.Getenv, a.HomeDir, file); err != nil {
		u.fail(configLabel, err)
	} else {
		kept = u.removeAll(configLabel, places.Dir, export)
		u.removeElsewhere(places)
	}
	if state, err := config.StateDir(a.Getenv, a.HomeDir); err != nil {
		u.fail(stateLabel, err)
	} else {
		kept = u.removeAll(stateLabel, state, export) || kept
	}
	if kept {
		u.done(exportLabel, "kept "+export+": the export's folder, which uninstall never removes")
	}
}

// removeElsewhere removes the config file places puts outside the config
// directory, that file alone, and the theme files in the themes directory it
// puts there, saying what became of each.
func (u *uninstalling) removeElsewhere(places config.Places) {
	if places.File != "" {
		removal, err := config.RemoveFile(places.File)
		u.removal(configLabel, places.File, removal, err)
	}
	if places.Themes == "" {
		return
	}
	removed, err := theme.NewLibrary(places.Themes).RemoveFiles()
	switch {
	case err != nil:
		u.fail(configLabel, err)
	case removed > 0:
		u.done(configLabel, "removed the theme files from "+places.Themes)
	default:
		u.done(configLabel, "no theme files in "+places.Themes)
	}
}

// notThere is what uninstall says of a piece that isn't there.
const notThere = "not there"

// The marks of uninstall's lines, as setup's: done, done but for something
// the user may want to see to, and failed.
const (
	doneMark   = "✓"
	warnMark   = "!"
	failedMark = "✗"
)

// uninstalling is uninstall's lines, written on out as it goes, in columns
// as wide as widths, and whether any piece failed.
type uninstalling struct {
	out    io.Writer
	widths []int
	failed bool
}

// done says label's piece is as detail says.
func (u *uninstalling) done(label, detail string) {
	u.say(doneMark, label, detail)
}

// warn says label's piece is as detail says, which the user may want to see
// to.
func (u *uninstalling) warn(label, detail string) {
	u.say(warnMark, label, detail)
}

// fail says label's piece failed, as err says.
func (u *uninstalling) fail(label string, err error) {
	u.broke(label, "failed: "+oneLine(err))
}

// broke says label's piece failed, as detail says.
func (u *uninstalling) broke(label, detail string) {
	u.failed = true
	u.say(failedMark, label, detail)
}

// oneLine is err's text on one line, as a row says it: each line break, as
// between the errors joined from several files, a "; ", anything shaped like
// a token hidden.
func oneLine(err error) string {
	return strings.ReplaceAll(redact.Text(err.Error()), "\n", "; ")
}

func (u *uninstalling) say(mark, label, detail string) {
	_ = writeRow(u.out, []string{mark, label, detail}, u.widths)
}

// outcome says what became of label's piece: that it failed, as err says,
// where it did; else, where it was removed, done as detail says; else that
// it wasn't there.
func (u *uninstalling) outcome(label, detail string, removed bool, err error) {
	switch {
	case err != nil:
		u.fail(label, err)
	case removed:
		u.done(label, detail)
	default:
		u.done(label, notThere)
	}
}

// removeAll removes place, everything in it but keep, the export's folder,
// saying what became of it under label, and reports whether keep was kept
// in it.
func (u *uninstalling) removeAll(label, place, keep string) bool {
	removal, err := config.RemoveAll(place, keep)
	u.removal(label, place, removal, err)
	return removal == config.KeptFor
}

// removal says what became of place, as removal and err say, under label.
func (u *uninstalling) removal(label, place string, removal config.Removal, err error) {
	switch {
	case err != nil:
		u.fail(label, err)
	case removal == config.NotThere:
		u.done(label, notThere+": "+place)
	case removal == config.KeptFor:
		u.done(label, "removed all but the export's folder from "+place)
	case removal == config.Kept:
		u.done(label, "kept "+place+": it's the export's folder")
	default:
		u.done(label, "removed "+place)
	}
}
