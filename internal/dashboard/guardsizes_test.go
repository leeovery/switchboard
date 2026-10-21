package dashboard_test

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/sourcecheck"
)

// sizeWords are the words a size is called by, a width or a height, as a
// variable, a field or a function giving one. namesASize holds a name to
// them: the word alone, in either case, as width or Width, or the last word
// of a name in camel case, as innerWidth or viewRows. A size held under any
// other name, as cells or phoneUnder, passes unseen.
var sizeWords = []string{"Width", "Height", "Rows", "Cols", "Room"}

func namesASize(name string) bool {
	return slices.ContainsFunc(sizeWords, func(word string) bool {
		return strings.EqualFold(name, word) || strings.HasSuffix(name, word)
	})
}

// comparisons are the operators a size is compared with a constant by.
var comparisons = []token.Token{token.LSS, token.LEQ, token.GTR, token.GEQ, token.EQL, token.NEQ}

// flipped is each ordered comparison with its sides swapped, as 80 > room is
// room < 80.
var flipped = map[token.Token]token.Token{token.LSS: token.GTR, token.LEQ: token.GEQ, token.GTR: token.LSS, token.GEQ: token.LEQ}

// sizes judge what's compared with a size in the drawing packages, by the
// widths worked out of them, and the packages' paths, whose functions giving
// a size read one.
type sizes struct {
	widths
	drawing map[string]bool
}

// sizesOf are the sizes of the files given, by their widths w.
func sizesOf(files []sourcecheck.File, w widths) sizes {
	s := sizes{widths: w, drawing: make(map[string]bool)}
	for _, f := range files {
		s.drawing[f.Pkg.Path()] = true
	}
	return s
}

// sized says how n compares a width or a height with a constant: a
// comparison, ordered or not, or a switch on a size, each case by equality,
// as compared has it.
func (s sizes) sized(n ast.Node, info *types.Info) string {
	switch n := n.(type) {
	case *ast.BinaryExpr:
		if slices.Contains(comparisons, n.Op) {
			return s.compared(n, info)
		}
	case *ast.SwitchStmt:
		if n.Tag != nil {
			return s.switched(n, info)
		}
	}
	return ""
}

// compared says how b compares a size with a constant, either side, as
// against has it.
func (s sizes) compared(b *ast.BinaryExpr, info *types.Info) string {
	for i, sides := range [][2]ast.Expr{{b.X, b.Y}, {b.Y, b.X}} {
		if says := s.against(sides[0], sides[1], b.Op, i == 0, types.ExprString(b), info); says != "" {
			return says
		}
	}
	return ""
}

// switched says how sw's first case that breaks the rule compares its size
// with a constant, as against has it: "" where none does.
func (s sizes) switched(sw *ast.SwitchStmt, info *types.Info) string {
	for _, stmt := range sw.Body.List {
		for _, c := range stmt.(*ast.CaseClause).List {
			compared := "switch " + types.ExprString(sw.Tag) + " case " + types.ExprString(c)
			if says := s.against(c, sw.Tag, token.EQL, false, compared, info); says != "" {
				return says
			}
		}
	}
	return ""
}

// against says how comparing size with c by op, c first where first says,
// breaks the rule, what's compared named as given: where size reads a size,
// as readsASize has it, and c is a number constant, written as one or
// followed a step back as widths' valueOf follows it, as f.Width <
// phoneUnder, width < limit or bodyRows == 3; but 0, as whether there's any,
// or, ordered, 1 as 0, as room < 1 is room <= 0. Where c names no variable
// and its type is lost, it can't be judged.
func (s sizes) against(c, size ast.Expr, op token.Token, first bool, compared string, info *types.Info) string {
	if !s.readsASize(size, info) {
		return ""
	}
	switch v := s.constantOf(c, info); {
	case v != nil && withNone(v, op, first):
		return ""
	case v != nil:
		return sizedSays(compared)
	case typeLost(c, info) && !namesAVariable(c, info):
		return sourcecheck.LostSays(compared, "the type of "+types.ExprString(c))
	}
	return ""
}

// withNone reports whether comparing a whole number with the number constant
// c, by op, the constant first where first says, asks whether there's any: c
// is 0, or 1 where the comparison is one with 0, as n < 1 or 1 <= n.
func withNone(c constant.Value, op token.Token, first bool) bool {
	if constant.Sign(c) == 0 {
		return true
	}
	if swapped, ordered := flipped[op]; first && ordered {
		op = swapped
	}
	return constant.Compare(c, token.EQL, constant.MakeInt64(1)) && (op == token.LSS || op == token.GEQ)
}

// readsASize reports whether e reads a size, as namesASize names one: a
// variable or a field of a whole number, or of a type the loader lost; a
// field of what the loader lost, by its name alone; or a call of a function
// or method of the drawing packages giving a whole number, as d.width() or
// f.viewRows(). It reads through a conversion, as int(width), but not into
// a call's arguments, as len(f.rowsOf(width)) counts what the call gives;
// and a measure from outside, as ansi.StringWidth, measures a string, not
// the frame.
func (s sizes) readsASize(e ast.Expr, info *types.Info) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.CallExpr:
			if info.Types[n.Fun].IsType() {
				return true
			}
			found = found || s.givesASize(n, info)
			return false
		case *ast.SelectorExpr:
			if info.Selections[n] == nil && info.Uses[n.Sel] == nil {
				found = found || namesASize(n.Sel.Name)
				return false
			}
		case *ast.Ident:
			v, isVar := info.Uses[n].(*types.Var)
			found = found || isVar && namesASize(n.Name) && (isWhole(v.Type()) || isLost(v.Type()))
		}
		return !found
	})
	return found
}

// givesASize reports whether call calls a function or method of the drawing
// packages a size word names, giving a whole number.
func (s sizes) givesASize(call *ast.CallExpr, info *types.Info) bool {
	var id *ast.Ident
	switch fn := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		id = fn
	case *ast.SelectorExpr:
		id = fn.Sel
	}
	fn, ok := info.Uses[id].(*types.Func)
	return ok && fn.Pkg() != nil && s.drawing[fn.Pkg().Path()] && namesASize(fn.Name()) && isWhole(info.Types[call].Type)
}

// typeLost reports whether the loader lost e's type to a package it stubs.
func typeLost(e ast.Expr, info *types.Info) bool {
	tv, ok := info.Types[e]
	return !ok || isLost(tv.Type)
}

// isLost reports whether t is lost to a package the loader stubs.
func isLost(t types.Type) bool {
	return t.Underlying() == types.Typ[types.Invalid]
}

// isWhole reports whether t is a whole number's type.
func isWhole(t types.Type) bool {
	basic, ok := t.Underlying().(*types.Basic)
	return ok && basic.Info()&types.IsInteger != 0
}
