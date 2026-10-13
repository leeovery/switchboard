// Package sourcecheck reads the module's source, type-checked, for the guards
// that hold packages to a rule: where a file reads a field it mustn't, uses a
// function it mustn't, or calls one that takes what it mustn't read, and what
// a guard lets break its rule. Tests alone import it, and its own tests keep
// it out of every binary.
package sourcecheck

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// File is one of a package's non-test files, parsed and type-checked.
type File struct {
	// Path is the file's path from the module's root, with slashes.
	Path string
	Fset *token.FileSet
	AST  *ast.File
	Info *types.Info
}

// At is where pos is in the file: its path and line.
func (f File) At(pos token.Pos) string {
	return fmt.Sprintf("%s:%d", f.Path, f.Fset.Position(pos).Line)
}

// Loader type-checks packages from their source, without the go command,
// which no test may run: the module's, from its root, and the standard
// library's, from GOROOT, built as this platform builds them, without cgo.
// Any other package, a module the module requires, is stubbed: an empty
// package, as its source is where only the go command finds it. Nothing a
// guard reads the fields of comes through one.
type Loader struct {
	t            *testing.T
	fset         *token.FileSet
	root, module string
	ctx          build.Context
	// read are the directories, from the module's root, whose packages are
	// read whole: every function body type-checked, and what each expression
	// is kept. Of the rest, their declarations alone are.
	read    []string
	pkgs    map[string]*types.Package
	checked map[string][]File
}

// NewLoader returns a loader of the module at root with the given path,
// which reads the packages in the directories read, from the root, whole.
func NewLoader(t *testing.T, root, module string, read []string) *Loader {
	t.Helper()
	ctx := build.Default
	ctx.CgoEnabled = false
	if ctx.GOROOT == "" {
		t.Fatal("no GOROOT, whose source the standard library's types are read from")
	}
	return &Loader{
		t: t, fset: token.NewFileSet(), root: root, module: module, ctx: ctx, read: read,
		pkgs: map[string]*types.Package{"unsafe": types.Unsafe}, checked: make(map[string][]File),
	}
}

// Load is the type-checked files of the package in dir, from the module's
// root, read whole.
func (l *Loader) Load(dir string) []File {
	l.t.Helper()
	if _, err := l.Import(path.Join(l.module, dir)); err != nil {
		l.t.Fatal(err)
	}
	files := l.checked[dir]
	if len(files) == 0 {
		l.t.Fatalf("%s has no Go files to read: the guard reads nothing of it", dir)
	}
	return files
}

func (l *Loader) Import(importPath string) (*types.Package, error) {
	return l.ImportFrom(importPath, l.root, 0)
}

// ImportFrom is the package at importPath, imported from the package in dir,
// type-checked from its source, once.
func (l *Loader) ImportFrom(importPath, dir string, _ types.ImportMode) (*types.Package, error) {
	if pkg, ok := l.pkgs[importPath]; ok {
		return pkg, nil
	}
	src, std, ok := l.dirOf(importPath, dir)
	if !ok {
		pkg := types.NewPackage(importPath, stubName(importPath))
		pkg.MarkComplete()
		l.pkgs[importPath] = pkg
		return pkg, nil
	}
	pkg, err := l.check(importPath, src, std)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", importPath, err)
	}
	l.pkgs[importPath] = pkg
	return pkg, nil
}

// dirOf is the directory of the source of the package at importPath, imported
// from the package in dir, and whether it's the standard library's: one of
// the module's, from its root; or one of the standard library's, or one it
// vendors, imported from it. It reports false of any other.
func (l *Loader) dirOf(importPath, dir string) (string, bool, bool) {
	if rel, ok := strings.CutPrefix(importPath, l.module); ok && (rel == "" || rel[0] == '/') {
		return filepath.Join(l.root, filepath.FromSlash(rel)), false, true
	}
	goSrc := filepath.Join(l.ctx.GOROOT, "src")
	var candidates []string
	if within(dir, goSrc) {
		candidates = append(candidates, filepath.Join(goSrc, "vendor", filepath.FromSlash(importPath)))
	}
	if first, _, _ := strings.Cut(importPath, "/"); !strings.Contains(first, ".") {
		candidates = append(candidates, filepath.Join(goSrc, filepath.FromSlash(importPath)))
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c, true, true
		}
	}
	return "", false, false
}

// within reports whether dir is base or one of its subdirectories.
func within(dir, base string) bool {
	rel, err := filepath.Rel(base, dir)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// stubName is the name a stubbed package goes by: the last part of its import
// path, but for a major version, as "v2", the part before it.
func stubName(importPath string) string {
	parts := strings.Split(importPath, "/")
	name := parts[len(parts)-1]
	if len(parts) > 1 && majorVersion.MatchString(name) {
		name = parts[len(parts)-2]
	}
	return strings.ReplaceAll(name, "-", "")
}

var majorVersion = regexp.MustCompile(`^v[0-9]+$`)

// check type-checks the package at importPath, its source in dir, as built
// for this platform: of a directory read whole, its function bodies too,
// keeping what each of its files' expressions is; of the rest, its
// declarations alone. A package of the module type-checks past any error,
// as what it uses of a stubbed package is unknown; the standard library's
// type-checks clean, or the guard reads it wrong.
func (l *Loader) check(importPath, dir string, std bool) (*types.Package, error) {
	bp, err := l.ctx.ImportDir(dir, 0)
	if err != nil {
		return nil, err
	}
	rel, _ := filepath.Rel(l.root, dir)
	rel = filepath.ToSlash(rel)
	whole := !std && slices.Contains(l.read, rel)
	files := make([]*ast.File, len(bp.GoFiles))
	for i, name := range bp.GoFiles {
		if files[i], err = parser.ParseFile(l.fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution); err != nil {
			return nil, err
		}
	}
	var typeErrs []error
	conf := types.Config{
		Importer:         l,
		IgnoreFuncBodies: !whole,
		Error:            func(err error) { typeErrs = append(typeErrs, err) },
	}
	info := &types.Info{Selections: make(map[*ast.SelectorExpr]*types.Selection), Uses: make(map[*ast.Ident]types.Object), Defs: make(map[*ast.Ident]types.Object)}
	pkg, _ := conf.Check(importPath, l.fset, files, info)
	if std && len(typeErrs) > 0 {
		return nil, typeErrs[0]
	}
	if whole {
		for i, name := range bp.GoFiles {
			l.checked[rel] = append(l.checked[rel], File{Path: path.Join(rel, name), Fset: l.fset, AST: files[i], Info: info})
		}
	}
	return pkg, nil
}
