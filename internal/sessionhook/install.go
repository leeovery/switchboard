package sessionhook

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/leeovery/switchboard/internal/atomicfile"
)

// Outcome is what Install did.
type Outcome int

const (
	// InPlace is switchboard's entry found as it should be: nothing is
	// written.
	InPlace Outcome = iota
	// Installed is the entry added, where there was none.
	Installed
	// PutRight is the entry corrected, or found more than once and the
	// others removed.
	PutRight
)

// timeout is how many seconds Claude Code gives the hook before it gives up
// on it: far more than it takes, as it answers at once.
const timeout = 5

// subcommand is what the hook runs switchboard with.
const subcommand = "hook session-start"

// newFileMode is the mode of a settings file Install makes.
const newFileMode fs.FileMode = 0o644

// Command returns the command the hook runs: switchboard at path, unless
// nothing there can be run, as when switchboard is removed, which leaves the
// hook doing nothing, and succeeding.
func Command(switchboard string) string {
	quoted := shellQuoted(switchboard)
	return "[ -x " + quoted + " ] && " + quoted + " " + subcommand + " || true"
}

// shellQuoted quotes s as one word for sh: between single quotes, each
// single quote in it ended, escaped and begun again.
func shellQuoted(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// isOurCommand reports whether command is shaped as Command shapes it, with
// any path: switchboard's, as a hook of the user's that merely mentions
// switchboard isn't.
func isOurCommand(command string) bool {
	rest, ok := strings.CutPrefix(command, "[ -x ")
	if !ok {
		return false
	}
	if rest, ok = cutQuoted(rest); !ok {
		return false
	}
	if rest, ok = strings.CutPrefix(rest, " ] && "); !ok {
		return false
	}
	if rest, ok = cutQuoted(rest); !ok {
		return false
	}
	return rest == " "+subcommand+" || true"
}

// cutQuoted cuts a word quoted as shellQuoted quotes it from the start of s,
// returning what follows it.
func cutQuoted(s string) (string, bool) {
	for {
		inside, ok := strings.CutPrefix(s, "'")
		if !ok {
			return "", false
		}
		_, after, ok := strings.Cut(inside, "'")
		if !ok {
			return "", false
		}
		if s, ok = strings.CutPrefix(after, `\'`); !ok {
			return after, true
		}
	}
}

// entry is switchboard's hook, running command.
func entry(command string) object {
	return object{
		{key: "type", value: encodeString("command")},
		{key: "command", value: encodeString(command)},
		{key: "timeout", value: json.RawMessage(fmt.Sprint(timeout))},
	}
}

// Install puts switchboard's entry, running switchboard at the path given,
// in Claude Code's settings file at path, or puts it right, and reports
// which, writing nothing where it's in place already. Every other setting and
// hook is left as it was, in its order, and the file is written whole, by
// its mode, through a link to it as to the file. A file that isn't missing,
// empty or a JSON object, or whose hooks aren't shaped as Claude Code reads
// them, is left as it is, and Install fails saying what's wrong.
func Install(path, switchboard string) (Outcome, error) {
	target, err := atomicfile.Target(path)
	if err != nil {
		return InPlace, fmt.Errorf("follow %s: %w", path, err)
	}
	data, mode, err := readSettings(target)
	if err != nil {
		return InPlace, err
	}
	settings, err := parseObject(data)
	if err != nil {
		return InPlace, fmt.Errorf("%s: %w", path, err)
	}
	outcome, err := place(&settings, Command(switchboard))
	if err != nil {
		return InPlace, fmt.Errorf("%s: %w", path, err)
	}
	if outcome == InPlace {
		return InPlace, nil
	}
	if err := writeSettings(target, settings, mode); err != nil {
		return InPlace, err
	}
	return outcome, nil
}

// readSettings reads the settings file at path, and its mode: an object with
// nothing in it, of newFileMode, where it's missing or holds only spaces.
func readSettings(path string) ([]byte, fs.FileMode, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return []byte("{}"), newFileMode, nil
	}
	if err != nil {
		return nil, 0, fmt.Errorf("read Claude Code's settings: %w", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, fmt.Errorf("read Claude Code's settings: %w", err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		data = []byte("{}")
	}
	return data, info.Mode().Perm(), nil
}

// writeSettings writes settings to path, indented as Claude Code writes them,
// making its directory.
func writeSettings(path string, settings object, mode fs.FileMode) error {
	var out bytes.Buffer
	if err := json.Indent(&out, settings.encode(), "", "  "); err != nil {
		return fmt.Errorf("write Claude Code's settings: %w", err)
	}
	out.WriteByte('\n')
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create Claude Code's config directory: %w", err)
	}
	if err := atomicfile.Write(path, out.Bytes(), mode); err != nil {
		return fmt.Errorf("write Claude Code's settings: %w", err)
	}
	return nil
}

// place puts switchboard's entry, running command, among settings' hooks,
// making hooks and its SessionStart where they're missing, and reports what
// it did.
func place(settings *object, command string) (Outcome, error) {
	hooks := object{}
	if raw, ok := settings.get("hooks"); ok {
		var err error
		if hooks, err = parseObject(raw); err != nil {
			return InPlace, fmt.Errorf("hooks: %w", err)
		}
	}
	var groups []json.RawMessage
	if raw, ok := hooks.get(Event); ok {
		var err error
		if groups, err = parseArray(raw); err != nil {
			return InPlace, fmt.Errorf("hooks.%s: %w", Event, err)
		}
	}
	p := placer{command: command}
	groups = p.groups(groups)
	outcome := p.outcome()
	if outcome != InPlace {
		hooks.set(Event, encodeArray(groups))
		settings.set("hooks", hooks.encode())
	}
	return outcome, nil
}

// placer puts switchboard's entry, running command, among SessionStart's
// groups: the first of its own it finds corrected, and any after it removed.
type placer struct {
	command string
	// found is set once one of switchboard's own entries is found, and
	// changed once one is corrected or removed.
	found, changed bool
}

func (p *placer) outcome() Outcome {
	switch {
	case !p.found:
		return Installed
	case p.changed:
		return PutRight
	}
	return InPlace
}

// groups returns the groups with switchboard's entry put right among them,
// or in a group of its own added at the end, where it's in none.
func (p *placer) groups(groups []json.RawMessage) []json.RawMessage {
	placed := make([]json.RawMessage, 0, len(groups)+1)
	for _, raw := range groups {
		if group, kept := p.group(raw); kept {
			placed = append(placed, group)
		}
	}
	if !p.found {
		placed = append(placed, object{{key: "hooks", value: encodeArray([]json.RawMessage{entry(p.command).encode()})}}.encode())
	}
	return placed
}

// group returns the group with switchboard's entries in it put right, and
// whether it's kept: one that held them alone, and has none left, isn't. A
// group not shaped as Claude Code reads one is the user's, and left as it
// is.
func (p *placer) group(raw json.RawMessage) (json.RawMessage, bool) {
	group, err := parseObject(raw)
	if err != nil {
		return raw, true
	}
	list, ok := group.get("hooks")
	if !ok {
		return raw, true
	}
	hooks, err := parseArray(list)
	if err != nil {
		return raw, true
	}
	kept := make([]json.RawMessage, 0, len(hooks))
	touched := false
	for _, hook := range hooks {
		placed, keep, changed := p.hook(hook)
		touched = touched || changed
		if keep {
			kept = append(kept, placed)
		}
	}
	if !touched {
		return raw, true
	}
	p.changed = true
	if len(kept) == 0 {
		return nil, false
	}
	group.set("hooks", encodeArray(kept))
	return group.encode(), true
}

// hook returns the hook put right where it's switchboard's, whether it's
// kept, as only the first of switchboard's is, and whether it changed.
func (p *placer) hook(raw json.RawMessage) (placed json.RawMessage, keep, changed bool) {
	hook, err := parseObject(raw)
	if err != nil || !isOurs(hook) {
		return raw, true, false
	}
	if p.found {
		return nil, false, true
	}
	p.found = true
	if !correct(&hook, entry(p.command)) {
		return raw, true, false
	}
	return hook.encode(), true, true
}

// isOurs reports whether hook is switchboard's, by its command.
func isOurs(hook object) bool {
	raw, ok := hook.get("command")
	if !ok {
		return false
	}
	var command string
	return json.Unmarshal(raw, &command) == nil && isOurCommand(command)
}

// correct gives hook each of want's members it hasn't, or has another value
// of, keeping its others, and reports whether it changed it.
func correct(hook *object, want object) bool {
	changed := false
	for _, m := range want {
		if have, ok := hook.get(m.key); ok && sameValue(have, m.value) {
			continue
		}
		hook.set(m.key, m.value)
		changed = true
	}
	return changed
}

// sameValue reports whether have holds the value want does, however it's
// written. want is a string or a number.
func sameValue(have, want json.RawMessage) bool {
	var h, w any
	if json.Unmarshal(have, &h) != nil || json.Unmarshal(want, &w) != nil {
		return false
	}
	// Comparing values of different types is false, not a panic, and want's
	// type is comparable.
	return h == w
}
