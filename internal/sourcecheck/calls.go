package sourcecheck

import (
	"go/ast"
	"go/constant"
	"go/types"
	"slices"
)

// CallBan is how a call of a function breaks a rule, and where, as Ban says:
// where Breaks judges it does, by its arguments, or, where Breaks is nil,
// every call. The function used as a value, not called, breaks the rule
// wherever the ban holds, as what it's given where it's called can't be
// judged.
type CallBan struct {
	Ban
	// Breaks judges a call by its arguments, each at its parameter's index:
	// a method expression's, as time.Time.Format(t, layout), less the
	// receiver.
	Breaks func(call *ast.CallExpr, info *types.Info) bool
}

// ConstantOtherThan breaks a call whose argument at index arg is a constant
// string, named or not, other than those allowed. An argument known only at
// run time isn't judged.
func ConstantOtherThan(arg int, allowed ...string) func(*ast.CallExpr, *types.Info) bool {
	return func(call *ast.CallExpr, info *types.Info) bool {
		if arg >= len(call.Args) {
			return false
		}
		v := info.Types[call.Args[arg]].Value
		return v != nil && v.Kind() == constant.String && !slices.Contains(allowed, constant.StringVal(v))
	}
}

// OperandOf breaks a call any of whose arguments is of a type named in
// names, or whose underlying type is, as a type defined as float64 is
// float64's: each named by its package's path in full, as "time.Duration",
// "example.com/fixture/internal/money.Picodollars" or "float64". An
// untyped constant is of the type it's passed as: given to fmt, its default.
func OperandOf(names ...string) func(*ast.CallExpr, *types.Info) bool {
	return func(call *ast.CallExpr, info *types.Info) bool {
		return slices.ContainsFunc(call.Args, func(arg ast.Expr) bool {
			t := info.TypeOf(arg)
			return t != nil && (slices.Contains(names, typeName(t)) || slices.Contains(names, typeName(t.Underlying())))
		})
	}
}

// typeName names t by its package's path in full, as "time.Duration".
func typeName(t types.Type) string {
	return types.TypeString(types.Unalias(t), nil)
}

// funcNamed is the full name of the function e names, as
// (*types.Func).FullName has it, or, of a package the loader stubs, its path
// and the function's name, and the identifier naming it: "" and nil where e
// names no function.
func funcNamed(e ast.Expr, info *types.Info) (string, *ast.Ident) {
	switch e := ast.Unparen(e).(type) {
	case *ast.Ident:
		if fn, ok := info.Uses[e].(*types.Func); ok {
			return fn.FullName(), e
		}
	case *ast.SelectorExpr:
		switch obj := info.Uses[e.Sel].(type) {
		case *types.Func:
			return obj.FullName(), e.Sel
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
