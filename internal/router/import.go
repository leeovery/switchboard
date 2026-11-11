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
	"github.com/leeovery/switchboard/internal/export"
	"github.com/leeovery/switchboard/internal/identity"
	"github.com/leeovery/switchboard/internal/idmove"
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
// kind of line of the day added lines to a day the state directory held;
// else added, where any was added to one it held none of; else past keeping,
// where any was left out as past keeping. A day that took nothing in, as one
// staged of no lines, or one held that gained none, is none of them: an
// import repeated tells of no day added or merged, and of each past keeping,
// left out again.
type ImportedDays struct {
	Added    int `json:"added"`
	Merged   int `json:"merged"`
	PastKeep int `json:"past_keep"`
}

// importRunning is what GET /import answers: whether an import is running.
type importRunning struct {
	Running bool `json:"running"`
}

var (
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
	// between, where it's set, is called once each folder's readings and
	// events are taken in, before its ledger's days are: a round may come
	// there, as tests have one.
	between func()
}

func newImports() *imports {
	ctx, cancel := context.WithCancel(context.Background())
	return &imports{ctx: ctx, cancel: cancel}
}

// run runs take on the imports' lifetime, and returns what it took in,
// failing with export.ErrImportRunning, running nothing, while another
// import runs, and with errImportStopped where take failed as the router
// stopped, as its writers stopping fails it.
func (i *imports) run(take func(ctx context.Context) (Imported, error)) (Imported, error) {
	if !i.running.CompareAndSwap(false, true) {
		return Imported{}, export.ErrImportRunning
	}
	defer i.running.Store(false)
	imported, err := take(i.ctx)
	if err != nil && i.stopped(err) {
		return Imported{}, errImportStopped
	}
	return imported, err
}

// stopped reports whether err, an import's, is of the router stopping: its
// lifetime ended, or a writer stopped. Work it handed a writer may then
// still be under way.
func (i *imports) stopped(err error) bool {
	return i.ctx.Err() != nil || errors.Is(err, dayfile.ErrStopped)
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

// serveImportRunning answers GET /import: whether an import is running, so a
// command about to stage one can refuse rather than stage over the files one
// still reads.
func (r *Router) serveImportRunning(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, importRunning{Running: r.imports.running.Load()})
}

// importStatus is the status POST /import answers err with: 409 while another
// import runs, and where an accounts record staged disagrees with the Mac's,
// 422 where one doesn't read, 400 with nothing staged, 503 while the router
// can't import, as it stops, and 500 where a day couldn't be taken in.
func importStatus(err error) int {
	switch {
	case errors.Is(err, export.ErrImportRunning), errors.Is(err, identity.ErrConflict):
		return http.StatusConflict
	case errors.Is(err, idmove.ErrStagedRecord):
		return http.StatusUnprocessableEntity
	case errors.Is(err, errNothingStaged):
		return http.StatusBadRequest
	case errors.Is(err, errImportStopped), errors.Is(err, errHistoryUnkept):
		return http.StatusServiceUnavailable
	}
	return http.StatusInternalServerError
}

// importStaged takes in the folders staged in the state directory's import
// directory, as ImportDir says, as takeStaged does; then removes the import
// directory, done with or failed, unless the router stopped: every piece of
// work it handed a writer has then returned, so none still reads it, and a
// failed import's copy, as big as the export, never stays to fill the disk.
// It fails as takeStaged does, keeping what it took in before.
func (r *Router) importStaged(ctx context.Context) (Imported, error) {
	// The ledger's events are opened last of the history's files.
	if r.eventFiles == nil {
		return Imported{}, errHistoryUnkept
	}
	dir := ImportDir(r.cfg.StateDir)
	imported, err := r.takeStaged(ctx, dir)
	if err == nil || !r.imports.stopped(err) {
		if err := os.RemoveAll(dir); err != nil {
			logger.Warn("can't remove what was staged to import; the next import removes it", "dir", dir, "error", err)
		}
	}
	return imported, err
}

