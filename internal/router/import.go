package router

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync/atomic"

	"github.com/leeovery/switchboard/internal/dayfile"
	"github.com/leeovery/switchboard/internal/events"
	"github.com/leeovery/switchboard/internal/ledger"
	"github.com/leeovery/switchboard/internal/readings"
)

// ImportDir is where import stages the history it brings in, for the router
// whose state directory is stateDir to take in as POST /import asks: a
// folder for each Mac's folder staged, named 1, 2, … in the order staged,
// each laid out as a state directory lays out its history.
func ImportDir(stateDir string) string {
	return filepath.Join(stateDir, "import")
}

// Imported is what POST /import answers: how many folders staged it read; the
// days it took in, each date counted once across every kind of line and
// folder, as ImportedDays says; how many lines it added to days the state
// directory held; and how many summaries it copied in.
type Imported struct {
	Folders   int          `json:"folders"`
	Days      ImportedDays `json:"days"`
	Lines     int          `json:"lines"`
	Summaries int          `json:"summaries"`
}

// ImportedDays are the days an import took in, by how: merged, where any
// kind of line of the day was merged into a day the state directory held;
// else added, where any was added to one it held none of; else past keeping,
// where any was left out as past keeping. A day that took nothing in, as one
// staged of no lines, is none of them.
type ImportedDays struct {
	Added    int `json:"added"`
	Merged   int `json:"merged"`
	PastKeep int `json:"past_keep"`
}

var (
	// errImportRunning is the error of an import asked for while another
	// runs.
	errImportRunning = errors.New("an import is running already")
	// errNothingStaged is the error of an import with no folder staged.
	errNothingStaged = errors.New("there's nothing staged to import")
	// errImportStopped is the error of an import the router stopped partway.
	errImportStopped = errors.New("the router stopped before the import was done: what it took in stays, and importing again takes in the rest")
	// errHistoryUnkept is the error of an import asked of a router that
	// isn't keeping its history, as one not yet running.
	errHistoryUnkept = errors.New("the router isn't keeping its history yet")
)

// imports runs the router's imports, one at a time, each on the router's
// lifetime rather than its request's, so a client that goes leaves none
// partway, until the router stops, which cuts short any under way.
type imports struct {
	// ctx ends as the router stops.
	ctx    context.Context
	cancel context.CancelFunc
	// running is set while an import runs.
	running atomic.Bool
}

func newImports() *imports {
	ctx, cancel := context.WithCancel(context.Background())
	return &imports{ctx: ctx, cancel: cancel}
}

// run runs take on the imports' lifetime, and returns what it took in,
// failing with errImportRunning, running nothing, while another import runs,
// and with errImportStopped where take failed as the router stopped, as its
// writers stopping fails it.
func (i *imports) run(take func(ctx context.Context) (Imported, error)) (Imported, error) {
	if !i.running.CompareAndSwap(false, true) {
		return Imported{}, errImportRunning
	}
	defer i.running.Store(false)
	imported, err := take(i.ctx)
	if err != nil && (i.ctx.Err() != nil || errors.Is(err, dayfile.ErrStopped)) {
		return Imported{}, errImportStopped
	}
	return imported, err
}

// stop cuts short the import under way, and any asked for after.
func (i *imports) stop() {
	i.cancel()
}

// serveImport answers POST /import: it takes in what's staged, as
// importStaged does, one import at a time, and answers with what it took in,
// or why it didn't, as importStatus says, warning of a day it couldn't take
// in.
func (r *Router) serveImport(w http.ResponseWriter, _ *http.Request) {
	imported, err := r.imports.run(r.importStaged)
	if err == nil {
		writeJSON(w, imported)
		return
	}
	status := importStatus(err)
	if status == http.StatusInternalServerError {
		logger.Warn("can't import", "error", err)
	}
	writeProblem(w, status, err.Error())
}

// importStatus is the status POST /import answers err with: 409 while another
// import runs, 400 with nothing staged, 503 while the router can't import,
// as it stops, and 500 where a day couldn't be taken in.
func importStatus(err error) int {
	switch {
	case errors.Is(err, errImportRunning):
		return http.StatusConflict
	case errors.Is(err, errNothingStaged):
		return http.StatusBadRequest
	case errors.Is(err, errImportStopped), errors.Is(err, errHistoryUnkept):
		return http.StatusServiceUnavailable
	}
	return http.StatusInternalServerError
}

