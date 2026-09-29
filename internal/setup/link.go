package setup

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/claude"
)

// link puts a link named claude, leading to switchboard, in a directory on
// PATH ahead of the real claude, so every claude goes through switchboard:
// the first of the user's that they can write to, once they agree, else the
// next they agree to. Where anything else is named claude, it's left alone,
// and with nowhere to put the link, setup says what's in the way.
func (r *run) link(context.Context) error {
	self, err := os.Stat(r.switchboard)
	if err != nil {
		return fmt.Errorf("find this switchboard binary: %w", err)
	}
	realClaude, err := claude.Find(r.Path, r.InstallPaths, func() (string, error) { return r.switchboard, nil })
	if err != nil {
		r.Terminal.sayf("There's no claude to go ahead of: %v. Install Claude Code, then run setup again.", err)
		return nil
	}
	p := place(r.Path, realClaude, self)
	if p.linked != "" {
		r.Terminal.sayf("claude goes through switchboard already: %s leads to it, ahead of %s.", p.linked, realClaude)
		return nil
	}
	return r.offer(p, realClaude)
}

// offer offers each directory p places the link in, in turn, and links
// claude in the first the user takes. One where something's named claude
// already is passed over, and said to be.
func (r *run) offer(p placement, realClaude string) error {
	where := ""
	if p.onPath {
		where = ", ahead of " + realClaude + " on PATH"
	}
	var declined, taken bool
	for _, dir := range p.dirs {
		link := filepath.Join(dir, claude.Command)
		if _, err := os.Lstat(link); err == nil {
			r.Terminal.sayf("%s is there already, and isn't switchboard, so it's left alone.", link)
			taken = true
			continue
		}
		yes, err := r.Terminal.confirm(fmt.Sprintf("Link %s to switchboard%s, so every claude goes through switchboard?", link, where), true)
		if err != nil {
			return err
		}
		if yes {
			return r.makeLink(link)
		}
		declined = true
	}
	r.Terminal.sayf("%s", notLinked(p, realClaude, declined, taken))
	return nil
}

// makeLink links claude, at link, to switchboard by the path it was run by,
// so an upgrade moves the link on.
func (r *run) makeLink(link string) error {
	if err := os.Symlink(r.switchboard, link); err != nil {
		return fmt.Errorf("link claude to switchboard: %w", err)
	}
	logger.Info("linked claude to switchboard", "link", link, "switchboard", r.switchboard)
	r.Terminal.sayf("Linked %s to %s: every claude goes through switchboard.", link, r.switchboard)
	return nil
}

// notLinked says why the link wasn't put anywhere p places it, ahead of the
// real claude, at realClaude: the user declined each place offered, or it's
// taken, or there was none.
func notLinked(p placement, realClaude string, declined, taken bool) string {
	ahead := ""
	if p.onPath {
		ahead = " ahead of " + filepath.Dir(realClaude)
	}
	switch {
	case declined:
		return "Not linked: claude starts Claude Code without switchboard. Run setup again to link it."
	case taken:
		return fmt.Sprintf("There's nowhere else to put the link: move aside what's named claude, or add another directory of yours to PATH%s, then run setup again.", ahead)
	case p.onPath:
		return fmt.Sprintf("There's nowhere to put the link: no directory of yours on PATH ahead of %s, where claude is, can be written to. Add one to PATH ahead of it, then run setup again.", filepath.Dir(realClaude))
	}
	return fmt.Sprintf("There's nowhere to put the link: claude is at %s, off PATH, and no directory of yours on PATH can be written to. Add one to PATH, then run setup again.", realClaude)
}

// placement is where on PATH the claude link goes.
type placement struct {
	// linked is switchboard's claude link, when there's one ahead of the
	// real claude already.
	linked string
	// dirs are the user's directories ahead of the real claude that they
	// can write to, in PATH's order, each once.
	dirs []string
	// onPath is set when the real claude is on PATH, not found off it.
	onPath bool
}

// place finds where on PATH, as pathList lists it, the claude link goes,
// ahead of the real claude, at realClaude: switchboard is the file self
// describes. Only absolute directories count, as claude.Find counts them.
func place(pathList, realClaude string, self os.FileInfo) placement {
	var p placement
	for _, dir := range filepath.SplitList(pathList) {
		if !filepath.IsAbs(dir) {
			continue
		}
		dir = filepath.Clean(dir)
		if dir == filepath.Dir(realClaude) {
			p.onPath = true
			break
		}
		if named := filepath.Join(dir, claude.Command); isSwitchboard(named, self) {
			return placement{linked: named}
		}
		if writable(dir) && !kept(dir) && !slices.Contains(p.dirs, dir) {
			p.dirs = append(p.dirs, dir)
		}
	}
	return p
}

// keepers are the directories other programs keep what they install in, as
// their own: Homebrew's kegs and casks, which an upgrade replaces, and
// package managers' packages.
var keepers = []string{"Cellar", "Caskroom", "node_modules", "vendor"}

// kept reports whether dir, its links resolved, is another program's rather
// than the user's to put things in: within one of keepers, or an app
// bundle, whose signature a link in it would break, or npm's global bin.
func kept(dir string) bool {
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return false
	}
	for part := range strings.SplitSeq(resolved, string(filepath.Separator)) {
		if slices.Contains(keepers, part) || strings.HasSuffix(part, ".app") {
			return true
		}
	}
	return npmsBin(resolved)
}

// npmsBin reports whether dir is npm's global bin, beside lib/node_modules,
// where npm links what it installs, Claude Code's claude among them when
// it's installed with npm: unless it's Homebrew's own prefix, beside its
// Cellar, whose bin is where links go.
func npmsBin(dir string) bool {
	prefix := filepath.Dir(dir)
	return filepath.Base(dir) == "bin" && exists(filepath.Join(prefix, "lib", "node_modules")) && !exists(filepath.Join(prefix, "Cellar"))
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// isSwitchboard reports whether path leads, links followed, to switchboard,
// the file self describes.
func isSwitchboard(path string, self os.FileInfo) bool {
	info, err := os.Stat(path)
	return err == nil && os.SameFile(info, self)
}

// writable reports whether dir is a directory the user can write to.
func writable(dir string) bool {
	info, err := os.Stat(dir)
	return err == nil && info.IsDir() && canWrite(dir)
}
