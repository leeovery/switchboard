package dashboard_test

import (
	"go/ast"
	"go/constant"
	"go/token"
	"go/types"
	"maps"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/sourcecheck"
)

// What the width guard works out of the packages it reads, before it judges
// a cutter's calls: which functions cut a name, and where among what they're
// given their widths lie, and which variables hold what they were given
// first, a step a constant width is followed back.

// widthAt is where a cutter's width lies among what it's given: the argument
// at index arg, or a field of it, by the fields' names from it, dotted, as a
// call's columns' "dir" is.
type widthAt struct {
	arg    int
	fields string
}

// path is the fields' names, from the argument.
func (w widthAt) path() []string {
	if w.fields == "" {
		return nil
	}
	return strings.Split(w.fields, ".")
}

// widths are what the guard works out of the files it reads: the variables
// given a value as they're declared and never written after, each with the
// value, a constant width's one step back, a local's or a package-level
// literal's; and every variable written after its declaration, through any
// field, index or pointer of it, or as its address is taken.
type widths struct {
	first   map[*types.Var]ast.Expr
	written map[*types.Var]bool
}

// widthsOf are the widths of the files given.
func widthsOf(files []sourcecheck.File) widths {
	w := widths{first: make(map[*types.Var]ast.Expr), written: make(map[*types.Var]bool)}
	for _, f := range files {
		ast.Inspect(f.AST, func(n ast.Node) bool {
			w.note(n, f.Info)
			return true
		})
	}
	for v := range w.written {
		delete(w.first, v)
	}
	return w
}

// note notes what n declares and what it writes.
func (w widths) note(n ast.Node, info *types.Info) {
	switch n := n.(type) {
	case *ast.ValueSpec:
		w.declared(n.Names, n.Values, info)
	case *ast.AssignStmt:
		w.assigned(n, info)
	case *ast.IncDecStmt:
		w.write(n.X, info)
	case *ast.UnaryExpr:
		if n.Op == token.AND {
			w.write(n.X, info)
		}
	case *ast.RangeStmt:
		if n.Tok == token.ASSIGN {
			w.write(n.Key, info)
			w.write(n.Value, info)
		}
	case *ast.SelectorExpr:
		if s := info.Selections[n]; s != nil && s.Kind() == types.MethodVal && pointerReceiver(s) {
			w.write(n.X, info)
		}
	}
}

// declared notes each name declared with a value of its own.
func (w widths) declared(names []*ast.Ident, values []ast.Expr, info *types.Info) {
	for i, name := range names {
		if v, ok := info.Defs[name].(*types.Var); ok && len(values) == len(names) {
			w.first[v] = values[i]
		}
	}
}

// assigned notes what an assignment declares, and what it writes.
func (w widths) assigned(a *ast.AssignStmt, info *types.Info) {
	if a.Tok != token.DEFINE {
		for _, lhs := range a.Lhs {
			w.write(lhs, info)
		}
		return
	}
	for i, lhs := range a.Lhs {
		id, _ := lhs.(*ast.Ident)
		switch v, defined := info.Defs[id].(*types.Var); {
		case defined && len(a.Rhs) == len(a.Lhs):
			w.first[v] = a.Rhs[i]
		case !defined:
			w.write(lhs, info)
		}
	}
}

// write notes the variable e is of written.
func (w widths) write(e ast.Expr, info *types.Info) {
	if v := rootOf(e, info); v != nil {
		w.written[v] = true
	}
}

// rootOf is the variable e is of, through its fields, indexes and pointers:
// nil where it's of none.
func rootOf(e ast.Expr, info *types.Info) *types.Var {
	switch e := ast.Unparen(e).(type) {
	case *ast.Ident:
		v, _ := info.Uses[e].(*types.Var)
		return v
	case *ast.SelectorExpr:
		return rootOf(e.X, info)
	case *ast.IndexExpr:
		return rootOf(e.X, info)
	case *ast.StarExpr:
		return rootOf(e.X, info)
	}
	return nil
}

