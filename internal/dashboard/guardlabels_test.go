package dashboard_test

import (
	"testing"

	"github.com/leeovery/switchboard/internal/sourcecheck"
)

// labelsOwner is the file that owns a column of accounts' names: its least
// and its most, which no other file of the dashboard names.
const labelsOwner = "internal/dashboard/labelcolumn.go"

func labelBoundSays(name string) string {
	return name + " names a column of names' bound; take the column, its least or a sentence's cut from " + labelsOwner
}

// No file of the dashboard's but labelsOwner names a column of names' least
// or its most, so every column of names takes its bounds from one owner.
func TestOnlyTheOwnerNamesAColumnOfNamesBounds(t *testing.T) {
	root := sourcecheck.ModuleRoot(t)
	module := sourcecheck.ModulePath(t, root)
	used := map[string]sourcecheck.Ban{
		"dashboard.labelLeast": {Says: labelBoundSays("dashboard.labelLeast")},
		"dashboard.labelMost":  {Says: labelBoundSays("dashboard.labelMost")},
	}
	sourcecheck.ReportEach(t, sourcecheck.Check(t, root, module, sourcecheck.Rule{Names: sourcecheck.NamesIn(module), Used: used}, drawingPackages, sourcecheck.Allowance{
		Entries: []string{labelsOwner}, List: "labelsOwner", Fix: "keep labelsOwner the file that owns a column of names",
	}))
}
