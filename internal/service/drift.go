package service

import (
	"encoding/xml"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
)

// environmentKey is the plist's key for the variables carried.
const environmentKey = "EnvironmentVariables"

// Options reads back the options the plist in place was installed with: the
// binary it runs, the config file and the log level it gives the router, and
// the variables carried and the log launchd writes, which Install keeps in
// place of those where it's installed. It fails for a plist it can't read, or
// that isn't XML, as a binary one isn't.
func (s *Service) Options() (InstallOptions, error) {
	installed, err := s.readPlist()
	if err != nil {
		return InstallOptions{}, err
	}
	program, _ := lookup(installed, "ProgramArguments")
	opts := optionsOf(program.strings())
	opts.environment = map[string]string{}
	variables, _ := lookup(installed, environmentKey)
	for _, v := range variables.entries() {
		if slices.Contains(carried, v.key) {
			opts.environment[v.key] = v.value.Text
		}
	}
	log, _ := lookup(installed, "StandardOutPath")
	opts.log = log.Text
	return opts, nil
}

// OtherEnvironment names the variables carried that are set otherwise where
// the service is asked about than in the environment opts keep, as Options
// read it back: none for opts that keep none.
func (s *Service) OtherEnvironment(opts InstallOptions) ([]string, error) {
	if opts.environment == nil {
		return nil, nil
	}
	here, err := carry(s.cfg.Getenv)
	if err != nil {
		return nil, err
	}
	var other []string
	for _, name := range carried {
		value := ""
		if i := slices.IndexFunc(here, func(v variable) bool { return v.Name == name }); i >= 0 {
			value = here[i].Value
		}
		if value != opts.environment[name] {
			other = append(other, name)
		}
	}
	return other, nil
}

// Drift says how the plist in place differs from the one Install would write
// for opts, a difference a phrase, such as "no ProcessType", naming the keys
// and the arguments that differ but never their values: none when launchd
// would read the two alike, whatever their order of keys or their
// whitespace. The variables carried never count: they differ as the shells
// installing from differ, never as switchboards do, and OtherEnvironment
// says how. It fails as Options does.
func (s *Service) Drift(opts InstallOptions) ([]string, error) {
	installed, err := s.readPlist()
	if err != nil {
		return nil, err
	}
	a, err := s.describe(opts)
	if err != nil {
		return nil, err
	}
	data, err := a.plist()
	if err != nil {
		return nil, fmt.Errorf("describe the service: %w", err)
	}
	want, err := parsePlist(data)
	if err != nil {
		return nil, fmt.Errorf("read back the service's plist as written: %w", err)
	}
	return differences(withoutEnvironment(want), withoutEnvironment(installed), ""), nil
}

// withoutEnvironment is entries without the variables carried.
func withoutEnvironment(entries []entry) []entry {
	return slices.DeleteFunc(entries, func(e entry) bool { return e.key == environmentKey })
}

// readPlist reads the plist in place, its top dictionary's entries.
func (s *Service) readPlist() ([]entry, error) {
	data, err := os.ReadFile(s.Plist())
	if err != nil {
		return nil, fmt.Errorf("read the service's plist: %w", err)
	}
	entries, err := parsePlist(data)
	if err != nil {
		return nil, fmt.Errorf("parse the service's plist, %s: %w", s.Plist(), err)
	}
	return entries, nil
}

// node is an element of a property list.
type node struct {
	XMLName xml.Name
	Text    string `xml:",chardata"`
	Nodes   []node `xml:",any"`
}

// entry is one of a dictionary's keys, with its value.
type entry struct {
	key   string
	value node
}

// parsePlist returns the entries of a plist's top dictionary: none for one
// with no dictionary.
func parsePlist(data []byte) ([]entry, error) {
	var doc struct {
		Dict node `xml:"dict"`
	}
	if err := xml.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	return doc.Dict.entries(), nil
}

// entries are a dictionary's, each key paired with the element after it.
func (n node) entries() []entry {
	var entries []entry
	for i := 0; i+1 < len(n.Nodes); i++ {
		if n.Nodes[i].XMLName.Local == "key" {
			entries = append(entries, entry{key: n.Nodes[i].Text, value: n.Nodes[i+1]})
			i++
		}
	}
	return entries
}

