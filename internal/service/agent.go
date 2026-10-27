package service

import (
	"bytes"
	"cmp"
	"fmt"
	"maps"
	"slices"
	"strings"
	"text/template"
	"time"

	"github.com/leeovery/switchboard/internal/router"
)

const (
	// exitTimeout is how long launchd gives the router to stop before it kills
	// it: the longest its stop waits on what's still going, and saveMargin
	// after. launchd's own default, 20 seconds, is shorter than the drain
	// alone.
	exitTimeout = router.StopTimeout + saveMargin
	// saveMargin is what a stopping router has, past the longest it waits, to
	// save its state and write the request ledger's last lines.
	saveMargin = 7 * time.Second
)

// configFlag and logLevelFlag are serve's flags for the config file and the
// level the router logs at, which the service is installed with.
const (
	configFlag   = "--config"
	logLevelFlag = "--log-level"
)

// valueFlags are serve's flags that take a value: given without one after
// =, the argument after the flag is its value, whatever it reads.
var valueFlags = []string{configFlag, logLevelFlag}

// carried are the variables the service is given as they're set where it's
// installed, so it finds its config and its state where the CLI does, and
// Claude Code's config directory, where setup puts the skill, and logs as
// much as it would.
var carried = []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "SWITCHBOARD_CONFIG", "SWITCHBOARD_LOG_LEVEL", "CLAUDE_CONFIG_DIR"}

// madeAbsolute are the variables carried that name a file or a directory,
// which can be named relative to where the service is installed: they're
// carried absolute, as launchd runs the router elsewhere. XDG's directories
// are carried as they stand, as switchboard ignores a relative one, the
// router and the CLI alike.
var madeAbsolute = []string{"SWITCHBOARD_CONFIG", "CLAUDE_CONFIG_DIR"}

// agent is the LaunchAgent, as its plist describes it.
type agent struct {
	Label string
	// Program is what launchd runs: the program and its arguments.
	Program []string
	// Environment is what launchd sets in the program's environment.
	Environment []variable
	// Log is where launchd writes what the program prints, such as a crash's
	// output.
	Log string
	// ExitTimeOut is how many seconds launchd gives the program to stop
	// before it kills it.
	ExitTimeOut int
}

// variable is one of an environment's variables.
type variable struct {
	Name, Value string
}

// agent describes the LaunchAgent that serves as opts says, with the
// switchboard binary as prepare finds it, the config file and the log level
// given, when they are, the environment opts keep, else the variables
// carried, and its log in the state directory. launchd runs switchboard
// itself: the router reads the tokens from their files, so it needs nothing
// of the user's environment but the variables carried.
func (s *Service) agent(opts InstallOptions) (agent, error) {
	program := []string{opts.Executable, "serve"}
	config, err := absolute(opts.Config)
	if err != nil {
		return agent{}, fmt.Errorf("find the config file: %w", err)
	}
	if config != "" {
		program = append(program, configFlag, config)
	}
	if opts.LogLevel != "" {
		program = append(program, logLevelFlag, opts.LogLevel)
	}
	environment, err := s.variables(opts)
	if err != nil {
		return agent{}, err
	}
	return agent{Label: Label, Program: program, Environment: environment, Log: s.Log(), ExitTimeOut: int(exitTimeout / time.Second)}, nil
}

// variables are what launchd is to set in the router's environment: those
// opts keep, whole, as Options read them back, the variables carried first;
// else the variables carried, as they're set where the service is installed.
func (s *Service) variables(opts InstallOptions) ([]variable, error) {
	if opts.environment == nil {
		return carry(s.cfg.Getenv)
	}
	names := slices.Sorted(maps.Keys(opts.environment))
	slices.SortStableFunc(names, func(a, b string) int { return cmp.Compare(carriedRank(a), carriedRank(b)) })
	environment := make([]variable, len(names))
	for i, name := range names {
		environment[i] = variable{Name: name, Value: opts.environment[name]}
	}
	return environment, nil
}

// carriedRank is where name comes among the variables carried, those that
// aren't after them all.
func carriedRank(name string) int {
	if i := slices.Index(carried, name); i >= 0 {
		return i
	}
	return len(carried)
}

// carry is the variables carried that getenv says are set, those naming a
// file or a directory made absolute.
func carry(getenv func(string) string) ([]variable, error) {
	var environment []variable
	for _, name := range carried {
		value := getenv(name)
		if value == "" {
			continue
		}
		if slices.Contains(madeAbsolute, name) {
			var err error
			if value, err = absolute(value); err != nil {
				return nil, fmt.Errorf("find %s: %w", name, err)
			}
		}
		environment = append(environment, variable{Name: name, Value: value})
	}
	return environment, nil
}

// plist is the agent's plist, which launchd loads it from: it runs the
// program at load, which is at every login too, and again whenever it
// stops, and gives it ExitTimeOut to stop when it's asked to. It runs it as
// an interactive job, as launchd would otherwise throttle the CPU and I/O of
// a router every Claude Code request waits on.
func (a agent) plist() ([]byte, error) {
	var b bytes.Buffer
	if err := plistTemplate.Execute(&b, a); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// plistTemplate writes an agent as a property list, in the XML form
// launchd's own are in.
var plistTemplate = template.Must(template.New("plist").Funcs(template.FuncMap{"xml": xmlText}).Parse(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>{{xml .Label}}</string>
	<key>ProgramArguments</key>
	<array>
	{{- range .Program}}
		<string>{{xml .}}</string>
	{{- end}}
	</array>
	{{- with .Environment}}
	<key>EnvironmentVariables</key>
	<dict>
		{{- range .}}
		<key>{{xml .Name}}</key>
		<string>{{xml .Value}}</string>
		{{- end}}
	</dict>
	{{- end}}
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
	<key>ExitTimeOut</key>
	<integer>{{.ExitTimeOut}}</integer>
	<key>ProcessType</key>
	<string>Interactive</string>
	<key>StandardOutPath</key>
	<string>{{xml .Log}}</string>
	<key>StandardErrorPath</key>
	<string>{{xml .Log}}</string>
</dict>
</plist>
`))

// xmlText escapes text for an XML element to hold.
var xmlText = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace
