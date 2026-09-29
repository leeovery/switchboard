package logs

import (
	"context"
	"encoding"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/leeovery/switchboard/internal/redact"
)

// timeLayout shows a time to the millisecond with its zone's offset, which it
// writes as +00:00 rather than Z, so every line reads alike.
const timeLayout = "2006-01-02T15:04:05.000-07:00"

// newHandler writes each record from level up to w as a line of logfmt, with
// its secrets hidden, or discards every record when w is nil.
func newHandler(w io.Writer, level slog.Leveler) slog.Handler {
	if w == nil {
		return slog.DiscardHandler
	}
	return redactor{next: slog.NewTextHandler(w, &slog.HandlerOptions{Level: level, ReplaceAttr: localTime})}
}

// localTime shows every time, the record's and any attribute's, in the local
// zone and timeLayout.
func localTime(_ []string, a slog.Attr) slog.Attr {
	if a.Value.Kind() == slog.KindTime {
		return slog.String(a.Key, a.Value.Time().Local().Format(timeLayout))
	}
	return a
}

// redactor hides secrets in every record on its way to the handler it wraps:
// anything shaped like a Claude token, in the message or in the text of any
// attribute, and the whole of any attribute keyed Authorization. The rest of
// the code never logs a token; this catches one that slips through.
type redactor struct {
	next slog.Handler
}

func (r redactor) Enabled(ctx context.Context, level slog.Level) bool {
	return r.next.Enabled(ctx, level)
}

func (r redactor) Handle(ctx context.Context, rec slog.Record) error {
	clean := slog.NewRecord(rec.Time, rec.Level, redact.Text(rec.Message), rec.PC)
	rec.Attrs(func(a slog.Attr) bool {
		clean.AddAttrs(redactAttr(a))
		return true
	})
	return r.next.Handle(ctx, clean)
}

func (r redactor) WithAttrs(attrs []slog.Attr) slog.Handler {
	return redactor{next: r.next.WithAttrs(redactAttrs(attrs))}
}

func (r redactor) WithGroup(name string) slog.Handler {
	return redactor{next: r.next.WithGroup(name)}
}

func redactAttrs(attrs []slog.Attr) []slog.Attr {
	clean := make([]slog.Attr, len(attrs))
	for i, a := range attrs {
		clean[i] = redactAttr(a)
	}
	return clean
}

// redactAttr returns a with its value's secrets hidden. A value that isn't of
// one of slog's own kinds, such as an error or a Stringer, becomes the text
// the handler would have shown for it, redacted.
func redactAttr(a slog.Attr) slog.Attr {
	if strings.EqualFold(a.Key, "authorization") {
		return slog.String(a.Key, redact.Placeholder)
	}
	v := a.Value.Resolve()
	switch v.Kind() {
	case slog.KindString:
		v = slog.StringValue(redact.Text(v.String()))
	case slog.KindGroup:
		v = slog.GroupValue(redactAttrs(v.Group())...)
	case slog.KindAny:
		if v.Any() != nil {
			v = slog.StringValue(redact.Text(text(v.Any())))
		}
	}
	return slog.Attr{Key: a.Key, Value: v}
}

// text is how slog's text handler shows a value that isn't one of slog's own
// kinds, such as an error or a Stringer.
func text(v any) string {
	switch v := v.(type) {
	case []byte:
		return string(v)
	case encoding.TextMarshaler:
		if data, err := marshalText(v); err == nil {
			return string(data)
		}
	}
	return fmt.Sprintf("%+v", v)
}

// marshalText guards against a MarshalText that panics, as one on a nil
// pointer can: a record must never take the process down.
func marshalText(m encoding.TextMarshaler) (data []byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("MarshalText panicked: %v", r)
		}
	}()
	return m.MarshalText()
}
