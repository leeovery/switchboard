package claude

import (
	"bytes"
	"encoding/json"
	"errors"

	"github.com/leeovery/switchboard/internal/ledger"
)

// asked is what Asks reads of a messages request's body, in its one pass over
// it: the model it asks for, and its shape, as the request ledger keeps it,
// of which the most tokens it asks for tell its quota check. Of the rest of
// the body, nothing is kept.
type asked struct {
	Model             string                       `json:"model"`
	MaxTokens         setting[int64]               `json:"max_tokens"`
	Messages          []entry                      `json:"messages"`
	System            count                        `json:"system"`
	Tools             []entry                      `json:"tools"`
	Thinking          setting[ledger.Thinking]     `json:"thinking"`
	Stream            setting[bool]                `json:"stream"`
	ToolChoice        setting[ledger.ToolChoice]   `json:"tool_choice"`
	Temperature       setting[float64]             `json:"temperature"`
	TopK              setting[int64]               `json:"top_k"`
	TopP              setting[float64]             `json:"top_p"`
	ServiceTier       setting[string]              `json:"service_tier"`
	OutputConfig      setting[ledger.OutputConfig] `json:"output_config"`
	Speed             setting[string]              `json:"speed"`
	InferenceGeo      setting[string]              `json:"inference_geo"`
	ContextManagement setting[contextManagement]   `json:"context_management"`
}

// readAsked reads body into req, reporting false where it isn't a JSON
// object. A field of a type the API wouldn't take is left unread, as a list
// that isn't one, and the rest read all the same.
func readAsked(body []byte, req *asked) bool {
	err := json.Unmarshal(body, req)
	mistyped, ok := errors.AsType[*json.UnmarshalTypeError](err)
	return err == nil || ok && mistyped.Field != ""
}

// shape is the request's shape, its body being size bytes.
func (a asked) shape(size int) ledger.Shape {
	return ledger.Shape{
		Bytes:             size,
		Messages:          len(a.Messages),
		System:            int(a.System),
		Tools:             len(a.Tools),
		MaxTokens:         a.MaxTokens.pointer(),
		Thinking:          a.Thinking.read,
		Stream:            a.Stream.pointer(),
		ToolChoice:        a.ToolChoice.read,
		Temperature:       a.Temperature.pointer(),
		TopK:              a.TopK.pointer(),
		TopP:              a.TopP.pointer(),
		ServiceTier:       a.ServiceTier.read,
		OutputConfig:      a.OutputConfig.read,
		Speed:             a.Speed.read,
		InferenceGeo:      a.InferenceGeo.read,
		ContextManagement: a.ContextManagement.read.kept(),
	}
}

// contextManagement is how a request's body has its context managed, as the
// API takes it: of the edits it asks for, the type of each is read, and
// nothing of their parameters.
type contextManagement struct {
	Edits []struct {
		Type string `json:"type"`
	} `json:"edits"`
}

// kept is the context management as the request ledger keeps it: the types
// of its edits, in order.
func (c contextManagement) kept() ledger.ContextManagement {
	var kept ledger.ContextManagement
	for _, edit := range c.Edits {
		kept.Edits = append(kept.Edits, edit.Type)
	}
	return kept
}

// entry is an entry of a list a request's body gives, counted, as a list's
// length, but never read: its UnmarshalJSON has the decoder pass over each
// whole, whatever it is, in the pass it makes over the body, rather than
// walk its fields or read the list again to count it.
type entry struct{}

func (*entry) UnmarshalJSON([]byte) error {
	return nil
}

// setting is a setting a request's body gives, read where it's what the API
// takes: one given as null, or as a value of another type, which the API
// would refuse, is passed over, failing nothing else of the body's reading.
// What the request ledger alone reads of an answer is read as settings are,
// so a field of another type than the API gives fails nothing of the
// answer's counting.
type setting[T any] struct {
	// read is the setting's value, the zero value where it isn't given.
	read  T
	given bool
}

func (s *setting[T]) UnmarshalJSON(data []byte) error {
	var read T
	if string(data) != "null" && json.Unmarshal(data, &read) == nil {
		s.read, s.given = read, true
	}
	return nil
}

// pointer is the setting's value, nil where it isn't given.
func (s setting[T]) pointer() *T {
	if !s.given {
		return nil
	}
	return new(s.read)
}

// count is how many entries a request's body gives where it takes one or a
// list of them, as its system prompt: a list's, counted without reading the
// entries, one for a single entry, and none for null.
type count int

func (c *count) UnmarshalJSON(data []byte) error {
	*c = count(entries(data))
	return nil
}

// entries counts the entries of value, a JSON value: an array's elements,
// none of null, and one of anything else.
func entries(value []byte) int {
	value = bytes.TrimSpace(value)
	switch {
	case len(value) == 0 || string(value) == "null":
		return 0
	case value[0] == '[':
		return elements(value)
	}
	return 1
}

// elements counts the top-level elements of array, a JSON array, passing
// over each string whole, as one can hold any of the characters that
// structure the array.
func elements(array []byte) int {
	if inner := bytes.TrimSpace(array[1:]); len(inner) == 0 || inner[0] == ']' {
		return 0
	}
	n, depth := 1, 0
	for i := 0; i < len(array); i++ {
		switch array[i] {
		case '"':
			i = closingQuote(array, i+1)
		case '[', '{':
			depth++
		case ']', '}':
			depth--
		case ',':
			if depth == 1 {
				n++
			}
		}
	}
	return n
}

// closingQuote returns where in data the string whose characters start at
// from ends: at the first quote after them that isn't escaped, as one is
// after a run of backslashes of odd length.
func closingQuote(data []byte, from int) int {
	for {
		quote := bytes.IndexByte(data[from:], '"')
		if quote < 0 {
			return len(data)
		}
		quote += from
		backslashes := 0
		for i := quote - 1; i >= from && data[i] == '\\'; i-- {
			backslashes++
		}
		if backslashes%2 == 0 {
			return quote
		}
		from = quote + 1
	}
}
