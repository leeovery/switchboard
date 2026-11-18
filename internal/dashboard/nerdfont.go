package dashboard

// nerdFont says whether the terminal's font draws Nerd Font's glyphs, as the
// config's [dashboard] nerd_font says. Every Nerd Font glyph the dashboard
// draws goes through it, which gives what stands in for one where the font
// hasn't them: such a font draws each as a box.
type nerdFont bool

// worktreeGlyph is Nerd Font's tree, fa-tree.
const worktreeGlyph = ""

// worktree is what marks a worktree's name, ahead of it: the tree and a
// space, where the font has it; else nothing, the name plain.
func (n nerdFont) worktree() string {
	if !n {
		return ""
	}
	return worktreeGlyph + " "
}
