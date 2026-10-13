package sourcecheck

import (
	"cmp"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"
)

// Names names what the module's internal packages declare, by the package's
// path under the module's internal directory, as "status" or
// "dashboard/watch".
type Names struct {
	// internal is the import path of the module's internal directory, which
	// the names go by.
	internal string
}

// NamesIn names what the internal packages of the module with the given path
// declare.
func NamesIn(module string) Names {
	return Names{internal: module + "/internal/"}
}

// Func is the function's name, by its package's, then, of a method, its
// receiver's type's, as "status.Document.RoutingOf": "" of one of no package
// under the internal directory.
func (n Names) Func(fn *types.Func) string {
	if fn.Pkg() == nil || !strings.HasPrefix(fn.Pkg().Path(), n.internal) {
		return ""
	}
	name := strings.TrimPrefix(fn.Pkg().Path(), n.internal) + "."
	if recv := fn.Signature().Recv(); recv != nil {
		named, _ := StructOf(recv.Type())
		if named == nil {
			return ""
		}
		name += named.Obj().Name() + "."
	}
	return name + fn.Name()
}

// Type is the named type's name, by its package's, as "status.Account": ""
// of a type of no package under the internal directory.
func (n Names) Type(named *types.Named) string {
	if named == nil {
		return ""
	}
	obj := named.Obj()
	if obj.Pkg() == nil || !strings.HasPrefix(obj.Pkg().Path(), n.internal) {
		return ""
	}
	return strings.TrimPrefix(obj.Pkg().Path(), n.internal) + "." + obj.Name()
}

// StructOf is the struct t is, or points to, and the type it's named by,
// where it is: nil where it's no struct.
func StructOf(t types.Type) (*types.Named, *types.Struct) {
	if p, ok := types.Unalias(t).(*types.Pointer); ok {
		t = p.Elem()
	}
	named, _ := types.Unalias(t).(*types.Named)
	s, _ := t.Underlying().(*types.Struct)
	return named, s
}

// Rule is what breaks a guard's rule, each named as Names names it.
type Rule struct {
	Names
	// Fields are the types, as "status.Account", none of whose fields may be
	// read, through any field they're embedded in; Read says how a read of
	// the field named, as "status.Account.ID", breaks the rule.
	Fields []string
	Read   func(field string) string
	// Used are the functions none of which may be used, called or not, each
	// with how using it breaks the rule.
	Used map[string]string
	// Owners are the packages, as "status", whose functions and methods that
	// take one of Fields, as their receiver or a parameter, may be called
	// only where Allowed names them; Takes says how calling one, as
	// "status.Sorted", taking the type named, breaks the rule.
	Owners []string
	Takes  func(fn, field string) string
	// Allowed are the calls allowed all the same.
	Allowed []string
}

// Breach is where a file breaks a rule: in which of its declarations, as
// DeclName names it, and how.
type Breach struct {
	At   token.Pos
	In   string
	Says string
}

// Use is where a file uses a function, by its name, as Names.Func has it: in
// which of its declarations, as DeclName names it.
type Use struct {
	At   token.Pos
	In   string
	Name string
}

// Breaches are where the type-checked file f breaks the rule, in the order
// they come in it, and the calls it makes that the rule allows.
func (r Rule) Breaches(f File) (found []Breach, allowed []Use) {
	for _, decl := range f.AST.Decls {
		in := DeclName(decl)
		var inDecl []Breach
		ast.Inspect(decl, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.SelectorExpr:
				if sel, ok := f.Info.Selections[n]; ok && sel.Kind() == types.FieldVal {
					if field := r.fieldRead(sel); field != "" {
						inDecl = append(inDecl, Breach{At: n.Sel.Pos(), In: in, Says: r.Read(field)})
					}
				}
			case *ast.Ident:
				fn, ok := f.Info.Uses[n].(*types.Func)
				if !ok {
					break
				}
				switch name, says := r.called(fn); {
				case says != "":
					inDecl = append(inDecl, Breach{At: n.Pos(), In: in, Says: says})
				case slices.Contains(r.Allowed, name):
					allowed = append(allowed, Use{At: n.Pos(), In: in, Name: name})
				}
			}
			return true
		})
		slices.SortStableFunc(inDecl, func(x, y Breach) int { return cmp.Compare(x.At, y.At) })
		found = append(found, inDecl...)
	}
	return found, allowed
}

