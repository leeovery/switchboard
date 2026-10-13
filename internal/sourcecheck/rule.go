package sourcecheck

import (
	"cmp"
	"go/ast"
	"go/token"
	"go/types"
	"iter"
	"path"
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
	// with how using it breaks the rule, and where.
	Used map[string]Ban
	// Calls are the functions whose calls may break the rule, by what they're
	// given, each with how and where, by its full name, as
	// (*types.Func).FullName has it, the standard library's included, as
	// "(time.Time).Format" or "fmt.Sprintf": of a package the loader stubs,
	// by its path and the function's name, as "example.org/cells.Cut".
	Calls map[string]CallBan
	// Owners are the packages, as "status", whose functions and methods that
	// take one of Fields, as their receiver or a parameter, may be called
	// only where Allowed names them; Takes says how calling one, as
	// "status.Sorted", taking the type named, breaks the rule.
	Owners []string
	Takes  func(fn, field string) string
	// Allowed are the calls allowed all the same.
	Allowed []string
}

// Ban is how using a function breaks a rule, and where it does: in the
// packages In names, by directory from the module's root, or, where it names
// none, in every package.
type Ban struct {
	Says string
	In   []string
}

// holdsIn reports whether the ban holds in the package in dir.
func (b Ban) holdsIn(dir string) bool {
	return len(b.In) == 0 || slices.Contains(b.In, dir)
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
	dir := path.Dir(f.Path)
	for _, decl := range f.AST.Decls {
		w := &walk{r: r, info: f.Info, dir: dir, in: DeclName(decl), judged: make(map[*ast.Ident]bool)}
		ast.Inspect(decl, w.visit)
		slices.SortStableFunc(w.found, func(x, y Breach) int { return cmp.Compare(x.At, y.At) })
		found = append(found, w.found...)
		allowed = append(allowed, w.allowed...)
	}
	return found, allowed
}

// walk is a walk of one of a file's declarations, named in, of the package
// in dir, for where it breaks the rule r and the calls it makes that r
// allows.
type walk struct {
	r       Rule
	info    *types.Info
	dir, in string
	found   []Breach
	allowed []Use
	// judged are the identifiers naming a function of Calls judged already:
	// a call's callee, by its arguments, or one used as a value.
	judged map[*ast.Ident]bool
}

// visit finds where n breaks the rule, and walks on into it.
func (w *walk) visit(n ast.Node) bool {
	switch n := n.(type) {
	case *ast.CallExpr:
		w.call(n)
	case *ast.SelectorExpr:
		w.field(n)
		w.value(n)
	case *ast.Ident:
		w.used(n)
		w.value(n)
	}
	return true
}

func (w *walk) breach(at token.Pos, says string) {
	w.found = append(w.found, Breach{At: at, In: w.in, Says: says})
}

// field finds sel reading a field of one of Fields.
func (w *walk) field(sel *ast.SelectorExpr) {
	if s, ok := w.info.Selections[sel]; ok && s.Kind() == types.FieldVal {
		if field := w.r.fieldRead(s); field != "" {
			w.breach(sel.Sel.Pos(), w.r.Read(field))
		}
	}
}

// used finds id using a function of Used, or one of Owners' that takes one
// of Fields, or one the rule allows.
func (w *walk) used(id *ast.Ident) {
	fn, ok := w.info.Uses[id].(*types.Func)
	if !ok {
		return
	}
	switch name, says := w.r.called(fn, w.dir); {
	case says != "":
		w.breach(id.Pos(), says)
	case slices.Contains(w.r.Allowed, name):
		w.allowed = append(w.allowed, Use{At: id.Pos(), In: w.in, Name: name})
	}
}

// call finds the call of a function of Calls that breaks the rule here, by
// its arguments.
func (w *walk) call(call *ast.CallExpr) {
	id, ban, ok := w.callBan(call.Fun)
	if !ok {
		return
	}
	w.judged[id] = true
	if ban.Breaks == nil || ban.Breaks(argued(call, w.info), w.info) {
		w.breach(id.Pos(), ban.Says)
	}
}

// value finds e naming a function of Calls banned here as a value, not the
// callee of a call: what it's given where it's called can't be judged, so it
// breaks the rule.
func (w *walk) value(e ast.Expr) {
	id, ban, ok := w.callBan(e)
	if !ok || w.judged[id] {
		return
	}
	w.judged[id] = true
	w.breach(id.Pos(), ban.Says)
}

// callBan is the identifier naming the function e names, and its ban, where
// it's one of Calls, banned here.
func (w *walk) callBan(e ast.Expr) (*ast.Ident, CallBan, bool) {
	name, id := funcNamed(e, w.info)
	ban, ok := w.r.Calls[name]
	return id, ban, ok && ban.holdsIn(w.dir)
}

// called names the function fn, as Names.Func has it, and says how using it
// in the package in dir breaks the rule, "" where it doesn't: it's one of
// Used, banned there; or it's of Owners and takes one of Fields, and the rule
// doesn't allow it.
func (r Rule) called(fn *types.Func, dir string) (name, says string) {
	name = r.Func(fn)
	pkg, _, _ := strings.Cut(name, ".")
	switch ban, ok := r.Used[name]; {
	case ok && ban.holdsIn(dir):
		return name, ban.Says
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
	if recv := sig.Recv(); recv != nil {
		if held := r.held(recv.Type()); held != "" {
			return held
		}
	}
	return r.heldIn(typesOf(sig.Params()))
}

// held names the one of Fields t is, or holds through pointers, slices,
// arrays, maps, channels, a func's parameters and results, and a generic
// named type's type arguments, as "status.Account": "" where it's none. It
// looks into no named type but by its type arguments, so a recursive type
// ends it as any other does.
func (r Rule) held(t types.Type) string {
	switch t := types.Unalias(t).(type) {
	case *types.Pointer:
		return r.held(t.Elem())
	case *types.Slice:
		return r.held(t.Elem())
	case *types.Array:
		return r.held(t.Elem())
	case *types.Chan:
		return r.held(t.Elem())
	case *types.Map:
		return cmp.Or(r.held(t.Key()), r.held(t.Elem()))
	case *types.Signature:
		return cmp.Or(r.heldIn(typesOf(t.Params())), r.heldIn(typesOf(t.Results())))
	case *types.Named:
		if name := r.Type(t); slices.Contains(r.Fields, name) {
			return name
		}
		return r.heldIn(t.TypeArgs().Types())
	}
	return ""
}

// heldIn names the first of Fields one of ts holds, as held finds it: ""
// where none does.
func (r Rule) heldIn(ts iter.Seq[types.Type]) string {
	for t := range ts {
		if held := r.held(t); held != "" {
			return held
		}
	}
	return ""
}

// typesOf are the types of the tuple's variables, in order.
func typesOf(tuple *types.Tuple) iter.Seq[types.Type] {
	return func(yield func(types.Type) bool) {
		for v := range tuple.Variables() {
			if !yield(v.Type()) {
				return
			}
		}
	}
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