// importStaged takes in the folders staged in the state directory's import
// directory, as ImportDir says, in the order staged, as importFolder takes
// each in; then has the ledger make its round at once, so the days taken in
// are summarised and digested; then removes the import directory, every
// piece of work it handed a writer having returned, so none still reads it.
// It fails with errNothingStaged where the directory holds no folder, and as
// importFolder does, leaving the directory as it is, and what it took in
// before.
func (r *Router) importStaged(ctx context.Context) (Imported, error) {
	// The ledger's events are opened last of the history's files.
	if r.eventFiles == nil {
		return Imported{}, errHistoryUnkept
	}
	dir := ImportDir(r.cfg.StateDir)
	folders, err := stagedFolders(dir)
	switch {
	case err != nil:
		return Imported{}, fmt.Errorf("read what's staged to import: %w", err)
	case len(folders) == 0:
		return Imported{}, fmt.Errorf("%w: %s holds no folder to take in", errNothingStaged, dir)
	}
	took := importing{days: make(map[string]dayfile.Taken)}
	for _, folder := range folders {
		if err := r.importFolder(ctx, folder, &took); err != nil {
			return Imported{}, err
		}
	}
	if err := r.ledger.roundNow(ctx); err != nil {
		return Imported{}, fmt.Errorf("summarise the days imported: %w", err)
	}
	imported := took.imported(len(folders))
	logger.Info("imported", "folders", imported.Folders, "added", imported.Days.Added, "merged", imported.Days.Merged,
		"past_keep", imported.Days.PastKeep, "lines", imported.Lines, "summaries", imported.Summaries)
	if err := os.RemoveAll(dir); err != nil {
		logger.Warn("can't remove what was staged to import; the next import removes it", "dir", dir, "error", err)
	}
	return imported, nil
}

// importFolder takes in the folder staged at folder, adding what it took in
// to took: the readings history's days, then the router's events', each by
// its own writer, as importLines takes them in, then the request ledger's,
// as importLedger does. The readings go first, so a round made as the
// ledger's days come in, as the hourly one, summarises them with their
// readings.
func (r *Router) importFolder(ctx context.Context, folder string, took *importing) error {
	for _, staged := range []stagedLines{
		{files: readings.Files(readings.Dir(folder), logger), take: r.history.importDay},
		{files: events.Files(ledger.Dir(folder), logger), take: r.eventFiles.Import},
	} {
		if err := importLines(ctx, staged, took); err != nil {
			return err
		}
	}
	return r.importLedger(ctx, folder, took)
}

// stagedLines are the files of a kind of line staged in a folder, and what
// takes in a day of them, by its date, from where it's staged.
type stagedLines struct {
	files *dayfile.Files
	take  func(ctx context.Context, date, staged string) (dayfile.Imported, error)
}

// importLines takes in each day of the lines staged, as stagedDates finds
// them, oldest first, adding what it took in to took. It fails where the
// staged files can't be listed, and as take does.
func importLines(ctx context.Context, staged stagedLines, took *importing) error {
	dates, err := stagedDates(staged.files)
	if err != nil {
		return fmt.Errorf("read the %s staged to import: %w", staged.files.Name, err)
	}
	for _, date := range dates {
		imported, err := staged.take(ctx, date, staged.files.CompressedPath(date))
		if err != nil {
			return err
		}
		took.day(date, imported)
	}
	return nil
}

// importLedger takes in each of the request ledger's days staged in folder,
// as stagedLedger finds them, oldest first, its lines and its summary, as
// ledger.Ledger's Import takes them in, adding what it took in to took. It
// fails where they can't be listed, and as Import does.
func (r *Router) importLedger(ctx context.Context, folder string, took *importing) error {
	days, err := stagedLedger(ledger.Dir(folder))
	if err != nil {
		return fmt.Errorf("read the request ledger staged to import: %w", err)
	}
	for _, day := range days {
		imported, err := r.ledger.importDay(ctx, day)
		if err != nil {
			return err
		}
		took.day(day.Date, imported.Lines)
		if imported.Summary {
			took.summaries++
		}
	}
	return nil
}

