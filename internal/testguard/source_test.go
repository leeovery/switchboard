package testguard_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// These tests read the module's source, so a package can't leave the guard
// out, nor a test or the code under it slip around it, without failing here.

// processStarters are the functions in production code allowed to start a
// process, as "file:function". Each runs one program, and tests replace it,
// so none runs the real one. A new one goes here deliberately.
var processStarters = []string{
	"internal/claude/version.go:commandOutput",
	"internal/dashboard/notify/notify.go:runCommand",
}

const (
	guardImport = "github.com/leeovery/switchboard/internal/testguard"
	// guardDir is testguard's directory, the one place allowed to change the
	// environment.
	guardDir = "internal/testguard"
	// allowListFile is this file, which holds processStarters.
	allowListFile = guardDir + "/source_test.go"
)

// The fix each problem found gives.
const (
	testMainFix    = "func TestMain(m *testing.M) { os.Exit(testguard.Main(m)) }"
	environmentFix = "set a variable with t.Setenv in a test, or inject the environment (a getenv, or exec.Cmd's Env)"
	processFix     = "start it in a runner tests replace, and add that runner to processStarters in " + allowListFile
	userFix        = "take the home directory from the injected HomeDir, which follows HOME"
)

// target is some of a package's functions: its import path and their names.
type target struct {
	importPath string
	names      []string
}

var (
	environmentChangers = []target{
		{importPath: "os", names: []string{"Setenv", "Unsetenv", "Clearenv"}},
		{importPath: "syscall", names: []string{"Setenv", "Unsetenv", "Clearenv"}},
	}
	processStarting = []target{
		{importPath: "os/exec", names: []string{"Command", "CommandContext"}},
		{importPath: "os", names: []string{"StartProcess"}},
		{importPath: "syscall", names: []string{"Exec", "ForkExec", "StartProcess"}},
	}
)

func TestEveryPackageRunsItsTestsThroughTheGuard(t *testing.T) {
	reportEach(t, unguardedPackages(parseModule(t, moduleRoot(t))))
}

func TestOnlyTheGuardChangesTheEnvironment(t *testing.T) {
	reportEach(t, environmentChanges(parseModule(t, moduleRoot(t))))
}

func TestOnlyTheFunctionsAllowedStartProcesses(t *testing.T) {
	reportEach(t, processStarts(parseModule(t, moduleRoot(t)), processStarters))
}

func TestProductionCodeDoesntImportOSUser(t *testing.T) {
	reportEach(t, userImports(parseModule(t, moduleRoot(t))))
}

func TestTheSourceGuardsFindWhatTheyGuardAgainst(t *testing.T) {
	files := parseModule(t, writeModule(t, fixture))
	tests := []struct {
		name string
		got  []string
		want []string
	}{
		{
			name: "unguardedPackages",
			got:  unguardedPackages(files),
			want: []string{
				"exits/main_test.go:10: TestMain doesn't exit with what testguard.Main returns; make it " + testMainFix,
				"unguarded/unguarded_test.go:1: the package has tests but no TestMain to guard them; add main_test.go holding " + testMainFix,
				"unwatched/main_test.go:8: TestMain doesn't exit with what testguard.Main returns; make it " + testMainFix,
			},
		},
		{
			name: "environmentChanges",
			got:  environmentChanges(files),
			want: []string{
				"internal/testguard/guard_test.go:11: os.Setenv changes the process's environment, which only testguard may; " + environmentFix,
				"setenv/setenv.go:5: os.Setenv changes the process's environment, which only testguard may; " + environmentFix,
				"setenv/setenv_test.go:14: os.Setenv changes the process's environment, which only testguard may; " + environmentFix,
				"setenv/setenv_test.go:15: os.Unsetenv changes the process's environment, which only testguard may; " + environmentFix,
				"setenv/setenv_test.go:16: syscall.Clearenv changes the process's environment, which only testguard may; " + environmentFix,
			},
		},
		{
			name: "processStarts",
			got:  processStarts(files, []string{"starter/starter.go:runCommand", "starter/gone.go:runner"}),
			want: []string{
				"starter/starter.go:15: exec.Command starts a process outside the functions allowed to; " + processFix,
				"starter/starter.go:16: os.StartProcess starts a process outside the functions allowed to; " + processFix,
				"starter/starter.go:17: syscall.Exec starts a process outside the functions allowed to; " + processFix,
				"starter/starter.go:20: exec.Command starts a process outside the functions allowed to; " + processFix,
				"internal/testguard/source_test.go: processStarters allows starter/gone.go:runner, which starts no process; remove it",
			},
		},
		{
			name: "userImports",
			got:  userImports(files),
			want: []string{
				"user/user.go:4: os/user finds the home directory past HOME, where testguard can't move it; " + userFix,
			},
		},
	}
	for _, tt := range tests {
		if !slices.Equal(tt.got, tt.want) {
			t.Errorf("%s found\n%s\nwant\n%s", tt.name, strings.Join(tt.got, "\n"), strings.Join(tt.want, "\n"))
		}
	}
}

