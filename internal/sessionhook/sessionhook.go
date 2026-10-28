// Package sessionhook is the hook switchboard puts in Claude Code's settings,
// which every session runs as it starts, however it was started: what it
// tells a session that didn't start through switchboard, and switchboard's
// entry in the settings, which setup installs or puts right.
package sessionhook

import (
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"

	"github.com/leeovery/switchboard/internal/skill"
)

// Event is the Claude Code hook event the hook runs on.
const Event = "SessionStart"

// Path returns Claude Code's settings file, which holds the hook:
// settings.json in its config directory.
func Path(getenv func(string) string, homeDir func() (string, error)) (string, error) {
	dir, err := skill.ConfigDir(getenv, homeDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "settings.json"), nil
}

// maxInput is the most of Claude Code's hook input that's read: a few
// hundred bytes in practice.
const maxInput = 64 << 10

// errInputTooLong is what ReadInput fails with for input past maxInput.
var errInputTooLong = fmt.Errorf("the hook's input is longer than %d bytes", maxInput)

// ReadInput reads Claude Code's hook input, a JSON object, from r, failing
// for more of it than a hook is ever given.
func ReadInput(r io.Reader) ([]byte, error) {
	input, err := io.ReadAll(io.LimitReader(r, maxInput+1))
	switch {
	case err != nil:
		return nil, fmt.Errorf("read the hook's input: %w", err)
	case len(input) > maxInput:
		return nil, errInputTooLong
	}
	return input, nil
}

// message is what a session that isn't going through switchboard shows the
// user. It's true of every session the hook reaches: one started from a
// shell whose PATH finds the real claude first, which setup puts right, and
// one an app starts with a Claude Code of its own, which nothing routes.
const message = "This session isn't going through switchboard, as the claude that started it isn't switchboard's: " +
	"from a terminal, switchboard setup puts that right; an app that runs its own Claude Code isn't routed."

// notice is what the hook prints, in the shape Claude Code reads a hook's
// output in: a message the user sees, and nothing Claude is told, which
// every headless run, such as a script's claude -p, would carry in its
// prompt.
type notice struct {
	SystemMessage string `json:"systemMessage"`
}

// Notice returns what the hook prints for a session starting with input,
// Claude Code's hook input, which started says switchboard started it: a
// notice, as a line of JSON, that it isn't going through switchboard, or nil
// to say nothing. A session switchboard started says nothing, nor does one
// cleared or compacted, which said it as it started; one starting or
// resumed, or whose input can't be read, which may be either, says it.
func Notice(input []byte, started bool) []byte {
	if started || !speaks(input) {
		return nil
	}
	out, err := json.Marshal(notice{SystemMessage: message})
	if err != nil {
		panic(fmt.Sprintf("encode the session hook's notice: %v", err))
	}
	return append(out, '\n')
}

// speaks reports whether a session starting with input, unmarked, hears it
// isn't going through switchboard: as it starts, or is resumed, or where its
// source can't be read.
func speaks(input []byte) bool {
	var hook struct {
		Source *string `json:"source"`
	}
	if err := json.Unmarshal(input, &hook); err != nil || hook.Source == nil {
		return true
	}
	switch *hook.Source {
	case "startup", "resume":
		return true
	}
	return false
}
