package sourcecheck

import (
	"cmp"
	"go/ast"
	"go/types"
	"slices"
	"strings"
)

// Lost is an identifier the type checker never resolved, as it lost the type
// of what it's selected from, or never reached it. The loader stubs every
// package but the module's and the standard library's, so the type checker
// loses the type of what a package of the module takes from one, and of all
// that's worked out of it: what's selected of it, the type asserted of it,
// the keys of a literal of its type. What a rule would judge there, it
// can't, so what's lost breaks the rule.
type Lost struct {
	Name *ast.Ident
	// Of is what Name is selected from: nil of a bare identifier.
	Of ast.Expr
}

// Says says how what's lost breaks a rule that would judge it.
func (l Lost) Says() string {
	if l.Of == nil {
		return lostSays(l.Name.Name, "its type")
	}
	of := types.ExprString(l.Of)
	return lostSays(of+"."+l.Name.Name, "the type of "+of)
}

// LostIn are the identifiers in n the type checker never resolved, in the
// order they come: each selected from what it lost the type of, but from a
// stubbed package's name, which names what it selects by the package's
// path; and each bare, as a key of a literal of a type it lost, or what a
// stubbed package dot-imported declares.
func (f File) LostIn(n ast.Node) []Lost {
	var lost []Lost
	selected := make(map[*ast.Ident]bool)
	ast.Inspect(n, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if f.Info.Uses[n.Sel] == nil {
				selected[n.Sel] = true
				if !f.isPackage(n.X) {
					lost = append(lost, Lost{Name: n.Sel, Of: n.X})
				}
			}
		case *ast.Ident:
			if !selected[n] && !f.resolved(n) {
				lost = append(lost, Lost{Name: n})
			}
		}
		return true
	})
	slices.SortStableFunc(lost, func(a, b Lost) int { return cmp.Compare(a.Name.Pos(), b.Name.Pos()) })
	return lost
}

// isPackage reports whether e names an imported package.
func (f File) isPackage(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	if !ok {
		return false
	}
	_, ok = f.Info.Uses[id].(*types.PkgName)
	return ok
}

// resolved reports whether the type checker resolved id, as what it uses or
// what it declares: nil, of the blank identifier and a package's own name.
func (f File) resolved(id *ast.Ident) bool {
	if _, ok := f.Info.Uses[id]; ok {
		return true
	}
	_, ok := f.Info.Defs[id]
	return ok
}

// lostFix says how to give a guard what the loader lost.
const lostFix = "give the guard a type it can see, as a variable declared with one"

// lostSays says how what breaks a rule that can't judge it, as of, its type,
// is lost to a package the loader stubs.
func lostSays(what, of string) string {
	return what + " can't be judged, as " + of + " is lost to a package the loader stubs; " + lostFix
}

// typeSeen is the type and value of e, and whether the guard can see its
// type: false where the type checker lost it, or the type it's defined as,
// to a package the loader stubs.
func typeSeen(e ast.Expr, info *types.Info) (types.TypeAndValue, bool) {
	tv, ok := info.Types[e]
	return tv, ok && tv.Type.Underlying() != types.Typ[types.Invalid]
}

// unseen are the names by which what's lost may break a rule: of a selector,
// a field it may read, or a function or method it may use; of a bare
// identifier, a function it may use.
type unseen struct {
	selected, bare map[string]bool
}

// bans reports whether what's lost may break the rule.
func (u unseen) bans(l Lost) bool {
	if l.Of == nil {
		return u.bare[l.Name.Name]
	}
	return u.selected[l.Name.Name]
}

// function adds the function or method named, as Names or
// (*types.Func).FullName names it.
func (u unseen) function(name string, method bool) {
	name = name[strings.LastIndex(name, ".")+1:]
	u.selected[name] = true
	if !method {
		u.bare[name] = true
	}
}