// pointerReceiver reports whether the method selected takes its receiver by
// pointer, so calling it on a variable takes its address.
func pointerReceiver(s *types.Selection) bool {
	_, ptr := s.Obj().Type().(*types.Signature).Recv().Type().(*types.Pointer)
	return ptr
}

// valueOf is what e gives at the fields' path, as the widths have it: e
// itself, else a field of a composite literal, through a variable's first
// value once, as laneCall's dir is its literal's 11; nil where it can't be
// followed there.
func (w widths) valueOf(e ast.Expr, fields []string, info *types.Info, followed bool) ast.Expr {
	switch e := ast.Unparen(e).(type) {
	case *ast.SelectorExpr:
		if s := info.Selections[e]; s != nil && s.Kind() == types.FieldVal {
			return w.valueOf(e.X, append([]string{e.Sel.Name}, fields...), info, followed)
		}
	case *ast.Ident:
		if v, ok := info.Uses[e].(*types.Var); ok && !followed && w.first[v] != nil {
			return w.valueOf(w.first[v], fields, info, true)
		}
	case *ast.CompositeLit:
		if len(fields) > 0 {
			if el := elementOf(e, fields[0], info); el != nil {
				return w.valueOf(el, fields[1:], info, followed)
			}
			return nil
		}
	}
	if len(fields) > 0 {
		return nil
	}
	return e
}

// elementOf is the value the composite literal gives its field named: nil
// where it gives none.
func elementOf(lit *ast.CompositeLit, field string, info *types.Info) ast.Expr {
	_, s := sourcecheck.StructOf(info.Types[lit].Type)
	if s == nil {
		return nil
	}
	for i, el := range lit.Elts {
		kv, keyed := el.(*ast.KeyValueExpr)
		switch {
		case keyed:
			if key, ok := kv.Key.(*ast.Ident); ok && key.Name == field {
				return kv.Value
			}
		case i < s.NumFields() && s.Field(i).Name() == field:
			return el
		}
	}
	return nil
}

// constantOf is the number constant e gives, written as one or followed a
// step back, as valueOf follows it: nil where it gives none.
func (w widths) constantOf(e ast.Expr, info *types.Info) constant.Value {
	value := w.valueOf(e, nil, info, false)
	if value == nil {
		return nil
	}
	switch c := info.Types[value].Value; {
	case c == nil, c.Kind() != constant.Int && c.Kind() != constant.Float:
		return nil
	default:
		return c
	}
}

// constantWidth breaks a call of a cutter whose widths lie where at says,
// where one is a constant other than whole, as Breaks judges a call: written
// as one, or followed a step back, as widths' valueOf follows it. One known
// only at run time isn't judged; nor is one whose type the loader lost,
// where it names a variable, so it's no constant; any other whose type is
// lost can't be. sourcecheck.ConstantOtherThan judges strings alone.
func (w widths) constantWidth(at []widthAt) func(*ast.CallExpr, *types.Info) (bool, ast.Expr) {
	return func(call *ast.CallExpr, info *types.Info) (bool, ast.Expr) {
		var lost ast.Expr
		for _, a := range at {
			if a.arg >= len(call.Args) {
				continue
			}
			width := w.valueOf(call.Args[a.arg], a.path(), info, false)
			if width == nil {
				continue
			}
			switch tv, seen := info.Types[width]; {
			case seen && tv.Type.Underlying() != types.Typ[types.Invalid]:
				if tv.Value != nil && !constant.Compare(tv.Value, token.EQL, whole) {
					return true, nil
				}
			case !namesAVariable(width, info):
				lost = width
			}
		}
		return false, lost
	}
}

// namesAVariable reports whether e names a variable, so it's no constant, as
// the Go spec has a constant expression: but within len, cap or unsafe's,
// which may make a constant of an array.
func namesAVariable(e ast.Expr, info *types.Info) bool {
	found := false
	ast.Inspect(e, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok && measures(call, info) {
			return false
		}
		if id, ok := n.(*ast.Ident); ok {
			_, isVar := info.Uses[id].(*types.Var)
			found = found || isVar
		}
		return !found
	})
	return found
}

