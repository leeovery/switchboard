package export

import (
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"strings"

	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
)

// Undownloaded returns how many of the files staging takes of the Mac's
// folder at folder iCloud Drive hasn't downloaded yet: each stands in the
// folder as a placeholder of its own, .<name>.icloud, which staging passes
// over, as it isn't the file. It counts only those whose name is one staging
// takes, and fails where a directory of the folder's history can't be
// listed.
func Undownloaded(folder string) (int, error) {
	n := 0
	for _, dir := range []string{ledger.Dir(folder), readings.Dir(folder)} {
		entries, err := os.ReadDir(dir)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			continue
		case err != nil:
			return 0, fmt.Errorf("read %s: %w", dir, err)
		}
		for _, e := range entries {
			if name, ok := placeholderOf(e.Name()); ok && takes(folder, dir, name) {
				n++
			}
		}
	}
	return n, nil
}

// placeholderOf returns the name of the file the iCloud Drive placeholder
// with the given name stands for, reporting false for a name that isn't one.
func placeholderOf(name string) (string, bool) {
	inner, dotted := strings.CutPrefix(name, ".")
	inner, placeholder := strings.CutSuffix(inner, ".icloud")
	return inner, dotted && placeholder && inner != ""
}

// takes reports whether staging takes the file with the given name in dir, a
// directory of the Mac's folder at folder: a day's compressed file of a kind
// of line the directory holds, or, in the request ledger's, a day's summary.
func takes(folder, dir, name string) bool {
	for _, files := range linesIn(folder, slog.New(slog.DiscardHandler)) {
		if _, ok := files.CompressedDate(name); ok && files.Dir == dir {
			return true
		}
	}
	_, ok := ledger.SummaryDate(name)
	return ok && dir == ledger.Dir(folder)
}
