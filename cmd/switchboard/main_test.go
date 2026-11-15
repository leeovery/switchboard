package main

import (
	"os"
	"path/filepath"
	"runtime"
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
	if got, inRepository := find(filepath.Join(repo, "cmd")); got != repo || !inRepository {
		t.Errorf("FindProject(api/cmd) = %q, %v; want the repository %q", got, inRepository, repo)
	}
}

func TestTheDepsMainHandsRunFindTheProcessesASessionRunsIn(t *testing.T) {
	find := deps().FindProcess
	if find == nil {
		t.Fatal("main hands run no FindProcess: no claude is found within the session that started it")
	}
	if runtime.GOOS != "darwin" {
		t.Skip("only macOS's processes are read")
	}
	if p, ok := find(os.Getpid()); !ok || p.Parent != os.Getppid() {
		t.Errorf("FindProcess(this test) = %+v, %v; want it running, started by %d", p, ok, os.Getppid())
	}
}