// called names the function fn, as Names.Func has it, and says how using it
// breaks the rule, "" where it doesn't: it's one of Used; or it's of Owners
// and takes one of Fields, and the rule doesn't allow it.
func (r Rule) called(fn *types.Func) (name, says string) {
	name = r.Func(fn)
	pkg, _, _ := strings.Cut(name, ".")
	switch {
	case r.Used[name] != "":
		return name, r.Used[name]
	case !slices.Contains(r.Owners, pkg) || slices.Contains(r.Allowed, name):
		return name, ""
	}
	if field := r.takes(fn); field != "" {
		return name, r.Takes(name, field)
	}
	return name, ""
}

// takes names the first of Fields the function fn takes, as its receiver or
// a parameter, as held finds it: "" where it takes none.
func (r Rule) takes(fn *types.Func) string {
	sig := fn.Signature()
	var taken []types.Type
	if recv := sig.Recv(); recv != nil {
		taken = append(taken, recv.Type())
	}
	for v := range sig.Params().Variables() {
		taken = append(taken, v.Type())
	}
	for _, t := range taken {
		if held := r.held(t); held != "" {
			return held
		}
	}
	return ""
}

// held names the one of Fields t is, or holds through pointers, slices,
// arrays and maps, as "status.Account": "" where it's none.
func (r Rule) held(t types.Type) string {
	switch t := types.Unalias(t).(type) {
	case *types.Pointer:
		return r.held(t.Elem())
	case *types.Slice:
		return r.held(t.Elem())
	case *types.Array:
		return r.held(t.Elem())
	case *types.Map:
		return cmp.Or(r.held(t.Key()), r.held(t.Elem()))
	case *types.Named:
		if name := r.Type(t); slices.Contains(r.Fields, name) {
			return name
		}
	}
	return ""
}

// fieldRead names the field of one of Fields sel reads, as
// "status.Account.ID": the field it selects, of the first of Fields the
// selection passes through, from the type it selects from through each field
// embedded on the way; "" where it passes through none.
func (r Rule) fieldRead(sel *types.Selection) string {
	t := sel.Recv()
	for _, index := range sel.Index() {
		named, s := StructOf(t)
		if s == nil {
			return ""
		}
		if name := r.Type(named); slices.Contains(r.Fields, name) {
			return name + "." + sel.Obj().Name()
		}
		t = s.Field(index).Type()
	}
	return ""
}

// DeclName names the function decl declares, by its name, and of a method,
// its receiver's type's name first, as "eventsText.row": "" of any other
// declaration.
func DeclName(decl ast.Decl) string {
	fd, ok := decl.(*ast.FuncDecl)
	if !ok {
		return ""
	}
	if fd.Recv == nil || len(fd.Recv.List) == 0 {
		return fd.Name.Name
	}
	return receiverName(fd.Recv.List[0].Type) + "." + fd.Name.Name
}

// receiverName is the name of a method's receiver's type, as its declaration
// writes it, less a pointer and type parameters.
func receiverName(e ast.Expr) string {
	switch e := e.(type) {
	case *ast.StarExpr:
		return receiverName(e.X)
	case *ast.IndexExpr:
		return receiverName(e.X)
	case *ast.IndexListExpr:
		return receiverName(e.X)
	case *ast.ParenExpr:
		return receiverName(e.X)
	case *ast.Ident:
		return e.Name
	}
	return ""
}
