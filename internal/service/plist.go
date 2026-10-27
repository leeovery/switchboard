package service

import (
	"encoding/xml"
	"slices"
	"strconv"
	"strings"
)

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
// its SoftResourceLimits", or "" at the top. A dictionary in both is set
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
// a flag by its name, given otherwise or more than once, and what's not a
// flag, the program and its command, as "another program".
func argumentDifferences(want, have []string, in string) []string {
	wantPlain, wantFlags, _ := arguments(want)
	havePlain, haveFlags, repeated := arguments(have)
	var found []string
	if !slices.Equal(wantPlain, havePlain) {
		found = append(found, "another program"+in)
	}
	found = append(found, differences(wantFlags, haveFlags, in)...)
	for _, name := range repeated {
		found = append(found, "a repeated "+name+in)
	}
	return found
}

// arguments splits a program's arguments as serve parses its flags: what's
// not a flag, the program first; its flags by name, each with its value,
// given after = or as the argument after it, the last given of a flag given
// more than once, --config's as launchd runs the router finds it; and the
// names of those given more than once.
func arguments(program []string) (plain []string, flags []entry, repeated []string) {
	for i := 0; i < len(program); i++ {
		arg := program[i]
		if i == 0 || !strings.HasPrefix(arg, "-") {
			plain = append(plain, arg)
			continue
		}
		name, value, given := strings.Cut(arg, "=")
		if !given && i+1 < len(program) && (slices.Contains(valueFlags, name) || !strings.HasPrefix(program[i+1], "-")) {
			i++
			value = program[i]
		}
		if name == configFlag {
			value = asLaunchdRuns(value)
		}
		flag := entry{key: name, value: node{XMLName: xml.Name{Local: "string"}, Text: value}}
		if j := slices.IndexFunc(flags, func(e entry) bool { return e.key == name }); j >= 0 {
			flags[j] = flag
			if !slices.Contains(repeated, name) {
				repeated = append(repeated, name)
			}
			continue
		}
		flags = append(flags, flag)
	}
	return plain, flags, repeated
}