// measures reports whether the call is of len, cap or unsafe's, which may
// make a constant of what it's given.
func measures(call *ast.CallExpr, info *types.Info) bool {
	switch fn := ast.Unparen(call.Fun).(type) {
	case *ast.Ident:
		b, ok := info.Uses[fn].(*types.Builtin)
		return ok && (b.Name() == "len" || b.Name() == "cap")
	case *ast.SelectorExpr:
		_, ok := info.Uses[fn.Sel].(*types.Builtin)
		return ok
	}
	return false
}

// wrapped are the cutters given, by their full names, and every function and
// method of the files given that hands a width it's given on to one of them
// unchanged, a parameter it never writes or a field of one, as the place
// that width lies among what it's given, worked out till none is added.
func (w widths) wrapped(files []sourcecheck.File, cutters map[string][]widthAt) map[string][]widthAt {
	all := maps.Clone(cutters)
	for added := true; added; {
		added = false
		for _, f := range files {
			for _, decl := range f.AST.Decls {
				if fd, ok := decl.(*ast.FuncDecl); ok && fd.Body != nil {
					added = w.wraps(fd, f.Info, all) || added
				}
			}
		}
	}
	return all
}

// wraps adds fd to cutters where it hands a width it's given on to one of
// them, reporting whether it added a width.
func (w widths) wraps(fd *ast.FuncDecl, info *types.Info, cutters map[string][]widthAt) bool {
	params := paramsOf(fd, info)
	name := info.Defs[fd.Name].(*types.Func).FullName()
	added := false
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		callee, args := sourcecheck.Callee(call, info)
		for _, at := range cutters[callee] {
			if at.arg >= len(args) {
				continue
			}
			p, fields := paramPath(args[at.arg], info)
			i := slices.Index(params, p)
			if p == nil || i < 0 || w.written[p] {
				continue
			}
			path := append(fields, at.path()...)
			if recurs(p.Type(), path) {
				continue
			}
			mine := widthAt{arg: i, fields: strings.Join(path, ".")}
			if !slices.Contains(cutters[name], mine) {
				cutters[name] = append(cutters[name], mine)
				added = true
			}
		}
		return true
	})
	return added
}

// recurs reports whether the fields' path from a value of type t passes
// through a struct it has passed through already, as a list's next.w does:
// a wrapper handing on such a path, as one walking the list, would have the
// fixpoint follow it without end, next.next.w and on, so it's refused.
func recurs(t types.Type, path []string) bool {
	seen := make(map[*types.Struct]bool)
	for _, name := range path {
		_, s := sourcecheck.StructOf(t)
		if s == nil {
			return false
		}
		if seen[s] {
			return true
		}
		seen[s] = true
		field := fieldNamed(s, name)
		if field == nil {
			return false
		}
		t = field.Type()
	}
	return false
}

// fieldNamed is the struct's field named: nil where it has none.
func fieldNamed(s *types.Struct, name string) *types.Var {
	for f := range s.Fields() {
		if f.Name() == name {
			return f
		}
	}
	return nil
}

// paramsOf are fd's parameters, in order, but its receiver.
func paramsOf(fd *ast.FuncDecl, info *types.Info) []*types.Var {
	var params []*types.Var
	for _, field := range fd.Type.Params.List {
		for _, name := range field.Names {
			v, _ := info.Defs[name].(*types.Var)
			params = append(params, v)
		}
		if len(field.Names) == 0 {
			params = append(params, nil)
		}
	}
	return params
}

// paramPath is the variable e reads and the fields of it it reads, by name,
// as k.dir reads k's dir: nil where e reads anything else.
func paramPath(e ast.Expr, info *types.Info) (*types.Var, []string) {
	switch e := ast.Unparen(e).(type) {
	case *ast.Ident:
		v, _ := info.Uses[e].(*types.Var)
		return v, nil
	case *ast.SelectorExpr:
		if s := info.Selections[e]; s != nil && s.Kind() == types.FieldVal {
			v, fields := paramPath(e.X, info)
			return v, append(fields, e.Sel.Name)
		}
	}
	return nil, nil
}