// strings are the text of an array's elements.
func (n node) strings() []string {
	texts := make([]string, len(n.Nodes))
	for i, element := range n.Nodes {
		texts[i] = element.Text
	}
	return texts
}

// canonical is the value as launchd reads it, written so that two values
// launchd reads alike are written alike: a dictionary's keys in order, and
// the whitespace between elements, and around a number or a date, dropped.
func (n node) canonical() string {
	kind := n.XMLName.Local
	switch kind {
	case "dict":
		entries := n.entries()
		slices.SortFunc(entries, func(a, b entry) int { return strings.Compare(a.key, b.key) })
		parts := make([]string, len(entries))
		for i, e := range entries {
			parts[i] = strconv.Quote(e.key) + ":" + e.value.canonical()
		}
		return "dict{" + strings.Join(parts, ",") + "}"
	case "array":
		parts := make([]string, len(n.Nodes))
		for i, element := range n.Nodes {
			parts[i] = element.canonical()
		}
		return "array[" + strings.Join(parts, ",") + "]"
	case "string", "key":
		return kind + " " + strconv.Quote(n.Text)
	}
	return kind + " " + strconv.Quote(strings.TrimSpace(n.Text))
}

// lookup returns the value of key among entries, and whether it's there.
func lookup(entries []entry, key string) (node, bool) {
	i := slices.IndexFunc(entries, func(e entry) bool { return e.key == key })
	if i < 0 {
		return node{}, false
	}
	return entries[i].value, true
}

// differences are how have's entries differ from want's, in want's order,
// then those want hasn't, each phrase followed by in, where they are: " in
// its EnvironmentVariables", or "" at the top. A dictionary in both is set
// apart key by key, and an array in both, as ProgramArguments is, argument
// by argument.
func differences(want, have []entry, in string) []string {
	var found []string
	for _, w := range want {
		h, ok := lookup(have, w.key)
		switch {
		case !ok:
			found = append(found, "no "+w.key+in)
		case w.value.canonical() == h.canonical():
		case w.value.XMLName.Local == "dict" && h.XMLName.Local == "dict":
			found = append(found, differences(w.value.entries(), h.entries(), " in its "+w.key)...)
		case w.value.XMLName.Local == "array" && h.XMLName.Local == "array":
			found = append(found, argumentDifferences(w.value.strings(), h.strings(), " in its "+w.key)...)
		default:
			found = append(found, "another "+w.key+in)
		}
	}
	for _, h := range have {
		if _, ok := lookup(want, h.key); !ok {
			found = append(found, "an extra "+h.key+in)
		}
	}
	return found
}

// argumentDifferences are how a program's arguments have differ from want's:
// a flag by its name, and what's not a flag, the program and its command, as
// "another program".
func argumentDifferences(want, have []string, in string) []string {
	wantPlain, wantFlags := arguments(want)
	havePlain, haveFlags := arguments(have)
	var found []string
	if !slices.Equal(wantPlain, havePlain) {
		found = append(found, "another program"+in)
	}
	return append(found, differences(wantFlags, haveFlags, in)...)
}

// arguments splits a program's arguments into what's not a flag, the program
// first, and its flags, each with the argument after it as its value, unless
// that's a flag too.
func arguments(program []string) (plain []string, flags []entry) {
	for i := 0; i < len(program); i++ {
		arg := program[i]
		if i == 0 || !strings.HasPrefix(arg, "-") {
			plain = append(plain, arg)
			continue
		}
		flag := entry{key: arg, value: node{XMLName: xml.Name{Local: "string"}}}
		if i+1 < len(program) && !strings.HasPrefix(program[i+1], "-") {
			i++
			flag.value.Text = program[i]
		}
		flags = append(flags, flag)
	}
	return plain, flags
}

// optionsOf reads back the options a program, as agent writes it, was
// installed with: its binary, the first argument, and the flags agent gives
// it.
func optionsOf(program []string) InstallOptions {
	plain, flags := arguments(program)
	var opts InstallOptions
	if len(plain) > 0 {
		opts.Executable = plain[0]
	}
	if config, ok := lookup(flags, configFlag); ok {
		opts.Config = config.Text
	}
	if level, ok := lookup(flags, logLevelFlag); ok {
		opts.LogLevel = level.Text
	}
	return opts
}
