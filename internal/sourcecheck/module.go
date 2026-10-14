package sourcecheck

import (
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// ModulePath is the path of the module at root, as its go.mod declares it.
func ModulePath(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		t.Fatal(err)
	}
	for line := range strings.Lines(string(data)) {
		if p, ok := strings.CutPrefix(strings.TrimSpace(line), "module "); ok {
			return strings.Trim(strings.TrimSpace(p), `"`)
		}
	}
	t.Fatal("go.mod declares no module")
	return ""
}

// ModuleRoot is the module's root: the directory above the test's package's,
// where go test runs its tests, that holds go.mod.
func ModuleRoot(t *testing.T) string {
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

// Packages are the directories of the packages of the module at root, by path
// from the root, with slashes, in their paths' order: each directory holding
// a Go file but a test's, but those the go command passes over.
func Packages(t *testing.T, root string) []string {
	t.Helper()
	dirs := make(map[string]bool)
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && p != root && PassedOver(d.Name()):
			return filepath.SkipDir
		case d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go"):
			return nil
		}
		rel, err := filepath.Rel(root, filepath.Dir(p))
		dirs[filepath.ToSlash(rel)] = true
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return slices.Sorted(maps.Keys(dirs))
}

// PassedOver reports whether the go command passes over the directory with
// the given name, and all under it: testdata, vendor, and one whose name
// starts with a dot or an underscore.
func PassedOver(name string) bool {
	return name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

// WriteModule writes files, by path from the root, into a new directory, and
// returns it.
func WriteModule(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, content := range files {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

// ReportEach fails the test with each problem.
func ReportEach(t *testing.T, problems []string) {
	t.Helper()
	for _, problem := range problems {
		t.Error(problem)
	}
}
