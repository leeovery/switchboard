package service

import (
	"bytes"
	"strings"
	"text/template"
	"time"

	"github.com/leeovery/switchboard/internal/router"
)

const (
	// zsh runs the launcher that loads an env file: with -f, reading none of
	// the user's startup files, such as ~/.zshenv, which would otherwise run
	// in the router's launcher, alongside every token.
	zsh = "/bin/zsh"
	// loadEnv is that launcher: it sources the env file, its first argument,
	// then becomes the switchboard at its second, serving with the rest. The
	// paths come as arguments, never pasted into the script, so no path can
	// be read as script. Its $0 is switchboard, which zsh's errors, such as
	// an env file gone missing, name in launchd's log.
	loadEnv = `source "$1" && exec "$2" serve "${@:3}"`
	// exitTimeout is how long launchd gives the router to stop before it
	// kills it: the time the router gives requests in flight to finish, and
	// time after to save its state and post what's due. launchd's own
	// default, 20 seconds, is shorter than the first alone.
	exitTimeout = router.DrainTimeout + 15*time.Second
)

// carried are the variables the service is given as they're set where it's
// installed, so it finds its config and its state where the CLI does, and
// logs as much as it would.
var carried = []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", configVariable, "SWITCHBOARD_LOG_LEVEL"}

// configVariable names the config file, which can be named relative to where
// the service is installed: it's carried absolute, as launchd runs the router
// elsewhere. XDG's directories are carried as they stand, as switchboard
// ignores a relative one, the router and the CLI alike.
const configVariable = "SWITCHBOARD_CONFIG"

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
// switchboard binary and env file, if any, as prepare finds them: through zsh
// sourcing the env file first, when there's one, and with the config file and
// the log level given, when they are.
func (s *Service) agent(opts InstallOptions) (agent, error) {
	program := []string{opts.Executable, "serve"}
	if opts.EnvFile != "" {
		program = []string{zsh, "-f", "-c", loadEnv, "switchboard", opts.EnvFile, opts.Executable}
	}
	config, err := absolute(opts.Config)
	if err != nil {
		return agent{}, err
	}
	if config != "" {
		program = append(program, "--config", config)
	}
	if opts.LogLevel != "" {
		program = append(program, "--log-level", opts.LogLevel)
	}
	environment, err := s.environment()
	if err != nil {
		return agent{}, err
	}
	return agent{Label: Label, Program: program, Environment: environment, Log: s.Log(), ExitTimeOut: int(exitTimeout / time.Second)}, nil
}

// environment is the variables carried that are set where the service is
// installed, the config file's made absolute.
func (s *Service) environment() ([]variable, error) {
	var environment []variable
	for _, name := range carried {
		value := s.cfg.Getenv(name)
		if value == "" {
			continue
		}
		if name == configVariable {
			var err error
			if value, err = absolute(value); err != nil {
				return nil, err
			}
		}
		environment = append(environment, variable{Name: name, Value: value})
	}
	return environment, nil
}

// plist is the agent's plist, which launchd loads it from: it runs the
// program at load, which is at every login too, and again whenever it
// stops, and gives it ExitTimeOut to stop when it's asked to.
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
	<key>StandardOutPath</key>
	<string>{{xml .Log}}</string>
	<key>StandardErrorPath</key>
	<string>{{xml .Log}}</string>
</dict>
</plist>
`))

// xmlText escapes text for an XML element to hold.
var xmlText = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace
