package views_test

import (
	"encoding/json"
	"fmt"
	"go/types"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/leeovery/switchboard/internal/sourcecheck"
	"github.com/leeovery/switchboard/internal/views"
)

// storedKey is a key of a stored form's JSON: the struct's, by its package's
// name, as "views.storedRequest", and the key its tag gives a field. A
// struct embedded untagged has its keys counted as its own, wherever it's
// embedded.
type storedKey struct {
	of, key string
}

func (k storedKey) String() string {
	return k.of + `'s "` + k.key + `"`
}

// storedAs are the types a digest's JSON holds in place of those its stored
// forms declare, by the name of the type declared, each in its own stored
// form, as its MarshalJSON writes it: a spend; a spent, a list of what it
// holds of each model and geo; a climb; and a window kept.
var storedAs = map[string]string{
	"ledger.Spend":      "ledger.storedSpend",
	"ledger.Spent":      "[]ledger.storedSpentGroup",
	"ledger.Climb":      "ledger.storedClimb",
	"claude.KeptWindow": "claude.storedWindow",
}

// rawAs are the types a digest's JSON holds in the fields its stored forms
// declare as JSON of another's, by the field: an entry's state, the views'
// body, a stored session; and what that carries of its page.
var rawAs = map[storedKey]string{
	{of: "ledger.storedEntry", key: "state"}: "views.storedSession",
	{of: "views.storedSession", key: "p"}:    "views.storedPage",
}

// unwritten are the keys a stored form declares that no digest holds, each
// with why: a golden holding one is told to drop it here.
var unwritten = map[storedKey]string{
	{of: "ledger.storedSpend", key: "q"}:    "a quota check is never counted, so the store never holds one",
	{of: "views.storedPage", key: "nn"}:     unpending,
	{of: "views.storedTurn", key: "pn"}:     unpending,
	{of: "views.storedTurn", key: "mp"}:     unpending,
	{of: "views.storedTimes", key: "f"}:     "a request went upstream in the years nanoseconds since the epoch hold, 1678 to 2262",
	{of: "ledger.storedReading", key: "rs"}: unsaid,
	{of: "ledger.storedReading", key: "p"}:  unsaid,
	{of: "ledger.storedReading", key: "as"}: unsaid,
	{of: "ledger.storedReading", key: "ap"}: unsaid,
}

// unpending is why a page's stored forms hold no request pending, and
// unsaid why a climb's holds no reading's restart, pace or allowance.
const (
	unpending = "a page sealed of its lines holds no request pending, which the stream alone tells of"
	unsaid    = "a page's points read their windows from an answer's headers, which never say it"
)

// Every key the stored forms of a day's digest and its parts declare, the
// ledger's envelope and every struct of the views' body it holds, nested,
// and of a session's part, holds a value in the golden of the version
// written, as TestADigestIsEncodedAsTheGoldenOfItsVersionsHoldsIt keeps it:
// a key the golden left out, as its zero, would be renamed or retyped
// without the golden failing. But those unwritten has, which it holds none
// of.
func TestTheDigestsGoldenHoldsEveryKeyOfItsStoredForms(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", fmt.Sprintf("digest-v%d.json", views.DigestVersion)))
	if err != nil {
		t.Fatal(err)
	}
	var golden struct {
		Digest any `json:"digest"`
		Parts  any `json:"parts"`
	}
	if err := json.Unmarshal(data, &golden); err != nil {
		t.Fatal(err)
	}
	root := sourcecheck.ModuleRoot(t)
	k := newStoredKeys(t, root, sourcecheck.ModulePath(t, root))
	k.walk(k.named("ledger.storedDigest"), golden.Digest)
	k.each(k.named("views.storedPart"), golden.Parts)
	for _, key := range slices.SortedFunc(maps.Keys(k.declared), func(a, b storedKey) int { return strings.Compare(a.String(), b.String()) }) {
		why, exempt := unwritten[key]
		switch held := k.held[key]; {
		case exempt && held:
			t.Errorf("the golden holds %s, which unwritten says no digest holds, as %s: remove it from unwritten", key, why)
		case !exempt && !held:
			t.Errorf("the golden holds no value of %s: give the golden world what stores one", key)
		}
	}
	for key := range unwritten {
		if !k.declared[key] {
			t.Errorf("unwritten names %s, which no stored form declares: remove it", key)
		}
	}
}

// storedKeys are the keys of a digest's stored forms, found by walking
// their types beside a digest's JSON: those declared, and those the JSON
// holds a value of.
type storedKeys struct {
	t        *testing.T
	pkgs     map[string]*types.Package
	declared map[storedKey]bool
	held     map[storedKey]bool
	walked   map[string]bool
}

