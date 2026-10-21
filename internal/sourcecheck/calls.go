package sourcecheck

import (
	"cmp"
	"go/ast"
	"go/constant"
	"go/types"
	"slices"
)

// CallBan is how a call of a function breaks a rule, and where, as Ban says:
// where Breaks judges it does, by its arguments, or can't, or, where Breaks
// is nil, every call. The function used as a value, not called, breaks the
// rule wherever the ban holds, as what it's given where it's called can't be
// judged.
type CallBan struct {
	Ban
	// AsValue says how using the function as a value breaks the rule: Says
	// where it's "".
	AsValue string
	// Breaks judges a call by its arguments, each at its parameter's index:
	// a method expression's, as time.Time.Format(t, layout), less the
	// receiver. It reports whether the call breaks the rule, else the
	// argument it can't judge, as the type checker lost its type to a
	// package the loader stubs: nil where it judged them all.
	Breaks func(call *ast.CallExpr, info *types.Info) (breaks bool, lost ast.Expr)
	// Seen has the ban judge only the calls of the function the type
	// checker sees: its name selected from what the loader lost isn't taken
	// for it, as a ban on a method as common as String would otherwise fail
	// that of every value a stubbed package gives.
	Seen bool
}

// judge is how the call breaks the rule, by its arguments, as Breaks judges
// it: every call, without Breaks.
func (b CallBan) judge(call *ast.CallExpr, info *types.Info) (bool, ast.Expr) {
	if b.Breaks == nil {
		return true, nil
	}
	return b.Breaks(call, info)
}

// ConstantOtherThan breaks a call whose argument at index arg is a constant
// string, named or not, other than those allowed. An argument known only at
// run time isn't judged; one whose type is lost can't be.
func ConstantOtherThan(arg int, allowed ...string) func(*ast.CallExpr, *types.Info) (bool, ast.Expr) {
	return func(call *ast.CallExpr, info *types.Info) (bool, ast.Expr) {
		if arg >= len(call.Args) {
			return false, nil
		}
		tv, seen := typeSeen(call.Args[arg], info)
		if !seen {
			return false, call.Args[arg]
		}
		v := tv.Value
		return v != nil && v.Kind() == constant.String && !slices.Contains(allowed, constant.StringVal(v)), nil
	}
}

// OperandOf breaks a call any of whose arguments is of a type named in
// names, or whose underlying type is, as a type defined as float64 is
// float64's: each named by its package's path in full, as "time.Duration",
// "example.com/fixture/internal/money.Picodollars" or "float64". An
// untyped constant is of the type it's passed as: given to fmt, its default.
// An argument whose type is lost can't be judged, unless another breaks the
// call.
func OperandOf(names ...string) func(*ast.CallExpr, *types.Info) (bool, ast.Expr) {
	return func(call *ast.CallExpr, info *types.Info) (bool, ast.Expr) {
		var lost ast.Expr
		for _, arg := range call.Args {
			tv, seen := typeSeen(arg, info)
			switch {
			case !seen:
				lost = cmp.Or(lost, arg)
			case slices.Contains(names, typeName(tv.Type)) || slices.Contains(names, typeName(tv.Type.Underlying())):
				return true, nil
			}
		}
		return false, lost
	}
}

// typeName names t by its package's path in full, as "time.Duration".
func typeName(t types.Type) string {
	return types.TypeString(types.Unalias(t), nil)
}

// funcNamed is the full name of the function e names, as
// (*types.Func).FullName has it of the function declared, a generic type's
// method by its type parameters, as "(example.com/fixture.box[T]).fit", not
// the type arguments it's called with; or, of a package the loader stubs,
// its path and the function's name; and the identifier naming it: "" and nil
// where e names no function.
func funcNamed(e ast.Expr, info *types.Info) (string, *ast.Ident) {
	switch e := ast.Unparen(e).(type) {
	case *ast.Ident:
		if fn, ok := info.Uses[e].(*types.Func); ok {
			return fn.Origin().FullName(), e
		}
	case *ast.SelectorExpr:
		switch obj := info.Uses[e.Sel].(type) {
		case *types.Func:
			return obj.Origin().FullName(), e.Sel
		case nil:
			if x, ok := e.X.(*ast.Ident); ok {
				if pkg, ok := info.Uses[x].(*types.PkgName); ok {
					return pkg.Imported().Path() + "." + e.Sel.Name, e.Sel
				}
			}
		}
	}
	return "", nil
}

// Callee is the full name of the function call calls, as Calls names it, and
// the arguments Breaks judges it by: "" where it calls no function.
func Callee(call *ast.CallExpr, info *types.Info) (string, []ast.Expr) {
	name, _ := funcNamed(call.Fun, info)
	return name, argued(call, info).Args
}

// argued is call as Breaks judges it: of a method expression, its arguments
// less the receiver.
func argued(call *ast.CallExpr, info *types.Info) *ast.CallExpr {
	sel, _ := ast.Unparen(call.Fun).(*ast.SelectorExpr)
	if s := info.Selections[sel]; s == nil || s.Kind() != types.MethodExpr || len(call.Args) == 0 {
		return call
	}
	method := *call
	method.Args = call.Args[1:]
	return &method
}