// fixture is a module with a package for each problem the guards look for,
// and some they must let be, by path from its root.
var fixture = map[string]string{
	"go.mod":             "module example.com/fixture\n",
	"plain/plain.go":     "package plain\n",
	"testdata/x_test.go": "package x\n",
	"vendor/x_test.go":   "package x\n",
	".hidden/x_test.go":  "package x\n",
	"_ignored/x_test.go": "package x\n",
	"guarded/main_test.go": `package guarded_test

import (
	"os"
	"testing"

	"github.com/leeovery/switchboard/internal/testguard"
)

func TestMain(m *testing.M) { os.Exit(testguard.Main(m)) }
`,
	"aliased/main_test.go": `package aliased

import (
	sys "os"
	"testing"

	guard "github.com/leeovery/switchboard/internal/testguard"
)

func TestMain(m *testing.M) { sys.Exit(guard.Main(m)) }
`,
	"unguarded/unguarded_test.go": `package unguarded

import "testing"

func TestSomething(t *testing.T) {}
`,
	"exits/main_test.go": `package exits_test

import (
	"os"
	"testing"

	"github.com/leeovery/switchboard/internal/testguard"
)

func TestMain(m *testing.M) {
	testguard.Main(m)
	os.Exit(0)
}
`,
	"unwatched/main_test.go": `package unwatched

import (
	"os"
	"testing"
)

func TestMain(m *testing.M) { os.Exit(m.Run()) }
`,
	"setenv/setenv.go": `package setenv

import "os"

func configure() { _ = os.Setenv("TZ", "UTC") }
`,
	"setenv/setenv_test.go": `package setenv

import (
	"os"
	"syscall"
	"testing"

	"github.com/leeovery/switchboard/internal/testguard"
)

func TestMain(m *testing.M) { os.Exit(testguard.Main(m)) }

func TestSomething(t *testing.T) {
	os.Setenv("SWITCHBOARD_CONFIG", "/nonexistent")
	defer os.Unsetenv("SWITCHBOARD_CONFIG")
	_ = syscall.Clearenv
	t.Setenv("SWITCHBOARD_LOG_LEVEL", "debug")
}
`,
	"internal/testguard/env.go": `package testguard

import "os"

func isolate() { _ = os.Setenv("HOME", "/nonexistent") }
`,
	"internal/testguard/guard_test.go": `package testguard_test

import (
	"os"
	"testing"

	"github.com/leeovery/switchboard/internal/testguard"
)

func TestMain(m *testing.M) { os.Exit(testguard.Main(m)) }
func TestSomething(t *testing.T) { _ = os.Setenv("HOME", "/nonexistent") }
`,
	"starter/starter.go": `package starter

import (
	"context"
	"os"
	"os/exec"
	"syscall"
)

func runCommand(ctx context.Context, name string) error {
	return exec.CommandContext(ctx, name).Run()
}

func unlisted() {
	_ = exec.Command("launchctl", "list").Run()
	_, _ = os.StartProcess("/usr/bin/open", nil, &os.ProcAttr{})
	_ = syscall.Exec("/usr/bin/osascript", nil, nil)
}

var run = exec.Command
`,
	"starter/starter_test.go": `package starter

import (
	"os"
	"os/exec"
	"testing"

	"github.com/leeovery/switchboard/internal/testguard"
)

func TestMain(m *testing.M) { os.Exit(testguard.Main(m)) }

func TestSomething(t *testing.T) { _ = exec.Command("true").Run() }
`,
	"user/user.go": `package user

import (
	osuser "os/user"
)

func home() string {
	u, err := osuser.Current()
	if err != nil {
		return ""
	}
	return u.HomeDir
}
`,
}