// newStoredKeys are the stored keys of the ledger's, claude's and the views'
// types, of the module at root with the given path, none walked yet.
func newStoredKeys(t *testing.T, root, module string) *storedKeys {
	t.Helper()
	loader := sourcecheck.NewLoader(t, root, module, nil)
	pkgs := map[string]*types.Package{}
	for _, name := range []string{"ledger", "claude", "views"} {
		pkg, err := loader.Import(module + "/internal/" + name)
		if err != nil {
			t.Fatal(err)
		}
		pkgs[name] = pkg
	}
	return &storedKeys{t: t, pkgs: pkgs, declared: map[storedKey]bool{}, held: map[storedKey]bool{}, walked: map[string]bool{}}
}

// named is the type the ledger, claude or the views declare with the given
// name, by its package's, as "views.storedSession"; or a list of one, as
// "[]ledger.storedSpentGroup".
func (k *storedKeys) named(name string) types.Type {
	k.t.Helper()
	if elem, list := strings.CutPrefix(name, "[]"); list {
		return types.NewSlice(k.named(elem))
	}
	pkg, local, _ := strings.Cut(name, ".")
	obj, ok := k.pkgs[pkg].Scope().Lookup(local).(*types.TypeName)
	if !ok {
		k.t.Fatalf("no type %s is declared", name)
	}
	return obj.Type()
}

// nameOf is the name t goes by, where it's named or an alias, by its
// package's, as "views.storedSession" or "json.RawMessage": "" of one of no
// package.
func nameOf(t types.Type) string {
	var obj *types.TypeName
	switch t := t.(type) {
	case *types.Alias:
		obj = t.Obj()
	case *types.Named:
		obj = t.Obj()
	}
	if obj == nil || obj.Pkg() == nil {
		return ""
	}
	return obj.Pkg().Name() + "." + obj.Name()
}

// walk notes the keys t declares, and those v, t's JSON, holds a value of:
// t as storedAs stores it; nil where the JSON holds none.
func (k *storedKeys) walk(t types.Type, v any) {
	if stored := storedAs[nameOf(t)]; stored != "" {
		t = k.named(stored)
	}
	switch u := t.Underlying().(type) {
	case *types.Pointer:
		k.walk(u.Elem(), v)
	case *types.Slice:
		k.each(u.Elem(), v)
	case *types.Map:
		k.each(u.Elem(), v)
	case *types.Struct:
		obj, _ := v.(map[string]any)
		k.fields(nameOf(t), u, obj)
	}
}

// each walks each of v's elements, a JSON array or object's values, as elem:
// one without a value, where it holds none.
func (k *storedKeys) each(elem types.Type, v any) {
	var values []any
	switch v := v.(type) {
	case []any:
		values = v
	case map[string]any:
		values = slices.Collect(maps.Values(v))
	}
	if len(values) == 0 {
		values = []any{nil}
	}
	for _, e := range values {
		k.walk(elem, e)
	}
}

// fields notes the keys s, the struct named of, declares, and those obj, its
// JSON object, holds, walking each field beside its value: an untagged
// embedded struct's keys as its own, in obj, and an untagged field's key its
// name, as encoding/json gives it; and a field of JSON of another's as the
// type rawAs names, failing the test where it names none. A struct is walked
// without a value once.
func (k *storedKeys) fields(of string, s *types.Struct, obj map[string]any) {
	if obj == nil {
		if k.walked[of] {
			return
		}
		k.walked[of] = true
	}
	for i := range s.NumFields() {
		f := s.Field(i)
		key, _, _ := strings.Cut(reflect.StructTag(s.Tag(i)).Get("json"), ",")
		if f.Embedded() && key == "" {
			named, embedded := sourcecheck.StructOf(f.Type())
			k.fields(nameOf(named), embedded, obj)
			continue
		}
		if !f.Exported() || key == "-" {
			continue
		}
		if key == "" {
			key = f.Name()
		}
		at := storedKey{of: of, key: key}
		k.declared[at] = true
		value, held := obj[key]
		k.held[at] = k.held[at] || held
		k.walk(k.rawAs(at, f.Type()), value)
	}
}

// rawAs is t, the type of the field at, as rawAs has it where it's JSON of
// another's, failing the test where rawAs names none.
func (k *storedKeys) rawAs(at storedKey, t types.Type) types.Type {
	if nameOf(t) != "json.RawMessage" {
		return t
	}
	stored, ok := rawAs[at]
	if !ok {
		k.t.Errorf("%s is JSON of another's that rawAs names no stored form of", at)
		return t
	}
	return k.named(stored)
}
