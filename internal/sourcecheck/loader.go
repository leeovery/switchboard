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
	"sync"
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
// guard reads the fields of comes through one. Each package is read once a
// test binary, whichever loader asks for it first, as a universe has it.
type Loader struct {
	t *testing.T
	u *universe
	// read are the directories, from the module's root, whose packages Load
	// gives.
	read []string
}

// platform is a GOOS and GOARCH a package is built for, and the build tag
// it's built with, if any.
type platform struct {
	goos, goarch, tag string
}

// platforms are those a file this platform doesn't build is read as built
// for, the first that builds it: one built for none of them can't be
// judged. The last is a release's build, with the notifications' app
// embedded.
var platforms = []platform{
	{"linux", "amd64", ""}, {"darwin", "arm64", ""}, {"windows", "amd64", ""}, {"freebsd", "amd64", ""},
	{"openbsd", "amd64", ""}, {"plan9", "amd64", ""}, {"js", "wasm", ""}, {"wasip1", "wasm", ""},
	{"darwin", "arm64", "notifyapp"},
}

// here is the platform the guard runs on.
var here = platform{build.Default.GOOS, build.Default.GOARCH, ""}

// contextOf is the build context of the platform p, without cgo.
func contextOf(p platform) build.Context {
	ctx := build.Default
	ctx.CgoEnabled = false
	ctx.GOOS, ctx.GOARCH = p.goos, p.goarch
	if p.tag != "" {
		ctx.BuildTags = append(slices.Clone(ctx.BuildTags), p.tag)
	}
	return ctx
}

// A universe is every package read of a module, or of the standard library,
// as one platform builds them, each type-checked once and shared by every
// loader in the test binary, whether its guards run in parallel or not: a
// module's packages read whole, every function body type-checked and what
// each expression is kept; the standard library's, its declarations alone.
// Each package a module imports is its own, the standard library's universe
// for the same platform's, or a stub of its own.
type universe struct {
	// root and module are the module's root and path: "" of the standard
	// library's.
	root, module string
	p            platform
	ctx          build.Context
	mu           sync.Mutex
	reads        map[string]*read
}

// read is a package read once, by its import path.
type read struct {
	once sync.Once
	pkg  *types.Package
	err  error
	// files are a module's package's non-test files this platform builds,
	// type-checked, and elsewhere the names of those it doesn't.
	files     []File
	elsewhere []string
}

type universeKey struct {
	root, module string
	p            platform
}

var (
	// fset holds every file read, so a position from any universe, the
	// standard library's among them, is found in it.
	fset        = token.NewFileSet()
	universesMu sync.Mutex
	universes   = make(map[universeKey]*universe)
)

// universeOf is the universe of the module at root with the given path, or
// of the standard library where both are "", as p builds it.
func universeOf(root, module string, p platform) *universe {
	universesMu.Lock()
	defer universesMu.Unlock()
	k := universeKey{root, module, p}
	if universes[k] == nil {
		universes[k] = &universe{root: root, module: module, p: p, ctx: contextOf(p), reads: make(map[string]*read)}
	}
	return universes[k]
}

// NewLoader returns a loader of the module at root with the given path,
// whose Load gives the packages in the directories read, from the root.
func NewLoader(t *testing.T, root, module string, read []string) *Loader {
	t.Helper()
	if build.Default.GOROOT == "" {
		t.Fatal("no GOROOT, whose source the standard library's types are read from")
	}
	return &Loader{t: t, u: universeOf(root, module, here), read: read}
}

