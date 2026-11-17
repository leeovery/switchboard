package router_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/sourcecheck"
)

// claudePackage is the Claude provider's import path, under switchboard's
// module.
const claudePackage = "github.com/leeovery/switchboard/internal/claude"

// The router knows Claude only through the interfaces it declares, Provider
// and Prober among them, which internal/claude satisfies as the CLI wires it:
// none of its files but its tests imports it.
func TestTheRouterImportsNoClaudeButInItsTests(t *testing.T) {
	sourcecheck.ReportEach(t, claudeImported(t, filepath.Join(sourcecheck.ModuleRoot(t), "internal", "router")))
}

func TestTheClaudeGuardFindsAFileButATestImportingClaude(t *testing.T) {
	importing := "package router\n\nimport _ \"" + claudePackage + "\"\n"
	root := sourcecheck.WriteModule(t, map[string]string{
		"go.mod":                         "module example.com/fixture\n",
		"router/state.go":                importing,
		"router/kept.go":                 "package router\n\nimport \"strings\"\n\nvar _ = strings.ToUpper\n",
		"router/state_test.go":           importing,
		"router/testdata/nopools/x.go":   importing,
		"router/state_internal_test.go":  importing,
		"router/darwin_only_darwin.go":   "//go:build darwin\n\n" + importing,
		"router/other_platform_other.go": "//go:build !darwin\n\npackage router\n",
	})
	got := claudeImported(t, filepath.Join(root, "router"))
	want := []string{"darwin_only_darwin.go" + claudeImportedFix, "state.go" + claudeImportedFix}
	if !slices.Equal(got, want) {
		t.Errorf("found %q, want %q", got, want)
	}
}

// claudeImportedFix says why a file of the router mustn't import Claude's
// provider.
const claudeImportedFix = " imports " + claudePackage + ": reach it through an interface the router declares, which internal/claude satisfies"

// claudeImported are the Go files in dir, but its tests, that import Claude's
// provider, in their names' order: every file, whatever its build
// constraints, and none in a directory beneath it.
func claudeImported(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	var problems []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		if slices.ContainsFunc(file.Imports, func(spec *ast.ImportSpec) bool {
			path, err := strconv.Unquote(spec.Path.Value)
			return err == nil && path == claudePackage
		}) {
			problems = append(problems, name+claudeImportedFix)
		}
	}
	return problems
}
