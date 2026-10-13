package sourcecheck_test

import (
	"slices"
	"testing"

	"github.com/leeovery/switchboard/internal/sourcecheck"
)

func TestAModulesPackagesAreItsDirectoriesOfGoFiles(t *testing.T) {
	const source = "package p\n"
	root := sourcecheck.WriteModule(t, map[string]string{
		"go.mod":                          "module example.com/fixture\n",
		"main.go":                         source,
		"cmd/tool/main.go":                source,
		"internal/a/a.go":                 source,
		"internal/a/a_test.go":            source,
		"internal/a/b/b.go":               source,
		"internal/tested/tested_test.go":  source,
		"internal/notes/notes.md":         "",
		"internal/testdata/data.go":       source,
		"internal/_old/old.go":            source,
		"internal/.hidden/hidden.go":      source,
		"vendor/example.org/v/v.go":       source,
		"internal/a/testdata/fixture.go":  source,
		"internal/a/b/vendor/v/vendor.go": source,
	})
	got := sourcecheck.Packages(t, root)
	want := []string{".", "cmd/tool", "internal/a", "internal/a/b"}
	if !slices.Equal(got, want) {
		t.Errorf("Packages() = %q, want %q", got, want)
	}
}