// stagedFolders returns the folders staged in dir, named 1, 2, … in the order
// staged, in that order, passing over anything else: none where dir isn't
// there.
func stagedFolders(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, err
	}
	var staged []int
	for _, e := range entries {
		if n, ok := stagedNumber(e.Name()); ok && e.IsDir() {
			staged = append(staged, n)
		}
	}
	slices.Sort(staged)
	folders := make([]string, len(staged))
	for i, n := range staged {
		folders[i] = filepath.Join(dir, strconv.Itoa(n))
	}
	return folders, nil
}

// stagedNumber returns the number a folder staged with the given name was
// staged as, reporting false for a name that isn't one: a whole number from
// 1, written as it is.
func stagedNumber(name string) (int, bool) {
	n, err := strconv.Atoi(name)
	return n, err == nil && n > 0 && strconv.Itoa(n) == name
}

// stagedDates returns the dates of the days whose compressed files files
// holds, as export writes a day's, oldest first, passing over anything else,
// as a temporary file an export cut short left: none where their directory
// isn't there.
func stagedDates(files *dayfile.Files) ([]string, error) {
	entries, err := files.List()
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return nil, nil
	case err != nil:
		return nil, err
	}
	var dates []string
	for _, e := range entries {
		date, ok := files.DateOf(e.Name())
		if ok && e.Type().IsRegular() && e.Name() == filepath.Base(files.CompressedPath(date)) {
			dates = append(dates, date)
		}
	}
	slices.Sort(dates)
	return dates, nil
}

// stagedLedger returns the request ledger's days staged in dir, oldest first:
// each whose lines' file it holds, as stagedDates finds them, or whose
// summary, as a day past keeping where it was exported has its summary
// alone.
func stagedLedger(dir string) ([]ledger.StagedDay, error) {
	files := ledger.Files(dir, logger)
	lines, err := stagedDates(files)
	if err != nil {
		return nil, err
	}
	summaries, err := ledger.SummaryDates(dir)
	if err != nil {
		return nil, err
	}
	days := make(map[string]ledger.StagedDay)
	for _, date := range lines {
		day := days[date]
		day.Date, day.Lines = date, files.CompressedPath(date)
		days[date] = day
	}
	for _, date := range summaries {
		day := days[date]
		day.Date, day.Summary = date, ledger.SummaryFile(dir, date)
		days[date] = day
	}
	return slices.SortedFunc(maps.Values(days), func(a, b ledger.StagedDay) int { return cmp.Compare(a.Date, b.Date) }), nil
}

// importing is what an import has taken in so far: how each day was taken
// in, by its date, as the most it was taken as of any kind of line or
// folder, as taking ranks them; the lines added to days held; and the
// summaries copied in.
type importing struct {
	days      map[string]dayfile.Taken
	lines     int
	summaries int
}

// day notes the day with the given date taken in as imported says.
func (i *importing) day(date string, imported dayfile.Imported) {
	if taking(imported.Taken) > taking(i.days[date]) {
		i.days[date] = imported.Taken
	}
	i.lines += imported.Lines
}

// taking ranks how a day was taken in, as ImportedDays counts a day taken in
// more than once: merged over added, added over past keeping, and each over
// a day that took nothing in.
func taking(t dayfile.Taken) int {
	switch t {
	case dayfile.Merged:
		return 3
	case dayfile.Added:
		return 2
	case dayfile.PastKeep:
		return 1
	}
	return 0
}

// imported is what the import of the given number of folders took in.
func (i *importing) imported(folders int) Imported {
	counted := make(map[dayfile.Taken]int)
	for _, taken := range i.days {
		counted[taken]++
	}
	return Imported{
		Folders:   folders,
		Days:      ImportedDays{Added: counted[dayfile.Added], Merged: counted[dayfile.Merged], PastKeep: counted[dayfile.PastKeep]},
		Lines:     i.lines,
		Summaries: i.summaries,
	}
}
