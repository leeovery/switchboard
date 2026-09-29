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
// installed, so it finds its config and its state where the CLI does.
var carried = []string{"XDG_CONFIG_HOME", "XDG_STATE_HOME", "SWITCHBOARD_CONFIG"}

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

// agent describes the LaunchAgent that serves with the switchboard at
// binary: through zsh sourcing envFile first, when there's one, and serving
// the config file at config, when there's one.
func (s *Service) agent(binary, envFile, config string) agent {
	program := []string{binary, "serve"}
	if envFile != "" {
		program = []string{zsh, "-f", "-c", loadEnv, "switchboard", envFile, binary}
	}
	if config != "" {
		program = append(program, "--config", config)
	}
	var environment []variable
	for _, name := range carried {
		if value := s.cfg.Getenv(name); value != "" {
			environment = append(environment, variable{Name: name, Value: value})
		}
	}
	return agent{Label: Label, Program: program, Environment: environment, Log: s.Log(), ExitTimeOut: int(exitTimeout / time.Second)}
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
