package watch

import (
	"cmp"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"unsafe"
)

// heldDepth is how far into the model's fields heldOf breaks what they hold
// down, and heldShown how many of the biggest it names.
const (
	heldDepth = 3
	heldShown = 14
)

// heldOf names what the model holds, the biggest first: the bytes of the
// heap each of its fields, and their fields in turn, heldDepth deep, reaches
// that none before it does, as reach counts them, and the total. The
// harness's own fakes are passed over.
func heldOf(m Model) string {
	r := reach{seen: make(map[uintptr]int64), walked: make(map[uintptr]int), pointers: make(map[reflect.Type]bool)}
	var held []heldPath
	r.breakdown("", reflect.ValueOf(m), 0, &held)
	slices.SortFunc(held, func(a, b heldPath) int { return cmp.Compare(b.bytes, a.bytes) })
	var total int64
	for _, h := range held {
		total += h.bytes
	}
	words := []string{fmt.Sprintf("%.1f MB in all", float64(total)/(1<<20))}
	for _, h := range held[:min(heldShown, len(held))] {
		words = append(words, fmt.Sprintf("%s %.1f MB", h.path, float64(h.bytes)/(1<<20)))
	}
	return strings.Join(words, ", ")
}

// heldPath is a field of the model's, by its path, and the bytes it reaches.
type heldPath struct {
	path  string
	bytes int64
}

// reach counts the bytes of the heap values reach, each once: seen are the
// bytes counted of what each pointer, slice's array, map and string's bytes
// leads to, by address, the most of a pointer's targets where one leads to
// the start of another's, as a pointer to a struct's first field does;
// walked how far into each slice's array its elements have been walked; and
// pointers whether a type holds any pointer, so a value of one that holds
// none is counted without being walked.
type reach struct {
	seen     map[uintptr]int64
	walked   map[uintptr]int
	pointers map[reflect.Type]bool
	bytes    int64
}

// counts counts n bytes at the address p, reporting whether they're more
// than were counted there before, as only the more are counted.
func (r *reach) counts(p uintptr, n int64) bool {
	if r.seen[p] >= n {
		return false
	}
	r.bytes += n - r.seen[p]
	r.seen[p] = n
	return true
}

// breakdown counts what v reaches under path, a field at a time where it's
// a struct, or leads to one, short of heldDepth, adding each field it
// counts whole to held.
func (r *reach) breakdown(path string, v reflect.Value, depth int, held *[]heldPath) {
	if skipped(v) {
		return
	}
	s := v
	for s.Kind() == reflect.Pointer || s.Kind() == reflect.Interface {
		if s.IsNil() {
			return
		}
		if s.Kind() == reflect.Pointer && !r.counts(s.Pointer(), int64(s.Type().Elem().Size())) {
			return
		}
		s = s.Elem()
	}
	if s.Kind() != reflect.Struct || depth == heldDepth {
		before := r.bytes
		r.value(v)
		*held = append(*held, heldPath{path: path, bytes: r.bytes - before})
		return
	}
	for field, value := range s.Fields() {
		r.breakdown(strings.TrimPrefix(path+"."+field.Name, "."), value, depth+1, held)
	}
}

// skipped reports whether v is one of the harness's own fakes, which the
// dashboard doesn't hold.
func skipped(v reflect.Value) bool {
	switch v.Type() {
	case reflect.TypeFor[*fakeSource](), reflect.TypeFor[*fakeNotifier]():
		return true
	}
	if v.Kind() == reflect.Interface && !v.IsNil() {
		return skipped(v.Elem())
	}
	return false
}

// value counts what v reaches, each pointer's target, slice's array, map
// and string's bytes once: funcs and channels, which reflection can't see
// into, count nothing.
func (r *reach) value(v reflect.Value) {
	if skipped(v) {
		return
	}
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() || !r.counts(v.Pointer(), int64(v.Type().Elem().Size())) {
			return
		}
		r.value(v.Elem())
	case reflect.Interface:
		if v.IsNil() {
			return
		}
		if e := v.Elem(); e.Kind() != reflect.Pointer {
			r.bytes += int64(e.Type().Size())
		}
		r.value(v.Elem())
	case reflect.Struct:
		if !r.holdsPointers(v.Type()) {
			return
		}
		for _, field := range v.Fields() {
			r.value(field)
		}
	case reflect.Array:
		if !r.holdsPointers(v.Type()) {
			return
		}
		for i := range v.Len() {
			r.value(v.Index(i))
		}
	case reflect.Slice:
		r.slice(v)
	case reflect.Map:
		r.mapped(v)
	case reflect.String:
		r.text(v.String())
	}
}

// slice counts the array of the slice v, once, and walks its elements, each
// once, as far as v reaches into it.
func (r *reach) slice(v reflect.Value) {
	if v.IsNil() || v.Cap() == 0 {
		return
	}
	p := v.Pointer()
	r.counts(p, int64(v.Cap())*int64(v.Type().Elem().Size()))
	if !r.holdsPointers(v.Type().Elem()) {
		return
	}
	from := r.walked[p]
	if from >= v.Len() {
		return
	}
	r.walked[p] = v.Len()
	for i := from; i < v.Len(); i++ {
		r.value(v.Index(i))
	}
}

// mapped counts the map v, once, as its keys and values take a slot each of
// a table filled seven parts in eight, and walks them.
func (r *reach) mapped(v reflect.Value) {
	t := v.Type()
	if v.IsNil() || !r.counts(v.Pointer(), int64(v.Len())*int64(t.Key().Size()+t.Elem().Size()+1)*8/7) {
		return
	}
	if !r.holdsPointers(t.Key()) && !r.holdsPointers(t.Elem()) {
		return
	}
	for it := v.MapRange(); it.Next(); {
		r.value(it.Key())
		r.value(it.Value())
	}
}

// text counts the bytes of s, once for every string sharing them.
func (r *reach) text(s string) {
	if s == "" {
		return
	}
	r.counts(uintptr(unsafe.Pointer(unsafe.StringData(s))), int64(len(s)))
}

// holdsPointers reports whether a value of the type t holds a pointer of any
// kind, a string's and a slice's among them.
func (r *reach) holdsPointers(t reflect.Type) bool {
	if holds, ok := r.pointers[t]; ok {
		return holds
	}
	r.pointers[t] = true
	holds := true
	switch t.Kind() {
	case reflect.Array:
		holds = r.holdsPointers(t.Elem())
	case reflect.Struct:
		holds = false
		for field := range t.Fields() {
			holds = holds || r.holdsPointers(field.Type)
		}
	case reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16,
		reflect.Uint32, reflect.Uint64, reflect.Uintptr, reflect.Float32, reflect.Float64, reflect.Complex64, reflect.Complex128:
		holds = false
	}
	r.pointers[t] = holds
	return holds
}