// unseenIn are the names by which what's lost in the package pkg, in dir,
// may break the rule: the fields of each of Fields, through any field
// embedded in it, that pkg can read; each function and field of Used, a
// field selected as a method is, and each function of Calls, whose ban holds
// in dir; and each function and method of Owners that takes one of Fields,
// that pkg can use, but those allowed.
func (r Rule) unseenIn(pkg *types.Package, dir string) unseen {
	u := unseen{selected: make(map[string]bool), bare: make(map[string]bool)}
	for name, ban := range r.Used {
		if ban.holdsIn(dir) {
			u.function(name, namesMethod(name))
		}
	}
	for name, ban := range r.Calls {
		if ban.holdsIn(dir) {
			u.function(name, strings.HasPrefix(name, "("))
		}
	}
	pkgs := reachable(pkg)
	embedded := make(map[*types.Struct]bool)
	for _, name := range r.Fields {
		if obj := r.typeNamed(pkgs, name); obj != nil {
			fieldsOf(obj.Type(), pkg, u.selected, embedded)
		}
	}
	for _, owner := range r.Owners {
		if p := pkgs[r.internal+owner]; p != nil {
			r.taking(p, pkg, u)
		}
	}
	return u
}

// namesMethod reports whether name, as Names.Func has it, names a method:
// its package's name, its receiver's type's, then its own.
func namesMethod(name string) bool {
	return strings.Count(name[strings.LastIndex(name, "/")+1:], ".") == 2
}

// reachable are the packages pkg imports, through any package between, and
// pkg itself, by path.
func reachable(pkg *types.Package) map[string]*types.Package {
	pkgs := make(map[string]*types.Package)
	var reach func(p *types.Package)
	reach = func(p *types.Package) {
		if pkgs[p.Path()] != nil {
			return
		}
		pkgs[p.Path()] = p
		for _, imported := range p.Imports() {
			reach(imported)
		}
	}
	reach(pkg)
	return pkgs
}

// typeNamed is the type name names, as Names.Type has it, among pkgs: nil
// where none of them declares it.
func (r Rule) typeNamed(pkgs map[string]*types.Package, name string) *types.TypeName {
	i := strings.LastIndex(name, ".")
	if i < 0 || pkgs[r.internal+name[:i]] == nil {
		return nil
	}
	obj, _ := pkgs[r.internal+name[:i]].Scope().Lookup(name[i+1:]).(*types.TypeName)
	return obj
}

// fieldsOf adds to names the fields of the struct t is, or points to, that
// pkg can read, through each field embedded in it, once each struct.
func fieldsOf(t types.Type, pkg *types.Package, names map[string]bool, seen map[*types.Struct]bool) {
	_, s := StructOf(t)
	if s == nil || seen[s] {
		return
	}
	seen[s] = true
	for field := range s.Fields() {
		if field.Exported() || field.Pkg() == pkg {
			names[field.Name()] = true
		}
		if field.Embedded() {
			fieldsOf(field.Type(), pkg, names, seen)
		}
	}
}

// taking adds to u each function and method of the owner p that takes one
// of Fields, that pkg can use, but those the rule allows.
func (r Rule) taking(p, pkg *types.Package, u unseen) {
	add := func(fn *types.Func, method bool) {
		if (fn.Exported() || fn.Pkg() == pkg) && !slices.Contains(r.Allowed, r.Func(fn)) && r.takes(fn) != "" {
			u.function(fn.Name(), method)
		}
	}
	for _, name := range p.Scope().Names() {
		switch obj := p.Scope().Lookup(name).(type) {
		case *types.Func:
			add(obj, false)
		case *types.TypeName:
			named, ok := obj.Type().(*types.Named)
			if !ok {
				continue
			}
			for m := range named.Methods() {
				add(m, true)
			}
			if iface, ok := named.Underlying().(*types.Interface); ok {
				for m := range iface.Methods() {
					add(m, true)
				}
			}
		}
	}
}
