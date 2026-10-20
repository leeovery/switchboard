package views_test

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/switchboard/internal/sourcecheck"
	"github.com/leeovery/switchboard/internal/views"
)

// Every key the stored forms of a chunk of a day's rows declare, each row's
// and its meta's, nested, holds a value in the golden of the version
// written, as TestAChunkIsEncodedAsTheGoldenOfItsVersionHoldsIt keeps it: a
// key the golden left out, as its zero, would be renamed or retyped without
// the golden failing.
func TestTheChunksGoldenHoldsEveryKeyOfItsStoredForms(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("testdata", fmt.Sprintf("rows-chunk-v%d.json", views.RowsVersion)))
	if err != nil {
		t.Fatal(err)
	}
	var golden struct {
		Rows any `json:"rows"`
		Meta any `json:"meta"`
	}
	if err := json.Unmarshal(data, &golden); err != nil {
		t.Fatal(err)
	}
	root := sourcecheck.ModuleRoot(t)
	k := newStoredKeys(t, root, sourcecheck.ModulePath(t, root))
	k.each(k.named("views.storedRow"), golden.Rows)
	k.walk(k.named("views.storedChunkMeta"), golden.Meta)
	k.everyHeld(nil)
}
