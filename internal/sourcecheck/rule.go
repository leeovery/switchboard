package sourcecheck

import (
	"cmp"
	"go/ast"
	"go/token"
	"go/types"
	"iter"
	"maps"
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

// Declared is the name of obj, declared at its package's level, by its
// package's, as "status.Account": "" of one of no package under the
// internal directory, or declared within a function or a type.
func (n Names) Declared(obj types.Object) string {
	pkg := obj.Pkg()
	if pkg == nil || obj.Parent() != pkg.Scope() || !strings.HasPrefix(pkg.Path(), n.internal) {
		return ""
	}
	return strings.TrimPrefix(pkg.Path(), n.internal) + "." + obj.Name()
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
	// Used are the functions none of which may be used, called or not; the
	// types, constants and package-level variables, as "status.Account",
	// none of which may be named; and the fields, as "status.Account.ID",
	// none of which may be read, each with how using it breaks the rule, and
	// where. A type of Fields or Used is named, and its fields read, through
	// an alias of it, and through a type defined as it, as
	// "type Over status.Account" is, wherever they're declared.
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
	// Node says how a node of a file's declarations breaks the rule, by what
	// the type checker knows of it, as a comparison by what it compares: ""
	// where it doesn't. Nil judges none.
	Node func(n ast.Node, info *types.Info) string
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
// they come in it, and the calls it makes that the rule allows. What the
// rule would judge but can't, as the type checker lost its type to a
// package the loader stubs, breaks it too.
func (r Rule) Breaches(f File) (found []Breach, allowed []Use) {
	dir := path.Dir(f.Path)
	unseen := r.unseenIn(f.Pkg, dir)
	banned := r.structsBanned(reachable(f.Pkg))
	for _, decl := range f.AST.Decls {
		w := &walk{r: r, info: f.Info, dir: dir, in: DeclName(decl), banned: banned, judged: make(map[*ast.Ident]bool)}
		ast.Inspect(decl, w.visit)
		for _, lost := range f.LostIn(decl) {
			if unseen.bans(lost) {
				w.breach(lost.Name.Pos(), lost.Says())
			}
		}
		slices.SortStableFunc(w.found, func(x, y Breach) int { return cmp.Compare(x.At, y.At) })
		found = append(found, w.found...)
		allowed = append(allowed, w.allowed...)
	}
	return found, allowed
}

// walk is a walk of one of a file's declarations, named in, of the package
// in dir, for where it breaks the rule r and the calls it makes that r
// allows, the structs of the types r bans as structsBanned gives them.
type walk struct {
	r       Rule
	info    *types.Info
	dir, in string
	banned  map[*types.Struct]string
	found   []Breach
	allowed []Use
	// judged are the identifiers naming a function of Calls judged already:
	// a call's callee, by its arguments, or one used as a value.
	judged map[*ast.Ident]bool
}

// visit finds where n breaks the rule, and walks on into it.
func (w *walk) visit(n ast.Node) bool {
	if n != nil && w.r.Node != nil {
		if says := w.r.Node(n, w.info); says != "" {
			w.breach(n.Pos(), says)
		}
	}
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

// field finds sel reading a field of one of Fields, or a field of Used
// banned here.
func (w *walk) field(sel *ast.SelectorExpr) {
	s, ok := w.info.Selections[sel]
	if !ok || s.Kind() != types.FieldVal {
		return
	}
	if field := w.fieldRead(s); field != "" {
		w.breach(sel.Sel.Pos(), w.r.Read(field))
	}
	if ban, ok := w.r.Used[w.fieldNamed(s)]; ok && ban.holdsIn(w.dir) {
		w.breach(sel.Sel.Pos(), ban.Says)
	}
}

// used finds id using a function of Used, or one of Owners' that takes one
// of Fields, or one the rule allows; or a type, a constant or a
// package-level variable of Used, a type by its own name, or that of the
// type it's an alias of, or defined as, as typeOf names it.
func (w *walk) used(id *ast.Ident) {
	switch obj := w.info.Uses[id].(type) {
	case *types.Func:
		w.usedFunc(id, obj)
	case *types.TypeName:
		named, _ := types.Unalias(obj.Type()).(*types.Named)
		w.usedNamed(id, w.r.Declared(obj), w.typeOf(named))
	case *types.Const, *types.Var:
		w.usedNamed(id, w.r.Declared(obj))
	}
}

// usedNamed finds id naming, by one of names, what Used bans here.
func (w *walk) usedNamed(id *ast.Ident, names ...string) {
	for _, name := range names {
		if ban, ok := w.r.Used[name]; ok && ban.holdsIn(w.dir) {
			w.breach(id.Pos(), ban.Says)
			return
		}
	}
}

// usedFunc finds id using fn, where it's of Used, or one of Owners' that
// takes one of Fields, or one the rule allows.
func (w *walk) usedFunc(id *ast.Ident, fn *types.Func) {
	switch name, says := w.r.called(fn, w.dir); {
	case says != "":
		w.breach(id.Pos(), says)
	case slices.Contains(w.r.Allowed, name):
		w.allowed = append(w.allowed, Use{At: id.Pos(), In: w.in, Name: name})
	}
}

// call finds the call of a function of Calls that breaks the rule here, by
// its arguments, or that can't be judged by them.
func (w *walk) call(call *ast.CallExpr) {
	name, id, ban, ok := w.callBan(call.Fun)
	if !ok {
		return
	}
	w.judged[id] = true
	switch breaks, lost := ban.judge(argued(call, w.info), w.info); {
	case breaks:
		w.breach(id.Pos(), ban.Says)
	case lost != nil:
		w.breach(id.Pos(), lostSays(name, "the type of its argument "+types.ExprString(lost)))
	}
}

// value finds e naming a function of Calls banned here as a value, not the
// callee of a call: what it's given where it's called can't be judged, so it
// breaks the rule.
func (w *walk) value(e ast.Expr) {
	_, id, ban, ok := w.callBan(e)
	if !ok || w.judged[id] {
		return
	}
	w.judged[id] = true
	w.breach(id.Pos(), ban.Says)
}

// callBan is the full name of the function e names, the identifier naming
// it, and its ban, where it's one of Calls, banned here.
func (w *walk) callBan(e ast.Expr) (string, *ast.Ident, CallBan, bool) {
	name, id := funcNamed(e, w.info)
	ban, ok := w.r.Calls[name]
	return name, id, ban, ok && ban.holdsIn(w.dir)
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
	h := holding{r: r, seen: make(map[string]bool)}
	if recv := sig.Recv(); recv != nil {
		if held := h.held(recv.Type()); held != "" {
			return held
		}
	}
	return h.heldIn(typesOf(sig.Params()))
}

// holding is a look for the one of a rule's Fields a type holds, and the
// named types it has looked into, by their names in full, so a recursive one
// ends it.
type holding struct {
	r    Rule
	seen map[string]bool
}

// held names the one of Fields t is, or holds through pointers, slices,
// arrays, maps, channels, a func's parameters and results, a generic named
// type's type arguments, and a named type, declared where one of Fields is,
// as the type it's defined as, as "status.Account": "" where it's none. Any
// other named type, as the read surface's own, is judged by its name alone.
func (h holding) held(t types.Type) string {
	switch t := types.Unalias(t).(type) {
	case *types.Pointer:
		return h.held(t.Elem())
	case *types.Slice:
		return h.held(t.Elem())
	case *types.Array:
		return h.held(t.Elem())
	case *types.Chan:
		return h.held(t.Elem())
	case *types.Map:
		return cmp.Or(h.held(t.Key()), h.held(t.Elem()))
	case *types.Signature:
		return cmp.Or(h.heldIn(typesOf(t.Params())), h.heldIn(typesOf(t.Results())))
	case *types.Named:
		return h.named(t)
	}
	return ""
}

// named names the one of Fields the named type t is, or holds through its
// type arguments, or, declared where one of Fields is, the type it's defined
// as, looked into once.
func (h holding) named(t *types.Named) string {
	if name := h.r.Type(t); slices.Contains(h.r.Fields, name) {
		return name
	}
	if held := h.heldIn(t.TypeArgs().Types()); held != "" {
		return held
	}
	name := types.TypeString(t, nil)
	if h.seen[name] || !h.r.declaresFields(t) {
		return ""
	}
	h.seen[name] = true
	return h.held(t.Underlying())
}

// heldIn names the first of Fields one of ts holds, as held finds it: ""
// where none does.
func (h holding) heldIn(ts iter.Seq[types.Type]) string {
	for t := range ts {
		if held := h.held(t); held != "" {
			return held
		}
	}
	return ""
}

// declaresFields reports whether the package that declares named declares
// one of Fields.
func (r Rule) declaresFields(named *types.Named) bool {
	pkg := named.Obj().Pkg()
	if pkg == nil || !strings.HasPrefix(pkg.Path(), r.internal) {
		return false
	}
	declared := strings.TrimPrefix(pkg.Path(), r.internal) + "."
	return slices.ContainsFunc(r.Fields, func(field string) bool { return strings.HasPrefix(field, declared) })
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
// embedded on the way, each named as typeOf names it; "" where it passes
// through none.
func (w *walk) fieldRead(sel *types.Selection) string {
	t := sel.Recv()
	for _, index := range sel.Index() {
		named, s := StructOf(t)
		if s == nil {
			return ""
		}
		if name := w.typeOf(named); slices.Contains(w.r.Fields, name) {
			return name + "." + sel.Obj().Name()
		}
		t = s.Field(index).Type()
	}
	return ""
}

// fieldNamed names the field sel reads by the type that declares it, as
// "status.Account.ID", through each field embedded on the way, named as
// typeOf names it: "" where that type is no named struct of the module's
// internal packages.
func (w *walk) fieldNamed(sel *types.Selection) string {
	t, path := sel.Recv(), sel.Index()
	for _, index := range path[:len(path)-1] {
		_, s := StructOf(t)
		if s == nil {
			return ""
		}
		t = s.Field(index).Type()
	}
	named, _ := StructOf(t)
	if name := w.typeOf(named); name != "" {
		return name + "." + sel.Obj().Name()
	}
	return ""
}

// typeOf names the named type t as Names.Type does, but one defined as a
// type of Fields or Used, directly or through others, by that type's name:
// it's that type's struct, as structsBanned has it, under another name.
func (w *walk) typeOf(t *types.Named) string {
	if t != nil {
		if s, ok := t.Underlying().(*types.Struct); ok && w.banned[s] != "" {
			return w.banned[s]
		}
	}
	return w.r.Type(t)
}

// structsBanned are the structs of the types of Fields and Used that pkgs
// declare, each by its type's name, the first of Fields, then of Used in
// order, that names it. A type defined as another, as
// "type Over status.Account" is, has the same struct, as go/types gives it.
func (r Rule) structsBanned(pkgs map[string]*types.Package) map[*types.Struct]string {
	banned := make(map[*types.Struct]string)
	for _, name := range slices.Concat(r.Fields, slices.Sorted(maps.Keys(r.Used))) {
		obj := r.typeNamed(pkgs, name)
		if obj == nil {
			continue
		}
		if s, ok := obj.Type().Underlying().(*types.Struct); ok && banned[s] == "" {
			banned[s] = name
		}
	}
	return banned
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
