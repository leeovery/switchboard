package setup_test

import (
	"reflect"
	"testing"

	"github.com/leeovery/switchboard/internal/setup"
)

func TestARemedyIsWhatTheSituationLeavesToPutRightByHand(t *testing.T) {
	const (
		line   = `if [ -r "$HOME/.local/share/switchboard/shell/init.zsh" ]; then . "$HOME/.local/share/switchboard/shell/init.zsh"; fi  # switchboard`
		dir    = "$HOME/.local/share/switchboard/bin"
		orDir  = "Or, where you keep your PATH another way, put this directory first on it:"
		addIt  = "Add this line to your shell's startup file yourself, after anything else there that changes PATH:"
		moveIt = "Move switchboard's line to the end of ~/.zshrc, or what changes PATH after it to before it."
		after  = "put switchboard's line after what changes PATH, wherever that's read."
	)
	byLine := func(says string) setup.Remedy {
		return setup.Remedy{Parts: []setup.RemedyPart{{Says: says, Put: line}, {Says: orDir, Put: dir}}, Line: line, Dir: dir, Checkable: true}
	}
	moved := func(says string) setup.Remedy {
		return setup.Remedy{Parts: []setup.RemedyPart{{Says: says}, {Says: orDir, Put: dir}}, Dir: dir, Checkable: true}
	}
	tests := []struct {
		name  string
		found setup.ShellFound
		want  setup.Remedy
	}{
		{name: "unreached: nothing", found: setup.ShellFound{Situation: setup.Unreached}},
		{name: "through: nothing", found: setup.ShellFound{Situation: setup.Through}},
		{name: "a shell setup doesn't ask: the directory alone, unchecked", found: setup.ShellFound{Situation: setup.UnknownShell},
			want: setup.Remedy{Parts: []setup.RemedyPart{{Says: "Put this directory first on PATH, wherever your shell sets it:", Put: dir}}, Dir: dir}},
		{name: "an alias in front: asking again alone", found: setup.ShellFound{Situation: setup.InFront}, want: setup.Remedy{Checkable: true}},
		{name: "the line added: asking again alone", found: setup.ShellFound{Situation: setup.LineAdded}, want: setup.Remedy{Checkable: true}},
		{name: "a file setup can't look in: the line added", found: setup.ShellFound{Situation: setup.CantLook}, want: byLine(addIt)},
		{name: "which file: the line added", found: setup.ShellFound{Situation: setup.WhichFile}, want: byLine(addIt)},
		{name: "a rival file: the line added", found: setup.ShellFound{Situation: setup.RivalFile}, want: byLine(addIt)},
		{name: "the line offered, declined: the line added", found: setup.ShellFound{Situation: setup.OfferLine}, want: byLine(addIt)},
		{name: "a line it couldn't add: the line added", found: setup.ShellFound{Situation: setup.CantAdd}, want: byLine(addIt)},
		{name: "another switchboard's line: the line put in its place", found: setup.ShellFound{Situation: setup.OtherLine}, want: byLine("Put this line in its place:")},
		{name: "the line not taking effect: moved, no line to copy", found: setup.ShellFound{Situation: setup.NotTaking}, want: moved(moveIt)},
		{name: "the line last, not taking effect: put after what's read later", found: setup.ShellFound{Situation: setup.NotTaking, Last: true},
			want: moved("P" + after[1:])},
		{name: "the line there, the shell unasked: should it not take effect", found: setup.ShellFound{Situation: setup.NotTaking, Last: true, Unasked: true},
			want: moved("Should it not, something read after ~/.zshrc changes PATH: " + after)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found := tt.found
			found.Line, found.Dir, found.Where = line, dir, "~/.zshrc"
			if got := found.Remedy(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Remedy() = %+v\nwant %+v", got, tt.want)
			}
		})
	}
}
