package setup

import (
	"cmp"
	"testing"
)

func TestTheClaudeLinksLineInEverySituation(t *testing.T) {
	const (
		line = "<line>"
		dir  = "$HOME/.local/share/switchboard/bin"
		// front is what an alias in front runs, and how to put it right, as
		// the step words it.
		front = "zsh's claude is alias claude=~/bin/claude, which runs ~/bin/claude, not switchboard's claude: removing the alias, or pointing it at claude, puts it right"
		// repaired is the line's start, where the step repaired a link that
		// led to ~/go/bin/switchboard.
		repaired = "repaired: it led to ~/go/bin/switchboard"
	)
	made := []Said{{Mark: Done, Label: "Link", Detail: "~/.local/share/switchboard/bin/claude → switchboard"}}
	notMade := []Said{{Mark: Warning, Label: "Link", Detail: "not made: ~/.local/share/switchboard/bin/claude is there already, and isn't a link, so it's left alone: move it aside, then run setup again"}}
	tests := []struct {
		situation Situation
		// made is what the step did to the link: created, where it's not
		// set, but where said is set, the link not made.
		made      LinkMade
		unasked   bool
		last      bool
		front     string
		said      []Said
		mark      Mark
		want, add string
		directory bool
	}{
		{situation: Through, mark: Done, want: "created · your shell runs it"},
		{situation: Through, made: LinkInPlace, mark: Done, want: "your shell runs it"},
		{situation: Through, made: LinkRepaired, mark: Done, want: repaired + " · your shell runs the claude link"},
		{situation: LineAdded, mark: Warning, want: "created, and switchboard's line is added to ~/.zshrc, but setup hasn't asked your shell again whether a new terminal's claude goes through it"},
		{situation: LineAdded, made: LinkInPlace, mark: Warning, want: "switchboard's line is added to ~/.zshrc, but setup hasn't asked your shell again whether a new terminal's claude goes through it"},
		{situation: LineAdded, front: front, mark: Warning, want: "created, and switchboard's line is added to ~/.zshrc, but " + front + "; and setup hasn't asked your shell again whether a new terminal's claude goes through the claude link"},
		{situation: LineAdded, made: LinkInPlace, front: front, mark: Warning, want: "switchboard's line is added to ~/.zshrc, but " + front + "; and setup hasn't asked your shell again whether a new terminal's claude goes through the claude link"},
		{situation: Unreached, said: notMade, mark: Warning, want: notMade[0].Detail},
		{situation: Unreached, said: []Said{}, mark: Warning, want: "not made"},
		{situation: Unreached, mark: Warning, want: "created, but there's no Claude Code for it to start: install Claude Code, then run setup again"},
		{situation: Unreached, made: LinkInPlace, mark: Warning, want: "there's no Claude Code for it to start: install Claude Code, then run setup again"},
		{situation: OfferLine, mark: Warning, want: "created, but it isn't first on your PATH. Add this line to ~/.zshrc:", add: line},
		{situation: OfferLine, made: LinkInPlace, mark: Warning, want: "it isn't first on your PATH. Add this line to ~/.zshrc:", add: line},
		{situation: OfferLine, made: LinkRepaired, mark: Warning, want: repaired + ", but the claude link isn't first on your PATH. Add this line to ~/.zshrc:", add: line},
		{situation: OfferLine, front: front, mark: Warning, want: "created, but " + front + "; and the claude link isn't first on your PATH. Add this line to ~/.zshrc:", add: line},
		{situation: OfferLine, made: LinkInPlace, front: front, mark: Warning, want: front + "; and the claude link isn't first on your PATH. Add this line to ~/.zshrc:", add: line},
		{situation: CantAdd, mark: Warning, want: "created, but it isn't first on your PATH, and setup couldn't add its line to ~/.zshrc. Add this line to it yourself:", add: line},
		{situation: CantAdd, front: front, mark: Warning, want: "created, but " + front + "; and the claude link isn't first on your PATH, and setup couldn't add its line to ~/.zshrc. Add this line to it yourself:", add: line},
		{situation: WhichFile, unasked: true, mark: Warning, want: "created, but setup couldn't ask your shell whether it is first on your PATH. Add this line to your shell's startup file, usually ~/.zshrc:", add: line},
		{situation: WhichFile, made: LinkInPlace, unasked: true, mark: Warning, want: "setup couldn't ask your shell whether it is first on your PATH. Add this line to your shell's startup file, usually ~/.zshrc:", add: line},
		{situation: RivalFile, mark: Warning, want: "created, but it isn't first on your PATH. Add this line to the end of whichever of ~/.zshrc and ~/.config/zsh/.zshrc a new terminal reads:", add: line},
		{situation: CantLook, mark: Warning, want: "created, but it isn't first on your PATH, and setup can't look in ~/.zshrc for its line. Add this line to the end of it, if it isn't there:", add: line},
		{situation: CantLook, unasked: true, mark: Warning, want: "created, but setup couldn't ask your shell whether it is first on your PATH, and setup can't look in ~/.zshrc for its line. Add this line to the end of it, if it isn't there:", add: line},
		{situation: OtherLine, mark: Warning, want: "created, but switchboard's line in ~/.zshrc reads another switchboard's file. Put this line in its place:", add: line},
		{situation: NotTaking, mark: Warning, want: "created, but something after switchboard's line in ~/.zshrc changes PATH. Move the line to the end of ~/.zshrc, or what changes PATH after it to before it"},
		{situation: NotTaking, last: true, mark: Warning, want: "created, but something read after ~/.zshrc changes PATH. Put switchboard's line after it, wherever that's read"},
		{situation: NotTaking, unasked: true, mark: Warning, want: "created, but setup couldn't ask your shell whether switchboard's line in ~/.zshrc takes effect"},
		{situation: NotTaking, made: LinkRepaired, mark: Warning, want: repaired + ", but something after switchboard's line in ~/.zshrc changes PATH. Move the line to the end of ~/.zshrc, or what changes PATH after it to before it"},
		{situation: InFront, front: front, mark: Warning, want: "created, but " + front},
		{situation: InFront, made: LinkInPlace, front: front, mark: Warning, want: front},
		{situation: InFront, made: LinkRepaired, front: front, mark: Warning, want: repaired + ", but " + front},
		{situation: UnknownShell, unasked: true, mark: Warning, want: "created, but setup can't check it for your shell. Put this directory first on your PATH:", add: dir, directory: true},
		{situation: UnknownShell, made: LinkInPlace, unasked: true, mark: Warning, want: "setup can't check it for your shell. Put this directory first on your PATH:", add: dir, directory: true},
	}
	for _, tt := range tests {
		said, did := tt.said, cmp.Or(tt.made, LinkCreated)
		if said != nil {
			did = LinkNotMade
		} else {
			said = made
		}
		found := LinkFound{Said: said, Made: did, Shell: ShellFound{
			Situation: tt.situation, Said: Said{Mark: Warning, Label: "Your shell", Detail: "the shell's own row"},
			Line: line, Where: "~/.zshrc", Rival: "~/.config/zsh/.zshrc", Front: tt.front, Unasked: tt.unasked, Dir: dir, Last: tt.last,
		}}
		if did == LinkRepaired {
			found.Led = "~/go/bin/switchboard"
		}
		got := found.line()
		if got.Mark != tt.mark || got.Detail != tt.want || got.Add != tt.add || got.Directory != tt.directory {
			t.Errorf("in situation %v, the link %v, unasked %v, an alias in front %v, the line is %+v\nwant %v saying %q, adding %q, a directory %v",
				tt.situation, did, tt.unasked, tt.front != "", got, tt.mark, tt.want, tt.add, tt.directory)
		}
	}
}
