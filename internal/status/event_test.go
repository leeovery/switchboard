package status_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/status"
)

// An event is the same as another only where every field tells the same,
// times as the same instants in whatever zone: each field changed alone
// makes another, so a field added to Event and left out of Same fails here.
func TestAnEventIsTheSameOnlyWhereEveryFieldTellsTheSame(t *testing.T) {
	at := time.Date(2026, 10, 1, 14, 0, 0, 0, time.UTC)
	e := status.Event{ID: 1, At: at, Kind: status.EventCap, Joined: map[string]time.Time{"work": at}}
	zoned := e
	zone := time.FixedZone("BST", 3600)
	zoned.At, zoned.Joined = at.In(zone), map[string]time.Time{"work": at.In(zone)}
	if !e.Same(zoned) {
		t.Error("the same event in another zone isn't the same, want it the same")
	}
	typ := reflect.TypeFor[status.Event]()
	for i := range typ.NumField() {
		field := typ.Field(i)
		t.Run(field.Name, func(t *testing.T) {
			other := e
			changed(t, reflect.ValueOf(&other).Elem().Field(i), at.Add(time.Second))
			if e.Same(other) {
				t.Errorf("an event with its %s changed is the same, want it another", field.Name)
			}
		})
	}
}

// changed sets v, a field of an Event, to a value other than its own: at for
// a time, and alike for the rest by their kinds.
func changed(t *testing.T, v reflect.Value, at time.Time) {
	t.Helper()
	switch v.Type() {
	case reflect.TypeFor[time.Time]():
		v.Set(reflect.ValueOf(at))
		return
	case reflect.TypeFor[[]string]():
		v.Set(reflect.ValueOf([]string{"side"}))
		return
	case reflect.TypeFor[map[string]time.Time]():
		v.Set(reflect.ValueOf(map[string]time.Time{"work": at}))
		return
	}
	switch v.Kind() {
	case reflect.Int:
		v.SetInt(v.Int() + 2)
	case reflect.String:
		v.SetString("other")
	case reflect.Float64:
		v.SetFloat(0.5)
	case reflect.Bool:
		v.SetBool(true)
	default:
		t.Fatalf("no change made to a field of type %s", v.Type())
	}
}
