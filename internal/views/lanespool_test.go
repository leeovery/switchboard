package views_test

import (
	"testing"

	"github.com/leeovery/switchboard/internal/views"
)

// The pool is at its limit in a column where none of its accounts has room,
// of those whose state is known; one where none's is known isn't, nor one
// where any has room. The pool's lane is at its limit where any column is.
func TestThePoolIsAtItsLimitWhereNoneOfItsKnownAccountsHasRoom(t *testing.T) {
	tests := []struct {
		name    string
		columns []views.PoolColumn
		want    bool
	}{
		{name: "none with room, known", columns: []views.PoolColumn{{With: 2, Known: true}, {Known: true}}, want: true},
		{name: "one with room", columns: []views.PoolColumn{{With: 1, Known: true}}},
		{name: "none known", columns: []views.PoolColumn{{}}},
		{name: "no columns"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := (views.Pool{Of: 2, Columns: tt.columns}).Limited(); got != tt.want {
				t.Errorf("the pool is at its limit %t, want %t", got, tt.want)
			}
		})
	}
}
