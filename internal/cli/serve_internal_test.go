package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/leeovery/switchboard/internal/config"
)

func TestTheRouterIsGivenTheFolderToExportInto(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte("[[account]]\nid = \"work\"\n\n[export]\ndir = \"~/Dropbox/Switchboard\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	a := &app{
		Getenv:  func(string) string { return "" },
		Environ: func() []string { return nil },
		HomeDir: func() (string, error) { return "/Users/test", nil },
	}

	cfg, _, err := a.startingFrom(t.TempDir(), path)
	if err != nil {
		t.Fatalf("startingFrom() error = %v", err)
	}
	if want := (config.Export{Dir: "~/Dropbox/Switchboard"}); cfg.Export != want {
		t.Errorf("the router exports as %+v, want %+v", cfg.Export, want)
	}
	if got, err := cfg.Export.Folder(cfg.HomeDir); err != nil || got != "/Users/test/Dropbox/Switchboard" {
		t.Errorf("the router finds the folder %q (%v), want it in the home directory given", got, err)
	}
}
