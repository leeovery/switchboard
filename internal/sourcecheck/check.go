package sourcecheck

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Allowance is what a guard lets break its rule, and how it names what it
// lets, for an allowance that no longer stands to say what to remove.
type Allowance struct {
	// Entries are the files allowed to break the rule, by path from the
	// module's root, and the functions and package-level variables and
	// constants, by their file's path, a colon and their name, as NameAt
	// names it, as "internal/cli/requests.go:requestRow".
	Entries []string
	// List names the list of Entries, as "readsAllowed", and Fix says how to
	// fix one that's gone, or keeps the rule.
	List, Fix string
	// Calls names the list of the calls the rule allows, as "callsAllowed",
	// Callers those who'd make them, as "view", and CallFix says how to fix
	// one none of them makes.
	Calls, Callers, CallFix string
}

// Check finds where the non-test files of the packages in dirs, of the module
// at root with the given path, break the rule r, but where a allows: each
// breach, by where it is and how it breaks the rule. Then each of a's entries
// that's gone, or keeps the rule, and each call the rule allows that none of
// the rest makes.
func Check(t *testing.T, root, module string, r Rule, dirs []string, a Allowance) []string {
	t.Helper()
	return NewLoader(t, root, module, dirs).Check(r, dirs, a)
}

// Check is Check of the packages in dirs as l loads them, so a rule judges
// the very objects a guard worked something out of with l beforehand.
func (l *Loader) Check(r Rule, dirs []string, a Allowance) []string {
	t, root := l.t, l.root
	t.Helper()
	var problems []string
	broken, declared, called := make(map[string]bool), make(map[string]bool), make(map[string]bool)
	for _, dir := range dirs {
		for _, f := range l.Load(dir) {
			for _, decl := range f.AST.Decls {
				for _, name := range DeclNames(decl) {
					declared[f.Path+":"+name] = true
				}
			}
			found, calls := r.Breaches(f)
			for _, b := range found {
				if entry, ok := a.entryOf(f.Path, b.In); ok {
					broken[entry] = true
					continue
				}
				problems = append(problems, f.At(b.At)+": "+b.Says)
			}
			for _, c := range calls {
				if _, ok := a.entryOf(f.Path, c.In); !ok {
					called[c.Name] = true
				}
			}
		}
	}
	for _, entry := range a.Entries {
		switch {
		case !exists(t, root, entry, declared):
			problems = append(problems, entry+": "+a.List+" allows it, but it's gone; "+a.Fix)
		case !broken[entry]:
			problems = append(problems, entry+": "+a.List+" allows it, but it keeps the rule; "+a.Fix)
		}
	}
	for _, call := range r.Allowed {
		if !called[call] {
			problems = append(problems, call+": "+a.Calls+" allows it, but no "+a.Callers+" calls it; "+a.CallFix)
		}
	}
	return problems
}

// entryOf is the entry that allows what's in the declaration named in, of the
// file at path: the file's, else the function's, reporting false where none
// does.
func (a Allowance) entryOf(path, in string) (string, bool) {
	if slices.Contains(a.Entries, path) {
		return path, true
	}
	if fn := path + ":" + in; in != "" && slices.Contains(a.Entries, fn) {
		return fn, true
	}
	return "", false
}

// exists reports whether the entry is there: a file, under the module's root;
// a function, a variable or a constant, among those declared.
func exists(t *testing.T, root, entry string, declared map[string]bool) bool {
	t.Helper()
	if strings.Contains(entry, ":") {
		return declared[entry]
	}
	switch _, err := os.Stat(filepath.Join(root, filepath.FromSlash(entry))); {
	case errors.Is(err, os.ErrNotExist):
		return false
	case err != nil:
		t.Fatal(err)
	}
	return true
}
