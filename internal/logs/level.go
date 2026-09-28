package logs

import (
	"fmt"
	"log/slog"
	"strings"
)

// levelVariable names the environment variable that sets how much is logged.
const levelVariable = "SWITCHBOARD_LOG_LEVEL"

// ParseLevel reads a level's name: debug, info, warn or error, in any case.
func ParseLevel(name string) (slog.Level, error) {
	switch strings.ToLower(name) {
	case "debug":
		return slog.LevelDebug, nil
	case "info":
		return slog.LevelInfo, nil
	case "warn":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	}
	return 0, fmt.Errorf("unknown log level %q: give debug, info, warn or error", name)
}

// level is the least severe level to log: the one given, else the one
// SWITCHBOARD_LOG_LEVEL names, else info. When the variable holds something
// that isn't a level's name, it returns that too, to warn of.
func (o Options) level() (level slog.Leveler, unknown string) {
	if o.Level != nil {
		return o.Level, ""
	}
	var name string
	if o.Getenv != nil {
		name = o.Getenv(levelVariable)
	}
	if name == "" {
		return slog.LevelInfo, ""
	}
	parsed, err := ParseLevel(name)
	if err != nil {
		return slog.LevelInfo, name
	}
	return parsed, ""
}