// takeStaged takes in the folders staged in dir, in the order staged, as
// importFolder takes each in: once their accounts records are joined to the
// state directory's, as joinStaged joins them, each moved onto the ids the
// record then names, as idmove.RenameFolder renames it, before it's taken in, so
// a line staged under an account's old id is the line the Mac holds under its
// new; then has the ledger make its round at once, so
// the days taken in are summarised: each first summary, and eight of those
// summarised again, as days whose lines merged or of an older version, before
// it returns, the rest in the rounds the ledger makes at once after, and the
// days digested once none is left. It fails with
// errNothingStaged where dir holds no folder, and as importFolder does.
func (r *Router) takeStaged(ctx context.Context, dir string) (Imported, error) {
	folders, err := stagedFolders(dir)
	switch {
	case err != nil:
		return Imported{}, fmt.Errorf("read what's staged to import: %w", err)
	case len(folders) == 0:
		return Imported{}, fmt.Errorf("%w: %s holds no folder to take in", errNothingStaged, dir)
	}
	renames, err := r.joinStaged(folders)
	if err != nil {
		return Imported{}, err
	}
	took := importing{days: make(map[string]dayfile.Taken)}
	for i, folder := range folders {
		if err := idmove.RenameFolder(ctx, folder, renames[i]); err != nil {
			return Imported{}, fmt.Errorf("move what's staged to import onto the accounts' ids: %w", err)
		}
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
	return imported, nil
}

// joinStaged joins the accounts each of folders names to the state
// directory's accounts record, in the order staged, as idmove's JoinFolder
// joins them, writing the record where that changed it, and returns how each
// folder is renamed onto its ids, in the same order. It fails, writing
// nothing, as JoinFolder does: with idmove.ErrStagedRecord wrapped where a
// folder's record can't be read, and identity.ErrConflict where one
// contradicts the state directory's.
func (r *Router) joinStaged(folders []string) ([]idmove.Renames, error) {
	renames := make([]idmove.Renames, len(folders))
	err := identity.NewFile(r.cfg.StateDir).Change(func(record *identity.Record, _ func() error) error {
		for i, folder := range folders {
			rename, err := idmove.JoinFolder(folder, record, nil, r.cfg.Now())
			if err != nil {
				return fmt.Errorf("nothing was imported, as %w", err)
			}
			renames[i] = rename
		}
		return nil
	})
	return renames, err
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
	if r.imports.between != nil {
		r.imports.between()
	}
	return r.importLedger(ctx, folder, took)
}

// stagedLines are the files of a kind of line staged in a folder, and what
// takes in a day of them, by its date, from where it's staged.
type stagedLines struct {
	files *dayfile.Files
	take  func(ctx context.Context, date, staged string) (dayfile.Imported, error)
}

// importLines takes in each day of the lines staged, as Files' Compressed
// finds them, oldest first, adding what it took in to took. It fails where
// the staged files can't be listed, and as take does.
func importLines(ctx context.Context, staged stagedLines, took *importing) error {
	dates, err := staged.files.Compressed()
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

// stagedLedger returns the request ledger's days staged in dir, oldest first:
// each whose lines' file it holds, as Files' Compressed finds them, or whose
// summary, as ledger.SummaryDates finds them, as a day past keeping where it
// was exported has its summary alone.
func stagedLedger(dir string) ([]ledger.StagedDay, error) {
	files := ledger.Files(dir, logger)
	lines, err := files.Compressed()
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

// day notes the day with the given date taken in as imported says, a day
// merged that gained no line as one that took nothing in.
func (i *importing) day(date string, imported dayfile.Imported) {
	taken := imported.Taken
	if taken == dayfile.Merged && imported.Lines == 0 {
		taken = 0
	}
	if taking(taken) > taking(i.days[date]) {
		i.days[date] = taken
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
