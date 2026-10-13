package sourcecheck_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/sourcecheck"
)

// sourceCheck is this package's import path, under switchboard's module.
const sourceCheck = "github.com/leeovery/switchboard/internal/sourcecheck"

func TestNothingButTestsImportsTheSourceCheck(t *testing.T) {
	sourcecheck.ReportEach(t, importedOutsideTests(t, sourcecheck.ModuleRoot(t)))
}

func TestTheImportGuardFindsWhatItGuardsAgainst(t *testing.T) {
	importing := "package main\n\nimport _ \"" + sourceCheck + "\"\n"
	root := sourcecheck.WriteModule(t, map[string]string{
		"go.mod":                     "module example.com/fixture\n",
		"cmd/tool/main.go":           importing,
		"internal/kept/kept.go":      "package kept\n\nimport \"strings\"\n\nvar _ = strings.ToUpper\n",
		"internal/kept/kept_test.go": importing,
		"testdata/main.go":           importing,
	})
	got := importedOutsideTests(t, root)
	want := []string{"cmd/tool/main.go imports " + sourceCheck + importedFix}
	if !slices.Equal(got, want) {
		t.Errorf("found %q, want %q", got, want)
	}
}

// importedFix says why a file but a test mustn't import the source check.
const importedFix = ": it reads the module's source for tests, and must stay out of every binary"

// importedOutsideTests are the files of the module at root, but its tests,
// that import the source check, in their paths' order.
func importedOutsideTests(t *testing.T, root string) []string {
	t.Helper()
	files := importsOf(t, root)
	var problems []string
	for _, file := range slices.Sorted(maps.Keys(files)) {
		if !strings.HasSuffix(file, "_test.go") && files[file][sourceCheck] {
			problems = append(problems, file+" imports "+sourceCheck+importedFix)
		}
	}
	return problems
}

func TestTheGuardsReadTheSourceThroughTheSourceCheck(t *testing.T) {
	imports := importsOf(t, sourcecheck.ModuleRoot(t))
	for _, guard := range []string{"internal/status/guard_test.go"} {
		if !imports[guard][sourceCheck] {
			t.Errorf("%s doesn't import %s, so the guard keeping it out of the binaries guards nothing it's known to", guard, sourceCheck)
		}
	}
}

// importsOf are the import paths of each Go file of the module at root, by
// its path from the root: every file, whatever its build constraints, so a
// build for any platform is among them; none under testdata, vendor, or a
// directory the go command passes over.
func importsOf(t *testing.T, root string) map[string]map[string]bool {
	t.Helper()
	fset := token.NewFileSet()
	files := make(map[string]map[string]bool)
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && p != root && skipped(d.Name()):
			return filepath.SkipDir
		case d.IsDir() || !strings.HasSuffix(p, ".go"):
			return nil
		}
		file, err := parser.ParseFile(fset, p, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		imports := make(map[string]bool)
		for _, spec := range file.Imports {
			path, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			imports[path] = true
		}
		files[filepath.ToSlash(rel)] = imports
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func skipped(dir string) bool {
	return dir == "testdata" || dir == "vendor" || strings.HasPrefix(dir, ".") || strings.HasPrefix(dir, "_")
}
