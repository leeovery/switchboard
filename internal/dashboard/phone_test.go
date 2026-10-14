package dashboard

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

// Phone alone reads the width a frame is a phone's under, so every form
// turns to a phone's where Phone says, and the cut moves in one place: this
// test reads the package's own source, its tests aside.
func TestPhoneAloneReadsTheWidthAFrameIsAPhonesUnder(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	files := token.NewFileSet()
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(files, name, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil && fn.Name.Name == "Phone" {
				continue
			}
			for _, at := range reads(d, "phoneUnder") {
				t.Errorf("%s reads phoneUnder, want Phone(width), the rule's one owner", files.Position(at))
			}
		}
	}
}

// reads are where node reads the name given, but where it declares it.
func reads(node ast.Node, name string) []token.Pos {
	var at []token.Pos
	ast.Inspect(node, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.ValueSpec:
			for _, v := range n.Values {
				at = append(at, reads(v, name)...)
			}
			return false
		case *ast.Ident:
			if n.Name == name {
				at = append(at, n.Pos())
			}
		}
		return true
	})
	return at
}
