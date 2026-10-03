package main

import (
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const (
	// module is switchboard's module path.
	module      = "github.com/leeovery/switchboard"
	switchboard = module + "/cmd/switchboard"
	captureTool = module + "/cmd/capturetool"
	capturePkg  = module + "/internal/capture"
)

func TestSwitchboardBuildsNothingOfTheCapture(t *testing.T) {
	built := imported(t, switchboard)

	if !built[module+"/internal/dashboard/watch"] {
		t.Fatalf("%s reaches %v, but not the dashboard it's known to: the guard below reads nothing", switchboard, built)
	}
	if built[capturePkg] {
		t.Errorf("%s imports %s, through one of its packages: the capture harness's fakes and fixtures must stay out of switchboard's binary", switchboard, capturePkg)
	}
}

func TestTheCaptureToolBuildsTheCapture(t *testing.T) {
	if !imported(t, captureTool)[capturePkg] {
		t.Errorf("%s doesn't import %s, so the guard against switchboard importing it guards nothing", captureTool, capturePkg)
	}
}

// imported are the module's packages pkg imports, directly or through
// others, read from their source: every file of each but its tests, whatever
// its build constraints, so a build for any platform is among them.
func imported(t *testing.T, pkg string) map[string]bool {
	t.Helper()
	root := moduleRoot(t)
	seen := make(map[string]bool)
	for queue := []string{pkg}; len(queue) > 0; queue = queue[1:] {
		for _, p := range imports(t, filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(queue[0], module)))) {
			if strings.HasPrefix(p, module+"/") && !seen[p] {
				seen[p] = true
				queue = append(queue, p)
			}
		}
	}
	return seen
}

// imports are the import paths of the Go files in dir, but its tests'.
func imports(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	fset := token.NewFileSet()
	for _, e := range entries {
		if name := e.Name(); e.IsDir() || path.Ext(name) != ".go" || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, spec := range file.Imports {
			p, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				t.Fatal(err)
			}
			paths = append(paths, p)
		}
	}
	return paths
}

// moduleRoot is the module's root: the directory above this package's, where
// go test runs its tests, that holds go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod in any directory above the package's")
		}
		dir = parent
	}
}