// unguardedPackages finds each package with tests whose TestMain doesn't run
// them through testguard.Main, or that has none.
func unguardedPackages(files []sourceFile) []string {
	tests := make(map[string][]sourceFile)
	for _, f := range files {
		if f.isTest() {
			tests[f.dir()] = append(tests[f.dir()], f)
		}
	}
	var problems []string
	for _, dir := range slices.Sorted(maps.Keys(tests)) {
		if problem := testMainProblem(tests[dir]); problem != "" {
			problems = append(problems, problem)
		}
	}
	return problems
}

// testMainProblem says what's wrong with the TestMain among a package's test
// files, or "" when nothing is.
func testMainProblem(files []sourceFile) string {
	for _, f := range files {
		for _, decl := range f.ast.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || fn.Name.Name != "TestMain" {
				continue
			}
			if exitsWithGuard(f, fn) {
				return ""
			}
			return fmt.Sprintf("%s: TestMain doesn't exit with what testguard.Main returns; make it %s", f.at(fn.Pos()), testMainFix)
		}
	}
	first := files[0]
	return fmt.Sprintf("%s: the package has tests but no TestMain to guard them; add main_test.go holding %s", first.at(first.ast.Package), testMainFix)
}

// exitsWithGuard reports whether fn calls os.Exit(testguard.Main(…)).
func exitsWithGuard(f sourceFile, fn *ast.FuncDecl) bool {
	if fn.Body == nil {
		return false
	}
	osName, guardName := f.importName("os"), f.importName(guardImport)
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok && !found && selects(call.Fun, osName, "Exit") && len(call.Args) == 1 {
			inner, ok := call.Args[0].(*ast.CallExpr)
			found = ok && selects(inner.Fun, guardName, "Main")
		}
		return !found
	})
	return found
}

// environmentChanges finds each reference, anywhere but testguard's own code,
// to a function that changes the process's environment.
func environmentChanges(files []sourceFile) []string {
	var problems []string
	for _, f := range files {
		if f.dir() == guardDir && !f.isTest() {
			continue
		}
		for _, u := range uses(f, f.ast, environmentChangers) {
			problems = append(problems, fmt.Sprintf("%s: %s changes the process's environment, which only testguard may; %s", f.at(u.pos), u.what, environmentFix))
		}
	}
	return problems
}

// processStarts finds each place production code starts a process outside the
// functions allowed to, and each function allowed that no longer does.
func processStarts(files []sourceFile, allowed []string) []string {
	var problems []string
	starting := make(map[string]bool)
	for _, f := range files {
		if f.isTest() {
			continue
		}
		for _, decl := range f.ast.Decls {
			name := f.path + ":" + funcName(decl)
			for _, u := range uses(f, decl, processStarting) {
				if slices.Contains(allowed, name) {
					starting[name] = true
					continue
				}
				problems = append(problems, fmt.Sprintf("%s: %s starts a process outside the functions allowed to; %s", f.at(u.pos), u.what, processFix))
			}
		}
	}
	for _, name := range allowed {
		if !starting[name] {
			problems = append(problems, fmt.Sprintf("%s: processStarters allows %s, which starts no process; remove it", allowListFile, name))
		}
	}
	return problems
}

