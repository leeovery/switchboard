package logs_test

import (
	"bytes"
	"cmp"
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/leeovery/switchboard/internal/logs"
)

// cached and derived are taken before any test starts logging, as a
// package's logger is.
var (
	cached  = logs.For("cached")
	derived = cached.With("account", "work")
)

func TestLineFormat(t *testing.T) {
	path := start(t, logs.Options{})
	logs.For("probe").Info("probed account", "account", "work", "windows", 3)
	logs.Close(0)

	line := strings.TrimSuffix(readLog(t, path), "\n")
	format := regexp.MustCompile(`^time=(\S+) level=INFO msg="probed account" component=probe pid=(\d+) account=work windows=3$`)
	m := format.FindStringSubmatch(line)
	if m == nil {
		t.Fatalf("logged %q, want a line matching %s", line, format)
	}
	if !regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d\.\d{3}[+-]\d\d:\d\d$`).MatchString(m[1]) {
		t.Errorf("time = %s, want it to the millisecond with a numeric zone offset", m[1])
	}
	stamp, err := time.Parse("2006-01-02T15:04:05.000-07:00", m[1])
	if err != nil {
		t.Fatal(err)
	}
	if _, offset := stamp.Zone(); offset != zoneOffset(stamp) {
		t.Errorf("time = %s, want it in the local zone", m[1])
	}
	if age := time.Since(stamp); age < 0 || age > time.Minute {
		t.Errorf("time = %s, want about now", m[1])
	}
	if want := strconv.Itoa(os.Getpid()); m[2] != want {
		t.Errorf("pid = %s, want %s", m[2], want)
	}
}

func TestTimesInAttributesAreLocalToo(t *testing.T) {
	path := start(t, logs.Options{})
	due := time.Date(2026, 9, 28, 13, 42, 0, 0, time.UTC)
	logs.For("watch").Info("usage read", "next", due)
	logs.Close(0)

	if want := "next=" + due.Local().Format("2006-01-02T15:04:05.000-07:00"); !strings.Contains(readLog(t, path), want) {
		t.Errorf("logged %q, want it to show %s", readLog(t, path), want)
	}
}

func TestLevel(t *testing.T) {
	const warning = `level=WARN msg="unknown SWITCHBOARD_LOG_LEVEL; logging at info" component=logs`
	fromInfo := []string{"INFO", "WARN", "ERROR"}
	tests := []struct {
		name        string
		value       string
		want        []string
		wantWarning string
	}{
		{name: "info when unset", value: "", want: fromInfo},
		{name: "debug", value: "debug", want: []string{"DEBUG", "INFO", "WARN", "ERROR"}},
		{name: "debug in any case", value: "DeBuG", want: []string{"DEBUG", "INFO", "WARN", "ERROR"}},
		{name: "info", value: "INFO", want: fromInfo},
		{name: "warn", value: "warn", want: []string{"WARN", "ERROR"}},
		{name: "error", value: "Error", want: []string{"ERROR"}},
		{name: "info in place of a name that isn't a level", value: "verbose", want: fromInfo, wantWarning: "value=verbose"},
		{name: "info in place of warning, which isn't one", value: "warning", want: fromInfo, wantWarning: "value=warning"},
		{name: "info in place of a level with a space", value: "debug ", want: fromInfo, wantWarning: `value="debug "`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := start(t, logs.Options{Getenv: envFrom(map[string]string{"SWITCHBOARD_LOG_LEVEL": tt.value})})
			logEveryLevel(logs.For("test"))
			logs.Close(0)

			log := readLog(t, path)
			if got := levelsLogged(log); !slices.Equal(got, tt.want) {
				t.Errorf("logged records at %v, want %v", got, tt.want)
			}
			if warned, want := strings.Contains(log, warning), tt.wantWarning != ""; warned != want {
				t.Errorf("log reads\n%s\nwant a warning: %v", log, want)
			}
			if tt.wantWarning != "" && !hasLine(log, warning, tt.wantWarning) {
				t.Errorf("log reads\n%s\nwant the warning to name the value: %s", log, tt.wantWarning)
			}
		})
	}
}

func TestLevelGivenOutranksTheEnvironment(t *testing.T) {
	path := start(t, logs.Options{Level: slog.LevelDebug, Getenv: envFrom(map[string]string{"SWITCHBOARD_LOG_LEVEL": "loud"})})
	logEveryLevel(logs.For("test"))
	logs.Close(0)

	log := readLog(t, path)
	if got, want := levelsLogged(log), []string{"DEBUG", "INFO", "WARN", "ERROR"}; !slices.Equal(got, want) {
		t.Errorf("logged records at %v, want %v", got, want)
	}
	if strings.Contains(log, "SWITCHBOARD_LOG_LEVEL") {
		t.Errorf("log reads\n%s\nwant no word on the variable, which a level given overrides", log)
	}
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		name    string
		want    slog.Level
		wantErr string
	}{
		{name: "debug", want: slog.LevelDebug},
		{name: "Info", want: slog.LevelInfo},
		{name: "WARN", want: slog.LevelWarn},
		{name: "error", want: slog.LevelError},
		{name: "warning", wantErr: `unknown log level "warning": give debug, info, warn or error`},
		{name: "info+4", wantErr: `unknown log level "info+4": give debug, info, warn or error`},
		{name: "", wantErr: `unknown log level "": give debug, info, warn or error`},
	}
	for _, tt := range tests {
		got, err := logs.ParseLevel(tt.name)
		if gotErr := errorText(err); gotErr != tt.wantErr || (err == nil && got != tt.want) {
			t.Errorf("ParseLevel(%q) = %v, %q; want %v, %q", tt.name, got, gotErr, tt.want, tt.wantErr)
		}
	}
}

func TestBeforeInitRecordsGoNowhere(t *testing.T) {
	logger := logs.For("early")
	for _, level := range []slog.Level{slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError} {
		if logger.Enabled(t.Context(), level) {
			t.Errorf("before Init, a logger takes records at %v", level)
		}
	}
	logger.Error("nowhere", "account", "work")
	logger.WithGroup("request").With("account", "work").Error("nowhere")
	logs.StdLogger("early", slog.LevelError).Print("nowhere")
	logs.Close(0)
}

func TestLoggersTakenBeforeInitWriteAfterIt(t *testing.T) {
	path := start(t, logs.Options{})
	cached.Info("cached")
	derived.Info("derived")
	logs.Close(0)
	cached.Info("after close")

	log := readLog(t, path)
	for _, want := range []string{`msg=cached component=cached pid=`, `msg=derived component=cached pid=` + strconv.Itoa(os.Getpid()) + ` account=work`} {
		if !strings.Contains(log, want) {
			t.Errorf("log reads\n%s\nwant a line with %s", log, want)
		}
	}
	if strings.Contains(log, "after close") {
		t.Errorf("log reads\n%s\nwant nothing from after Close", log)
	}
}

func TestStartAndExit(t *testing.T) {
	tests := []struct {
		name  string
		role  logs.Role
		level slog.Level
		// wantAt is the level they're logged at, or empty when they aren't.
		wantAt string
	}{
		{name: "the router's at info", role: logs.RoleRouter, level: slog.LevelInfo, wantAt: "INFO"},
		{name: "none of the router's at warn", role: logs.RoleRouter, level: slog.LevelWarn},
		{name: "a command's at debug", role: logs.RoleCLI, level: slog.LevelDebug, wantAt: "DEBUG"},
		{name: "none of a command's at info", role: logs.RoleCLI, level: slog.LevelInfo},
		{name: "a command's when no role's given", level: slog.LevelDebug, wantAt: "DEBUG"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := start(t, logs.Options{Role: tt.role, Level: tt.level, Version: "1.2.3", Command: "switchboard status"})
			time.Sleep(20 * time.Millisecond)
			logs.Close(3)

			log := readLog(t, path)
			if tt.wantAt == "" {
				if log != "" {
					t.Errorf("log reads\n%s\nwant nothing", log)
				}
				return
			}
			role := string(cmp.Or(tt.role, logs.RoleCLI))
			started := regexp.MustCompile(`(?m)^time=\S+ level=` + tt.wantAt + ` msg=start component=process pid=\d+ role=` + role + ` version=1\.2\.3 command="switchboard status"$`)
			exited := regexp.MustCompile(`(?m)^time=\S+ level=` + tt.wantAt + ` msg=exit component=process pid=\d+ status=3 duration=(\S+)$`)
			if !started.MatchString(log) {
				t.Errorf("log reads\n%s\nwant a line matching %s", log, started)
			}
			m := exited.FindStringSubmatch(log)
			if m == nil {
				t.Fatalf("log reads\n%s\nwant a line matching %s", log, exited)
			}
			if ran, err := time.ParseDuration(m[1]); err != nil || ran < 20*time.Millisecond {
				t.Errorf("exit noted a duration of %s, want the 20ms or more the process ran", m[1])
			}
		})
	}
}

func TestLogFilesArePrivate(t *testing.T) {
	state := filepath.Join(t.TempDir(), "state")
	dir := logs.Dir(state)
	path := start(t, logs.Options{Dir: dir, Role: logs.RoleRouter})
	logs.Close(0)

	for name, want := range map[string]fs.FileMode{state: 0o700, dir: 0o700, path: 0o600} {
		info, err := os.Stat(name)
		if err != nil {
			t.Fatal(err)
		}
		if got := info.Mode().Perm(); got != want {
			t.Errorf("%s has mode %v, want %v", name, got, want)
		}
	}
}

func TestMirror(t *testing.T) {
	var mirror bytes.Buffer
	path := start(t, logs.Options{Role: logs.RoleRouter, Mirror: &mirror})
	logs.For("router").Info("listening", "address", "127.0.0.1:4747")
	logs.Close(0)

	log := readLog(t, path)
	if mirror.String() != log {
		t.Errorf("mirror took\n%s\nwant what the file took\n%s", mirror.String(), log)
	}
	if len(strings.Split(strings.TrimSpace(log), "\n")) != 3 {
		t.Errorf("log reads\n%s\nwant start, the record and exit", log)
	}
}

func TestLoggingWhenTheFileCantBeOpened(t *testing.T) {
	blocked := filepath.Join(t.TempDir(), "logs")
	if err := os.WriteFile(blocked, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		dir  string
	}{
		{name: "a file where the directory should be", dir: blocked},
		{name: "no directory", dir: ""},
		{name: "a relative directory", dir: filepath.Join("relative", "logs")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			var mirror bytes.Buffer
			logs.Init(logs.Options{Dir: tt.dir, Role: logs.RoleRouter, Mirror: &mirror})
			logs.For("router").Info("still mirrored")
			logs.Close(0)

			for _, want := range []string{"msg=start", `level=WARN msg="can't write the log file" component=logs`, `msg="still mirrored"`, "msg=exit"} {
				if !strings.Contains(mirror.String(), want) {
					t.Errorf("mirror took\n%s\nwant a line with %s", mirror.String(), want)
				}
			}
			if entries, err := os.ReadDir("."); err != nil || len(entries) > 0 {
				t.Errorf("working directory holds %v (%v), want nothing written there", entries, err)
			}
		})
	}

	t.Run("without a mirror", func(t *testing.T) {
		logs.Init(logs.Options{Dir: blocked, Role: logs.RoleCLI})
		logger := logs.For("cli")
		if logger.Enabled(t.Context(), slog.LevelError) {
			t.Error("with nowhere to write, a logger takes records")
		}
		logger.Error("nowhere")
		logs.Close(0)
	})
}

func TestInitAgainReplacesTheFirst(t *testing.T) {
	first := start(t, logs.Options{})
	second := start(t, logs.Options{})
	logs.For("test").Info("to the second")
	logs.Close(0)

	if log := readLog(t, first); strings.Contains(log, "to the second") {
		t.Errorf("first log reads\n%s\nwant nothing after the second Init", log)
	}
	if log := readLog(t, second); !strings.Contains(log, `msg="to the second"`) {
		t.Errorf("second log reads\n%s\nwant the record", log)
	}
}

func TestStdLogger(t *testing.T) {
	errorLog := logs.StdLogger("router", slog.LevelWarn)
	quiet := logs.StdLogger("router", slog.LevelDebug)
	path := start(t, logs.Options{})
	errorLog.Printf("http: TLS handshake error from %s: EOF", "127.0.0.1:52100")
	quiet.Print("below the level")
	logs.Close(0)

	log := readLog(t, path)
	if want := `level=WARN msg="http: TLS handshake error from 127.0.0.1:52100: EOF" component=router pid=`; !strings.Contains(log, want) {
		t.Errorf("log reads\n%s\nwant a line with %s", log, want)
	}
	if strings.Contains(log, "below the level") {
		t.Errorf("log reads\n%s\nwant nothing below info", log)
	}
}

// start starts logging with opts, in a directory of the test's own unless
// opts names one, until the test ends. It returns the log's path.
func start(t *testing.T, opts logs.Options) string {
	t.Helper()
	if opts.Dir == "" {
		opts.Dir = t.TempDir()
	}
	logs.Init(opts)
	t.Cleanup(func() { logs.Close(0) })
	return cmp.Or(opts.Role, logs.RoleCLI).Path(opts.Dir)
}

// readLog returns what the log at path holds: nothing when there's no log.
func readLog(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		t.Fatal(err)
	}
	return string(data)
}

// hasLine reports whether a line of log contains every one of parts.
func hasLine(log string, parts ...string) bool {
	for line := range strings.Lines(log) {
		if !slices.ContainsFunc(parts, func(part string) bool { return !strings.Contains(line, part) }) {
			return true
		}
	}
	return false
}

// logEveryLevel logs a record at each level, saying which.
func logEveryLevel(logger *slog.Logger) {
	logger.Debug("at DEBUG")
	logger.Info("at INFO")
	logger.Warn("at WARN")
	logger.Error("at ERROR")
}

// levelsLogged lists the levels of logEveryLevel's records in log.
func levelsLogged(log string) []string {
	var levels []string
	for _, level := range []string{"DEBUG", "INFO", "WARN", "ERROR"} {
		if strings.Contains(log, `msg="at `+level+`"`) {
			levels = append(levels, level)
		}
	}
	return levels
}

// zoneOffset is the local zone's offset from UTC at t, in seconds.
func zoneOffset(t time.Time) int {
	_, offset := t.Local().Zone()
	return offset
}

// envFrom returns a getenv backed by vars, so tests never read the real environment.
func envFrom(vars map[string]string) func(string) string {
	return func(key string) string { return vars[key] }
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
