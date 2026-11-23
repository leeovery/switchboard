package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/leeovery/switchboard/internal/dashboard/watch"
	"github.com/leeovery/switchboard/internal/filestate"
	"github.com/leeovery/switchboard/internal/logs"
	"github.com/leeovery/switchboard/internal/router"
	"github.com/leeovery/switchboard/internal/theme"
)

// shownVariable names, in the environment of a dashboard restarted onto an
// upgrade, what the dashboard before showed, as carried has it, for it to
// open on in place of the preferences, which another dashboard may have
// kept otherwise since. It's taken once, as the dashboard opens, and nothing
// the dashboard runs inherits it. As with handover.Variable, one version
// writes it and the next reads it, so its form only ever gains fields.
const shownVariable = "SWITCHBOARD_DASHBOARD_SHOWN"

// carried is what a dashboard shows, as shownVariable carries it: the
// preferences as it shows them, in the preferences file's own shape, and
// the pool shown, by its id. A field a reader doesn't know it passes over.
type carried struct {
	Prefs json.RawMessage `json:"prefs"`
	Pool  string          `json:"pool,omitempty"`
}

// carriedShown is shown in shownVariable's form.
func carriedShown(shown watch.Shown) (string, error) {
	prefs, err := json.Marshal(shown.Prefs)
	if err != nil {
		return "", fmt.Errorf("carry what the dashboard shows: %w", err)
	}
	data, err := json.Marshal(carried{Prefs: prefs, Pool: shown.Pool})
	if err != nil {
		return "", fmt.Errorf("carry what the dashboard shows: %w", err)
	}
	return string(data), nil
}

// takeShown takes what the dashboard before showed, where this one is
// restarted onto an upgrade of it, from shownVariable, its preferences read
// as the preferences file's are: false where the variable is unset, or
// can't be read, as the log says. From here on, the variable is gone from
// the environment the commands read and pass on.
func (a *app) takeShown() (watch.Shown, bool) {
	value := a.Getenv(shownVariable)
	getenv, environ := a.Getenv, a.Environ
	a.Getenv = func(key string) string {
		if key == shownVariable {
			return ""
		}
		return getenv(key)
	}
	a.Environ = func() []string { return withoutVariable(environ(), shownVariable) }
	if value == "" {
		return watch.Shown{}, false
	}
	shown, err := shownIn(value)
	if err != nil {
		logger.Warn("can't read what the dashboard showed before it restarted; opening as the preferences keep it", "error", err)
		return watch.Shown{}, false
	}
	return shown, true
}

// shownIn reads what a dashboard showed from value, shownVariable's.
func shownIn(value string) (watch.Shown, error) {
	var c carried
	if err := json.Unmarshal([]byte(value), &c); err != nil {
		return watch.Shown{}, fmt.Errorf("%s: %w", shownVariable, err)
	}
	if len(c.Prefs) == 0 || string(c.Prefs) == "null" {
		return watch.Shown{}, fmt.Errorf("%s holds no preferences", shownVariable)
	}
	prefs, err := theme.PrefsOf(c.Prefs)
	if err != nil {
		return watch.Shown{}, fmt.Errorf("%s's preferences: %w", shownVariable, err)
	}
	return watch.Shown{Prefs: prefs, Pool: c.Pool}, nil
}

// withoutVariable is env without the variable called name.
func withoutVariable(env []string, name string) []string {
	return slices.DeleteFunc(slices.Clone(env), func(kv string) bool { return strings.HasPrefix(kv, name+"=") })
}

// dashboardUpgrade is how the dashboard opened with opts restarts itself
// onto an upgrade of this binary, as watch.Upgrade says: by the path it was
// run by, as it stands now; the router's health asked as the verbs ask it;
// the binary asked its version as --version prints it, as versionIn reads
// it; and the process replaced with it, as restartInto has it, run by the
// command line commandLine gives, never the one this process was started
// by, as setup's, which opens a dashboard at Done. None where the binary
// can't be found.
func (a *app) dashboardUpgrade(opts dashboardOptions) watch.Upgrade {
	exe, err := a.Executable()
	if err != nil {
		logger.Warn("can't find this switchboard binary, so the dashboard won't restart onto an upgrade", "error", err)
		return watch.Upgrade{}
	}
	argv, env := opts.commandLine(exe, a.configPath), a.Environ()
	return watch.Upgrade{
		Version: a.Version,
		Binary:  router.Watch(exe),
		Look:    func() filestate.State { return filestate.Of(exe) },
		Health:  a.routersHealth,
		Ask: func(ctx context.Context) (string, error) {
			out, err := a.Check(ctx, exe, []string{exe, "--version"}, env)
			if err != nil {
				return "", err
			}
			return versionIn(out)
		},
		Exec: func(shown watch.Shown) error {
			return a.restartInto(exe, argv, env, shown)
		},
	}
}

// commandLine is the command line a dashboard opened with o restarts by,
// run as the binary at exe: the dashboard command, with the config file
// named, where --config named one, and o's flags and interval, the interval
// only where it's given and isn't the default, so a release's default is
// taken up; but never --pool, as the pool shown is carried in shownVariable.
func (o dashboardOptions) commandLine(exe, config string) []string {
	argv := []string{exe, dashboardName}
	if config != "" {
		argv = append(argv, "--config", config)
	}
	for _, flag := range []struct {
		name string
		set  bool
	}{{probeFlag, o.probe}, {noNotifyFlag, o.noNotify}} {
		if flag.set {
			argv = append(argv, "--"+flag.name)
		}
	}
	if o.interval != 0 && o.interval != defaultInterval {
		argv = append(argv, o.interval.String())
	}
	return argv
}

// restartInto replaces this process with the binary at exe, run by argv, in
// env, the environment the dashboard was run in, carrying what it shows,
// shown, in shownVariable: the pool shown among it, which the dashboard
// opens on where the config still has it, never refusing it. The log is
// closed first, so it ends with this process's exit, and opened again where
// the process can't be replaced, saying so first.
func (a *app) restartInto(exe string, argv, env []string, shown watch.Shown) error {
	value, err := carriedShown(shown)
	if err != nil {
		return err
	}
	env = append(withoutVariable(env, shownVariable), shownVariable+"="+value)
	logs.Close(0)
	if err := a.Exec(exe, argv, env); err != nil {
		logs.Reopen(a.logging, "the dashboard wasn't replaced by its upgrade; it carries on")
		return fmt.Errorf("replace the dashboard with %s: %w", exe, err)
	}
	return nil
}