// userImports finds each production file that imports os/user, which finds the
// home directory from the system rather than HOME, and so past testguard.
func userImports(files []sourceFile) []string {
	var problems []string
	for _, f := range files {
		if f.isTest() {
			continue
		}
		for _, spec := range f.ast.Imports {
			if p, err := strconv.Unquote(spec.Path.Value); err == nil && p == "os/user" {
				problems = append(problems, fmt.Sprintf("%s: os/user finds the home directory past HOME, where testguard can't move it; %s", f.at(spec.Pos()), userFix))
			}
		}
	}
	return problems
}

// funcName is the name of the function decl declares, or "" when it declares
// something else, which is never allowed to start a process.
func funcName(decl ast.Decl) string {
	if fn, ok := decl.(*ast.FuncDecl); ok {
		return fn.Name.Name
	}
	return ""
}

// use is a reference to a target function: where it is, and which, such as
// "os.Setenv".
type use struct {
	pos  token.Pos
	what string
}

// uses finds, in node of f, each reference to one of targets' functions, in
// the order they come.
func uses(f sourceFile, node ast.Node, targets []target) []use {
	pkgs := make([]string, len(targets))
	for i, tg := range targets {
		pkgs[i] = f.importName(tg.importPath)
	}
	var found []use
	ast.Inspect(node, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		for i, tg := range targets {
			if selects(sel, pkgs[i], tg.names...) {
				found = append(found, use{pos: sel.Pos(), what: path.Base(tg.importPath) + "." + sel.Sel.Name})
			}
		}
		return true
	})
	return found
}

// selects reports whether expr is one of names selected from pkg, the name a
// file imports a package by. No package is imported by "".
func selects(expr ast.Expr, pkg string, names ...string) bool {
	sel, ok := expr.(*ast.SelectorExpr)
	if !ok || pkg == "" {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && x.Name == pkg && slices.Contains(names, sel.Sel.Name)
}

// sourceFile is one of the module's Go files, parsed.
type sourceFile struct {
	// path is the file's path from the module's root, with slashes.
	path string
	fset *token.FileSet
	ast  *ast.File
}

func (f sourceFile) isTest() bool {
	return strings.HasSuffix(f.path, "_test.go")
}

func (f sourceFile) dir() string {
	return path.Dir(f.path)
}

// at is where pos is in the file: its path and line.
func (f sourceFile) at(pos token.Pos) string {
	return fmt.Sprintf("%s:%d", f.path, f.fset.Position(pos).Line)
}

// importName is the name the file refers to the package at importPath by, or
// "" when it doesn't import it by a name.
func (f sourceFile) importName(importPath string) string {
	for _, spec := range f.ast.Imports {
		if p, err := strconv.Unquote(spec.Path.Value); err != nil || p != importPath {
			continue
		}
		if spec.Name == nil {
			return path.Base(importPath)
		}
		if name := spec.Name.Name; name != "_" && name != "." {
			return name
		}
	}
	return ""
}

// parseModule parses every Go file of the module at root, but those in
// directories ./... skips too: testdata and vendor, and those whose names
// start with a dot or an underscore.
func parseModule(t *testing.T, root string) []sourceFile {
	t.Helper()
	fset := token.NewFileSet()
	var files []sourceFile
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if p != root && skipped(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(p, ".go") {
			return nil
		}
		file, err := parser.ParseFile(fset, p, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		files = append(files, sourceFile{path: filepath.ToSlash(rel), fset: fset, ast: file})
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

// writeModule writes files, by path from the root, into a new directory, and
// returns it.
func writeModule(t *testing.T, files map[string]string) string {
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

func reportEach(t *testing.T, problems []string) {
	t.Helper()
	for _, problem := range problems {
		t.Error(problem)
	}
}
