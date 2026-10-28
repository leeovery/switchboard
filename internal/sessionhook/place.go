package sessionhook

import (
	"encoding/json"
	"fmt"
)

// timeout is how many seconds Claude Code gives the hook before it gives up
// on it: far more than it takes, as it answers at once.
const timeout = 5

// entry is switchboard's hook, running command.
func entry(command string) object {
	return object{
		{key: "type", value: encodeString("command")},
		{key: "command", value: encodeString(command)},
		{key: "timeout", value: json.RawMessage(fmt.Sprint(timeout))},
	}
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
// groups: the first of its own in a group without a matcher, which runs for
// every session, corrected, and every other removed.
type placer struct {
	command string
	// found is set once any of switchboard's own entries is found, kept once
	// one is kept, and changed once one is corrected or removed.
	found, kept, changed bool
}

func (p *placer) outcome() Outcome {
	switch {
	case !p.found:
		return Installed
	case !p.kept, p.changed:
		return PutRight
	}
	return InPlace
}

// groups returns the groups with switchboard's entry put right among them,
// or in a group of its own added at the end, where none is kept.
func (p *placer) groups(groups []json.RawMessage) []json.RawMessage {
	placed := make([]json.RawMessage, 0, len(groups)+1)
	for _, raw := range groups {
		if group, kept := p.group(raw); kept {
			placed = append(placed, group)
		}
	}
	if !p.kept {
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
	everySession := matchesEverySession(group)
	kept := make([]json.RawMessage, 0, len(hooks))
	touched := false
	for _, hook := range hooks {
		placed, keep, changed := p.hook(hook, everySession)
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

// matchesEverySession reports whether group's hooks run for every session,
// whatever its source: where its matcher is missing, empty or "*", which
// Claude Code reads as matching everything.
func matchesEverySession(group object) bool {
	raw, ok := group.get("matcher")
	if !ok {
		return true
	}
	var matcher string
	return json.Unmarshal(raw, &matcher) == nil && (matcher == "" || matcher == "*")
}

// hook returns the hook put right where it's switchboard's, whether it's
// kept, as only the first of switchboard's in a group for every session is,
// and whether it changed.
func (p *placer) hook(raw json.RawMessage, everySession bool) (placed json.RawMessage, keep, changed bool) {
	hook, err := parseObject(raw)
	if err != nil || !isOurs(hook) {
		return raw, true, false
	}
	p.found = true
	if p.kept || !everySession {
		return nil, false, true
	}
	p.kept = true
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
