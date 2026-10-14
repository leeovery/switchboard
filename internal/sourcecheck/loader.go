// Package sourcecheck reads the module's source, type-checked, for the guards
// that hold packages to a rule: where a file reads a field it mustn't, uses a
// function it mustn't, calls one that takes what it mustn't read, or calls
// one, the standard library's included, with arguments it mustn't give, and
// what a guard lets break its rule. Tests alone import it, and its own tests
// keep it out of every binary.
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
	// Pkg is the package it's of.
	Pkg *types.Package
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
	// elsewhere are the non-test files of the directories read whole that
	// this platform doesn't build, by their directory; and others the
	// loaders of the platforms that build them, as builtBy finds them.
	elsewhere map[string][]string
	others    map[platform]*Loader
}

// platform is a GOOS and GOARCH a package is built for.
type platform struct {
	goos, goarch string
}

// platforms are those a file this platform doesn't build is read as built
// for, the first that builds it: one built for none of them can't be
// judged.
var platforms = []platform{
	{"linux", "amd64"}, {"darwin", "arm64"}, {"windows", "amd64"}, {"freebsd", "amd64"},
	{"openbsd", "amd64"}, {"plan9", "amd64"}, {"js", "wasm"}, {"wasip1", "wasm"},
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
	return newLoader(t, root, module, read, ctx)
}

// newLoader returns a loader of the module at root with the given path,
// which reads the packages in the directories read, from the root, whole,
// as ctx builds them.
func newLoader(t *testing.T, root, module string, read []string, ctx build.Context) *Loader {
	return &Loader{
		t: t, fset: token.NewFileSet(), root: root, module: module, ctx: ctx, read: read,
		pkgs: map[string]*types.Package{"unsafe": types.Unsafe}, checked: make(map[string][]File),
		elsewhere: make(map[string][]string), others: make(map[platform]*Loader),
	}
}

// Load is the type-checked files of the package in dir, from the module's
// root, read whole: those this platform builds, as it builds them, and
// each it doesn't, as the first of platforms that builds it does, so no
// file goes unjudged for the platform the guard runs on.
func (l *Loader) Load(dir string) []File {
	l.t.Helper()
	if _, err := l.Import(path.Join(l.module, dir)); err != nil {
		l.t.Fatal(err)
	}
	files := slices.Clone(l.checked[dir])
	for _, name := range l.elsewhere[dir] {
		files = append(files, l.builtElsewhere(dir, name))
	}
	if len(files) == 0 {
		l.t.Fatalf("%s has no Go files to read: the guard reads nothing of it", dir)
	}
	return files
}

// builtElsewhere is the file with the given name in dir, from the module's
// root, type-checked as the first of platforms that builds it builds it.
func (l *Loader) builtElsewhere(dir, name string) File {
	l.t.Helper()
	other := l.builtBy(dir, name)
	if _, err := other.Import(path.Join(l.module, dir)); err != nil {
		l.t.Fatal(err)
	}
	at := path.Join(dir, name)
	i := slices.IndexFunc(other.checked[dir], func(f File) bool { return f.Path == at })
	if i < 0 {
		l.t.Fatalf("%s is built for %s/%s, but wasn't read as built for it", at, other.ctx.GOOS, other.ctx.GOARCH)
	}
	return other.checked[dir][i]
}

// builtBy is the loader of the first of platforms that builds the file with
// the given name in dir, from the module's root: the guard fails where none
// does, as it can't judge the file.
func (l *Loader) builtBy(dir, name string) *Loader {
	l.t.Helper()
	for _, p := range platforms {
		ctx := l.ctx
		ctx.GOOS, ctx.GOARCH = p.goos, p.goarch
		if built, err := ctx.MatchFile(filepath.Join(l.root, dir), name); err != nil || !built {
			continue
		}
		if l.others[p] == nil {
			l.others[p] = newLoader(l.t, l.root, l.module, l.read, ctx)
		}
		return l.others[p]
	}
	l.t.Fatalf("%s is built for no platform the guard reads: it can't judge it", path.Join(dir, name))
	return nil
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
// for the loader's platform, noting the non-test files it doesn't build: of
// a directory read whole, its function bodies too,
// keeping what each of its files' expressions is, its type and any constant
// value; of the rest, its declarations alone. A package of the module
// type-checks past any error, as what it uses of a stubbed package is
// unknown: what a rule would judge there, it can't, so it breaks the rule,
// as Rule.Breaches has it. The standard library's type-checks clean, or the
// guard reads it wrong.
func (l *Loader) check(importPath, dir string, std bool) (*types.Package, error) {
	bp, err := l.ctx.ImportDir(dir, 0)
	if err != nil {
		return nil, err
	}
	rel, _ := filepath.Rel(l.root, dir)
	rel = filepath.ToSlash(rel)
	whole := !std && slices.Contains(l.read, rel)
	if whole {
		l.elsewhere[rel] = slices.DeleteFunc(slices.Clone(bp.IgnoredGoFiles), func(name string) bool { return strings.HasSuffix(name, "_test.go") })
	}
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
	var info *types.Info
	if whole {
		info = &types.Info{
			Types:      make(map[ast.Expr]types.TypeAndValue),
			Selections: make(map[*ast.SelectorExpr]*types.Selection),
			Uses:       make(map[*ast.Ident]types.Object),
			Defs:       make(map[*ast.Ident]types.Object),
		}
	}
	pkg, _ := conf.Check(importPath, l.fset, files, info)
	if std && len(typeErrs) > 0 {
		return nil, typeErrs[0]
	}
	if whole {
		for i, name := range bp.GoFiles {
			l.checked[rel] = append(l.checked[rel], File{Path: path.Join(rel, name), Fset: l.fset, AST: files[i], Info: info, Pkg: pkg})
		}
	}
	return pkg, nil
}