// Load is the type-checked files of the package in dir, from the module's
// root, one of the directories the loader reads: those this platform
// builds, as it builds them, and each it doesn't, as the first of platforms
// that builds it does, so no file goes unjudged for the platform the guard
// runs on.
func (l *Loader) Load(dir string) []File {
	l.t.Helper()
	if !slices.Contains(l.read, dir) {
		l.t.Fatalf("%s isn't among the directories the loader reads", dir)
	}
	r := l.u.pkgIn(dir)
	if r.err != nil {
		l.t.Fatal(r.err)
	}
	files := slices.Clone(r.files)
	for _, name := range r.elsewhere {
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
	p := l.builtBy(dir, name)
	r := universeOf(l.u.root, l.u.module, p).pkgIn(dir)
	if r.err != nil {
		l.t.Fatal(r.err)
	}
	at := path.Join(dir, name)
	i := slices.IndexFunc(r.files, func(f File) bool { return f.Path == at })
	if i < 0 {
		l.t.Fatalf("%s is built for %s/%s, but wasn't read as built for it", at, p.goos, p.goarch)
	}
	return r.files[i]
}

// builtBy is the first of platforms that builds the file with the given
// name in dir, from the module's root: the guard fails where none does, as
// it can't judge the file.
func (l *Loader) builtBy(dir, name string) platform {
	l.t.Helper()
	for _, p := range platforms {
		ctx := contextOf(p)
		if built, err := ctx.MatchFile(filepath.Join(l.u.root, dir), name); err == nil && built {
			return p
		}
	}
	l.t.Fatalf("%s is built for no platform the guard reads: it can't judge it", path.Join(dir, name))
	return platform{}
}

// Import is the package at importPath, imported from the module's root.
func (l *Loader) Import(importPath string) (*types.Package, error) {
	return l.u.ImportFrom(importPath, l.u.root, 0)
}

// pkgIn is the read of the module's package in dir, from its root.
func (u *universe) pkgIn(dir string) *read {
	importPath := path.Join(u.module, dir)
	return u.readOf(importPath, func(r *read) { u.check(r, importPath, filepath.Join(u.root, filepath.FromSlash(dir))) })
}

// readOf is the read of the package at importPath, read by fill the first
// time it's asked for: whoever else asks meanwhile waits for it.
func (u *universe) readOf(importPath string, fill func(*read)) *read {
	u.mu.Lock()
	r := u.reads[importPath]
	if r == nil {
		r = &read{}
		u.reads[importPath] = r
	}
	u.mu.Unlock()
	r.once.Do(func() { fill(r) })
	return r
}

// Import is the package at importPath, imported from the module's root.
func (u *universe) Import(importPath string) (*types.Package, error) {
	return u.ImportFrom(importPath, u.root, 0)
}

// ImportFrom is the package at importPath, imported from the package in dir:
// one of the module's; one of the standard library's, or one it vendors,
// imported from it; or else a stub.
func (u *universe) ImportFrom(importPath, dir string, _ types.ImportMode) (*types.Package, error) {
	if importPath == "unsafe" {
		return types.Unsafe, nil
	}
	if rel, ok := strings.CutPrefix(importPath, u.module); u.module != "" && ok && (rel == "" || rel[0] == '/') {
		r := u.pkgIn(strings.TrimPrefix(rel, "/"))
		return r.pkg, r.err
	}
	std := universeOf("", "", u.p)
	if src, ok := std.dirOf(importPath, dir); ok {
		r := std.readOf(importPath, func(r *read) { std.check(r, importPath, src) })
		return r.pkg, r.err
	}
	return u.readOf(importPath, func(r *read) {
		r.pkg = types.NewPackage(importPath, stubName(importPath))
		r.pkg.MarkComplete()
	}).pkg, nil
}

// dirOf is the directory of the source of the standard library's package at
// importPath, imported from the package in dir: one of its own, or one it
// vendors, imported from it. It reports false of any other.
func (u *universe) dirOf(importPath, dir string) (string, bool) {
	goSrc := filepath.Join(u.ctx.GOROOT, "src")
	var candidates []string
	if within(dir, goSrc) {
		candidates = append(candidates, filepath.Join(goSrc, "vendor", filepath.FromSlash(importPath)))
	}
	if first, _, _ := strings.Cut(importPath, "/"); !strings.Contains(first, ".") {
		candidates = append(candidates, filepath.Join(goSrc, filepath.FromSlash(importPath)))
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			return c, true
		}
	}
	return "", false
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

// check reads into r the package at importPath, its source in dir, as built
// for the universe's platform.
func (u *universe) check(r *read, importPath, dir string) {
	if err := u.typeCheck(r, importPath, dir); err != nil {
		r.pkg, r.err = nil, fmt.Errorf("reading %s: %w", importPath, err)
	}
}

// typeCheck type-checks into r the package at importPath, its source in dir,
// as built for the universe's platform, noting a module's package's non-test
// files it doesn't build: of a module, its function bodies too, keeping what
// each of its files' expressions is, its type and any constant value; of the
// standard library, its declarations alone. A module's package type-checks
// past any error, as what it uses of a stubbed package is unknown: what a
// rule would judge there, it can't, so it breaks the rule, as Rule.Breaches
// has it. The standard library's type-checks clean, or the guard reads it
// wrong.
func (u *universe) typeCheck(r *read, importPath, dir string) error {
	bp, err := u.ctx.ImportDir(dir, 0)
	if err != nil {
		return err
	}
	files := make([]*ast.File, len(bp.GoFiles))
	for i, name := range bp.GoFiles {
		if files[i], err = parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.SkipObjectResolution); err != nil {
			return err
		}
	}
	whole := u.module != ""
	var typeErrs []error
	conf := types.Config{
		Importer:         u,
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
	r.pkg, _ = conf.Check(importPath, fset, files, info)
	if !whole {
		if len(typeErrs) > 0 {
			return typeErrs[0]
		}
		return nil
	}
	rel, _ := filepath.Rel(u.root, dir)
	rel = filepath.ToSlash(rel)
	r.elsewhere = slices.DeleteFunc(slices.Clone(bp.IgnoredGoFiles), func(name string) bool { return strings.HasSuffix(name, "_test.go") })
	for i, name := range bp.GoFiles {
		r.files = append(r.files, File{Path: path.Join(rel, name), Fset: fset, AST: files[i], Info: info, Pkg: r.pkg})
	}
	return nil
}
