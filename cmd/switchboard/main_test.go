package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/switchboard/internal/testguard"
)

func TestMain(m *testing.M) { os.Exit(testguard.Main(m)) }

func TestTheDepsMainHandsRunFindTheProjectASessionRunsIn(t *testing.T) {
	repo := filepath.Join(t.TempDir(), "api")
	for _, dir := range []string{".git", "cmd"} {
		if err := os.MkdirAll(filepath.Join(repo, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	find := deps().FindProject
	if find == nil {
		t.Fatal("main hands run no FindProject: no session is told its project")
	}
	if got := find(filepath.Join(repo, "cmd")); got != repo {
		t.Errorf("FindProject(api/cmd) = %q, want the repository %q", got, repo)
	}
}
