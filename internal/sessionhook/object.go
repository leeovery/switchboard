package sessionhook

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
)

// What a part of the settings is, where Claude Code reads another shape.
var (
	errNotObject = errors.New("not a JSON object")
	errNotArray  = errors.New("not a JSON array")
)

// member is a key of a JSON object, and its value as it's written.
type member struct {
	key   string
	value json.RawMessage
}

// object is a JSON object, its members in the order they're written, so
// one written again keeps the user's order, where a map would sort it.
type object []member

// parseObject reads data, a JSON object.
func parseObject(data []byte) (object, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := expectDelim(dec, '{'); err != nil {
		return nil, err
	}
	var o object
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return nil, err
		}
		key, ok := token.(string)
		if !ok {
			return nil, errNotObject
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, err
		}
		o = append(o, member{key: key, value: value})
	}
	if err := expectDelim(dec, '}'); err != nil {
		return nil, err
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: more follows it", errNotObject)
	}
	return o, nil
}

// expectDelim reads dec's next token, failing unless it's delim.
func expectDelim(dec *json.Decoder, delim json.Delim) error {
	token, err := dec.Token()
	if err != nil {
		return err
	}
	if token != delim {
		return errNotObject
	}
	return nil
}

// parseArray reads data, a JSON array, an item each as it's written.
func parseArray(data json.RawMessage) ([]json.RawMessage, error) {
	if trimmed := bytes.TrimLeft(data, " \t\r\n"); len(trimmed) == 0 || trimmed[0] != '[' {
		return nil, errNotArray
	}
	var items []json.RawMessage
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, err
	}
	return items, nil
}

// get returns key's value, and whether o has it. Of a key written twice, the
// last counts, as Claude Code reads it.
func (o object) get(key string) (json.RawMessage, bool) {
	if i := o.index(key); i >= 0 {
		return o[i].value, true
	}
	return nil, false
}

// set gives key value, where get finds it, or as a member added at the end.
func (o *object) set(key string, value json.RawMessage) {
	if i := o.index(key); i >= 0 {
		(*o)[i].value = value
		return
	}
	*o = append(*o, member{key: key, value: value})
}

// index is where get finds key among o's members, or -1.
func (o object) index(key string) int {
	for i, m := range slices.Backward(o) {
		if m.key == key {
			return i
		}
	}
	return -1
}

// encode writes o as JSON, each value as it was written.
func (o object) encode() json.RawMessage {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, m := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(encodeString(m.key))
		b.WriteByte(':')
		b.Write(m.value)
	}
	b.WriteByte('}')
	return b.Bytes()
}

// encodeArray writes items as a JSON array, each as it was written.
func encodeArray(items []json.RawMessage) json.RawMessage {
	var b bytes.Buffer
	b.WriteByte('[')
	for i, item := range items {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(item)
	}
	b.WriteByte(']')
	return b.Bytes()
}

// encodeString writes s as a JSON string, leaving <, > and & as they are, as
// Claude Code does, where json.Marshal would escape them.
func encodeString(s string) json.RawMessage {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(s); err != nil {
		panic(fmt.Sprintf("encode a string as JSON: %v", err))
	}
	return bytes.TrimSuffix(b.Bytes(), []byte{'\n'})
}
