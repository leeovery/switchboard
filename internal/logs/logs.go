// Package logs is switchboard's logging, and the one place that decides where
// records go, how they read and what they must never show. A package logs
// through a logger For gives it, held at package level:
//
//	var logger = logs.For("status")
//
// Until Init, records go nowhere. From Init to Close they go to the log file
// of the part the process plays, one line each, with anything shaped like a
// token hidden.
package logs

import (
	"cmp"
	"context"
	"io"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"time"
)

// Role is the part a process plays, which picks the file it logs to.
type Role string

const (
	// RoleRouter is the proxy: one long-running process, logging to router.log.
	RoleRouter Role = "router"
	// RoleCLI is every other command, all of them sharing cli.log.
	RoleCLI Role = "cli"
)

// Path is where the role's log lives in dir.
func (r Role) Path(dir string) string {
	return filepath.Join(dir, string(r)+".log")
}

// noteLevel is the level a process in the role notes its start and exit at.
// A statusline can run a command every few seconds, so a CLI command notes
// them only at debug.
func (r Role) noteLevel() slog.Level {
	if r == RoleRouter {
		return slog.LevelInfo
	}
	return slog.LevelDebug
}

// Dir is where the logs live in switchboard's state directory.
func Dir(stateDir string) string {
	return filepath.Join(stateDir, "logs")
}

// Options say how a process logs.
type Options struct {
	// Dir is the directory the log files live in: see Dir. Without an
	// absolute one, records go to no file.
	Dir string
	// Role is the part the process plays. Empty means RoleCLI.
	Role Role
	// Level is the least severe level logged. Nil means the one
	// SWITCHBOARD_LOG_LEVEL names, else info.
	Level slog.Leveler
	// Getenv reads SWITCHBOARD_LOG_LEVEL.
	Getenv func(key string) string
	// Mirror, when set, is written every record as well as the file, and
	// still is when the file can't be: the terminal the router runs in.
	Mirror io.Writer
	// Version is switchboard's, and Command the command path run, such as
	// "switchboard status", for the start record. Never the arguments: run
	// passes Claude Code's through, and they can hold a prompt.
	Version string
	Command string
}

// pid is on every line, since several processes can share a file.
var pid = os.Getpid()

// current is where records go, from Init to Close. While it's nil they go
// nowhere.
var current atomic.Pointer[session]

var (
	// process notes the process's start and exit.
	process = For("process")
	// logger notes trouble with logging itself.
	logger = For("logs")
)

// session is a process's logging, from Init to Close.
type session struct {
	handler slog.Handler
	role    Role
	started time.Time
	// file is nil when it couldn't be opened.
	file *logFile
}

func (s *session) close() {
	if s.file != nil {
		_ = s.file.Close()
	}
}

// For returns the logger a component logs through, which names the component
// on every line. It can be called at any time, and is meant to be called
// once, at package level: its records go wherever logging sends them at the
// time they're logged.
func For(component string) *slog.Logger {
	return slog.New(forwarder{}).With("component", component, "pid", pid)
}

// StdLogger returns a standard library logger whose every line is logged as
// a record of component's at level, for an API that takes one, such as
// http.Server.ErrorLog.
func StdLogger(component string, level slog.Level) *log.Logger {
	return slog.NewLogLogger(For(component).Handler(), level)
}

// Init starts logging for the process. From here on, records from every
// logger For has given or will give go to the role's file in opts.Dir, and to
// opts.Mirror; the first notes the start. Init never fails: when the file
// can't be opened, records go only to the mirror, if there is one. Another
// Init replaces this one without noting an exit.
func Init(opts Options) {
	role := cmp.Or(opts.Role, RoleCLI)
	level, unknown := opts.level()
	file, err := openFile(opts.Dir, role)
	s := &session{handler: newHandler(output(file, opts.Mirror), level), role: role, started: time.Now(), file: file}
	if old := current.Swap(s); old != nil {
		old.close()
	}
	process.Log(context.Background(), role.noteLevel(), "start",
		"role", string(role), "version", opts.Version, "command", opts.Command)
	if unknown != "" {
		logger.Warn("unknown "+levelVariable+"; logging at info", "value", unknown)
	}
	if err != nil {
		logger.Warn("can't write the log file", "error", err)
	}
}

// Close notes the process's exit, with its exit status and how long it ran,
// and closes its log file. Records after it go nowhere, as before Init.
// Without an Init before it, it does nothing.
func Close(status int) {
	s := current.Load()
	if s == nil {
		return
	}
	process.Log(context.Background(), s.role.noteLevel(), "exit",
		"status", status, "duration", time.Since(s.started).Round(time.Millisecond))
	if current.CompareAndSwap(s, nil) {
		s.close()
	}
}

// output is where records are written: the file, the mirror, both, or
// nowhere, which is nil.
func output(file *logFile, mirror io.Writer) io.Writer {
	switch {
	case file != nil && mirror != nil:
		return tee{file, mirror}
	case file != nil:
		return file
	default:
		return mirror
	}
}

// forwarder hands each record to the handler of the session under way, or to
// none. A logger For gives out holds one for good, so the attributes and
// groups the logger is derived with are replayed onto the session's handler
// at every record: bound once, they'd tie the logger to the session of the
// moment, which is none at package level.
type forwarder struct {
	derive []func(slog.Handler) slog.Handler
}

func (f forwarder) Enabled(ctx context.Context, level slog.Level) bool {
	s := current.Load()
	return s != nil && s.handler.Enabled(ctx, level)
}

func (f forwarder) Handle(ctx context.Context, r slog.Record) error {
	s := current.Load()
	if s == nil {
		return nil
	}
	h := s.handler
	for _, step := range f.derive {
		h = step(h)
	}
	return h.Handle(ctx, r)
}

func (f forwarder) WithAttrs(attrs []slog.Attr) slog.Handler {
	return f.then(func(h slog.Handler) slog.Handler { return h.WithAttrs(attrs) })
}

func (f forwarder) WithGroup(name string) slog.Handler {
	return f.then(func(h slog.Handler) slog.Handler { return h.WithGroup(name) })
}

func (f forwarder) then(step func(slog.Handler) slog.Handler) forwarder {
	return forwarder{derive: append(slices.Clip(f.derive), step)}
}
