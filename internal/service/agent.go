package service

import (
	"bytes"
	"encoding/xml"
	"fmt"
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
// switchboard binary as prepare finds it, and the config file and the log
// level given, when they are. launchd runs switchboard itself: the router
// reads the tokens from their files, so it needs nothing of the user's
// environment but the variables carried.
func (s *Service) agent(opts InstallOptions) (agent, error) {
	program := []string{opts.Executable, "serve"}
	config, err := absolute(opts.Config)
	if err != nil {
		return agent{}, fmt.Errorf("find the config file: %w", err)
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
// installed, those naming a file or a directory made absolute.
func (s *Service) environment() ([]variable, error) {
	var environment []variable
	for _, name := range carried {
		value := s.cfg.Getenv(name)
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

// binaryOf returns the program a LaunchAgent's plist has launchd run, as the
// first of its ProgramArguments names it: "" for a plist that names none, or
// isn't XML, as a binary one isn't.
func binaryOf(plist []byte) string {
	var doc struct {
		Dict struct {
			Entries []struct {
				XMLName xml.Name
				Text    string   `xml:",chardata"`
				Strings []string `xml:"string"`
			} `xml:",any"`
		} `xml:"dict"`
	}
	if xml.Unmarshal(plist, &doc) != nil {
		return ""
	}
	entries := doc.Dict.Entries
	for i := 1; i < len(entries); i++ {
		key, value := entries[i-1], entries[i]
		if key.XMLName.Local == "key" && key.Text == "ProgramArguments" && len(value.Strings) > 0 {
			return value.Strings[0]
		}
	}
	return ""
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
